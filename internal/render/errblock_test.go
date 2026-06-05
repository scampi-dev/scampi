// SPDX-License-Identifier: GPL-3.0-only

package render

import (
	"bytes"
	"strings"
	"testing"
)

func TestRenderSnapshotRejected_SingleError(t *testing.T) {
	var buf bytes.Buffer
	if err := renderSnapshotRejected(&buf, "2026-06-05 10:00:00", "typecheck",
		`file.x: missing required attr "content"`, false); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "typecheck") {
		t.Errorf("output should mention phase; got %q", out)
	}
	if !strings.Contains(out, "1 error") || strings.Contains(out, "1 errors") {
		t.Errorf("singular form expected; got %q", out)
	}
	if !strings.Contains(out, `file.x: missing required attr "content"`) {
		t.Errorf("error body should appear; got %q", out)
	}
}

func TestRenderSnapshotRejected_MultipleErrorsAllShown(t *testing.T) {
	joined := `file.x: missing required attr "content"` + "\n" +
		`file.x: unknown attr "contnet"`
	var buf bytes.Buffer
	if err := renderSnapshotRejected(&buf, "2026-06-05 10:00:00", "typecheck", joined, false); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "2 errors") {
		t.Errorf("output should say 2 errors (plural); got %q", out)
	}
	if !strings.Contains(out, `file.x: missing required attr "content"`) {
		t.Errorf("first error missing; got %q", out)
	}
	if !strings.Contains(out, `file.x: unknown attr "contnet"`) {
		t.Errorf("second error missing; got %q", out)
	}
}

func TestRenderSnapshotRejected_ErrorLinesIndented(t *testing.T) {
	joined := "a\nb"
	var buf bytes.Buffer
	if err := renderSnapshotRejected(&buf, "ts", "typecheck", joined, false); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected header + 2 error lines; got %d:\n%s", len(lines), buf.String())
	}
	for _, line := range lines[1:] {
		if !strings.HasPrefix(line, "    ") {
			t.Errorf("error line not indented: %q", line)
		}
	}
}

func TestRenderSnapshotRejected_TrailingNewlineNotDoubled(t *testing.T) {
	var buf bytes.Buffer
	if err := renderSnapshotRejected(&buf, "ts", "typecheck", "one\ntwo\n", false); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "    \n") {
		t.Errorf("trailing newline produced empty indented line: %q", buf.String())
	}
}

func TestRenderSnapshotRejected_ColoredHasAnsi(t *testing.T) {
	var buf bytes.Buffer
	if err := renderSnapshotRejected(&buf, "ts", "typecheck", "one", true); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "\x1b[") {
		t.Errorf("colored output should contain ANSI escapes; got %q", buf.String())
	}
}
