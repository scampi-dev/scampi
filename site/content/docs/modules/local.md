---
title: Local Modules
weight: 4
---

Local modules let you develop and test modules alongside your project without
publishing to a git repository first.

## Adding a local module

```text
scampi mod add my/helpers ./modules/helpers
```

This adds a local dependency to `scampi.mod` with a filesystem path instead
of a version:

```text {filename="scampi.mod"}
module codeberg.org/yourname/yourproject

require (
    codeberg.org/scampi-modules/npm v1.0.0
    my/helpers ./modules/helpers
)
```

## How it works

- The module path (`my/helpers`) is what you use in `load()` calls
- The filesystem path (`./modules/helpers`) is where the `.scampi` files live
- Relative paths resolve from the directory containing `scampi.mod`
- Absolute paths work too
- No caching, no checksums — files are read directly from disk
- Changes are picked up immediately on the next run

## Using local modules

```starlark {filename="deploy.scampi"}
load("my/helpers", "make_config")

target.local(name = "server")

deploy(
    name = "app",
    targets = ["server"],
    steps = [
        make_config(port = 3000),
    ],
)
```

The `load()` path must match the module path in the require table exactly.

## Entry point resolution

Same rules as remote modules:

1. `_index.scampi` at the module root (checked first)
2. `<last-segment>.scampi` (e.g. `helpers.scampi` for `my/helpers`)

## Transitioning to a published module

When a local module is ready to be shared:

1. Move it to its own git repository
2. Add a `scampi.mod` with the module path matching the repo URL
3. Tag a release: `git tag v1.0.0 && git push origin v1.0.0`
4. Update your project's `scampi.mod`:

```text
# Before (local)
my/helpers ./modules/helpers

# After (remote)
codeberg.org/yourname/helpers v1.0.0
```

5. Update `load()` calls to use the new path
6. Run `scampi mod download`

## Module path flexibility

Local module paths don't need to look like URLs. Any slash-separated path
works:

```text
require (
    my/helpers ./modules/helpers
    infra/common ../shared/infra
)
```

The only requirement: the path in `require` must match the path in `load()`.
