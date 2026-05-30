# Constants
# -----------------------------------------------------------------------------

build_dir       := "./build"
bin_dir         := f"{{build_dir}}/bin"
required_tools  := "shellcheck jq curl"
cross_targets   := "linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 freebsd/amd64 freebsd/arm64"

# Help
# -----------------------------------------------------------------------------

[default]
[doc("Show this help message")]
@help *args:
  just --unsorted --list {{args}}

# Modules
# -----------------------------------------------------------------------------

[group('modules')]
[doc("Run tests")]
mod test

# Build
# -----------------------------------------------------------------------------

version  := `git describe --tags --always --dirty 2>/dev/null || echo dev`
ldflags  := "-s -w -X main.version=" + version

[group("build")]
[doc("Build the scampi binary")]
build:
  #!/usr/bin/env bash
  set -euo pipefail
  mkdir -p {{bin_dir}}
  out="{{bin_dir}}/scampi${GOOS:+-${GOOS}-${GOARCH}}"
  CGO_ENABLED=0 go build -trimpath -ldflags '{{ldflags}}' -o "$out" .

[group('build')]
[doc("Cross-compile for all supported platforms")]
cross:
  #!/usr/bin/env bash
  set -uo pipefail
  fail=0
  for pair in {{cross_targets}}; do
    os="${pair%/*}"
    arch="${pair#*/}"
    printf '  %-13s' "${os}/${arch}"
    if out=$(GOOS="${os}" GOARCH="${arch}" just build 2>&1); then
      echo ' ... OK'
    else
      echo ' ... FAIL'
      printf '    %s\n' "${out//$'\n'/$'\n'    }"
      fail=1
    fi
  done
  exit "$fail"

# Run
# -----------------------------------------------------------------------------

[group('run')]
[doc("Build and run scampi locally")]
scampi *args:
  go run . {{args}}

# Code quality
# -----------------------------------------------------------------------------

[group('quality')]
[doc("Format all code")]
fmt:
  go fmt ./...

[group("quality")]
[doc("Lint project")]
lint:
  @./scripts/lint.sh

[group('quality')]
[doc("Check SPDX license headers")]
license-check:
  @./scripts/license-check.sh

# Dependencies
# -----------------------------------------------------------------------------

[group('deps')]
[doc("Install dev dependencies")]
@setup:
  ./scripts/setup.sh {{required_tools}}

# Diagnostics & Cleanup
# -----------------------------------------------------------------------------

[group('cleanup')]
[doc("Clean project")]
clean:
  rm -rf {{build_dir}}
  go clean -testcache
