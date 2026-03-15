---
title: pkg
---

Ensure packages are present, absent, or at the latest version on the target.

Scampi detects the target's package manager automatically. Currently supported
backends: apt (Debian/Ubuntu), pacman (Arch Linux).

## Fields

| Field      | Type   | Required | Default     | Description                                     |
|------------|--------|:--------:|-------------|-------------------------------------------------|
| `packages` | list   |    ✓     |             | Package names to manage                         |
| `desc`     | string |          |             | Human-readable description                      |
| `state`    | string |          | `"present"` | Desired state: `present`, `absent`, or `latest` |

## States

| State     | Behavior                                                                        |
|-----------|---------------------------------------------------------------------------------|
| `present` | Install if not already installed. Don't touch if already present (any version). |
| `absent`  | Remove if installed. No-op if already absent.                                   |
| `latest`  | Install or upgrade to the latest version available in the package index.        |

## How it works

The `pkg` step queries the target's package manager to check installed packages.
For `present`, it checks if each package is installed. For `latest`, it checks
if the installed version matches the latest available. For `absent`, it checks if
the package is installed at all.

Only missing or outdated packages trigger an install/upgrade. Already-satisfied
packages are skipped.

## Examples

### Install packages

```python
pkg(packages=["nginx", "curl", "jq"])
```

### Ensure latest

```python
pkg(
    desc = "keep security tools up to date",
    packages = ["openssl", "ca-certificates"],
    state = "latest",
)
```

### Remove packages

```python
pkg(packages=["apache2"], state="absent")
```
