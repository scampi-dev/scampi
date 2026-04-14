// SPDX-License-Identifier: GPL-3.0-only

package lsp

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
)

// writeModuleFiles creates a temp module directory with the given
// files. Each entry is filename → content. Returns the directory path.
func writeModuleFiles(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	return dir
}

func TestMultiFileModule_CheckSiblingDecls(t *testing.T) {
	dir := writeModuleFiles(t, map[string]string{
		"api.scampi": `module mymod
import "std"
import "std/rest"
func list_items(check: rest.Check? = none) std.Step {
  return rest.request {
    method = "GET"
    path   = "/items"
    check  = check
  }
}
`,
		"_index.scampi": `module mymod
import "std"
import "std/rest"
decl item() std.Step {
  return rest.resource {
    query = list_items()
    missing = rest.request {
      method = "POST"
      path   = "/items"
    }
    state = { "name": "test" }
  }
}
`,
	})

	// CheckFile on _index.scampi should see list_items from api.scampi.
	path := filepath.Join(dir, "_index.scampi")
	res := CheckFile(context.Background(), path)
	if res.Panic != "" {
		t.Fatalf("panic: %s", res.Panic)
	}
	if len(res.Diagnostics) > 0 {
		for _, d := range res.Diagnostics {
			t.Errorf("diagnostic: %s", d.Message)
		}
	}
}

func TestMultiFileModule_CheckDoesNotLeakAcrossModules(t *testing.T) {
	dir := writeModuleFiles(t, map[string]string{
		"a.scampi": `module alpha
func helper() int {
  return 1
}
`,
		"b.scampi": `module beta
func other() int {
  return helper()
}
`,
	})

	// b.scampi declares module beta, a.scampi declares module alpha.
	// helper() should NOT be visible from b.scampi — different modules.
	path := filepath.Join(dir, "b.scampi")
	res := CheckFile(context.Background(), path)
	hasUndefined := false
	for _, d := range res.Diagnostics {
		if strings.Contains(d.Message, "undefined") {
			hasUndefined = true
		}
	}
	if !hasUndefined {
		t.Error("expected 'undefined: helper' error — different module names should not share scope")
	}
}

func TestMultiFileModule_GotoDefAcrossSiblings(t *testing.T) {
	dir := writeModuleFiles(t, map[string]string{
		"api.scampi": `module mymod
import "std"
import "std/rest"
func list_items(check: rest.Check? = none) std.Step {
  return rest.request {
    method = "GET"
    path   = "/items"
    check  = check
  }
}
`,
		"_index.scampi": `module mymod
import "std"
import "std/rest"
decl item() std.Step {
  return rest.resource {
    query = list_items()
    missing = rest.request {
      method = "POST"
      path   = "/items"
    }
    state = { "name": "test" }
  }
}
`,
	})

	s := newOneShotServer(filepath.Join(dir, "_index.scampi"))
	indexPath := filepath.Join(dir, "_index.scampi")
	indexData, _ := os.ReadFile(indexPath)
	docURI := protocol.DocumentURI(uri.File(indexPath))
	s.docs.Open(docURI, string(indexData), 1)

	// Line 6: "query = list_items()" — cursor on "list_items"
	// Find the line containing "list_items"
	lines := strings.Split(string(indexData), "\n")
	var defLine, defCol uint32
	for i, line := range lines {
		if idx := strings.Index(line, "list_items"); idx >= 0 {
			defLine = uint32(i)
			defCol = uint32(idx + 1)
			break
		}
	}

	locs, err := s.Definition(context.Background(), &protocol.DefinitionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
			Position:     protocol.Position{Line: defLine, Character: defCol},
		},
	})
	if err != nil {
		t.Fatalf("Definition: %v", err)
	}
	if len(locs) == 0 {
		t.Fatal("expected at least one definition location")
	}
	// Should point to api.scampi, not _index.scampi.
	apiPath := filepath.Join(dir, "api.scampi")
	if !strings.HasSuffix(string(locs[0].URI), "api.scampi") {
		t.Errorf("expected definition in api.scampi, got %s", locs[0].URI)
	}
	_ = apiPath
}

func TestMultiFileModule_FindRefsAcrossSiblings(t *testing.T) {
	dir := writeModuleFiles(t, map[string]string{
		"api.scampi": `module mymod
import "std"
import "std/rest"
func list_items(check: rest.Check? = none) std.Step {
  return rest.request {
    method = "GET"
    path   = "/items"
    check  = check
  }
}
`,
		"_index.scampi": `module mymod
import "std"
import "std/rest"
decl item() std.Step {
  return rest.resource {
    query = list_items()
    missing = rest.request {
      method = "POST"
      path   = "/items"
    }
    state = { "name": "test" }
  }
}
`,
	})

	s := newOneShotServer(filepath.Join(dir, "api.scampi"))
	apiPath := filepath.Join(dir, "api.scampi")
	apiData, _ := os.ReadFile(apiPath)
	docURI := protocol.DocumentURI(uri.File(apiPath))
	s.docs.Open(docURI, string(apiData), 1)

	// Find "list_items" in api.scampi (the func declaration).
	lines := strings.Split(string(apiData), "\n")
	var refLine, refCol uint32
	for i, line := range lines {
		if idx := strings.Index(line, "list_items"); idx >= 0 {
			refLine = uint32(i)
			refCol = uint32(idx + 1)
			break
		}
	}

	locs, err := s.References(context.Background(), &protocol.ReferenceParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
			Position:     protocol.Position{Line: refLine, Character: refCol},
		},
	})
	if err != nil {
		t.Fatalf("References: %v", err)
	}

	// Should find at least 2 locations: the def in api.scampi +
	// the usage in _index.scampi.
	if len(locs) < 2 {
		t.Errorf("expected at least 2 references, got %d", len(locs))
		for _, l := range locs {
			t.Logf("  %s:%d", l.URI, l.Range.Start.Line)
		}
	}

	// At least one location should be in _index.scampi.
	hasIndex := false
	for _, l := range locs {
		if strings.HasSuffix(string(l.URI), "_index.scampi") {
			hasIndex = true
		}
	}
	if !hasIndex {
		t.Error("expected a reference in _index.scampi")
	}
}

func TestMultiFileModule_MainModuleStaysStandalone(t *testing.T) {
	dir := writeModuleFiles(t, map[string]string{
		"deploy.scampi": `module main
import "std"
import "std/posix"
let x = 1
`,
		"other.scampi": `module main
func helper() int { return 2 }
`,
	})

	// module main files are standalone — sibling loading should
	// NOT fire, so helper() from other.scampi is NOT visible.
	path := filepath.Join(dir, "deploy.scampi")
	res := CheckFile(context.Background(), path)
	if res.Panic != "" {
		t.Fatalf("panic: %s", res.Panic)
	}
	// No errors expected — deploy.scampi doesn't reference helper().
	if len(res.Diagnostics) > 0 {
		for _, d := range res.Diagnostics {
			t.Errorf("diagnostic: %s", d.Message)
		}
	}
}
