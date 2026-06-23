// SPDX-License-Identifier: GPL-3.0-only

package integration

import (
	"errors"
	"reflect"
	"sort"
	"strings"
	"testing"

	"scampi.dev/scampi/internal/diagnostic"
	"scampi.dev/scampi/internal/diagnostic/event"
	"scampi.dev/scampi/internal/engine"
	"scampi.dev/scampi/internal/render/template"
	"scampi.dev/scampi/internal/spec"
	"scampi.dev/scampi/test/harness"
)

func TestPlan_CyclicDependencies(t *testing.T) {
	mkFakeOp := func(name string) *harness.FakeOp {
		return &harness.FakeOp{
			Name:    name,
			CheckFn: harness.PanicCheckFn("Check must not be called for cycle detection"),
			ExecFn:  harness.PanicExecFn("Execute must not be called for cycle detection"),
		}
	}

	tests := []struct {
		name      string
		build     func() spec.Plan
		wantPaths [][]string
	}{
		{
			name: "two node cycle",
			build: func() spec.Plan {
				a := mkFakeOp("A")
				b := mkFakeOp("B")

				a.Deps = []spec.Op{b}
				b.Deps = []spec.Op{a}

				return spec.Plan{
					Deploy: spec.Deploy{
						ID:   "fakeUnit",
						Desc: "fakeUnit description",
						Actions: []spec.Action{
							harness.MkAction(a, b),
						},
					},
				}
			},
			wantPaths: [][]string{
				{"A", "B", "A"},
			},
		},
		{
			name: "three node cycle",
			build: func() spec.Plan {
				a := mkFakeOp("A")
				b := mkFakeOp("B")
				c := mkFakeOp("C")

				a.Deps = []spec.Op{b}
				b.Deps = []spec.Op{c}
				c.Deps = []spec.Op{a}

				return spec.Plan{
					Deploy: spec.Deploy{
						ID:   "fakeUnit",
						Desc: "fakeUnit description",
						Actions: []spec.Action{
							harness.MkAction(a, b, c),
						},
					},
				}
			},
			wantPaths: [][]string{
				{"A", "B", "C", "A"},
			},
		},
		{
			name: "two independent cycles",
			build: func() spec.Plan {
				a := mkFakeOp("A")
				b := mkFakeOp("B")
				c := mkFakeOp("C")
				d := mkFakeOp("D")

				a.Deps = []spec.Op{b}
				b.Deps = []spec.Op{a}

				c.Deps = []spec.Op{d}
				d.Deps = []spec.Op{c}

				return spec.Plan{
					Deploy: spec.Deploy{
						ID:   "fakeUnit",
						Desc: "fakeUnit description",
						Actions: []spec.Action{
							harness.MkAction(a, b, c, d),
						},
					},
				}
			},
			wantPaths: [][]string{
				{"A", "B", "A"},
				{"C", "D", "C"},
			},
		},
		{
			name: "self cycle",
			build: func() spec.Plan {
				a := mkFakeOp("A")
				a.Deps = []spec.Op{a}

				return spec.Plan{
					Deploy: spec.Deploy{
						ID:   "fakeUnit",
						Desc: "fakeUnit description",
						Actions: []spec.Action{
							harness.MkAction(a),
						},
					},
				}
			},
			wantPaths: [][]string{
				{"A", "A"},
			},
		},
		{
			name: "overlapping cycles sharing nodes",
			build: func() spec.Plan {
				a := mkFakeOp("A")
				b := mkFakeOp("B")
				c := mkFakeOp("C")
				d := mkFakeOp("D")

				a.Deps = []spec.Op{b}
				b.Deps = []spec.Op{c}
				c.Deps = []spec.Op{a, d}
				d.Deps = []spec.Op{c}

				return spec.Plan{
					Deploy: spec.Deploy{
						ID:   "fakeUnit",
						Desc: "fakeUnit description",
						Actions: []spec.Action{
							harness.MkAction(a, b, c, d),
						},
					},
				}
			},
			wantPaths: [][]string{
				{"A", "B", "C", "A"},
				{"C", "D", "C"},
			},
		},
		{
			name: "cycle plus acyclic tail",
			build: func() spec.Plan {
				a := mkFakeOp("A")
				b := mkFakeOp("B")
				c := mkFakeOp("C")
				e := mkFakeOp("E")
				f := mkFakeOp("F")

				a.Deps = []spec.Op{b}
				b.Deps = []spec.Op{c}
				c.Deps = []spec.Op{a}

				e.Deps = []spec.Op{f}
				f.Deps = nil

				return spec.Plan{
					Deploy: spec.Deploy{
						ID:   "fakeUnit",
						Desc: "fakeUnit description",
						Actions: []spec.Action{
							harness.MkAction(a, b, c, e, f),
						},
					},
				}
			},
			wantPaths: [][]string{
				{"A", "B", "C", "A"},
			},
		},
		{
			name: "diamond dependency with back edge",
			build: func() spec.Plan {
				a := mkFakeOp("A")
				b := mkFakeOp("B")
				c := mkFakeOp("C")
				d := mkFakeOp("D")

				a.Deps = []spec.Op{b, c}
				b.Deps = []spec.Op{d}
				c.Deps = []spec.Op{d}
				d.Deps = []spec.Op{a}

				return spec.Plan{
					Deploy: spec.Deploy{
						ID:   "fakeUnit",
						Desc: "fakeUnit description",
						Actions: []spec.Action{
							harness.MkAction(a, b, c, d),
						},
					},
				}
			},
			wantPaths: [][]string{
				{"A", "B", "D", "A"},
			},
		},
		{
			name: "cycle across actions",
			build: func() spec.Plan {
				a := mkFakeOp("A")
				b := mkFakeOp("B")
				c := mkFakeOp("C")

				a.Deps = []spec.Op{b}
				b.Deps = []spec.Op{c}
				c.Deps = []spec.Op{a}

				return spec.Plan{
					Deploy: spec.Deploy{
						ID:   "fakeUnit",
						Desc: "fakeUnit description",
						Actions: []spec.Action{
							harness.MkAction(a),
							harness.MkAction(b),
							harness.MkAction(c),
						},
					},
				}
			},
			wantPaths: [][]string{
				{"A", "B", "C", "A"},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := &harness.RecordingDisplayer{}
			em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)

			err := engine.DetectPlanCycles(em, tc.build())

			// ---- assert abort ----
			var abort engine.AbortError
			if !errors.As(err, &abort) {
				t.Fatalf("expected AbortError, got %T: %v", err, err)
			}

			// ---- collect diagnostics ----
			for i, ev := range rec.Diagnostics {
				tmpl := event.TemplateOf(ev)
				if tmpl.ID != "engine.CyclicDependency" {
					t.Fatalf(
						"[%d] expected template ID %q, got %q",
						i,
						"engine.CyclicDependency",
						tmpl.ID,
					)
				}
			}

			got := extractCyclePaths(rec.Diagnostics)

			if len(got) != len(tc.wantPaths) {
				t.Fatalf("expected %d cycle paths, got %d", len(tc.wantPaths), len(got))
			}

			normalizeAll := func(paths [][]string) []string {
				var out []string
				for _, p := range paths {
					n := normalizeCycle(p)
					out = append(out, strings.Join(n, "->"))
				}
				sort.Strings(out)
				return out
			}

			want := normalizeAll(tc.wantPaths)
			have := normalizeAll(got)

			if !reflect.DeepEqual(want, have) {
				t.Fatalf("cycles mismatch\nwant: %v\ngot:  %v", want, have)
			}
		})
	}
}

func extractCyclePaths(diags []event.Event) [][]string {
	var paths [][]string

	for _, ev := range diags {
		tmpl := event.TemplateOf(ev)
		if tmpl.ID != "engine.CyclicDependency" {
			continue
		}

		hint, _ := template.Render(tmpl.HintField())
		paths = append(paths, strings.Split(hint, " -> "))
	}

	return paths
}

func normalizeCycle(path []string) []string {
	// drop the repeated last node for normalization
	n := len(path) - 1

	minIdx := 0
	for i := 1; i < n; i++ {
		if path[i] < path[minIdx] {
			minIdx = i
		}
	}

	out := make([]string, 0, n+1)

	for i := range n {
		out = append(out, path[(minIdx+i)%n])
	}
	out = append(out, out[0]) // close the cycle again

	return out
}
