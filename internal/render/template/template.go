// SPDX-License-Identifier: GPL-3.0-only

package template

import (
	"strings"
	"text/template"

	"scampi.dev/scampi/internal/errs"
)

// Funcs is the shared FuncMap available in all scampi templates —
// both diagnostic render templates and user-facing posix.template.
var Funcs = template.FuncMap{
	"join":       join,
	"upper":      strings.ToUpper,
	"lower":      strings.ToLower,
	"trimSpace":  strings.TrimSpace,
	"trimPrefix": strings.TrimPrefix,
	"trimSuffix": strings.TrimSuffix,
	"contains":   strings.Contains,
	"hasPrefix":  strings.HasPrefix,
	"hasSuffix":  strings.HasSuffix,
	"replace":    strings.ReplaceAll,
}

// New creates a template with the shared FuncMap and missingkey=error.
func New(name string) *template.Template {
	return template.New(name).Option("missingkey=error").Funcs(Funcs)
}

// Renderable is the contract for anything the template renderer can render.
// Every caller supplies its own concrete type; the renderer never accepts
// bare strings.  Contract tested in test/template_render_test.go.
type Renderable interface {
	TemplateID() string
	TemplateText() string
	TemplateData() any
}

func Render(r Renderable) (string, bool) {
	t, err := New(r.TemplateID()).Parse(r.TemplateText())
	if err != nil {
		panic(errs.BUG("template '%s' failed parsing: %w", r.TemplateText(), err))
	}

	b := strings.Builder{}
	if err := t.Execute(&b, r.TemplateData()); err != nil {
		panic(errs.BUG("template '%s' failed to render: %w", r.TemplateText(), err))
	}

	res := b.String()
	return res, strings.TrimSpace(res) != ""
}

// Template funcs
// -----------------------------------------------------------------------------

func join(sep string, s []string) string {
	return strings.Join(s, sep)
}
