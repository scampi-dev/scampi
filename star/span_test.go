// SPDX-License-Identifier: GPL-3.0-only

package star_test

import (
	"context"
	"testing"

	"scampi.dev/scampi/diagnostic"
	"scampi.dev/scampi/source"
	"scampi.dev/scampi/star"
)

func TestKwargFieldSpans(t *testing.T) {
	src := source.NewMemSource()
	src.Files["/config.scampi"] = []byte(`target.local(name="host")
deploy(
    name="main",
    targets=["host"],
    steps=[
        copy(
            src=local("/tmp/a.txt"),
            dest="/tmp/b.txt",
            perm="0644",
            owner="root",
            group="root",
        ),
    ],
)
`)

	store := diagnostic.NewSourceStore()
	cfg, err := star.Eval(context.Background(), "/config.scampi", store, src)
	if err != nil {
		t.Fatalf("Eval failed: %v", err)
	}

	step := cfg.Deploy["main"].Steps[0]

	tests := []struct {
		field        string
		wantLine     int
		wantStartCol int
	}{
		{"src", 7, 17},
		{"dest", 8, 18},
		{"perm", 9, 18},
		{"owner", 10, 19},
		{"group", 11, 19},
	}

	for _, tt := range tests {
		t.Run(tt.field, func(t *testing.T) {
			fs, ok := step.Fields[tt.field]
			if !ok {
				t.Fatalf("field %q not found in Fields map", tt.field)
			}
			if fs.Value.Filename != "/config.scampi" {
				t.Errorf("Filename = %q, want /config.scampi", fs.Value.Filename)
			}
			if fs.Value.StartLine != tt.wantLine {
				t.Errorf("StartLine = %d, want %d", fs.Value.StartLine, tt.wantLine)
			}
			if fs.Value.StartCol != tt.wantStartCol {
				t.Errorf("StartCol = %d, want %d", fs.Value.StartCol, tt.wantStartCol)
			}
		})
	}
}

func TestKwargFieldSpans_SingleLine(t *testing.T) {
	src := source.NewMemSource()
	src.Files["/config.scampi"] = []byte(`target.local(name="host")
deploy(name="main", targets=["host"], steps=[
    pkg(packages=["vim", "curl"], source=system()),
])
`)

	store := diagnostic.NewSourceStore()
	cfg, err := star.Eval(context.Background(), "/config.scampi", store, src)
	if err != nil {
		t.Fatalf("Eval failed: %v", err)
	}

	step := cfg.Deploy["main"].Steps[0]

	// packages=["vim", "curl"] on line 3 — value starts at [
	fs := step.Fields["packages"]
	if fs.Value.StartLine != 3 {
		t.Errorf("packages: StartLine = %d, want 3", fs.Value.StartLine)
	}
	if fs.Value.StartCol != 18 {
		t.Errorf("packages: StartCol = %d, want 18", fs.Value.StartCol)
	}
}

func TestKwargFieldSpans_SSHTarget(t *testing.T) {
	src := source.NewMemSource()
	src.Files["/config.scampi"] = []byte(`target.ssh(
    name="remote",
    host="10.0.0.1",
    user="admin",
    port=2222,
    insecure=True,
)
deploy(name="main", targets=["remote"], steps=[
    dir(path="/tmp/test"),
])
`)

	store := diagnostic.NewSourceStore()
	cfg, err := star.Eval(context.Background(), "/config.scampi", store, src)
	if err != nil {
		t.Fatalf("Eval failed: %v", err)
	}

	tgt := cfg.Targets["remote"]
	tests := []struct {
		field        string
		wantLine     int
		wantStartCol int
	}{
		{"host", 3, 10},
		{"user", 4, 10},
		{"port", 5, 10},
		{"insecure", 6, 14},
	}

	for _, tt := range tests {
		t.Run(tt.field, func(t *testing.T) {
			fs, ok := tgt.Fields[tt.field]
			if !ok {
				t.Fatalf("field %q not found in target Fields map", tt.field)
			}
			if fs.Value.StartLine != tt.wantLine {
				t.Errorf("StartLine = %d, want %d", fs.Value.StartLine, tt.wantLine)
			}
			if fs.Value.StartCol != tt.wantStartCol {
				t.Errorf("StartCol = %d, want %d", fs.Value.StartCol, tt.wantStartCol)
			}
		})
	}
}

func TestKwargFieldSpans_Loaded(t *testing.T) {
	src := source.NewMemSource()
	src.Files["/lib.scampi"] = []byte(`steps = [
    dir(
        path="/tmp/loaded",
        perm="0755",
    ),
]
`)
	src.Files["/config.scampi"] = []byte(`load("/lib.scampi", "steps")
target.local(name="host")
deploy(name="main", targets=["host"], steps=steps)
`)

	store := diagnostic.NewSourceStore()
	cfg, err := star.Eval(context.Background(), "/config.scampi", store, src)
	if err != nil {
		t.Fatalf("Eval failed: %v", err)
	}

	step := cfg.Deploy["main"].Steps[0]

	// path="/tmp/loaded" is on line 3 of lib.scampi
	fs := step.Fields["path"]
	if fs.Value.Filename != "/lib.scampi" {
		t.Errorf("path: Filename = %q, want /lib.scampi", fs.Value.Filename)
	}
	if fs.Value.StartLine != 3 {
		t.Errorf("path: StartLine = %d, want 3", fs.Value.StartLine)
	}
}

func TestKwargFieldSpans_FallbackForMissing(t *testing.T) {
	src := source.NewMemSource()
	src.Files["/config.scampi"] = []byte(`target.local(name="host")
deploy(name="main", targets=["host"], steps=[
    dir(path="/tmp/x"),
])
`)

	store := diagnostic.NewSourceStore()
	cfg, err := star.Eval(context.Background(), "/config.scampi", store, src)
	if err != nil {
		t.Fatalf("Eval failed: %v", err)
	}

	step := cfg.Deploy["main"].Steps[0]

	// "perm" is not passed in the call, so kwargsFieldSpans should fall back
	// to the call-site position (the Lparen of dir(...) on line 3)
	fs := step.Fields["perm"]
	if fs.Value.StartLine != 3 {
		t.Errorf("perm: StartLine = %d, want 3 (call-site fallback)", fs.Value.StartLine)
	}
}
