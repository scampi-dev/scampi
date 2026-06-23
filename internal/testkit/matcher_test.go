// SPDX-License-Identifier: GPL-3.0-only

package testkit

import (
	"strings"
	"testing"

	"scampi.dev/scampi/internal/lang/eval"
)

// matcher constructs a fake StructVal representing a matcher call.
func matcher(name string, fields map[string]string) *eval.StructVal {
	sv := &eval.StructVal{
		TypeName: name,
		QualName: "matchers." + name,
		RetType:  "Matcher",
		Fields:   make(map[string]eval.Value, len(fields)),
	}
	for k, v := range fields {
		sv.Fields[k] = &eval.StringVal{V: v}
	}
	return sv
}

func TestMatch_StringContent(t *testing.T) {
	cases := []struct {
		name     string
		matcher  *eval.StructVal
		observed any
		wantErr  string // "" = expect success, otherwise substring of Reason
	}{
		{
			name:     "exact match",
			matcher:  matcher("has_exact_content", map[string]string{"content": "hello"}),
			observed: "hello",
		},
		{
			name:     "exact mismatch",
			matcher:  matcher("has_exact_content", map[string]string{"content": "hello"}),
			observed: "world",
			wantErr:  "exact content mismatch",
		},
		{
			name:     "substring present",
			matcher:  matcher("has_substring", map[string]string{"substring": "world"}),
			observed: "hello world",
		},
		{
			name:     "substring missing",
			matcher:  matcher("has_substring", map[string]string{"substring": "missing"}),
			observed: "hello world",
			wantErr:  "missing substring",
		},
		{
			name:     "regex match",
			matcher:  matcher("matches_regex", map[string]string{"pattern": "^hel+o"}),
			observed: "hello",
		},
		{
			name:     "regex no match",
			matcher:  matcher("matches_regex", map[string]string{"pattern": "^bye"}),
			observed: "hello",
			wantErr:  "did not match regex",
		},
		{
			name:     "regex invalid",
			matcher:  matcher("matches_regex", map[string]string{"pattern": "[unclosed"}),
			observed: "x",
			wantErr:  "invalid regex",
		},
		{
			name:     "is_empty success",
			matcher:  matcher("is_empty", nil),
			observed: "",
		},
		{
			name:     "is_empty failure",
			matcher:  matcher("is_empty", nil),
			observed: "non-empty",
			wantErr:  "expected empty",
		},
		{
			name:     "byte slice observed",
			matcher:  matcher("has_substring", map[string]string{"substring": "json"}),
			observed: []byte(`{"kind":"json"}`),
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			m := Match(tt.matcher, SlotFileContent, "/x", tt.observed)
			assertMismatch(t, m, tt.wantErr)
		})
	}
}

func TestMatch_StringMatcher_WrongSlot(t *testing.T) {
	m := Match(
		matcher("has_substring", map[string]string{"substring": "x"}),
		SlotPackageStatus,
		"nginx",
		PackagePresent,
	)
	assertMismatch(t, m, "string-content slot")
}

func TestMatch_Presence(t *testing.T) {
	present := matcher("is_present", nil)
	absent := matcher("is_absent", nil)

	cases := []struct {
		name     string
		matcher  *eval.StructVal
		slot     Slot
		observed any
		wantErr  string
	}{
		{"file present", present, SlotFileContent, "any", ""},
		{"file nil passes is_absent", absent, SlotFileContent, nil, ""},
		{"file value fails is_absent", absent, SlotFileContent, "x", "expected absent"},
		{"package present", present, SlotPackageStatus, PackagePresent, ""},
		{"package absent fails is_present", present, SlotPackageStatus, PackageAbsent, "expected present"},
		{"dir present bool", present, SlotDirPresence, true, ""},
		{"dir absent bool", absent, SlotDirPresence, false, ""},
		{"symlink target present", present, SlotSymlinkTarget, "/opt/foo", ""},
		{"symlink empty fails is_present", present, SlotSymlinkTarget, "", "expected present"},
		{"presence on request body is invalid", present, SlotRequestBody, []byte("x"), "don't apply"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			m := Match(tt.matcher, tt.slot, "k", tt.observed)
			assertMismatch(t, m, tt.wantErr)
		})
	}
}

func TestMatch_SvcStatus(t *testing.T) {
	cases := []struct {
		name     string
		want     string
		observed ServiceObserved
		wantErr  string
	}{
		{"running matches running", "running", ServiceRunning, ""},
		{"stopped matches stopped", "stopped", ServiceStopped, ""},
		{"running but observed stopped", "running", ServiceStopped, "expected service running, got stopped"},
		{"restarted normalizes to running", "restarted", ServiceRunning, ""},
		{"reloaded normalizes to running", "reloaded", ServiceRunning, ""},
		{"restarted fails when stopped", "restarted", ServiceStopped, "expected service restarted, got stopped"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			m := Match(
				matcher("has_svc_status", map[string]string{"status": tt.want}),
				SlotServiceStatus,
				"nginx",
				tt.observed,
			)
			assertMismatch(t, m, tt.wantErr)
		})
	}
}

func TestMatch_SvcStatus_WrongSlot(t *testing.T) {
	m := Match(
		matcher("has_svc_status", map[string]string{"status": "running"}),
		SlotFileContent,
		"/x",
		"hello",
	)
	assertMismatch(t, m, "only applies to services")
}

func TestMatch_PkgStatus(t *testing.T) {
	cases := []struct {
		name     string
		want     string
		observed PackagePresence
		wantErr  string
	}{
		{"present matches present", "present", PackagePresent, ""},
		{"absent matches absent", "absent", PackageAbsent, ""},
		{"present but observed absent", "present", PackageAbsent, "expected package present, got absent"},
		{"latest normalizes to present", "latest", PackagePresent, ""},
		{"latest fails when absent", "latest", PackageAbsent, "expected package latest, got absent"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			m := Match(
				matcher("has_pkg_status", map[string]string{"status": tt.want}),
				SlotPackageStatus,
				"nginx",
				tt.observed,
			)
			assertMismatch(t, m, tt.wantErr)
		})
	}
}

func TestMatch_PkgStatus_WrongSlot(t *testing.T) {
	m := Match(
		matcher("has_pkg_status", map[string]string{"status": "present"}),
		SlotServiceStatus,
		"nginx",
		ServiceRunning,
	)
	assertMismatch(t, m, "only applies to packages")
}

func TestMatch_NilMatcher(t *testing.T) {
	m := Match(nil, SlotFileContent, "/x", "")
	assertMismatch(t, m, "matcher is nil")
}

func TestMatch_WrongRetType(t *testing.T) {
	sv := &eval.StructVal{
		TypeName: "has_substring",
		RetType:  "NotAMatcher",
		Fields:   map[string]eval.Value{"substring": &eval.StringVal{V: "x"}},
	}
	m := Match(sv, SlotFileContent, "/x", "x")
	assertMismatch(t, m, "not a matcher")
}

func TestMatch_UnknownKind(t *testing.T) {
	m := Match(
		matcher("has_quantum_state", nil),
		SlotFileContent,
		"/x",
		"x",
	)
	assertMismatch(t, m, "unknown matcher kind")
}

// assertMismatch checks the result of a Match call against an
// expected substring. wantErr == "" means "expect success" (m == nil).
func assertMismatch(t *testing.T, m *Mismatch, wantErr string) {
	t.Helper()
	if wantErr == "" {
		if m != nil {
			t.Errorf("expected match, got mismatch: %s", m.Reason)
		}
		return
	}
	if m == nil {
		t.Errorf("expected mismatch containing %q, got match", wantErr)
		return
	}
	if !strings.Contains(m.Reason, wantErr) {
		t.Errorf("mismatch reason %q does not contain %q", m.Reason, wantErr)
	}
}
