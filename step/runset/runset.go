// SPDX-License-Identifier: GPL-3.0-only

package runset

import (
	"strings"

	"scampi.dev/scampi/errs"
	"scampi.dev/scampi/spec"
)

type (
	RunSet       struct{}
	RunSetConfig struct {
		_ struct{} `summary:"Shell-driven set reconciliation: list / add / remove a CLI-managed collection"`

		Desc string `step:"Human-readable description" optional:"true"`
		//nolint:revive // line-length: long step tag
		List    string   `step:"Shell command listing identifiers, one per line" example:"samba-tool group listmembers admins"`
		Add     string   `step:"Add command; use {{ item }}, {{ items }}, or {{ items_csv }}" optional:"true"`
		Remove  string   `step:"Remove command; same template shape as add" optional:"true"`
		Desired []string `step:"Identifiers that should be present" optional:"true"`
		Init    string   `step:"Bootstrap command run if list exits non-zero" optional:"true"`
		//nolint:revive // line-length unavoidable: tag set
		Env      map[string]string `step:"Environment variables for list/add/remove/init invocations" optional:"true"`
		Promises []string          `step:"Resources this step produces (cross-deploy ordering)" optional:"true"`
		Inputs   []string          `step:"Resources this step requires (cross-deploy ordering)" optional:"true"`
	}

	runSetAction struct {
		desc      string
		step      spec.StepInstance
		list      string
		addTpl    *itemTemplate
		removeTpl *itemTemplate
		desired   []string
		init      string
		env       map[string]string
	}
)

func (RunSet) Kind() string   { return "run_set" }
func (RunSet) NewConfig() any { return &RunSetConfig{} }

func (c *RunSetConfig) ResourceDeclarations() (promises, inputs []string) {
	return c.Promises, c.Inputs
}

func (RunSet) Plan(step spec.StepInstance) (spec.Action, error) {
	cfg, ok := step.Config.(*RunSetConfig)
	if !ok {
		return nil, errs.BUG("expected %T got %T", &RunSetConfig{}, step.Config)
	}

	if cfg.Add == "" && cfg.Remove == "" {
		return nil, NothingToDeclareError{Source: step.Source}
	}

	addTpl, err := parseTemplate("add", cfg.Add, step.Source)
	if err != nil {
		return nil, err
	}
	removeTpl, err := parseTemplate("remove", cfg.Remove, step.Source)
	if err != nil {
		return nil, err
	}

	desired := dedupePreserve(cfg.Desired)

	return &runSetAction{
		desc:      cfg.Desc,
		step:      step,
		list:      cfg.List,
		addTpl:    addTpl,
		removeTpl: removeTpl,
		desired:   desired,
		init:      cfg.Init,
		env:       cfg.Env,
	}, nil
}

func (a *runSetAction) Desc() string { return a.desc }
func (a *runSetAction) Kind() string { return "run_set" }

func (a *runSetAction) Ops() []spec.Op {
	op := &runSetOp{
		list:      a.list,
		addTpl:    a.addTpl,
		removeTpl: a.removeTpl,
		desired:   a.desired,
		init:      a.init,
		env:       a.env,
		source:    a.step.Source,
	}
	op.SetAction(a)
	return []spec.Op{op}
}

// dedupePreserve returns xs with duplicates removed, preserving the
// first occurrence of each value. Stable identifier order in the
// rendered batch command matters for diffability.
func dedupePreserve(xs []string) []string {
	if len(xs) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(xs))
	out := make([]string, 0, len(xs))
	for _, x := range xs {
		if _, dup := seen[x]; dup {
			continue
		}
		seen[x] = struct{}{}
		out = append(out, x)
	}
	return out
}

// templatePlaceholder identifies which placeholder a template uses.
type templatePlaceholder int

const (
	tplEmpty templatePlaceholder = iota
	tplPerItem
	tplBatchSpace
	tplBatchCSV
)

// itemTemplate is a parsed add/remove command template.
type itemTemplate struct {
	raw  string
	kind templatePlaceholder
}

// Recognised placeholders. Whitespace inside the braces is tolerated:
// `{{item}}`, `{{ item }}`, `{{  item  }}` all match.
var placeholderForms = map[templatePlaceholder][]string{
	tplPerItem:    {"{{item}}", "{{ item }}", "{{  item  }}"},
	tplBatchSpace: {"{{items}}", "{{ items }}", "{{  items  }}"},
	tplBatchCSV:   {"{{items_csv}}", "{{ items_csv }}", "{{  items_csv  }}"},
}

func detectPlaceholder(cmd string) templatePlaceholder {
	hasItem := containsAny(cmd, placeholderForms[tplPerItem])
	hasItems := containsAny(cmd, placeholderForms[tplBatchSpace])
	hasCSV := containsAny(cmd, placeholderForms[tplBatchCSV])
	switch {
	case hasItem && (hasItems || hasCSV):
		return -1 // mixed — caller treats as invalid
	case hasItems && hasCSV:
		return tplBatchSpace // both batch forms — fold to space (rare; CSV is the strict subset)
	case hasItem:
		return tplPerItem
	case hasItems:
		return tplBatchSpace
	case hasCSV:
		return tplBatchCSV
	default:
		return tplEmpty
	}
}

func containsAny(s string, candidates []string) bool {
	for _, c := range candidates {
		if strings.Contains(s, c) {
			return true
		}
	}
	return false
}

func parseTemplate(field, cmd string, src spec.SourceSpan) (*itemTemplate, error) {
	if cmd == "" {
		return nil, nil
	}
	kind := detectPlaceholder(cmd)
	if kind == -1 {
		return nil, InvalidTemplateError{Field: field, Cmd: cmd, Source: src}
	}
	if kind == tplEmpty {
		return nil, MissingTemplateError{Field: field, Cmd: cmd, Source: src}
	}
	return &itemTemplate{raw: cmd, kind: kind}, nil
}

// render expands the template with the given items. For batch forms
// it returns one command; for per-item form it returns one command
// per item.
func (t *itemTemplate) render(items []string) []string {
	if t == nil || len(items) == 0 {
		return nil
	}
	switch t.kind {
	case tplPerItem:
		out := make([]string, 0, len(items))
		for _, it := range items {
			out = append(out, replaceAll(t.raw, placeholderForms[tplPerItem], it))
		}
		return out
	case tplBatchSpace:
		return []string{replaceAll(t.raw, placeholderForms[tplBatchSpace], strings.Join(items, " "))}
	case tplBatchCSV:
		return []string{replaceAll(t.raw, placeholderForms[tplBatchCSV], strings.Join(items, ","))}
	default:
		return nil
	}
}

func replaceAll(s string, candidates []string, with string) string {
	for _, c := range candidates {
		s = strings.ReplaceAll(s, c, with)
	}
	return s
}
