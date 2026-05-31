# Help
# -----------------------------------------------------------------------------

[default]
[doc("Show this help message")]
@help *args:
  just --unsorted --list test {{args}}

# Test
# -----------------------------------------------------------------------------

[doc("Run all tests")]
all:
  go test ./...

[doc("Show test coverage per package")]
cover:
  ./scripts/cover.sh
