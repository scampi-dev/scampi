// SPDX-License-Identifier: GPL-3.0-only

package lsp

import (
	"context"
	"strings"

	"go.lsp.dev/protocol"

	"scampi.dev/scampi/errs"
)

func (s *Server) prepareRename(
	_ context.Context,
	params *protocol.PrepareRenameParams,
) (*protocol.Range, error) {
	doc, ok := s.docs.Get(params.TextDocument.URI)
	if !ok {
		return nil, nil
	}

	word := wordAtOffset(doc.Content, offsetFromPosition(
		doc.Content,
		params.Position.Line,
		params.Position.Character,
	))
	s.log.Printf("prepareRename: %s %q", params.TextDocument.URI, word)
	if word == "" {
		return nil, nil
	}

	// Reject rename on dotted/qualified names (stdlib symbols).
	if strings.Contains(word, ".") {
		return nil, nil
	}
	// Reject rename on keywords.
	for _, kw := range keywords {
		if word == kw.label {
			return nil, nil
		}
	}

	filePath := uriToPath(params.TextDocument.URI)
	f, _ := Parse(filePath, []byte(doc.Content))
	if f == nil {
		return nil, nil
	}

	// The word must resolve to a definition somewhere.
	data := []byte(doc.Content)
	if span := findDefinition(f, word); span != nil {
		r := tokenSpanToRange(data, *span)
		return &r, nil
	}

	// Check siblings for multi-file modules.
	modName := fileModuleName(f)
	if modName != "" && modName != "main" {
		if _, ok := s.findInSiblings(filePath, modName, word); ok {
			// Return the range of the word under cursor.
			offset := offsetFromPosition(doc.Content, params.Position.Line, params.Position.Character)
			start, end := wordBoundsAtOffset(doc.Content, offset)
			startLine, startChar := positionAtOffset(doc.Content, start)
			endLine, endChar := positionAtOffset(doc.Content, end)
			r := protocol.Range{
				Start: protocol.Position{Line: startLine, Character: startChar},
				End:   protocol.Position{Line: endLine, Character: endChar},
			}
			return &r, nil
		}
	}

	return nil, nil
}

func (s *Server) rename(
	_ context.Context,
	params *protocol.RenameParams,
) (*protocol.WorkspaceEdit, error) {
	doc, ok := s.docs.Get(params.TextDocument.URI)
	if !ok {
		return nil, nil
	}

	word := wordAtOffset(doc.Content, offsetFromPosition(
		doc.Content,
		params.Position.Line,
		params.Position.Character,
	))
	if word == "" || strings.Contains(word, ".") {
		return nil, nil
	}

	filePath := uriToPath(params.TextDocument.URI)
	f, _ := Parse(filePath, []byte(doc.Content))
	if f == nil {
		return nil, nil
	}

	s.log.Printf("rename: %s %q → %q", filePath, word, params.NewName)

	// Reject if the new name already exists as a definition in this file.
	if span := findDefinition(f, params.NewName); span != nil {
		// bare-error: LSP response error, not a diagnostic
		return nil, errs.Errorf("name %q already exists", params.NewName)
	}

	data := []byte(doc.Content)
	var locs []protocol.Location

	offset := offsetFromPosition(doc.Content, params.Position.Line, params.Position.Character)
	if isWithinFieldKeySpan(f, uint32(offset)) {
		// Field rename — only rename field keys and field declarations.
		locs = append(locs, findFieldKeyIdents(f, filePath, data, word)...)
	} else {
		// Type/variable rename.
		locs = append(locs, findIdents(f, filePath, data, word)...)
		modName := fileModuleName(f)
		if modName != "" && modName != "main" {
			locs = append(locs, s.refsInSiblings(filePath, modName, word)...)
		}
	}

	locs = dedup(locs, locationKey)

	if len(locs) == 0 {
		return nil, nil
	}

	// Group edits by URI.
	changes := map[protocol.DocumentURI][]protocol.TextEdit{}
	for _, loc := range locs {
		changes[loc.URI] = append(changes[loc.URI], protocol.TextEdit{
			Range:   loc.Range,
			NewText: params.NewName,
		})
	}

	return &protocol.WorkspaceEdit{Changes: changes}, nil
}

func wordBoundsAtOffset(content string, offset int) (start, end int) {
	start = offset
	for start > 0 && isIdentByte(content[start-1]) {
		start--
	}
	end = offset
	for end < len(content) && isIdentByte(content[end]) {
		end++
	}
	return start, end
}

func isIdentByte(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') || b == '_'
}

func positionAtOffset(content string, offset int) (line, char uint32) {
	for i := 0; i < offset && i < len(content); i++ {
		if content[i] == '\n' {
			line++
			char = 0
		} else {
			char++
		}
	}
	return line, char
}
