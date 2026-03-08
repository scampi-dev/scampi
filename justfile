GOLANGCI_LINT_VERSION := `go list -m -f '{{.Version}}' github.com/golangci/golangci-lint/v2`
BENCHSTAT_VERSION     := `go list -m -f '{{.Version}}' golang.org/x/perf`

build_dir := "./build"
bin_dir   := f"{{build_dir}}/bin"
bin_path  := f"{{bin_dir}}/scampi"


[default]
[doc("Show this help message")]
@help *args:
  just --unsorted --list {{args}}

# Daily drivers
# ##############################################################################

[doc("Build the scampi CLI binary")]
build:
  mkdir -p {{bin_dir}}
  go build -o {{bin_path}} ./cmd

[doc("Build and run scampi locally")]
scampi *args:
  go run ./cmd {{args}}

[doc("Run tests (just test --list for subcommands)")]
mod test

[doc("Format all code")]
fmt:
  go fmt ./...

[doc("Lint project")]
lint:
  golangci-lint run
  just license-check

# Housekeeping
# ##############################################################################

[doc("Run code generation")]
generate:
  go generate ./...

[doc("Install prerequisites")]
install-prereqs:
  go mod tidy
  go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@{{GOLANGCI_LINT_VERSION}}
  go install golang.org/x/perf/cmd/benchstat@{{BENCHSTAT_VERSION}}

[doc("Check SPDX license headers")]
license-check:
  #!/usr/bin/env bash
  set -euo pipefail
  header="// SPDX-License-Identifier: GPL-3.0-only"
  missing=()
  stray=()
  while IFS= read -r f; do
    if [[ "$(head -1 "$f")" != "$header" ]]; then
      missing+=("$f")
    fi
    count=$(grep -c 'SPDX-License-Identifier' "$f")
    if [[ "$count" -gt 1 ]]; then
      stray+=("$f (${count}x)")
    fi
  done < <(find . -name '*.go' -not -path './vendor/*')
  ok=true
  if [[ ${#missing[@]} -gt 0 ]]; then
    echo "Missing SPDX header:"
    printf '  %s\n' "${missing[@]}"
    ok=false
  fi
  if [[ ${#stray[@]} -gt 0 ]]; then
    echo "Duplicate SPDX header:"
    printf '  %s\n' "${stray[@]}"
    ok=false
  fi
  [[ "$ok" == true ]]

[doc("Clean project")]
clean:
  rm -rf {{build_dir}}
  go clean -testcache
