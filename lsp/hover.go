// SPDX-License-Identifier: GPL-3.0-only

package lsp

import (
	"context"
	"strings"

	"go.lsp.dev/protocol"
)

func (s *Server) Hover(
	_ context.Context,
	params *protocol.HoverParams,
) (*protocol.Hover, error) {
	doc, ok := s.docs.Get(params.TextDocument.URI)
	if !ok {
		return nil, nil
	}

	cur := AnalyzeCursor(doc.Content, params.Position.Line, params.Position.Character)
	s.log.Printf(
		"hover: line=%d col=%d word=%q inCall=%v func=%q",
		params.Position.Line,
		params.Position.Character,
		cur.WordUnderCursor,
		cur.InCall,
		cur.FuncName,
	)

	// Known function name always wins — handles nested calls like
	// deploy(steps=[copy(...)]) where copy is inside deploy's parens.
	if md := s.hoverFunc(params.TextDocument.URI, cur.WordUnderCursor); md != "" {
		s.log.Printf("hover: returning func doc (%d bytes), kind=%q\n---\n%s\n---", len(md), protocol.Markdown, md)
		return &protocol.Hover{
			Contents: protocol.MarkupContent{
				Kind:  protocol.Markdown,
				Value: md,
			},
		}, nil
	}

	// Kwarg name inside a call?
	if cur.InCall {
		if md := s.hoverKwarg(params.TextDocument.URI, cur); md != "" {
			s.log.Printf("hover: returning kwarg doc (%d bytes), kind=%q", len(md), protocol.Markdown)
			return &protocol.Hover{
				Contents: protocol.MarkupContent{
					Kind:  protocol.Markdown,
					Value: md,
				},
			}, nil
		}
	}

	return nil, nil
}

func (s *Server) hoverFunc(docURI protocol.DocumentURI, word string) string {
	f, ok := s.lookupFunc(docURI, word)
	if !ok {
		return ""
	}
	return formatFuncDoc(f)
}

func (s *Server) hoverKwarg(docURI protocol.DocumentURI, cur CursorContext) string {
	f, ok := s.lookupFunc(docURI, cur.FuncName)
	if !ok {
		return ""
	}

	// Check if the word under cursor matches a param name.
	for _, p := range f.Params {
		if p.Name == cur.WordUnderCursor {
			return formatParamDoc(cur.FuncName, p)
		}
	}
	return ""
}

func formatFuncDoc(f FuncInfo) string {
	var b strings.Builder

	// Signature in a fenced code block, like gopls.
	b.WriteString("```python\n" + formatSignature(f) + "\n```\n\n")

	if f.Summary != "" {
		b.WriteString("---\n\n" + f.Summary + "\n")
	}

	if len(f.Params) == 0 {
		return b.String()
	}

	b.WriteString("\n---\n\n")
	for _, p := range f.Params {
		line := "`" + p.Name + "` *" + p.Type + "*"
		if p.Required {
			line += " **required**"
		}
		line += "\n" + p.Desc
		if p.Default != "" {
			line += " (default: " + p.Default + ")"
		}
		if len(p.Examples) > 0 {
			line += " (e.g. " + strings.Join(p.Examples, ", ") + ")"
		}
		b.WriteString(line + "\n\n")
	}

	return b.String()
}

func formatSignature(f FuncInfo) string {
	var params []string
	for _, p := range f.Params {
		s := p.Name
		if !p.Required {
			s += "?"
		}
		params = append(params, s)
	}
	return f.Name + "(" + strings.Join(params, ", ") + ")"
}

func formatParamDoc(funcName string, p ParamInfo) string {
	var b strings.Builder

	// Signature block.
	req := "optional"
	if p.Required {
		req = "required"
	}
	b.WriteString("```python\n")
	b.WriteString(funcName + "(" + p.Name + ": " + p.Type + ")  # " + req + "\n")
	b.WriteString("```\n\n---\n\n")

	// Description.
	b.WriteString(p.Desc + "\n")

	// Metadata.
	if p.Default != "" || len(p.Examples) > 0 {
		b.WriteString("\n---\n\n")
		if p.Default != "" {
			b.WriteString("**Default:** " + p.Default + "\n\n")
		}
		if len(p.Examples) > 0 {
			b.WriteString("**Examples:** `" + strings.Join(p.Examples, "`, `") + "`\n")
		}
	}

	return b.String()
}
