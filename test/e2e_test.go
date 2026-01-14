package test

import (
	"context"
	"testing"

	"godoit.dev/doit/diagnostic"
	"godoit.dev/doit/engine"
	"godoit.dev/doit/source"
	"godoit.dev/doit/spec"
	"godoit.dev/doit/target"
)

func TestCopyEndToEnd(t *testing.T) {
	cfg := `
package test

import "godoit.dev/doit/builtin"

units: [
	builtin.copy & {
		name:  "builtin.copy action"
		src:   "/src.txt"
		dest:  "/dest.txt"
		perm:  "0644"
		owner: "e2e_owner"
		group: "e2e_group"
	}
]
`

	src := source.NewMemSource()
	tgt := target.NewMemTarget()

	src.Files["/src.txt"] = []byte("hello")
	src.Files["/config.cue"] = []byte(cfg)

	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)

	if err := engine.ApplyWithEnv(context.Background(), em, "/config.cue", spec.NewSourceStore(), src, tgt); err != nil {
		t.Fatalf("expected successful call to engine.Apply, got err: %q\n%s", err, rec)
	}

	data := tgt.Files["/dest.txt"]
	if string(data) != "hello" {
		t.Fatalf("unexpected dest contents: %q", data)
	}
}
