// SPDX-License-Identifier: GPL-3.0-only

package render

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"scampi.dev/scampi/internal/engine"
)

func emit(r *ReportRenderer, code engine.Code, kind, name string, args ...any) {
	ref := &engine.Ref{Kind: kind, Name: name}
	r.Emit(context.Background(), code, ref, args...)
}

func assertLine(t *testing.T, out, sigil, ref, label string) {
	t.Helper()
	if !strings.Contains(out, sigil) {
		t.Errorf("expected sigil %q; got %q", sigil, out)
	}
	if !strings.Contains(out, ref) {
		t.Errorf("expected ref %q; got %q", ref, out)
	}
	if !strings.Contains(out, label) {
		t.Errorf("expected label %q; got %q", label, out)
	}
}

func TestReportRenderer_CreateLine(t *testing.T) {
	var buf bytes.Buffer
	r := NewReportRenderer(&buf, ASCIIGlyphs, false, 0)
	emit(r, engine.CodeApplySuccess, "file", "x", "action", "create", "path", "/p")
	assertLine(t, buf.String(), "+", "file.x", "create")
}

func TestReportRenderer_UpdateLine(t *testing.T) {
	var buf bytes.Buffer
	r := NewReportRenderer(&buf, ASCIIGlyphs, false, 0)
	emit(r, engine.CodeApplySuccess, "file", "y", "action", "update", "path", "/p")
	assertLine(t, buf.String(), "~", "file.y", "update")
}

func TestReportRenderer_AdoptLine(t *testing.T) {
	var buf bytes.Buffer
	r := NewReportRenderer(&buf, ASCIIGlyphs, false, 0)
	emit(r, engine.CodeApplySuccess, "file", "z", "action", "adopt", "path", "/p")
	assertLine(t, buf.String(), "@", "file.z", "adopt")
}

func TestReportRenderer_DestroyLine(t *testing.T) {
	var buf bytes.Buffer
	r := NewReportRenderer(&buf, ASCIIGlyphs, false, 0)
	emit(r, engine.CodeDestroySuccess, "file", "old", "path", "/p")
	assertLine(t, buf.String(), "-", "file.old", "destroy")
}

func TestReportRenderer_HaltLine(t *testing.T) {
	var buf bytes.Buffer
	r := NewReportRenderer(&buf, ASCIIGlyphs, false, 0)
	emit(r, engine.CodeApplyHalted, "file", "claim", "state", "matching")
	assertLine(t, buf.String(), "!", "file.claim", "halt")
}

func TestReportRenderer_FailedLineCarriesErr(t *testing.T) {
	var buf bytes.Buffer
	r := NewReportRenderer(&buf, ASCIIGlyphs, false, 0)
	emit(r, engine.CodeApplyFailed, "file", "boom", "err", errors.New("permission denied"))
	out := buf.String()
	assertLine(t, out, "x", "file.boom", "failed")
	if !strings.Contains(out, "permission denied") {
		t.Errorf("expected err message; got %q", out)
	}
}

func TestReportRenderer_SuppressesLogDebugAndInfo(t *testing.T) {
	var buf bytes.Buffer
	r := NewReportRenderer(&buf, ASCIIGlyphs, false, 0)
	r.Emit(context.Background(), engine.CodeLogDebug, nil, "msg", "noise")
	r.Emit(context.Background(), engine.CodeLogInfo, nil, "msg", "lifecycle chatter")
	r.Emit(context.Background(), engine.CodeSnapshotReceived, nil, "resources", 3)
	if buf.Len() != 0 {
		t.Errorf("expected no output for debug/info/snapshot.received; got %q", buf.String())
	}
}

func TestReportRenderer_PassesLogWarn(t *testing.T) {
	var buf bytes.Buffer
	r := NewReportRenderer(&buf, ASCIIGlyphs, false, 0)
	r.Emit(context.Background(), engine.CodeLogWarn, nil, "msg", "something weird")
	if !strings.Contains(buf.String(), "something weird") {
		t.Errorf("warn should pass through; got %q", buf.String())
	}
}

func TestReportRenderer_FinalizeShowsCounts(t *testing.T) {
	var buf bytes.Buffer
	r := NewReportRenderer(&buf, ASCIIGlyphs, false, 0)
	emit(r, engine.CodeApplySuccess, "file", "a", "action", "create")
	emit(r, engine.CodeApplySuccess, "file", "b", "action", "create")
	emit(r, engine.CodeApplySuccess, "file", "c", "action", "update")
	emit(r, engine.CodeDestroySuccess, "file", "d")
	r.Finalize(nil)
	out := buf.String()
	if !strings.Contains(out, "Reconciled:") {
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

func TestReportRenderer_FinalizeNothingToDo(t *testing.T) {
	var buf bytes.Buffer
	r := NewReportRenderer(&buf, ASCIIGlyphs, false, 0)
	r.Finalize(nil)
	if !strings.Contains(buf.String(), "nothing to do") {
		t.Errorf("expected nothing-to-do message; got:\n%s", buf.String())
	}
}

func TestReportRenderer_FinalizeSkippedAfterRejection(t *testing.T) {
	var buf bytes.Buffer
	r := NewReportRenderer(&buf, ASCIIGlyphs, false, 0)
	r.Emit(context.Background(), engine.CodeSnapshotRejected, nil,
		"phase", "typecheck", "err", errors.New("file.x: missing required attr \"content\""))
	cursor := buf.Len()
	r.Finalize(errors.New("snapshot rejected"))
	if buf.Len() != cursor {
		t.Errorf(
			"Finalize should be a no-op after snapshot rejection; got extra: %q",
			buf.String()[cursor:],
		)
	}
}
