// SPDX-License-Identifier: GPL-3.0-only

package testkit

import (
	"regexp"

	"scampi.dev/scampi/lang/eval"
)

// Slot identifies the kind of value a matcher is being applied to.
// Used for both slot/matcher compatibility validation and for
// rendering the observed value in mismatch diagnostics.
type Slot uint8

const (
	// SlotFileContent is a file's content. Observed: string.
	SlotFileContent Slot = iota
	// SlotPackageStatus is a package's installed state.
	// Observed: PackagePresence (present/absent).
	SlotPackageStatus
	// SlotServiceStatus is a service's runtime state.
	// Observed: ServiceObserved (running/stopped).
	SlotServiceStatus
	// SlotDirPresence is a directory's existence. Observed: bool.
	SlotDirPresence
	// SlotSymlinkTarget is a symlink's target path.
	// Observed: string (target) or "" if absent.
	SlotSymlinkTarget
	// SlotRequestBody is the body of a recorded HTTP request.
	// Observed: []byte.
	SlotRequestBody
)

// PackagePresence is the observed installed state of a package on
// a mock target. The verifier translates the matcher's expected
// posix.PkgState into a presence check.
type PackagePresence uint8

const (
	PackageAbsent PackagePresence = iota
	PackagePresent
)

// ServiceObserved is the observed state of a service on a mock
// target. Same idea as PackagePresence.
type ServiceObserved uint8

const (
	ServiceStopped ServiceObserved = iota
	ServiceRunning
)

// Mismatch describes a single matcher failure.
type Mismatch struct {
	Slot    Slot   // the slot the failure occurred in
	Key     string // file path / package name / service name / ...
	Matcher string // matcher kind (TypeName from the eval StructVal)
	Reason  string // human-readable reason ("expected X, got Y" etc.)
}

// Match runs a matcher against an observed value in a given slot.
// Returns nil on success, a *Mismatch describing the failure
// otherwise.
//
// matcher is the eval-time StructVal produced by a `matchers.*`
// constructor — TypeName carries the matcher kind, Fields carry the
// constructor's kwargs.
func Match(matcher *eval.StructVal, slot Slot, key string, observed any) *Mismatch {
	if matcher == nil {
		return &Mismatch{
			Slot: slot, Key: key, Matcher: "<nil>",
			Reason: "matcher is nil",
		}
	}
	if matcher.RetType != "Matcher" {
		return &Mismatch{
			Slot: slot, Key: key, Matcher: matcher.TypeName,
			Reason: "value is not a matcher (RetType=" + matcher.RetType + ")",
		}
	}

	mk := matcher.TypeName
	switch mk {
	case "is_present":
		return matchPresence(slot, key, mk, observed, true)
	case "is_absent":
		return matchPresence(slot, key, mk, observed, false)

	case "has_exact_content", "has_substring", "matches_regex", "is_empty":
		return matchString(matcher, slot, key, observed)

	case "has_svc_status":
		return matchSvcStatus(matcher, slot, key, observed)
	case "has_pkg_status":
		return matchPkgStatus(matcher, slot, key, observed)
	}

	return &Mismatch{
		Slot: slot, Key: key, Matcher: mk,
		Reason: "unknown matcher kind",
	}
}

// matchPresence handles is_present / is_absent against any slot.
// "Present" means the observed value indicates existence:
//
//	SlotFileContent     → string is non-nil (any content, including "")
//	SlotPackageStatus   → PackagePresent
//	SlotServiceStatus   → any service entry exists
//	SlotDirPresence     → true
//	SlotSymlinkTarget   → non-empty target string
//	SlotRequestBody     → not meaningful, returns mismatch
func matchPresence(slot Slot, key, mk string, observed any, wantPresent bool) *Mismatch {
	present := false
	switch slot {
	case SlotFileContent:
		_, present = observed.(string)
	case SlotPackageStatus:
		if p, ok := observed.(PackagePresence); ok && p == PackagePresent {
			present = true
		}
	case SlotServiceStatus:
		// Service slot uses ServiceObserved; "present" means there's any
		// observed state at all (running or stopped). Absence is signaled
		// by passing nil.
		if observed != nil {
			present = true
		}
	case SlotDirPresence:
		if b, ok := observed.(bool); ok {
			present = b
		}
	case SlotSymlinkTarget:
		if s, ok := observed.(string); ok && s != "" {
			present = true
		}
	case SlotRequestBody:
		return &Mismatch{
			Slot: slot, Key: key, Matcher: mk,
			Reason: "presence matchers don't apply to request bodies",
		}
	}
	if present == wantPresent {
		return nil
	}
	want := "present"
	if !wantPresent {
		want = "absent"
	}
	got := "absent"
	if present {
		got = "present"
	}
	return &Mismatch{
		Slot: slot, Key: key, Matcher: mk,
		Reason: "expected " + want + ", got " + got,
	}
}

// matchString handles the string-content matchers
// (has_exact_content, has_substring, matches_regex, is_empty).
// Valid only in string-content slots.
func matchString(matcher *eval.StructVal, slot Slot, key string, observed any) *Mismatch {
	mk := matcher.TypeName
	if !slotAcceptsString(slot) {
		return &Mismatch{
			Slot: slot, Key: key, Matcher: mk,
			Reason: mk + " requires a string-content slot (file content, request body)",
		}
	}
	got, ok := stringFromObserved(observed)
	if !ok {
		return &Mismatch{
			Slot: slot, Key: key, Matcher: mk,
			Reason: "slot has no observed value",
		}
	}

	switch mk {
	case "has_exact_content":
		want := stringField(matcher, "content")
		if got != want {
			return &Mismatch{
				Slot: slot, Key: key, Matcher: mk,
				Reason: "exact content mismatch:\n  want: " + quote(want) + "\n  got:  " + quote(got),
			}
		}
	case "has_substring":
		sub := stringField(matcher, "substring")
		if !containsString(got, sub) {
			return &Mismatch{
				Slot: slot, Key: key, Matcher: mk,
				Reason: "missing substring " + quote(sub),
			}
		}
	case "matches_regex":
		pat := stringField(matcher, "pattern")
		re, err := regexp.Compile(pat)
		if err != nil {
			return &Mismatch{
				Slot: slot, Key: key, Matcher: mk,
				Reason: "invalid regex " + quote(pat) + ": " + err.Error(),
			}
		}
		if !re.MatchString(got) {
			return &Mismatch{
				Slot: slot, Key: key, Matcher: mk,
				Reason: "did not match regex " + quote(pat),
			}
		}
	case "is_empty":
		if got != "" {
			return &Mismatch{
				Slot: slot, Key: key, Matcher: mk,
				Reason: "expected empty, got " + quote(got),
			}
		}
	}
	return nil
}

// matchSvcStatus handles has_svc_status against the service slot.
// The matcher carries the expected status as a StringVal of the
// posix.ServiceState variant name (running, stopped, ...).
func matchSvcStatus(matcher *eval.StructVal, slot Slot, key string, observed any) *Mismatch {
	mk := matcher.TypeName
	if slot != SlotServiceStatus {
		return &Mismatch{
			Slot: slot, Key: key, Matcher: mk,
			Reason: "has_svc_status only applies to services",
		}
	}
	want := stringField(matcher, "status")
	obs, ok := observed.(ServiceObserved)
	if !ok {
		return &Mismatch{
			Slot: slot, Key: key, Matcher: mk,
			Reason: "service has no observed state",
		}
	}
	got := "stopped"
	if obs == ServiceRunning {
		got = "running"
	}
	// Reduce the language-level enum vocabulary down to the runtime
	// observation set (running / stopped). The other ServiceState
	// variants (restarted, reloaded) are transient — they only make
	// sense as desired states, not observed ones.
	wantNorm := want
	switch want {
	case "restarted", "reloaded":
		wantNorm = "running"
	}
	if got != wantNorm {
		return &Mismatch{
			Slot: slot, Key: key, Matcher: mk,
			Reason: "expected service " + want + ", got " + got,
		}
	}
	return nil
}

// matchPkgStatus handles has_pkg_status against the package slot.
// The matcher carries the expected status as a StringVal of the
// posix.PkgState variant name (present, absent, latest).
func matchPkgStatus(matcher *eval.StructVal, slot Slot, key string, observed any) *Mismatch {
	mk := matcher.TypeName
	if slot != SlotPackageStatus {
		return &Mismatch{
			Slot: slot, Key: key, Matcher: mk,
			Reason: "has_pkg_status only applies to packages",
		}
	}
	want := stringField(matcher, "status")
	obs, ok := observed.(PackagePresence)
	if !ok {
		return &Mismatch{
			Slot: slot, Key: key, Matcher: mk,
			Reason: "package has no observed state",
		}
	}
	got := "absent"
	if obs == PackagePresent {
		got = "present"
	}
	// `latest` is a desired state that the runtime can't distinguish
	// from `present` after the fact — both result in "package is
	// installed". Normalize for comparison.
	wantNorm := want
	if want == "latest" {
		wantNorm = "present"
	}
	if got != wantNorm {
		return &Mismatch{
			Slot: slot, Key: key, Matcher: mk,
			Reason: "expected package " + want + ", got " + got,
		}
	}
	return nil
}

// Helpers
// -----------------------------------------------------------------------------

func slotAcceptsString(slot Slot) bool {
	return slot == SlotFileContent ||
		slot == SlotRequestBody ||
		slot == SlotSymlinkTarget
}

func stringFromObserved(observed any) (string, bool) {
	switch v := observed.(type) {
	case string:
		return v, true
	case []byte:
		return string(v), true
	}
	return "", false
}

func stringField(sv *eval.StructVal, name string) string {
	if sv.Fields == nil {
		return ""
	}
	if v, ok := sv.Fields[name].(*eval.StringVal); ok {
		return v.V
	}
	return ""
}

func containsString(s, sub string) bool {
	if sub == "" {
		return true
	}
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func quote(s string) string {
	return "\"" + s + "\""
}
