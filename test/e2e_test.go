package test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"godoit.dev/doit/diagnostic"
	"godoit.dev/doit/engine"
	"godoit.dev/doit/spec"
)

func TestCopyEndToEnd(t *testing.T) {
	tmp := t.TempDir()

	src := filepath.Join(tmp, "src.txt")
	dst := filepath.Join(tmp, "dst.txt")

	err := os.WriteFile(src, []byte("hello"), 0o644)
	if err != nil {
		panic(err)
	}

	cfg := fmt.Sprintf(`
package test

import "godoit.dev/doit/builtin"

units: [
	builtin.copy & {
		name:  "builtin.copy action"
		src:   %q
		dest:  %q
		perm:  "0644"
		owner: "pskry"
		group: "staff"
	}
]
`, src, dst)

	cfgPath := filepath.Join(tmp, "config.cue")
	err = os.WriteFile(cfgPath, []byte(cfg), 0o644)
	if err != nil {
		panic(err)
	}

	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)

	err = engine.Apply(context.Background(), em, cfgPath, spec.NewSourceStore())
	if err != nil {
		t.Fatalf("expected successful call to engine.Apply, got err: %q\n%s", err, rec)
	}

	data, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello" {
		t.Fatalf("unexpected dest contents: %q", data)
	}
}
