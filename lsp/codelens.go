// SPDX-License-Identifier: GPL-3.0-only

package lsp

import (
	"context"
	"fmt"

	"go.lsp.dev/protocol"
)

// CodeLens emits one "N references" lens above each declaration.
// Editors render these as actionable annotations; clicking surfaces
// the reference locations.
func (s *Server) CodeLens(_ context.Context, params *protocol.CodeLensParams) ([]protocol.CodeLens, error) {
	doc, ok := s.docs.Get(params.TextDocument.URI)
	if !ok {
		return nil, nil
	}

	filePath := uriToPath(params.TextDocument.URI)
	f, _ := Parse(filePath, []byte(doc.Content))
	if f == nil {
		return nil, nil
	}

	data := []byte(doc.Content)
	var lenses []protocol.CodeLens

	for _, d := range f.Decls {
		name, span := declNameAndSpan(d)
		if name == "" {
			continue
		}
		refs := findIdents(f, filePath, data, name)
		// Subtract 1 for the definition itself.
		count := max(len(refs)-1, 0)
		label := fmt.Sprintf("%d references", count)
		if count == 1 {
			label = "1 reference"
		}
		r := tokenSpanToRange(data, span)
		lenses = append(lenses, protocol.CodeLens{
			Range: r,
			Command: &protocol.Command{
				Title:   label,
				Command: "editor.action.showReferences",
				Arguments: []any{
					params.TextDocument.URI,
					r.Start,
					refs,
				},
			},
		})
	}

	s.log.Printf("codeLens: %s → %d lenses", filePath, len(lenses))
	return lenses, nil
}

// CodeLensResolve is a no-op — all lens data ships in the initial
// CodeLens response.
func (s *Server) CodeLensResolve(context.Context, *protocol.CodeLens) (*protocol.CodeLens, error) {
	return nil, nil
}

// CodeLensRefresh signals clients to re-fetch lenses. scampi doesn't
// push refreshes — returning nil is a no-op acknowledgement.
func (s *Server) CodeLensRefresh(context.Context) error { return nil }
