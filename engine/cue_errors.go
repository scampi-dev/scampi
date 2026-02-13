// SPDX-License-Identifier: GPL-3.0-only

package engine

import (
	"fmt"
	"regexp"
	"unicode/utf8"

	cueerr "cuelang.org/go/cue/errors"

	"godoit.dev/doit/diagnostic"
	"godoit.dev/doit/diagnostic/event"
	"godoit.dev/doit/signal"
	"godoit.dev/doit/spec"
)

var (
	// resourceBombRe matches CUE string/bytes multiplication with large multipliers
	// that would cause resource exhaustion, e.g. "x"*10000000 or "x"*++++10000000
	resourceBombRe = regexp.MustCompile(`\*[\s+\-]*\d{7,}`)

	// stringMultRe matches string/bytes literal multiplication which can trigger CUE hangs
	// e.g. "x"*7T, 'x'*something, or 7*"x" (single quotes are byte literals in CUE)
	stringMultRe = regexp.MustCompile(
		`(["'][^"']*["']\s*\*\s*[^\s|]+|[A-Za-z0-9_)\]]+\s*\*\s*\(?\s*["'][^"']*["'])`,
	)

	// unicodeMarkRe matches Unicode combining marks which can cause CUE stack overflow
	// when used in certain contexts (triggers infinite recursion in error path resolution)
	unicodeMarkRe = regexp.MustCompile(`\p{M}`)

	// whitespaceRe matches all whitespace characters for normalization
	whitespaceRe = regexp.MustCompile(`\s+`)
)

func ValidateCueInput(data []byte) error {
	// Reject invalid UTF-8 - CUE hangs on certain malformed byte sequences
	if !utf8.Valid(data) {
		return MalformedInputError{Reason: "invalid UTF-8 encoding"}
	}
	// Reject Unicode control characters (C0 and C1 ranges) except tab, newline, CR
	// CUE hangs or crashes on certain control characters
	if hasDisallowedControlChars(data) {
		return MalformedInputError{Reason: "disallowed control characters"}
	}
	// Reject Unicode combining marks which cause CUE stack overflow
	if unicodeMarkRe.Match(data) {
		return MalformedInputError{Reason: "Unicode combining marks trigger CUE stack overflow"}
	}
	if resourceBombRe.Match(data) {
		return MalformedInputError{Reason: "resource exhaustion pattern detected (large multiplier)"}
	}
	if stringMultRe.Match(data) {
		return MalformedInputError{Reason: "string literal multiplication triggers CUE hang"}
	}
	// Normalize whitespace before checking hang pattern
	normalized := whitespaceRe.ReplaceAll(data, nil)
	// Check if input lacks structural braces (outside of strings) and has at least one colon
	// These patterns trigger CUE evaluator bugs
	if !hasStructuralBraces(normalized) && countColons(normalized) >= 1 {
		return MalformedInputError{Reason: "pattern triggers CUE evaluator bug (upstream issue #4231)"}
	}
	return nil
}

// hasDisallowedControlChars checks for C0 (0x00-0x1F), DEL (0x7F), and C1 (0x80-0x9F)
// control characters except common whitespace (tab, newline, CR). These cause CUE to hang.
func hasDisallowedControlChars(data []byte) bool {
	for _, r := range string(data) {
		// C0 control characters (except tab, newline, CR)
		if r < 0x20 && r != '\t' && r != '\n' && r != '\r' {
			return true
		}
		// DEL character
		if r == 0x7F {
			return true
		}
		// C1 control characters (0x80-0x9F)
		if r >= 0x80 && r <= 0x9F {
			return true
		}
	}
	return false
}

// hasStructuralBraces checks if the input has { or } outside of string literals.
// This indicates the input has CUE structure (struct definitions) rather than
// being just a bare expression which can trigger CUE bugs.
func hasStructuralBraces(data []byte) bool {
	inString := false
	stringChar := byte(0)
	escaped := false

	for _, b := range data {
		if escaped {
			escaped = false
			continue
		}
		if b == '\\' && inString {
			escaped = true
			continue
		}
		if !inString && (b == '"' || b == '\'') {
			inString = true
			stringChar = b
			continue
		}
		if inString && b == stringChar {
			inString = false
			continue
		}
		if !inString && (b == '{' || b == '}') {
			return true
		}
	}
	return false
}

func countColons(data []byte) int {
	n := 0
	for _, b := range data {
		if b == ':' {
			n++
		}
	}
	return n
}

type MalformedInputError struct {
	Reason string
}

func (e MalformedInputError) Error() string {
	return fmt.Sprintf("malformed input: %s", e.Reason)
}

func (e MalformedInputError) EventTemplate() event.Template {
	return event.Template{
		ID:   "config.MalformedInput",
		Text: "malformed configuration input",
		Hint: "{{.Reason}}",
		Data: e,
	}
}

func (MalformedInputError) Severity() signal.Severity { return signal.Error }
func (MalformedInputError) Impact() diagnostic.Impact { return diagnostic.ImpactAbort }

type CueDiagnostic struct {
	Err   cueerr.Error
	Phase string // "load", "validate", "decode"

	Source *spec.SourceSpan
}

func (d CueDiagnostic) Error() string {
	return fmt.Sprintf("cue.error in phase %q: %v", d.Phase, d.Err)
}

func (d CueDiagnostic) EventTemplate() event.Template {
	var src *spec.SourceSpan

	if d.Source != nil {
		src = d.Source
	} else if sp := spanFromPos(d.Err.Position()); sp.StartLine != 0 {
		src = &sp
	}

	msg := cueerr.Sanitize(d.Err).Error()

	if !d.Err.Position().IsValid() {
		if p := cueerr.Path(d.Err); len(p) > 0 {
			msg = fmt.Sprintf("%s (%#v)", msg, p)
		}
	}

	return event.Template{
		ID:     "cue." + d.Phase,
		Text:   "{{.}}",
		Data:   msg,
		Source: src,
	}
}

func (CueDiagnostic) Severity() signal.Severity { return signal.Error }
func (CueDiagnostic) Impact() diagnostic.Impact { return diagnostic.ImpactAbort }

type CueMissingField struct {
	Field  string
	Env    *string
	Source spec.SourceSpan
}

type MissingFieldDiagnostic struct {
	Missing CueMissingField
}

func (d MissingFieldDiagnostic) Error() string {
	return fmt.Sprintf("field %q is mandatory", d.Missing.Field)
}

func (d MissingFieldDiagnostic) EventTemplate() event.Template {
	m := d.Missing
	return event.Template{
		ID:   "config.MissingField",
		Text: `field {{.Field}} is mandatory`,
		Hint: `{{- if .Env }}This field may be set using the {{.Env}} environment variable{{end}}`,
		Source: &spec.SourceSpan{
			Filename:  m.Source.Filename,
			StartLine: m.Source.StartLine,
			StartCol:  m.Source.StartCol,
			EndCol:    m.Source.EndCol,
		},
		Data: m,
	}
}

func (MissingFieldDiagnostic) Severity() signal.Severity { return signal.Error }
func (MissingFieldDiagnostic) Impact() diagnostic.Impact { return diagnostic.ImpactAbort }

type TypeMismatchError struct {
	Source spec.SourceSpan
	Path   string
	Have   string
	Want   string
}

func (e TypeMismatchError) Error() string {
	return fmt.Sprintf("type mismatch in %q: have %q, want %q", e.Path, e.Have, e.Want)
}

func (e TypeMismatchError) EventTemplate() event.Template {
	return event.Template{
		ID:     "core.TypeMismatch",
		Text:   "type mismatch in '{{.Path}}', expected {{.Want}}",
		Hint:   "expected {{.Want}}, have {{.Have}}",
		Source: &e.Source,
		Data:   e,
	}
}

func (e TypeMismatchError) Severity() signal.Severity { return signal.Error }
func (TypeMismatchError) Impact() diagnostic.Impact   { return diagnostic.ImpactAbort }

type FieldNotAllowedError struct {
	Source spec.SourceSpan
	Path   string
}

func (e FieldNotAllowedError) Error() string {
	return fmt.Sprintf("field not allowed: %s", e.Path)
}

func (e FieldNotAllowedError) EventTemplate() event.Template {
	return event.Template{
		ID:     "core.FieldNotAllowed",
		Text:   "field '{{.Path}}' is not allowed here",
		Hint:   "remove the unknown field or check for typos",
		Source: &e.Source,
		Data:   e,
	}
}

func (FieldNotAllowedError) Severity() signal.Severity { return signal.Error }
func (FieldNotAllowedError) Impact() diagnostic.Impact { return diagnostic.ImpactAbort }

// CuePanicError wraps a panic recovered from the CUE library.
// This handles (un-)known upstream bugs where CUE panics on malformed input.
type CuePanicError struct {
	Recovered any
}

func (e CuePanicError) Error() string {
	return fmt.Sprintf("cue panic: %v", e.Recovered)
}

func (e CuePanicError) EventTemplate() event.Template {
	return event.Template{
		ID:   "cue.InternalError",
		Text: "CUE encountered an internal error while parsing configuration",
		Hint: "this is likely a CUE bug triggered by malformed input",
		Help: "recovered panic: {{.Recovered}}",
		Data: e,
	}
}

func (CuePanicError) Severity() signal.Severity { return signal.Error }
func (CuePanicError) Impact() diagnostic.Impact { return diagnostic.ImpactAbort }

// UnknownStepKindError is emitted when a step references an unknown or missing kind.
type UnknownStepKindError struct {
	Kind   string
	Source spec.SourceSpan
}

func (e UnknownStepKindError) Error() string {
	if e.Kind == "" {
		return "step has no kind"
	}
	return fmt.Sprintf("unknown step type %q", e.Kind)
}

func (e UnknownStepKindError) EventTemplate() event.Template {
	return event.Template{
		ID:     "config.UnknownStepKind",
		Text:   `{{if .Kind}}unknown step type "{{.Kind}}"{{else}}step has no kind{{end}}`,
		Hint:   "run 'doit index' to list available step types",
		Source: &e.Source,
		Data:   e,
	}
}

func (UnknownStepKindError) Severity() signal.Severity { return signal.Error }
func (UnknownStepKindError) Impact() diagnostic.Impact { return diagnostic.ImpactAbort }

// UnknownIndexKindError is emitted when a user requests documentation for an unknown step kind.
type UnknownIndexKindError struct {
	Kind string
}

func (e UnknownIndexKindError) Error() string {
	return fmt.Sprintf("unknown step kind %q", e.Kind)
}

func (e UnknownIndexKindError) EventTemplate() event.Template {
	return event.Template{
		ID:   "index.UnknownKind",
		Text: `unknown step kind "{{.Kind}}"`,
		Hint: "use 'doit index' to list available step types",
		Data: e,
	}
}

func (UnknownIndexKindError) Severity() signal.Severity { return signal.Error }
func (UnknownIndexKindError) Impact() diagnostic.Impact { return diagnostic.ImpactAbort }

// Resolution errors - emitted when resolving Config to ResolvedConfig

type UnknownDeployBlockError struct {
	Name string
}

func (e UnknownDeployBlockError) Error() string {
	return fmt.Sprintf("unknown deploy block %q", e.Name)
}

func (e UnknownDeployBlockError) EventTemplate() event.Template {
	return event.Template{
		ID:   "config.UnknownDeployBlock",
		Text: `unknown deploy block "{{.Name}}"`,
		Hint: "check that the deploy block name is spelled correctly",
		Data: e,
	}
}

func (UnknownDeployBlockError) Severity() signal.Severity { return signal.Error }
func (UnknownDeployBlockError) Impact() diagnostic.Impact { return diagnostic.ImpactAbort }

type NoDeployBlocksError struct{}

func (NoDeployBlocksError) Error() string {
	return "no deploy blocks defined"
}

func (NoDeployBlocksError) EventTemplate() event.Template {
	return event.Template{
		ID:   "config.NoDeployBlocks",
		Text: "no deploy blocks defined",
		Hint: "add at least one deploy block to the configuration",
	}
}

func (NoDeployBlocksError) Severity() signal.Severity { return signal.Error }
func (NoDeployBlocksError) Impact() diagnostic.Impact { return diagnostic.ImpactAbort }

type NoTargetsInDeployError struct {
	Deploy string
}

func (e NoTargetsInDeployError) Error() string {
	return fmt.Sprintf("deploy block %q has no targets", e.Deploy)
}

func (e NoTargetsInDeployError) EventTemplate() event.Template {
	return event.Template{
		ID:   "config.NoTargetsInDeploy",
		Text: `deploy block "{{.Deploy}}" has no targets`,
		Hint: "add at least one target to the deploy block's targets list",
		Data: e,
	}
}

func (NoTargetsInDeployError) Severity() signal.Severity { return signal.Error }
func (NoTargetsInDeployError) Impact() diagnostic.Impact { return diagnostic.ImpactAbort }

type UnknownTargetError struct {
	Name   string
	Deploy string
}

func (e UnknownTargetError) Error() string {
	return fmt.Sprintf("unknown target %q referenced in deploy block %q", e.Name, e.Deploy)
}

func (e UnknownTargetError) EventTemplate() event.Template {
	return event.Template{
		ID:   "config.UnknownTarget",
		Text: `unknown target "{{.Name}}" referenced in deploy block "{{.Deploy}}"`,
		Hint: "check that the target is defined in the targets map",
		Data: e,
	}
}

func (UnknownTargetError) Severity() signal.Severity { return signal.Error }
func (UnknownTargetError) Impact() diagnostic.Impact { return diagnostic.ImpactAbort }

type TargetNotInDeployError struct {
	Target string
	Deploy string
}

func (e TargetNotInDeployError) Error() string {
	return fmt.Sprintf("target %q is not in deploy block %q's target list", e.Target, e.Deploy)
}

func (e TargetNotInDeployError) EventTemplate() event.Template {
	return event.Template{
		ID:   "config.TargetNotInDeploy",
		Text: `target "{{.Target}}" is not in deploy block "{{.Deploy}}"'s target list`,
		Hint: "add the target to the deploy block's targets list or select a different target",
		Data: e,
	}
}

func (TargetNotInDeployError) Severity() signal.Severity { return signal.Error }
func (TargetNotInDeployError) Impact() diagnostic.Impact { return diagnostic.ImpactAbort }

type InventoryNotFoundError struct {
	Path string
}

func (e InventoryNotFoundError) Error() string {
	return fmt.Sprintf("inventory file not found: %s", e.Path)
}

func (e InventoryNotFoundError) EventTemplate() event.Template {
	return event.Template{
		ID:   "config.InventoryNotFound",
		Text: `inventory file not found: {{.Path}}`,
		Hint: "check that the inventory file path is correct",
		Data: e,
	}
}

func (InventoryNotFoundError) Severity() signal.Severity { return signal.Error }
func (InventoryNotFoundError) Impact() diagnostic.Impact { return diagnostic.ImpactAbort }
