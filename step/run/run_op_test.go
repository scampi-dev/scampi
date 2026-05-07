// SPDX-License-Identifier: GPL-3.0-only

package run

import "testing"

func TestEnvPrefix_Empty(t *testing.T) {
	if got := envPrefix(nil); got != "" {
		t.Errorf("nil env → %q, want empty", got)
	}
	if got := envPrefix(map[string]string{}); got != "" {
		t.Errorf("empty map → %q, want empty", got)
	}
}

func TestEnvPrefix_Single(t *testing.T) {
	// ShellQuote always single-quotes for safety; even shell-safe
	// values come out wrapped. That's intentional — the prefix is
	// machine-generated, not for human readability.
	got := envPrefix(map[string]string{"FOO": "bar"})
	want := "FOO='bar' "
	if got != want {
		t.Errorf("envPrefix = %q, want %q", got, want)
	}
}

func TestEnvPrefix_DeterministicOrdering(t *testing.T) {
	// Sorted keys → stable output across runs. Important for
	// debuggability, diffability, and the renderer (line equality
	// matters for live updates).
	got := envPrefix(map[string]string{
		"ZED":    "z",
		"ALPHA":  "a",
		"MIDDLE": "m",
	})
	want := "ALPHA='a' MIDDLE='m' ZED='z' "
	if got != want {
		t.Errorf("envPrefix = %q, want %q", got, want)
	}
}

func TestEnvPrefix_QuotesUnsafeValues(t *testing.T) {
	got := envPrefix(map[string]string{
		"WITH_SPACE":  "hello world",
		"WITH_QUOTE":  "it's quoted",
		"WITH_DOLLAR": "$HOME/etc",
	})
	// ShellQuote uses single-quotes; embedded singles get escaped
	// as '\''. Values with $ stay literal (no shell expansion).
	for _, expect := range []string{
		`WITH_DOLLAR='$HOME/etc' `,
		`WITH_QUOTE='it'\''s quoted' `,
		`WITH_SPACE='hello world' `,
	} {
		if !contains(got, expect) {
			t.Errorf("envPrefix = %q\nmissing: %q", got, expect)
		}
	}
}

func TestRunOp_WithEnv_Prepends(t *testing.T) {
	op := &runOp{env: map[string]string{"FOO": "bar"}}
	got := op.withEnv("echo hi")
	want := "FOO='bar' echo hi"
	if got != want {
		t.Errorf("withEnv = %q, want %q", got, want)
	}
}

func TestRunOp_WithEnv_PassthroughWhenEmpty(t *testing.T) {
	op := &runOp{}
	if got := op.withEnv("echo hi"); got != "echo hi" {
		t.Errorf("empty env mutated cmd: %q", got)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
