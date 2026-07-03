// SPDX-License-Identifier: GPL-3.0-only

package mod_test

import (
	"errors"
	"testing"

	"scampi.dev/scampi/internal/diagnostic/event"
	"scampi.dev/scampi/internal/mod"
)

const testFile = "scampi.mod"

func Test_Parse_AcceptsModuleOnly(t *testing.T) {
	data := []byte("module github.com/pskry/skrynet\n")
	m, err := mod.Parse(testFile, data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.Module != "github.com/pskry/skrynet" {
		t.Errorf("module = %q, want %q", m.Module, "github.com/pskry/skrynet")
	}
	if m.ModuleLine != 1 {
		t.Errorf("ModuleLine = %d, want 1", m.ModuleLine)
	}
	if len(m.Require) != 0 {
		t.Errorf("Require = %v, want empty", m.Require)
	}
}

func Test_Parse_AcceptsRequireBlock(t *testing.T) {
	data := []byte(`module github.com/pskry/skrynet

require (
    github.com/scampi-modules/npm v1.0.0
    github.com/scampi-modules/authelia v0.3.2
)
`)
	m, err := mod.Parse(testFile, data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.Module != "github.com/pskry/skrynet" {
		t.Errorf("module = %q", m.Module)
	}
	if len(m.Require) != 2 {
		t.Fatalf("len(Require) = %d, want 2", len(m.Require))
	}
	if m.Require[0].Path != "github.com/scampi-modules/npm" {
		t.Errorf("Require[0].Path = %q", m.Require[0].Path)
	}
	if m.Require[0].Version != "v1.0.0" {
		t.Errorf("Require[0].Version = %q", m.Require[0].Version)
	}
	if m.Require[1].Path != "github.com/scampi-modules/authelia" {
		t.Errorf("Require[1].Path = %q", m.Require[1].Path)
	}
	if m.Require[1].Version != "v0.3.2" {
		t.Errorf("Require[1].Version = %q", m.Require[1].Version)
	}
}

func Test_Parse_AcceptsEmptyRequireBlock(t *testing.T) {
	data := []byte("module github.com/pskry/skrynet\n\nrequire (\n)\n")
	m, err := mod.Parse(testFile, data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(m.Require) != 0 {
		t.Errorf("Require should be empty, got %v", m.Require)
	}
}

func Test_Parse_IgnoresComments(t *testing.T) {
	data := []byte(`// This is a module manifest
module github.com/pskry/skrynet // inline comment

require (
    // pin npm for compatibility
    github.com/scampi-modules/npm v1.0.0 // locked
)
`)
	m, err := mod.Parse(testFile, data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.Module != "github.com/pskry/skrynet" {
		t.Errorf("module = %q", m.Module)
	}
	if len(m.Require) != 1 {
		t.Fatalf("len(Require) = %d, want 1", len(m.Require))
	}
	if m.Require[0].Path != "github.com/scampi-modules/npm" {
		t.Errorf("Require[0].Path = %q", m.Require[0].Path)
	}
}

func Test_Parse_AcceptsPreReleaseVersion(t *testing.T) {
	data := []byte("module github.com/pskry/skrynet\n\n" +
		"require (\n    github.com/scampi-modules/npm v1.0.0-alpha.1\n)\n")
	m, err := mod.Parse(testFile, data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.Require[0].Version != "v1.0.0-alpha.1" {
		t.Errorf("Version = %q, want v1.0.0-alpha.1", m.Require[0].Version)
	}
}

func Test_Parse_TracksLineNumbers(t *testing.T) {
	data := []byte(`module github.com/pskry/skrynet

require (
    github.com/scampi-modules/npm v1.0.0
)
`)
	m, err := mod.Parse(testFile, data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.ModuleLine != 1 {
		t.Errorf("ModuleLine = %d, want 1", m.ModuleLine)
	}
	if m.Require[0].Line != 4 {
		t.Errorf("Require[0].Line = %d, want 4", m.Require[0].Line)
	}
}

func Test_Parse_SetsFilename(t *testing.T) {
	data := []byte("module github.com/pskry/skrynet\n")
	m, err := mod.Parse("path/to/scampi.mod", data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.Filename != "path/to/scampi.mod" {
		t.Errorf("Filename = %q", m.Filename)
	}
}

func Test_Parse_RejectsMissingModuleDirective(t *testing.T) {
	data := []byte("require (\n    github.com/scampi-modules/npm v1.0.0\n)\n")
	_, err := mod.Parse(testFile, data)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var pe mod.ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("expected ParseError, got %T: %v", err, err)
	}
}

func Test_Parse_RejectsEmptyFile(t *testing.T) {
	_, err := mod.Parse(testFile, []byte{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var pe mod.ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("expected ParseError, got %T", err)
	}
	if pe.Span.StartLine != 0 {
		t.Errorf("expected no line for missing-module error, got %d", pe.Span.StartLine)
	}
	if pe.Span.Filename != testFile {
		t.Errorf("Source.Filename = %q, want %q", pe.Span.Filename, testFile)
	}
}

func Test_Parse_RejectsDuplicateModule(t *testing.T) {
	data := []byte("module github.com/pskry/skrynet\nmodule github.com/pskry/other\n")
	_, err := mod.Parse(testFile, data)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var pe mod.ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("expected ParseError, got %T", err)
	}
	if pe.Span.StartLine != 2 {
		t.Errorf("expected error on line 2, got %d", pe.Span.StartLine)
	}
}

func Test_Parse_RejectsInvalidModulePath(t *testing.T) {
	data := []byte("module notavalidpath\n")
	_, err := mod.Parse(testFile, data)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var pe mod.ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("expected ParseError, got %T", err)
	}
	if pe.Span.StartLine != 1 {
		t.Errorf("expected error on line 1, got %d", pe.Span.StartLine)
	}
}

func Test_Parse_AcceptsBranchVersion(t *testing.T) {
	data := []byte("module github.com/pskry/skrynet\n\nrequire (\n    github.com/scampi-modules/npm main\n)\n")
	m, err := mod.Parse(testFile, data)
	if err != nil {
		t.Fatalf("expected branch version to be accepted, got: %v", err)
	}
	if len(m.Require) != 1 || m.Require[0].Version != "main" {
		t.Errorf("got %+v", m.Require)
	}
}

func Test_Parse_RejectsMalformedRequireEntry(t *testing.T) {
	data := []byte("module github.com/pskry/skrynet\n\nrequire (\n    github.com/scampi-modules/npm\n)\n")
	_, err := mod.Parse(testFile, data)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var pe mod.ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("expected ParseError, got %T", err)
	}
	if pe.Span.StartLine != 4 {
		t.Errorf("expected error on line 4, got %d", pe.Span.StartLine)
	}
}

func Test_Parse_SetsErrorSpanFilename(t *testing.T) {
	const filename = "path/to/scampi.mod"
	data := []byte("module notapath\n")
	_, err := mod.Parse(filename, data)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var pe mod.ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("expected ParseError, got %T", err)
	}
	if pe.Span.Filename != filename {
		t.Errorf("Source.Filename = %q, want %q", pe.Span.Filename, filename)
	}
}

func Test_Parse_ErrorMessageIncludesLine(t *testing.T) {
	// Use a genuinely invalid entry (missing version entirely).
	data := []byte("module github.com/pskry/skrynet\n\nrequire (\n    github.com/scampi-modules/npm\n)\n")
	_, err := mod.Parse(testFile, data)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	msg := err.Error()
	if msg == "" {
		t.Error("Error() returned empty string")
	}
	// Should include filename and line
	if !contains(msg, testFile) {
		t.Errorf("error message %q does not contain filename %q", msg, testFile)
	}
	if !contains(msg, "4") {
		t.Errorf("error message %q does not contain line number", msg)
	}
}

func Test_Parse_RejectsUnclosedRequireBlock(t *testing.T) {
	data := []byte("module github.com/pskry/skrynet\n\nrequire (\n    github.com/scampi-modules/npm v1.0.0\n")
	_, err := mod.Parse(testFile, data)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var pe mod.ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("expected ParseError, got %T", err)
	}
}

func Test_Parse_RejectsUnexpectedToken(t *testing.T) {
	data := []byte("module github.com/pskry/skrynet\nfoobar\n")
	_, err := mod.Parse(testFile, data)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var pe mod.ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("expected ParseError, got %T", err)
	}
	if pe.Span.StartLine != 2 {
		t.Errorf("expected error on line 2, got %d", pe.Span.StartLine)
	}
}

func Test_Parse_EmitsTypedDiagnostic(t *testing.T) {
	data := []byte("module notapath\n")
	_, err := mod.Parse(testFile, data)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var pe mod.ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("expected ParseError, got %T", err)
	}
	tmpl := pe.Diagnostic().(event.Error).Template
	if tmpl.ID != "mod.ParseError" {
		t.Errorf("Template.ID = %q, want %q", tmpl.ID, "mod.ParseError")
	}
	if tmpl.Span == nil {
		t.Error("Template.Span is nil")
	}
	if tmpl.Hint == "" {
		t.Error("Template.Hint is empty")
	}
}

func Test_DepSpan_MatchesDepLine(t *testing.T) {
	data := []byte("module github.com/pskry/skrynet\n\nrequire (\n    github.com/scampi-modules/npm v1.0.0\n)\n")
	m, err := mod.Parse(testFile, data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	dep := &m.Require[0]
	span := m.DepSpan(dep)
	if span.Filename != testFile {
		t.Errorf("DepSpan.Filename = %q, want %q", span.Filename, testFile)
	}
	if span.StartLine != dep.Line {
		t.Errorf("DepSpan.StartLine = %d, want %d", span.StartLine, dep.Line)
	}
	if span.EndLine != dep.Line {
		t.Errorf("DepSpan.EndLine = %d, want %d", span.EndLine, dep.Line)
	}
}

func Test_IsModulePath_ClassifiesPaths(t *testing.T) {
	valid := []string{
		"github.com/pskry/skrynet",
		"github.com/foo/bar",
		"github.com/scampi-modules/npm",
	}
	for _, p := range valid {
		if !mod.IsModulePath(p) {
			t.Errorf("IsModulePath(%q) = false, want true", p)
		}
	}
	invalid := []string{
		"",
		"notapath",
		"nodot/path",
		"host.com",
		"host.com/",
	}
	for _, p := range invalid {
		if mod.IsModulePath(p) {
			t.Errorf("IsModulePath(%q) = true, want false", p)
		}
	}
}

func Test_Parse_DetectsIndirectFlag(t *testing.T) {
	data := []byte(`module github.com/pskry/skrynet

require (
    github.com/scampi-modules/npm v1.0.0
    github.com/scampi-modules/authelia v0.3.2 // indirect
)
`)
	m, err := mod.Parse(testFile, data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(m.Require) != 2 {
		t.Fatalf("len(Require) = %d, want 2", len(m.Require))
	}
	if m.Require[0].Indirect {
		t.Errorf("Require[0].Indirect = true, want false for direct dep")
	}
	if !m.Require[1].Indirect {
		t.Errorf("Require[1].Indirect = false, want true for indirect dep")
	}
}

func Test_Parse_LeavesIndirectFalse(t *testing.T) {
	data := []byte(`module github.com/pskry/skrynet

require (
    github.com/scampi-modules/npm v1.0.0
    github.com/scampi-modules/authelia v0.3.2
)
`)
	m, err := mod.Parse(testFile, data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for i, dep := range m.Require {
		if dep.Indirect {
			t.Errorf("Require[%d].Indirect = true, want false", i)
		}
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || indexString(s, sub) >= 0)
}

func indexString(s, sub string) int {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
