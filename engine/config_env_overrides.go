package engine

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"cuelang.org/go/cue"
	"godoit.dev/doit/diagnostic"
	"godoit.dev/doit/diagnostic/event"
	"godoit.dev/doit/errs"
	"godoit.dev/doit/signal"
	"godoit.dev/doit/source"
)

func extractEnvMap(v cue.Value) map[string]string {
	res := make(map[string]string)

	iter, _ := v.Fields(cue.Optional(true))
	for iter.Next() {
		field := iter.Selector().String()
		envVar := extractAttr(iter.Value(), cueAttrEnv)
		if envVar != "" {
			res[field] = envVar
		}
	}
	return res
}

// fillNonConcreteFromEnv fills non-concrete CUE fields from environment variables.
// This is called BEFORE validation to handle "env-only" fields like `host: string @env(VAR)`.
// Concrete values are left unchanged (they'll be overridden after decode).
func fillNonConcreteFromEnv(v cue.Value, envMap map[string]string, src source.Source) (cue.Value, error) {
	var diags diagnostic.Diagnostics

	for field, envVar := range envMap {
		envVal, ok := src.LookupEnv(envVar)
		if !ok {
			continue
		}

		path := cue.ParsePath(field)
		fieldVal := v.LookupPath(path)

		// Skip if already concrete - will be handled after decode
		if fieldVal.IsConcrete() {
			continue
		}

		kind := fieldVal.IncompleteKind()
		parsed, diag := parseEnvValForKind(kind, envVar, envVal)
		if diag != nil {
			diags = append(diags, diag)
			continue
		}

		v = v.FillPath(path, parsed)
	}

	if len(diags) > 0 {
		return v, diags
	}
	return v, nil
}

func parseEnvValForKind(kind cue.Kind, envVar, envVal string) (any, diagnostic.Diagnostic) {
	switch {
	case kind.IsAnyOf(cue.StringKind):
		return envVal, nil

	case kind.IsAnyOf(cue.IntKind):
		n, err := strconv.ParseInt(envVal, 0, 64)
		if err != nil {
			return nil, InvalidEnvVar{Key: envVar, Value: envVal, Kind: "int", Err: err}
		}
		return n, nil

	case kind.IsAnyOf(cue.FloatKind):
		f, err := strconv.ParseFloat(envVal, 64)
		if err != nil {
			return nil, InvalidEnvVar{Key: envVar, Value: envVal, Kind: "float", Err: err}
		}
		return f, nil

	case kind.IsAnyOf(cue.BoolKind):
		b, err := strconv.ParseBool(envVal)
		if err != nil {
			return nil, InvalidEnvVar{Key: envVar, Value: envVal, Kind: "bool", Err: err}
		}
		return b, nil
	}

	return envVal, nil
}

// applyEnvOverridesToStruct applies environment variable overrides directly to a Go struct.
// This is called AFTER CUE decoding to override concrete values in user config.
func applyEnvOverridesToStruct(cfg any, envMap map[string]string, src source.Source) error {
	rv := reflect.ValueOf(cfg)
	if rv.Kind() != reflect.Pointer || rv.Elem().Kind() != reflect.Struct {
		return nil
	}
	rv = rv.Elem()
	rt := rv.Type()

	var diags diagnostic.Diagnostics
	for i := 0; i < rt.NumField(); i++ {
		field := rt.Field(i)
		fieldName := strings.ToLower(field.Name)

		envVar, ok := envMap[fieldName]
		if !ok {
			continue
		}

		envVal, ok := src.LookupEnv(envVar)
		if !ok {
			continue
		}

		fv := rv.Field(i)
		if !fv.CanSet() {
			continue
		}

		switch field.Type.Kind() {
		case reflect.String:
			fv.SetString(envVal)

		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			n, err := strconv.ParseInt(envVal, 0, 64)
			if err != nil {
				diags = append(diags, InvalidEnvVar{
					Key:   envVar,
					Value: envVal,
					Kind:  "int",
					Err:   errs.UnwrapAll(err),
				})
				continue
			}
			fv.SetInt(n)

		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			n, err := strconv.ParseUint(envVal, 0, 64)
			if err != nil {
				diags = append(diags, InvalidEnvVar{
					Key:   envVar,
					Value: envVal,
					Kind:  "uint",
					Err:   errs.UnwrapAll(err),
				})
				continue
			}
			fv.SetUint(n)

		case reflect.Float32, reflect.Float64:
			f, err := strconv.ParseFloat(envVal, 64)
			if err != nil {
				diags = append(diags, InvalidEnvVar{
					Key:   envVar,
					Value: envVal,
					Kind:  "float",
					Err:   errs.UnwrapAll(err),
				})
				continue
			}
			fv.SetFloat(f)

		case reflect.Bool:
			b, err := strconv.ParseBool(envVal)
			if err != nil {
				diags = append(diags, InvalidEnvVar{
					Key:   envVar,
					Value: envVal,
					Kind:  "bool",
					Err:   errs.UnwrapAll(err),
				})
				continue
			}
			fv.SetBool(b)
		}
	}

	if len(diags) > 0 {
		return diags
	}
	return nil
}

type InvalidEnvVar struct {
	Key   string
	Value string
	Kind  string
	Err   error
}

func (e InvalidEnvVar) Error() string {
	return fmt.Sprintf("invalid environment variable %q (%q): %v", e.Key, e.Value, e.Err)
}

func (e InvalidEnvVar) EventTemplate() event.Template {
	return event.Template{
		ID:   "env.InvalidEnvVar",
		Text: `failed to parse ENV "{{.Key}}"`,
		Hint: `"{{.Value}}" could not be parsed to {{.Kind}}`,
		Help: `{{- if .Err}}underlying error was: {{.Err}}{{end}}`,
		Data: e,
	}
}

func (InvalidEnvVar) Severity() signal.Severity { return signal.Warning }
func (InvalidEnvVar) Impact() diagnostic.Impact { return diagnostic.ImpactAbort }
