// SPDX-License-Identifier: GPL-3.0-only

package lsp

import (
	"context"
	"strings"

	"go.lsp.dev/protocol"

	"scampi.dev/scampi/lang/check"
)

func (s *Server) codeAction(
	_ context.Context,
	params *protocol.CodeActionParams,
) ([]protocol.CodeAction, error) {
	doc, ok := s.docs.Get(params.TextDocument.URI)
	if !ok {
		return nil, nil
	}

	var actions []protocol.CodeAction
	for _, diag := range params.Context.Diagnostics {
		actions = append(actions, s.actionsForDiagnostic(params.TextDocument.URI, doc.Content, diag)...)
	}
	return actions, nil
}

func (s *Server) actionsForDiagnostic(
	docURI protocol.DocumentURI,
	content string,
	diag protocol.Diagnostic,
) []protocol.CodeAction {
	code, _ := diag.Code.(string)
	msg := diag.Message

	switch code {
	case check.CodeUnknownModule:
		return s.fixUnknownModule(docURI, content, diag)
	case check.CodeUndefined:
		name := strings.TrimPrefix(msg, "undefined: ")
		return s.fixUndefinedAsImport(docURI, content, diag, name)
	case check.CodeUnknownType:
		name := strings.TrimPrefix(msg, "unknown type: ")
		return s.fixUndefinedAsImport(docURI, content, diag, name)
	case check.CodeDuplicateImport:
		return fixRemoveLine(docURI, content, diag, "Remove duplicate import")
	case check.CodeDuplicateField:
		return fixRemoveLine(docURI, content, diag, "Remove duplicate field")
	}

	return nil
}

// fixUnknownModule offers to add an import for a known module.
func (s *Server) fixUnknownModule(
	docURI protocol.DocumentURI,
	content string,
	diag protocol.Diagnostic,
) []protocol.CodeAction {
	modPath := strings.TrimPrefix(diag.Message, "unknown module: ")

	// Only offer if it's a known std module.
	known := []string{"std", "std/posix", "std/local", "std/ssh", "std/container"}
	found := false
	for _, k := range known {
		if k == modPath {
			found = true
			break
		}
	}
	if !found {
		return nil
	}

	// Find insertion point: after the last import, or after the module declaration.
	insertLine := findImportInsertLine(content)
	importText := "import \"" + modPath + "\"\n"

	return []protocol.CodeAction{{
		Title:       "Add import \"" + modPath + "\"",
		Kind:        protocol.QuickFix,
		Diagnostics: []protocol.Diagnostic{diag},
		Edit: &protocol.WorkspaceEdit{
			Changes: map[protocol.DocumentURI][]protocol.TextEdit{
				docURI: {{
					Range: protocol.Range{
						Start: protocol.Position{Line: uint32(insertLine), Character: 0},
						End:   protocol.Position{Line: uint32(insertLine), Character: 0},
					},
					NewText: importText,
				}},
			},
		},
		IsPreferred: true,
	}}
}

// moduleForName maps leaf identifiers to their std import paths.
var moduleForName = map[string]string{
	"posix":     "std/posix",
	"local":     "std/local",
	"ssh":       "std/ssh",
	"container": "std/container",
}

// fixUndefinedAsImport offers to add an import when an undefined name
// matches a known std module leaf.
func (s *Server) fixUndefinedAsImport(
	docURI protocol.DocumentURI,
	content string,
	diag protocol.Diagnostic,
	name string,
) []protocol.CodeAction {
	modPath, ok := moduleForName[name]
	if !ok {
		return nil
	}
	// Don't offer if already imported.
	if strings.Contains(content, "\""+modPath+"\"") {
		return nil
	}

	insertLine := findImportInsertLine(content)
	importText := "import \"" + modPath + "\"\n"

	return []protocol.CodeAction{{
		Title:       "Add import \"" + modPath + "\"",
		Kind:        protocol.QuickFix,
		Diagnostics: []protocol.Diagnostic{diag},
		Edit: &protocol.WorkspaceEdit{
			Changes: map[protocol.DocumentURI][]protocol.TextEdit{
				docURI: {{
					Range: protocol.Range{
						Start: protocol.Position{Line: uint32(insertLine), Character: 0},
						End:   protocol.Position{Line: uint32(insertLine), Character: 0},
					},
					NewText: importText,
				}},
			},
		},
		IsPreferred: true,
	}}
}

// fixRemoveLine offers to delete the line containing the diagnostic.
func fixRemoveLine(
	docURI protocol.DocumentURI,
	content string,
	diag protocol.Diagnostic,
	title string,
) []protocol.CodeAction {
	line := diag.Range.Start.Line
	lines := strings.Split(content, "\n")
	if int(line) >= len(lines) {
		return nil
	}

	return []protocol.CodeAction{{
		Title:       title,
		Kind:        protocol.QuickFix,
		Diagnostics: []protocol.Diagnostic{diag},
		Edit: &protocol.WorkspaceEdit{
			Changes: map[protocol.DocumentURI][]protocol.TextEdit{
				docURI: {{
					Range: protocol.Range{
						Start: protocol.Position{Line: line, Character: 0},
						End:   protocol.Position{Line: line + 1, Character: 0},
					},
					NewText: "",
				}},
			},
		},
	}}
}

// findImportInsertLine returns the 0-based line number where a new
// import should be inserted — after the last existing import, or
// after the module declaration if there are no imports.
func findImportInsertLine(content string) int {
	lines := strings.Split(content, "\n")
	lastImport := -1
	moduleLine := -1
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "import ") {
			lastImport = i
		}
		if strings.HasPrefix(trimmed, "module ") {
			moduleLine = i
		}
	}
	if lastImport >= 0 {
		return lastImport + 1
	}
	if moduleLine >= 0 {
		return moduleLine + 2 // blank line after module decl
	}
	return 0
}
