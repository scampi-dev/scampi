build_dir    := "./build"
bin_dir      := f"{{build_dir}}/bin"
bin_path     := f"{{bin_dir}}/scampi"
spdx_header  := "// SPDX-License-Identifier: GPL-3.0-only"


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
  go tool golangci-lint run
  go tool gomarklint
  just license-check

[doc("Site build/dev (just site --list for subcommands)")]
mod site

[doc("Codeberg repo management")]
mod codeberg

# Housekeeping
# ##############################################################################

[doc("Run code generation")]
generate:
  go generate ./...
  just _patch-license-headers

# Prepend SPDX header to generated files that stringer overwrites.
[private]
_patch-license-headers:
  #!/usr/bin/env bash
  set -euo pipefail
  header="{{spdx_header}}"
  while IFS= read -r f; do
    first=$(head -1 "$f")
    if [[ "$first" != "$header" ]]; then
      tmp=$(mktemp)
      { echo "$header"; echo; cat "$f"; } > "$tmp"
      mv "$tmp" "$f"
    fi
  done < <(find . -name '*_string.go' -not -path './vendor/*')

[doc("Check SPDX license headers")]
license-check:
  #!/usr/bin/env bash
  set -euo pipefail
  header="{{spdx_header}}"
  missing=()
  wrong=()
  stray=()
  while IFS= read -r f; do
    first=$(head -1 "$f")
    if [[ "$first" != "$header" ]]; then
      if grep -q 'SPDX-License-Identifier' "$f"; then
        wrong+=("$f  (line 1: $first)")
      else
        missing+=("$f")
      fi
    fi
    count=$(grep -c 'SPDX-License-Identifier' "$f")
    if [[ "$count" -gt 1 ]]; then
      stray+=("$f  (${count} occurrences)")
    fi
  done < <(find . -name '*.go' -not -path './vendor/*')
  ok=true
  if [[ ${#missing[@]} -gt 0 ]]; then
    echo "✗ Missing SPDX header (not present anywhere in file):"
    printf '  %s\n' "${missing[@]}"
    ok=false
  fi
  if [[ ${#wrong[@]} -gt 0 ]]; then
    echo "✗ SPDX header present but not on line 1:"
    printf '  %s\n' "${wrong[@]}"
    ok=false
  fi
  if [[ ${#stray[@]} -gt 0 ]]; then
    echo "✗ Duplicate SPDX headers:"
    printf '  %s\n' "${stray[@]}"
    ok=false
  fi
  if [[ "$ok" == true ]]; then
    echo "✓ All $(find . -name '*.go' -not -path './vendor/*' | wc -l | tr -d ' ') files have correct SPDX headers"
  fi
  [[ "$ok" == true ]]

[doc("Clean project")]
clean:
  rm -rf {{build_dir}}
  go clean -testcache
