// SPDX-License-Identifier: GPL-3.0-only

package lsp

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime/debug"
	"sort"
	"strings"

	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
)

// CheckResult is the outcome of a one-shot diagnostic run on a single
// file. Diagnostics are LSP-shaped (same as a real editor session
// would receive). Panic is non-empty if the pipeline panicked, with
// a captured stack — useful for narrowing crashes that only show up
// inside the LSP.
type CheckResult struct {
	Diagnostics []protocol.Diagnostic
	Panic       string
}

// HoverResult is the outcome of a single hover request at a specific
// cursor position. Empty Markdown means the LSP returned no hover
// info — which is itself a useful debug signal when the user
// expects info to show up.
type HoverResult struct {
	Markdown string
	Panic    string
}

// DefResult is the outcome of a single goto-definition request.
// Locations is empty when the LSP returned no result.
type DefResult struct {
	Locations []protocol.Location
	Panic     string
}

// DefFile runs an LSP goto-definition request at the given 1-based
// line and column in filePath and returns the resulting locations.
func DefFile(ctx context.Context, filePath string, line, col uint32) DefResult {
	abs, err := filepath.Abs(filePath)
	if err != nil {
		return DefResult{}
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return DefResult{}
	}

	s := newOneShotServer(abs)
	docURI := protocol.DocumentURI(uri.File(abs))
	s.docs.Open(docURI, string(data), 1)

	var result DefResult
	func() {
		defer func() {
			if r := recover(); r != nil {
				result.Panic = fmt.Sprintf("%v\n\n%s", r, captureStack())
			}
		}()
		pos := protocol.Position{Line: line - 1, Character: col - 1}
		locs, _ := s.Definition(ctx, &protocol.DefinitionParams{
			TextDocumentPositionParams: protocol.TextDocumentPositionParams{
				TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
				Position:     pos,
			},
		})
		result.Locations = locs
	}()
	return result
}

// HoverFile runs an LSP hover request at the given 1-based line and
// column in filePath and returns the resulting markdown (or panic).
// Mirrors what an editor would do when the cursor lands on that
// position and waits for a hover popup.
func HoverFile(ctx context.Context, filePath string, line, col uint32) HoverResult {
	abs, err := filepath.Abs(filePath)
	if err != nil {
		return HoverResult{Markdown: err.Error()}
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return HoverResult{Markdown: err.Error()}
	}

	s := newOneShotServer(abs)
	docURI := protocol.DocumentURI(uri.File(abs))
	s.docs.Open(docURI, string(data), 1)

	var result HoverResult
	func() {
		defer func() {
			if r := recover(); r != nil {
				result.Panic = fmt.Sprintf("%v\n\n%s", r, captureStack())
			}
		}()
		// LSP positions are 0-based; the CLI takes 1-based for ergonomics.
		pos := protocol.Position{Line: line - 1, Character: col - 1}
		h, _ := s.Hover(ctx, &protocol.HoverParams{
			TextDocumentPositionParams: protocol.TextDocumentPositionParams{
				TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
				Position:     pos,
			},
		})
		if h == nil {
			return
		}
		result.Markdown = h.Contents.Value
	}()
	return result
}

// ScanResult is the outcome of running an LSP request at every cursor
// position in a file. Each Crash captures the line/col plus a stack
// trace, so a single scan can surface every position that triggers a
// panic. Used by `scampls scan` to narrow LSP crashes without making
// the user pinpoint the cursor location manually.
type ScanResult struct {
	Crashes []ScanCrash
}

// ScanCrash records a single panic seen during a position scan.
type ScanCrash struct {
	Request string // "completion", "hover", "signature"
	Line    uint32 // 1-based
	Col     uint32 // 1-based
	Panic   string // recovered value + stack
}

// CheckFile runs the same diagnostic pipeline a real editor session
// would run when the user opens or edits the given file. It builds a
// throw-away Server (bootstrapped with stdlib + user modules from the
// nearest scampi.mod), recovers from any panic, and returns the
// resulting diagnostics.
//
// The intended caller is the `scampls check` subcommand, used as a
// debug tool to reproduce LSP crashes deterministically without
// running an editor.
func CheckFile(ctx context.Context, filePath string) CheckResult {
	abs, err := filepath.Abs(filePath)
	if err != nil {
		return CheckResult{Diagnostics: errDiag(err)}
	}

	data, err := os.ReadFile(abs)
	if err != nil {
		return CheckResult{Diagnostics: errDiag(err)}
	}

	s := newOneShotServer(abs)

	var result CheckResult
	func() {
		defer func() {
			if r := recover(); r != nil {
				result.Panic = fmt.Sprintf("%v\n\n%s", r, captureStack())
			}
		}()
		docURI := protocol.DocumentURI(uri.File(abs))
		result.Diagnostics = s.evaluate(ctx, docURI, string(data))
	}()
	return result
}

// ScanFile runs every cursor-driven LSP request (completion, hover,
// signature help) at every line/column position in the file, with a
// per-call panic recover. Returns every position that crashed. If
// the slice is empty, no LSP request panicked anywhere in the file.
//
// This is the workhorse for narrowing crashes that only show up
// during interactive editing — instead of having the user pinpoint
// the cursor location, we brute-force every position and let the
// recover net catch the bad one.
func ScanFile(ctx context.Context, filePath string) (ScanResult, error) {
	abs, err := filepath.Abs(filePath)
	if err != nil {
		return ScanResult{}, err
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return ScanResult{}, err
	}

	s := newOneShotServer(abs)
	docURI := protocol.DocumentURI(uri.File(abs))
	s.docs.Open(docURI, string(data), 1)

	lines := strings.Split(string(data), "\n")

	var result ScanResult
	for li, line := range lines {
		// Test every column from 0 to len(line). The trailing position
		// (col == len(line)) matters because that's where the cursor
		// sits right after typing the last character on the line.
		for col := 0; col <= len(line); col++ {
			lineU := uint32(li)
			colU := uint32(col)
			if p := tryRequest(ctx, s, docURI, lineU, colU, "completion"); p != "" {
				result.Crashes = append(result.Crashes, ScanCrash{
					Request: "completion", Line: lineU + 1, Col: colU + 1, Panic: p,
				})
			}
			if p := tryRequest(ctx, s, docURI, lineU, colU, "hover"); p != "" {
				result.Crashes = append(result.Crashes, ScanCrash{
					Request: "hover", Line: lineU + 1, Col: colU + 1, Panic: p,
				})
			}
			if p := tryRequest(ctx, s, docURI, lineU, colU, "signature"); p != "" {
				result.Crashes = append(result.Crashes, ScanCrash{
					Request: "signature", Line: lineU + 1, Col: colU + 1, Panic: p,
				})
			}
		}
	}
	return result, nil
}

// tryRequest invokes a single LSP request at the given position with
// a panic recover. Returns the captured panic+stack on crash, or ""
// on clean execution.
func tryRequest(
	ctx context.Context,
	s *Server,
	docURI protocol.DocumentURI,
	line, col uint32,
	kind string,
) (recovered string) {
	defer func() {
		if r := recover(); r != nil {
			recovered = fmt.Sprintf("%v\n\n%s", r, captureStack())
		}
	}()
	pos := protocol.Position{Line: line, Character: col}
	tdoc := protocol.TextDocumentIdentifier{URI: docURI}
	switch kind {
	case "completion":
		_, _ = s.Completion(ctx, &protocol.CompletionParams{
			TextDocumentPositionParams: protocol.TextDocumentPositionParams{
				TextDocument: tdoc, Position: pos,
			},
		})
	case "hover":
		_, _ = s.Hover(ctx, &protocol.HoverParams{
			TextDocumentPositionParams: protocol.TextDocumentPositionParams{
				TextDocument: tdoc, Position: pos,
			},
		})
	case "signature":
		_, _ = s.SignatureHelp(ctx, &protocol.SignatureHelpParams{
			TextDocumentPositionParams: protocol.TextDocumentPositionParams{
				TextDocument: tdoc, Position: pos,
			},
		})
	}
	return ""
}

// newOneShotServer builds a Server populated with stdlib + user
// modules, mirroring what Initialize/loadModule/loadUserModules do
// in a real LSP session, but without any client/conn wiring. The
// workspace root is found by walking up from filePath looking for
// scampi.mod.
func newOneShotServer(filePath string) *Server {
	s := &Server{
		catalog:  NewCatalog(),
		modules:  bootstrapModules(),
		stubDefs: NewStubDefs(),
		docs:     NewDocuments(),
		log:      discardLogger(),
	}
	if root := findWorkspaceRoot(filePath); root != "" {
		s.rootDir = root
		s.loadModule()
		s.loadUserModules()
	}
	return s
}

// findWorkspaceRoot walks up from filePath looking for a scampi.mod
// file. Returns the first directory containing one, or "" if none
// is found before reaching the filesystem root.
func findWorkspaceRoot(filePath string) string {
	dir := filepath.Dir(filePath)
	for {
		if _, err := os.Stat(filepath.Join(dir, "scampi.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// FormatDiagnostics renders LSP diagnostics into the classic
// `file:line:col: msg` form for terminal output. Diagnostics are
// sorted by position so the output is stable.
func FormatDiagnostics(filePath string, diags []protocol.Diagnostic) string {
	if len(diags) == 0 {
		return ""
	}
	out := make([]protocol.Diagnostic, len(diags))
	copy(out, diags)
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i].Range.Start, out[j].Range.Start
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		return a.Character < b.Character
	})
	var b strings.Builder
	for _, d := range out {
		_, _ = fmt.Fprintf(
			&b,
			"%s:%d:%d: %s\n",
			filePath,
			d.Range.Start.Line+1,
			d.Range.Start.Character+1,
			d.Message,
		)
	}
	return b.String()
}

func discardLogger() *log.Logger {
	return log.New(io.Discard, "", 0)
}

func captureStack() string {
	return string(debug.Stack())
}

func errDiag(err error) []protocol.Diagnostic {
	return []protocol.Diagnostic{{
		Severity: protocol.DiagnosticSeverityError,
		Source:   diagSourceLSP,
		Message:  err.Error(),
	}}
}
