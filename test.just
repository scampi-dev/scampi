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
  go test -v ./test/...

[doc("Run tests with race-detector")]
race:
  go test -race ./...

[doc("Run all tests with containers")]
containers:
  SCAMPI_TEST_CONTAINERS=1 go test -v ./test/...

[doc("Run all tests with race-detector and containers")]
everything:
  SCAMPI_TEST_CONTAINERS=1 go test -race -v ./...
  just test testkit
  go test ./test -bench=. -benchmem -count=1 -benchtime=1x
  just test fuzz 5s

# Focused
# ##############################################################################

[doc("Run diagnostic validation tests")]
diagnostics:
  go test -v ./test -run TestDiagnostics

[doc("Run drift detection tests")]
drift:
  go test -v ./test -run TestDrift

[doc("Run e2e scenario tests")]
e2e:
  go test -v ./test -run TestE2E

[doc("Run SSH tests (requires containers)")]
ssh:
  SCAMPI_TEST_CONTAINERS=1 go test -v ./test -run TestSSH

[doc("Run scampi testkit tests (scampi test framework)")]
testkit:
  go run ./cmd/scampi test test/testdata/testkit/...

[doc("Run gen api golden tests")]
genapi:
  go test -v ./test -run TestGenAPI

[doc("Run code style rule tests")]
rules:
  go test -v ./test -run 'TestImportCapabilities|TestFuncSignatureStyle|TestMarkdownTableAlignment|TestBareErrorBan|TestLangPackageIsolation|Test_StepType'

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
      f"go test ./test -bench=. -benchmem -count={{count}} -benchtime={{benchtime}}"
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
      go test ./test -bench=. -benchmem -count=2 -benchtime={{benchtime}}
      echo 'Recorded run...'
      go test ./test -bench=. -benchmem -count={{count}} -benchtime={{benchtime}} \
        | tee "{{bench_dir}}/$ts-{{save_as}}.txt"
      '''
    }
  }}

[doc("Show test coverage by package (filter: grep pattern)")]
coverage filter='':
  #!/usr/bin/env bash
  set -euo pipefail
  pkglist=$(go list ./... | grep -v '/bin/')
  if [[ -n "{{filter}}" ]]; then
    pkglist=$(echo "$pkglist" | grep "{{filter}}")
  fi
  printf "%-50s %8s\n" "PACKAGE" "COVERAGE"
  printf "%-50s %8s\n" "-------" "--------"
  echo "$pkglist" | xargs go test -cover 2>&1 \
    | grep -E '^ok|^\?' \
    | while read -r line; do
        pkg=$(echo "$line" | awk '{print $2}' | sed 's|scampi.dev/scampi/||')
        cov=$(echo "$line" | grep -o '[0-9.]*% of' | grep -o '[0-9.]*%' || echo "-")
        if echo "$line" | grep -q '^\?'; then cov="no test"; fi
        printf "%-50s %8s\n" "$pkg" "$cov"
      done

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
