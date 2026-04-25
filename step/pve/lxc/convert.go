// SPDX-License-Identifier: GPL-3.0-only

package lxc

import (
	"reflect"

	"scampi.dev/scampi/lang/eval"
	"scampi.dev/scampi/spec"
)

// Converters returns the type converters owned by the pve.lxc step.
func Converters() spec.ConverterMap {
	return spec.ConverterMap{
		reflect.TypeFor[LxcMount](): ConvertMount,
	}
}

// ConvertMount converts a StructVal produced by bind_mount or
// volume_mount into an LxcMount.
func ConvertMount(typeName string, fields map[string]eval.Value, _ spec.ConvertContext) (any, error) {
	m := LxcMount{Backup: true}
	switch typeName {
	case "bind_mount":
		m.Kind = MountBind
		if s, ok := fields["source"].(*eval.StringVal); ok {
			m.Source = s.V
		}
	case "volume_mount":
		m.Kind = MountVolume
		if s, ok := fields["storage"].(*eval.StringVal); ok {
			m.Storage = s.V
		}
		if s, ok := fields["size"].(*eval.StringVal); ok {
			m.Size = s.V
		}
	}
	if s, ok := fields["mountpoint"].(*eval.StringVal); ok {
		m.Mountpoint = s.V
	}
	if b, ok := fields["ro"].(*eval.BoolVal); ok {
		m.ReadOnly = b.V
	}
	if b, ok := fields["backup"].(*eval.BoolVal); ok {
		m.Backup = b.V
	}
	return m, nil
}
