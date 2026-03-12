// SPDX-License-Identifier: GPL-3.0-only

package svcmgr

import (
	"strings"
	"testing"
)

func TestDetect(t *testing.T) {
	tests := []struct {
		name     string
		cmds     map[string]int // command -> exit code
		wantName string         // "" means nil
	}{
		{
			name:     "systemd",
			cmds:     map[string]int{"command -v systemctl": 0},
			wantName: "systemd",
		},
		{
			name:     "openrc",
			cmds:     map[string]int{"command -v systemctl": 1, "command -v rc-service": 0},
			wantName: "openrc",
		},
		{
			name:     "launchctl",
			cmds:     map[string]int{"command -v systemctl": 1, "command -v rc-service": 1, "command -v launchctl": 0},
			wantName: "launchctl",
		},
		{
			name:     "systemd preferred over openrc",
			cmds:     map[string]int{"command -v systemctl": 0, "command -v rc-service": 0},
			wantName: "systemd",
		},
		{
			name:     "nothing found",
			cmds:     map[string]int{"command -v systemctl": 1, "command -v rc-service": 1, "command -v launchctl": 1},
			wantName: "",
		},
		{
			name:     "all commands error",
			cmds:     map[string]int{},
			wantName: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			run := fakeRunner(tt.cmds)
			got := Detect(run)
			if tt.wantName == "" {
				if got != nil {
					t.Errorf("expected nil, got %q", got.Name())
				}
				return
			}
			if got == nil {
				t.Fatalf("expected %q, got nil", tt.wantName)
			}
			if got.Name() != tt.wantName {
				t.Errorf("got %q, want %q", got.Name(), tt.wantName)
			}
		})
	}
}

func TestTemplateBackendCommands(t *testing.T) {
	tests := []struct {
		backend string
		method  string
		name    string
		want    string
	}{
		{"systemd", "IsActive", "nginx", "systemctl is-active 'nginx'"},
		{"systemd", "IsEnabled", "nginx", "systemctl is-enabled 'nginx'"},
		{"systemd", "Start", "nginx", "systemctl start 'nginx'"},
		{"systemd", "Stop", "nginx", "systemctl stop 'nginx'"},
		{"systemd", "Enable", "nginx", "systemctl enable 'nginx'"},
		{"systemd", "Disable", "nginx", "systemctl disable 'nginx'"},
		{"systemd", "DaemonReload", "", "systemctl daemon-reload"},
		{"systemd", "Restart", "nginx", "systemctl restart 'nginx'"},
		{"systemd", "Reload", "nginx", "systemctl reload 'nginx'"},

		{"openrc", "IsActive", "nginx", "rc-service 'nginx' status"},
		{"openrc", "IsEnabled", "nginx", "rc-update show default | grep -q 'nginx'"},
		{"openrc", "Start", "nginx", "rc-service 'nginx' start"},
		{"openrc", "Stop", "nginx", "rc-service 'nginx' stop"},
		{"openrc", "Enable", "nginx", "rc-update add 'nginx' default"},
		{"openrc", "Disable", "nginx", "rc-update del 'nginx' default"},
		{"openrc", "DaemonReload", "", ""},
		{"openrc", "Restart", "nginx", "rc-service 'nginx' restart"},
		{"openrc", "Reload", "nginx", "rc-service 'nginx' reload"},
	}

	for _, tt := range tests {
		label := tt.backend + "/" + tt.method
		t.Run(label, func(t *testing.T) {
			b := backends[tt.backend]
			if b == nil {
				t.Fatalf("unknown backend %q", tt.backend)
			}
			got := callMethod(b, tt.method, tt.name)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTemplateBackendQuotesSpecialChars(t *testing.T) {
	b := backends["systemd"]
	got := b.CmdStart("it's a test")
	want := "systemctl start 'it'\\''s a test'"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestTemplateBackendProperties(t *testing.T) {
	tests := []struct {
		backend   string
		name      string
		needsRoot bool
	}{
		{"systemd", "systemd", true},
		{"openrc", "openrc", true},
	}
	for _, tt := range tests {
		t.Run(tt.backend, func(t *testing.T) {
			b := backends[tt.backend]
			if b.Name() != tt.name {
				t.Errorf("Name(): got %q, want %q", b.Name(), tt.name)
			}
			if b.NeedsRoot() != tt.needsRoot {
				t.Errorf("NeedsRoot(): got %v, want %v", b.NeedsRoot(), tt.needsRoot)
			}
		})
	}
}

func TestLaunchctlBackendCommands(t *testing.T) {
	b := &launchctlBackend{domain: "system"}

	if b.Name() != "launchctl" {
		t.Errorf("Name(): got %q, want %q", b.Name(), "launchctl")
	}
	if !b.NeedsRoot() {
		t.Error("system domain should NeedsRoot()")
	}
	if b.CmdDaemonReload() != "" {
		t.Errorf("DaemonReload should be empty, got %q", b.CmdDaemonReload())
	}

	// IsActive should use launchctl list with the quoted label.
	isActive := b.CmdIsActive("com.example.svc")
	if !strings.Contains(isActive, "launchctl list") {
		t.Errorf("IsActive should use 'launchctl list', got %q", isActive)
	}
	if !strings.Contains(isActive, "'com.example.svc'") {
		t.Errorf("IsActive should quote the label, got %q", isActive)
	}

	// Start/Stop/Enable/Disable should use launchctl load/unload with plist finding.
	start := b.CmdStart("homebrew.mxcl.nginx")
	if !strings.Contains(start, "launchctl load -w") {
		t.Errorf("Start should use 'launchctl load -w', got %q", start)
	}
	if !strings.Contains(start, "homebrew.mxcl.nginx.plist") {
		t.Errorf("Start should search for plist file, got %q", start)
	}

	stop := b.CmdStop("homebrew.mxcl.nginx")
	if !strings.Contains(stop, "launchctl unload") {
		t.Errorf("Stop should use 'launchctl unload', got %q", stop)
	}

	disable := b.CmdDisable("homebrew.mxcl.nginx")
	if !strings.Contains(disable, "launchctl unload -w") {
		t.Errorf("Disable should use 'launchctl unload -w', got %q", disable)
	}

	restart := b.CmdRestart("homebrew.mxcl.nginx")
	if !strings.Contains(restart, "launchctl unload") || !strings.Contains(restart, "launchctl load -w") {
		t.Errorf("Restart should unload then load, got %q", restart)
	}

	reload := b.CmdReload("homebrew.mxcl.nginx")
	if reload != "" {
		t.Errorf("Reload should be empty for launchctl, got %q", reload)
	}
}

func TestLaunchctlUserDomain(t *testing.T) {
	b := &launchctlBackend{domain: "user"}
	if b.NeedsRoot() {
		t.Error("user domain should not NeedsRoot()")
	}
}

func TestNewLaunchctlRootDetection(t *testing.T) {
	// Root user: test $(id -u) -ne 0 exits non-zero.
	root := newLaunchctl(fakeRunner(map[string]int{
		"test $(id -u) -ne 0": 1,
	}))
	if root.domain != "system" {
		t.Errorf("root should get system domain, got %q", root.domain)
	}

	// Non-root user: test $(id -u) -ne 0 exits zero.
	user := newLaunchctl(fakeRunner(map[string]int{
		"test $(id -u) -ne 0": 0,
		"id -u":               0,
	}))
	if user.domain != "user" {
		t.Errorf("non-root should get user domain, got %q", user.domain)
	}
}

func TestShellQuote(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"nginx", "'nginx'"},
		{"it's", "'it'\\''s'"},
		{"", "''"},
		{"a b c", "'a b c'"},
		{"$(evil)", "'$(evil)'"},
		{"`evil`", "'`evil`'"},
		{"foo;bar", "'foo;bar'"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := ShellQuote(tt.input)
			if got != tt.want {
				t.Errorf("ShellQuote(%q): got %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestPlistFindExpr(t *testing.T) {
	expr := plistFindExpr("com.example.svc")
	for _, dir := range launchctlDirs {
		if !strings.Contains(expr, dir) {
			t.Errorf("plistFindExpr should search %s, got %q", dir, expr)
		}
	}
	if !strings.Contains(expr, "'com.example.svc.plist'") {
		t.Errorf("plistFindExpr should quote the plist filename, got %q", expr)
	}
}

// fakeRunner returns a run function that maps commands to exit codes.
// Unknown commands return (1, error).
func fakeRunner(cmds map[string]int) func(string) (int, error) {
	return func(cmd string) (int, error) {
		if code, ok := cmds[cmd]; ok {
			return code, nil
		}
		return 1, nil
	}
}

func callMethod(b Backend, method, name string) string {
	switch method {
	case "IsActive":
		return b.CmdIsActive(name)
	case "IsEnabled":
		return b.CmdIsEnabled(name)
	case "Start":
		return b.CmdStart(name)
	case "Stop":
		return b.CmdStop(name)
	case "Enable":
		return b.CmdEnable(name)
	case "Disable":
		return b.CmdDisable(name)
	case "DaemonReload":
		return b.CmdDaemonReload()
	case "Restart":
		return b.CmdRestart(name)
	case "Reload":
		return b.CmdReload(name)
	default:
		panic("unknown method: " + method)
	}
}
