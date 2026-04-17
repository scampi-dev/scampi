// SPDX-License-Identifier: GPL-3.0-only

package mount

import (
	"fmt"

	"scampi.dev/scampi/diagnostic"
	"scampi.dev/scampi/diagnostic/event"
	"scampi.dev/scampi/spec"
)

type MountCommandError struct {
	diagnostic.FatalError
	Op     string
	Dest   string
	Stderr string
}

func (e MountCommandError) Error() string {
	return fmt.Sprintf("%s %s failed: %s", e.Op, e.Dest, e.Stderr)
}

func (e MountCommandError) EventTemplate() event.Template {
	return event.Template{
		ID:   CodeCommandFailed,
		Text: "{{.Op}} {{.Dest}} failed",
		Hint: "{{.Stderr}}",
		Data: e,
	}
}

type MissingToolError struct {
	diagnostic.FatalError
	FsType string
	Source spec.SourceSpan
}

func (e MissingToolError) Error() string {
	return fmt.Sprintf("mount type %q requires tools not found on target", e.FsType)
}

func (e MissingToolError) EventTemplate() event.Template {
	return event.Template{
		ID:   CodeMissingTool,
		Text: `mount type "{{.FsType}}" requires tools not found on target`,
		Hint: `{{if or (eq .FsType "nfs") (eq .FsType "nfs4")}}` +
			`add a pkg step for nfs-common (Debian/Ubuntu) or nfs-utils (RHEL/Fedora)` +
			`{{else if eq .FsType "cifs"}}add a pkg step for cifs-utils` +
			`{{else if eq .FsType "ceph"}}add a pkg step for ceph-common` +
			`{{else if eq .FsType "glusterfs"}}add a pkg step for glusterfs-client` +
			`{{else}}ensure the required filesystem tools are installed via a pkg step{{end}}`,
		Data:   e,
		Source: &e.Source,
	}
}
