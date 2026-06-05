// SPDX-License-Identifier: GPL-3.0-only

package main

import (
	"bytes"
	"context"
	"errors"
	"regexp"
	"strings"
	"testing"

	"scampi.dev/scampi/internal/engine"
)

func TestRunRenderer_TimestampPrefix(t *testing.T) {
	var buf bytes.Buffer
	r := newRunRenderer(&buf, ASCIIGlyphs, false)
	ref := &engine.Ref{Kind: "file", Name: "a"}
	r.Emit(context.Background(), engine.CodeApplySuccess, ref, "action", "create")
	out := buf.String()
	if !regexp.MustCompile(`\d{2}:\d{2}:\d{2}`).MatchString(out) {
		t.Errorf("expected timestamp prefix; got %q", out)
	}
	if !strings.Contains(out, "file.a") {
		t.Errorf("expected ref in output; got %q", out)
	}
}

func TestRunRenderer_SigilForApplySuccess(t *testing.T) {
	var buf bytes.Buffer
	r := newRunRenderer(&buf, ASCIIGlyphs, false)
	ref := &engine.Ref{Kind: "file", Name: "a"}
	r.Emit(context.Background(), engine.CodeApplySuccess, ref, "action", "create")
	out := buf.String()
	if !strings.Contains(out, "+") {
		t.Errorf("expected create sigil; got %q", out)
	}
	if !strings.Contains(out, "file.a") {
		t.Errorf("expected ref; got %q", out)
	}
	if !strings.Contains(out, "create") {
		t.Errorf("expected label; got %q", out)
	}
}

func TestRunRenderer_SuppressesDebugAndSnapshotReceived(t *testing.T) {
	var buf bytes.Buffer
	r := newRunRenderer(&buf, ASCIIGlyphs, false)
	r.Emit(context.Background(), engine.CodeLogDebug, nil, "msg", "parsing")
	r.Emit(context.Background(), engine.CodeSnapshotReceived, nil, "resources", 3)
	if buf.Len() != 0 {
		t.Errorf("expected debug/snapshot.received suppressed; got %q", buf.String())
	}
}

func TestRunRenderer_PassesInfoWarnError(t *testing.T) {
	for _, c := range []struct {
		name string
		code engine.Code
		msg  string
		tag  string
	}{
		{"info", engine.CodeLogInfo, "starting run loop", "INF"},
		{"warn", engine.CodeLogWarn, "something funky", "WRN"},
		{"error", engine.CodeLogError, "things broke", "ERR"},
	} {
		t.Run(c.name, func(t *testing.T) {
			var buf bytes.Buffer
			r := newRunRenderer(&buf, ASCIIGlyphs, false)
			r.Emit(context.Background(), c.code, nil, "msg", c.msg)
			out := buf.String()
			if !strings.Contains(out, c.tag) {
				t.Errorf("expected tag %s; got %q", c.tag, out)
			}
			if !strings.Contains(out, c.msg) {
				t.Errorf("expected msg %s; got %q", c.msg, out)
			}
		})
	}
}

func TestRunRenderer_InfoKeepsAttrs(t *testing.T) {
	var buf bytes.Buffer
	r := newRunRenderer(&buf, ASCIIGlyphs, false)
	r.Emit(context.Background(), engine.CodeLogInfo, nil,
		"msg", "starting run loop", "dir", "/cfg", "interval", "500ms")
	out := buf.String()
	if !strings.Contains(out, "dir=/cfg") {
		t.Errorf("expected dir attr; got %q", out)
	}
	if !strings.Contains(out, "interval=500ms") {
		t.Errorf("expected interval attr; got %q", out)
	}
}

func TestRunRenderer_TickSummaryAfterEvents(t *testing.T) {
	var buf bytes.Buffer
	r := newRunRenderer(&buf, ASCIIGlyphs, false)
	ref := &engine.Ref{Kind: "file", Name: "a"}
	r.Emit(context.Background(), engine.CodeApplySuccess, ref, "action", "create")
	r.Emit(context.Background(), engine.CodeTickComplete, nil, "duration", "12ms", "status", "ok")
	out := buf.String()
	if !strings.Contains(out, "OK") || !strings.Contains(out, "reconcile") {
		t.Errorf("expected reconcile-ok summary line; got %q", out)
	}
	if !strings.Contains(out, "12ms") {
		t.Errorf("expected duration; got %q", out)
	}
}

func TestRunRenderer_NoSummaryWhenSilentTick(t *testing.T) {
	var buf bytes.Buffer
	r := newRunRenderer(&buf, ASCIIGlyphs, false)
	r.Emit(context.Background(), engine.CodeTickComplete, nil, "duration", "1ms", "status", "ok")
	if buf.Len() != 0 {
		t.Errorf("expected silent on tick with no events; got %q", buf.String())
	}
}

func TestRunRenderer_TickSummaryFailedTag(t *testing.T) {
	var buf bytes.Buffer
	r := newRunRenderer(&buf, ASCIIGlyphs, false)
	ref := &engine.Ref{Kind: "file", Name: "boom"}
	r.Emit(context.Background(), engine.CodeApplyFailed, ref, "err", "permission denied")
	r.Emit(context.Background(), engine.CodeTickComplete, nil, "duration", "5ms", "status", "failed")
	out := buf.String()
	if !strings.Contains(out, "ERR") {
		t.Errorf("expected ERR tag for failed tick; got %q", out)
	}
	if !strings.Contains(out, "failed") {
		t.Errorf("expected failed status; got %q", out)
	}
}

func TestRunRenderer_SnapshotRejectedAsBlock(t *testing.T) {
	var buf bytes.Buffer
	r := newRunRenderer(&buf, ASCIIGlyphs, false)
	r.Emit(context.Background(), engine.CodeSnapshotRejected, nil,
		"phase", "typecheck",
		"err", errors.New(`file.x: missing required attr "content"`),
	)
	out := buf.String()
	if !strings.Contains(out, "snapshot rejected at typecheck") {
		t.Errorf("expected snapshot block header; got %q", out)
	}
	if !strings.Contains(out, "missing required attr") {
		t.Errorf("expected error body; got %q", out)
	}
}
