// SPDX-License-Identifier: GPL-3.0-only

package pve

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/knownhosts"

	"scampi.dev/scampi/errs"
	"scampi.dev/scampi/source"
	"scampi.dev/scampi/spec"
	"scampi.dev/scampi/target"
	"scampi.dev/scampi/target/posix"
)

const knownHostsFile = "~/.ssh/known_hosts"

type (
	LXC    struct{}
	Config struct {
		_        struct{} `summary:"Connect to an LXC container via the PVE host (pct exec)"`
		Host     string   `step:"PVE host" example:"10.0.0.1"`
		Port     int      `step:"SSH port to PVE host" default:"22"`
		User     string   `step:"SSH user on PVE host" example:"root"`
		Key      string   `step:"Path to SSH private key" optional:"true"`
		Insecure bool     `step:"Skip host key verification" optional:"true"`
		Timeout  string   `step:"Connection timeout" default:"5s"`
		VMID     int      `step:"Container VMID"`
	}
)

func (LXC) Kind() string   { return "pve.lxc_target" }
func (LXC) NewConfig() any { return &Config{} }
func (LXC) Create(ctx context.Context, src source.Source, tgt spec.TargetInstance) (target.Target, error) {
	cfg, ok := tgt.Config.(*Config)
	if !ok {
		return nil, errs.BUG("expected %T got %T", &Config{}, cfg)
	}
	if cfg.Port == 0 {
		cfg.Port = 22
	}
	if cfg.Timeout == "" {
		cfg.Timeout = "5s"
	}
	if cfg.VMID < 100 {
		// bare-error: validate-time check, no source span available here
		return nil, errs.Errorf("pve.target: VMID must be >= 100")
	}

	timeout, err := time.ParseDuration(cfg.Timeout)
	if err != nil {
		return nil, errs.WrapErrf(err, "parse timeout")
	}

	sshCfg, closeAgent, err := buildSSHConfig(ctx, src, cfg, timeout)
	if err != nil {
		_ = closeAgent()
		return nil, err
	}

	addr := net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))
	client, err := ssh.Dial("tcp", addr, sshCfg)
	if err != nil {
		_ = closeAgent()
		return nil, errs.WrapErrf(err, "ssh dial %s", addr)
	}

	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		_ = closeAgent()
		_ = client.Close()
		return nil, errs.WrapErrf(err, "sftp session")
	}

	t := &LXCTarget{
		config:     cfg,
		client:     client,
		sftp:       sftpClient,
		closeAgent: closeAgent,
		vmid:       cfg.VMID,
	}
	t.Runner = t.runInContainer

	// Detect platform inside the container so step code can dispatch.
	// Fall back to Linux/Debian — PVE LXCs are Linux.
	if r, err := t.runInContainer(ctx, "uname -s"); err == nil {
		t.OSInfo.Platform = target.ParseKernel(strings.TrimSpace(r.Stdout))
		if osr, err := t.ReadFile(ctx, "/etc/os-release"); err == nil {
			t.OSInfo = target.ResolveLinuxPlatform(osr)
		}
	}

	// Detect host-side escalation needed for pct (it requires root).
	hostRunner := func(ctx context.Context, cmd string) (target.CommandResult, error) {
		return t.runOnHost(ctx, cmd)
	}
	if r, err := hostRunner(ctx, "id -u"); err == nil {
		t.hostIsRoot = strings.TrimSpace(r.Stdout) == "0"
	}
	t.hostEscalate = posix.DetectEscalation(ctx, hostRunner, t.hostIsRoot)

	// Inside the container we run as root (pct exec is root by default),
	// so no escalation needed for in-container ops.
	t.IsRoot = true
	t.Escalate = ""

	return t, nil
}

// buildSSHConfig is duplicated from target/ssh — could be factored
// into a shared package later.
func buildSSHConfig(
	ctx context.Context,
	src source.Source,
	c *Config,
	timeout time.Duration,
) (*ssh.ClientConfig, func() error, error) {
	var authMethods []ssh.AuthMethod
	closeAgent := func() error { return nil }

	if c.Key != "" {
		keyPath, err := expandTilde(c.Key)
		if err != nil {
			return nil, closeAgent, errs.WrapErrf(err, "expand %s", c.Key)
		}
		key, err := src.ReadFile(ctx, keyPath)
		if err != nil {
			return nil, closeAgent, errs.WrapErrf(err, "read key %s", keyPath)
		}
		signer, err := ssh.ParsePrivateKey(key)
		if err != nil {
			return nil, closeAgent, errs.WrapErrf(err, "parse key %s", keyPath)
		}
		authMethods = append(authMethods, ssh.PublicKeys(signer))
	}

	if sock, ok := src.LookupEnv("SSH_AUTH_SOCK"); ok {
		conn, err := net.Dial("unix", sock)
		if err == nil {
			ag := agent.NewClient(conn)
			authMethods = append(authMethods, ssh.PublicKeysCallback(ag.Signers))
			closeAgent = conn.Close
		}
	}

	if len(authMethods) == 0 {
		// bare-error: no source span at this layer
		return nil, closeAgent, errs.Errorf("pve.target: no SSH auth method available")
	}

	var hostKeyCallback ssh.HostKeyCallback
	if c.Insecure {
		hostKeyCallback = ssh.InsecureIgnoreHostKey()
	} else {
		khPath, _ := expandTilde(knownHostsFile)
		var err error
		hostKeyCallback, err = knownhosts.New(khPath)
		if err != nil {
			return nil, closeAgent, errs.WrapErrf(err, "load known_hosts %s", khPath)
		}
	}

	return &ssh.ClientConfig{
		User:            c.User,
		Auth:            authMethods,
		HostKeyCallback: hostKeyCallback,
		Timeout:         timeout,
	}, closeAgent, nil
}

func expandTilde(p string) (string, error) {
	if p == "" {
		return "", nil
	}
	if p == "~" || strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		if p == "~" {
			return home, nil
		}
		return filepath.Join(home, p[2:]), nil
	}
	return p, nil
}

// pctPrefix returns the pct command with optional sudo escalation.
func (t *LXCTarget) pctPrefix() string {
	if t.hostEscalate != "" {
		return t.hostEscalate + " pct"
	}
	return "pct"
}
