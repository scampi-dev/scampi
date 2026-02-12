package template

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"text/template"

	"godoit.dev/doit/capability"
	"godoit.dev/doit/source"
	"godoit.dev/doit/spec"
	"godoit.dev/doit/step/sharedops"
	"godoit.dev/doit/target"
)

const renderTemplateID = "builtin.render-template"

type renderTemplateOp struct {
	sharedops.BaseOp
	src     string
	content string
	dest    string
	data    DataConfig
}

func (op *renderTemplateOp) Check(
	ctx context.Context, src source.Source, tgt target.Target,
) (spec.CheckResult, []spec.DriftDetail, error) {
	fsTgt := target.Must[target.Filesystem](renderTemplateID, tgt)

	data, err := mergeData(op.data, src)
	if err != nil {
		return spec.CheckUnsatisfied, nil, err
	}

	tmplContent, err := op.getTemplateContent(ctx, src)
	if err != nil {
		return spec.CheckUnsatisfied, nil, err
	}

	tmpl, err := template.New("template").Parse(string(tmplContent))
	if err != nil {
		return spec.CheckUnsatisfied, nil, TemplateParseError{
			Err:    err,
			Source: op.SrcSpan,
		}
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return spec.CheckUnsatisfied, nil, TemplateExecError{
			Err:    err,
			Source: op.SrcSpan,
		}
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
		return spec.CheckUnsatisfied, []spec.DriftDetail{{
			Field:   "content",
			Current: fmt.Sprintf("%d bytes", len(destData)),
			Desired: fmt.Sprintf("%d bytes", buf.Len()),
		}}, nil
	}

	return spec.CheckSatisfied, nil, nil
}

func (op *renderTemplateOp) Execute(ctx context.Context, src source.Source, tgt target.Target) (spec.Result, error) {
	fsTgt := target.Must[target.Filesystem](renderTemplateID, tgt)

	data, err := mergeData(op.data, src)
	if err != nil {
		return spec.Result{}, err
	}

	tmplContent, err := op.getTemplateContent(ctx, src)
	if err != nil {
		return spec.Result{}, err
	}

	tmpl, err := template.New("template").Parse(string(tmplContent))
	if err != nil {
		return spec.Result{}, err
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return spec.Result{}, err
	}

	destData, err := fsTgt.ReadFile(ctx, op.dest)
	if err == nil && bytes.Equal(buf.Bytes(), destData) {
		return spec.Result{Changed: false}, nil
	}

	if err := fsTgt.WriteFile(ctx, op.dest, buf.Bytes()); err != nil {
		if target.IsPermission(err) {
			return spec.Result{}, sharedops.PermissionDeniedError{
				Operation: "write " + op.dest,
				Source:    op.DestSpan,
				Err:       err,
			}
		}
		return spec.Result{}, sharedops.DiagnoseTargetError(err)
	}

	return spec.Result{Changed: true}, nil
}

func (renderTemplateOp) RequiredCapabilities() capability.Capability {
	return capability.Filesystem
}

func (op *renderTemplateOp) getTemplateContent(ctx context.Context, src source.Source) ([]byte, error) {
	if op.content != "" {
		return []byte(op.content), nil
	}

	data, err := src.ReadFile(ctx, op.src)
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

func (op *renderTemplateOp) OpDescription() spec.OpDescription {
	src := op.src
	if src == "" {
		src = "(inline)"
	}
	return renderTemplateDesc{
		Src:  src,
		Dest: op.dest,
	}
}
