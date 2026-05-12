// SPDX-License-Identifier: GPL-3.0-only

package lsp

import (
	"context"
	"strings"

	"go.lsp.dev/protocol"
)

// DocumentHighlight returns all occurrences of the word under the
// cursor in the current document. The definition (if found) is
// marked as Write; every other occurrence as Read. Editors typically
// render Write occurrences in a different colour.
func (s *Server) DocumentHighlight(
	_ context.Context,
	params *protocol.DocumentHighlightParams,
) ([]protocol.DocumentHighlight, error) {
	doc, ok := s.docs.Get(params.TextDocument.URI)
	if !ok {
		return nil, nil
	}

	word := wordAtOffset(doc.Content, offsetFromPosition(
		doc.Content,
		params.Position.Line,
		params.Position.Character,
	))
	if word == "" {
		return nil, nil
	}

	filePath := uriToPath(params.TextDocument.URI)
	f, _ := Parse(filePath, []byte(doc.Content))
	if f == nil {
		return nil, nil
	}

	data := []byte(doc.Content)
	var locs []protocol.Location
	if strings.Contains(word, ".") {
		locs = findDottedRefs(f, filePath, data, word)
	} else {
		locs = findIdents(f, filePath, data, word)
	}

	// Find definition to mark it as Write, all others as Read.
	defSpan := findDefinition(f, word)

	var highlights []protocol.DocumentHighlight
	for _, loc := range locs {
		kind := protocol.DocumentHighlightKindRead
		if defSpan != nil {
			r := tokenSpanToRange(data, *defSpan)
			if r.Start == loc.Range.Start {
				kind = protocol.DocumentHighlightKindWrite
			}
		}
		highlights = append(highlights, protocol.DocumentHighlight{
			Range: loc.Range,
			Kind:  kind,
		})
	}
	s.log.Printf("documentHighlight: %s %q → %d highlights", filePath, word, len(highlights))
	return highlights, nil
}
