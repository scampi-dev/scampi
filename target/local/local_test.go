package local

import (
	"context"
	"runtime"
	"testing"

	"godoit.dev/doit/capability"
	"godoit.dev/doit/spec"
)

func TestCreate_DetectsPkgBackend(t *testing.T) {
	tgt, err := Local{}.Create(context.Background(), nil, spec.TargetInstance{})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !tgt.Capabilities().Has(capability.Pkg) {
		t.Fatalf("expected local target on %s/%s to provide Pkg capability, but it didn't", runtime.GOOS, runtime.GOARCH)
	}
}
