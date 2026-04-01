// SPDX-License-Identifier: GPL-3.0-only

package star

import (
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	"scampi.dev/scampi/spec"
	"scampi.dev/scampi/star/testkit"
	"scampi.dev/scampi/target"
)

// testModule builds the `test` namespace for *_test.scampi files.
func testModule(tc *testkit.Collector) *starlarkstruct.Module {
	return &starlarkstruct.Module{
		Name: "test",
		Members: starlark.StringDict{
			"target":   testTargetModule(),
			"assert":   testAssertModule(tc),
			"response": starlark.NewBuiltin("test.response", builtinTestResponse),
		},
	}
}

func testTargetModule() *starlarkstruct.Module {
	return &starlarkstruct.Module{
		Name: "test.target",
		Members: starlark.StringDict{
			"in_memory": starlark.NewBuiltin(
				"test.target.in_memory",
				builtinTestInMemory,
			),
			"rest_mock": starlark.NewBuiltin(
				"test.target.rest_mock",
				builtinTestRESTMock,
			),
		},
	}
}

func testAssertModule(tc *testkit.Collector) *starlarkstruct.Module {
	return &starlarkstruct.Module{
		Name: "test.assert",
		Members: starlark.StringDict{
			"that": starlark.NewBuiltin(
				"test.assert.that",
				builtinTestAssertThat(tc),
			),
		},
	}
}

// StarlarkTestTarget wraps a test target as a Starlark value. It is passed
// to test.assert.that() and registered as a target via the collector.
// Exactly one of Mem or REST is non-nil.
type StarlarkTestTarget struct {
	Name string
	Mem  *target.MemTarget
	REST *testkit.MockREST
}

var _ starlark.Value = (*StarlarkTestTarget)(nil)

func (t *StarlarkTestTarget) String() string        { return "test.target(" + t.Name + ")" }
func (t *StarlarkTestTarget) Type() string          { return "test_target" }
func (t *StarlarkTestTarget) Freeze()               {}
func (t *StarlarkTestTarget) Truth() starlark.Bool  { return starlark.True }
func (t *StarlarkTestTarget) Hash() (uint32, error) { return 0, nil }

// test.target.in_memory
// -----------------------------------------------------------------------------

func builtinTestInMemory(
	thread *starlark.Thread,
	_ *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var (
		name     string
		files    *starlark.Dict
		packages *starlark.List
		services *starlark.Dict
		dirs     *starlark.List
	)

	if err := starlark.UnpackArgs("test.target.in_memory", args, kwargs,
		"name", &name,
		"files?", &files,
		"packages?", &packages,
		"services?", &services,
		"dirs?", &dirs,
	); err != nil {
		return nil, err
	}

	span := callSpan(thread)

	if name == "" {
		return nil, &EmptyNameError{
			Func:   "test.target.in_memory",
			Source: span,
		}
	}

	fileMap := map[string]string{}
	if files != nil {
		for _, item := range files.Items() {
			k, _ := starlark.AsString(item[0])
			v, _ := starlark.AsString(item[1])
			fileMap[k] = v
		}
	}

	pkgList := []string{}
	if packages != nil {
		iter := packages.Iterate()
		defer iter.Done()
		var val starlark.Value
		for iter.Next(&val) {
			s, _ := starlark.AsString(val)
			pkgList = append(pkgList, s)
		}
	}

	svcMap := map[string]string{}
	if services != nil {
		for _, item := range services.Items() {
			k, _ := starlark.AsString(item[0])
			v, _ := starlark.AsString(item[1])
			svcMap[k] = v
		}
	}

	dirList := []string{}
	if dirs != nil {
		iter := dirs.Iterate()
		defer iter.Done()
		var val starlark.Value
		for iter.Next(&val) {
			s, _ := starlark.AsString(val)
			dirList = append(dirList, s)
		}
	}

	mem := testkit.BuildMemTarget(fileMap, pkgList, svcMap, dirList)

	inst := spec.TargetInstance{
		Type:   testkit.InMemoryTargetType{Tgt: mem},
		Source: span,
		Fields: make(map[string]spec.FieldSpan),
	}

	c := threadCollector(thread)
	if err := c.AddTarget(name, inst, span); err != nil {
		return nil, err
	}

	return &StarlarkTestTarget{Name: name, Mem: mem}, nil
}

// test.assert.that
// -----------------------------------------------------------------------------

func builtinTestAssertThat(tc *testkit.Collector) func(
	*starlark.Thread, *starlark.Builtin, starlark.Tuple, []starlark.Tuple,
) (starlark.Value, error) {
	return func(
		_ *starlark.Thread,
		_ *starlark.Builtin,
		args starlark.Tuple,
		kwargs []starlark.Tuple,
	) (starlark.Value, error) {
		var tgt *StarlarkTestTarget
		if err := starlark.UnpackPositionalArgs(
			"test.assert.that",
			args,
			kwargs,
			1,
			&tgt,
		); err != nil {
			return nil, err
		}
		if tgt.REST != nil {
			return testkit.NewAssertionBuilder(tgt.REST, tc), nil
		}
		return testkit.NewAssertionBuilder(tgt.Mem, tc), nil
	}
}

// test.target.rest_mock
// -----------------------------------------------------------------------------

// starlarkResponse wraps a MockResponse as a Starlark value.
type starlarkResponse struct {
	resp testkit.MockResponse
}

func (r starlarkResponse) String() string        { return "test.response" }
func (r starlarkResponse) Type() string          { return "test_response" }
func (r starlarkResponse) Freeze()               {}
func (r starlarkResponse) Truth() starlark.Bool  { return starlark.True }
func (r starlarkResponse) Hash() (uint32, error) { return 0, nil }

func builtinTestResponse(
	_ *starlark.Thread,
	_ *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var (
		status  int
		body    string
		headers *starlark.Dict
	)
	if err := starlark.UnpackArgs("test.response", args, kwargs,
		"status", &status,
		"body?", &body,
		"headers?", &headers,
	); err != nil {
		return nil, err
	}

	resp := testkit.MockResponse{
		StatusCode: status,
		Body:       []byte(body),
	}

	if headers != nil {
		resp.Headers = make(map[string][]string, headers.Len())
		for _, item := range headers.Items() {
			k, _ := starlark.AsString(item[0])
			v, _ := starlark.AsString(item[1])
			resp.Headers[k] = []string{v}
		}
	}

	return starlarkResponse{resp: resp}, nil
}

func builtinTestRESTMock(
	thread *starlark.Thread,
	_ *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var (
		name   string
		routes *starlark.Dict
	)
	if err := starlark.UnpackArgs("test.target.rest_mock", args, kwargs,
		"name", &name,
		"routes?", &routes,
	); err != nil {
		return nil, err
	}

	span := callSpan(thread)

	if name == "" {
		return nil, &EmptyNameError{
			Func:   "test.target.rest_mock",
			Source: span,
		}
	}

	routeMap := make(map[string]testkit.MockResponse)
	if routes != nil {
		for _, item := range routes.Items() {
			key, ok := starlark.AsString(item[0])
			if !ok {
				return nil, &TypeError{
					Context:  "test.target.rest_mock: route key",
					Expected: "string (e.g. \"GET /items\")",
					Got:      item[0].Type(),
					Source:   span,
				}
			}
			resp, ok := item[1].(starlarkResponse)
			if !ok {
				return nil, &TypeError{
					Context:  "test.target.rest_mock: route value",
					Expected: "test.response()",
					Got:      item[1].Type(),
					Source:   span,
				}
			}
			routeMap[key] = resp.resp
		}
	}

	mock := testkit.NewMockREST(routeMap)

	inst := spec.TargetInstance{
		Type:   testkit.RESTMockTargetType{Tgt: mock},
		Source: span,
		Fields: make(map[string]spec.FieldSpan),
	}

	c := threadCollector(thread)
	if err := c.AddTarget(name, inst, span); err != nil {
		return nil, err
	}

	return &StarlarkTestTarget{Name: name, REST: mock}, nil
}
