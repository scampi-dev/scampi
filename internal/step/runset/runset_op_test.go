// SPDX-License-Identifier: GPL-3.0-only

package runset

import (
	"reflect"
	"testing"

	"scampi.dev/scampi/internal/spec"
)

func Test_ParseTemplate_ClassifiesPlaceholderForms(t *testing.T) {
	cases := []struct {
		name string
		cmd  string
		want templatePlaceholder
	}{
		{"per-item-spaced", "samba-tool group addmember admins {{ item }}", tplPerItem},
		{"per-item-tight", "do {{item}}", tplPerItem},
		{"batch-space", "iface up {{ items }}", tplBatchSpace},
		{"batch-csv", "samba-tool group addmembers admins {{ items_csv }}", tplBatchCSV},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tpl, err := parseTemplate("add", tc.cmd, anySpan())
			if err != nil {
				t.Fatalf("parseTemplate(%q): %v", tc.cmd, err)
			}
			if tpl.kind != tc.want {
				t.Errorf("kind = %v, want %v", tpl.kind, tc.want)
			}
		})
	}
}

func Test_ParseTemplate_RejectsMissingPlaceholder(t *testing.T) {
	_, err := parseTemplate("add", "samba-tool group addmembers admins", anySpan())
	if _, ok := err.(MissingTemplateError); !ok {
		t.Fatalf("expected MissingTemplateError, got %T: %v", err, err)
	}
}

func Test_ParseTemplate_RejectsMixedPlaceholders(t *testing.T) {
	_, err := parseTemplate("add", "do {{ item }} or {{ items }}", anySpan())
	if _, ok := err.(InvalidTemplateError); !ok {
		t.Fatalf("expected InvalidTemplateError, got %T: %v", err, err)
	}
}

func Test_ParseTemplate_EmptyReturnsNil(t *testing.T) {
	tpl, err := parseTemplate("remove", "", anySpan())
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if tpl != nil {
		t.Fatalf("expected nil template for empty cmd, got %v", tpl)
	}
}

func Test_ItemTemplate_RendersBatchCSV(t *testing.T) {
	tpl, err := parseTemplate("add", "samba-tool group addmembers admins {{ items_csv }}", anySpan())
	if err != nil {
		t.Fatal(err)
	}
	got := tpl.render([]string{"alice", "bob", "carol"})
	want := []string{"samba-tool group addmembers admins alice,bob,carol"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v\nwant %v", got, want)
	}
}

func Test_ItemTemplate_RendersBatchSpaceSeparated(t *testing.T) {
	tpl, _ := parseTemplate("add", "ip route add {{ items }}", anySpan())
	got := tpl.render([]string{"a", "b"})
	want := []string{"ip route add a b"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v\nwant %v", got, want)
	}
}

func Test_ItemTemplate_RendersPerItem(t *testing.T) {
	tpl, _ := parseTemplate("add", "addone {{ item }}", anySpan())
	got := tpl.render([]string{"a", "b", "c"})
	want := []string{"addone a", "addone b", "addone c"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v\nwant %v", got, want)
	}
}

func Test_ItemTemplate_RendersNilForEmptyItems(t *testing.T) {
	tpl, _ := parseTemplate("add", "addone {{ item }}", anySpan())
	if got := tpl.render(nil); got != nil {
		t.Errorf("nil items -> %v, want nil", got)
	}
	if got := tpl.render([]string{}); got != nil {
		t.Errorf("empty items -> %v, want nil", got)
	}
}

func Test_ParseListStdout_TrimsAndDedupes(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{"empty", "", nil},
		{"trailing-newline", "alice\nbob\n", []string{"alice", "bob"}},
		{"blank-lines", "alice\n\n  \nbob\n", []string{"alice", "bob"}},
		{"trim-whitespace", "  alice  \n\tbob\t\n", []string{"alice", "bob"}},
		{"dedupe", "alice\nbob\nalice\n", []string{"alice", "bob"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseListStdout(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func Test_Diff_ComputesAddAndRemove(t *testing.T) {
	live := []string{"alice", "bob", "stale"}
	desired := []string{"bob", "carol"}
	got := diff(live, desired, true, true)
	wantAdd := []string{"carol"}
	wantRemove := []string{"alice", "stale"} // sorted
	if !reflect.DeepEqual(got.toAdd, wantAdd) {
		t.Errorf("toAdd = %v, want %v", got.toAdd, wantAdd)
	}
	if !reflect.DeepEqual(got.toRemove, wantRemove) {
		t.Errorf("toRemove = %v, want %v", got.toRemove, wantRemove)
	}
}

func Test_Diff_SkipsOrphansWhenRemoveDisabled(t *testing.T) {
	// User declared `add` but no `remove` - orphans must NOT be reported
	// as drift (one-way reconciliation).
	got := diff([]string{"keep-me"}, []string{"new"}, true, false)
	if !reflect.DeepEqual(got.toAdd, []string{"new"}) {
		t.Errorf("toAdd = %v, want [new]", got.toAdd)
	}
	if got.toRemove != nil {
		t.Errorf("toRemove should be nil when remove disabled, got %v", got.toRemove)
	}
}

func Test_Diff_SkipsAdditionsWhenAddDisabled(t *testing.T) {
	got := diff([]string{"orphan"}, []string{"declared"}, false, true)
	if got.toAdd != nil {
		t.Errorf("toAdd should be nil when add disabled, got %v", got.toAdd)
	}
	if !reflect.DeepEqual(got.toRemove, []string{"orphan"}) {
		t.Errorf("toRemove = %v, want [orphan]", got.toRemove)
	}
}

func Test_Diff_ReturnsEmptyWhenConverged(t *testing.T) {
	got := diff([]string{"a", "b"}, []string{"a", "b"}, true, true)
	if got.toAdd != nil || got.toRemove != nil {
		t.Errorf("converged but got add=%v remove=%v", got.toAdd, got.toRemove)
	}
}

func Test_DedupePreserve_KeepsFirstOccurrence(t *testing.T) {
	got := dedupePreserve([]string{"a", "b", "a", "c", "b"})
	want := []string{"a", "b", "c"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func Test_EnvPrefix_SortsKeys(t *testing.T) {
	got := envPrefix(map[string]string{"BETA": "2", "ALPHA": "1"})
	want := "ALPHA='1' BETA='2' "
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func anySpan() spec.Span { return spec.Span{} }
