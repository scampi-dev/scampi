---
title: Publishing
weight: 2
---

A scampi module is a git repository containing `.scampi` files. Publishing
is as simple as tagging a release.

## Repository layout

```text
your-module/
    _index.scampi          # entry point (or <module-name>.scampi)
    helpers.scampi         # internal helpers
    sub/
        _index.scampi      # subpath entry point
    scampi.mod             # optional: only needed if this module has dependencies
    README.md
```

## Naming

The module path is the repository URL without `https://` and `.git`:

```text
codeberg.org/scampi-modules/npm
github.com/yourname/scampi-webserver
```

Choose a path that's stable — consumers reference it in their `scampi.mod`
and `import` statements.

### Monorepo modules

Multiple modules can live in subdirectories of a single git repository.
The module path extends beyond the repo path:

```text
codeberg.org/yourname/scampi-modules/npm       → repo/npm/
codeberg.org/yourname/scampi-modules/authelia   → repo/authelia/
```

scampi resolves this automatically — it probes progressively shorter
paths until it finds a valid git repo, then treats the remaining
segments as a subdirectory. Each subdirectory has its own entry point
(`_index.scampi` or `<name>.scampi`) and optional `scampi.mod`.

### Vanity import paths

You can host modules under a custom domain, even if the git repository
lives elsewhere. This works the same way as Go's `go-import` meta tag.

When scampi can't find a git repo at any path prefix, it fetches
`https://<module-path>?scampi-get=1` and looks for a meta tag that maps
the path to the real repository:

```html
<meta name="scampi-import"
      content="scampi.dev/modules git https://codeberg.org/scampi-dev/scampi-modules.git">
```

The `content` attribute has three space-separated fields:

| Field    | Description                                        |
| -------- | -------------------------------------------------- |
| `prefix` | Module path prefix (must match the requested path) |
| `vcs`    | Version control system (must be `git`)             |
| `url`    | Clone URL for the actual repository                |

When the module path extends beyond the prefix (e.g.
`scampi.dev/modules/npm` with prefix `scampi.dev/modules`), the
remainder is treated as a subdirectory within the repo — just like
monorepo modules.

The `scampi.mod` always uses the vanity path — consumers never see the
underlying repository URL. This lets you move between forges without
breaking downstream modules.

If no meta tag is found, scampi falls back to cloning
`https://<module-path>.git` directly.

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

- Use `_index.scampi` for the main entry point
- Export public declarations and functions at the top level
- Keep internal helpers in separate files, imported relatively

## Dependencies

If your module depends on other modules, include a `scampi.mod`:

```text {filename="scampi.mod"}
module codeberg.org/yourname/my-module

require (
    codeberg.org/scampi-modules/helpers v1.0.0
)
```

Consumers don't need to declare your transitive dependencies — they only
list what they directly `import`.

## Testing

Include `*_test.scampi` files in your module repository. Consumers and
CI pipelines can run them with `scampi test`:

```text
scampi test ./...
```

See [Testing]({{< relref "../testing" >}}) for details on writing test files.
