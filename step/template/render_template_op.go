package template

import (
	"bytes"
	"context"
	"path/filepath"
	"text/template"

	"godoit.dev/doit/capability"
	"godoit.dev/doit/source"
	"godoit.dev/doit/spec"
	"godoit.dev/doit/step/sharedops"
	"godoit.dev/doit/target"
)

type renderTemplateOp struct {
	sharedops.BaseOp
	src     string
	content string
	dest    string
	data    DataConfig
}

func (op *renderTemplateOp) Check(ctx context.Context, src source.Source, tgt target.Target) (spec.CheckResult, error) {
	// Merge data with env overrides
	data, err := mergeData(op.data, src)
	if err != nil {
		return spec.CheckUnsatisfied, err
	}

	// Get template content
	tmplContent, err := op.getTemplateContent(ctx, src)
	if err != nil {
		return spec.CheckUnsatisfied, err
	}

	// Parse template (catches syntax errors early)
	tmpl, err := template.New("template").Parse(string(tmplContent))
	if err != nil {
		return spec.CheckUnsatisfied, TemplateParseError{
			Err:    err,
			Source: op.SrcSpan,
		}
	}

	// Render template to check for execution errors
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return spec.CheckUnsatisfied, TemplateExecError{
			Err:    err,
			Source: op.SrcSpan,
		}
	}

	// Destination parent must exist
	if _, err := tgt.Stat(ctx, filepath.Dir(op.dest)); err != nil {
		return spec.CheckUnsatisfied, DestDirMissing{
			Path:   filepath.Dir(op.dest),
			Err:    err,
			Source: op.DestSpan,
		}
	}

	// Compare with existing file
	destData, err := tgt.ReadFile(ctx, op.dest)
	if err != nil {
		return spec.CheckUnsatisfied, nil // expected drift
	}

	if !bytes.Equal(buf.Bytes(), destData) {
		return spec.CheckUnsatisfied, nil // expected drift
	}

	return spec.CheckSatisfied, nil
}

func (op *renderTemplateOp) Execute(ctx context.Context, src source.Source, tgt target.Target) (spec.Result, error) {
	// Merge data with env overrides
	data, err := mergeData(op.data, src)
	if err != nil {
		return spec.Result{}, err
	}

	// Get template content
	tmplContent, err := op.getTemplateContent(ctx, src)
	if err != nil {
		return spec.Result{}, err
	}

	// Parse and execute template
	tmpl, err := template.New("template").Parse(string(tmplContent))
	if err != nil {
		return spec.Result{}, err
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return spec.Result{}, err
	}

	// Check if content matches existing file
	destData, err := tgt.ReadFile(ctx, op.dest)
	if err == nil && bytes.Equal(buf.Bytes(), destData) {
		return spec.Result{Changed: false}, nil
	}

	// Write rendered content
	if err := tgt.WriteFile(ctx, op.dest, buf.Bytes(), 0o644); err != nil {
		return spec.Result{}, err
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
		return nil, TemplateSourceMissing{
			Path:   op.src,
			Err:    err,
			Source: op.SrcSpan,
		}
	}

	return data, nil
}

// mergeData merges values with environment variable overrides.
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
			return data, EnvKeyNotInValues{
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
		ID:   "builtin.render-template",
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
