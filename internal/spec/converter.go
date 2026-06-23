// SPDX-License-Identifier: GPL-3.0-only

package spec

import (
	"context"
	"reflect"

	"scampi.dev/scampi/internal/lang/eval"
	"scampi.dev/scampi/internal/source"
)

// TypeConverter converts a StructVal into a Go value for a composable
// config type. Step and target packages register converters for the
// types they own; the linker dispatches to them instead of hardcoding
// type knowledge.
type TypeConverter func(
	typeName string,
	fields map[string]eval.Value,
	ctx ConvertContext,
) (any, error)

// ConvertContext exposes linker internals to type converters without
// leaking the linker's own types.
type ConvertContext struct {
	CfgPath string
	Src     source.Source
	Ctx     context.Context
}

// ConverterMap is the return type of per-package Converters() functions.
// Keys are the reflect.Type of the destination config field.
type ConverterMap = map[reflect.Type]TypeConverter
