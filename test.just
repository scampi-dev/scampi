bench_dir := "./benchmarks"

# Broad
# ##############################################################################

[default]
[doc("Run all tests")]
all:
  go test ./...

[doc("Run unit tests (per-package, excludes integration)")]
unit:
  go test $(go list ./... | grep -v scampi.dev/scampi/test)

[doc("Run integration tests")]
integration:
  go test -v ./test/integration/...

[doc("Run tests with race-detector")]
race:
  go test -race ./...

[doc("Run all tests with containers")]
containers:
  SCAMPI_TEST_CONTAINERS=1 go test -v ./test/...

[doc("Run everything except container-gated tests (pre-commit gate)")]
nocontainers:
  go test -race -v ./...
  just test testkit
  go test ./test/integration -bench=. -benchmem -count=1 -benchtime=1x
  just test fuzz 5s

[doc("Run all tests with race-detector and containers (pre-push gate)")]
everything:
  SCAMPI_TEST_CONTAINERS=1 go test -race -v ./...
  just test testkit
  go test ./test/integration -bench=. -benchmem -count=1 -benchtime=1x
  just test fuzz 5s

# Focused
# ##############################################################################

[doc("Run SSH tests (requires containers)")]
ssh:
  SCAMPI_TEST_CONTAINERS=1 go test -v ./test/ssh/...

[doc("Run scampi testkit tests (scampi test framework)")]
testkit:
  go run ./cmd/scampi test test/testdata/testkit/...

# Fuzz and benchmarks
# ##############################################################################

[doc("Run fuzz tests (optional package filter, e.g. just test fuzz 10m test)")]
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
      f"go test ./test/integration -bench=. -benchmem -count={{count}} -benchtime={{benchtime}}"
    } else {
      f'''
      mkdir -p "{{bench_dir}}"
      latest=$(ls {{bench_dir}}/*.txt 2>/dev/null | sort | tail -1)
      ts=$(date '+%Y-%m-%dT%H%M')
      if [ -n "$latest" ]; then
        prev_ts=$(basename "$latest" | sed 's/-[^-]*\.txt$//')
        if [ ! -f "{{bench_dir}}/$prev_ts-{{save_as}}.txt" ]; then
          ts=$prev_ts
        fi
      fi
      echo 'Warmup run...'
      go test ./test/integration -bench=. -benchmem -count=2 -benchtime={{benchtime}}
      echo 'Recorded run...'
      go test ./test/integration -bench=. -benchmem -count={{count}} -benchtime={{benchtime}} \
        | tee "{{bench_dir}}/$ts-{{save_as}}.txt"
      '''
    }
  }}

[doc("Show test coverage by package — instruments the whole module so cross-package tests are credited (filter: grep pattern)")]
coverage filter='':
  #!/usr/bin/env bash
  set -euo pipefail
  pkglist=$(go list ./... | grep -v '/bin/')
  if [[ -n "{{filter}}" ]]; then
    pkglist=$(echo "$pkglist" | grep "{{filter}}")
  fi
  go test -coverpkg=./... -coverprofile=coverage.out $pkglist >/dev/null
  printf "%-50s %8s\n" "PACKAGE" "COVERAGE"
  printf "%-50s %8s\n" "-------" "--------"
  go tool cover -func=coverage.out \
    | grep -v '^total:' \
    | sed -E 's|/[^/]+\.go:[0-9]+:.*[[:space:]]([0-9.]+%)$| \1|' \
    | awk '
        {
          pkg=$1; sub("scampi.dev/scampi/", "", pkg)
          pct=$2; sub("%", "", pct)
          sum[pkg] += pct; n[pkg]++
        }
        END {
          for (p in sum) printf "%-50s %7.1f%%\n", p, sum[p]/n[p]
        }' \
    | sort
  echo
  go tool cover -func=coverage.out | tail -1

[doc("Per-function coverage from coverage.out (filter: grep pattern; run `just test coverage` first)")]
coverage-func filter='':
  #!/usr/bin/env bash
  set -euo pipefail
  if [[ ! -f coverage.out ]]; then
    echo "coverage.out not found — run 'just test coverage' first" >&2
    exit 1
  fi
  if [[ -n "{{filter}}" ]]; then
    go tool cover -func=coverage.out | grep "{{filter}}"
  else
    go tool cover -func=coverage.out
  fi

[doc("Open HTML coverage report from coverage.out (run `just test coverage` first)")]
coverage-html:
  #!/usr/bin/env bash
  set -euo pipefail
  if [[ ! -f coverage.out ]]; then
    echo "coverage.out not found — run 'just test coverage' first" >&2
    exit 1
  fi
  go tool cover -html=coverage.out

[doc("Compare the latest 2 existing benchmarks with suffix")]
benchcomp suffix:
  curr=`ls {{bench_dir}}/*-{{suffix}}.txt | sort | tail -n 1`; \
  prev=`ls {{bench_dir}}/*-{{suffix}}.txt | sort | tail -n 2 | head -n 1`; \
  echo "Comparing:"; \
  echo "  $prev"; \
  echo "  $curr"; \
  go tool benchstat -table .config -row .fullname -col .file "$prev" "$curr"

[doc("Plot benchmark history as HTML dashboard for given suffix")]
benchplot suffix:
  go run ./bin/benchplot \
    -o ./bin/benchplot/{{suffix}}.html \
    {{bench_dir}}/*-{{suffix}}.txt
  open ./bin/benchplot/{{suffix}}.html
