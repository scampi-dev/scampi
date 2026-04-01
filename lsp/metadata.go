// SPDX-License-Identifier: GPL-3.0-only

package lsp

import (
	"sort"

	"scampi.dev/scampi/engine"
	"scampi.dev/scampi/spec"
)

// BuiltinParam describes one parameter of a builtin function.
type BuiltinParam struct {
	Name     string
	Type     string
	Desc     string
	Default  string
	Required bool
	Examples []string
}

// BuiltinFunc describes a predeclared Starlark builtin available in
// scampi configuration files.
type BuiltinFunc struct {
	Name    string
	Summary string
	Params  []BuiltinParam
	IsStep  bool
}

// Catalog holds all builtin metadata, indexed for fast lookup during
// completion, hover, and signature help.
type Catalog struct {
	funcs   map[string]BuiltinFunc
	names   []string
	modules map[string][]string // "container" → ["instance", "healthcheck.cmd"]
}

func NewCatalog() *Catalog {
	c := &Catalog{
		funcs:   make(map[string]BuiltinFunc),
		modules: make(map[string][]string),
	}

	c.loadStepBuiltins()
	c.loadNonStepBuiltins()
	c.buildIndex()

	return c
}

// Lookup returns the builtin function with the given name, or false.
func (c *Catalog) Lookup(name string) (BuiltinFunc, bool) {
	f, ok := c.funcs[name]
	return f, ok
}

// Names returns all builtin names in sorted order.
func (c *Catalog) Names() []string { return c.names }

// ModuleMembers returns the sub-function names for a dotted module
// (e.g. "container" → ["instance", "healthcheck.cmd"]).
func (c *Catalog) ModuleMembers(module string) []string {
	return c.modules[module]
}

// Modules returns the top-level module names (container, target, rest).
func (c *Catalog) Modules() []string {
	out := make([]string, 0, len(c.modules))
	for k := range c.modules {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Step builtins — metadata from engine registry
// -----------------------------------------------------------------------------

func (c *Catalog) loadStepBuiltins() {
	reg := engine.NewRegistry()
	for _, st := range reg.StepTypes() {
		doc := engine.LoadStepDoc(st.Kind())
		c.funcs[st.Kind()] = stepDocToBuiltin(doc)
	}
}

func stepDocToBuiltin(doc spec.StepDoc) BuiltinFunc {
	params := make([]BuiltinParam, len(doc.Fields))
	for i, f := range doc.Fields {
		params[i] = BuiltinParam{
			Name:     f.Name,
			Type:     f.Type,
			Desc:     f.Desc,
			Default:  f.Default,
			Required: f.Required,
			Examples: f.Examples,
		}
	}

	// Every step accepts on_change — only add if not already present from config.
	has := make(map[string]bool, len(params))
	for _, p := range params {
		has[p.Name] = true
	}
	if !has["on_change"] {
		params = append(params,
			BuiltinParam{Name: "on_change", Type: "string | list", Desc: "Hook ID(s) to trigger on change"},
		)
	}

	return BuiltinFunc{
		Name:    doc.Kind,
		Summary: doc.Summary,
		Params:  params,
		IsStep:  true,
	}
}

// Non-step builtins — manually defined
// -----------------------------------------------------------------------------

func (c *Catalog) loadNonStepBuiltins() {
	for _, b := range nonStepBuiltins() {
		c.funcs[b.Name] = b
	}
}

func nonStepBuiltins() []BuiltinFunc {
	return []BuiltinFunc{
		{
			Name:    "deploy",
			Summary: "Deploy a set of steps to one or more targets",
			Params: []BuiltinParam{
				{Name: "name", Type: "string", Desc: "Deploy block name", Required: true},
				{Name: "targets", Type: "list", Desc: "Target names to deploy to", Required: true},
				{Name: "steps", Type: "list", Desc: "Steps to execute", Required: true},
				{Name: "hooks", Type: "struct", Desc: "Hook ID to step mapping"},
			},
		},

		// Source resolvers
		{
			Name:    "local",
			Summary: "Reference a local file as a source",
			Params: []BuiltinParam{
				{Name: "path", Type: "string", Desc: "Path relative to config directory", Required: true},
			},
		},
		{
			Name:    "inline",
			Summary: "Use an inline string as a source",
			Params: []BuiltinParam{
				{Name: "content", Type: "string", Desc: "Inline content", Required: true},
			},
		},
		{
			Name:    "remote",
			Summary: "Download a remote file as a source",
			Params: []BuiltinParam{
				{Name: "url", Type: "string", Desc: "URL to download", Required: true},
				{Name: "checksum", Type: "string", Desc: "Expected checksum (algo:hex)"},
			},
		},

		// Package sources
		{
			Name:    "system",
			Summary: "Use the system package manager",
		},
		{
			Name:    "apt_repo",
			Summary: "Add an APT repository as a package source",
			Params: []BuiltinParam{
				{Name: "url", Type: "string", Desc: "Repository URL", Required: true},
				{Name: "key_url", Type: "string", Desc: "GPG key URL", Required: true},
				{Name: "components", Type: "list", Desc: "Repository components", Default: `"[\"main\"]"`},
				{Name: "suite", Type: "string", Desc: "Distribution suite"},
			},
		},
		{
			Name:    "dnf_repo",
			Summary: "Add a DNF repository as a package source",
			Params: []BuiltinParam{
				{Name: "url", Type: "string", Desc: "Repository base URL", Required: true},
				{Name: "key_url", Type: "string", Desc: "GPG key URL", Required: true},
			},
		},

		// Utilities
		{
			Name:    "ref",
			Summary: "Reference output from another step",
			Params: []BuiltinParam{
				{Name: "step", Type: "step", Desc: "Step to reference", Required: true},
				{Name: "field", Type: "string", Desc: "jq expression to extract", Required: true},
			},
		},
		{
			Name:    "env",
			Summary: "Read an environment variable",
			Params: []BuiltinParam{
				{Name: "key", Type: "string", Desc: "Environment variable name", Required: true},
				{Name: "default", Type: "string", Desc: "Default value if unset"},
			},
		},
		{
			Name:    "secret",
			Summary: "Read a secret value from the configured backend",
			Params: []BuiltinParam{
				{Name: "key", Type: "string", Desc: "Secret key name", Required: true},
			},
		},
		{
			Name:    "secrets",
			Summary: "Configure the secrets backend",
			Params: []BuiltinParam{
				{Name: "backend", Type: "string", Desc: "Backend type (age, file)", Required: true},
				{Name: "path", Type: "string", Desc: "Path to secrets file"},
				{Name: "recipients", Type: "list", Desc: "Age recipient public keys"},
			},
		},

		// Target constructors
		{
			Name:    "target.local",
			Summary: "Define a local execution target",
			Params: []BuiltinParam{
				{Name: "name", Type: "string", Desc: "Target name", Required: true},
			},
		},
		{
			Name:    "target.ssh",
			Summary: "Define an SSH execution target",
			Params: []BuiltinParam{
				{Name: "name", Type: "string", Desc: "Target name", Required: true},
				{Name: "host", Type: "string", Desc: "SSH hostname", Required: true},
				{Name: "user", Type: "string", Desc: "SSH username", Required: true},
				{Name: "port", Type: "int", Desc: "SSH port", Default: "22"},
				{Name: "key", Type: "string", Desc: "Path to SSH private key"},
				{Name: "insecure", Type: "bool", Desc: "Skip host key verification"},
				{Name: "timeout", Type: "string", Desc: "Connection timeout"},
			},
		},
		{
			Name:    "target.rest",
			Summary: "Define a REST API execution target",
			Params: []BuiltinParam{
				{Name: "name", Type: "string", Desc: "Target name", Required: true},
				{Name: "base_url", Type: "string", Desc: "Base URL for API requests", Required: true},
				{Name: "auth", Type: "struct", Desc: "Auth config (no_auth, basic, bearer, header)"},
				{Name: "tls", Type: "struct", Desc: "TLS config (secure, insecure, ca_cert)"},
			},
		},

		// REST composables
		{Name: "rest.no_auth", Summary: "No authentication"},
		{
			Name:    "rest.basic",
			Summary: "HTTP Basic authentication",
			Params: []BuiltinParam{
				{Name: "username", Type: "string", Desc: "Username", Required: true},
				{Name: "password", Type: "string", Desc: "Password", Required: true},
			},
		},
		{
			Name:    "rest.bearer",
			Summary: "Bearer token authentication",
			Params: []BuiltinParam{
				{Name: "token", Type: "string", Desc: "Bearer token"},
				{Name: "token_endpoint", Type: "string", Desc: "OAuth2 token endpoint"},
				{Name: "client_id", Type: "string", Desc: "OAuth2 client ID"},
				{Name: "client_secret", Type: "string", Desc: "OAuth2 client secret"},
			},
		},
		{
			Name:    "rest.header",
			Summary: "Custom header authentication",
			Params: []BuiltinParam{
				{Name: "name", Type: "string", Desc: "Header name", Required: true},
				{Name: "value", Type: "string", Desc: "Header value", Required: true},
			},
		},
		{
			Name:    "rest.status",
			Summary: "Check response status code",
			Params: []BuiltinParam{
				{Name: "code", Type: "int", Desc: "Expected HTTP status code", Required: true},
			},
		},
		{
			Name:    "rest.jq",
			Summary: "Check response body with a jq expression",
			Params: []BuiltinParam{
				{Name: "expr", Type: "string", Desc: "jq expression", Required: true},
				{Name: "expected", Type: "string", Desc: "Expected value", Required: true},
			},
		},
		{Name: "rest.tls.secure", Summary: "Use system CA trust store (default)"},
		{Name: "rest.tls.insecure", Summary: "Skip TLS certificate verification"},
		{
			Name:    "rest.tls.ca_cert",
			Summary: "Use a custom CA certificate",
			Params: []BuiltinParam{
				{Name: "path", Type: "string", Desc: "Path to CA certificate file", Required: true},
			},
		},
		{
			Name:    "rest.body.json",
			Summary: "Set JSON request body",
			Params: []BuiltinParam{
				{Name: "data", Type: "struct", Desc: "JSON-serializable data", Required: true},
			},
		},
		{
			Name:    "rest.body.string",
			Summary: "Set raw string request body",
			Params: []BuiltinParam{
				{Name: "content", Type: "string", Desc: "Raw body content", Required: true},
				{Name: "content_type", Type: "string", Desc: "Content-Type header value"},
			},
		},

		// container namespace
		{
			Name:    "container.healthcheck.cmd",
			Summary: "Container health check command",
			Params: []BuiltinParam{
				{Name: "cmd", Type: "list", Desc: "Health check command", Required: true},
				{Name: "interval", Type: "string", Desc: "Check interval", Default: `"30s"`},
				{Name: "timeout", Type: "string", Desc: "Check timeout", Default: `"30s"`},
				{Name: "retries", Type: "int", Desc: "Failure threshold", Default: "3"},
				{Name: "start_period", Type: "string", Desc: "Grace period before checks start", Default: `"0s"`},
			},
		},

		// Test builtins (available in *_test.scampi files)
		{
			Name:    "test.target.in_memory",
			Summary: "Create an in-memory target for testing POSIX steps",
			Params: []BuiltinParam{
				{Name: "name", Type: "string", Desc: "Target name", Required: true},
				{Name: "files", Type: "struct", Desc: "Pre-populated files (path → content)"},
				{Name: "packages", Type: "list", Desc: "Pre-installed packages"},
				{Name: "services", Type: "struct", Desc: "Service states (name → running/stopped)"},
				{Name: "dirs", Type: "list", Desc: "Pre-existing directories"},
			},
		},
		{
			Name:    "test.target.rest_mock",
			Summary: "Create a mock REST target for testing REST steps",
			Params: []BuiltinParam{
				{Name: "name", Type: "string", Desc: "Target name", Required: true},
				{Name: "routes", Type: "struct", Desc: "Route responses (\"METHOD /path\" → test.response())"},
			},
		},
		{
			Name:    "test.response",
			Summary: "Define a mock HTTP response for test.target.rest_mock routes",
			Params: []BuiltinParam{
				{Name: "status", Type: "int", Desc: "HTTP status code", Required: true},
				{Name: "body", Type: "string", Desc: "Response body"},
				{Name: "headers", Type: "struct", Desc: "Response headers"},
			},
		},
		{
			Name:    "test.assert.that",
			Summary: "Create an assertion builder for a test target",
			Params: []BuiltinParam{
				{Name: "target", Type: "test_target", Desc: "Test target to assert against", Required: true},
			},
		},
	}
}

// Index building
// -----------------------------------------------------------------------------

func (c *Catalog) buildIndex() {
	c.names = make([]string, 0, len(c.funcs))
	for name := range c.funcs {
		c.names = append(c.names, name)
	}
	sort.Strings(c.names)

	// Build module membership from dotted names.
	for _, name := range c.names {
		parts := splitDot(name)
		if len(parts) < 2 {
			continue
		}
		mod := parts[0]
		member := name[len(mod)+1:] // everything after first dot
		c.modules[mod] = append(c.modules[mod], member)
	}
}

func splitDot(s string) []string {
	var parts []string
	start := 0
	for i := range len(s) {
		if s[i] == '.' {
			parts = append(parts, s[start:i])
			start = i + 1
		}
	}
	parts = append(parts, s[start:])
	return parts
}
