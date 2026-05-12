// SPDX-License-Identifier: GPL-3.0-only

package lsp

import (
	"context"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
)

func testServer() *Server {
	return &Server{
		catalog:  NewCatalog(),
		modules:  bootstrapModules(),
		stubDefs: NewStubDefs(),
		docs:     NewDocuments(),
		log:      log.New(io.Discard, "", 0),
	}
}

func TestCompletionTopLevel(t *testing.T) {
	s := testServer()
	docURI := protocol.DocumentURI("file:///test.scampi")
	// Top-level completion: modules are offered as namespaces.
	s.docs.Open(docURI, "pos", 1)

	result, err := s.Completion(context.Background(), &protocol.CompletionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
			Position:     protocol.Position{Line: 0, Character: 3},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result == nil || len(result.Items) == 0 {
		t.Fatal("expected completion items for 'pos'")
	}

	found := false
	for _, item := range result.Items {
		if item.Label == "posix" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'posix' module in completion items")
	}
}

func TestCompletionKwargs(t *testing.T) {
	s := testServer()
	docURI := protocol.DocumentURI("file:///test.scampi")
	// Cursor inside a call to posix.copy: posix.copy { src = posix.source_local { path = "./f" }, |
	text := `posix.copy { src = posix.source_local { path = "./f" }, `
	s.docs.Open(docURI, text, 1)

	result, err := s.Completion(context.Background(), &protocol.CompletionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
			Position:     protocol.Position{Line: 0, Character: uint32(len(text))},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result == nil || len(result.Items) == 0 {
		t.Fatal("expected kwarg completions")
	}

	// "src" should be excluded since it's already present.
	for _, item := range result.Items {
		if item.Label == "src" {
			t.Error("src should be excluded from completions (already present)")
		}
	}

	// "dest" should be offered.
	found := false
	for _, item := range result.Items {
		if item.Label == "dest" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'dest' in kwarg completions")
	}
}

func TestCompletionModule(t *testing.T) {
	s := testServer()
	docURI := protocol.DocumentURI("file:///test.scampi")
	s.docs.Open(docURI, "posix.", 1)

	result, err := s.Completion(context.Background(), &protocol.CompletionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
			Position:     protocol.Position{Line: 0, Character: 6},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result == nil || len(result.Items) == 0 {
		t.Fatal("expected module member completions")
	}

	labels := make(map[string]bool)
	for _, item := range result.Items {
		labels[item.Label] = true
	}
	for _, want := range []string{"copy", "dir", "service"} {
		if !labels[want] {
			t.Errorf("missing posix.%s in completions", want)
		}
	}
}

func TestCompletionEnumValues(t *testing.T) {
	s := testServer()
	docURI := protocol.DocumentURI("file:///test.scampi")
	text := `posix.service { name = "nginx", state = "`
	s.docs.Open(docURI, text, 1)

	result, err := s.Completion(context.Background(), &protocol.CompletionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
			Position:     protocol.Position{Line: 0, Character: uint32(len(text))},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result == nil || len(result.Items) == 0 {
		t.Fatal("expected enum value completions")
	}

	labels := make(map[string]bool)
	for _, item := range result.Items {
		labels[item.Label] = true
	}
	for _, want := range []string{"running", "stopped", "restarted", "reloaded"} {
		if !labels[want] {
			t.Errorf("missing enum value: %s", want)
		}
	}
}

func TestCompletionSourceResolvers(t *testing.T) {
	s := testServer()
	docURI := protocol.DocumentURI("file:///test.scampi")
	text := `posix.copy { src = `
	s.docs.Open(docURI, text, 1)

	result, err := s.Completion(context.Background(), &protocol.CompletionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
			Position:     protocol.Position{Line: 0, Character: uint32(len(text))},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result == nil || len(result.Items) == 0 {
		t.Fatal("expected source resolver completions")
	}

	labels := make(map[string]bool)
	for _, item := range result.Items {
		labels[item.Label] = true
	}
	for _, want := range []string{"posix.source_local", "posix.source_inline", "posix.source_remote"} {
		if !labels[want] {
			t.Errorf("missing source resolver: %s", want)
		}
	}
}

func TestCompletionStringKwargOffersEnv(t *testing.T) {
	s := testServer()
	docURI := protocol.DocumentURI("file:///test.scampi")
	text := "ssh.target {\n    name = \"test\",\n    host = "
	s.docs.Open(docURI, text, 1)

	result, err := s.Completion(context.Background(), &protocol.CompletionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
			Position:     protocol.Position{Line: 2, Character: 11},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result == nil || len(result.Items) == 0 {
		t.Fatal("expected completions for string kwarg")
	}

	labels := make(map[string]bool)
	for _, item := range result.Items {
		labels[item.Label] = true
	}
	if !labels["std.env"] {
		t.Error("expected 'std.env' completion for string kwarg")
	}
}

func TestCompletionSecretKeys(t *testing.T) {
	dir := t.TempDir()

	secretsJSON := `{"db.host": "encrypted1", "db.pass": "encrypted2"}`
	if err := os.WriteFile(filepath.Join(dir, "secrets.age.json"), []byte(secretsJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	s := testServer()
	mainPath := filepath.Join(dir, "test.scampi")
	docURI := protocol.DocumentURI(uri.File(mainPath))
	text := "import \"std/secrets\"\nlet resolver = secrets.from_age(path = \"secrets.age.json\")\nresolver.get(\""
	s.docs.Open(docURI, text, 1)

	result, err := s.Completion(context.Background(), &protocol.CompletionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
			Position:     protocol.Position{Line: 2, Character: 14},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result == nil || len(result.Items) == 0 {
		t.Fatal("expected secret key completions")
	}

	labels := make(map[string]bool)
	for _, item := range result.Items {
		labels[item.Label] = true
	}
	if !labels["db.host"] {
		t.Error("missing secret key: db.host")
	}
	if !labels["db.pass"] {
		t.Error("missing secret key: db.pass")
	}
}

func TestCompletionUserDefinedFuncKwargs(t *testing.T) {
	dir := t.TempDir()

	libContent := `module lib

func proxy_host(domain: string, forward_host: string, forward_port: int = 443) string {
  return ""
}
`
	if err := os.WriteFile(filepath.Join(dir, "lib.scampi"), []byte(libContent), 0o644); err != nil {
		t.Fatal(err)
	}

	s := testServer()
	mainPath := filepath.Join(dir, "main.scampi")
	docURI := protocol.DocumentURI(uri.File(mainPath))
	// User-defined function in same file
	text := `
module main

func proxy_host(domain: string, forward_host: string, forward_port: int = 443) string {
  return ""
}

proxy_host()
`
	s.docs.Open(docURI, text, 1)

	// Cursor between the parens on line 6, col 11
	result, err := s.Completion(context.Background(), &protocol.CompletionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
			Position:     protocol.Position{Line: 6, Character: 11},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result == nil || len(result.Items) == 0 {
		t.Fatal("expected kwarg completions for user-defined function")
	}

	labels := make(map[string]bool)
	for _, item := range result.Items {
		labels[item.Label] = true
	}
	for _, want := range []string{"domain", "forward_host", "forward_port"} {
		if !labels[want] {
			t.Errorf("missing kwarg: %s", want)
		}
	}
}

// TestCompletionTopLevelIncludesUserDecls — completion at the start
// of an expression should include user-defined funcs/types/lets from
// the current document, not just stdlib catalog members. This is
// the path that was empty when typing `b` before `b()` in the
// user-reported repro.
func TestCompletionTopLevelIncludesUserDecls(t *testing.T) {
	dir := t.TempDir()
	mainPath := filepath.Join(dir, "main.scampi")

	s := testServer()
	docURI := protocol.DocumentURI(uri.File(mainPath))
	text := `
module main

type X {
  name: string
}

func b() X {
  return X { name = "hello" }
}

func bb() string {
  b
}
`
	s.docs.Open(docURI, text, 1)

	// Cursor right after `b` on line 11 (`  b`)
	result, err := s.Completion(context.Background(), &protocol.CompletionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
			Position:     protocol.Position{Line: 12, Character: 3},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result == nil {
		t.Fatal("nil result")
	}
	labels := make(map[string]bool)
	for _, item := range result.Items {
		labels[item.Label] = true
	}
	if !labels["b"] {
		t.Errorf("expected user-defined func 'b' in top-level completion, got %v", labels)
	}
}

// TestCompletionUFCS — when the user types `var.` where `var` is a
// let-bound variable, completion should offer free functions in
// scope whose first parameter accepts the variable's type. Mirrors
// the lang/check resolution rules in resolveCallee.
func TestCompletionUFCS(t *testing.T) {
	dir := t.TempDir()
	mainPath := filepath.Join(dir, "main.scampi")

	s := testServer()
	docURI := protocol.DocumentURI(uri.File(mainPath))
	text := `
module main

func double(n: int) int {
  return n + n
}

func inc(n: int) int {
  return n + 1
}

func shout(s: string) string {
  return s
}

let n = 5
n.
`
	s.docs.Open(docURI, text, 1)

	// Cursor right after `n.` on the last (line 15) — empty prefix
	// expecting `double` and `inc` (both take int as first arg) but
	// NOT `shout` (takes string).
	result, err := s.Completion(context.Background(), &protocol.CompletionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
			Position:     protocol.Position{Line: 16, Character: 2},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result == nil {
		t.Fatal("expected completion result")
	}

	labels := make(map[string]bool)
	for _, item := range result.Items {
		labels[item.Label] = true
	}
	if !labels["double"] {
		t.Errorf("missing UFCS completion: double")
	}
	if !labels["inc"] {
		t.Errorf("missing UFCS completion: inc")
	}
	if labels["shout"] {
		t.Errorf("shout should NOT be offered (string param, int receiver)")
	}
}

func TestCompletion_UserType_InList_NewLine(t *testing.T) {
	s := testServer()
	docURI := protocol.DocumentURI("file:///test.scampi")

	text := `
module main

type Container {
  id:       int
  hostname: string
}

let items = [
  Container { id = 1, hostname = "a" },
  Container {

  },
]
`
	s.docs.Open(docURI, text, 1)

	items := completionAt(t, s, docURI, 11, 4)
	if len(items) == 0 {
		t.Fatal("expected field completions on new line inside user type in list")
	}
	requireLabels(t, items, "id", "hostname")
}

func TestCompletion_UserType_InList_PartialFields(t *testing.T) {
	s := testServer()
	docURI := protocol.DocumentURI("file:///test.scampi")
	text := `
module main

type Server {
  name: string
  port: int
  tls:  bool = false
}

let servers = [
  Server { name = "web", port = 443 },
  Server { name = "api",
  },
]
`
	s.docs.Open(docURI, text, 1)

	items := completionAt(t, s, docURI, 11, 25)
	if len(items) == 0 {
		t.Fatal("expected completions")
	}
	requireLabels(t, items, "port", "tls")
	rejectLabels(t, items, "name")
}

func completionAt(t *testing.T, s *Server, docURI protocol.DocumentURI, line, col uint32) []protocol.CompletionItem {
	t.Helper()
	result, err := s.Completion(context.Background(), &protocol.CompletionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
			Position:     protocol.Position{Line: line, Character: col},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result == nil {
		return nil
	}
	return result.Items
}

func labels(items []protocol.CompletionItem) map[string]bool {
	m := make(map[string]bool, len(items))
	for _, item := range items {
		m[item.Label] = true
	}
	return m
}

func requireLabels(t *testing.T, items []protocol.CompletionItem, want ...string) {
	t.Helper()
	have := labels(items)
	for _, w := range want {
		if !have[w] {
			var all []string
			for _, item := range items {
				all = append(all, item.Label)
			}
			t.Errorf("expected %q in completions, got %v", w, all)
		}
	}
}

func rejectLabels(t *testing.T, items []protocol.CompletionItem, reject ...string) {
	t.Helper()
	have := labels(items)
	for _, r := range reject {
		if have[r] {
			t.Errorf("%q should NOT be in completions", r)
		}
	}
}

func TestCompletion_UserType_InListAfterExisting(t *testing.T) {
	s := testServer()
	docURI := protocol.DocumentURI("file:///test.scampi")
	text := `
module main

type Item {
  id:   int
  name: string
  tag:  string = "default"
}

let items = [
  Item { id = 1, name = "a" },
  Item { id = 2,
]
`
	s.docs.Open(docURI, text, 1)

	items := completionAt(t, s, docURI, 11, 16)
	if len(items) == 0 {
		t.Fatal("expected field completions")
	}
	requireLabels(t, items, "name", "tag")
	rejectLabels(t, items, "id")
}

func TestCompletion_UserType_InNestedCall(t *testing.T) {
	s := testServer()
	docURI := protocol.DocumentURI("file:///test.scampi")
	text := `
module main
import "std"
import "std/ssh"
import "std/posix"

type Config {
  path: string
  mode: string = "644"
}

let t = ssh.target { name = "web", host = "1.2.3.4", user = "root" }

std.deploy(name = "d", targets = [t]) {
  posix.copy {
    src = posix.source_inline { content = "hello" }
    dest = "/tmp/test"
  }
}
`

	s.docs.Open(docURI, text, 1)
	items := completionAt(t, s, docURI, 16, 4)
	if len(items) == 0 {
		t.Fatal("expected completions inside deploy body")
	}
}

func TestCompletion_UserType_InForLoop(t *testing.T) {
	s := testServer()
	docURI := protocol.DocumentURI("file:///test.scampi")
	text := `
module main

type Box {
  label: string
  color: string
}

func make_boxes() list[Box] {
  return [Box { label = "a", color = "red" }]
}

let boxes = [
  Box { label = "a", color = "red" },
]

func use() string {
  for b in boxes {
    let x = Box {
  }
  return ""
}
`
	s.docs.Open(docURI, text, 1)

	items := completionAt(t, s, docURI, 18, 18)
	if len(items) == 0 {
		t.Fatal("expected field completions for user type in for loop body")
	}
	requireLabels(t, items, "label", "color")
}

func TestCompletion_UserType_WithDefaults(t *testing.T) {
	s := testServer()
	docURI := protocol.DocumentURI("file:///test.scampi")
	text := `
module main

type Entry {
  key:   string
  value: string = ""
  ttl:   int = 300
}

let e = Entry {
`
	s.docs.Open(docURI, text, 1)

	items := completionAt(t, s, docURI, 9, 16)
	if len(items) == 0 {
		t.Fatal("expected completions")
	}
	requireLabels(t, items, "key", "value", "ttl")
}

func TestCompletion_DotAccess_LetBindingPrefix(t *testing.T) {
	s := testServer()
	docURI := protocol.DocumentURI("file:///test.scampi")
	text := `
module main

type Server {
  name: string
  port: int
  tls:  bool = false
}

let srv = Server { name = "web", port = 443 }
let n = srv.na
`
	s.docs.Open(docURI, text, 1)

	items := completionAt(t, s, docURI, 10, 14)
	if len(items) == 0 {
		t.Fatal("expected filtered struct field completions")
	}
	requireLabels(t, items, "name")
	rejectLabels(t, items, "port", "tls")
}

func TestCompletion_DotAccess_NoFieldsOnNonStruct(t *testing.T) {
	s := testServer()
	docURI := protocol.DocumentURI("file:///test.scampi")
	text := `
module main

let x = "hello"
let y = x.
`
	s.docs.Open(docURI, text, 1)

	items := completionAt(t, s, docURI, 4, 10)

	for _, item := range items {
		if item.Kind == protocol.CompletionItemKindField {
			t.Errorf("strings should not have field completions, got %q", item.Label)
		}
	}
}

func TestCompletion_DotAccess_InsideListLiteral(t *testing.T) {
	s := testServer()
	docURI := protocol.DocumentURI("file:///test.scampi")
	text := `
module main

type Cfg {
  host: string
  port: int
}

let c = Cfg { host = "a", port = 1 }
let items = [c.
`
	s.docs.Open(docURI, text, 1)

	items := completionAt(t, s, docURI, 9, 15)
	if len(items) == 0 {
		t.Fatal("expected struct field completions inside list literal")
	}
	requireLabels(t, items, "host", "port")
}

func TestCompletion_DotAccess_FuncParam(t *testing.T) {
	s := testServer()
	docURI := protocol.DocumentURI("file:///test.scampi")
	text := `
module main

type Rec {
  a: int
  b: string
}

func process(r: Rec) string {
  let x = r.
  return ""
}
`
	s.docs.Open(docURI, text, 1)

	items := completionAt(t, s, docURI, 9, 12)
	if len(items) == 0 {
		t.Fatal("expected struct field completions for func param")
	}
	requireLabels(t, items, "a", "b")
}

func TestCompletion_KwargsExclusion_MixedCommasNewlines(t *testing.T) {
	s := testServer()
	docURI := protocol.DocumentURI("file:///test.scampi")
	text := `
posix.copy {
  src = posix.source_local { path = "./f" },
  dest = "/etc/foo"
  owner = "root"

}
`
	s.docs.Open(docURI, text, 1)

	items := completionAt(t, s, docURI, 5, 2)
	if len(items) == 0 {
		t.Fatal("expected completions")
	}
	rejectLabels(t, items, "src", "dest", "owner")
}

func TestCompletion_KwargsExclusion_CursorAtStart(t *testing.T) {
	s := testServer()
	docURI := protocol.DocumentURI("file:///test.scampi")

	text := "posix.copy {\n  \n}"
	s.docs.Open(docURI, text, 1)

	items := completionAt(t, s, docURI, 1, 2)
	if len(items) == 0 {
		t.Fatal("expected all kwargs when none are present")
	}
	requireLabels(t, items, "src", "dest")
}

func TestCompletion_KwargsExclusion_AllPresent(t *testing.T) {
	s := testServer()
	docURI := protocol.DocumentURI("file:///test.scampi")
	text := `
posix.dir {
  path = "/tmp/test"
  state = "present"
  owner = "root"
  group = "root"
  mode = "755"

}
`
	s.docs.Open(docURI, text, 1)

	items := completionAt(t, s, docURI, 7, 2)

	rejectLabels(t, items, "path", "state", "owner", "group", "mode")
}

func TestCompletion_RealWorld_PVEForLoop(t *testing.T) {
	s := testServer()
	docURI := protocol.DocumentURI("file:///test.scampi")
	text := `
module main
import "std"
import "std/ssh"
import "std/pve"

type Container {
  id:       int
  hostname: string
  ip:       string
  size:     string
  cores:    int = 1
  memory:   string = "512M"
}

let midgard = ssh.target { name = "midgard", host = "10.10.2.10", user = "root" }
let debian = pve.Template { storage = "local", name = "debian.tar.zst" }

let containers = [
  Container { id = 999, hostname = "a", ip = "10.0.0.1/24", size = "4G", cores = 2, memory = "1G" },
  Container { id = 998, hostname = "b", ip = "10.0.0.2/24", size = "2G" },
]

std.deploy(name = "pve", targets = [midgard]) {
  for c in containers {
    pve.lxc {
      id       = c.id
      node     = "midgard"
      template = debian
      hostname = c.hostname
      memory   = c.memory
      size     = c.size

    }
  }
}
`
	s.docs.Open(docURI, text, 1)

	items := completionAt(t, s, docURI, 32, 6)
	if len(items) == 0 {
		t.Fatal("expected completions inside pve.lxc")
	}
	rejectLabels(t, items, "id", "node", "template", "hostname", "memory", "size")

	text2 := strings.Replace(text, "      memory   = c.memory", "      memory   = c.", 1)
	s.docs.Open(docURI, text2, 2)

	var dotLine uint32
	for i, line := range strings.Split(text2, "\n") {
		if strings.HasSuffix(strings.TrimSpace(line), "= c.") {
			dotLine = uint32(i)
			break
		}
	}
	if dotLine == 0 {
		t.Fatal("could not find c. line")
	}

	dotCol := uint32(strings.Index(strings.Split(text2, "\n")[dotLine], "c.") + 2)
	items2 := completionAt(t, s, docURI, dotLine, dotCol)
	if len(items2) == 0 {
		t.Fatal("expected struct field completions for 'c.' in for loop")
	}
	requireLabels(t, items2, "id", "hostname", "ip", "size", "cores", "memory")
}

func TestCompletion_DeployBody_TopLevel(t *testing.T) {
	s := testServer()
	docURI := protocol.DocumentURI("file:///test.scampi")
	text := `
module main
import "std"
import "std/ssh"
import "std/posix"

let t = ssh.target { name = "web", host = "1.2.3.4", user = "root" }

std.deploy(name = "d", targets = [t]) {
  pos
}
`
	s.docs.Open(docURI, text, 1)

	items := completionAt(t, s, docURI, 9, 5)
	if len(items) == 0 {
		t.Fatal("expected completions inside deploy body")
	}
	requireLabels(t, items, "posix")
}

func TestCompletion_KwargValue_StdlibConstructor(t *testing.T) {
	s := testServer()
	docURI := protocol.DocumentURI("file:///test.scampi")
	text := "posix.copy {\n  src = \n}"
	s.docs.Open(docURI, text, 1)

	items := completionAt(t, s, docURI, 1, 8)
	if len(items) == 0 {
		t.Fatal("expected constructor completions for src kwarg value")
	}

	found := false
	for _, item := range items {
		if strings.Contains(item.Label, "source") {
			found = true
			break
		}
	}
	if !found {
		var all []string
		for _, item := range items {
			all = append(all, item.Label)
		}
		t.Errorf("expected source_* constructors, got %v", all)
	}
}

func TestCompletion_TopLevel_ModulePrefix(t *testing.T) {
	s := testServer()
	docURI := protocol.DocumentURI("file:///test.scampi")
	s.docs.Open(docURI, "pv", 1)

	items := completionAt(t, s, docURI, 0, 2)
	requireLabels(t, items, "pve")
}

func TestCompletion_ModuleMembers(t *testing.T) {
	s := testServer()
	docURI := protocol.DocumentURI("file:///test.scampi")
	s.docs.Open(docURI, "posix.", 1)

	items := completionAt(t, s, docURI, 0, 6)
	requireLabels(t, items, "copy", "dir", "symlink")
}

func TestCompletion_ModuleMembersFiltered(t *testing.T) {
	s := testServer()
	docURI := protocol.DocumentURI("file:///test.scampi")
	s.docs.Open(docURI, "posix.co", 1)

	items := completionAt(t, s, docURI, 0, 8)
	requireLabels(t, items, "copy")
	rejectLabels(t, items, "dir", "symlink")
}

func TestCompletion_EmptyDoc(t *testing.T) {
	s := testServer()
	docURI := protocol.DocumentURI("file:///test.scampi")
	s.docs.Open(docURI, "", 1)

	items := completionAt(t, s, docURI, 0, 0)

	if items == nil {
		t.Log("no completions for empty doc (acceptable)")
	}
}

func TestCompletion_NoDocs(t *testing.T) {
	s := testServer()
	docURI := protocol.DocumentURI("file:///nonexistent.scampi")

	result, err := s.Completion(context.Background(), &protocol.CompletionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
			Position:     protocol.Position{Line: 0, Character: 0},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result != nil && len(result.Items) > 0 {
		t.Error("should return nil for unknown document")
	}
}

func TestCompletion_UserFunc_InTopLevel(t *testing.T) {
	s := testServer()
	docURI := protocol.DocumentURI("file:///test.scampi")
	text := `
module main

func proxy_host(domain: string) string {
  return ""
}

pro
`
	s.docs.Open(docURI, text, 1)

	items := completionAt(t, s, docURI, 7, 3)
	requireLabels(t, items, "proxy_host")
}

func TestCompletion_UserLet_InTopLevel(t *testing.T) {
	s := testServer()
	docURI := protocol.DocumentURI("file:///test.scampi")
	text := `
module main

let my_config = "test"

my
`
	s.docs.Open(docURI, text, 1)

	items := completionAt(t, s, docURI, 5, 2)
	requireLabels(t, items, "my_config")
}

func TestCompletion_UserType_InTopLevel(t *testing.T) {
	s := testServer()
	docURI := protocol.DocumentURI("file:///test.scampi")
	text := `
module main

type MyConfig {
  name: string
}

My
`
	s.docs.Open(docURI, text, 1)

	items := completionAt(t, s, docURI, 7, 2)
	requireLabels(t, items, "MyConfig")
}

func TestCompletion_NestedStructLit_InnerKwargs(t *testing.T) {
	s := testServer()
	docURI := protocol.DocumentURI("file:///test.scampi")
	text := `
posix.copy {
  src = posix.source_local {

  }
}
`
	s.docs.Open(docURI, text, 1)

	items := completionAt(t, s, docURI, 3, 4)
	if len(items) == 0 {
		t.Fatal("expected kwargs for source_local")
	}
	requireLabels(t, items, "path")
}

func TestCompletion_NestedStructLit_OuterKwargs(t *testing.T) {
	s := testServer()
	docURI := protocol.DocumentURI("file:///test.scampi")
	text := `
posix.copy {
  src = posix.source_local { path = "./f" }

}
`
	s.docs.Open(docURI, text, 1)

	items := completionAt(t, s, docURI, 3, 2)
	if len(items) == 0 {
		t.Fatal("expected kwargs for posix.copy")
	}
	requireLabels(t, items, "dest")
	rejectLabels(t, items, "src")
}

func TestCompletion_UFCS_InFuncBody(t *testing.T) {
	s := testServer()
	docURI := protocol.DocumentURI("file:///test.scampi")
	text := `
module main
import "std/posix"

type MyTarget {
  path: string
}

func setup(t: MyTarget) string {
  return ""
}

let tgt = MyTarget { path = "/tmp" }
let x = tgt.
`
	s.docs.Open(docURI, text, 1)

	items := completionAt(t, s, docURI, 13, 13)
	if len(items) == 0 {
		t.Fatal("expected completions for tgt.")
	}

	requireLabels(t, items, "path")
}

func TestCompletionKwargsExcludesPresent_RealPVE(t *testing.T) {
	s := testServer()
	docURI := protocol.DocumentURI("file:///test.scampi")

	const src = `module main

import "std"
import "std/ssh"
import "std/secrets"
import "std/pve"

let age = secrets.from_age(path = "secrets.age.json")

let midgard = ssh.target {
  name = "midgard"
  host = "10.10.2.10"
  user = "hal9000"
}

type Container {
  id:       int
  hostname: string
  ip:       string
  size:     string
  cores:    int = 1
  memory:   string = "512M"
}

// Shared defaults
// -----------------------------------------------------------------------------

let debian = pve.Template { storage = "local", name = "debian-12-standard_12.12-1_amd64.tar.zst" }
let gw = "10.10.2.1"

let ssh_keys = [
  "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIDUBFaQhgMITOPjYtq6SvDhUzDjLWP2se/nMyQRtQCeF hal9000",
]

// Container definitions
// -----------------------------------------------------------------------------

let containers = [
  Container { id = 999, hostname = "scampi-final", ip = "10.10.2.199/24", size = "4G", cores = 2, memory = "1G" },
  Container { id = 998, hostname = "scampi-test2", ip = "10.10.2.198/24", size = "2G" },
]

// Deploy
// -----------------------------------------------------------------------------

std.deploy(name = "pve", targets = [midgard]) {
  for c in containers {
    pve.lxc {
      id              = c.id

      node            = "midgard"
      template        = debian
      hostname        = c.hostname
      cpu             = pve.Cpu { cores = c.cores }
      memory          = c.memory
      size            = c.size
      features        = pve.Features { nesting = true }
      networks        = [pve.LxcNet { bridge = "vmbr0", ip = c.ip, gw = gw }]
      tags            = ["scampi"]
      ssh_public_keys = ssh_keys
    }
  }

  pve.datacenter {
    console     = pve.Console.xtermjs
    keyboard    = "en-us"
    language    = "en"
    mac_prefix  = "be:ef"
    max_workers = 4
    tags        = [
      pve.Tag { name = "ansible",   fg = "#000000", bg = "#73bbbe" },
      pve.Tag { name = "cac",       fg = "#000000", bg = "#81d983" },
      pve.Tag { name = "manual",    fg = "#ffffff", bg = "#6f74e5" },
      pve.Tag { name = "scampi",    fg = "#ffffff", bg = "#e8722a" },
      pve.Tag { name = "snowflake", fg = "#ffffff", bg = "#d1648a" },
    ]
  }
}
`
	s.docs.Open(docURI, src, 1)

	cursorLine := uint32(0)
	for i, line := range strings.Split(src, "\n") {
		if strings.TrimSpace(line) == "" && i > 0 {
			prev := strings.Split(src, "\n")[i-1]
			if strings.Contains(prev, "id") && strings.Contains(prev, "c.id") {
				cursorLine = uint32(i)
				break
			}
		}
	}
	if cursorLine == 0 {
		t.Fatal("could not find cursor line")
	}
	t.Logf("cursor at line %d", cursorLine)

	result, err := s.Completion(context.Background(), &protocol.CompletionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
			Position:     protocol.Position{Line: cursorLine, Character: 6},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result == nil || len(result.Items) == 0 {
		t.Fatal("expected kwarg completions")
	}

	present := map[string]bool{
		"id": true, "node": true, "template": true,
		"hostname": true, "cpu": true, "memory": true,
		"size": true, "features": true, "networks": true,
		"tags": true, "ssh_public_keys": true,
	}

	for _, item := range result.Items {
		if present[item.Label] {
			t.Errorf("%q should be excluded (already present)", item.Label)
		}
	}

	t.Logf("got %d completion items:", len(result.Items))
	for _, item := range result.Items {
		t.Logf("  %q kind=%v", item.Label, item.Kind)
	}
}

func TestCompletionKwargsExcludesPresent_Newlines(t *testing.T) {
	s := testServer()
	docURI := protocol.DocumentURI("file:///test.scampi")

	text := "posix.copy {\n  src = posix.source_local { path = \"./f\" }\n  dest = \"/etc/foo\"\n  \n}"
	s.docs.Open(docURI, text, 1)

	result, err := s.Completion(context.Background(), &protocol.CompletionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
			Position:     protocol.Position{Line: 3, Character: 2},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result == nil || len(result.Items) == 0 {
		t.Fatal("expected kwarg completions")
	}

	for _, item := range result.Items {
		if item.Label == "src" {
			t.Error("src should be excluded (already present)")
		}
		if item.Label == "dest" {
			t.Error("dest should be excluded (already present)")
		}
	}
}

func TestCompletionKwargsExcludesPresent_Commas(t *testing.T) {
	s := testServer()
	docURI := protocol.DocumentURI("file:///test.scampi")

	text := "posix.copy {\n  src = posix.source_local { path = \"./f\" },\n  dest = \"/etc/foo\",\n  \n}"
	s.docs.Open(docURI, text, 1)

	result, err := s.Completion(context.Background(), &protocol.CompletionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
			Position:     protocol.Position{Line: 3, Character: 2},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result == nil || len(result.Items) == 0 {
		t.Fatal("expected kwarg completions")
	}

	for _, item := range result.Items {
		if item.Label == "src" {
			t.Error("src should be excluded (already present)")
		}
		if item.Label == "dest" {
			t.Error("dest should be excluded (already present)")
		}
	}
}

func TestCompletionKwargsNestedFieldsNotLeaked(t *testing.T) {
	s := testServer()
	docURI := protocol.DocumentURI("file:///test.scampi")

	text := "posix.copy {\n  src = posix.source_local { path = \"./f\" }\n  \n}"
	s.docs.Open(docURI, text, 1)

	result, err := s.Completion(context.Background(), &protocol.CompletionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
			Position:     protocol.Position{Line: 2, Character: 2},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result == nil || len(result.Items) == 0 {
		t.Fatal("expected kwarg completions")
	}

	found := false
	for _, item := range result.Items {
		if item.Label == "dest" {
			found = true
		}

	}
	if !found {
		t.Error("expected 'dest' in completions (not yet present)")
	}

	for _, item := range result.Items {
		if item.Label == "src" {
			t.Error("src should be excluded (already present)")
		}
	}
}

func TestCompletion_SuppressedInsideString(t *testing.T) {
	s := testServer()

	src := `module main
let x = "hello"
`
	docURI := protocol.DocumentURI(uri.File("/test/string_suppress.scampi"))
	s.docs.Open(docURI, src, 1)

	result, err := s.Completion(context.Background(), &protocol.CompletionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
			Position:     protocol.Position{Line: 1, Character: 11},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result != nil && len(result.Items) > 0 {
		t.Errorf("expected 0 completions inside bare string, got %d: %v",
			len(result.Items), result.Items)
	}
}

func TestCompletion_NotSuppressedOutsideString(t *testing.T) {
	s := testServer()

	src := "module main\npos"
	docURI := protocol.DocumentURI(uri.File("/test/string_nosuppress.scampi"))
	s.docs.Open(docURI, src, 1)

	result, err := s.Completion(context.Background(), &protocol.CompletionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
			Position:     protocol.Position{Line: 1, Character: 3},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result == nil || len(result.Items) == 0 {
		t.Fatal("expected completions outside string")
	}

	found := false
	for _, item := range result.Items {
		if item.Label == "posix" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'posix' in completions outside string")
	}
}

func TestCompletion_KeywordsOffered(t *testing.T) {
	s := testServer()
	src := "module main\nle"
	docURI := protocol.DocumentURI(uri.File("/test/keyword_complete.scampi"))
	s.docs.Open(docURI, src, 1)

	result, err := s.Completion(context.Background(), &protocol.CompletionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
			Position:     protocol.Position{Line: 1, Character: 2},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result == nil {
		t.Fatal("expected completion result")
	}

	found := false
	for _, item := range result.Items {
		if item.Label == "let" {
			found = true
			if item.Kind != protocol.CompletionItemKindKeyword {
				t.Errorf("expected Keyword kind for 'let', got %v", item.Kind)
			}
			break
		}
	}
	if !found {
		t.Error("expected 'let' keyword in completions")
	}
}

func TestCompletionStructFieldForLoopVar(t *testing.T) {
	s := testServer()
	docURI := protocol.DocumentURI("file:///test.scampi")
	text := `
module main

type Item {
  id:   int
  name: string
}

let items = [
  Item { id = 1, name = "a" },
]

func use(items: list[Item]) string {
  for item in items {
    let x = item.
  }
  return ""
}
`
	s.docs.Open(docURI, text, 1)

	result, err := s.Completion(context.Background(), &protocol.CompletionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
			Position:     protocol.Position{Line: 14, Character: 17},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result == nil || len(result.Items) == 0 {
		t.Fatal("expected struct field completions for 'item.' in for loop")
	}

	labels := make(map[string]bool)
	for _, item := range result.Items {
		labels[item.Label] = true
	}
	if !labels["id"] {
		t.Error("expected 'id' in completions")
	}
	if !labels["name"] {
		t.Error("expected 'name' in completions")
	}
}

func TestCompletionStructFieldKind(t *testing.T) {
	s := testServer()
	docURI := protocol.DocumentURI("file:///test.scampi")
	text := `
module main

type Box {
  label: string
}

let b = Box { label = "x" }
let l = b.
`
	s.docs.Open(docURI, text, 1)

	result, err := s.Completion(context.Background(), &protocol.CompletionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
			Position:     protocol.Position{Line: 8, Character: 10},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result == nil || len(result.Items) == 0 {
		t.Fatal("expected completions")
	}

	for _, item := range result.Items {
		if item.Label == "label" {
			if item.Kind != protocol.CompletionItemKindField {
				t.Errorf("struct field completion should have Field kind, got %v", item.Kind)
			}
			return
		}
	}
	t.Error("'label' not found in completions")
}

func TestCompletionUFCSNestedScope(t *testing.T) {
	dir := t.TempDir()
	mainPath := filepath.Join(dir, "main.scampi")

	s := testServer()
	docURI := protocol.DocumentURI(uri.File(mainPath))
	text := `
module main

type X {
  name: string
}

func a(x: X) string {
  return ""
}

func b() X {
  return X { name = "hello" }
}

func bb() string {
  let x = b()
  x.
}
`
	s.docs.Open(docURI, text, 1)

	result, err := s.Completion(context.Background(), &protocol.CompletionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
			Position:     protocol.Position{Line: 17, Character: 4},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result == nil {
		t.Fatal("nil result")
	}
	labels := make(map[string]bool)
	for _, item := range result.Items {
		labels[item.Label] = true
	}
	t.Logf("got %d items: %v", len(result.Items), labels)
	if !labels["a"] {
		t.Errorf("expected UFCS function 'a' on x: X, got %v", labels)
	}
}
