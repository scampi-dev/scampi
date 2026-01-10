package template

import (
	"strings"
	"text/template"
)

func Render(name, tmpl string, data any) (string, bool) {
	// TODO: parsing way too late
	t, err := template.
		New(name).
		Funcs(template.FuncMap{
			"join": join,
		}).
		Parse(tmpl)
	if err != nil {
		// FIXME: panic
		panic(err)
	}

	b := strings.Builder{}
	// FIXME: at this point we MUST be able to trust that the template renders
	if err := t.Execute(&b, data); err != nil {
		// FIXME: panic
		panic(err)
	}

	res := b.String()
	return res, strings.TrimSpace(res) != ""
}

// Template funcs
// ===============================================

func join(sep string, s []string) string {
	return strings.Join(s, sep)
}
