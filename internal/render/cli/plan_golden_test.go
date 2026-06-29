// SPDX-License-Identifier: GPL-3.0-only

package cli

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"scampi.dev/scampi/internal/diagnostic/result"
	"scampi.dev/scampi/internal/signal"
	"scampi.dev/scampi/internal/spec"
)

// op builds a planned op whose detail is the literal text (no placeholders, so
// no data needed - the renderer renders it verbatim).
func op(idx int, id, text string, deps ...int) result.PlannedOp {
	return result.PlannedOp{
		Index:     idx,
		DisplayID: id,
		DependsOn: deps,
		Template:  &spec.PlanTemplate{ID: id, Text: text},
	}
}

// planFixture is a deterministic plan exercising the layout: a parallel layer
// ([1] and [2] both depend on [0]), an op tree (ensure-mode under copy-file),
// varied label widths, and path-bearing op detail that must elide.
func planFixture() result.PlanDetail {
	return result.PlanDetail{
		DeployID:   "web",
		DeployDesc: "production",
		Steps: []result.PlannedStep{
			{
				Index: 0, Kind: "dir", Desc: "create web root",
				Ops: []result.PlannedOp{
					op(0, "step.dir", "ensure directory /var/www/site"),
				},
			},
			{
				Index: 1, Kind: "copy", Desc: "deploy index page", DependsOn: []int{0},
				Ops: []result.PlannedOp{
					op(1, "step.copy-file", "copy (inline) -> /var/www/site/index.html"),
					op(2, "step.ensure-mode", "ensure mode -rw-r--r-- on /var/www/site/index.html", 1),
				},
			},
			{
				Index: 2, Kind: "template", Desc: "render vhost config", DependsOn: []int{0},
				Ops: []result.PlannedOp{
					op(3, "step.render-template", "render (inline) -> /etc/nginx/sites/site.conf"),
				},
			},
			{
				Index: 3, Kind: "service", Desc: "restart web server", DependsOn: []int{1, 2},
				Ops: []result.PlannedOp{
					op(4, "step.service", "restart nginx"),
				},
			},
		},
	}
}

// TestPlanGolden locks the plan layout (eliding, desc-drop, alignment, the
// narrow warning) across widths and verbosities, for both glyph sets. Color is
// off so the goldens are plain and readable. Regenerate after intentional
// changes with: SCAMPI_UPDATE=1 just test all (or go test ./...).
func TestPlanGolden(t *testing.T) {
	fixture := planFixture()

	combos := []struct {
		name string
		v    signal.Verbosity
		w    int
	}{
		{"quiet.120", signal.Quiet, 120}, // full
		{"quiet.44", signal.Quiet, 44},   // descs drop
		{"v.120", signal.V, 120},         // full, ops no detail
		{"v.54", signal.V, 54},           // descs drop
		{"vv.120", signal.VV, 120},       // full detail
		{"vv.80", signal.VV, 80},         // detail elides
		{"vv.60", signal.VV, 60},         // detail elides harder
		{"vv.46", signal.VV, 46},         // descs drop
		{"vv.32", signal.VV, 32},         // floor: core + warn
	}
	sets := []struct {
		name   string
		glyphs glyphSet
	}{
		{"fancy", fancyGlyphs},
		{"ascii", asciiGlyphs},
	}

	for _, gs := range sets {
		t.Run(gs.name, func(t *testing.T) {
			var b strings.Builder
			for _, c := range combos {
				f := newFormatter(gs.glyphs, false, nil, nil) // color off -> plain
				pr := newPlanRenderer(gs.glyphs, c.w, c.v, f)
				b.WriteString("=== " + c.name + " (cols=" + strconv.Itoa(c.w) + ") ===\n")
				for _, e := range pr.renderPlan(fixture) {
					b.WriteString(e.line + "\n")
				}
			}
			compareGolden(t, filepath.Join("testdata", "plan_"+gs.name+".golden"), b.String())
		})
	}
}

func compareGolden(t *testing.T, path, got string) {
	t.Helper()
	if os.Getenv("SCAMPI_UPDATE") != "" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden (run SCAMPI_UPDATE=1 to create): %v", err)
	}
	if got != string(want) {
		t.Errorf("golden mismatch for %s; run SCAMPI_UPDATE=1 to update.\n--- got ---\n%s", path, got)
	}
}
