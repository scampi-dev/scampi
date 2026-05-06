// SPDX-License-Identifier: GPL-3.0-only

package dir

import (
	"fmt"

	"scampi.dev/scampi/diagnostic"
	"scampi.dev/scampi/diagnostic/event"
	"scampi.dev/scampi/spec"
)

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
