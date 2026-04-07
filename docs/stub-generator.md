# Stub Generator

The stub generator reads Go step config structs via reflection and emits
`.scampi` stub files for the scampi-lang type checker and LSP. It is a
build-time tool — its output is consumed by `lang/`, but it has no
dependency on `lang/` or any other scampi internal package.

## How it works

```
Go step config structs (tagged)
    ↓ reflection
gen/langstubs.Generate(configs, writer)
    ↓ emits
std/*.scampi stub files
    ↓ loaded by
lang/resolve → lang/check → LSP
```

The generator takes an opaque `[]any` of config struct pointers. It uses
`reflect` to read struct fields and tags, plus an optional interface for
enum values. The caller (a `go generate` target or standalone tool)
provides the concrete structs — the generator itself imports only stdlib.

## Struct tag contract

Step config structs use the following tags. All are optional except
`step:` which serves as the field description.

| Tag               | Purpose                                           | Example                                      |
| ----------------- | ------------------------------------------------- | -------------------------------------------- |
| `step:"..."`      | Field description (shown in LSP hover/completion) | `step:"Absolute path to ensure exists"`      |
| `optional:"true"` | Field is not required                             | `optional:"true"`                            |
| `default:"..."`   | Default value when field is omitted               | `default:"present"`                          |
| `example:"..."`   | Example values (pipe-separated for multiple)      | `example:"0755\|u=rwx,g=r-x"`                |
| `summary:"..."`   | Step-level description (on anonymous first field) | `summary:"Copy files with owner management"` |
| `exclusive:"..."` | Mutual exclusivity group name                     | `exclusive:"trigger"`                        |

### The anonymous summary field

Every step config struct has an anonymous first field that carries the
`summary` tag:

```go
type CopyConfig struct {
    _ struct{} `summary:"Copy files with owner and permission management"`

    Src    spec.SourceRef `step:"Source" example:"local(\"./config.yaml\")"`
    Dest   string         `step:"Destination file path" example:"/etc/app/config.yaml"`
    Perm   string         `step:"File permissions" example:"0644"`
    Owner  string         `step:"Owner user name or UID" example:"root"`
    Group  string         `step:"Group name or GID" example:"root"`
    Verify string         `step:"Validation command" optional:"true" example:"visudo -cf %s"`
}
```

## Enum provider interface

If a config struct implements this interface, the generator emits enum
type declarations for the fields that have closed value sets:

```go
type EnumProvider interface {
    FieldEnumValues() map[string][]string
}
```

The map keys are field names (matching struct field names, case-
insensitive). The values are the allowed enum variants.

Example implementation:

```go
func (*PkgConfig) FieldEnumValues() map[string][]string {
    return map[string][]string{
        "State": {"present", "absent", "latest"},
    }
}
```

This generates:

```scampi
enum PkgState { present, absent, latest }
```

The enum type name is derived as `StepName + FieldName` (e.g.
`Pkg` + `State` = `PkgState`).

## Go type → scampi-lang type mapping

| Go type                | scampi-lang type      |
| ---------------------- | --------------------- |
| `string`               | `string`              |
| `int`                  | `int`                 |
| `bool`                 | `bool`                |
| `[]string`             | `list[string]`        |
| `map[string]string`    | `map[string, string]` |
| `map[string]any`       | `map[string, any]`    |
| `spec.SourceRef`       | `Source`              |
| `spec.PkgSourceRef`    | `PkgSource`           |
| `*T` (pointer)         | `T?` (optional)       |
| Field with enum values | generated enum type   |
| Custom struct          | `any` (fallback)      |

Pointer fields are treated as optional (`T?`). Fields tagged
`optional:"true"` without a pointer type get `T?` as well.

## Output format

For each step, the generator emits a declaration in the unified
`decl NAME(field: type, ...) OutputType` syntax:

```scampi
# Auto-generated from Go struct tags. Do not edit.

enum PkgState { present, absent, latest }

# Copy files with owner and permission management.
decl copy(
  src:    Source,
  dest:   string,
  perm:   string,
  owner:  string,
  group:  string,
  verify: string?,
  desc:   string?,
) StepInstance
```

- One file per std submodule (`std.scampi`, `std/container.scampi`,
  `std/rest.scampi`, `std/target.scampi`)
- Enum declarations precede the decls that use them
- The `desc` and `on_change` fields are implicit on every decl (the
  generator adds them)
- Output type is `StepInstance` for desired-state decls, `Target` for
  target decls, etc.

## Step kind → output type mapping

The generator needs to know the output type for each step. This is
provided by the caller alongside each config struct:

```go
type StubInput struct {
    Kind       string // "pkg", "copy", "ssh", "deploy", etc.
    Config     any    // pointer to config struct
    OutputType string // "StepInstance", "Target", "Deploy", "SecretsConfig"
}
```

## Usage

```go
//go:generate go run ./cmd/gen-stubs

// In cmd/gen-stubs/main.go:
func main() {
    inputs := []langstubs.StubInput{
        {Kind: "copy", Config: &copy.CopyConfig{}, OutputType: "StepInstance"},
        {Kind: "pkg",  Config: &pkg.PkgConfig{},   OutputType: "StepInstance"},
        {Kind: "ssh",  Config: &ssh.Config{},       OutputType: "Target"},
        // ...
    }
    langstubs.Generate(inputs, os.Stdout)
}
```

## Dependencies

The generator package (`gen/langstubs/`) imports only:

- `reflect`
- `io`
- `strings`
- `sort`

No scampi internal packages. The `EnumProvider` interface is defined
in the generator package itself — it happens to match the method that
step configs already implement, but there is no shared import.
