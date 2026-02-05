package render

import (
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/mattn/go-runewidth"
	"godoit.dev/doit/diagnostic/event"
	"godoit.dev/doit/spec"
)

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*m`)

type Template struct {
	Name string
	Text string
	Hint string
	Help string

	Data any

	Source *spec.SourceSpan
}

type RunSummary struct {
	ChangedCount int
	FailedCount  int
	TotalCount   int
}

type Displayer interface {
	EmitEngineLifecycle(e event.EngineEvent)
	EmitPlanLifecycle(e event.PlanEvent)
	EmitActionLifecycle(e event.ActionEvent)
	EmitOpLifecycle(e event.OpEvent)

	EmitIndexAll(e event.IndexAllEvent)
	EmitIndexStep(e event.IndexStepEvent)

	EmitLegend()

	EmitEngineDiagnostic(e event.EngineDiagnostic)
	EmitPlanDiagnostic(e event.PlanDiagnostic)
	EmitActionDiagnostic(e event.ActionDiagnostic)
	EmitOpDiagnostic(e event.OpDiagnostic)

	Close()
}

func s(n int) string {
	if n == 1 {
		return ""
	}

	return "s"
}

func visibleLen(s string) int {
	return runewidth.StringWidth(ansiRe.ReplaceAllString(s, ""))
}

func getANSI(s string) (seq string, ok bool) {
	if len(s) < 2 || s[0] != '\x1b' || s[1] != '[' {
		return "", false
	}
	for i := 2; i < len(s); i++ {
		if s[i] >= '@' && s[i] <= '~' {
			return s[:i+1], true
		}
	}
	return "", false
}

func fitLine(s string, maxLen int) string {
	if maxLen <= 0 {
		return s
	}

	// Fast path
	if visibleLen(s) <= maxLen {
		return s
	}

	if maxLen == 1 {
		return "…"
	}

	var out strings.Builder
	var lastColor string // last non-reset ANSI sequence
	width := 0

	for len(s) > 0 {
		// ANSI sequence has zero width -> copy verbatim
		if seq, ok := getANSI(s); ok {
			out.WriteString(seq)
			if seq != "\x1b[0m" {
				lastColor = seq
			}
			s = s[len(seq):]
			continue
		}

		// Next rune
		r, size := utf8.DecodeRuneInString(s)
		if r == utf8.RuneError && size == 1 {
			break
		}

		rw := runewidth.RuneWidth(r)
		if width+rw >= maxLen {
			break
		}

		out.WriteRune(r)
		width += rw
		s = s[size:]
	}

	// Re-apply the last active color so the ellipsis inherits it
	// instead of appearing as uncolored white.
	if lastColor != "" {
		out.WriteString(lastColor)
	}
	out.WriteString("…")

	return out.String()
}
