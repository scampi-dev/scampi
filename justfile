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

version  := `git describe --tags --always --dirty 2>/dev/null || echo dev`
ldflags  := "-s -w -X main.version=" + version

[doc("Build the scampi CLI binary")]
build:
  mkdir -p {{bin_dir}}
  go build -ldflags '{{ldflags}}' -o {{bin_path}} ./cmd

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
  shellcheck scripts/*.sh
  just license-check

[doc("Site build/dev (just site --list for subcommands)")]
mod site

[doc("Codeberg repo management")]
mod cb 'codeberg.just'

[doc("Install external build/lint dependencies")]
setup:
  #!/usr/bin/env bash
  set -euo pipefail
  missing=()
  for cmd in shellcheck; do
    command -v "$cmd" &>/dev/null || missing+=("$cmd")
  done
  if [[ ${#missing[@]} -eq 0 ]]; then
    echo "All dependencies installed."
    exit 0
  fi
  echo "Missing: ${missing[*]}"
  if command -v brew &>/dev/null; then
    brew install "${missing[@]}"
  elif command -v pacman &>/dev/null; then
    sudo pacman -S "${missing[@]}"
  elif command -v dnf &>/dev/null; then
    sudo dnf install -y "${missing[@]}"
  elif command -v apt-get &>/dev/null; then
    sudo apt-get install -y "${missing[@]}"
  else
    echo "Install manually: ${missing[*]}"
    exit 1
  fi

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
  ./scripts/license-check.sh "{{spdx_header}}"

[doc("Check for outdated direct dependencies")]
outdated:
  @./scripts/go-outdated.sh

[doc("Upgrade all direct dependencies to latest")]
upgrade:
  @./scripts/go-upgrade.sh

[doc("Clean project")]
clean:
  rm -rf {{build_dir}}
  go clean -testcache
