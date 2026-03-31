// SPDX-License-Identifier: GPL-3.0-only

package mod

import (
	"context"
	"fmt"
	"strings"

	"scampi.dev/scampi/source"
)

// WriteModFile serialises module and deps to path in canonical scampi.mod format.
func WriteModFile(ctx context.Context, src source.Source, path, module string, deps []Dependency) error {
	var sb strings.Builder
	sb.WriteString("module ")
	sb.WriteString(module)
	sb.WriteString("\n")

	var direct, indirect []Dependency
	for _, dep := range deps {
		if dep.Indirect {
			indirect = append(indirect, dep)
		} else {
			direct = append(direct, dep)
		}
	}

	writeRequireBlock(&sb, direct, false)
	writeRequireBlock(&sb, indirect, true)

	if err := src.WriteFile(ctx, path, []byte(sb.String())); err != nil {
		return &WriteError{
			Detail: fmt.Sprintf("could not write scampi.mod: %v", err),
			Hint:   "check file permissions",
		}
	}
	return nil
}

func writeRequireBlock(sb *strings.Builder, deps []Dependency, indirect bool) {
	if len(deps) == 0 {
		return
	}
	sb.WriteString("\nrequire (\n")
	for _, dep := range deps {
		sb.WriteString("\t")
		sb.WriteString(dep.Path)
		sb.WriteString(" ")
		sb.WriteString(dep.Version)
		if indirect {
			sb.WriteString(" // indirect")
		}
		sb.WriteString("\n")
	}
	sb.WriteString(")\n")
}
