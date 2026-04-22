// SPDX-License-Identifier: GPL-3.0-only

package ssh

import (
	"context"
	"errors"
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
	"scampi.dev/scampi/target/ctrmgr"
	"scampi.dev/scampi/target/pkgmgr"
	"scampi.dev/scampi/target/posix"
	"scampi.dev/scampi/target/svcmgr"
)

const knownHostsFile = "~/.ssh/known_hosts"

type (
	SSH    struct{}
	Config struct {
		_        struct{} `summary:"Connect to a remote host via SSH"`
		Host     string   `step:"Hostname or IP address" example:"10.0.0.1"`
		Port     int      `step:"SSH port" default:"22"`
		User     string   `step:"SSH user" example:"root"`
		Key      string   `step:"Path to SSH private key" optional:"true"`
		Insecure bool     `step:"Skip host key verification" optional:"true"`
		Timeout  string   `step:"Connection timeout" default:"5s"`
	}
)

func (SSH) Kind() string   { return "ssh" }
func (SSH) NewConfig() any { return &Config{} }
func (SSH) Create(ctx context.Context, src source.Source, tgt spec.TargetInstance) (target.Target, error) {
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

	timeout, err := time.ParseDuration(cfg.Timeout)
	if err != nil {
		return nil, InvalidTimeoutError{
			Value:  cfg.Timeout,
			Source: tgt.Fields["timeout"].Value,
			Err:    err,
		}
	}

	sshCfg, closeAgent, err := buildSSHConfig(ctx, src, cfg, timeout)
	if err != nil {
		_ = closeAgent()
		return nil, err
	}

	if !isHostResolvable(cfg.Host) {
		_ = closeAgent()
		return nil, NoSuchHostError{
			Host:   cfg.Host,
			Source: tgt.Fields["host"].Value,
		}
	}

	addr := net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))
	client, err := ssh.Dial("tcp", addr, sshCfg)
	if err != nil {
		defer func() { _ = closeAgent() }()
		var ke *knownhosts.KeyError
		if errors.As(err, &ke) {
			if len(ke.Want) == 0 {
				return nil, UnknownKeyError{
					Err: err,
				}
			}
			return nil, KeyMismatchError{
				Known: toKnownKeys(ke.Want),
				Err:   err,
			}
		}

		var rk *knownhosts.RevokedError
		if errors.As(err, &rk) {
			return nil, KeyRevokedError{
				Revoked: toKnownKey(rk.Revoked),
				Err:     err,
			}
		}

		rootErr := errs.UnwrapAll(err)
		if strings.Contains(rootErr.Error(), "authenticate") {
			return nil, AuthError{
				Err: rootErr,
			}
		}

		return nil, ConnectionError{
			Host: cfg.Host,
			Port: cfg.Port,
			Err:  rootErr,
		}
	}

	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		_ = closeAgent()
		_ = client.Close()
		return nil, SFTPSessionError{Err: err}
	}

	sshTgt := &SSHTarget{
		config:     cfg,
		client:     client,
		sftp:       sftpClient,
		closeAgent: closeAgent,
	}
	sshTgt.Runner = sshTgt.RunCommand

	// OS detection for package manager and platform-specific dispatch.
	if result, err := sshTgt.RunCommand(ctx, "uname -s"); err == nil {
		kernel := strings.TrimSpace(result.Stdout)
		sshTgt.OSInfo.Platform = target.ParseKernel(kernel)

		// Linux needs distro detection via /etc/os-release.
		if kernel == "Linux" {
			if osRelease, err := sshTgt.ReadFile(ctx, "/etc/os-release"); err == nil {
				sshTgt.OSInfo = target.ResolveLinuxPlatform(osRelease)
			}
		}
	}

	sshTgt.PkgBackend = pkgmgr.Detect(sshTgt.OSInfo.Platform)

	// Init system and container runtime detection.
	detectCmd := func(cmd string) (int, error) {
		result, err := sshTgt.RunCommand(ctx, cmd)
		if err != nil {
			return -1, err
		}
		return result.ExitCode, nil
	}
	sshTgt.SvcBackend = svcmgr.Detect(detectCmd)
	sshTgt.CtrBackend = ctrmgr.Detect(detectCmd)
	sshTgt.HasPVE = detectAllCmds(detectCmd, "/usr/sbin/pct", "/usr/sbin/qm", "/usr/sbin/pvesm")

	// Privilege escalation detection.
	if result, err := sshTgt.RunCommand(ctx, "id -u"); err == nil {
		sshTgt.IsRoot = strings.TrimSpace(result.Stdout) == "0"
	}
	sshTgt.Escalate = posix.DetectEscalation(ctx, sshTgt.RunCommand, sshTgt.IsRoot)

	return sshTgt, nil
}

func detectAllCmds(run func(string) (int, error), cmds ...string) bool {
	for _, cmd := range cmds {
		code, err := run("command -v " + cmd)
		if err != nil || code != 0 {
			return false
		}
	}
	return true
}

func buildSSHConfig(
	ctx context.Context,
	src source.Source,
	c *Config,
	timeout time.Duration,
) (*ssh.ClientConfig, func() error, error) {
	var authMethods []ssh.AuthMethod
	closeAgent := func() error { return nil }

	// Try explicit key first
	if c.Key != "" {
		keyPath, err := expandTilde(c.Key)
		if err != nil {
			return nil, closeAgent, KeyReadError{Path: keyPath, Err: err}
		}
		key, err := src.ReadFile(ctx, keyPath)
		if err != nil {
			return nil, closeAgent, KeyReadError{Path: keyPath, Err: err}
		}
		signer, err := ssh.ParsePrivateKey(key)
		if err != nil {
			_, _, _, _, pubErr := ssh.ParseAuthorizedKey(key)
			return nil, closeAgent, KeyParseError{
				Path:        keyPath,
				IsPublicKey: pubErr == nil,
				Err:         err,
			}
		}
		authMethods = append(authMethods, ssh.PublicKeys(signer))
	}

	// Try SSH agent
	if sock, ok := src.LookupEnv("SSH_AUTH_SOCK"); ok {
		conn, err := net.Dial("unix", sock)
		if err == nil {
			ag := agent.NewClient(conn)
			authMethods = append(authMethods, ssh.PublicKeysCallback(ag.Signers))
			closeAgent = conn.Close
		}
	}

	if len(authMethods) == 0 {
		return nil, closeAgent, NoAuthMethodError{}
	}

	var hostKeyCallback ssh.HostKeyCallback
	if c.Insecure {
		hostKeyCallback = ssh.InsecureIgnoreHostKey()
	} else {
		khPath, _ := expandTilde(knownHostsFile)
		var err error
		hostKeyCallback, err = knownhosts.New(khPath)
		if err != nil {
			return nil, closeAgent, NoKnownHostsError{Path: khPath, Err: err}
		}
	}

	return &ssh.ClientConfig{
		User:            c.User,
		Auth:            authMethods,
		HostKeyCallback: hostKeyCallback,
		Timeout:         timeout,
	}, closeAgent, nil
}

func isHostResolvable(h string) bool {
	if _, _, err := net.ParseCIDR(h); err == nil {
		return true
	}

	if net.ParseIP(h) != nil {
		return true
	}

	if _, err := net.ResolveIPAddr("ip", h); err == nil {
		return true
	}

	return false
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
