// SPDX-License-Identifier: GPL-3.0-only

package ssh

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/crypto/ssh"
	"scampi.dev/scampi/errs"
	"scampi.dev/scampi/target"
)

// Persistent shell sessions
// -----------------------------------------------------------------------------
//
// Each shellSession wraps one *ssh.Session running a long-lived shell
// (`/bin/sh`) on the target. Commands are streamed into the shell's
// stdin one after another and parsed back out of stdout. The session
// is reused across many ops, amortising the SSH channel-open + exec
// handshake (~2 RTTs) over the lifetime of the session instead of
// paying it per op.
//
// Framing — the shell wrapper for each command is:
//
//   { <user-cmd>; } 2><tmp>
//   __SC_RC=$?
//   __SC_EB=$(wc -c <<tmp> 2>/dev/null | tr -d ' ')
//   printf '%s:%d:%d\n' '<sentinel>' "$__SC_RC" "${__SC_EB:-0}"
//   cat <tmp> 2>/dev/null
//   rm -f <tmp>
//
// On the wire the reader sees:
//
//   <stdout of user-cmd>          (zero or more lines)
//   <sentinel>:<rc>:<errBytes>\n  (one line)
//   <stderr of user-cmd>          (exactly errBytes bytes, no terminator)
//
// The sentinel is `__SC_END_<128-bit-nonce>_<seq>__`. The nonce is
// per-session, the seq increments per command — both are needed so
// that a malicious or accidental command output containing the
// sentinel cannot collide. Length-prefixed stderr (the `errBytes`
// field) means we never have to scan for a stderr terminator.

// shellSession is a single long-lived shell on the target. Not safe
// for concurrent use — the pool serialises Run calls per session.
type shellSession struct {
	sess   *ssh.Session
	stdin  io.WriteCloser
	stdout *bufio.Reader
	stderr io.Reader  // captured for the rare case we need it on init
	doneC  chan error // shell exit
	nonce  string     // 128-bit per-session, hex
	seq    uint64     // increments per command, for unique sentinel + tmp
	tmpDir string     // /tmp by default
	mu     sync.Mutex
	unsafe bool // true after any I/O failure; pool will drop
}

// openShellSession starts /bin/sh on the target and returns a session
// ready to take commands.
func openShellSession(client *ssh.Client) (*shellSession, error) {
	sess, err := client.NewSession()
	if err != nil {
		return nil, err
	}
	stdin, err := sess.StdinPipe()
	if err != nil {
		_ = sess.Close()
		return nil, err
	}
	stdout, err := sess.StdoutPipe()
	if err != nil {
		_ = sess.Close()
		return nil, err
	}
	stderr, err := sess.StderrPipe()
	if err != nil {
		_ = sess.Close()
		return nil, err
	}

	// /bin/sh is the lowest-common-denominator POSIX shell. Our
	// wrapper uses only printf, wc, cat, rm — POSIX, no bashisms.
	// Pass -s so the shell reads commands from stdin even when no
	// args are given (some shells need this hint).
	if err := sess.Start("/bin/sh -s"); err != nil {
		_ = sess.Close()
		return nil, err
	}

	doneC := make(chan error, 1)
	go func() { doneC <- sess.Wait() }()

	var nonceBytes [16]byte
	if _, err := rand.Read(nonceBytes[:]); err != nil {
		_ = sess.Close()
		return nil, err
	}

	return &shellSession{
		sess:   sess,
		stdin:  stdin,
		stdout: bufio.NewReader(stdout),
		stderr: stderr,
		doneC:  doneC,
		nonce:  hex.EncodeToString(nonceBytes[:]),
		tmpDir: "/tmp",
	}, nil
}

// healthy reports whether this session is still usable.
func (s *shellSession) healthy() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return !s.unsafe
}

// markUnsafe flags the session for disposal — it must not be reused.
// Called on any I/O error or on cancellation.
func (s *shellSession) markUnsafe() {
	s.mu.Lock()
	s.unsafe = true
	s.mu.Unlock()
}

// run sends one command into the shell, parses the framed result.
//
// Cancellation: ctx.Done closes the shell — the shell is unrecoverable
// after that and the pool will drop it on release. This is fine; the
// pool reopens lazily on next acquire.
func (s *shellSession) run(ctx context.Context, cmd string) (target.CommandResult, error) {
	s.mu.Lock()
	if s.unsafe {
		s.mu.Unlock()
		return target.CommandResult{}, errs.WrapErrf(errSession, "session no longer usable")
	}
	s.seq++
	seq := s.seq
	s.mu.Unlock()

	tmp := fmt.Sprintf("%s/.sc-%s-%d", s.tmpDir, s.nonce, seq)
	sentinel := fmt.Sprintf("__SC_END_%s_%d__", s.nonce, seq)

	// Wrapper around the user command. Newlines surrounding the
	// brace group make this safe for both single-line and multi-
	// line user commands: a trailing newline inside cmd would
	// otherwise produce `<NL>; }` which bash rejects as a syntax
	// error near `;`. With explicit newlines around `{` / `}` the
	// shape parses regardless of cmd's trailing whitespace.
	wrapper := fmt.Sprintf(
		"{\n%s\n} 2>%s; __SC_RC=$?; __SC_EB=$(wc -c <%s 2>/dev/null | tr -d ' '); "+
			"printf '%%s:%%d:%%d\\n' '%s' \"$__SC_RC\" \"${__SC_EB:-0}\"; "+
			"cat %s 2>/dev/null; rm -f %s\n",
		cmd, tmp, tmp, sentinel, tmp, tmp,
	)

	// Watch ctx in a goroutine — on cancel, kill the session so the
	// blocking ReadBytes below unblocks with an error.
	doneRead := make(chan struct{})
	defer close(doneRead)
	go func() {
		select {
		case <-doneRead:
		case <-ctx.Done():
			s.markUnsafe()
			_ = s.sess.Close()
		}
	}()

	if _, err := io.WriteString(s.stdin, wrapper); err != nil {
		s.markUnsafe()
		return target.CommandResult{}, errs.WrapErrf(errSession, "write: %v", err)
	}

	stdoutBuf, rc, errBytes, err := s.readFramed(sentinel)
	if err != nil {
		s.markUnsafe()
		if ctx.Err() != nil {
			return target.CommandResult{}, errs.WrapErrf(errSession, "%v", ctx.Err())
		}
		return target.CommandResult{}, errs.WrapErrf(errSession, "read: %v", err)
	}

	var stderrBuf []byte
	if errBytes > 0 {
		stderrBuf = make([]byte, errBytes)
		if _, err := io.ReadFull(s.stdout, stderrBuf); err != nil {
			s.markUnsafe()
			return target.CommandResult{}, errs.WrapErrf(errSession, "read stderr: %v", err)
		}
	}

	return target.CommandResult{
		Stdout:   stdoutBuf.String(),
		Stderr:   string(stderrBuf),
		ExitCode: rc,
	}, nil
}

// readFramed reads stdout up to and including the sentinel line, returning
// everything before the sentinel as stdout, and parsing the trailing
// "<sentinel>:<rc>:<errBytes>\n" suffix.
func (s *shellSession) readFramed(sentinel string) (
	stdout bytes.Buffer,
	rc int,
	errBytes int,
	err error,
) {
	prefix := []byte(sentinel + ":")
	for {
		line, readErr := s.stdout.ReadBytes('\n')
		if readErr != nil {
			return stdout, 0, 0, readErr
		}

		before, after, found := bytes.Cut(line, prefix)
		if !found {
			_, _ = stdout.Write(line)
			continue
		}

		// Anything before the sentinel on this line was tail-end of
		// command stdout (no trailing \n on the last user line).
		_, _ = stdout.Write(before)

		rest := strings.TrimRight(string(after), "\n")
		parts := strings.SplitN(rest, ":", 2)
		if len(parts) != 2 {
			return stdout, 0, 0, fmt.Errorf("malformed sentinel suffix: %q", rest)
		}
		rc, err = strconv.Atoi(parts[0])
		if err != nil {
			return stdout, 0, 0, fmt.Errorf("bad rc %q: %w", parts[0], err)
		}
		errBytes, err = strconv.Atoi(parts[1])
		if err != nil {
			return stdout, 0, 0, fmt.Errorf("bad errBytes %q: %w", parts[1], err)
		}
		return stdout, rc, errBytes, nil
	}
}

// close ends the shell and tears down the SSH session. Safe to call
// multiple times.
func (s *shellSession) close() error {
	s.mu.Lock()
	if s.unsafe {
		s.mu.Unlock()
		_ = s.sess.Close()
		return nil
	}
	s.unsafe = true
	s.mu.Unlock()

	// Best-effort polite shutdown; ignore errors — we're closing.
	_, _ = io.WriteString(s.stdin, "exit 0\n")
	_ = s.stdin.Close()
	return s.sess.Close()
}
