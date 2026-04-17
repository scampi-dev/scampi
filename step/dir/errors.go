// SPDX-License-Identifier: GPL-3.0-only

package dir

import (
	"fmt"

	"scampi.dev/scampi/diagnostic"
	"scampi.dev/scampi/diagnostic/event"
	"scampi.dev/scampi/spec"
)

type NotADirectoryError struct {
	diagnostic.FatalError
	Path   string
	Source spec.SourceSpan
}

func (e NotADirectoryError) Error() string {
	return fmt.Sprintf("path %q exists but is not a directory", e.Path)
}

func (e NotADirectoryError) EventTemplate() event.Template {
	return event.Template{
		ID:     CodeNotADirectory,
		Text:   `path "{{.Path}}" exists but is not a directory`,
		Hint:   `remove or rename the file at "{{.Path}}", then rerun to create it as a directory`,
		Data:   e,
		Source: &e.Source,
	}
}

type PartialOwnershipError struct {
	diagnostic.FatalError
	Set     string
	Missing string
	Source  spec.SourceSpan
}

func (e PartialOwnershipError) Error() string {
	return fmt.Sprintf("%s is set but %s is empty — set both or neither", e.Set, e.Missing)
}

func (e PartialOwnershipError) EventTemplate() event.Template {
	return event.Template{
		ID:     CodePartialOwnership,
		Text:   `{{.Set}} is set but {{.Missing}} is empty`,
		Hint:   `add {{.Missing}}="<value>" or remove {{.Set}}`,
		Data:   e,
		Source: &e.Source,
	}
}
