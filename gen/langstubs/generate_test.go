// SPDX-License-Identifier: GPL-3.0-only

package langstubs

import (
	"bytes"
	"strings"
	"testing"
)

// Test config structs (no scampi imports)
// -----------------------------------------------------------------------------

type testPkgConfig struct {
	_        struct{} `summary:"Ensure packages are present or absent"`
	Packages []string `step:"Packages to manage"`
	State    string   `step:"Desired state" default:"present"`
	Desc     string   `step:"Description" optional:"true"`
}

type testCopyConfig struct {
	_      struct{} `summary:"Copy files with owner and permission management"`
	Src    string   `step:"Source"`
	Dest   string   `step:"Destination file path"`
	Perm   string   `step:"File permissions"`
	Owner  string   `step:"Owner user name or UID"`
	Group  string   `step:"Group name or GID"`
	Verify string   `step:"Validation command" optional:"true"`
}

type testSSHConfig struct {
	_    struct{} `summary:"SSH target"`
	Name string   `step:"Target name"`
	Host string   `step:"Hostname or IP"`
	User string   `step:"SSH user"`
	Port int      `step:"SSH port" default:"22"`
}

// Tests
// -----------------------------------------------------------------------------

func TestGenerateBasic(t *testing.T) {
	var buf bytes.Buffer
	err := Generate("test", []StubInput{
		{
			Kind:       "pkg",
			Config:     &testPkgConfig{},
			OutputType: "Step",
			Enums: map[string][]string{
				"State": {"present", "absent", "latest"},
			},
		},
	}, Options{}, &buf)
	if err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	assertContains(t, out, "enum PkgState { present, absent, latest }")
	assertContains(t, out, "decl pkg(")
	assertContains(t, out, "packages:")
	assertContains(t, out, "list[string]")
	assertContains(t, out, "PkgState = PkgState.present")
	assertContains(t, out, ") Step")
}

func TestGenerateNoEnums(t *testing.T) {
	var buf bytes.Buffer
	err := Generate("test", []StubInput{
		{
			Kind:       "copy",
			Config:     &testCopyConfig{},
			OutputType: "Step",
		},
	}, Options{}, &buf)
	if err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	assertContains(t, out, "decl copy(")
	assertContains(t, out, "src:")
	assertContains(t, out, "verify:")
	assertContains(t, out, "string?")
	assertContains(t, out, ") Step")
	assertNotContains(t, out, "enum")
}

func TestGenerateTargetOutputType(t *testing.T) {
	var buf bytes.Buffer
	err := Generate("test", []StubInput{
		{
			Kind:       "ssh",
			Config:     &testSSHConfig{},
			OutputType: "Target",
		},
	}, Options{}, &buf)
	if err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	assertContains(t, out, ") Target")
	assertContains(t, out, "port:")
	assertContains(t, out, "int = 22")
}

func TestGenerateSummaryComment(t *testing.T) {
	var buf bytes.Buffer
	err := Generate("test", []StubInput{
		{Kind: "pkg", Config: &testPkgConfig{}, OutputType: "Step"},
	}, Options{}, &buf)
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, buf.String(), "# Ensure packages are present or absent")
}

func TestGenerateMultipleSteps(t *testing.T) {
	var buf bytes.Buffer
	err := Generate("test", []StubInput{
		{Kind: "pkg", Config: &testPkgConfig{}, OutputType: "Step",
			Enums: map[string][]string{"State": {"present", "absent"}}},
		{Kind: "copy", Config: &testCopyConfig{}, OutputType: "Step"},
		{Kind: "ssh", Config: &testSSHConfig{}, OutputType: "Target"},
	}, Options{}, &buf)
	if err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	assertContains(t, out, "decl pkg(")
	assertContains(t, out, "decl copy(")
	assertContains(t, out, "decl ssh(")
}

func TestGenerateImplicitFields(t *testing.T) {
	var buf bytes.Buffer
	err := Generate("test", []StubInput{
		{Kind: "pkg", Config: &testPkgConfig{}, OutputType: "Step"},
	}, Options{}, &buf)
	if err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	assertContains(t, out, "desc:")
	assertContains(t, out, "on_change:")
}

func TestToSnake(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Packages", "packages"},
		{"State", "state"},
		{"KeyURL", "key_url"},
		{"BaseURL", "base_url"},
		{"GID", "gid"},
		{"Src", "src"},
	}
	for _, tc := range cases {
		got := toSnake(tc.in)
		if got != tc.want {
			t.Errorf("toSnake(%q): got %q, want %q", tc.in, got, tc.want)
		}
	}
}

// Helpers
// -----------------------------------------------------------------------------

func assertContains(t *testing.T, s, sub string) {
	t.Helper()
	if !strings.Contains(s, sub) {
		t.Errorf("output missing %q\ngot:\n%s", sub, s)
	}
}

func assertNotContains(t *testing.T, s, sub string) {
	t.Helper()
	if strings.Contains(s, sub) {
		t.Errorf("output should not contain %q\ngot:\n%s", sub, s)
	}
}
