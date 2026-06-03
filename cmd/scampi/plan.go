// SPDX-License-Identifier: GPL-3.0-only

package main

import (
	"fmt"
	"io"
	"strings"

	"scampi.dev/scampi/internal/engine"
)

func printPlan(w io.Writer, p *engine.Plan) {
	sections := []struct {
		label string
		refs  []engine.Ref
	}{
		{"create", p.Create},
		{"update", p.Update},
		{"adopt", p.Adopt},
		{"halt", p.Halt},
		{"destroy", p.Destroy},
	}
	wrote := false
	for _, s := range sections {
		if len(s.refs) == 0 {
			continue
		}
		wrote = true
		names := make([]string, len(s.refs))
		for i, r := range s.refs {
			names[i] = r.String()
		}
		_, _ = fmt.Fprintf(w, "%-9s %s\n", s.label+":", strings.Join(names, ", "))
	}
	if len(p.InSync) > 0 {
		_, _ = fmt.Fprintf(w, "%-9s %d resources\n", "in-sync:", len(p.InSync))
		wrote = true
	}
	if !wrote {
		_, _ = fmt.Fprintln(w, "plan: empty")
	}
}
