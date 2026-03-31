---
title: Module Format
weight: 1
---

## scampi.mod

Every project and module has a `scampi.mod` file at its root. It declares the
module path and lists dependencies.

```text {filename="scampi.mod"}
module codeberg.org/yourname/yourproject

require (
    codeberg.org/scampi-modules/npm v1.0.0
    codeberg.org/scampi-modules/authelia v0.3.2
)
```

### Module directive

The `module` line declares this project's path. For published modules, this
must match the git repository URL (minus `https://` and `.git`).

### Require block

Each line in the `require` block is a dependency: `<module-path> <version>`.

- Module paths look like repository URLs: `host/org/repo`
- Versions are semver tags: `v1.0.0`, `v2.0.0-alpha.1`
- Local modules use a filesystem path instead of a version (see [Local Modules](../local))

### Indirect dependencies

When a dependency is required transitively (by one of your direct
dependencies, not by your project itself), scampi marks it with a
`// indirect` comment:

```text {filename="scampi.mod"}
module codeberg.org/yourname/yourproject

require (
    codeberg.org/scampi-modules/npm v1.0.0
)

require (
    codeberg.org/scampi-modules/helpers v2.1.0 // indirect
)
```

Indirect deps get their own `require` block, separate from your direct
dependencies. Both blocks are managed automatically by `scampi mod add`
and `scampi mod download` — you should not need to edit them by hand.

When multiple versions of the same transitive module are required,
scampi uses **minimum version selection** — the highest version
requested by any dependency wins.

### Comments

Lines starting with `//` are comments.

```text {filename="scampi.mod"}
module codeberg.org/yourname/yourproject

require (
    // Nginx Proxy Manager API wrappers
    codeberg.org/scampi-modules/npm v1.0.0
)
```

## scampi.sum

`scampi.sum` is auto-generated and records SHA-256 checksums of downloaded
modules. It ensures that cached modules haven't been tampered with.

```text {filename="scampi.sum"}
codeberg.org/scampi-modules/npm v1.0.0 h1:abc123...
codeberg.org/scampi-modules/authelia v0.3.2 h1:def456...
```

Commit `scampi.sum` to version control. It's verified on every `scampi mod download`
and can be checked explicitly with `scampi mod verify`.

## Entry points

When you `load("codeberg.org/user/module", ...)`, scampi looks for an entry
point in the module's root directory:

1. `_index.star` — checked first
2. `<module-name>.star` — e.g. `npm.star` for a module named `npm`

If both exist, `_index.star` takes precedence.

### Subpath loads

You can load from subdirectories within a module:

```starlark
load("codeberg.org/user/module/internal/helpers", "helper_func")
```

This resolves to `internal/helpers.star` or `internal/helpers/_index.star`
within the module's directory.
