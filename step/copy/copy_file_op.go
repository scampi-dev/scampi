// SPDX-License-Identifier: GPL-3.0-only

package copy

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"

	"scampi.dev/scampi/capability"
	"scampi.dev/scampi/diagnostic"
	"scampi.dev/scampi/diagnostic/event"
	"scampi.dev/scampi/signal"
	"scampi.dev/scampi/source"
	"scampi.dev/scampi/spec"
	"scampi.dev/scampi/step/sharedop"
	"scampi.dev/scampi/step/sharedop/fileop"
	"scampi.dev/scampi/target"
)

const copyFileID = "step.copy-file"

type copyFileOp struct {
	sharedop.BaseOp
	src    string
	srcRef spec.SourceRef
	dest   string
	verify string
	backup bool
}

func (op *copyFileOp) getContent(ctx context.Context, src source.Source) ([]byte, error) {
	data, err := src.ReadFile(ctx, op.src)
	if err != nil {
		return nil, CopySourceMissingError{
			Path:   op.src,
			Err:    err,
			Source: op.SrcSpan,
		}
	}

	return data, nil
}

func (op *copyFileOp) Check(
	ctx context.Context,
	src source.Source,
	tgt target.Target,
) (spec.CheckResult, []spec.DriftDetail, error) {
	fsTgt := target.Must[target.Filesystem](copyFileID, tgt)

	srcData, err := op.getContent(ctx, src)
	if err != nil {
		if result, drift, ok := sharedop.CheckSourcePending(op.srcRef, "content"); ok {
			return result, drift, nil
		}
		return spec.CheckUnsatisfied, nil, err
	}

	if _, err := fsTgt.Stat(ctx, filepath.Dir(op.dest)); err != nil {
		return spec.CheckUnsatisfied, nil, CopyDestDirMissingError{
			Path:   filepath.Dir(op.dest),
			Err:    err,
			Source: op.DestSpan,
		}
	}

	destData, err := fsTgt.ReadFile(ctx, op.dest)
	if err != nil {
		return spec.CheckUnsatisfied, []spec.DriftDetail{{
			Field:   "content",
			Desired: fmt.Sprintf("%d bytes", len(srcData)),
		}}, nil
	}

	if !bytes.Equal(srcData, destData) {
		drift := []spec.DriftDetail{{
			Field:   "content",
			Current: fmt.Sprintf("%d bytes", len(destData)),
			Desired: fmt.Sprintf("%d bytes", len(srcData)),
		}}
		if op.backup {
			drift = append(drift, spec.DriftDetail{
				Field:     "backup",
				Desired:   op.dest + ".*.bak",
				Verbosity: signal.VVV,
			})
		}
		return spec.CheckUnsatisfied, drift, nil
	}

	return spec.CheckSatisfied, nil, nil
}

func (op *copyFileOp) Execute(ctx context.Context, src source.Source, tgt target.Target) (spec.Result, error) {
	fsTgt := target.Must[target.Filesystem](copyFileID, tgt)

	srcData, err := op.getContent(ctx, src)
	if err != nil {
		return spec.Result{}, err
	}

	if _, err := fsTgt.Stat(ctx, filepath.Dir(op.dest)); err != nil {
		return spec.Result{}, CopyDestDirMissingError{
			Path:   filepath.Dir(op.dest),
			Err:    err,
			Source: op.DestSpan,
		}
	}

	destData, err := fsTgt.ReadFile(ctx, op.dest)
	if err == nil && bytes.Equal(srcData, destData) {
		return spec.Result{Changed: false}, nil
	}

	if op.backup {
		if err := fileop.Backup(ctx, fsTgt, op.dest); err != nil {
			return spec.Result{}, sharedop.DiagnoseTargetError(err)
		}
	}

	if op.verify != "" {
		if err := fileop.VerifiedWrite(ctx, tgt, op.dest, srcData, op.verify); err != nil {
			return spec.Result{}, sharedop.DiagnoseTargetError(err)
		}
		return spec.Result{Changed: true}, nil
	}

	if err := fsTgt.WriteFile(ctx, op.dest, srcData); err != nil {
		return spec.Result{}, sharedop.DiagnoseTargetError(err)
	}

	return spec.Result{Changed: true}, nil
}

func (op *copyFileOp) RequiredCapabilities() capability.Capability {
	if op.verify != "" {
		return capability.Filesystem | capability.Command
	}
	return capability.Filesystem
}

func (op *copyFileOp) DesiredContent(ctx context.Context, src source.Source) ([]byte, error) {
	return op.getContent(ctx, src)
}

func (op *copyFileOp) CurrentContent(ctx context.Context, _ source.Source, tgt target.Target) ([]byte, error) {
	fsTgt := target.Must[target.Filesystem](copyFileID, tgt)
	return fsTgt.ReadFile(ctx, op.dest)
}

func (op *copyFileOp) DestPath() string {
	return op.dest
}

type copyFileDesc struct {
	Src  string
	Dest string
}

func (d copyFileDesc) PlanTemplate() spec.PlanTemplate {
	return spec.PlanTemplate{
		ID:   copyFileID,
		Text: `copy "{{.Src}}" -> "{{.Dest}}"`,
		Data: d,
	}
}

func (op *copyFileOp) OpDescription() spec.OpDescription {
	return copyFileDesc{
		Src:  op.srcRef.DisplayPath(),
		Dest: op.dest,
	}
}

func (op *copyFileOp) Inspect() []spec.InspectField {
	fields := []spec.InspectField{
		{Label: "src", Value: op.srcRef.DisplayPath()},
		{Label: "dest", Value: op.dest},
	}
	if op.verify != "" {
		fields = append(fields, spec.InspectField{Label: "verify", Value: op.verify})
	}
	return fields
}

// Errors
// -----------------------------------------------------------------------------

type CopySourceMissingError struct {
	diagnostic.FatalError
	Path   string
	Source spec.SourceSpan
	Err    error
}

func (e CopySourceMissingError) Error() string {
	return fmt.Sprintf("source file %q does not exist", e.Path)
}

func (e CopySourceMissingError) EventTemplate() event.Template {
	return event.Template{
		ID:     CodeSourceMissing,
		Text:   `source file "{{.Path}}" does not exist`,
		Hint:   "ensure the source file exists and is readable",
		Help:   "the copy action cannot proceed without a readable source file",
		Data:   e,
		Source: &e.Source,
	}
}

type CopyDestDirMissingError struct {
	diagnostic.FatalError
	Path   string
	Source spec.SourceSpan
	Err    error
}

func (e CopyDestDirMissingError) Error() string {
	return fmt.Sprintf("destination directory %q does not exist", e.Path)
}

func (e CopyDestDirMissingError) EventTemplate() event.Template {
	return event.Template{
		ID:     CodeDestDirMissing,
		Text:   `destination directory "{{.Path}}" does not exist`,
		Hint:   "create the destination directory before running this action",
		Help:   "the copy action does not create directories automatically",
		Data:   e,
		Source: &e.Source,
	}
}

func (e CopyDestDirMissingError) DeferredResource() spec.Resource {
	return spec.PathResource(e.Path)
}
