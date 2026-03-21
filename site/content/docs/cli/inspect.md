---
title: inspect
weight: 4
---

```text
scampi inspect [flags] <config>
```

Reads a declarative configuration file, extracts file content from inspectable
ops (e.g. copy, template), and opens a diff tool to compare the desired state
against what currently exists on the target.

## Flags

| Flag        | Description                                                        |
| ----------- | ------------------------------------------------------------------ |
| `--only`    | Filter to specific deploy blocks (comma-separated)                 |
| `--targets` | Filter to specific targets (comma-separated)                       |
| `--step`    | Filter to a specific file op by destination path (substring match) |

## Diff tool selection

Scampi checks the following environment variables in order:

1. `SCAMPI_DIFFTOOL`
2. `DIFFTOOL`
3. `EDITOR`

If none are set, it falls back to plain [`diff(1)`](https://man7.org/linux/man-pages/man1/diff.1.html).
