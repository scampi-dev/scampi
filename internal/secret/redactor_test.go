// SPDX-License-Identifier: GPL-3.0-only

package secret

import "testing"

func TestRedactor_RedactsKnownValue(t *testing.T) {
	r := NewRedactor()
	r.Add("test-fixture-secret-not-real")

	got := r.Redact("samba-tool ... --adminpass='test-fixture-secret-not-real'")
	want := "samba-tool ... --adminpass='" + DefaultMask + "'"
	if got != want {
		t.Errorf("Redact = %q, want %q", got, want)
	}
}

func TestRedactor_RedactsMultipleOccurrences(t *testing.T) {
	r := NewRedactor()
	r.Add("hunter2")

	got := r.Redact("first: hunter2 / second: hunter2 / third: hunter2")
	for i, ch := range got {
		_ = i
		_ = ch
	}
	if !contains(got, DefaultMask) || contains(got, "hunter2") {
		t.Errorf("Redact = %q, expected all hunter2 replaced", got)
	}
}

func TestRedactor_RedactsMultipleSecrets(t *testing.T) {
	r := NewRedactor()
	r.Add("admin-password-1234")
	r.Add("api-key-abcdefgh")

	got := r.Redact("user=admin pass=admin-password-1234 token=api-key-abcdefgh")
	if contains(got, "admin-password-1234") || contains(got, "api-key-abcdefgh") {
		t.Errorf("Redact failed to remove both: %q", got)
	}
}

func TestRedactor_IgnoresShortSecrets(t *testing.T) {
	r := NewRedactor()
	// 3-char secrets are too short — substring redaction would
	// false-positive on legitimate text containing the same chars.
	// The threshold is conservative; users with truly short secrets
	// have a worse problem than redaction failing.
	r.Add("abc")

	got := r.Redact("abcdef should not be redacted")
	if !contains(got, "abcdef") {
		t.Errorf("Redact dropped legitimate text containing short secret: %q", got)
	}
}

func TestRedactor_IgnoresEmptyValues(t *testing.T) {
	r := NewRedactor()
	r.Add("")
	if got := r.Redact("anything"); got != "anything" {
		t.Errorf("Redact mangled output for empty-secret input: %q", got)
	}
}

func TestRedactor_HandlesNilReceiver(t *testing.T) {
	// A nil redactor is a no-op — useful for code paths that may not
	// have wiring in place yet (LSP, tests).
	var r *Redactor
	if got := r.Redact("plain text"); got != "plain text" {
		t.Errorf("nil redactor changed output: %q", got)
	}
	r.Add("ignored") // must not panic
}

func TestRedactor_DedupsRepeatedAdds(t *testing.T) {
	r := NewRedactor()
	r.Add("password1234")
	r.Add("password1234")
	r.Add("password1234")
	if n := r.Size(); n != 1 {
		t.Errorf("Size = %d after duplicate Adds, want 1", n)
	}
}

func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
