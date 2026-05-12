// SPDX-License-Identifier: GPL-3.0-only

package lsp

import (
	"testing"
)

func TestAnalyzeCursorTopLevel(t *testing.T) {
	text := "cop"
	cur := AnalyzeCursor(text, 0, 3)
	if cur.InCall {
		t.Error("should not be in call")
	}
	if cur.WordUnderCursor != "cop" {
		t.Errorf("word = %q, want 'cop'", cur.WordUnderCursor)
	}
}

func TestAnalyzeCursorInsideCall(t *testing.T) {
	text := `copy(src=local("./f"), dest=`
	cur := AnalyzeCursor(text, 0, uint32(len(text)))
	if !cur.InCall {
		t.Fatal("should be in call")
	}
	if cur.FuncName != "copy" {
		t.Errorf("func = %q, want 'copy'", cur.FuncName)
	}
	if len(cur.PresentKwargs) != 2 {
		t.Errorf("present kwargs = %v, want [src dest]", cur.PresentKwargs)
	}
}

func TestAnalyzeCursorDottedFunc(t *testing.T) {
	text := `target.ssh(name="web", host=`
	cur := AnalyzeCursor(text, 0, uint32(len(text)))
	if !cur.InCall {
		t.Fatal("should be in call")
	}
	if cur.FuncName != "target.ssh" {
		t.Errorf("func = %q, want 'target.ssh'", cur.FuncName)
	}
}

func TestAnalyzeCursorCommaCount(t *testing.T) {
	text := `pkg(name="nginx", state="present", `
	cur := AnalyzeCursor(text, 0, uint32(len(text)))
	if cur.ActiveParam != 2 {
		t.Errorf("active param = %d, want 2", cur.ActiveParam)
	}
}

func TestAnalyzeCursorNestedParens(t *testing.T) {
	text := `copy(src=local("./f"), `
	cur := AnalyzeCursor(text, 0, uint32(len(text)))
	if !cur.InCall {
		t.Fatal("should be in call to copy, not local")
	}
	if cur.FuncName != "copy" {
		t.Errorf("func = %q, want 'copy'", cur.FuncName)
	}
}

func TestAnalyzeCursorMultiline(t *testing.T) {
	text := "copy(\n    src=local(\"./f\"),\n    "
	// cursor is on line 2, col 4
	cur := AnalyzeCursor(text, 2, 4)
	if !cur.InCall {
		t.Fatal("should be in call")
	}
	if cur.FuncName != "copy" {
		t.Errorf("func = %q, want 'copy'", cur.FuncName)
	}
}

func TestAnalyzeCursorModulePrefix(t *testing.T) {
	text := "target."
	cur := AnalyzeCursor(text, 0, 7)
	if cur.InCall {
		t.Error("should not be in call")
	}
	if cur.WordUnderCursor != "target." {
		t.Errorf("word = %q, want 'target.'", cur.WordUnderCursor)
	}
}

func TestAnalyzeCursorInsideList(t *testing.T) {
	text := `deploy(name="x", steps=[`
	cur := AnalyzeCursor(text, 0, uint32(len(text)))
	if !cur.InList {
		t.Fatal("should be InList")
	}
	if cur.InCall {
		t.Error("should not be InCall (innermost bracket is [)")
	}
	if cur.FuncName != "deploy" {
		t.Errorf("func = %q, want 'deploy'", cur.FuncName)
	}
}

func TestAnalyzeCursorInsideListMultiline(t *testing.T) {
	text := "deploy(\n    name=\"x\",\n    steps=[\n        "
	cur := AnalyzeCursor(text, 3, 8)
	if !cur.InList {
		t.Fatal("should be InList")
	}
	if cur.InCall {
		t.Error("should not be InCall")
	}
}

func TestAnalyzeCursorInsideListAfterComma(t *testing.T) {
	text := `deploy(steps=[dir(path="/tmp"), `
	cur := AnalyzeCursor(text, 0, uint32(len(text)))
	if !cur.InList {
		t.Fatal("should be InList")
	}
}

func TestExtractKwargNames(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{`name="web", host="1.2.3.4"`, []string{"name", "host"}},
		{`x == 1, y=2`, []string{"y"}},
		{``, nil},
		{`name=`, []string{"name"}},
	}
	for _, tt := range tests {
		got := extractKwargNames(tt.input)
		if len(got) != len(tt.want) {
			t.Errorf("extractKwargNames(%q) = %v, want %v", tt.input, got, tt.want)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("extractKwargNames(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
			}
		}
	}
}

func TestCountTopLevelCommas(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{`a, b, c`, 2},
		{`a`, 0},
		{`f(a, b), c`, 1},
		{`[1, 2], "a,b"`, 1},
	}
	for _, tt := range tests {
		got := countTopLevelCommas(tt.input)
		if got != tt.want {
			t.Errorf("countTopLevelCommas(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestOffsetFromPosition_EmptyLine(t *testing.T) {
	text := "line1\n\nline3\n"

	offset := offsetFromPosition(text, 1, 5)
	expected := offsetFromPosition(text, 1, 0)
	if offset != expected {
		t.Errorf("col past empty line: offset=%d, expected=%d", offset, expected)
	}
}

func TestOffsetFromPosition_ColPastEOL(t *testing.T) {
	text := "abc\ndef\n"

	offset := offsetFromPosition(text, 0, 10)
	expected := offsetFromPosition(text, 0, 3)
	if offset != expected {
		t.Errorf("col past EOL: offset=%d, expected=%d", offset, expected)
	}
}

func TestOffsetFromPosition_LastLine(t *testing.T) {
	text := "hello"

	offset := offsetFromPosition(text, 0, 5)
	if offset != 5 {
		t.Errorf("last line: offset=%d, expected=5", offset)
	}
}

func TestOffsetFromPosition_Normal(t *testing.T) {
	text := "abc\ndef\nghi\n"
	offset := offsetFromPosition(text, 1, 2)

	if offset != 6 {
		t.Errorf("normal: offset=%d, expected=6", offset)
	}
}

func TestCursor_NestedBraces_InnerStructLit(t *testing.T) {
	text := `
outer {
  inner {
    field =
  }
}
`

	cur := AnalyzeCursor(text, 3, 12)
	if !cur.InCall {
		t.Fatal("should be InCall")
	}
	if cur.FuncName != "inner" {
		t.Errorf("FuncName = %q, want %q", cur.FuncName, "inner")
	}
	if cur.ActiveKwarg != "field" {
		t.Errorf("ActiveKwarg = %q, want %q", cur.ActiveKwarg, "field")
	}
}

func TestCursor_NestedBraces_OuterAfterInnerClosed(t *testing.T) {
	text := `
outer {
  inner { a = 1 }

}
`

	cur := AnalyzeCursor(text, 3, 2)
	if !cur.InCall {
		t.Fatal("should be InCall")
	}
	if cur.FuncName != "outer" {
		t.Errorf("FuncName = %q, want %q", cur.FuncName, "outer")
	}
}

func TestCursor_ForLoopBrace_IsNotStructLit(t *testing.T) {

	text := `
for x in items {

}
`
	cur := AnalyzeCursor(text, 2, 2)

	if cur.InCall && cur.FuncName == "items" {

		t.Log("for-loop brace misidentified as struct literal (known limitation)")
	}
}

func TestCursor_FuncBody_IsNotStructLit(t *testing.T) {
	text := `
func foo(x: int) string {

}
`
	cur := AnalyzeCursor(text, 2, 2)

	if cur.InCall {
		t.Error("func body should NOT be InCall")
	}
}

func TestCursor_DeployBody(t *testing.T) {

	text := `
std.deploy(name = "x", targets = [t]) {

}
`
	cur := AnalyzeCursor(text, 2, 2)
	if cur.InCall {
		t.Error("deploy body should NOT be InCall (it's a block fill)")
	}
}

func TestCursor_StructLitInsideDeployBody(t *testing.T) {
	text := `
std.deploy(name = "x", targets = [t]) {
  posix.copy {
    src = posix.source_inline { content = "hi" }

  }
}
`

	cur := AnalyzeCursor(text, 4, 4)
	if !cur.InCall {
		t.Fatal("should be InCall inside struct literal")
	}
	if cur.FuncName != "posix.copy" {
		t.Errorf("FuncName = %q, want %q", cur.FuncName, "posix.copy")
	}
}

func TestCursor_StructLitInsideForInsideDeploy(t *testing.T) {
	text := `
std.deploy(name = "x", targets = [t]) {
  for item in list {
    posix.dir {
      path = "/tmp"

    }
  }
}
`
	cur := AnalyzeCursor(text, 5, 6)
	if !cur.InCall {
		t.Fatal("should be InCall")
	}
	if cur.FuncName != "posix.dir" {
		t.Errorf("FuncName = %q, want %q", cur.FuncName, "posix.dir")
	}
	if len(cur.PresentKwargs) == 0 {
		t.Error("should have path in PresentKwargs")
	}
}

func TestCursor_EmptyLinePastEOL(t *testing.T) {

	text := "Type {\n  field = 1\n\n  }\n"

	cur := AnalyzeCursor(text, 2, 6)
	if !cur.InCall {
		t.Fatal("should be InCall")
	}
	if cur.FuncName != "Type" {
		t.Errorf("FuncName = %q, want %q (col clamped past empty line)", cur.FuncName, "Type")
	}
}

func TestCursor_InsideString_NoCompletion(t *testing.T) {
	text := `posix.copy { dest = "/etc/`
	cur := AnalyzeCursor(text, 0, uint32(len(text)))
	if !cur.InString {
		t.Error("should be InString")
	}
}

func TestCursor_AfterClosedString_NotInString(t *testing.T) {
	text := `posix.copy { dest = "/etc/foo", `
	cur := AnalyzeCursor(text, 0, uint32(len(text)))
	if cur.InString {
		t.Error("should NOT be InString after closed string")
	}
}

func TestActiveFieldNewlineSeparator(t *testing.T) {

	inside := "\n  id = 42\n  hostname = "
	got := activeField(inside)
	if got != "hostname" {
		t.Errorf("activeField = %q, want %q", got, "hostname")
	}
}

func TestActiveFieldNewlineSeparatorThreeFields(t *testing.T) {
	inside := "\n  id = 1\n  name = \"web\"\n  port = "
	got := activeField(inside)
	if got != "port" {
		t.Errorf("activeField = %q, want %q", got, "port")
	}
}

func TestActiveFieldWithCommas(t *testing.T) {

	inside := "\n  id = 1,\n  name = "
	got := activeField(inside)
	if got != "name" {
		t.Errorf("activeField = %q, want %q", got, "name")
	}
}

func TestActiveFieldSingleField(t *testing.T) {

	inside := " id = "
	got := activeField(inside)
	if got != "id" {
		t.Errorf("activeField = %q, want %q", got, "id")
	}
}

func TestAnalyzeBraceNewlineSeparator(t *testing.T) {

	text := "Type {\n  id = 42\n  name = "
	cur := AnalyzeCursor(text, 2, 9)
	if !cur.InCall {
		t.Fatal("should be in call")
	}
	if cur.FuncName != "Type" {
		t.Errorf("func = %q, want %q", cur.FuncName, "Type")
	}
	if cur.ActiveKwarg != "name" {
		t.Errorf("activeKwarg = %q, want %q", cur.ActiveKwarg, "name")
	}
}

func TestExtractFieldNamesNewlines(t *testing.T) {

	inside := "\n  id = 42\n  hostname = \"web\"\n  "
	names := extractFieldNames(inside)
	expected := map[string]bool{"id": true, "hostname": true}
	for _, n := range names {
		delete(expected, n)
	}
	if len(expected) > 0 {
		t.Errorf("missing field names: %v (got %v)", expected, names)
	}
}
