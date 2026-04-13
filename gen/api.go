// SPDX-License-Identifier: GPL-3.0-only

// Package gen implements code generators for scampi modules.
package gen

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/getkin/kin-openapi/openapi2"
	"github.com/getkin/kin-openapi/openapi2conv"
	"github.com/getkin/kin-openapi/openapi3"
	"gopkg.in/yaml.v3"

	"scampi.dev/scampi/diagnostic"
	"scampi.dev/scampi/diagnostic/event"
	"scampi.dev/scampi/engine"
	"scampi.dev/scampi/errs"
)

// APIOptions configures code generation behavior.
type APIOptions struct {
	PathPrefix string    // prepended to all generated route paths
	NoTest     bool      // skip generating the companion *_test.scampi file
	TestWriter io.Writer // if set, write test output here instead of to disk
}

// API generates a .api.scampi module from an OpenAPI specification file.
// Supports both OpenAPI 3.x and Swagger 2.0 specs. Diagnostics are
// emitted through em; on failure an AbortError is returned.
func API(specPath string, scampiVersion string, w io.Writer, em diagnostic.Emitter, opts APIOptions) error {
	doc, err := loadSpec(specPath)
	if err != nil {
		return emitAndAbort(em, err)
	}

	if verr := doc.Validate(context.Background()); verr != nil {
		emitWarning(em, "spec validation: %v (proceeding anyway)", verr)
	}

	if doc.Info == nil {
		return emitAndAbort(em, errs.WrapErrf(errBadSpec, "spec has no info section"))
	}
	if doc.Paths == nil || doc.Paths.Len() == 0 {
		return emitAndAbort(em, errs.WrapErrf(errBadSpec, "spec has no paths"))
	}

	prefix := cleanPathPrefix(opts.PathPrefix)

	g := &apiGenerator{
		w: w, doc: doc, specPath: specPath,
		scampiVersion: scampiVersion, pathPrefix: prefix,
	}
	if err := g.generate(); err != nil {
		return err
	}
	if !opts.NoTest && opts.TestWriter != nil {
		g.writeTest(opts.TestWriter)
	}
	return nil
}

// bare-error: sentinel for unrecoverable spec issues
var errBadSpec = errs.New("invalid spec")

// Diagnostics
// -----------------------------------------------------------------------------

// GenWarning is a non-fatal diagnostic emitted during code generation.
type GenWarning struct {
	diagnostic.Warning
	Detail string
}

func (w *GenWarning) EventTemplate() event.Template {
	return event.Template{
		ID:   "gen.Warning",
		Text: "{{.Detail}}",
		Data: w,
	}
}

// GenError is a fatal diagnostic emitted when code generation fails.
type GenError struct {
	diagnostic.FatalError
	Detail string
}

func (e *GenError) Error() string { return e.Detail }

func (e *GenError) EventTemplate() event.Template {
	return event.Template{
		ID:   "gen.Error",
		Text: "{{.Detail}}",
		Data: e,
	}
}

func emitWarning(em diagnostic.Emitter, format string, args ...any) {
	em.EmitEngineDiagnostic(diagnostic.RaiseEngineDiagnostic("", &GenWarning{
		Detail: fmt.Sprintf(format, args...),
	}))
}

func emitAndAbort(em diagnostic.Emitter, err error) error {
	diag := &GenError{Detail: err.Error()}
	em.EmitEngineDiagnostic(diagnostic.RaiseEngineDiagnostic("", diag))
	return engine.AbortError{Causes: []error{diag}}
}

// Spec loading
// -----------------------------------------------------------------------------

func loadSpec(specPath string) (*openapi3.T, error) {
	raw, err := os.ReadFile(specPath)
	if err != nil {
		return nil, errs.WrapErrf(err, "reading spec")
	}

	if detectSwagger2(raw) {
		return loadSwagger2(raw)
	}

	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromFile(specPath)
	if err != nil {
		return nil, errs.WrapErrf(err, "loading spec")
	}
	return doc, nil
}

func detectSwagger2(raw []byte) bool {
	var probe struct {
		Swagger string `json:"swagger" yaml:"swagger"`
		OpenAPI string `json:"openapi" yaml:"openapi"`
	}
	if json.Unmarshal(raw, &probe) != nil {
		_ = yaml.Unmarshal(raw, &probe)
	}
	return probe.Swagger != "" && probe.OpenAPI == ""
}

func loadSwagger2(raw []byte) (*openapi3.T, error) {
	var doc2 openapi2.T

	// openapi2.T only supports JSON. If the input is YAML, convert via
	// yaml → any → json round-trip.
	if json.Unmarshal(raw, &doc2) != nil {
		var intermediate any
		if err := yaml.Unmarshal(raw, &intermediate); err != nil {
			return nil, errs.WrapErrf(err, "parsing swagger 2.0 yaml")
		}
		jsonBytes, err := json.Marshal(intermediate)
		if err != nil {
			return nil, errs.WrapErrf(err, "converting swagger 2.0 yaml to json")
		}
		if err := json.Unmarshal(jsonBytes, &doc2); err != nil {
			return nil, errs.WrapErrf(err, "parsing swagger 2.0 spec")
		}
	}

	doc3, err := openapi2conv.ToV3(&doc2)
	if err != nil {
		return nil, errs.WrapErrf(err, "converting swagger 2.0 to openapi 3")
	}
	return doc3, nil
}

// Code generation
// -----------------------------------------------------------------------------

// testOp captures enough info about a generated function to emit a
// companion test call: the function name, HTTP method/path, sample
// path param values, and whether a body param exists (for body
// assertion in the test's expect_requests).
type testOp struct {
	funcName       string
	method         string
	fullPath       string
	pathParams     []string
	hasSampleBody  bool
	sampleBodyName string
}

type apiGenerator struct {
	w             io.Writer
	doc           *openapi3.T
	specPath      string
	scampiVersion string
	pathPrefix    string
	ops           []testOp // collected during generation for test emission
}

func (g *apiGenerator) generate() error {
	g.header()

	groups := groupByPrefix(sortedPaths(g.doc.Paths))

	for i, group := range groups {
		if i > 0 {
			g.line("")
		}
		g.line("")
		g.line("// %s", group.title)
		g.line("// -----------------------------------------------------------------------------")

		for _, path := range group.paths {
			item := g.doc.Paths.Find(path)
			for _, m := range methods(item) {
				if m.op == nil {
					continue
				}
				g.line("")
				g.writeOperation(path, m.name, m.op)
			}
		}
	}

	return nil
}

func (g *apiGenerator) header() {
	g.line(
		"// Generated from %s by scampi gen api (%s)",
		filepath.Base(g.specPath),
		g.scampiVersion,
	)
	g.line("//")
	g.line("// %s %s", g.doc.Info.Title, g.doc.Info.Version)
	g.line("//")
	g.line("// This file was mechanically generated from an OpenAPI specification.")
	g.line("// It is provided as-is with no warranty. Scampi's license does not")
	g.line("// apply to generated output. If the source specification carries its")
	g.line("// own license terms, those terms govern this file.")
	modName := strings.TrimSuffix(filepath.Base(g.specPath), filepath.Ext(g.specPath))
	modName = sanitizePath(modName)
	g.line("")
	g.line("module %s", modName)
	g.line("")
	g.line(`import "std/rest"`)
}

func (g *apiGenerator) writeOperation(path, method string, op *openapi3.Operation) {
	funcName := toSnakeCase(op.OperationID)
	if funcName == "" {
		funcName = strings.ToLower(method) + "_" + sanitizePath(path)
	}

	params := buildParams(op, method)

	summary := op.Summary
	if summary == "" {
		summary = method + " " + path
	}
	g.line("// %s %s — %s", method, path, summary)

	g.writeFuncSignature(funcName, params.allTyped())

	fullPath := g.pathPrefix + path
	pathExpr := interpolatePathParams(fullPath, params.pathParams)

	// Track for test generation.
	top := testOp{
		funcName:   funcName,
		method:     method,
		fullPath:   fullPath,
		pathParams: params.pathParams,
	}
	if body := params.allBody(); len(body) > 0 {
		top.hasSampleBody = true
		top.sampleBodyName = body[0].apiName
	}
	g.ops = append(g.ops, top)

	if params.hasBody() {
		g.line("  let body = {}")
		for _, f := range params.allBody() {
			g.line("  if %s != none {", f.paramName)
			g.line("    body[%q] = %s", f.apiName, f.paramName)
			g.line("  }")
		}
		g.line("  return rest.request {")
		g.line("    method = %q", method)
		g.line("    path   = %s", pathExpr)
		g.line("    body   = rest.body_json { data = body }")
		if method == "GET" {
			g.line("    check  = check")
		}
		g.line("  }")
	} else {
		g.line("  return rest.request {")
		g.line("    method = %q", method)
		g.line("    path   = %s", pathExpr)
		if method == "GET" {
			g.line("    check  = check")
		}
		g.line("  }")
	}
	g.line("}")
}

func (g *apiGenerator) writeFuncSignature(name string, params []string) {
	if len(params) <= 2 {
		g.line("func %s(%s) std.Step {", name, strings.Join(params, ", "))
		return
	}
	g.line("func %s(", name)
	for i, p := range params {
		suffix := ","
		if i == len(params)-1 {
			suffix = ""
		}
		g.line("  %s%s", p, suffix)
	}
	g.line(") std.Step {")
}

func (g *apiGenerator) line(format string, args ...any) {
	_, _ = fmt.Fprintf(g.w, format+"\n", args...)
}

// Parameter building
// -----------------------------------------------------------------------------

type field struct {
	apiName   string // name in the API schema (dict key)
	paramName string // name in the generated function signature
}

type opParams struct {
	pathParams []string
	required   []field
	optional   []field
	isGET      bool
}

func (p *opParams) hasBody() bool {
	return len(p.required) > 0 || len(p.optional) > 0
}

func (p *opParams) allBody() []field {
	out := make([]field, 0, len(p.required)+len(p.optional))
	out = append(out, p.required...)
	out = append(out, p.optional...)
	return out
}

// allTyped returns the scampi-lang typed param list. Path params are
// required strings; body params are optional strings defaulting to
// none (so the function works as a rest.resource template). GET
// functions get a trailing `check: rest.Check?` parameter.
func (p *opParams) allTyped() []string {
	var out []string
	for _, name := range p.pathParams {
		out = append(out, name+": string")
	}
	for _, f := range p.required {
		out = append(out, f.paramName+": string? = none")
	}
	for _, f := range p.optional {
		out = append(out, f.paramName+": string? = none")
	}
	if p.isGET {
		out = append(out, "check: rest.Check? = none")
	}
	return out
}

func buildParams(op *openapi3.Operation, method string) opParams {
	pathParams := collectPathParams(op)
	required, optional := collectBodyFields(op)

	toFields := func(names []string) []field {
		fields := make([]field, len(names))
		for i, name := range names {
			pn := name
			if slices.Contains(pathParams, name) {
				pn = "body_" + name
			}
			fields[i] = field{apiName: name, paramName: pn}
		}
		return fields
	}

	return opParams{
		pathParams: pathParams,
		required:   toFields(required),
		optional:   toFields(optional),
		isGET:      method == "GET",
	}
}

func collectPathParams(op *openapi3.Operation) []string {
	var params []string
	for _, p := range op.Parameters {
		if p.Value != nil && p.Value.In == "path" {
			params = append(params, p.Value.Name)
		}
	}
	return params
}

func collectBodyFields(op *openapi3.Operation) (required, optional []string) {
	if op.RequestBody == nil || op.RequestBody.Value == nil {
		return nil, nil
	}
	ct := op.RequestBody.Value.Content.Get("application/json")
	if ct == nil || ct.Schema == nil || ct.Schema.Value == nil {
		return nil, nil
	}

	schema := ct.Schema.Value
	reqSet := map[string]bool{}
	for _, r := range schema.Required {
		reqSet[r] = true
	}

	for _, name := range sortedPropertyNames(schema) {
		if reqSet[name] {
			required = append(required, name)
		} else {
			optional = append(optional, name)
		}
	}
	return required, optional
}

// Path grouping
// -----------------------------------------------------------------------------

type pathGroup struct {
	title string
	paths []string
}

func groupByPrefix(paths []string) []pathGroup {
	groups := map[string][]string{}
	var order []string

	for _, p := range paths {
		parts := strings.SplitN(strings.TrimPrefix(p, "/"), "/", 3)
		prefix := parts[0]
		if len(parts) > 1 {
			prefix = parts[0] + "/" + parts[1]
		}
		if _, exists := groups[prefix]; !exists {
			order = append(order, prefix)
		}
		groups[prefix] = append(groups[prefix], p)
	}

	result := make([]pathGroup, len(order))
	for i, prefix := range order {
		result[i] = pathGroup{title: titleFromPrefix(prefix), paths: groups[prefix]}
	}
	return result
}

// Helpers
// -----------------------------------------------------------------------------

func sortedPropertyNames(schema *openapi3.Schema) []string {
	names := make([]string, 0, len(schema.Properties))
	for name := range schema.Properties {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func sortedPaths(paths *openapi3.Paths) []string {
	out := make([]string, 0, paths.Len())
	for path := range paths.Map() {
		out = append(out, path)
	}
	sort.Strings(out)
	return out
}

type methodOp struct {
	name string
	op   *openapi3.Operation
}

func methods(item *openapi3.PathItem) []methodOp {
	return []methodOp{
		{"GET", item.Get},
		{"POST", item.Post},
		{"PUT", item.Put},
		{"PATCH", item.Patch},
		{"DELETE", item.Delete},
	}
}

func titleFromPrefix(prefix string) string {
	parts := strings.Split(prefix, "/")
	last := parts[len(parts)-1]
	last = strings.ReplaceAll(last, "-", " ")
	last = strings.ReplaceAll(last, "_", " ")
	return strings.Title(last) //nolint:staticcheck // simple title case
}

func sanitizePath(path string) string {
	r := strings.NewReplacer("/", "_", "{", "", "}", "", "-", "_")
	return strings.Trim(r.Replace(path), "_")
}

// interpolatePathParams converts a path like "/v1/sites/{siteId}/networks"
// into a scampi expression: "/v1/sites/" + siteId + "/networks".
// If there are no path params, returns the quoted literal.
func interpolatePathParams(path string, params []string) string {
	if len(params) == 0 {
		return fmt.Sprintf("%q", path)
	}

	// Build replacement expression by splitting on each {param}.
	result := path
	for _, p := range params {
		placeholder := "{" + p + "}"
		result = strings.Replace(result, placeholder, "\x00"+p+"\x00", 1)
	}

	// Split on the sentinel and build scampi string concatenation.
	parts := strings.Split(result, "\x00")
	var segments []string
	for i, part := range parts {
		if i%2 == 0 {
			if part != "" {
				segments = append(segments, fmt.Sprintf("%q", part))
			}
		} else {
			segments = append(segments, part)
		}
	}
	return strings.Join(segments, " + ")
}

// cleanPathPrefix normalises a user-supplied prefix: strips redundant
// slashes and ensures the result starts with "/" (or is empty).
func cleanPathPrefix(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	// Collapse runs of slashes, trim trailing slash.
	var b strings.Builder
	prev := byte(0)
	for i := range len(raw) {
		ch := raw[i]
		if ch == '/' && prev == '/' {
			continue
		}
		b.WriteByte(ch)
		prev = ch
	}
	result := strings.TrimRight(b.String(), "/")
	if result == "" {
		return ""
	}
	if result[0] != '/' {
		result = "/" + result
	}
	return result
}

// Test generation
// -----------------------------------------------------------------------------

// writeTest emits a companion *_test.scampi file that exercises every
// generated function against a rest_mock. The test file imports the
// generated module, constructs a mock with canned routes, calls each
// endpoint, and verifies the expected requests were made.
func (g *apiGenerator) writeTest(w io.Writer) {
	t := &testWriter{w: w}
	t.line("// Auto-generated smoke test")
	t.line("// Verifies each endpoint sends the expected method and path.")
	t.line("")
	t.line("module main")
	t.line("")
	t.line("import \"std\"")
	t.line("import \"std/rest\"")
	t.line("import \"std/test\"")
	t.line("import \"std/test/matchers\"")
	t.line("")

	// Build routes + expect_requests from collected ops.
	t.line("let api = test.target_rest_mock(")
	t.line("  name = \"api\",")
	t.line("  base_url = \"http://localhost\",")
	t.line("  routes = {")
	for _, op := range g.ops {
		sample := samplePath(op)
		status := defaultStatus(op.method)
		t.line("    \"%s %s\": test.response(status = %d),", op.method, sample, status)
	}
	t.line("  },")
	t.line("  expect_requests = [")
	for _, op := range g.ops {
		sample := samplePath(op)
		if op.hasSampleBody {
			t.line("    test.request(")
			t.line("      method = \"%s\",", op.method)
			t.line("      path   = \"%s\",", sample)
			t.line("      body   = matchers.has_substring(\"\\\"%s\\\"\"),", op.sampleBodyName)
			t.line("    ),")
		} else {
			t.line("    test.request(method = \"%s\", path = \"%s\"),", op.method, sample)
		}
	}
	t.line("  ],")
	t.line(")")
	t.line("")

	// Deploy block with inline rest.request steps — self-contained,
	// no module import needed. Tests the request shapes directly
	// rather than calling the generated wrapper functions.
	t.line("std.deploy(name = \"smoke\", targets = [api]) {")
	for _, op := range g.ops {
		sample := samplePath(op)
		if op.hasSampleBody {
			t.line("  rest.request {")
			t.line("    method = \"%s\"", op.method)
			t.line("    path   = \"%s\"", sample)
			t.line("    body   = rest.body_json { data = { \"%s\": \"test\" } }", op.sampleBodyName)
			t.line("  }")
		} else {
			t.line("  rest.request {")
			t.line("    method = \"%s\"", op.method)
			t.line("    path   = \"%s\"", sample)
			if op.method == "GET" {
				t.line("    check  = rest.status { code = 200 }")
			}
			t.line("  }")
		}
	}
	t.line("}")
	t.line("")
}

type testWriter struct {
	w io.Writer
}

func (t *testWriter) line(format string, args ...any) {
	_, _ = fmt.Fprintf(t.w, format+"\n", args...)
}

// samplePath replaces path params with sample values for the test
// route and assertion. Uses "1" as the default sample for every
// path param — good enough for smoke tests.
func samplePath(op testOp) string {
	p := op.fullPath
	for _, param := range op.pathParams {
		p = strings.Replace(p, "{"+param+"}", "1", 1)
	}
	return p
}

func defaultStatus(method string) int {
	switch method {
	case "POST":
		return 201
	case "DELETE":
		return 204
	default:
		return 200
	}
}

func toSnakeCase(s string) string {
	if s == "" {
		return ""
	}
	var buf strings.Builder
	for i, r := range s {
		if r >= 'A' && r <= 'Z' {
			if i > 0 {
				buf.WriteByte('_')
			}
			buf.WriteRune(r + ('a' - 'A'))
		} else {
			buf.WriteRune(r)
		}
	}
	return buf.String()
}
