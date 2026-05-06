// SPDX-License-Identifier: GPL-3.0-only

package symlink

import (
	"fmt"

	"scampi.dev/scampi/diagnostic"
	"scampi.dev/scampi/diagnostic/event"
	"scampi.dev/scampi/spec"
)

type LinkDirMissingError struct {
	diagnostic.FatalError
	Path   string
	Source spec.SourceSpan
	Err    error
}

func (e LinkDirMissingError) Error() string {
	return fmt.Sprintf("link directory %q does not exist", e.Path)
}

func (e LinkDirMissingError) EventTemplate() event.Template {
	return event.Template{
		ID:     CodeLinkDirMissing,
		Text:   `link directory "{{.Path}}" does not exist`,
		Hint:   `add dir(path="{{.Path}}") to your deploy steps before this symlink`,
		Help:   "the symlink action does not create directories automatically",
		Data:   e,
		Source: &e.Source,
	}
}

func (e LinkDirMissingError) DeferredResource() spec.Resource {
	return spec.PathResource(e.Path)
}

type LinkReadError struct {
	diagnostic.FatalError
	Path   string
	Source spec.SourceSpan
	Err    error
}

func (e LinkReadError) Error() string {
	return fmt.Sprintf("cannot read link %q: %v", e.Path, e.Err)
}

func (e LinkReadError) Unwrap() error {
	return e.Err
}

func (e LinkReadError) EventTemplate() event.Template {
	return event.Template{
		ID:     CodeLinkRead,
		Text:   `cannot read link "{{.Path}}"`,
		Hint:   `verify the parent directory of "{{.Path}}" exists and scampi has read permission on it`,
		Help:   `{{.Err}}`,
		Data:   e,
		Source: &e.Source,
	}
}
