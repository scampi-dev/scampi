// SPDX-License-Identifier: GPL-3.0-only

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"scampi.dev/scampi/internal/engine"
)

func TestJSONRenderer_EmitsOneObjectPerLine(t *testing.T) {
	var buf bytes.Buffer
	r := newJSONRenderer(&buf, VerbosityDefault)
	ref := &engine.Ref{Kind: "file", Name: "a"}
	r.Emit(context.Background(), engine.CodeApplySuccess, ref, "action", "create", "path", "/x")
	r.Emit(context.Background(), engine.CodeTickComplete, nil, "duration", "5ms", "status", "ok")

	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines; got %d:\n%s", len(lines), buf.String())
	}
	for i, line := range lines {
		var rec map[string]any
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Errorf("line %d not JSON: %v\n%s", i, err, line)
		}
	}
}

func TestJSONRenderer_IncludesCodeRefAndAttrs(t *testing.T) {
	var buf bytes.Buffer
	r := newJSONRenderer(&buf, VerbosityDefault)
	ref := &engine.Ref{Kind: "file", Name: "x"}
	r.Emit(context.Background(), engine.CodeApplySuccess, ref,
		"action", "create", "path", "/p")

	var rec map[string]any
	if err := json.Unmarshal(bytes.TrimRight(buf.Bytes(), "\n"), &rec); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if rec["code"] != string(engine.CodeApplySuccess) {
		t.Errorf("code wrong: %v", rec["code"])
	}
	if rec["ref"] != "file.x" {
		t.Errorf("ref wrong: %v", rec["ref"])
	}
	if rec["action"] != "create" {
		t.Errorf("action wrong: %v", rec["action"])
	}
	if rec["path"] != "/p" {
		t.Errorf("path wrong: %v", rec["path"])
	}
	if _, ok := rec["ts"].(string); !ok {
		t.Errorf("ts missing or not string: %v", rec["ts"])
	}
}

func TestJSONRenderer_ErrSerializesAsString(t *testing.T) {
	var buf bytes.Buffer
	r := newJSONRenderer(&buf, VerbosityDefault)
	r.Emit(context.Background(), engine.CodeSnapshotRejected, nil,
		"phase", "typecheck",
		"err", errors.New(`file.x: missing required attr "content"`),
	)
	var rec map[string]any
	if err := json.Unmarshal(bytes.TrimRight(buf.Bytes(), "\n"), &rec); err != nil {
		t.Fatalf("decode: %v", err)
	}
	s, ok := rec["err"].(string)
	if !ok {
		t.Fatalf("err should be string; got %T: %v", rec["err"], rec["err"])
	}
	if !strings.Contains(s, "missing required attr") {
		t.Errorf("err body wrong: %v", s)
	}
}

func TestJSONRenderer_DefaultSuppressesDebug(t *testing.T) {
	var buf bytes.Buffer
	r := newJSONRenderer(&buf, VerbosityDefault)
	r.Emit(context.Background(), engine.CodeLogDebug, nil, "msg", "noise")
	r.Emit(context.Background(), engine.CodeLogInfo, nil, "msg", "starting")
	r.Emit(context.Background(), engine.CodeSnapshotReceived, nil, "resources", 3)
	if strings.Count(buf.String(), "\n") != 2 {
		t.Errorf("expected info+lifecycle (no debug); got: %q", buf.String())
	}
}

func TestJSONRenderer_QuietSuppressesInfo(t *testing.T) {
	var buf bytes.Buffer
	r := newJSONRenderer(&buf, VerbosityQuiet)
	r.Emit(context.Background(), engine.CodeLogInfo, nil, "msg", "starting")
	r.Emit(context.Background(), engine.CodeSnapshotReceived, nil, "resources", 3)
	if strings.Count(buf.String(), "\n") != 1 {
		t.Errorf("expected only lifecycle (no info); got: %q", buf.String())
	}
}

func TestJSONRenderer_VerbosePassesDebug(t *testing.T) {
	var buf bytes.Buffer
	r := newJSONRenderer(&buf, VerbosityVerbose)
	r.Emit(context.Background(), engine.CodeLogDebug, nil, "msg", "noise")
	if !strings.Contains(buf.String(), "noise") {
		t.Errorf("expected debug passed at -v; got: %q", buf.String())
	}
}
