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

// ConvertHealthcheck converts a StructVal produced by container.healthcheck
// into a *target.Healthcheck.
func ConvertHealthcheck(_ string, fields map[string]eval.Value, _ spec.ConvertContext) (any, error) {
	hc := &target.Healthcheck{
		Interval: 30 * time.Second,
		Timeout:  30 * time.Second,
		Retries:  3,
	}
	if c, ok := fields["cmd"].(*eval.StringVal); ok {
		hc.Cmd = c.V
	}
	if i, ok := fields["interval"].(*eval.StringVal); ok {
		if d, err := time.ParseDuration(i.V); err == nil {
			hc.Interval = d
		}
	}
	if t, ok := fields["timeout"].(*eval.StringVal); ok {
		if d, err := time.ParseDuration(t.V); err == nil {
			hc.Timeout = d
		}
	}
	if r, ok := fields["retries"].(*eval.IntVal); ok {
		hc.Retries = int(r.V)
	}
	return hc, nil
}
