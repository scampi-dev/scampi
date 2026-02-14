// SPDX-License-Identifier: GPL-3.0-only

//go:build tools

package tools

import (
	_ "github.com/golangci/golangci-lint/v2/cmd/golangci-lint"
	_ "golang.org/x/perf/cmd/benchstat"
)
