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
	stringMultRe = regexp.MustCompile(`(["'][^"']*["']\s*\*|\*\s*["'][^"']*["'])`)

	// unicodeMarkRe matches Unicode combining marks which can cause CUE stack overflow
	// when used in certain contexts (triggers infinite recursion in error path resolution)
	unicodeMarkRe = regexp.MustCompile(`\p{M}`)

	// whitespaceRe matches all whitespace characters for normalization
	whitespaceRe = regexp.MustCompile(`\s+`)
)

func ValidateCueInput(data []byte) error {
	// Reject invalid UTF-8 - CUE hangs on certain malformed byte sequences
	if !utf8.Valid(data) {
		return MalformedInput{Reason: "invalid UTF-8 encoding"}
	}
	// Reject Unicode control characters (C0 and C1 ranges) except tab, newline, CR
	// CUE hangs or crashes on certain control characters
	if hasDisallowedControlChars(data) {
		return MalformedInput{Reason: "disallowed control characters"}
	}
	// Reject Unicode combining marks which cause CUE stack overflow
	if unicodeMarkRe.Match(data) {
		return MalformedInput{Reason: "Unicode combining marks trigger CUE stack overflow"}
	}
	if resourceBombRe.Match(data) {
		return MalformedInput{Reason: "resource exhaustion pattern detected (large multiplier)"}
	}
	if stringMultRe.Match(data) {
		return MalformedInput{Reason: "string literal multiplication triggers CUE hang"}
	}
	// Normalize whitespace before checking hang pattern
	normalized := whitespaceRe.ReplaceAll(data, nil)
	// Check if input lacks structural braces (outside of strings) and has at least one colon
	// These patterns trigger CUE evaluator bugs
	if !hasStructuralBraces(normalized) && countColons(normalized) >= 1 {
		return MalformedInput{Reason: "pattern triggers CUE evaluator bug (upstream issue #4231)"}
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

type MalformedInput struct {
	Reason string
}

func (e MalformedInput) Error() string {
	return fmt.Sprintf("malformed input: %s", e.Reason)
}

func (e MalformedInput) EventTemplate() event.Template {
	return event.Template{
		ID:   "config.MalformedInput",
		Text: "malformed configuration input",
		Hint: e.Reason,
	}
}

func (MalformedInput) Severity() signal.Severity { return signal.Error }
func (MalformedInput) Impact() diagnostic.Impact { return diagnostic.ImpactAbort }

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
		Text:   msg,
		Source: src,
	}
}

func (CueDiagnostic) Severity() signal.Severity { return signal.Error }
func (CueDiagnostic) Impact() diagnostic.Impact { return diagnostic.ImpactAbort }

type CueMissingField struct {
	Field  string
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
		Text: d.Error(),
		Source: &spec.SourceSpan{
			Filename:  m.Source.Filename,
			StartLine: m.Source.StartLine,
			StartCol:  m.Source.StartCol,
			EndCol:    m.Source.EndCol,
		},
	}
}

func (MissingFieldDiagnostic) Severity() signal.Severity { return signal.Error }
func (MissingFieldDiagnostic) Impact() diagnostic.Impact { return diagnostic.ImpactAbort }

type TypeMismatch struct {
	Source spec.SourceSpan
	Path   string
	Have   string
	Want   string
}

func (e TypeMismatch) Error() string {
	return fmt.Sprintf("type mismatch in %q: have %q, want %q", e.Path, e.Have, e.Want)
}

func (e TypeMismatch) EventTemplate() event.Template {
	return event.Template{
		ID:     "core.TypeMismatch",
		Text:   "type mismatch in '{{.Path}}', expected {{.Want}}",
		Hint:   "expected {{.Want}}, have {{.Have}}",
		Source: &e.Source,
		Data:   e,
	}
}

func (e TypeMismatch) Severity() signal.Severity { return signal.Error }
func (TypeMismatch) Impact() diagnostic.Impact   { return diagnostic.ImpactAbort }

type InvalidUnitShape struct {
	Source spec.SourceSpan
	Have   string
	Want   string
}

func (e InvalidUnitShape) Error() string {
	return fmt.Sprintf("invalid unit declaration: have %q, want %q", e.Have, e.Want)
}

func (e InvalidUnitShape) EventTemplate() event.Template {
	return event.Template{
		ID:     "core.InvalidUnitShape",
		Text:   "invalid 'unit' declaration, expected {{.Want}}",
		Hint:   "expected {{.Want}}, have {{.Have}}",
		Help:   `the 'unit' field must be a {{.Want}}, e.g. unit: {id: "...", desc: "..." }`,
		Source: &e.Source,
		Data:   e,
	}
}

func (e InvalidUnitShape) Severity() signal.Severity { return signal.Error }
func (InvalidUnitShape) Impact() diagnostic.Impact   { return diagnostic.ImpactAbort }

type InvalidStepsShape struct {
	Source spec.SourceSpan
	Have   string
	Want   string
}

func (e InvalidStepsShape) Error() string {
	return fmt.Sprintf("invalid steps declaration: have %q, want %q", e.Have, e.Want)
}

func (e InvalidStepsShape) EventTemplate() event.Template {
	return event.Template{
		ID:     "core.InvalidStepsShape",
		Text:   "invalid 'steps' declaration, expected {{.Want}}",
		Hint:   "expected {{.Want}}, have {{.Have}}",
		Help:   "the 'steps' field must be a {{.Want}}, e.g. steps: [ {...}, {...} ]",
		Source: &e.Source,
		Data:   e,
	}
}

func (e InvalidStepsShape) Severity() signal.Severity { return signal.Error }
func (InvalidStepsShape) Impact() diagnostic.Impact   { return diagnostic.ImpactAbort }

type UnknownStepKind struct {
	Kind   string
	Source spec.SourceSpan
}

func (e UnknownStepKind) Error() string {
	return fmt.Sprintf("unknown step kind %q", e.Kind)
}

func (e UnknownStepKind) EventTemplate() event.Template {
	return event.Template{
		ID:     "config.UnknownStepKind",
		Text:   `unknown step kind "{{.Kind}}"`,
		Hint:   "check that the step kind is spelled correctly",
		Help:   "available kinds are registered in the engine; see documentation for supported step types",
		Source: &e.Source,
		Data:   e,
	}
}

func (UnknownStepKind) Severity() signal.Severity { return signal.Error }
func (UnknownStepKind) Impact() diagnostic.Impact { return diagnostic.ImpactAbort }

// CuePanic wraps a panic recovered from the CUE library.
// This handles (un-)known upstream bugs where CUE panics on malformed input.
type CuePanic struct {
	Recovered any
}

func (e CuePanic) Error() string {
	return fmt.Sprintf("cue panic: %v", e.Recovered)
}

func (e CuePanic) EventTemplate() event.Template {
	return event.Template{
		ID:   "cue.InternalError",
		Text: "CUE encountered an internal error while parsing configuration",
		Hint: "this is likely a CUE bug triggered by malformed input",
		Help: "recovered panic: {{.Recovered}}",
		Data: e,
	}
}

func (CuePanic) Severity() signal.Severity { return signal.Error }
func (CuePanic) Impact() diagnostic.Impact { return diagnostic.ImpactAbort }

// UnknownIndexKind is emitted when a user requests documentation for an unknown step kind.
type UnknownIndexKind struct {
	Kind string
}

func (e UnknownIndexKind) Error() string {
	return fmt.Sprintf("unknown step kind %q", e.Kind)
}

func (e UnknownIndexKind) EventTemplate() event.Template {
	return event.Template{
		ID:   "index.UnknownKind",
		Text: `unknown step kind "{{.Kind}}"`,
		Hint: "use 'doit index' to list available step types",
		Data: e,
	}
}

func (UnknownIndexKind) Severity() signal.Severity { return signal.Error }
func (UnknownIndexKind) Impact() diagnostic.Impact { return diagnostic.ImpactAbort }
