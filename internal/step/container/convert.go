// SPDX-License-Identifier: GPL-3.0-only

package container

import (
	"reflect"
	"time"

	"scampi.dev/scampi/internal/lang/eval"
	"scampi.dev/scampi/internal/spec"
	"scampi.dev/scampi/internal/target"
)

// Converters returns the type converters owned by the container step.
func Converters() spec.ConverterMap {
	return spec.ConverterMap{
		reflect.TypeFor[*target.Healthcheck](): ConvertHealthcheck,
	}
}

// ConvertHealthcheck converts a StructVal produced by container.Healthcheck
// into a *target.Healthcheck. Defaults (interval/timeout/retries) are
// declared on the stub type and arrive materialized; an unparseable
// duration is an error, never silently dropped.
func ConvertHealthcheck(_ string, fields map[string]eval.Value, _ spec.ConvertContext) (any, error) {
	hc := &target.Healthcheck{}
	if c, ok := fields["cmd"].(*eval.StringVal); ok {
		hc.Cmd = c.V
	}
	if i, ok := fields["interval"].(*eval.StringVal); ok {
		d, err := time.ParseDuration(i.V)
		if err != nil {
			return nil, InvalidHealthcheckError{Field: "interval", Value: i.V, Err: err}
		}
		hc.Interval = d
	}
	if t, ok := fields["timeout"].(*eval.StringVal); ok {
		d, err := time.ParseDuration(t.V)
		if err != nil {
			return nil, InvalidHealthcheckError{Field: "timeout", Value: t.V, Err: err}
		}
		hc.Timeout = d
	}
	if r, ok := fields["retries"].(*eval.IntVal); ok {
		hc.Retries = int(r.V)
	}
	return hc, nil
}
