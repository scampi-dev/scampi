// SPDX-License-Identifier: GPL-3.0-only

package lsp

import (
	"context"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
)

// Data-driven LSP test runner. Fixtures live under
// `lsp/testdata/<request>/<name>.scampi` with a sibling
// `<name>.json` describing the expected response.
//
// Each .scampi file may contain a single ‸ marker showing where
// the cursor sits when the request fires. The runner strips the
// marker before opening the document and uses its position as the
// LSP cursor. Fixtures without a marker fall back to the `cursor`
// field in the JSON.
//
// Supported request kinds and their `expect` shapes:
//
//   completion → { "labels_include": [...], "labels_exclude": [...] }
//   hover      → { "contains": [...], "empty": bool }
//   definition → { "uri_suffix": "...", "line": N }
//
// The `_include`/`_exclude`/`contains` shapes deliberately use
// substring/subset matching, not exact equality, so fixtures don't
// have to enumerate every catalog item — they just pin the things
// that matter.

const cursorMarker = "$0"

type fixtureCase struct {
	Request string          `json:"request"`
	Cursor  *cursorPosition `json:"cursor,omitempty"`
	Expect  json.RawMessage `json:"expect"`
}

type cursorPosition struct {
	Line uint32 `json:"line"`
	Char uint32 `json:"char"`
}

type completionExpect struct {
	Labels        []string `json:"labels,omitempty"`
	LabelsInclude []string `json:"labels_include,omitempty"`
	LabelsExclude []string `json:"labels_exclude,omitempty"`
}

type hoverExpect struct {
	Contains    []string `json:"contains,omitempty"`
	NotContains []string `json:"not_contains,omitempty"`
	Empty       bool     `json:"empty,omitempty"`
}

type definitionExpect struct {
	URISuffix string `json:"uri_suffix,omitempty"`
	Line      uint32 `json:"line"`
}

type referencesExpect struct {
	Count        int  `json:"count"`
	CountAtLeast int  `json:"count_at_least,omitempty"`
	Empty        bool `json:"empty,omitempty"`
}

type renameExpect struct {
	FileCount int  `json:"file_count,omitempty"`
	EditCount int  `json:"edit_count"`
	Error     bool `json:"error,omitempty"`
}

type codeactionExpect struct {
	TitlesInclude []string              `json:"titles_include,omitempty"`
	TitlesExclude []string              `json:"titles_exclude,omitempty"`
	Count         int                   `json:"count,omitempty"`
	Empty         bool                  `json:"empty,omitempty"`
	Diagnostics   []syntheticDiagnostic `json:"diagnostics,omitempty"`
}

type syntheticDiagnostic struct {
	Code    string `json:"code,omitempty"`
	Message string `json:"message"`
	Line    uint32 `json:"line"`
}

func TestLSPFixtures(t *testing.T) {
	root := "testdata"
	if _, err := os.Stat(root); os.IsNotExist(err) {
		t.Skip("no LSP fixtures yet")
	}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(p, ".scampi") {
			return nil
		}
		jsonPath := strings.TrimSuffix(p, ".scampi") + ".json"
		if _, err := os.Stat(jsonPath); err != nil {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		name := strings.TrimSuffix(filepath.ToSlash(rel), ".scampi")
		t.Run(name, func(t *testing.T) {
			runFixture(t, p, jsonPath)
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func runFixture(t *testing.T, scampiPath, jsonPath string) {
	t.Helper()
	rawSrc, err := os.ReadFile(scampiPath)
	if err != nil {
		t.Fatal(err)
	}
	expBytes, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Fatal(err)
	}

	var spec fixtureCase
	if err := json.Unmarshal(expBytes, &spec); err != nil {
		t.Fatalf("bad expectation JSON: %v", err)
	}

	src := string(rawSrc)
	var line, char uint32
	idx := strings.Index(src, cursorMarker)
	cursorRequired := spec.Request != "codeaction"
	switch {
	case idx >= 0:
		line, char = lineColAtByteOffset(src, idx)
		src = src[:idx] + src[idx+len(cursorMarker):]
	case spec.Cursor != nil:
		line, char = spec.Cursor.Line, spec.Cursor.Char
	case cursorRequired:
		t.Fatal("fixture must contain a $0 marker or a cursor field in JSON")
	}

	s := testServer()
	// Use a real path so file URI conversion works on hover/goto-def.
	docURI := protocol.DocumentURI(uri.File(scampiPath))
	s.docs.Open(docURI, src, 1)

	switch spec.Request {
	case "completion":
		runCompletionFixture(t, s, docURI, line, char, spec.Expect)
	case "hover":
		runHoverFixture(t, s, docURI, line, char, spec.Expect)
	case "definition":
		runDefinitionFixture(t, s, docURI, line, char, spec.Expect)
	case "references":
		runReferencesFixture(t, s, docURI, line, char, spec.Expect)
	case "rename":
		runRenameFixture(t, s, docURI, line, char, spec.Expect)
	case "codeaction":
		runCodeActionFixture(t, s, docURI, src, spec.Expect)
	default:
		t.Fatalf("unknown request type: %q", spec.Request)
	}
}

func runCompletionFixture(
	t *testing.T,
	s *Server,
	docURI protocol.DocumentURI,
	line, char uint32,
	rawExpect json.RawMessage,
) {
	var exp completionExpect
	if err := json.Unmarshal(rawExpect, &exp); err != nil {
		t.Fatalf("bad completion expect JSON: %v", err)
	}
	result, err := s.Completion(context.Background(), &protocol.CompletionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
			Position:     protocol.Position{Line: line, Character: char},
		},
	})
	if err != nil {
		t.Fatalf("Completion: %v", err)
	}
	labelSet := make(map[string]bool)
	if result != nil {
		for _, item := range result.Items {
			labelSet[item.Label] = true
		}
	}
	// Exact match: labels must match the result set exactly.
	if len(exp.Labels) > 0 {
		want := make(map[string]bool, len(exp.Labels))
		for _, l := range exp.Labels {
			want[l] = true
		}
		for l := range labelSet {
			if !want[l] {
				t.Errorf("unexpected completion label %q", l)
			}
		}
		for l := range want {
			if !labelSet[l] {
				t.Errorf("missing completion label %q; got %v", l, sortedLabels(labelSet))
			}
		}
	}
	for _, w := range exp.LabelsInclude {
		if !labelSet[w] {
			t.Errorf("expected completion label %q in result; got %v", w, sortedLabels(labelSet))
		}
	}
	for _, banned := range exp.LabelsExclude {
		if labelSet[banned] {
			t.Errorf("completion label %q should NOT be present", banned)
		}
	}
}

func runHoverFixture(
	t *testing.T,
	s *Server,
	docURI protocol.DocumentURI,
	line, char uint32,
	rawExpect json.RawMessage,
) {
	var exp hoverExpect
	if err := json.Unmarshal(rawExpect, &exp); err != nil {
		t.Fatalf("bad hover expect JSON: %v", err)
	}
	result, err := s.Hover(context.Background(), &protocol.HoverParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
			Position:     protocol.Position{Line: line, Character: char},
		},
	})
	if err != nil {
		t.Fatalf("Hover: %v", err)
	}
	if exp.Empty {
		if result != nil && result.Contents.Value != "" {
			t.Errorf("expected empty hover, got %q", result.Contents.Value)
		}
		return
	}
	if result == nil {
		t.Fatalf("expected hover with %v, got nil", exp.Contains)
	}
	for _, want := range exp.Contains {
		if !strings.Contains(result.Contents.Value, want) {
			t.Errorf("expected hover to contain %q; got:\n%s", want, result.Contents.Value)
		}
	}
	for _, banned := range exp.NotContains {
		if strings.Contains(result.Contents.Value, banned) {
			t.Errorf("hover should NOT contain %q; got:\n%s", banned, result.Contents.Value)
		}
	}
}

func runDefinitionFixture(
	t *testing.T,
	s *Server,
	docURI protocol.DocumentURI,
	line, char uint32,
	rawExpect json.RawMessage,
) {
	var exp definitionExpect
	if err := json.Unmarshal(rawExpect, &exp); err != nil {
		t.Fatalf("bad definition expect JSON: %v", err)
	}
	result, err := s.Definition(context.Background(), &protocol.DefinitionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
			Position:     protocol.Position{Line: line, Character: char},
		},
	})
	if err != nil {
		t.Fatalf("Definition: %v", err)
	}
	if len(result) == 0 {
		t.Fatal("expected at least one definition location")
	}
	loc := result[0]
	if exp.URISuffix != "" && !strings.HasSuffix(string(loc.URI), exp.URISuffix) {
		t.Errorf("expected definition URI to end with %q; got %q", exp.URISuffix, loc.URI)
	}
	if loc.Range.Start.Line != exp.Line {
		t.Errorf("expected definition at line %d; got %d", exp.Line, loc.Range.Start.Line)
	}
}

func runReferencesFixture(
	t *testing.T,
	s *Server,
	docURI protocol.DocumentURI,
	line, char uint32,
	rawExpect json.RawMessage,
) {
	var exp referencesExpect
	if err := json.Unmarshal(rawExpect, &exp); err != nil {
		t.Fatalf("bad references expect JSON: %v", err)
	}
	result, err := s.References(context.Background(), &protocol.ReferenceParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
			Position:     protocol.Position{Line: line, Character: char},
		},
	})
	if err != nil {
		t.Fatalf("References: %v", err)
	}
	if exp.Empty {
		if len(result) != 0 {
			t.Errorf("expected no references, got %d", len(result))
		}
		return
	}
	if exp.CountAtLeast > 0 {
		if len(result) < exp.CountAtLeast {
			t.Errorf("expected at least %d references, got %d", exp.CountAtLeast, len(result))
		}
		return
	}
	if len(result) != exp.Count {
		t.Errorf("expected %d references, got %d", exp.Count, len(result))
	}
}

func runRenameFixture(
	t *testing.T,
	s *Server,
	docURI protocol.DocumentURI,
	line, char uint32,
	rawExpect json.RawMessage,
) {
	var exp renameExpect
	if err := json.Unmarshal(rawExpect, &exp); err != nil {
		t.Fatalf("bad rename expect JSON: %v", err)
	}
	result, err := s.Rename(context.Background(), &protocol.RenameParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
			Position:     protocol.Position{Line: line, Character: char},
		},
		NewName: "renamed",
	})
	if exp.Error {
		if err == nil {
			t.Fatal("expected rename error, got nil")
		}
		return
	}
	if err != nil {
		t.Fatalf("Rename: %v", err)
	}
	if result == nil {
		t.Fatal("expected workspace edit, got nil")
	}
	totalEdits := 0
	for _, edits := range result.Changes {
		totalEdits += len(edits)
	}
	if exp.EditCount > 0 && totalEdits != exp.EditCount {
		t.Errorf("expected %d edits, got %d", exp.EditCount, totalEdits)
	}
	if exp.FileCount > 0 && len(result.Changes) != exp.FileCount {
		t.Errorf("expected edits in %d files, got %d", exp.FileCount, len(result.Changes))
	}
}

func runCodeActionFixture(
	t *testing.T,
	s *Server,
	docURI protocol.DocumentURI,
	content string,
	rawExpect json.RawMessage,
) {
	var exp codeactionExpect
	if err := json.Unmarshal(rawExpect, &exp); err != nil {
		t.Fatalf("bad codeaction expect JSON: %v", err)
	}

	// Build diagnostics: use synthetic ones from the fixture if
	// provided, otherwise try evaluating the document.
	var diags []protocol.Diagnostic
	if len(exp.Diagnostics) > 0 {
		for _, sd := range exp.Diagnostics {
			diags = append(diags, protocol.Diagnostic{
				Range: protocol.Range{
					Start: protocol.Position{Line: sd.Line, Character: 0},
					End:   protocol.Position{Line: sd.Line, Character: 0},
				},
				Code:    sd.Code,
				Message: sd.Message,
				Source:  "scampls",
			})
		}
	} else {
		diags = s.evaluate(context.Background(), docURI, content)
	}

	result, err := s.CodeAction(context.Background(), &protocol.CodeActionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
		Range:        protocol.Range{},
		Context: protocol.CodeActionContext{
			Diagnostics: diags,
		},
	})
	if err != nil {
		t.Fatalf("CodeAction: %v", err)
	}

	if exp.Empty {
		if len(result) != 0 {
			t.Errorf("expected no code actions, got %d", len(result))
		}
		return
	}

	if exp.Count > 0 && len(result) != exp.Count {
		t.Errorf("expected %d code actions, got %d", exp.Count, len(result))
	}

	titles := make(map[string]bool)
	for _, a := range result {
		titles[a.Title] = true
	}
	for _, want := range exp.TitlesInclude {
		if !titles[want] {
			var got []string
			for _, a := range result {
				got = append(got, a.Title)
			}
			t.Errorf("expected code action %q, got %v", want, got)
		}
	}
	for _, exclude := range exp.TitlesExclude {
		if titles[exclude] {
			t.Errorf("unexpected code action %q", exclude)
		}
	}
}

// lineColAtByteOffset converts a byte offset in src to (line, char)
// where line and char are 0-based and char counts UTF-16 code units
// (LSP convention). For ASCII-only sources the count matches bytes.
// Multi-byte characters before the offset are counted as their
// UTF-16 width.
func lineColAtByteOffset(src string, offset int) (line, char uint32) {
	for i := 0; i < offset && i < len(src); {
		r := rune(src[i])
		size := 1
		if r >= 0x80 {
			r, size = decodeRune(src[i:])
		}
		if r == '\n' {
			line++
			char = 0
			i += size
			continue
		}
		// UTF-16 code-unit width: characters in the BMP take 1 unit;
		// characters above U+FFFF take 2.
		if r > 0xFFFF {
			char += 2
		} else {
			char++
		}
		i += size
	}
	return line, char
}

// decodeRune is a tiny stand-in for utf8.DecodeRuneInString that
// avoids importing unicode/utf8 just for this one call.
func decodeRune(s string) (rune, int) {
	if len(s) == 0 {
		return 0, 0
	}
	b := s[0]
	switch {
	case b < 0x80:
		return rune(b), 1
	case b < 0xC0:
		return 0xFFFD, 1
	case b < 0xE0:
		if len(s) < 2 {
			return 0xFFFD, 1
		}
		return rune(b&0x1F)<<6 | rune(s[1]&0x3F), 2
	case b < 0xF0:
		if len(s) < 3 {
			return 0xFFFD, 1
		}
		return rune(b&0x0F)<<12 | rune(s[1]&0x3F)<<6 | rune(s[2]&0x3F), 3
	default:
		if len(s) < 4 {
			return 0xFFFD, 1
		}
		return rune(b&0x07)<<18 | rune(s[1]&0x3F)<<12 | rune(s[2]&0x3F)<<6 | rune(s[3]&0x3F), 4
	}
}

func sortedLabels(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
