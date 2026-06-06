// SPDX-License-Identifier: GPL-3.0-only

package main

import (
	"net"
	"os"
	"slices"
	"strconv"
	"testing"
)

func saveFlagVars(t *testing.T) {
	t.Helper()
	origPort := instancePort
	origBind := meshBind
	origAdv := meshAdvertise
	origName := meshName
	origJoin := joinSeeds
	t.Cleanup(func() {
		instancePort = origPort
		meshBind = origBind
		meshAdvertise = origAdv
		meshName = origName
		joinSeeds = origJoin
	})
}

func TestResolveInstancePort_EnvWinsOverFlag(t *testing.T) {
	saveFlagVars(t)
	t.Setenv("SCAMPI_INSTANCE_PORT", "12345")
	instancePort = 99999
	if got := resolveInstancePort(); got != 12345 {
		t.Errorf("got %d, want 12345", got)
	}
}

func TestResolveInstancePort_FlagFallback(t *testing.T) {
	saveFlagVars(t)
	t.Setenv("SCAMPI_INSTANCE_PORT", "")
	instancePort = 7777
	if got := resolveInstancePort(); got != 7777 {
		t.Errorf("got %d, want 7777", got)
	}
}

func TestResolveInstancePort_InvalidEnvFallsBack(t *testing.T) {
	saveFlagVars(t)
	t.Setenv("SCAMPI_INSTANCE_PORT", "not-a-number")
	instancePort = 4242
	if got := resolveInstancePort(); got != 4242 {
		t.Errorf("got %d, want 4242", got)
	}
}

func TestResolveMeshBind_EnvWinsOverFlag(t *testing.T) {
	saveFlagVars(t)
	t.Setenv("SCAMPI_MESH_BIND", "10.0.0.5")
	meshBind = "127.0.0.1"
	if got := resolveMeshBind(); got != "10.0.0.5" {
		t.Errorf("got %s, want 10.0.0.5", got)
	}
}

func TestResolveMeshBind_FlagFallback(t *testing.T) {
	saveFlagVars(t)
	t.Setenv("SCAMPI_MESH_BIND", "")
	meshBind = "0.0.0.0"
	if got := resolveMeshBind(); got != "0.0.0.0" {
		t.Errorf("got %s, want 0.0.0.0", got)
	}
}

func TestResolveMeshAdvertise_FlagFallback(t *testing.T) {
	saveFlagVars(t)
	t.Setenv("SCAMPI_MESH_ADVERTISE", "")
	meshAdvertise = ""
	if got := resolveMeshAdvertise(); got != "" {
		t.Errorf("got %s, want empty", got)
	}
}

func TestResolveMeshName_HostnameWhenEmpty(t *testing.T) {
	saveFlagVars(t)
	t.Setenv("SCAMPI_MESH_NAME", "")
	t.Setenv("SCAMPI_INSTANCE_PORT", "")
	meshName = ""
	instancePort = defaultInstancePort
	want, _ := os.Hostname()
	if got := resolveMeshName(); got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestResolveMeshName_PortSuffixOnNonDefaultPort(t *testing.T) {
	saveFlagVars(t)
	t.Setenv("SCAMPI_MESH_NAME", "")
	t.Setenv("SCAMPI_INSTANCE_PORT", "")
	meshName = ""
	instancePort = defaultInstancePort + 1
	host, _ := os.Hostname()
	want := host + "-" + strconv.Itoa(defaultInstancePort+1)
	if got := resolveMeshName(); got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestResolveMeshName_EnvWinsOverFlagAndHostname(t *testing.T) {
	saveFlagVars(t)
	t.Setenv("SCAMPI_MESH_NAME", "peer-from-env")
	meshName = "peer-from-flag"
	if got := resolveMeshName(); got != "peer-from-env" {
		t.Errorf("got %s, want peer-from-env", got)
	}
}

func TestResolveJoinSeeds_ParsesCommaListWithWhitespace(t *testing.T) {
	saveFlagVars(t)
	t.Setenv("SCAMPI_MESH_JOIN", "")
	joinSeeds = "10.0.0.5:7946, 10.0.0.6:7946 ,10.0.0.7:7946"
	want := []string{"10.0.0.5:7946", "10.0.0.6:7946", "10.0.0.7:7946"}
	if got := resolveJoinSeeds(); !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestResolveJoinSeeds_EmptyWhenUnset(t *testing.T) {
	saveFlagVars(t)
	t.Setenv("SCAMPI_MESH_JOIN", "")
	joinSeeds = ""
	if got := resolveJoinSeeds(); got != nil {
		t.Errorf("got %v, want nil", got)
	}
}

func TestInstanceAddr_FormatsBindAndPort(t *testing.T) {
	saveFlagVars(t)
	t.Setenv("SCAMPI_MESH_BIND", "")
	t.Setenv("SCAMPI_INSTANCE_PORT", "")
	meshBind = "10.0.0.5"
	instancePort = 9999
	if got := instanceAddr(); got != "10.0.0.5:9999" {
		t.Errorf("got %s, want 10.0.0.5:9999", got)
	}
}

func TestAcquireInstanceListener_CollidesOnSecondCall(t *testing.T) {
	saveFlagVars(t)
	t.Setenv("SCAMPI_MESH_BIND", "")
	t.Setenv("SCAMPI_INSTANCE_PORT", "")
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

func TestAcquireInstanceListener_SucceedsAfterRelease(t *testing.T) {
	saveFlagVars(t)
	t.Setenv("SCAMPI_MESH_BIND", "")
	t.Setenv("SCAMPI_INSTANCE_PORT", "")
	meshBind = "127.0.0.1"
	instancePort = pickFreePort(t)

	l1, err := acquireInstanceListener()
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	_ = l1.Close()

	l2, err := acquireInstanceListener()
	if err != nil {
		t.Errorf("second acquire after release: %v", err)
	}
	if l2 != nil {
		_ = l2.Close()
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
