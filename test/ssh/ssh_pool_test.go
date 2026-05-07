// SPDX-License-Identifier: GPL-3.0-only

package ssh

import (
	"context"
	"sync"
	"testing"
	"time"

	"scampi.dev/scampi/test/harness"
)

// TestSSH_ConnectionPool_OneTCPDialPerTarget is the acceptance test for
// #238: confirms that the per-target SSH connection is reused across
// every RunCommand. DialCount must be exactly 1 for the target's
// lifetime, regardless of how many ops run sequentially or in parallel.
//
// This is the invariant the marketing claim "scampi opens a single
// multiplexed session per host" rests on. If a regression introduces a
// stray re-dial somewhere (e.g. someone adds a "fresh client" pattern
// for retries), DialCount will go above 1 and this test fails.
func TestSSH_ConnectionPool_OneTCPDialPerTarget(t *testing.T) {
	env, cleanup := harness.SetupSSHTestEnv(t)
	defer cleanup()

	tgt := harness.ConnectSSH(t, env)
	defer tgt.Close()

	// Baseline counters — Create() opens its own sessions for OS,
	// service-manager, container-runtime, PVE, and escalation
	// detection. We measure ops *after* setup so this test asserts
	// against our own work, not whatever probing Create() did.
	baseline := tgt.Stats()

	const sequentialOps = 20
	for i := range sequentialOps {
		if _, err := tgt.RunCommand(context.Background(), "true"); err != nil {
			t.Fatalf("op %d: %v", i, err)
		}
	}

	const parallelOps = 5
	var wg sync.WaitGroup
	errs := make(chan error, parallelOps)
	for range parallelOps {
		wg.Go(func() {
			if _, err := tgt.RunCommand(context.Background(), "true"); err != nil {
				errs <- err
			}
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("parallel op: %v", err)
	}

	stats := tgt.Stats()
	if stats.DialCount != 1 {
		t.Errorf("DialCount = %d, want 1 — connection should be pooled per target", stats.DialCount)
	}
	wantCommands := int64(sequentialOps + parallelOps)
	gotCommands := stats.CommandsRun - baseline.CommandsRun
	if gotCommands != wantCommands {
		t.Errorf("CommandsRun delta = %d, want %d (baseline = %d, total = %d)",
			gotCommands, wantCommands, baseline.CommandsRun, stats.CommandsRun)
	}
	// SessionsOpened is now a measure of pool churn, not command
	// count. With persistent shells, a small handful of sessions
	// should serve every command. Cap is loose — small fluctuations
	// based on parallelism timing are fine.
	gotSessions := stats.SessionsOpened - baseline.SessionsOpened
	if gotSessions > int64(parallelOps) {
		t.Errorf("SessionsOpened delta = %d, want at most %d — shells should be reused",
			gotSessions, parallelOps)
	}
}

// TestSSH_RunCommand_MultiLine is the acceptance test for the
// multi-line command bug discovered while testing #297 backtick
// strings. A user command with a literal trailing newline used to
// produce a `<NL>; }` sequence in the persistent-shell wrapper that
// bash rejects as a syntax error, causing the framed sentinel to
// never be written and the read loop to panic on EOF.
func TestSSH_RunCommand_MultiLine(t *testing.T) {
	env, cleanup := harness.SetupSSHTestEnv(t)
	defer cleanup()

	tgt := harness.ConnectSSH(t, env)
	defer tgt.Close()

	cases := []struct {
		name string
		cmd  string
	}{
		{
			name: "trailing-newline",
			cmd:  "echo hello\n",
		},
		{
			name: "embedded-pipeline",
			cmd:  "echo a |\n  cat |\n  cat\n",
		},
		{
			name: "multi-statement-and-and",
			cmd:  "echo first\necho second && echo third\n",
		},
		{
			name: "set-pipefail-prefix",
			// Repro from .issues/from_skrynet/posix-run-pipefail-ssh-panic.md
			cmd: "set -o pipefail; echo hello | grep -q hello",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := tgt.RunCommand(context.Background(), tc.cmd)
			if err != nil {
				t.Fatalf("RunCommand: %v", err)
			}
			if res.ExitCode != 0 {
				t.Errorf("exit %d, stderr=%q", res.ExitCode, res.Stderr)
			}
		})
	}
}

// TestSSH_RetryHandlesContention is the acceptance test for the
// retry-with-backoff resilience: scheduling far more parallel ops
// than what the server allows concurrently must still complete
// cleanly. The slot pool caps client-side concurrency at MaxSessions
// (a sanity ceiling), but the server can still reject channel opens
// (e.g. SFTP holds a session, so OpenSSH MaxSessions=10 effectively
// allows 9 of ours). retryNewSession must transparently absorb that
// backpressure.
func TestSSH_RetryHandlesContention(t *testing.T) {
	env, cleanup := harness.SetupSSHTestEnv(t)
	defer cleanup()

	tgt := harness.ConnectSSH(t, env)
	defer tgt.Close()

	maxSessions := int(tgt.Stats().MaxSessions)
	if maxSessions <= 0 {
		t.Fatalf("MaxSessions = %d, expected positive (slot pool size)", maxSessions)
	}

	// 3× the slot pool capacity in parallel. With each holding a
	// session for ~50ms, this guarantees the server-side cap is hit
	// AND the client-side slot pool saturates. Both backpressure
	// mechanisms get exercised. All ops must still complete.
	parallelOps := maxSessions * 3
	var wg sync.WaitGroup
	errs := make(chan error, parallelOps)
	for range parallelOps {
		wg.Go(func() {
			if _, err := tgt.RunCommand(context.Background(), "sleep 0.05"); err != nil {
				errs <- err
			}
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("parallel op (retry should handle contention): %v", err)
	}

	stats := tgt.Stats()
	if stats.SessionsPeakInFlight > int64(maxSessions) {
		t.Errorf("peak in-flight = %d, must not exceed slot pool size (%d)",
			stats.SessionsPeakInFlight, maxSessions)
	}
	if stats.SessionsPeakInFlight == 0 {
		t.Error("peak in-flight = 0; slot acquisition not being tracked")
	}
	t.Logf("ops=%d, peak in-flight=%d, server retries=%d",
		parallelOps, stats.SessionsPeakInFlight, stats.SessionRetries)
}

// TestSSH_RunCommand_ContextCancellation verifies that an op blocked
// waiting for a slot returns promptly when its context is cancelled,
// rather than hanging forever.
func TestSSH_RunCommand_ContextCancellation(t *testing.T) {
	env, cleanup := harness.SetupSSHTestEnv(t)
	defer cleanup()

	tgt := harness.ConnectSSH(t, env)
	defer tgt.Close()

	maxSessions := int(tgt.Stats().MaxSessions)
	if maxSessions <= 0 {
		t.Fatalf("MaxSessions = %d, expected positive", maxSessions)
	}

	// Saturate the pool with long-running ops so the next acquire blocks.
	saturating := make(chan struct{})
	released := make(chan struct{})
	for range maxSessions {
		go func() {
			saturating <- struct{}{}
			_, _ = tgt.RunCommand(context.Background(), "sleep 5")
			released <- struct{}{}
		}()
	}
	for range maxSessions {
		<-saturating
	}
	// Give the sleeping ops a moment to actually grab their slots.
	time.Sleep(100 * time.Millisecond)

	// This call should block on slot acquire, then return ctx.Err()
	// when ctx is cancelled — not hang or panic.
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := tgt.RunCommand(ctx, "true")
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected context-cancelled error, got success")
	}
	if elapsed > 1*time.Second {
		t.Errorf("RunCommand took %v after ctx cancel — should have returned promptly", elapsed)
	}

	// Drain the saturating ops so the test cleans up.
	for range maxSessions {
		<-released
	}
}
