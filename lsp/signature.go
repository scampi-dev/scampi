// SPDX-License-Identifier: GPL-3.0-only

package lsp

import (
	"context"
	"fmt"
	"strings"

	"go.lsp.dev/protocol"
)

func (s *Server) SignatureHelp(
	_ context.Context,
	params *protocol.SignatureHelpParams,
) (*protocol.SignatureHelp, error) {
	doc, ok := s.docs.Get(params.TextDocument.URI)
	if !ok {
		return nil, nil
	}

	cur := AnalyzeCursor(doc.Content, params.Position.Line, params.Position.Character)
	s.log.Printf(
		"signatureHelp: line=%d col=%d inCall=%v func=%q",
		params.Position.Line,
		params.Position.Character,
		cur.InCall,
		cur.FuncName,
	)
	if !cur.InCall {
		return nil, nil
	}

	f, ok := s.lookupFunc(params.TextDocument.URI, cur.FuncName)
	if !ok {
		s.log.Printf("signatureHelp: unknown func %q", cur.FuncName)
		return nil, nil
	}

	sig := buildSignature(f)
	activeParam := uint32(cur.ActiveParam)
	if int(activeParam) >= len(f.Params) {
		activeParam = uint32(len(f.Params) - 1)
	}

	return &protocol.SignatureHelp{
		Signatures:      []protocol.SignatureInformation{sig},
		ActiveSignature: 0,
		ActiveParameter: activeParam,
	}, nil
}

func buildSignature(f FuncInfo) protocol.SignatureInformation {
	var paramLabels []string
	var paramInfos []protocol.ParameterInformation
	for _, p := range f.Params {
		label := p.Name
		if !p.Required {
			label += "?"
		}
		paramLabels = append(paramLabels, label)

		doc := p.Desc
		if p.Default != "" {
			doc += fmt.Sprintf(" (default: %s)", p.Default)
		}

		paramInfos = append(paramInfos, protocol.ParameterInformation{
			Label:         label,
			Documentation: doc,
		})
	}

	sigLabel := f.Name + "(" + strings.Join(paramLabels, ", ") + ")"

	return protocol.SignatureInformation{
		Label:         sigLabel,
		Documentation: f.Summary,
		Parameters:    paramInfos,
	}
}
