// SPDX-License-Identifier: GPL-3.0-only

package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"scampi.dev/scampi/internal/engine"
)

func emit(r *applyRenderer, code engine.Code, kind, name string, args ...any) {
	ref := &engine.Ref{Kind: kind, Name: name}
	r.Emit(context.Background(), code, ref, args...)
}

func TestApplyRenderer_CreateLine(t *testing.T) {
	var buf bytes.Buffer
	r := newApplyRenderer(&buf, ASCIIGlyphs, false)
	emit(r, engine.CodeApplySuccess, "file", "x", "action", "create", "path", "/p")
	out := buf.String()
	if !strings.Contains(out, "+ file.x") {
		t.Errorf("expected create sigil + ref; got %q", out)
	}
	if !strings.Contains(out, "create") {
		t.Errorf("expected 'create' label; got %q", out)
	}
}

func TestApplyRenderer_UpdateLine(t *testing.T) {
	var buf bytes.Buffer
	r := newApplyRenderer(&buf, ASCIIGlyphs, false)
	emit(r, engine.CodeApplySuccess, "file", "y", "action", "update", "path", "/p")
	if !strings.Contains(buf.String(), "~ file.y") {
		t.Errorf("expected update sigil + ref; got %q", buf.String())
	}
}

func TestApplyRenderer_AdoptLine(t *testing.T) {
	var buf bytes.Buffer
	r := newApplyRenderer(&buf, ASCIIGlyphs, false)
	emit(r, engine.CodeApplySuccess, "file", "z", "action", "adopt", "path", "/p")
	if !strings.Contains(buf.String(), "@ file.z") {
		t.Errorf("expected adopt sigil + ref; got %q", buf.String())
	}
}

func TestApplyRenderer_DestroyLine(t *testing.T) {
	var buf bytes.Buffer
	r := newApplyRenderer(&buf, ASCIIGlyphs, false)
	emit(r, engine.CodeDestroySuccess, "file", "old", "path", "/p")
	if !strings.Contains(buf.String(), "- file.old") {
		t.Errorf("expected destroy sigil + ref; got %q", buf.String())
	}
}

func TestApplyRenderer_HaltLine(t *testing.T) {
	var buf bytes.Buffer
	r := newApplyRenderer(&buf, ASCIIGlyphs, false)
	emit(r, engine.CodeApplyHalted, "file", "claim", "state", "matching")
	if !strings.Contains(buf.String(), "! file.claim") {
		t.Errorf("expected halt sigil + ref; got %q", buf.String())
	}
}

func TestApplyRenderer_FailedLineCarriesErr(t *testing.T) {
	var buf bytes.Buffer
	r := newApplyRenderer(&buf, ASCIIGlyphs, false)
	emit(r, engine.CodeApplyFailed, "file", "boom", "err", errors.New("permission denied"))
	out := buf.String()
	if !strings.Contains(out, "x file.boom") {
		t.Errorf("expected failed sigil + ref; got %q", out)
	}
	if !strings.Contains(out, "permission denied") {
		t.Errorf("expected err message; got %q", out)
	}
}

func TestApplyRenderer_SuppressesLogDebugAndInfo(t *testing.T) {
	var buf bytes.Buffer
	r := newApplyRenderer(&buf, ASCIIGlyphs, false)
	r.Emit(context.Background(), engine.CodeLogDebug, nil, "msg", "noise")
	r.Emit(context.Background(), engine.CodeLogInfo, nil, "msg", "lifecycle chatter")
	r.Emit(context.Background(), engine.CodeSnapshotReceived, nil, "resources", 3)
	if buf.Len() != 0 {
		t.Errorf("expected no output for debug/info/snapshot.received; got %q", buf.String())
	}
}

func TestApplyRenderer_PassesLogWarn(t *testing.T) {
	var buf bytes.Buffer
	r := newApplyRenderer(&buf, ASCIIGlyphs, false)
	r.Emit(context.Background(), engine.CodeLogWarn, nil, "msg", "something weird")
	if !strings.Contains(buf.String(), "something weird") {
		t.Errorf("warn should pass through; got %q", buf.String())
	}
}

func TestApplyRenderer_FinalizeShowsCounts(t *testing.T) {
	var buf bytes.Buffer
	r := newApplyRenderer(&buf, ASCIIGlyphs, false)
	emit(r, engine.CodeApplySuccess, "file", "a", "action", "create")
	emit(r, engine.CodeApplySuccess, "file", "b", "action", "create")
	emit(r, engine.CodeApplySuccess, "file", "c", "action", "update")
	emit(r, engine.CodeDestroySuccess, "file", "d")
	r.Finalize(nil)
	out := buf.String()
	if !strings.Contains(out, "Applied:") {
		t.Errorf("summary missing; got:\n%s", out)
	}
	if !strings.Contains(out, "2 created") {
		t.Errorf("create count wrong; got:\n%s", out)
	}
	if !strings.Contains(out, "1 updated") {
		t.Errorf("update count wrong; got:\n%s", out)
	}
	if !strings.Contains(out, "1 destroyed") {
		t.Errorf("destroy count wrong; got:\n%s", out)
	}
}

func TestApplyRenderer_FinalizeNothingToDo(t *testing.T) {
	var buf bytes.Buffer
	r := newApplyRenderer(&buf, ASCIIGlyphs, false)
	r.Finalize(nil)
	if !strings.Contains(buf.String(), "nothing to do") {
		t.Errorf("expected nothing-to-do message; got:\n%s", buf.String())
	}
}

func TestApplyRenderer_FinalizeSkippedAfterRejection(t *testing.T) {
	var buf bytes.Buffer
	r := newApplyRenderer(&buf, ASCIIGlyphs, false)
	r.Emit(context.Background(), engine.CodeSnapshotRejected, nil,
		"phase", "typecheck", "err", errors.New("file.x: missing required attr \"content\""))
	cursor := buf.Len()
	r.Finalize(errors.New("snapshot rejected"))
	if buf.Len() != cursor {
		t.Errorf("Finalize should be a no-op after snapshot rejection; got extra: %q", buf.String()[cursor:])
	}
}
