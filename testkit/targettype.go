// SPDX-License-Identifier: GPL-3.0-only

package testkit

import (
	"context"

	"scampi.dev/scampi/lang/eval"
	"scampi.dev/scampi/source"
	"scampi.dev/scampi/spec"
	"scampi.dev/scampi/target"
)

// MemTargetConfig is the linker-mapped config for a
// `test.target_in_memory(...)` call. The Initial and Expect fields
// hold the raw eval StructVals so the constructor can interpret them
// against runtime mock state — they're declared as `any` so the
// linker's reflection-based mapper stuffs them in untouched (see
// linker/fields.go:setStructVal).
type MemTargetConfig struct {
	Name    string
	Initial any
	Expect  any
}

// MemTargetType is the spec.TargetType implementation for
// `test.target_in_memory(...)`. Each Create call constructs a fresh
// target.MemTarget seeded from the `initial` field, registers it
// with the bound TestRegistry alongside the `expect` field, and
// returns the mock to the engine.
type MemTargetType struct {
	Registry *TestRegistry
}

// Kind returns the dotted form so the linker's QualName lookup
// matches `test.target_in_memory` directly.
func (MemTargetType) Kind() string   { return "test.target_in_memory" }
func (MemTargetType) NewConfig() any { return &MemTargetConfig{} }

func (t MemTargetType) Create(
	_ context.Context,
	_ source.Source,
	tgt spec.TargetInstance,
) (target.Target, error) {
	cfg, ok := tgt.Config.(*MemTargetConfig)
	if !ok {
		return nil, &TestSetupError{
			Reason: "MemTargetType.Create: wrong config type",
		}
	}

	mock := target.NewMemTarget()
	if initial, ok := cfg.Initial.(*eval.StructVal); ok {
		seedMemTarget(mock, initial)
	}

	if t.Registry != nil {
		entry := MemTargetEntry{
			Name: cfg.Name,
			Mock: mock,
		}
		if expect, ok := cfg.Expect.(*eval.StructVal); ok {
			entry.Expect = expect
		}
		t.Registry.AddMemTarget(entry)
	}
	return mock, nil
}

// MemRESTTargetConfig is the linker-mapped config for a
// `test.target_rest_mock(...)` call.
type MemRESTTargetConfig struct {
	Name           string
	BaseURL        string
	Routes         any // *eval.MapVal of "METHOD /path" → response StructVal
	ExpectRequests any // *eval.ListVal of request matcher StructVals
}

// MemRESTTargetType is the spec.TargetType implementation for
// `test.target_rest_mock(...)`.
type MemRESTTargetType struct {
	Registry *TestRegistry
}

func (MemRESTTargetType) Kind() string   { return "test.target_rest_mock" }
func (MemRESTTargetType) NewConfig() any { return &MemRESTTargetConfig{} }

func (t MemRESTTargetType) Create(
	_ context.Context,
	_ source.Source,
	tgt spec.TargetInstance,
) (target.Target, error) {
	cfg, ok := tgt.Config.(*MemRESTTargetConfig)
	if !ok {
		return nil, &TestSetupError{
			Reason: "MemRESTTargetType.Create: wrong config type",
		}
	}

	routes := make(map[string]target.MemRESTResponse)
	if mp, ok := cfg.Routes.(*eval.MapVal); ok {
		for i, k := range mp.Keys {
			ks, ok := k.(*eval.StringVal)
			if !ok {
				continue
			}
			respSV, ok := mp.Values[i].(*eval.StructVal)
			if !ok {
				continue
			}
			routes[ks.V] = buildResponse(respSV)
		}
	}
	mock := target.NewMemREST(routes)

	if t.Registry != nil {
		entry := MemRESTEntry{
			Name: cfg.Name,
			Mock: mock,
		}
		if list, ok := cfg.ExpectRequests.(*eval.ListVal); ok {
			entry.ExpectRequests = list
		}
		t.Registry.AddMemREST(entry)
	}
	return mock, nil
}

// Seeding
// -----------------------------------------------------------------------------

// seedMemTarget reads the `initial` StructVal from a
// test.target_in_memory call and pre-populates the mock's state
// maps. Slot fields that are absent or the wrong shape are skipped
// — type-checking has already validated structure, so this is just
// a lenient runtime walk.
func seedMemTarget(mock *target.MemTarget, initial *eval.StructVal) {
	if initial == nil {
		return
	}
	seedFiles(mock, initial.Fields["files"])
	seedPackages(mock, initial.Fields["packages"])
	seedServices(mock, initial.Fields["services"])
	seedDirs(mock, initial.Fields["dirs"])
	seedSymlinks(mock, initial.Fields["symlinks"])
}

func seedFiles(mock *target.MemTarget, raw eval.Value) {
	mp, ok := raw.(*eval.MapVal)
	if !ok {
		return
	}
	for i, k := range mp.Keys {
		path, ok := k.(*eval.StringVal)
		if !ok {
			continue
		}
		// File values are source composables — for the simple
		// `posix.source_inline { content = "..." }` case the
		// content is a literal string we can drop straight into
		// the mock's Files map. Other source kinds are seeded as
		// empty placeholders for now (Phase 5 follow-up).
		src, ok := mp.Values[i].(*eval.StructVal)
		if !ok {
			continue
		}
		mock.Files[path.V] = []byte(extractInlineContent(src))
		mock.Modes[path.V] = 0o644
		mock.Owners[path.V] = target.Owner{User: "testuser", Group: "testgroup"}
	}
}

func seedPackages(mock *target.MemTarget, raw eval.Value) {
	list, ok := raw.(*eval.ListVal)
	if !ok {
		return
	}
	for _, item := range list.Items {
		s, ok := item.(*eval.StringVal)
		if !ok {
			continue
		}
		mock.Pkgs[s.V] = true
	}
}

func seedServices(mock *target.MemTarget, raw eval.Value) {
	mp, ok := raw.(*eval.MapVal)
	if !ok {
		return
	}
	for i, k := range mp.Keys {
		name, ok := k.(*eval.StringVal)
		if !ok {
			continue
		}
		state, ok := mp.Values[i].(*eval.StringVal)
		if !ok {
			continue
		}
		// posix.ServiceState variants — running and "transient"
		// states (restarted, reloaded) all imply "running" at
		// seed time. stopped is the only false case.
		mock.Services[name.V] = state.V != "stopped"
		mock.EnabledServices[name.V] = mock.Services[name.V]
	}
}

func seedDirs(mock *target.MemTarget, raw eval.Value) {
	list, ok := raw.(*eval.ListVal)
	if !ok {
		return
	}
	for _, item := range list.Items {
		s, ok := item.(*eval.StringVal)
		if !ok {
			continue
		}
		mock.Dirs[s.V] = 0o755
	}
}

func seedSymlinks(mock *target.MemTarget, raw eval.Value) {
	mp, ok := raw.(*eval.MapVal)
	if !ok {
		return
	}
	for i, k := range mp.Keys {
		link, ok := k.(*eval.StringVal)
		if !ok {
			continue
		}
		dest, ok := mp.Values[i].(*eval.StringVal)
		if !ok {
			continue
		}
		mock.Symlinks[link.V] = dest.V
	}
}

// Helpers
// -----------------------------------------------------------------------------

func extractInlineContent(sv *eval.StructVal) string {
	if sv == nil {
		return ""
	}
	if c, ok := sv.Fields["content"].(*eval.StringVal); ok {
		return c.V
	}
	return ""
}

func buildResponse(sv *eval.StructVal) target.MemRESTResponse {
	r := target.MemRESTResponse{}
	if status, ok := sv.Fields["status"].(*eval.IntVal); ok {
		r.StatusCode = int(status.V)
	}
	if body, ok := sv.Fields["body"].(*eval.StringVal); ok {
		r.Body = []byte(body.V)
	}
	if headers, ok := sv.Fields["headers"].(*eval.MapVal); ok {
		r.Headers = make(map[string][]string, len(headers.Keys))
		for i, k := range headers.Keys {
			ks, ok := k.(*eval.StringVal)
			if !ok {
				continue
			}
			vs, ok := headers.Values[i].(*eval.StringVal)
			if !ok {
				continue
			}
			r.Headers[ks.V] = []string{vs.V}
		}
	}
	return r
}

// TestSetupError wraps mistakes in test framework wiring (wrong
// config type from the linker, etc.) so they surface as typed
// errors instead of panicking.
type TestSetupError struct {
	Reason string
}

func (e *TestSetupError) Error() string { return "testkit: " + e.Reason }
