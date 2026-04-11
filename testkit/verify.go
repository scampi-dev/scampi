// SPDX-License-Identifier: GPL-3.0-only

package testkit

import (
	"sort"

	"scampi.dev/scampi/lang/eval"
	"scampi.dev/scampi/target"
)

// VerifyMemTarget walks an `expect = test.ExpectedState{ ... }`
// value tree and runs each entry's matcher against the recorded
// state of a target.MemTarget. Returns every mismatch found.
//
// expect is the StructVal produced by `test.ExpectedState{...}` in
// scampi-lang. Its Fields map carries one entry per slot kind
// (files, packages, services, dirs, symlinks); each slot is a
// MapVal of string → Matcher StructVal. Slots that are nil or
// NoneVal are skipped.
//
// The mock argument carries the runtime state recorded during
// engine apply. Slot lookups consult the mock's maps directly.
//
// Mismatches are returned in a stable order: first by slot kind
// (files → packages → services → dirs → symlinks), then by key
// alphabetically. This makes diagnostic output deterministic across
// runs.
func VerifyMemTarget(expect *eval.StructVal, mock *target.MemTarget) []Mismatch {
	if expect == nil || mock == nil {
		return nil
	}
	var out []Mismatch

	out = appendMismatches(out, verifySlot(
		expect,
		"files",
		SlotFileContent,
		func(key string) any { return observeFile(mock, key) },
	))
	out = appendMismatches(out, verifySlot(
		expect,
		"packages",
		SlotPackageStatus,
		func(key string) any { return observePackage(mock, key) },
	))
	out = appendMismatches(out, verifySlot(
		expect,
		"services",
		SlotServiceStatus,
		func(key string) any { return observeService(mock, key) },
	))
	out = appendMismatches(out, verifySlot(
		expect,
		"dirs",
		SlotDirPresence,
		func(key string) any { return observeDir(mock, key) },
	))
	out = appendMismatches(out, verifySlot(
		expect,
		"symlinks",
		SlotSymlinkTarget,
		func(key string) any { return observeSymlink(mock, key) },
	))

	return out
}

// verifySlot looks up the named slot field on the ExpectedState
// struct, walks its map of (key → matcher StructVal) entries in
// sorted order, calls Match for each one, and returns every
// mismatch found.
func verifySlot(
	expect *eval.StructVal,
	field string,
	slot Slot,
	observe func(key string) any,
) []Mismatch {
	raw, ok := expect.Fields[field]
	if !ok {
		return nil
	}
	mp, ok := raw.(*eval.MapVal)
	if !ok {
		// Slot omitted entirely (None) or wrong shape — both
		// mean "no expectations to verify here".
		return nil
	}

	keys := make([]string, 0, len(mp.Keys))
	matchers := make(map[string]*eval.StructVal, len(mp.Keys))
	for i, k := range mp.Keys {
		ks, ok := k.(*eval.StringVal)
		if !ok {
			continue
		}
		mv, ok := mp.Values[i].(*eval.StructVal)
		if !ok {
			continue
		}
		keys = append(keys, ks.V)
		matchers[ks.V] = mv
	}
	sort.Strings(keys)

	var out []Mismatch
	for _, key := range keys {
		if m := Match(matchers[key], slot, key, observe(key)); m != nil {
			out = append(out, *m)
		}
	}
	return out
}

func appendMismatches(dst, src []Mismatch) []Mismatch {
	if len(src) == 0 {
		return dst
	}
	return append(dst, src...)
}

// Slot observers
// -----------------------------------------------------------------------------
//
// These read raw state out of MemTarget's public maps and shape it
// into the form Match expects for the corresponding slot. Returning
// `any` keeps the Match interface uniform.
//
// Locking note: VerifyMemTarget is called after engine.Apply has
// returned, by which point no goroutine is mutating the mock. The
// observers read the public maps directly without holding
// MemTarget's internal mutex (which is unexported anyway). If you
// ever call this concurrently with apply, you're holding it wrong.

func observeFile(mock *target.MemTarget, path string) any {
	data, ok := mock.Files[path]
	if !ok {
		return nil
	}
	return string(data)
}

func observePackage(mock *target.MemTarget, name string) any {
	if mock.Pkgs[name] {
		return PackagePresent
	}
	return PackageAbsent
}

func observeService(mock *target.MemTarget, name string) any {
	// A service entry exists if it's been touched (started, stopped,
	// enabled, disabled, etc.) — return its current observed state.
	// Absent service → return nil so presence matchers can detect it.
	_, knownActive := mock.Services[name]
	_, knownEnabled := mock.EnabledServices[name]
	if !knownActive && !knownEnabled {
		return nil
	}
	if mock.Services[name] {
		return ServiceRunning
	}
	return ServiceStopped
}

func observeDir(mock *target.MemTarget, path string) any {
	_, ok := mock.Dirs[path]
	return ok
}

func observeSymlink(mock *target.MemTarget, path string) any {
	return mock.Symlinks[path]
}
