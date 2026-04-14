---
title: Module Format
weight: 1
---

## scampi.mod

Every project and module has a `scampi.mod` file at its root. It declares the
module path and lists dependencies.

```scampi-mod {filename="scampi.mod"}
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
- Versions can be semver tags (`v1.0.0`), branch names (`main`),
  or module-prefixed tags (`npm-v0.1.0`)
- Local modules use a filesystem path instead of a version (see [Local Modules](../local))

### Indirect dependencies

When a dependency is required transitively (by one of your direct
dependencies, not by your project itself), scampi marks it with a
`// indirect` comment:

```scampi-mod {filename="scampi.mod"}
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

```scampi-mod {filename="scampi.mod"}
module codeberg.org/yourname/yourproject

require (
    // Nginx Proxy Manager API wrappers
    codeberg.org/scampi-modules/npm v1.0.0
)
```

## scampi.sum

`scampi.sum` is auto-generated and records SHA-256 checksums of downloaded
modules. It ensures that cached modules haven't been tampered with.

```scampi-mod {filename="scampi.sum"}
codeberg.org/scampi-modules/npm v1.0.0 h1:abc123...
codeberg.org/scampi-modules/authelia v0.3.2 h1:def456...
```

Commit `scampi.sum` to version control. It's verified on every `scampi mod download`
and can be checked explicitly with `scampi mod verify`.

## Entry points

When you `import "codeberg.org/user/module"`, scampi looks for an entry
point in the module's root directory:

1. `_index.scampi` — checked first
2. `<module-name>.scampi` — e.g. `npm.scampi` for a module named `npm`

If both exist, `_index.scampi` takes precedence.

### Multi-file modules

A module directory can contain multiple `.scampi` files. All files with the
same `module` declaration are loaded together as one package — like Go
packages. Functions defined in any file are directly callable from any other
file in the same module without import.

```text
npm/
    _index.scampi         module npm  — convergence wrappers
    api.scampi            module npm  — generated request functions
    proxy_host_test.scampi            — test (excluded, _test suffix)
    scampi.mod
```

This is how generated API layers compose with hand-authored convergence
wrappers: `api.scampi` defines raw request functions like
`get_nginx_proxy_hosts(...)`, and `_index.scampi` calls them directly
inside `rest.resource` steps — no import needed between files in the same
module.

Test files (`*_test.scampi`) are excluded from the module scope.

### Implicit self-availability

A module declared by `scampi.mod` is automatically available by its own
path — no self-require needed. If your `scampi.mod` says
`module scampi.dev/modules/npm`, then test files inside the module can
`import "scampi.dev/modules/npm"` without listing it in `require`.

### Subpath imports

You can import from subdirectories within a module:

```scampi
import "codeberg.org/user/module/internal/helpers"
```

This resolves to `internal/helpers.scampi` or `internal/helpers/_index.scampi`
within the module's directory.
