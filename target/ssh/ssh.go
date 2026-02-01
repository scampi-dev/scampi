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

	"godoit.dev/doit/errs"
	"godoit.dev/doit/source"
	"godoit.dev/doit/spec"
	"godoit.dev/doit/target"
)

const knownHostsFile = "~/.ssh/known_hosts"

type (
	SSH    struct{}
	Config struct {
		Host     string
		Port     int
		User     string
		Key      string
		Insecure bool // Skip host key verification
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

	sshCfg, closeAgent, err := buildSSHConfig(ctx, src, cfg)
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
			// Source: tgt.Fields["host"].Value,
			Err: rootErr,
		}
	}

	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		_ = closeAgent()
		_ = client.Close()
		return nil, SFTPError{Err: err}
	}

	return &SSHTarget{
		config:     cfg,
		client:     client,
		sftp:       sftpClient,
		closeAgent: closeAgent,
	}, nil
}

func buildSSHConfig(ctx context.Context, src source.Source, c *Config) (*ssh.ClientConfig, func() error, error) {
	var authMethods []ssh.AuthMethod
	closeAgent := func() error { return nil }

	// Try explicit key first
	if c.Key != "" {
		keyPath, err := expandPath(c.Key)
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
		khPath, _ := expandPath(knownHostsFile)
		var err error
		hostKeyCallback, err = knownhosts.New(khPath)
		if err != nil {
			// Fall back to insecure if known_hosts doesn't exist
			// TODO: Make this configurable
			hostKeyCallback = ssh.InsecureIgnoreHostKey()
		}
	}

	return &ssh.ClientConfig{
		User:            c.User,
		Auth:            authMethods,
		HostKeyCallback: hostKeyCallback,
		// TODO: ahrdcoded timeout
		Timeout: 1 * time.Second,
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

func expandPath(p string) (string, error) {
	if p == "" {
		return "", nil
	}

	if p == "~" || strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}

		if p == "~" {
			p = home
		} else {
			p = filepath.Join(home, p[2:])
		}
	}

	return filepath.Abs(p)
}
