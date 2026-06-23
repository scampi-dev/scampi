# Help
# -----------------------------------------------------------------------------

[default]
[doc("Show this help message")]
@help *args:
  just --unsorted --list test {{args}}

# Broad
# -----------------------------------------------------------------------------

[group('broad')]
[doc("Run all tests (pass 'quiet' to suppress passing output)")]
all mode='':
  #!/usr/bin/env bash
  case "{{mode}}" in
    quiet) just test _q 'go test ./...' ;;
    *)     go test ./... ;;
  esac

[group('broad')]
[doc("Shortcut for 'just test all quiet'")]
quiet:
  just test all quiet

[group('broad')]
[doc("Run all tests with containers")]
containers:
  SCAMPI_TEST_CONTAINERS=1 go test -v ./test/...

# Fuzz funcs are NOT run with -fuzz here: plain `go test` already
# executes their f.Add() seeds (and any saved corpus) as deterministic
# subtests during `go test -race -v ./...`, which catches "fuzz code is
# broken" without doing exploratory fuzzing. Real fuzzing belongs in
# scheduled jobs or `just test fuzz <dur>`.

[group('broad')]
[doc("Run everything except container-gated tests (pre-commit gate; pass 'quiet' to suppress passing output)")]
nocontainers mode='':
  #!/usr/bin/env bash
  set -e
  case "{{mode}}" in
    quiet)
      just test _q 'go test -race -v ./...'
      just test _q 'just test testkit'
      just test _q 'go test ./test/integration -bench=. -benchmem -count=1 -benchtime=1x'
      ;;
    *)
      go test -race -v ./...
      just test testkit
      go test ./test/integration -bench=. -benchmem -count=1 -benchtime=1x
      ;;
  esac

[group('broad')]
[doc("Run all tests with race-detector and containers (pre-push gate; pass 'quiet' to suppress passing output)")]
everything mode='':
  #!/usr/bin/env bash
  set -e
  case "{{mode}}" in
    quiet)
      just test _q 'SCAMPI_TEST_CONTAINERS=1 go test -race -v ./...'
      just test _q 'just test testkit'
      just test _q 'go test ./test/integration -bench=. -benchmem -count=1 -benchtime=1x'
      ;;
    *)
      SCAMPI_TEST_CONTAINERS=1 go test -race -v ./...
      just test testkit
      go test ./test/integration -bench=. -benchmem -count=1 -benchtime=1x
      ;;
  esac

# Run a command and show output only on failure. Used by the quiet
# variants above.
[private]
_q cmd:
  #!/usr/bin/env bash
  out=$(eval '{{cmd}}' 2>&1)
  rc=$?
  if [[ $rc -ne 0 ]]; then
    echo "$out" | grep -E '^(FAIL|---|panic|\s+--- FAIL|\s+\S+\.go:|\s+Error:|\s+Got:|\s+Want:)' || true
  fi
  exit $rc

# Focused
# -----------------------------------------------------------------------------

[group('focused')]
[doc("Run unit tests (per-package, excludes integration)")]
unit:
  go test $(go list ./... | grep -v scampi.dev/scampi/test)

[group('focused')]
[doc("Run integration tests")]
integration:
  go test -v ./test/integration/...

[group('focused')]
[doc("Run tests with race-detector")]
race:
  go test -race ./...

[group('focused')]
[doc("Run SSH tests (requires containers)")]
ssh:
  SCAMPI_TEST_CONTAINERS=1 go test -v ./test/ssh/...

[group('focused')]
[doc("Run scampi testkit tests (scampi test framework)")]
testkit:
  go run ./cmd/scampi test test/testdata/testkit/...

# Fuzz
# -----------------------------------------------------------------------------

[group('fuzz')]
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

# Coverage
# -----------------------------------------------------------------------------

[group('coverage')]
[doc("Show test coverage by package -- runs the full suite (incl. containers) with -coverpkg, filters generated/main files, summary sorted by coverage ascending (filter: grep pattern)")]
coverage filter='':
  #!/usr/bin/env bash
  set -euo pipefail
  pkglist=$(go list ./... | grep -v '/bin/')
  if [[ -n "{{filter}}" ]]; then
    pkglist=$(echo "$pkglist" | grep "{{filter}}")
  fi
  SCAMPI_TEST_CONTAINERS=1 go test -coverpkg=./... -coverprofile=coverage.raw.out $pkglist >/dev/null
  # Drop generated stringer files and bare CLI entrypoints -- noise in
  # the HTML view, and skews per-package numbers. The mode: header
  # doesn't match the pattern and survives.
  grep -v -E '(_string\.go|cmd/scampi/main\.go):' coverage.raw.out > coverage.out
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
    | sort -k2 -n
  echo
  go tool cover -func=coverage.out | tail -1

[group('coverage')]
[doc("Per-function coverage from coverage.out, sorted by coverage ascending (filter: grep pattern; run `just test coverage` first)")]
coverage-func filter='':
  #!/usr/bin/env bash
  set -euo pipefail
  if [[ ! -f coverage.out ]]; then
    echo "coverage.out not found -- run 'just test coverage' first" >&2
    exit 1
  fi
  if [[ -n "{{filter}}" ]]; then
    go tool cover -func=coverage.out | grep "{{filter}}" | sort -k3 -n
  else
    go tool cover -func=coverage.out | grep -v '^total:' | sort -k3 -n
    go tool cover -func=coverage.out | grep '^total:'
  fi

[group('coverage')]
[doc("Open HTML coverage report from coverage.out (run `just test coverage` first)")]
coverage-html:
  #!/usr/bin/env bash
  set -euo pipefail
  if [[ ! -f coverage.out ]]; then
    echo "coverage.out not found -- run 'just test coverage' first" >&2
    exit 1
  fi
  go tool cover -html=coverage.out
