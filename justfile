GOLANGCI_LINT_VERSION := `go list -m -f '{{.Version}}' github.com/golangci/golangci-lint/v2`
BENCHSTAT_VERSION     := `go list -m -f '{{.Version}}' golang.org/x/perf`

build_dir := "./build"
bin_dir   := f"{{build_dir}}/bin"
bin_path  := f"{{bin_dir}}/doit"
bench_dir := "./benchmarks"


[default]
[doc("Show this help message")]
@help:
  just --list

[doc("Install prerequisites")]
install-prereqs:
  go mod tidy
  go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@{{GOLANGCI_LINT_VERSION}}
  go install golang.org/x/perf/cmd/benchstat@{{BENCHSTAT_VERSION}}

[doc("Run code generation")]
generate:
  go generate ./...

[doc("Build the doit CLI binary")]
build:
  mkdir -p {{bin_dir}}
  go build -o {{bin_path}} ./cmd

[doc("Build and run doit locally")]
doit *args:
  go run ./cmd {{args}}

# Testing
# ##############################################################################

[doc("Run tests")]
test:
  go test ./...

[doc("Run tests against containers")]
test-containers:
  DOIT_TEST_CONTAINERS=1 go test -v ./test/...

[doc("Run all tests with race-detector and containers")]
test-all:
  DOIT_TEST_CONTAINERS=1 go test -race -v ./...

[doc("Run tests with race-detector")]
race:
  go test -race ./...

[doc("Run fuzz tests (optional package filter, e.g. just fuzz 10m test)")]
fuzz time='30s' pkg='':
  #!/usr/bin/env bash
  set -euo pipefail
  if [[ -n "{{pkg}}" ]]; then
    pkgs="{{pkg}}"
  else
    pkgs=$(grep -rl '^func Fuzz' --include='*_test.go' . \
      | xargs -n1 dirname | sort -u \
      | sed 's|^\./||')
  fi
  for pkg in $pkgs; do
    echo "fuzzing ./$pkg ({{time}})..."
    go test "./$pkg" -fuzz=. -fuzztime={{time}}
  done

[doc("Run benchmarks")]
bench save_as='' count='10' benchtime='100ms':
  {{
    if save_as == '' {
      f"go test ./test -bench=. -benchmem -count={{count}} -benchtime={{benchtime}}"
    } else {
      f'''
      mkdir -p "{{bench_dir}}"
      echo 'Warmup run...'
      go test ./test -bench=. -benchmem -count=2 -benchtime={{benchtime}}
      echo 'Recorded run...'
      go test ./test -bench=. -benchmem -count={{count}} -benchtime={{benchtime}} \
        | tee "{{bench_dir}}/$(date '+%Y-%m-%dT%H%M')-{{save_as}}.txt"
      '''
    }
  }}

[doc("Compare the latest 2 existing benchmarks with suffix")]
benchcomp suffix:
  curr=`ls {{bench_dir}}/*-{{suffix}}.txt | sort | tail -n 1`; \
  prev=`ls {{bench_dir}}/*-{{suffix}}.txt | sort | tail -n 2 | head -n 1`; \
  echo "Comparing:"; \
  echo "  $prev"; \
  echo "  $curr"; \
  benchstat -table .config -row .fullname -col .file "$prev" "$curr"

[doc("Plot available ns/op (geometric mean) benchmarks with suffix")]
benchplot suffix:
  pushd bin/benchplot/ \
    && rm -f *.csv \
    && rm -f *.svg \
    && go run benchplot.go ../../{{bench_dir}}/*{{suffix}}.txt > bench.csv \
    && gnuplot bench.gnuplot

[doc("Format all code")]
fmt:
  go fmt ./...

[doc("Lint project")]
lint:
  golangci-lint run
  just license-check

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
