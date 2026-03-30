---
title: mod
weight: 6
---

Manage module dependencies. See [Modules](/docs/modules) for the full guide.

## init

```text
scampi mod init [module-path]
```

Create a `scampi.mod` file in the current directory. If `module-path` is
omitted, it's inferred from the git remote origin URL.

## add

```text
scampi mod add <module[@version]>
scampi mod add <name> <local-path>
```

Add a dependency. For remote modules, resolves the latest stable tag if no
version is specified. For local modules, provide a name and filesystem path.

| Form             | Example                                                  |
| ---------------- | -------------------------------------------------------- |
| Latest stable    | `scampi mod add codeberg.org/user/module`                |
| Explicit version | `scampi mod add codeberg.org/user/module@v1.0.0`         |
| Pre-release      | `scampi mod add codeberg.org/user/module@v2.0.0-alpha.1` |
| Local module     | `scampi mod add my/helpers ./modules/helpers`            |

## tidy

```text
scampi mod tidy
```

Sync the require block with `load()` calls in `*.star` files. Adds missing
entries (with `v0.0.0` placeholder) and removes unreferenced entries.

## download

```text
scampi mod download
```

Fetch all required remote modules into the local cache. Skips local modules.
Updates `scampi.sum` for newly downloaded modules.

## update

```text
scampi mod update <module>
```

Update a module to the latest stable version. Re-fetches and updates
`scampi.mod` and `scampi.sum`.

## verify

```text
scampi mod verify
```

Check that cached remote modules match their recorded checksums in `scampi.sum`.

## cache

```text
scampi mod cache
```

Print the module cache directory path.

## clean

```text
scampi mod clean
```

Remove all cached modules.
