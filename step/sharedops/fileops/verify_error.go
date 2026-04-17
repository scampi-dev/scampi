// SPDX-License-Identifier: GPL-3.0-only

package fileops

import (
	"errors"
	"fmt"

	"scampi.dev/scampi/diagnostic"
	"scampi.dev/scampi/diagnostic/event"
	"scampi.dev/scampi/spec"
	"scampi.dev/scampi/target"
)

// VerifyError is returned when a verify command exits non-zero.
type VerifyError struct {
	diagnostic.FatalError
	Cmd      string
	Dest     string
	ExitCode int
	Stderr   string
	Source   spec.SourceSpan
}

func (e *VerifyError) Error() string {
	return fmt.Sprintf("verify %q failed (exit %d): %s", e.Cmd, e.ExitCode, e.Stderr)
}

func (e *VerifyError) EventTemplate() event.Template {
	return event.Template{
		ID:     CodeVerifyFailed,
		Text:   `verify command failed (exit {{.ExitCode}}): {{.Cmd}}`,
		Hint:   `the content did not pass validation — {{.Dest}} was not modified`,
		Help:   "{{.Stderr}}",
		Data:   e,
		Source: &e.Source,
	}
}

// VerifyPlaceholderError is returned when a verify command does not
// contain exactly one `%s` placeholder. The static @std.pattern
// attribute on copy.verify and template.verify catches literal verify
// strings at link time, so this only fires when the verify string came
// from a non-literal source (env var, secret, computed expression).
//
// The error counts as `Count` rather than as a binary "valid/invalid"
// so the message can show the user *which* malformation they hit
// (zero placeholders vs three placeholders are different mistakes).
type VerifyPlaceholderError struct {
	diagnostic.FatalError
	Cmd    string
	Count  int
	Source spec.SourceSpan
}

func (e *VerifyPlaceholderError) Error() string {
	return fmt.Sprintf(
		"verify command must contain exactly one %%s placeholder, got %d: %q",
		e.Count,
		e.Cmd,
	)
}

func (e *VerifyPlaceholderError) EventTemplate() event.Template {
	return event.Template{
		ID:   CodeVerifyPlaceholderErr,
		Text: `verify command must contain exactly one %s placeholder, got {{.Count}}: {{.Cmd}}`,
		Hint: "rewrite the verify command so the temp file path appears exactly once as %s",
		Data: e,
	}
}

// VerifyIOError is returned when verify infrastructure (temp dirs, temp files,
// running the verify command) fails due to I/O errors.
type VerifyIOError struct {
	diagnostic.FatalError
	Op     string
	Err    error
	Advice string
}

func (e VerifyIOError) Error() string {
	return e.Op + ": " + e.Err.Error()
}

func (e VerifyIOError) Unwrap() error { return e.Err }

func (e VerifyIOError) EventTemplate() event.Template {
	return event.Template{
		ID:   CodeVerifyIOError,
		Text: "verify failed: {{.Op}}",
		Hint: "{{.Advice}}",
		Help: "{{.Err}}",
		Data: e,
	}
}

func newVerifyIOError(op string, err error) VerifyIOError {
	return VerifyIOError{Op: op, Err: err, Advice: verifyIOAdvice(err)}
}

func verifyIOAdvice(err error) string {
	switch {
	case errors.Is(err, target.ErrPermission):
		return "the connecting user lacks write permission on the target — check ownership with ls -la"
	case errors.Is(err, target.ErrNotExist):
		return "path does not exist on target — check that parent directories are present"
	case errors.Is(err, target.ErrCommandNotFound):
		return "verify command not found — ensure it is installed on the target"
	default:
		return "check target filesystem permissions and connectivity"
	}
}
