// SPDX-License-Identifier: GPL-3.0-only

package render

import (
	"bytes"
	"strings"
	"testing"

	"scampi.dev/scampi/internal/engine"
)

func ref(kind, name string) engine.Ref { return engine.Ref{Kind: kind, Name: name} }

func TestPrintPlan_Empty(t *testing.T) {
	var buf bytes.Buffer
	PrintPlan(&buf, &engine.Plan{}, ASCIIGlyphs, false)
	if !strings.Contains(buf.String(), "plan: empty") {
		t.Errorf("expected 'plan: empty'; got %q", buf.String())
	}
}

func TestPrintPlan_AllSectionsRendered(t *testing.T) {
	p := &engine.Plan{
		Create:  []engine.Ref{ref("file", "new")},
		Update:  []engine.Ref{ref("file", "drift")},
		Adopt:   []engine.Ref{ref("file", "claim")},
		Halt:    []engine.Ref{ref("file", "unauth")},
		Destroy: []engine.Ref{ref("file", "gone")},
		InSync:  []engine.Ref{ref("file", "match")},
	}
	var buf bytes.Buffer
	PrintPlan(&buf, p, ASCIIGlyphs, false)
	out := buf.String()
	for _, want := range []string{
		"file.new", "file.drift", "file.claim",
		"file.unauth", "file.gone", "file.match",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %s; got:\n%s", want, out)
		}
	}
	for _, label := range []string{"create", "update", "adopt", "halt", "destroy", "in sync"} {
		if !strings.Contains(out, label) {
			t.Errorf("output missing label %q; got:\n%s", label, out)
		}
	}
}

func TestPrintPlan_RefsSortedAlphabetically(t *testing.T) {
	p := &engine.Plan{
		Create:  []engine.Ref{ref("file", "z")},
		Destroy: []engine.Ref{ref("file", "a")},
		InSync:  []engine.Ref{ref("file", "m")},
	}
	var buf bytes.Buffer
	PrintPlan(&buf, p, ASCIIGlyphs, false)
	out := buf.String()
	aIdx := strings.Index(out, "file.a")
	mIdx := strings.Index(out, "file.m")
	zIdx := strings.Index(out, "file.z")
	if aIdx >= mIdx || mIdx >= zIdx {
		t.Errorf("refs not sorted: a=%d m=%d z=%d\n%s", aIdx, mIdx, zIdx, out)
	}
}

func TestPrintPlan_SummaryStatsLine(t *testing.T) {
	p := &engine.Plan{
		Create: []engine.Ref{ref("file", "a"), ref("file", "b")},
		Update: []engine.Ref{ref("file", "c")},
	}
	var buf bytes.Buffer
	PrintPlan(&buf, p, ASCIIGlyphs, false)
	out := buf.String()
	if !strings.Contains(out, "Plan:") {
		t.Errorf("summary line missing; got:\n%s", out)
	}
	if !strings.Contains(out, "2 create") {
		t.Errorf("create count missing; got:\n%s", out)
	}
	if !strings.Contains(out, "1 update") {
		t.Errorf("update count missing; got:\n%s", out)
	}
}

func TestPrintPlan_GlyphsSwap(t *testing.T) {
	p := &engine.Plan{Create: []engine.Ref{ref("file", "a")}}
	var ascii, unicode bytes.Buffer
	PrintPlan(&ascii, p, ASCIIGlyphs, false)
	PrintPlan(&unicode, p, UnicodeGlyphs, false)
	if !strings.Contains(ascii.String(), "+") {
		t.Errorf("ascii output should contain '+'; got %q", ascii.String())
	}
	if !strings.Contains(unicode.String(), "✚") {
		t.Errorf("unicode output should contain glyph; got %q", unicode.String())
	}
	if ascii.String() == unicode.String() {
		t.Error("ascii and unicode renderings should differ")
	}
}

func TestPrintPlan_ColoredHasAnsi(t *testing.T) {
	p := &engine.Plan{Create: []engine.Ref{ref("file", "a")}}
	var buf bytes.Buffer
	PrintPlan(&buf, p, ASCIIGlyphs, true)
	if !strings.Contains(buf.String(), "\x1b[") {
		t.Errorf("colored output missing ANSI; got %q", buf.String())
	}
}
