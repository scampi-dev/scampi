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

func (e MalformedInput) Diagnostics(subject event.Subject) []event.Event {
	return []event.Event{
		diagnostic.DiagnosticRaised(subject, e),
	}
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

func (d CueDiagnostic) Diagnostics(subject event.Subject) []event.Event {
	var events []event.Event

	for _, e := range cueerr.Errors(d.Err) {
		events = append(events,
			diagnostic.DiagnosticRaised(subject, CueDiagnostic{
				Err:   e,
				Phase: d.Phase,
			}),
		)
	}

	return events
}

func (d CueDiagnostic) EventTemplate() event.Template {
	var src *spec.SourceSpan

	if d.Source != nil {
		src = d.Source
	} else if sp := spanFromPos(d.Err.Position()); sp.Line != 0 {
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
	Field      string
	UnitIndex  int
	UnitKind   string
	UnitName   string
	UnitSource spec.SourceSpan
}

type (
	MissingFieldDiagnostic struct {
		Missing CueMissingField
	}
)

func (d MissingFieldDiagnostic) Error() string {
	return fmt.Sprintf("field %q is mandatory", d.Missing.Field)
}

func (d MissingFieldDiagnostic) EventTemplate() event.Template {
	m := d.Missing
	return event.Template{
		ID:   "config.MissingField",
		Text: d.Error(),
		Source: &spec.SourceSpan{
			Filename: m.UnitSource.Filename,
			Line:     m.UnitSource.Line,
			StartCol: m.UnitSource.StartCol,
			EndCol:   m.UnitSource.EndCol,
		},
	}
}

func (d MissingFieldDiagnostic) Diagnostics(subject event.Subject) []event.Event {
	return []event.Event{
		diagnostic.DiagnosticRaised(subject, d),
	}
}

func (MissingFieldDiagnostic) Severity() signal.Severity { return signal.Error }
func (MissingFieldDiagnostic) Impact() diagnostic.Impact { return diagnostic.ImpactAbort }

type InvalidUnitsShape struct {
	Source spec.SourceSpan
	Have   string
	Want   string
}

func (e InvalidUnitsShape) Error() string {
	return fmt.Sprintf("invalid units declaration: have %q, want %q", e.Have, e.Want)
}

func (e InvalidUnitsShape) Diagnostics(subject event.Subject) []event.Event {
	return []event.Event{
		diagnostic.DiagnosticRaised(
			subject,
			e,
		),
	}
}

func (e InvalidUnitsShape) EventTemplate() event.Template {
	return event.Template{
		ID:     "core.InvalidUnitsShape",
		Text:   "invalid 'units' declaration, expected {{.Want}}",
		Hint:   "expected {{.Want}}, have {{.Have}}",
		Help:   "the 'units' field must be a list, e.g. units: [ {...}, {...} ]",
		Source: &e.Source,
		Data:   e,
	}
}

func (e InvalidUnitsShape) Severity() signal.Severity { return signal.Error }
func (InvalidUnitsShape) Impact() diagnostic.Impact   { return diagnostic.ImpactAbort }

type UnknownUnitKind struct {
	Kind   string
	Source spec.SourceSpan
}

func (e UnknownUnitKind) Error() string {
	return fmt.Sprintf("unknown unit kind %q", e.Kind)
}

func (e UnknownUnitKind) Diagnostics(subject event.Subject) []event.Event {
	return []event.Event{
		diagnostic.DiagnosticRaised(subject, e),
	}
}

func (e UnknownUnitKind) EventTemplate() event.Template {
	return event.Template{
		ID:     "config.UnknownUnitKind",
		Text:   `unknown unit kind "{{.Kind}}"`,
		Hint:   "check that the unit kind is spelled correctly",
		Help:   "available kinds are registered in the engine; see documentation for supported unit types",
		Source: &e.Source,
		Data:   e,
	}
}

func (UnknownUnitKind) Severity() signal.Severity { return signal.Error }
func (UnknownUnitKind) Impact() diagnostic.Impact { return diagnostic.ImpactAbort }

// CuePanic wraps a panic recovered from the CUE library.
// This handles (un-)known upstream bugs where CUE panics on malformed input.
type CuePanic struct {
	Recovered any
}

func (e CuePanic) Error() string {
	return fmt.Sprintf("cue panic: %v", e.Recovered)
}

func (e CuePanic) Diagnostics(subject event.Subject) []event.Event {
	return []event.Event{
		diagnostic.DiagnosticRaised(subject, e),
	}
}

func (e CuePanic) EventTemplate() event.Template {
	return event.Template{
		ID:   "cue.InternalError",
		Text: "CUE encountered an internal error while parsing configuration",
		Hint: "this is likely a CUE bug triggered by malformed input",
		Data: e,
	}
}

func (CuePanic) Severity() signal.Severity { return signal.Error }
func (CuePanic) Impact() diagnostic.Impact { return diagnostic.ImpactAbort }
