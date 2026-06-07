// SPDX-License-Identifier: GPL-3.0-only

package main

import (
	"net"
	"os"
	"slices"
	"testing"

	"github.com/urfave/cli/v3"

	"scampi.dev/scampi/internal/render"
)

func TestExpandShorthand_UniquePrefixExpands(t *testing.T) {
	cmds := []*cli.Command{{Name: "reconcile"}, {Name: "run"}, {Name: "plan"}}
	got := expandShorthand([]string{"scampi", "reco", "dir"}, cmds)
	want := []string{"scampi", "reconcile", "dir"}
	if !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestExpandShorthand_AmbiguousFallsThrough(t *testing.T) {
	cmds := []*cli.Command{{Name: "reconcile"}, {Name: "run"}, {Name: "plan"}}
	args := []string{"scampi", "r", "dir"} // matches reconcile + run
	got := expandShorthand(args, cmds)
	if !slices.Equal(got, args) {
		t.Errorf("expected unchanged on ambiguous; got %v", got)
	}
}

func TestExpandShorthand_ExactMatchUnchanged(t *testing.T) {
	cmds := []*cli.Command{{Name: "reconcile"}, {Name: "run"}}
	args := []string{"scampi", "run", "dir"}
	got := expandShorthand(args, cmds)
	if !slices.Equal(got, args) {
		t.Errorf("expected unchanged on exact match; got %v", got)
	}
}

func TestExpandShorthand_NoMatchUnchanged(t *testing.T) {
	cmds := []*cli.Command{{Name: "reconcile"}}
	args := []string{"scampi", "xyz", "dir"}
	got := expandShorthand(args, cmds)
	if !slices.Equal(got, args) {
		t.Errorf("expected unchanged on no match; got %v", got)
	}
}

func TestExpandShorthand_SkipsLeadingFlags(t *testing.T) {
	cmds := []*cli.Command{{Name: "reconcile"}}
	args := []string{"scampi", "--color=always", "reco", "dir"}
	got := expandShorthand(args, cmds)
	want := []string{"scampi", "--color=always", "reconcile", "dir"}
	if !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestParseJoinSeeds_CommaListWithWhitespace(t *testing.T) {
	got := parseJoinSeeds("10.0.0.5:7946, 10.0.0.6:7946 ,10.0.0.7:7946")
	want := []string{"10.0.0.5:7946", "10.0.0.6:7946", "10.0.0.7:7946"}
	if !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestParseJoinSeeds_EmptyReturnsNil(t *testing.T) {
	if got := parseJoinSeeds(""); got != nil {
		t.Errorf("got %v, want nil", got)
	}
}

func TestDefaultMeshName_HostnameWhenEmpty(t *testing.T) {
	want, _ := os.Hostname()
	if got := defaultMeshName("", defaultInstancePort); got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestDefaultMeshName_PortSuffixOnNonDefaultPort(t *testing.T) {
	host, _ := os.Hostname()
	port := defaultInstancePort + 1
	want := host + "-" + itoa(port)
	if got := defaultMeshName("", port); got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestDefaultMeshName_ExplicitWins(t *testing.T) {
	if got := defaultMeshName("custom", defaultInstancePort+1); got != "custom" {
		t.Errorf("got %s, want custom", got)
	}
}

func TestDecideColor_AlwaysReturnsTrue(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	if !decideColor("always", os.Stdout) {
		t.Error("always should win over NO_COLOR")
	}
}

func TestDecideColor_NeverReturnsFalse(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	if decideColor("never", os.Stdout) {
		t.Error("never should return false")
	}
}

func TestDecideColor_NoColorOverridesAuto(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	if decideColor("auto", os.Stdout) {
		t.Error("NO_COLOR should defeat auto")
	}
}

func TestDecideGlyphs_Ascii(t *testing.T) {
	if got := decideGlyphs(true); got != render.ASCIIGlyphs {
		t.Errorf("got %v, want ASCIIGlyphs", got)
	}
}

func TestDecideGlyphs_Unicode(t *testing.T) {
	if got := decideGlyphs(false); got != render.UnicodeGlyphs {
		t.Errorf("got %v, want UnicodeGlyphs", got)
	}
}

func TestProbeStale_FreePortReportsStale(t *testing.T) {
	port := pickFreePort(t)
	addr := net.JoinHostPort("127.0.0.1", itoa(port))
	if !probeStale(addr) {
		t.Error("free port should report stale=true")
	}
}

func TestProbeStale_BusyPortReportsLive(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close() })
	if probeStale(l.Addr().String()) {
		t.Error("busy port should report stale=false")
	}
}

func TestAcquireInstanceListener_CollidesOnSecondCall(t *testing.T) {
	port := pickFreePort(t)
	addr := net.JoinHostPort("127.0.0.1", itoa(port))
	l1, err := acquireInstanceListener(addr)
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	t.Cleanup(func() { _ = l1.Close() })

	if _, err := acquireInstanceListener(addr); err == nil {
		t.Error("second acquire on busy port should error")
	}
}

func pickFreePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	p := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()
	return p
}

func itoa(i int) string {
	const digits = "0123456789"
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var b [20]byte
	n := len(b)
	for i > 0 {
		n--
		b[n] = digits[i%10]
		i /= 10
	}
	if neg {
		n--
		b[n] = '-'
	}
	return string(b[n:])
}
