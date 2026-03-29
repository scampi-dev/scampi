// SPDX-License-Identifier: GPL-3.0-only

package mod_test

import (
	"errors"
	"testing"

	"scampi.dev/scampi/mod"
)

const testFile = "scampi.mod"

func TestParse_HappyPath_ModuleOnly(t *testing.T) {
	data := []byte("module codeberg.org/pskry/skrynet\n")
	m, err := mod.Parse(testFile, data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.Module != "codeberg.org/pskry/skrynet" {
		t.Errorf("module = %q, want %q", m.Module, "codeberg.org/pskry/skrynet")
	}
	if m.ModuleLine != 1 {
		t.Errorf("ModuleLine = %d, want 1", m.ModuleLine)
	}
	if len(m.Require) != 0 {
		t.Errorf("Require = %v, want empty", m.Require)
	}
}

func TestParse_HappyPath_WithRequire(t *testing.T) {
	data := []byte(`module codeberg.org/pskry/skrynet

require (
    codeberg.org/scampi-modules/npm v1.0.0
    codeberg.org/scampi-modules/authelia v0.3.2
)
`)
	m, err := mod.Parse(testFile, data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.Module != "codeberg.org/pskry/skrynet" {
		t.Errorf("module = %q", m.Module)
	}
	if len(m.Require) != 2 {
		t.Fatalf("len(Require) = %d, want 2", len(m.Require))
	}
	if m.Require[0].Path != "codeberg.org/scampi-modules/npm" {
		t.Errorf("Require[0].Path = %q", m.Require[0].Path)
	}
	if m.Require[0].Version != "v1.0.0" {
		t.Errorf("Require[0].Version = %q", m.Require[0].Version)
	}
	if m.Require[1].Path != "codeberg.org/scampi-modules/authelia" {
		t.Errorf("Require[1].Path = %q", m.Require[1].Path)
	}
	if m.Require[1].Version != "v0.3.2" {
		t.Errorf("Require[1].Version = %q", m.Require[1].Version)
	}
}

func TestParse_HappyPath_EmptyRequireBlock(t *testing.T) {
	data := []byte("module codeberg.org/pskry/skrynet\n\nrequire (\n)\n")
	m, err := mod.Parse(testFile, data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(m.Require) != 0 {
		t.Errorf("Require should be empty, got %v", m.Require)
	}
}

func TestParse_HappyPath_Comments(t *testing.T) {
	data := []byte(`// This is a module manifest
module codeberg.org/pskry/skrynet // inline comment

require (
    // pin npm for compatibility
    codeberg.org/scampi-modules/npm v1.0.0 // locked
)
`)
	m, err := mod.Parse(testFile, data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.Module != "codeberg.org/pskry/skrynet" {
		t.Errorf("module = %q", m.Module)
	}
	if len(m.Require) != 1 {
		t.Fatalf("len(Require) = %d, want 1", len(m.Require))
	}
	if m.Require[0].Path != "codeberg.org/scampi-modules/npm" {
		t.Errorf("Require[0].Path = %q", m.Require[0].Path)
	}
}

func TestParse_HappyPath_PreReleaseVersion(t *testing.T) {
	data := []byte("module codeberg.org/pskry/skrynet\n\n" +
		"require (\n    codeberg.org/scampi-modules/npm v1.0.0-alpha.1\n)\n")
	m, err := mod.Parse(testFile, data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.Require[0].Version != "v1.0.0-alpha.1" {
		t.Errorf("Version = %q, want v1.0.0-alpha.1", m.Require[0].Version)
	}
}

func TestParse_HappyPath_LineNumbers(t *testing.T) {
	data := []byte(`module codeberg.org/pskry/skrynet

require (
    codeberg.org/scampi-modules/npm v1.0.0
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

func TestParse_HappyPath_Filename(t *testing.T) {
	data := []byte("module codeberg.org/pskry/skrynet\n")
	m, err := mod.Parse("path/to/scampi.mod", data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.Filename != "path/to/scampi.mod" {
		t.Errorf("Filename = %q", m.Filename)
	}
}

func TestParse_Error_MissingModuleDirective(t *testing.T) {
	data := []byte("require (\n    codeberg.org/scampi-modules/npm v1.0.0\n)\n")
	_, err := mod.Parse(testFile, data)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var pe mod.ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("expected ParseError, got %T: %v", err, err)
	}
}

func TestParse_Error_MissingModuleDirective_EmptyFile(t *testing.T) {
	_, err := mod.Parse(testFile, []byte{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var pe mod.ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("expected ParseError, got %T", err)
	}
	if pe.Source.StartLine != 0 {
		t.Errorf("expected no line for missing-module error, got %d", pe.Source.StartLine)
	}
	if pe.Source.Filename != testFile {
		t.Errorf("Source.Filename = %q, want %q", pe.Source.Filename, testFile)
	}
}

func TestParse_Error_DuplicateModule(t *testing.T) {
	data := []byte("module codeberg.org/pskry/skrynet\nmodule codeberg.org/pskry/other\n")
	_, err := mod.Parse(testFile, data)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var pe mod.ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("expected ParseError, got %T", err)
	}
	if pe.Source.StartLine != 2 {
		t.Errorf("expected error on line 2, got %d", pe.Source.StartLine)
	}
}

func TestParse_Error_InvalidModulePath(t *testing.T) {
	data := []byte("module notavalidpath\n")
	_, err := mod.Parse(testFile, data)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var pe mod.ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("expected ParseError, got %T", err)
	}
	if pe.Source.StartLine != 1 {
		t.Errorf("expected error on line 1, got %d", pe.Source.StartLine)
	}
}

func TestParse_Error_InvalidVersion(t *testing.T) {
	data := []byte("module codeberg.org/pskry/skrynet\n\nrequire (\n    codeberg.org/scampi-modules/npm latest\n)\n")
	_, err := mod.Parse(testFile, data)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var pe mod.ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("expected ParseError, got %T", err)
	}
	if pe.Source.StartLine != 4 {
		t.Errorf("expected error on line 4, got %d", pe.Source.StartLine)
	}
}

func TestParse_Error_MalformedRequireEntry(t *testing.T) {
	data := []byte("module codeberg.org/pskry/skrynet\n\nrequire (\n    codeberg.org/scampi-modules/npm\n)\n")
	_, err := mod.Parse(testFile, data)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var pe mod.ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("expected ParseError, got %T", err)
	}
	if pe.Source.StartLine != 4 {
		t.Errorf("expected error on line 4, got %d", pe.Source.StartLine)
	}
}

func TestParse_Error_SourceSpanFilename(t *testing.T) {
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
	if pe.Source.Filename != filename {
		t.Errorf("Source.Filename = %q, want %q", pe.Source.Filename, filename)
	}
}

func TestParse_Error_ErrorMessageIncludesLine(t *testing.T) {
	data := []byte("module codeberg.org/pskry/skrynet\n\nrequire (\n    codeberg.org/scampi-modules/npm latest\n)\n")
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

func TestParse_Error_UnclosedRequireBlock(t *testing.T) {
	data := []byte("module codeberg.org/pskry/skrynet\n\nrequire (\n    codeberg.org/scampi-modules/npm v1.0.0\n")
	_, err := mod.Parse(testFile, data)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var pe mod.ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("expected ParseError, got %T", err)
	}
}

func TestParse_Error_UnexpectedToken(t *testing.T) {
	data := []byte("module codeberg.org/pskry/skrynet\nfoobar\n")
	_, err := mod.Parse(testFile, data)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var pe mod.ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("expected ParseError, got %T", err)
	}
	if pe.Source.StartLine != 2 {
		t.Errorf("expected error on line 2, got %d", pe.Source.StartLine)
	}
}

func TestParse_DiagnosticInterface(t *testing.T) {
	data := []byte("module notapath\n")
	_, err := mod.Parse(testFile, data)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var pe mod.ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("expected ParseError, got %T", err)
	}
	tmpl := pe.EventTemplate()
	if tmpl.ID != "mod.ParseError" {
		t.Errorf("EventTemplate().ID = %q, want %q", tmpl.ID, "mod.ParseError")
	}
	if tmpl.Source == nil {
		t.Error("EventTemplate().Source is nil")
	}
	if tmpl.Hint == "" {
		t.Error("EventTemplate().Hint is empty")
	}
}

func TestDepSpan(t *testing.T) {
	data := []byte("module codeberg.org/pskry/skrynet\n\nrequire (\n    codeberg.org/scampi-modules/npm v1.0.0\n)\n")
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

func TestIsModulePath(t *testing.T) {
	valid := []string{
		"codeberg.org/pskry/skrynet",
		"github.com/foo/bar",
		"codeberg.org/scampi-modules/npm",
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
