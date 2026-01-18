package test

import (
	"errors"
	"reflect"
	"sort"
	"strings"
	"testing"

	"godoit.dev/doit/diagnostic"
	"godoit.dev/doit/diagnostic/event"
	"godoit.dev/doit/engine"
	"godoit.dev/doit/spec"
)

func TestPlan_CyclicDependencies(t *testing.T) {
	mkFakeOp := func(name string) *fakeOp {
		return &fakeOp{
			name:    name,
			checkFn: panicCheckFn("Check must not be called for cycle detection"),
			execFn:  panicExecFn("Execute must not be called for cycle detection"),
		}
	}

	mkAction := func(ops ...*fakeOp) spec.Action {
		act := &fakeAction{}

		for _, op := range ops {
			act.ops = append(act.ops, op)
			op.action = act
		}

		return act
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

				a.deps = []spec.Op{b}
				b.deps = []spec.Op{a}

				return spec.Plan{
					Actions: []spec.Action{
						mkAction(a, b),
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

				a.deps = []spec.Op{b}
				b.deps = []spec.Op{c}
				c.deps = []spec.Op{a}

				return spec.Plan{
					Actions: []spec.Action{
						mkAction(a, b, c),
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

				a.deps = []spec.Op{b}
				b.deps = []spec.Op{a}

				c.deps = []spec.Op{d}
				d.deps = []spec.Op{c}

				return spec.Plan{
					Actions: []spec.Action{
						mkAction(a, b, c, d),
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
				a.deps = []spec.Op{a}

				return spec.Plan{
					Actions: []spec.Action{
						mkAction(a),
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

				a.deps = []spec.Op{b}
				b.deps = []spec.Op{c}
				c.deps = []spec.Op{a, d}
				d.deps = []spec.Op{c}

				return spec.Plan{
					Actions: []spec.Action{
						mkAction(a, b, c, d),
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

				a.deps = []spec.Op{b}
				b.deps = []spec.Op{c}
				c.deps = []spec.Op{a}

				e.deps = []spec.Op{f}
				f.deps = nil

				return spec.Plan{
					Actions: []spec.Action{
						mkAction(a, b, c, e, f),
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

				a.deps = []spec.Op{b, c}
				b.deps = []spec.Op{d}
				c.deps = []spec.Op{d}
				d.deps = []spec.Op{a}

				return spec.Plan{
					Actions: []spec.Action{
						mkAction(a, b, c, d),
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

				a.deps = []spec.Op{b}
				b.deps = []spec.Op{c}
				c.deps = []spec.Op{a}

				return spec.Plan{
					Actions: []spec.Action{
						mkAction(a),
						mkAction(b),
						mkAction(c),
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
			rec := &recordingDisplayer{}
			em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)

			err := engine.DetectPlanCycles(em, tc.build())

			// ---- assert abort ----
			var abort engine.AbortError
			if !errors.As(err, &abort) {
				t.Fatalf("expected AbortError, got %T: %v", err, err)
			}

			// ---- collect diagnostics ----
			var diags []event.Event
			for _, ev := range rec.events {
				if ev.Kind == event.DiagnosticRaised {
					diags = append(diags, ev)
				}
			}

			for i, ev := range diags {
				detail, ok := ev.Detail.(event.DiagnosticDetail)
				if !ok {
					t.Fatalf("[%d] expected DiagnosticDetail, got %T", i, ev.Detail)
				}

				if detail.Template.ID != "engine.CyclicDependency" {
					t.Fatalf(
						"[%d] expected template ID %q, got %q",
						i,
						"engine.CyclicDependency",
						detail.Template.ID,
					)
				}
			}

			got := extractCyclePaths(rec.events)

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

func extractCyclePaths(events []event.Event) [][]string {
	var paths [][]string

	for _, ev := range events {
		if ev.Kind != event.DiagnosticRaised {
			continue
		}

		d, ok := ev.Detail.(event.DiagnosticDetail)
		if !ok {
			continue
		}

		if d.Template.ID != "engine.CyclicDependency" {
			continue
		}

		hint := d.Template.Hint
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
