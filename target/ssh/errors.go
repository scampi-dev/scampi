// SPDX-License-Identifier: GPL-3.0-only

package ssh

import (
	"encoding/base64"
	"fmt"

	"golang.org/x/crypto/ssh/knownhosts"
	"scampi.dev/scampi/diagnostic"
	"scampi.dev/scampi/diagnostic/event"
	"scampi.dev/scampi/spec"
)

type NoKnownHostsError struct {
	diagnostic.FatalError
	Path string
	Err  error
}

func (e NoKnownHostsError) Error() string {
	return fmt.Sprintf("known_hosts file not found: %s", e.Path)
}

func (e NoKnownHostsError) Unwrap() error { return e.Err }

func (e NoKnownHostsError) EventTemplate() event.Template {
	return event.Template{
		ID:   CodeNoKnownHosts,
		Text: `known_hosts file "{{.Path}}" not found`,
		Hint: "create the file or use insecure: true to skip host key verification",
		Help: "without a known_hosts file, host key verification cannot proceed",
		Data: e,
	}
}

type NoSuchHostError struct {
	diagnostic.FatalError
	Host   string
	Source spec.SourceSpan
}

func (e NoSuchHostError) Error() string {
	return fmt.Sprintf("no such host %s", e.Host)
}

func (e NoSuchHostError) EventTemplate() event.Template {
	return event.Template{
		ID:     CodeNoSuchHost,
		Text:   "no such host {{.Host}}",
		Hint:   "make sure the host is reachable",
		Source: &e.Source,
		Data:   e,
	}
}

type ConnectionError struct {
	diagnostic.FatalError
	Host string
	Port int
	Err  error
}

func (e ConnectionError) Error() string {
	return fmt.Sprintf("failed to connect to %s:%d: %v", e.Host, e.Port, e.Err)
}

func (e ConnectionError) Unwrap() error { return e.Err }

func (e ConnectionError) EventTemplate() event.Template {
	return event.Template{
		ID:   CodeConnection,
		Text: "failed to connect to {{.Host}}:{{.Port}}",
		Hint: "make sure the host is reachable and SSH is running on the given port",
		Help: "underlying error was: {{.Err}}",
		Data: e,
	}
}

type UnknownKeyError struct {
	diagnostic.FatalError
	Err error
}

func (e UnknownKeyError) Error() string {
	return fmt.Sprintf("unknown host SSH-key: %v", e.Err)
}

func (e UnknownKeyError) Unwrap() error { return e.Err }

func (e UnknownKeyError) EventTemplate() event.Template {
	return event.Template{
		ID:   CodeUnknownKey,
		Text: "unknown host SSH-key",
		Hint: "connect manually once with ssh to add the host key, or use insecure: true to skip verification",
	}
}

func toKnownKeys(keys []knownhosts.KnownKey) []KnownKey {
	l := len(keys)
	res := make([]KnownKey, l)
	for i := range l {
		res[i] = toKnownKey(keys[i])
	}
	return res
}

func toKnownKey(k knownhosts.KnownKey) KnownKey {
	fingerprint := func(s string) string {
		l := len(s)
		if l <= 7*2 {
			return s
		}

		return s[:7] + "..." + s[l-7:]
	}
	key := base64.StdEncoding.EncodeToString(k.Key.Marshal())

	return KnownKey{
		Type:        k.Key.Type(),
		Key:         key,
		Fingerprint: fingerprint(key),
		Filename:    k.Filename,
		Line:        k.Line,
	}
}

type KnownKey struct {
	Type        string
	Key         string
	Fingerprint string
	Filename    string
	Line        int
}

type KeyMismatchError struct {
	diagnostic.FatalError
	Known []KnownKey
	Err   error
}

func (e KeyMismatchError) Error() string {
	return fmt.Sprintf("host SSH-key mismatch: %v", e.Err)
}

func (e KeyMismatchError) Unwrap() error { return e.Err }

func (e KeyMismatchError) EventTemplate() event.Template {
	return event.Template{
		ID:   CodeKeyMismatch,
		Text: "host SSH-key mismatch",
		Hint: "if the host was reinstalled, remove the old entry from known_hosts and reconnect",
		Help: `known host keys:
{{- range .Known}}
  - {{.Filename}}:{{.Line}}: {{.Type}} {{.Fingerprint}}
{{end}}`,
		Data: e,
	}
}

type KeyRevokedError struct {
	diagnostic.FatalError
	Revoked KnownKey
	Err     error
}

func (e KeyRevokedError) Error() string {
	return fmt.Sprintf("host SSH-key revoked: %v", e.Err)
}

func (e KeyRevokedError) Unwrap() error { return e.Err }

func (e KeyRevokedError) EventTemplate() event.Template {
	return event.Template{
		ID:   CodeKeyRevoked,
		Text: "host SSH-key revoked",
		Hint: "this host key was explicitly revoked in known_hosts — do not connect unless you trust the host",
		Help: `revoked host key:
  {{.Revoked.Filename}}:{{.Revoked.Line}}: {{.Revoked.Type}} {{.Revoked.Fingerprint}}`,
		Data: e,
	}
}

type KeyReadError struct {
	diagnostic.FatalError
	Path string
	Err  error
}

func (e KeyReadError) Error() string {
	return fmt.Sprintf("failed to read key file %q: %v", e.Path, e.Err)
}

func (e KeyReadError) EventTemplate() event.Template {
	return event.Template{
		ID:   CodeKeyRead,
		Text: `failed to read SSH-key file "{{.Path}}"`,
		Hint: "check that the file exists and is readable",
		Data: e,
	}
}

type KeyParseError struct {
	diagnostic.FatalError
	Path        string
	IsPublicKey bool
	Err         error
}

func (e KeyParseError) Error() string {
	return fmt.Sprintf("failed to parse key file %q: %v", e.Path, e.Err)
}

func (e KeyParseError) EventTemplate() event.Template {
	return event.Template{
		ID:   CodeKeyParse,
		Text: "failed to parse SSH-key file {{.Path}}",
		Hint: "the provided key-file must contain a valid *private* SSH-key",
		Help: `{{if .IsPublicKey}}found valid *public* SSH-key, while a *private* SSH-key is required{{end}}`,
		Data: e,
	}
}

type NoAuthMethodError struct {
	diagnostic.FatalError
}

func (NoAuthMethodError) Error() string {
	return "no SSH authentication method available (no key specified and SSH agent not available)"
}

func (e NoAuthMethodError) EventTemplate() event.Template {
	return event.Template{
		ID:   CodeNoAuthMethod,
		Text: "no SSH authentication method available",
		Hint: "no key specified and SSH agent unavailable",
		Help: "specify a key and/or start SSH agent",
		Data: e,
	}
}

type AuthError struct {
	diagnostic.FatalError
	Err error
}

func (e AuthError) Error() string {
	return fmt.Sprintf("authentication failed: %v", e.Err)
}

func (e AuthError) Unwrap() error { return e.Err }

func (e AuthError) EventTemplate() event.Template {
	return event.Template{
		ID:   CodeAuth,
		Text: "authentication failed: {{.Err}}",
		Hint: "check that the SSH key is authorized on the remote host",
		Data: e,
	}
}

type InvalidTimeoutError struct {
	diagnostic.FatalError
	Value  string
	Source spec.SourceSpan
	Err    error
}

func (e InvalidTimeoutError) Error() string {
	return fmt.Sprintf("invalid timeout %q: %v", e.Value, e.Err)
}

func (e InvalidTimeoutError) Unwrap() error { return e.Err }

func (e InvalidTimeoutError) EventTemplate() event.Template {
	return event.Template{
		ID:     CodeInvalidTimeout,
		Text:   `invalid timeout "{{.Value}}"`,
		Hint:   `use a human-readable duration like "2s", "1m30s", or "500ms"`,
		Source: &e.Source,
		Data:   e,
	}
}

type SFTPSessionError struct {
	diagnostic.FatalError
	Err error
}

func (e SFTPSessionError) Error() string {
	return fmt.Sprintf("failed to start SFTP session: %v", e.Err)
}

func (e SFTPSessionError) Unwrap() error { return e.Err }

func (e SFTPSessionError) EventTemplate() event.Template {
	return event.Template{
		ID:   CodeSFTPSession,
		Text: "failed to start SFTP session: {{.Err}}",
		Hint: "check that the SFTP subsystem is enabled on the remote host",
		Data: e,
	}
}
