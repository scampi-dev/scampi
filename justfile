GOLANGCI_LINT_VERSION := `go list -m -f '{{.Version}}' github.com/golangci/golangci-lint/v2`

build_dir := "./build"
bin_dir   := f"{{build_dir}}/bin"
bin_path  := f"{{bin_dir}}/doit"


[default]
[doc("Show this help message")]
@help:
  just --list

[doc("Install prerequisites")]
install-prereqs:
  go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@{{GOLANGCI_LINT_VERSION}}

[doc("Build the doit CLI binary")]
@build:
  mkdir -p {{bin_dir}}
  go build -o {{bin_path}} ./cmd

[doc("Build and run doit locally")]
@doit *args:
  go run ./cmd {{args}}

[doc("Format all go code")]
@fmt:
  go fmt ./...

[doc("Lint project")]
@lint:
  golangci-lint run

[doc("Clean project")]
@clean:
  rm -rf {{build_dir}}
  go clean -testcache
