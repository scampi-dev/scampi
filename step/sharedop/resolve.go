// SPDX-License-Identifier: GPL-3.0-only

package sharedop

import "scampi.dev/scampi/spec"

// DependencyAdder is satisfied by any op embedding BaseOp.
type DependencyAdder interface {
	AddDependency(dep spec.Op)
}

// ResolveSourceOps creates resolution ops for source references that need
// fetching (e.g. remote URLs). It wires the returned ops as dependencies of
// primaryOp and sets their action. Returns nil when no resolution is needed.
func ResolveSourceOps(
	ref spec.SourceRef,
	primaryOp DependencyAdder,
	action spec.Action,
	srcSpan spec.SourceSpan,
) []spec.Op {
	if !ref.NeedsResolution() {
		return nil
	}

	dl := &DownloadOp{
		BaseOp: BaseOp{
			SrcSpan: srcSpan,
		},
		URL:       ref.URL,
		Checksum:  ref.Checksum,
		CachePath: ref.Path,
	}
	dl.SetAction(action)
	primaryOp.AddDependency(dl)
	return []spec.Op{dl}
}

// CheckSourcePending handles the "source not yet available" case in Check for
// ops whose source requires resolution. Returns ok=true when the source is
// pending and the caller should return the provided result and drift early.
func CheckSourcePending(
	ref spec.SourceRef,
	driftField string,
) (spec.CheckResult, []spec.DriftDetail, bool) {
	if !ref.NeedsResolution() {
		return 0, nil, false
	}
	return spec.CheckUnsatisfied, []spec.DriftDetail{{
		Field:   driftField,
		Desired: ref.DisplayPath(),
	}}, true
}
