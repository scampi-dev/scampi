---
title: inspect
weight: 4
---

```text
scampi inspect [flags] <config> [path]
```

Reads a declarative configuration file and shows the resolved state of all
steps after Starlark evaluation.

## Modes

**List mode** (default) shows the resolved configuration for every step:

```bash
scampi inspect config.star
```

**Diff mode** compares file content against the current target state:

```bash
scampi inspect config.star --diff              # list diffable paths
scampi inspect config.star --diff nginx.conf   # diff a specific file
scampi inspect config.star --diff -i           # pick interactively
```

## Flags

| Flag                | Description                                                  |
| ------------------- | ------------------------------------------------------------ |
| `--only`            | Filter to specific deploy blocks (comma-separated)           |
| `--targets`         | Filter to specific targets (comma-separated)                 |
| `--diff`            | Diff file content (add a path argument to select which file) |
| `-i, --interactive` | Pick a file interactively using `$SCAMPI_FUZZY_FINDER`       |

## Environment variables

| Variable              | Description                                                              |
| --------------------- | ------------------------------------------------------------------------ |
| `SCAMPI_DIFFTOOL`     | Diff tool for `--diff` (checked first)                                   |
| `DIFFTOOL`            | Diff tool fallback                                                       |
| `EDITOR`              | Diff tool fallback                                                       |
| `SCAMPI_FUZZY_FINDER` | Fuzzy finder for `-i` (e.g. `fzf`, `sk`). Required for interactive mode. |

If no diff tool is set, scampi falls back to plain
[`diff(1)`](https://man7.org/linux/man-pages/man1/diff.1.html).
