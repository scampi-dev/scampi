# Modules
# -----------------------------------------------------------------------------

[group('modules')]
[doc("Run tests (just test --list for subcommands)")]
mod test

[group('modules')]
[doc("Site build/dev (just site --list for subcommands)")]
mod site

[group('modules')]
[doc("Release management (changelog, version bump, tag)")]
mod release 'release.just'

[group('modules')]
[doc("GitHub issue/PR/milestone helpers via the gh CLI")]
mod gh 'github.just'

[group('modules')]
[doc("Issue intake pipeline: eval-inbox -> claude QA -> pusher-inbox -> GitHub")]
mod issues 'issues.just'

# Constants
# -----------------------------------------------------------------------------

build_dir       := "./build"
bin_dir         := f"{{build_dir}}/bin"
spdx_header     := "// SPDX-License-Identifier: GPL-3.0-only"
required_tools  := "shellcheck jq curl"
cross_targets   := "linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 freebsd/amd64 freebsd/arm64"

# Help
# -----------------------------------------------------------------------------

[default]
[doc("Show this help message")]
@help *args:
  just --unsorted --list {{args}}

# Build
# -----------------------------------------------------------------------------

version  := `git describe --tags --always --dirty 2>/dev/null || echo dev`
ldflags  := "-s -w -X main.version=" + version

[group('build')]
[doc("Build scampi and scampls binaries")]
build:
  mkdir -p {{bin_dir}}
  CGO_ENABLED=0 go build -trimpath -ldflags '{{ldflags}}' -o {{bin_dir}}/scampi  ./cmd/scampi
  CGO_ENABLED=0 go build -trimpath -ldflags '{{ldflags}}' -o {{bin_dir}}/scampls ./cmd/scampls

[group('build')]
[doc("Cross-compile for all supported platforms (outdir=DIR)")]
cross outdir=bin_dir:
  #!/usr/bin/env bash
  set -euo pipefail
  mkdir -p "{{outdir}}"
  for pair in {{cross_targets}}; do
    os="${pair%/*}"
    arch="${pair#*/}"
    for bin in scampi scampls; do
      out="{{outdir}}/${bin}-${os}-${arch}"
      printf "  %-20s -> %s\n" "${bin} ${os}/${arch}" "$out"
      CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build -trimpath -ldflags '{{ldflags}}' -o "$out" ./cmd/${bin}
    done
  done

[group('build')]
[doc("Generate manpages from markdown sources")]
manpages:
  #!/usr/bin/env bash
  set -euo pipefail
  out="{{build_dir}}/man/man1"
  mkdir -p "$out"
  for src in doc/man/*.1.md; do
    [ -f "$src" ] || continue
    name=$(basename "$src" .md)
    go tool github.com/cpuguy83/go-md2man/v2 -in "$src" -out "$out/$name"
    echo "  $name"
  done

[group('build')]
[doc("Run code generation")]
generate:
  go generate ./...
  just _patch-license-headers

# Prepend SPDX header to generated files that go generate overwrites.
# The find commands here MUST match scripts/license-check.sh.
[private]
_patch-license-headers:
  #!/usr/bin/env bash
  set -euo pipefail
  go_header="{{spdx_header}}"
  scampi_header="// SPDX-License-Identifier: GPL-3.0-only"
  patch() {
    local f="$1" header="$2"
    first=$(head -1 "$f")
    if [[ "$first" != "$header" ]]; then
      tmp=$(mktemp)
      { echo "$header"; echo; cat "$f"; } > "$tmp"
      mv "$tmp" "$f"
    fi
  }
  while IFS= read -r f; do patch "$f" "$go_header"; done \
    < <(find . -name '*_string.go' -not -path './vendor/*')
  while IFS= read -r f; do
    [[ -z "$f" ]] && continue
    patch "$f" "$scampi_header"
  done < <(find . -name '*.scampi' -not -path './.sandbox/*' -not -path '*/testdata/*')

# Run
# -----------------------------------------------------------------------------

[group('run')]
[doc("Build and run scampi locally")]
scampi *args:
  go run ./cmd/scampi {{args}}

[group('run')]
[doc("Run scampls (LSP server) locally")]
scampls *args:
  go run ./cmd/scampls {{args}}

[group('run')]
[doc("Serve markdown files in a browser (default: .sandbox/ on :7080)")]
mdserve *args:
  go run ./bin/mdserve {{args}}

# Code quality
# -----------------------------------------------------------------------------

[group('quality')]
[doc("Format all code")]
fmt:
  go fmt ./...
  ./scripts/fix-markdown-tables.py
  just scampi fmt ./...

[group('quality')]
[doc("Lint project (severity: warning|hint)")]
lint severity='warning':
  go tool golangci-lint run
  go tool gomarklint
  go test -run 'TestMarkdownTableAlignment|TestFuncSignatureStyle|TestBareErrorBan' ./test/rules/
  shellcheck **/*.sh
  just license-check
  just _gopls-hints {{severity}}

[group('quality')]
[doc("Find suspicious unicode characters in code/docs")]
find-unicode:
  ./scripts/find-unicode.py

[group('quality')]
[doc("Check SPDX license headers")]
license-check:
  ./scripts/license-check.sh

[private]
_gopls-hints severity:
  #!/usr/bin/env bash
  [[ "{{severity}}" != "hint" ]] && exit 0
  files=$(find . -name '*.go' -not -name '*_test.go' -not -path './vendor/*' -not -path './.git/*')
  hints=$(echo "$files" | xargs gopls check -severity=hint 2>&1 | grep -v '^$')
  if [[ -n "$hints" ]]; then
    echo ""
    echo "gopls hints:"
    echo "$hints"
  fi

# Dependencies
# -----------------------------------------------------------------------------

[group('deps')]
[doc("Install external build/lint dependencies")]
setup:
  #!/usr/bin/env bash
  set -euo pipefail
  missing=()
  for cmd in {{required_tools}}; do
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

[group('deps')]
[doc("Check for outdated direct dependencies")]
outdated:
  @./scripts/go-outdated.sh

[group('deps')]
[doc("Upgrade all direct dependencies to latest")]
upgrade:
  @./scripts/go-upgrade.sh

# Diagnostics & cleanup
# -----------------------------------------------------------------------------

[group('cleanup')]
[doc("Analyze binary sizes")]
bin-size:
  ./scripts/bin-size.sh

[group('cleanup')]
[doc("Clean project")]
clean:
  rm -rf {{build_dir}}
  go clean -testcache
