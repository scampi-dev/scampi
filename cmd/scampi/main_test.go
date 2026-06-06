// SPDX-License-Identifier: GPL-3.0-only

package main

import (
	"net"
	"os"
	"slices"
	"testing"

	"scampi.dev/scampi/internal/render"
)

func TestEnvOr_EnvWins(t *testing.T) {
	t.Setenv("SCAMPI_TEST_KEY", "from-env")
	if got := envOr("SCAMPI_TEST_KEY", "fallback"); got != "from-env" {
		t.Errorf("got %s, want from-env", got)
	}
}

func TestEnvOr_FallbackWhenUnset(t *testing.T) {
	t.Setenv("SCAMPI_TEST_KEY", "")
	if got := envOr("SCAMPI_TEST_KEY", "fallback"); got != "fallback" {
		t.Errorf("got %s, want fallback", got)
	}
}

func TestEnvIntOr_ParsesEnv(t *testing.T) {
	t.Setenv("SCAMPI_TEST_KEY", "12345")
	if got := envIntOr("SCAMPI_TEST_KEY", 99); got != 12345 {
		t.Errorf("got %d, want 12345", got)
	}
}

func TestEnvIntOr_FallbackOnInvalid(t *testing.T) {
	t.Setenv("SCAMPI_TEST_KEY", "not-a-number")
	if got := envIntOr("SCAMPI_TEST_KEY", 42); got != 42 {
		t.Errorf("got %d, want 42", got)
	}
}

func TestEnvBool_True(t *testing.T) {
	t.Setenv("SCAMPI_TEST_KEY", "1")
	if got := envBool("SCAMPI_TEST_KEY", false); !got {
		t.Errorf("got false, want true")
	}
}

func TestEnvBool_FallbackOnInvalid(t *testing.T) {
	t.Setenv("SCAMPI_TEST_KEY", "yes-please")
	if got := envBool("SCAMPI_TEST_KEY", true); !got {
		t.Errorf("got false, want true (fallback)")
	}
}

func TestParseJoinSeeds_CommaListWithWhitespace(t *testing.T) {
	want := []string{"10.0.0.5:7946", "10.0.0.6:7946", "10.0.0.7:7946"}
	got := parseJoinSeeds("10.0.0.5:7946, 10.0.0.6:7946 ,10.0.0.7:7946")
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
	savePort, saveName := instancePort, meshName
	t.Cleanup(func() { instancePort, meshName = savePort, saveName })
	meshName = ""
	instancePort = defaultInstancePort
	want, _ := os.Hostname()
	if got := defaultMeshName(); got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestDefaultMeshName_PortSuffixOnNonDefaultPort(t *testing.T) {
	savePort, saveName := instancePort, meshName
	t.Cleanup(func() { instancePort, meshName = savePort, saveName })
	meshName = ""
	instancePort = defaultInstancePort + 1
	host, _ := os.Hostname()
	want := host + "-" + itoa(defaultInstancePort+1)
	if got := defaultMeshName(); got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestAcquireInstanceListener_CollidesOnSecondCall(t *testing.T) {
	saveBind, savePort := meshBind, instancePort
	t.Cleanup(func() { meshBind, instancePort = saveBind, savePort })
	meshBind = "127.0.0.1"
	instancePort = pickFreePort(t)

	l1, err := acquireInstanceListener()
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	t.Cleanup(func() { _ = l1.Close() })

	if _, err := acquireInstanceListener(); err == nil {
		t.Error("second acquire on busy port should error")
	}
}

func TestDecideColor_AlwaysReturnsTrue(t *testing.T) {
	save := colorMode
	t.Cleanup(func() { colorMode = save })
	t.Setenv("NO_COLOR", "1")
	colorMode = "always"
	if !decideColor(os.Stdout) {
		t.Error("colorMode=always should win over NO_COLOR")
	}
}

func TestDecideColor_NeverReturnsFalse(t *testing.T) {
	save := colorMode
	t.Cleanup(func() { colorMode = save })
	t.Setenv("NO_COLOR", "")
	colorMode = "never"
	if decideColor(os.Stdout) {
		t.Error("colorMode=never should return false")
	}
}

func TestDecideColor_NoColorOverridesAuto(t *testing.T) {
	save := colorMode
	t.Cleanup(func() { colorMode = save })
	t.Setenv("NO_COLOR", "1")
	colorMode = "auto"
	if decideColor(os.Stdout) {
		t.Error("NO_COLOR should defeat auto")
	}
}

func TestDecideGlyphs_AsciiFlag(t *testing.T) {
	save := asciiFlag
	t.Cleanup(func() { asciiFlag = save })
	asciiFlag = true
	if got := decideGlyphs(); got != render.ASCIIGlyphs {
		t.Errorf("got %v, want ASCIIGlyphs", got)
	}
}

func TestDecideGlyphs_Unicode(t *testing.T) {
	save := asciiFlag
	t.Cleanup(func() { asciiFlag = save })
	asciiFlag = false
	if got := decideGlyphs(); got != render.UnicodeGlyphs {
		t.Errorf("got %v, want UnicodeGlyphs", got)
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

// itoa is a thin wrapper that avoids importing strconv just for tests.
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
