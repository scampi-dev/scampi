package test

import (
	"context"
	"errors"
	"testing"

	"godoit.dev/doit/diagnostic"
	"godoit.dev/doit/engine"
	"godoit.dev/doit/source"
	"godoit.dev/doit/spec"
)

func TestPlan_CapabilityMismatch(t *testing.T) {
	cfgStr := `
package test
import "godoit.dev/doit/builtin"
steps: [
    builtin.copy & {
        src:   "/a"
        dest:  "/b"
        perm:  "0644"
        owner: "user"
        group: "group"
    }
]
`
	src := source.NewMemSource()
	tgt := newMinimalTarget() // Only implements Filesystem, not Ownership

	src.Files["/config.cue"] = []byte(cfgStr)

	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := spec.NewSourceStore()

	ctx := context.Background()
	cfg, err := engine.LoadConfig(ctx, em, "/config.cue", store, src)
	if err != nil {
		t.Fatalf("engine.LoadConfig() must not return error, got %v", err)
	}

	cfg.Target = mockTargetInstance(tgt)

	e, err := engine.New(src, cfg, em)
	if err != nil {
		t.Fatalf("engine.New() must not return error, got %v", err)
	}

	err = e.Apply(ctx)

	var capErr engine.AbortError
	if !errors.As(err, &capErr) {
		t.Fatalf("expected AbortError, got %T: %v", err, err)
	}

	diagIDs := rec.collectDiagnosticIDs()
	if len(diagIDs) != 1 {
		t.Fatalf("expected exactly 1 planDiagnostic, got %d", len(diagIDs))
	}

	if diagIDs[0] != "engine.CapabilityMismatch" {
		t.Fatalf("expected exactly one engine.CapabilityMismatch diagnostic, got %q", diagIDs[0])
	}
}
