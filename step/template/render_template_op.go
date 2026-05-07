// SPDX-License-Identifier: GPL-3.0-only

package template

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"scampi.dev/scampi/capability"
	"scampi.dev/scampi/errs"
	rendertmpl "scampi.dev/scampi/render/template"
	"scampi.dev/scampi/signal"
	"scampi.dev/scampi/source"
	"scampi.dev/scampi/spec"
	"scampi.dev/scampi/step/sharedop"
	"scampi.dev/scampi/step/sharedop/fileop"
	"scampi.dev/scampi/target"
)

const renderTemplateID = "step.render-template"

type renderTemplateOp struct {
	sharedop.BaseOp
	src    string
	srcRef spec.SourceRef
	dest   string
	data   DataConfig
	verify string
	backup bool
}

func (op *renderTemplateOp) Check(
	ctx context.Context,
	src source.Source,
	tgt target.Target,
) (spec.CheckResult, []spec.DriftDetail, error) {
	fsTgt := target.Must[target.Filesystem](renderTemplateID, tgt)

	data, err := mergeData(op.data, src)
	if err != nil {
		return spec.CheckUnsatisfied, nil, err
	}

	tmplContent, err := op.getTemplateContent(ctx, src, tgt)
	if err != nil {
		if result, drift, ok := sharedop.CheckSourcePending(op.srcRef, "content"); ok {
			return result, drift, nil
		}
		return spec.CheckUnsatisfied, nil, err
	}

	tmpl, err := rendertmpl.New("template").Parse(string(tmplContent))
	if err != nil {
		return spec.CheckUnsatisfied, nil, TemplateParseError{
			Err:    err,
			Source: op.SrcSpan,
		}
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return spec.CheckUnsatisfied, nil, op.execError(err, string(tmplContent))
	}

	if _, err := fsTgt.Stat(ctx, filepath.Dir(op.dest)); err != nil {
		return spec.CheckUnsatisfied, nil, DestDirMissingError{
			Path:   filepath.Dir(op.dest),
			Err:    err,
			Source: op.DestSpan,
		}
	}

	destData, err := fsTgt.ReadFile(ctx, op.dest)
	if err != nil {
		return spec.CheckUnsatisfied, []spec.DriftDetail{{
			Field:   "content",
			Desired: fmt.Sprintf("%d bytes", buf.Len()),
		}}, nil
	}

	if !bytes.Equal(buf.Bytes(), destData) {
		drift := []spec.DriftDetail{{
			Field:   "content",
			Current: fmt.Sprintf("%d bytes", len(destData)),
			Desired: fmt.Sprintf("%d bytes", buf.Len()),
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

func (op *renderTemplateOp) Execute(ctx context.Context, src source.Source, tgt target.Target) (spec.Result, error) {
	fsTgt := target.Must[target.Filesystem](renderTemplateID, tgt)

	data, err := mergeData(op.data, src)
	if err != nil {
		return spec.Result{}, err
	}

	tmplContent, err := op.getTemplateContent(ctx, src, tgt)
	if err != nil {
		return spec.Result{}, err
	}

	tmpl, err := rendertmpl.New("template").Parse(string(tmplContent))
	if err != nil {
		return spec.Result{}, err
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return spec.Result{}, op.execError(err, string(tmplContent))
	}

	if _, err := fsTgt.Stat(ctx, filepath.Dir(op.dest)); err != nil {
		return spec.Result{}, DestDirMissingError{
			Path:   filepath.Dir(op.dest),
			Err:    err,
			Source: op.DestSpan,
		}
	}

	destData, err := fsTgt.ReadFile(ctx, op.dest)
	if err == nil && bytes.Equal(buf.Bytes(), destData) {
		return spec.Result{Changed: false}, nil
	}

	if op.backup {
		if err := fileop.Backup(ctx, fsTgt, op.dest); err != nil {
			return spec.Result{}, sharedop.DiagnoseTargetError(err)
		}
	}

	if op.verify != "" {
		if err := fileop.VerifiedWrite(ctx, tgt, op.dest, buf.Bytes(), op.verify); err != nil {
			return spec.Result{}, sharedop.DiagnoseTargetError(err)
		}
		return spec.Result{Changed: true}, nil
	}

	if err := fsTgt.WriteFile(ctx, op.dest, buf.Bytes()); err != nil {
		if target.IsPermission(err) {
			return spec.Result{}, sharedop.PermissionDeniedError{
				Operation: "write " + op.dest,
				Source:    op.DestSpan,
				Err:       err,
			}
		}
		return spec.Result{}, sharedop.DiagnoseTargetError(err)
	}

	return spec.Result{Changed: true}, nil
}

func (op *renderTemplateOp) RequiredCapabilities() capability.Capability {
	if op.verify != "" {
		return capability.Filesystem | capability.Command
	}
	return capability.Filesystem
}

func (op *renderTemplateOp) DesiredContent(ctx context.Context, src source.Source, tgt target.Target) ([]byte, error) {
	data, err := mergeData(op.data, src)
	if err != nil {
		return nil, err
	}

	tmplContent, err := op.getTemplateContent(ctx, src, tgt)
	if err != nil {
		return nil, err
	}

	tmpl, err := rendertmpl.New("template").Parse(string(tmplContent))
	if err != nil {
		return nil, TemplateParseError{Err: err, Source: op.SrcSpan}
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, op.execError(err, string(tmplContent))
	}

	return buf.Bytes(), nil
}

func (op *renderTemplateOp) CurrentContent(ctx context.Context, _ source.Source, tgt target.Target) ([]byte, error) {
	fsTgt := target.Must[target.Filesystem](renderTemplateID, tgt)
	return fsTgt.ReadFile(ctx, op.dest)
}

func (op *renderTemplateOp) DestPath() string {
	return op.dest
}

func (op *renderTemplateOp) getTemplateContent(
	ctx context.Context,
	src source.Source,
	tgt target.Target,
) ([]byte, error) {
	var (
		data []byte
		err  error
	)
	if op.srcRef.Kind == spec.SourceTarget {
		// Read from the target itself (#286). Same payoff as
		// posix.copy + source_target — the template can pull its
		// source from a file produced by an earlier step on the
		// same target.
		fsTgt := target.Must[target.Filesystem](renderTemplateID, tgt)
		data, err = fsTgt.ReadFile(ctx, op.src)
	} else {
		data, err = src.ReadFile(ctx, op.src)
	}
	if err != nil {
		return nil, TemplateSourceMissingError{
			Path:   op.src,
			Err:    err,
			Source: op.SrcSpan,
		}
	}
	return data, nil
}

func mergeData(cfg DataConfig, src source.Source) (map[string]any, error) {
	data := make(map[string]any)

	// Copy values as base
	for k, v := range cfg.Values {
		data[k] = v
	}

	// Apply env overrides
	for envVar, key := range cfg.Env {
		// Check that key exists in values (schema validation)
		if _, exists := data[key]; !exists {
			return data, EnvKeyNotInValuesError{
				EnvVar: envVar,
				Key:    key,
			}
		}

		// If env var is set, override the value
		if val, ok := src.LookupEnv(envVar); ok {
			data[key] = val
		}
	}

	return data, nil
}

type renderTemplateDesc struct {
	Src  string
	Dest string
}

func (d renderTemplateDesc) PlanTemplate() spec.PlanTemplate {
	return spec.PlanTemplate{
		ID:   renderTemplateID,
		Text: `render "{{.Src}}" -> "{{.Dest}}"`,
		Data: d,
	}
}

// execError builds a TemplateExecError with a source span that points at
// the offending placeholder inside the template content when possible.
func (op *renderTemplateOp) execError(err error, tmplContent string) TemplateExecError {
	span := op.SrcSpan
	if op.srcRef.Kind == spec.SourceInline {
		if s, ok := tmplErrorSpan(err, tmplContent, op.SrcSpan); ok {
			span = s
		}
	} else if op.src != "" {
		if s, ok := tmplFileErrorSpan(err, op.src); ok {
			span = s
		}
	}

	key := extractMissingKey(err)
	if key != "" {
		return TemplateExecError{
			// bare-error: inner detail of TemplateExecError diagnostic
			Err:    errs.Errorf("missing template variable %q", key),
			Source: span,
		}
	}

	return TemplateExecError{Err: err, Source: span}
}

// tmplFileErrorSpan computes a SourceSpan pointing into the template source
// file. The Go template error gives us line/col within the file directly.
func tmplFileErrorSpan(err error, srcPath string) (spec.SourceSpan, bool) {
	msg := err.Error()
	parts := strings.SplitN(msg, ":", 5)
	if len(parts) < 5 {
		return spec.SourceSpan{}, false
	}
	tmplLine, lineErr := strconv.Atoi(strings.TrimSpace(parts[2]))
	tmplCol, colErr := strconv.Atoi(strings.TrimSpace(parts[3]))
	if lineErr != nil || colErr != nil {
		return spec.SourceSpan{}, false
	}

	exprLen := 0
	if idx := strings.Index(msg, "at <"); idx >= 0 {
		rest := msg[idx+4:]
		if end := strings.Index(rest, ">"); end >= 0 {
			exprLen = len(rest[:end])
		}
	}

	// Go template reports col at the second '{'; add 1 to reach the expression.
	srcCol := tmplCol + 1
	endCol := srcCol
	if exprLen > 0 {
		endCol = srcCol + exprLen
	}

	return spec.SourceSpan{
		Filename:  srcPath,
		StartLine: tmplLine,
		EndLine:   tmplLine,
		StartCol:  srcCol,
		EndCol:    endCol,
	}, true
}

// tmplErrorSpan computes a SourceSpan pointing at the offending expression
// inside an inline template string. The Go template error format is:
//
//	template: <name>:<line>:<col>: ...
//
// We offset from the content string literal's start position in the scampi
// source to land on the right line/col.
func tmplErrorSpan(err error, content string, contentSpan spec.SourceSpan) (spec.SourceSpan, bool) {
	// Parse "template: template:<line>:<col>: ..."
	msg := err.Error()
	parts := strings.SplitN(msg, ":", 5)
	if len(parts) < 5 {
		return spec.SourceSpan{}, false
	}
	tmplLine, lineErr := strconv.Atoi(strings.TrimSpace(parts[2]))
	tmplCol, colErr := strconv.Atoi(strings.TrimSpace(parts[3]))
	if lineErr != nil || colErr != nil {
		return spec.SourceSpan{}, false
	}

	// Extract the expression (between < and > in the error) for underline.
	// We underline the variable reference, not the {{ }} delimiters.
	exprLen := 0
	if idx := strings.Index(msg, "at <"); idx >= 0 {
		rest := msg[idx+4:]
		if end := strings.Index(rest, ">"); end >= 0 {
			exprLen = len(rest[:end])
		}
	}

	// scampi triple-quoted strings: the content span points at the opening
	// triple-quote. The actual content starts on the next line. For single-
	// quoted strings, content starts at col+1 on the same line.
	// We detect triple-quote by checking if content starts with a newline.
	srcLine := contentSpan.StartLine
	srcCol := contentSpan.StartCol

	// Go template reports col at the second '{'; add 1 to reach the expression.
	exprCol := tmplCol + 1

	if strings.HasPrefix(content, "\n") {
		srcLine += tmplLine
		srcCol = exprCol
	} else {
		if tmplLine == 1 {
			srcCol += exprCol - 1
		} else {
			srcLine += tmplLine - 1
			srcCol = exprCol
		}
	}

	endCol := srcCol
	if exprLen > 0 {
		endCol = srcCol + exprLen
	}

	return spec.SourceSpan{
		Filename:  contentSpan.Filename,
		StartLine: srcLine,
		EndLine:   srcLine,
		StartCol:  srcCol,
		EndCol:    endCol,
	}, true
}

// extractMissingKey pulls the key name from a "map has no entry for key" error.
func extractMissingKey(err error) string {
	const marker = "map has no entry for key "
	msg := err.Error()
	idx := strings.Index(msg, marker)
	if idx < 0 {
		return ""
	}
	return strings.Trim(msg[idx+len(marker):], "\"")
}

func (op *renderTemplateOp) OpDescription() spec.OpDescription {
	return renderTemplateDesc{
		Src:  op.srcRef.DisplayPath(),
		Dest: op.dest,
	}
}

func (op *renderTemplateOp) Inspect() []spec.InspectField {
	fields := []spec.InspectField{
		{Label: "src", Value: op.srcRef.DisplayPath()},
		{Label: "dest", Value: op.dest},
	}
	if len(op.data.Values) > 0 {
		keys := make([]string, 0, len(op.data.Values))
		for k := range op.data.Values {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		pairs := make([]string, 0, len(keys))
		for _, k := range keys {
			pairs = append(pairs, k+"="+fmt.Sprint(op.data.Values[k]))
		}
		fields = append(fields, spec.InspectField{Label: "data", Value: strings.Join(pairs, ", ")})
	}
	if op.verify != "" {
		fields = append(fields, spec.InspectField{Label: "verify", Value: op.verify})
	}
	return fields
}
