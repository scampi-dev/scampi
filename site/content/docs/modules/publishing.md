---
title: Publishing
weight: 2
---

A scampi module is a git repository containing `.star` files. Publishing
is as simple as tagging a release.

## Repository layout

```text
your-module/
    _index.star          # entry point (or <module-name>.star)
    helpers.star         # internal helpers (loaded relatively)
    sub/
        _index.star      # subpath entry point
    scampi.mod           # optional: only needed if this module has dependencies
    README.md
```

## Naming

The module path is the repository URL without `https://` and `.git`:

```text
codeberg.org/scampi-modules/npm
github.com/yourname/scampi-webserver
```

Choose a path that's stable — consumers reference it in their `scampi.mod`
and `load()` calls.

## Versioning

Tag releases with semver: `v1.0.0`, `v0.3.2`, `v2.0.0-alpha.1`.

```text
git tag v1.0.0
git push origin v1.0.0
```

`scampi mod add` resolves the latest **stable** tag (no pre-release suffix).
Users can pin to a specific version including pre-releases by specifying
it explicitly:

```text
scampi mod add codeberg.org/yourname/module@v2.0.0-alpha.1
```

## Entry point conventions

- Use `_index.star` for the main entry point
- Export public functions at the top level
- Keep internal helpers in separate files, loaded relatively:

```starlark {filename="_index.star"}
load("helpers.star", "_validate_config")

def my_step(name, config):
    _validate_config(config)
    return copy(
        desc = name,
        dest = "/etc/myapp/" + name + ".conf",
        src = inline(config),
        perm = "0644",
        owner = "root",
        group = "root",
    )
```

## Dependencies

If your module depends on other modules, include a `scampi.mod`:

```text {filename="scampi.mod"}
module codeberg.org/yourname/my-module

require (
    codeberg.org/scampi-modules/helpers v1.0.0
)
```

Consumers don't need to declare your transitive dependencies — they only
list what they directly `load()`.

## Testing

Include `*_test.star` files in your module repository. Consumers and
CI pipelines can run them with `scampi test`:

```text
scampi test ./...
```

See [Testing](../testing) for details on writing test files.
