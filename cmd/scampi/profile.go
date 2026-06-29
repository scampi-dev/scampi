// SPDX-License-Identifier: GPL-3.0-only

package main

import (
	"os"
	"runtime"
	"runtime/pprof"
	"runtime/trace"
	"strings"

	"scampi.dev/scampi/internal/errs"
)

// Profiling
// -----------------------------------------------------------------------------
//
// CPU profile, heap profile, and execution trace are wired in main() as
// early as possible — args are scanned by hand before urfave/cli runs,
// so profiling captures flag parsing, command dispatch, and engine
// boot, not just the step body. The same flags are also registered
// on the root command so --help advertises them and unknown-flag
// rejection doesn't trip.
//
// stopProfiling is idempotent. main() calls it at every exit path
// because os.Exit skips deferred functions.

const (
	flagCPUProfile = "cpuprofile"
	flagMemProfile = "memprofile"
	flagTrace      = "trace"
)

var profState struct {
	cpuFile   *os.File
	traceFile *os.File
	memPath   string
}

func startProfiling(args []string) error {
	cpuPath := scanProfileFlag(args, "--"+flagCPUProfile)
	memPath := scanProfileFlag(args, "--"+flagMemProfile)
	tracePath := scanProfileFlag(args, "--"+flagTrace)

	if cpuPath != "" {
		f, err := os.Create(cpuPath)
		if err != nil {
			// bare-error: CLI startup error, before the diagnostic pipeline exists
			return errs.Errorf("--%s: %v", flagCPUProfile, err)
		}
		if err := pprof.StartCPUProfile(f); err != nil {
			_ = f.Close()
			// bare-error: CLI startup error, before the diagnostic pipeline exists
			return errs.Errorf("--%s: %v", flagCPUProfile, err)
		}
		profState.cpuFile = f
	}

	if tracePath != "" {
		f, err := os.Create(tracePath)
		if err != nil {
			// bare-error: CLI startup error, before the diagnostic pipeline exists
			return errs.Errorf("--%s: %v", flagTrace, err)
		}
		if err := trace.Start(f); err != nil {
			_ = f.Close()
			// bare-error: CLI startup error, before the diagnostic pipeline exists
			return errs.Errorf("--%s: %v", flagTrace, err)
		}
		profState.traceFile = f
	}

	profState.memPath = memPath
	return nil
}

func stopProfiling() {
	if profState.cpuFile != nil {
		pprof.StopCPUProfile()
		_ = profState.cpuFile.Close()
		profState.cpuFile = nil
	}
	if profState.traceFile != nil {
		trace.Stop()
		_ = profState.traceFile.Close()
		profState.traceFile = nil
	}
	if profState.memPath != "" {
		f, err := os.Create(profState.memPath)
		if err == nil {
			runtime.GC()
			_ = pprof.WriteHeapProfile(f)
			_ = f.Close()
		}
		profState.memPath = ""
	}
}

// scanProfileFlag finds --name=value or --name value in args.
// Returns "" if not found.
func scanProfileFlag(args []string, name string) string {
	for i, a := range args {
		if a == name && i+1 < len(args) {
			return args[i+1]
		}
		if after, ok := strings.CutPrefix(a, name+"="); ok {
			return after
		}
	}
	return ""
}
