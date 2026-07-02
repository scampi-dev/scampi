// SPDX-License-Identifier: GPL-3.0-only

package linker

import (
	"scampi.dev/scampi/internal/diagnostic/event"
	"scampi.dev/scampi/internal/errs"
	"scampi.dev/scampi/internal/spec"
)

// UnresolvedError is returned when a stub declaration has no matching
// entry in the engine registry.
type UnresolvedError struct {
	Kind string // "step", "target", "type"
	Name string
}

func (e *UnresolvedError) Error() string      { return "unresolved " + e.Kind + ": " + e.Name }
func (e *UnresolvedError) GetCode() errs.Code { return CodeUnresolved }

// ConfigReadError is returned when the config file itself cannot be read,
// before the lang pipeline even starts.
type ConfigReadError struct {
	Path  string
	Cause error
}

type configReadData struct {
	Path   string
	Reason string
}

func (e *ConfigReadError) Error() string {
	return "cannot read config file " + e.Path + ": " + e.Cause.Error()
}

func (e *ConfigReadError) Diagnostic() event.Event {
	return event.Error{
		Impact: event.ImpactAbort,
		Template: event.Template{
			ID:   CodeConfigRead,
			Text: `cannot read config file "{{.Path}}": {{.Reason}}`,
			Hint: "check that the file exists and is readable; " +
				"a relative path resolves from the current working directory",
			Data: configReadData{
				Path:   e.Path,
				Reason: e.Cause.Error(),
			},
			Source: &spec.SourceSpan{Filename: e.Path},
		},
	}
}
