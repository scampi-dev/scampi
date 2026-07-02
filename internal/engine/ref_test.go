// SPDX-License-Identifier: GPL-3.0-only

package engine

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"scampi.dev/scampi/internal/spec"
)

func Test_RefResolver_ResolvesAndNormalizes(t *testing.T) {
	outputs := newStepOutputs()
	outputs.Store(1, map[string]any{
		"host": "10.0.0.5",
		"port": json.Number("5432"),
	})

	resolve := buildRefResolver(outputs, false)

	got, err := resolve(spec.Ref{TargetID: 1, Expr: ".host"})
	if err != nil {
		t.Fatalf("resolve .host: %v", err)
	}
	if got != "10.0.0.5" {
		t.Errorf(".host = %v, want 10.0.0.5", got)
	}

	// json.Number normalizes to float64 for the value pipeline.
	got, err = resolve(spec.Ref{TargetID: 1, Expr: ".port"})
	if err != nil {
		t.Fatalf("resolve .port: %v", err)
	}
	if got != 5432.0 {
		t.Errorf(".port = %v (%T), want float64 5432", got, got)
	}
}

func Test_RefResolver_RejectsInvalidRefs(t *testing.T) {
	outputs := newStepOutputs()
	outputs.Store(1, map[string]any{"host": "10.0.0.5"})
	resolve := buildRefResolver(outputs, false)

	cases := []struct {
		name       string
		ref        spec.Ref
		wantDetail string
	}{
		{
			name:       "missing output in apply mode",
			ref:        spec.Ref{TargetID: 99, Expr: "."},
			wantDetail: "no output",
		},
		{
			name:       "invalid jq",
			ref:        spec.Ref{TargetID: 1, Expr: "((("},
			wantDetail: "invalid jq",
		},
		{
			name:       "no result",
			ref:        spec.Ref{TargetID: 1, Expr: ".nope"},
			wantDetail: "no result",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := resolve(c.ref)
			var re RefError
			if !errors.As(err, &re) {
				t.Fatalf("expected RefError, got %T: %v", err, err)
			}
			if !strings.Contains(re.Detail, c.wantDetail) {
				t.Errorf("detail = %q, want it to mention %q", re.Detail, c.wantDetail)
			}
		})
	}
}

// In check mode a missing output is not an error: the producing step simply
// has not run yet ("would change"), so the resolver hands back the pending
// sentinel and drift detection reports would-change instead of aborting.
func Test_RefResolver_ReturnsPendingInCheckMode(t *testing.T) {
	resolve := buildRefResolver(newStepOutputs(), true)

	got, err := resolve(spec.Ref{TargetID: 99, Expr: "."})
	if err != nil {
		t.Fatalf("check-mode missing output must not error, got %v", err)
	}
	if _, ok := got.(refPending); !ok {
		t.Errorf("got %T, want refPending sentinel", got)
	}
}
