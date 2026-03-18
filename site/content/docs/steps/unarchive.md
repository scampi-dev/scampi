---
title: unarchive
---

Extract an archive to a target directory with optional recursive unpacking,
ownership, and permissions.

## Fields

| Field   | Type   | Required | Default | Description                                               |
|---------|--------|:--------:|:-------:|-----------------------------------------------------------|
| `src`   | source |    ✓     |         | Source archive: `local("./path.tar.gz")`                  |
| `dest`  | string |    ✓     |         | Target directory for extraction                           |
| `depth` | int    |          |   -1    | Nested archive recursion (-1=unlimited, 0=top-level only) |
| `owner` | string |          |         | Owner applied recursively after extraction                |
| `group` | string |          |         | Group applied recursively after extraction                |
| `perm`  | string |          |         | Permissions applied recursively after extraction          |
| `desc`  | string |          |         | Human-readable description                                |

If `owner` is set, `group` must also be set (and vice versa).

## Supported formats

Format is detected by extension (case-insensitive, longest match first):

| Extension             | Command                       |
|-----------------------|-------------------------------|
| `.tar.gz`, `.tgz`     | `tar xzf FILE -C DEST`        |
| `.tar.bz2`, `.tbz2`   | `tar xjf FILE -C DEST`        |
| `.tar.xz`, `.txz`     | `tar xJf FILE -C DEST`        |
| `.tar.zst`, `.tzst`   | `tar --zstd -xf FILE -C DEST` |
| `.tar`                | `tar xf FILE -C DEST`         |
| `.zip`                | `unzip -o FILE -d DEST`       |

Unsupported extensions produce a config error at plan time.

## How it works

The `unarchive` step produces a single op that handles the entire extraction
lifecycle:

1. **Check** — computes SHA256 of the source archive and compares it against a
   marker file on the target (`/var/lib/scampi/unarchive/<hash>.sha256`). If the
   marker matches, extraction is skipped entirely.
2. **Execute** — uploads the archive, extracts it, optionally unpacks nested
   archives, applies ownership/permissions, and writes the marker.

### Extraction backends

Two extraction backends are available, chosen per-execution:

- **Tool-based** (preferred for SSH targets): uploads archive to a temp path,
  runs `tar`/`unzip` on the target, removes the temp file. Probed with
  `command -v tar` / `command -v unzip`.
- **Go-native** (fallback): parses the archive in Go and writes files
  individually via the target filesystem. Available for all supported formats.

### Recursive unpacking

When `depth != 0`, after extracting the top-level archive, nested archives
found inside the destination are extracted in-place and removed. This repeats
until `depth` is exhausted or no more archives are found. `depth=-1` means
unlimited recursion.

### Idempotency

State is tracked via a checksum marker in `/var/lib/scampi/unarchive/`. The
marker file contains the SHA256 hex digest of the source archive. On subsequent
runs, if the marker matches the current archive hash, extraction is skipped
entirely. This avoids polluting user directories (git repos, web roots, etc.).

## Examples

### Basic extraction

```python {filename="deploy.star"}
unarchive(
    src = local("./files/site.tar.gz"),
    dest = "/var/www/mysite",
    depth = 0,
)
```

### With ownership and permissions

```python {filename="deploy.star"}
unarchive(
    src = local("./files/app.tar.gz"),
    dest = "/opt/myapp",
    owner = "myapp",
    group = "myapp",
    perm = "0755",
    desc = "deploy application bundle",
)
```

### Nested archive unpacking

```python {filename="deploy.star"}
unarchive(
    src = local("./release.tar.gz"),
    dest = "/opt/release",
    depth = -1,
    desc = "extract release with nested archives",
)
```

### Top-level only with hook

```python {filename="deploy.star"}
unarchive(
    src = local("./files/site.tar.gz"),
    dest = "/var/www/scampi.dev",
    depth = 0,
    owner = "www-data",
    group = "www-data",
    desc = "extract site content",
    on_change = ["restart-caddy"],
)
```
