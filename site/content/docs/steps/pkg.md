---
title: pkg
---

Ensure packages are present, absent, or at the latest version on the target.

Scampi detects the target's package manager automatically. Currently supported
backends: apt (Debian/Ubuntu), dnf (Fedora/RHEL), apk (Alpine), pacman (Arch),
zypper (openSUSE), pkg (FreeBSD), brew (macOS).

## Fields

| Field      | Type         | Required | Default     | Description                                     |
|------------|--------------|:--------:|-------------|-------------------------------------------------|
| `packages` | list         |    ✓     |             | Package names to manage                         |
| `source`   | `pkg_source` |    ✓     |             | Package source (see [below](#package-sources))  |
| `desc`     | string       |          |             | Human-readable description                      |
| `state`    | string       |          | `"present"` | Desired state: `present`, `absent`, or `latest` |

## States

| State     | Behavior                                                                        |
|-----------|---------------------------------------------------------------------------------|
| `present` | Install if not already installed. Don't touch if already present (any version). |
| `absent`  | Remove if installed. No-op if already absent.                                   |
| `latest`  | Install or upgrade to the latest version available in the package index.        |

## Package Sources

The `source` field tells scampi where packages come from. Use `system()` for the
target's built-in package manager, or a typed source function for third-party
repositories:

| Source       | Description                                  |
|--------------|----------------------------------------------|
| `system()`   | System package manager (apt, dnf, apk, etc.) |
| `apt_repo()` | Third-party APT repository (Debian/Ubuntu)   |
| `dnf_repo()` | Third-party DNF repository (Fedora/RHEL)     |

For third-party repositories, the step handles GPG key installation, repo
configuration, and cache refresh automatically before installing packages.

### `apt_repo()`

Configure an APT repository (Debian/Ubuntu).

| Field        | Type   | Required | Description                                            |
|--------------|--------|:--------:|--------------------------------------------------------|
| `url`        | string |    ✓     | Repository URL                                         |
| `key_url`    | string |    ✓     | URL to the GPG signing key                             |
| `components` | list   |          | Repository components (defaults to `["main"]`)         |
| `suite`      | string |          | Distribution codename (auto-detected from target)      |

### `dnf_repo()`

Configure a DNF repository (Fedora/RHEL).

| Field     | Type   | Required | Description                                               |
|-----------|--------|:--------:|-----------------------------------------------------------|
| `url`     | string |    ✓     | Repository base URL                                       |
| `key_url` | string |          | URL to the GPG signing key (optional for unsigned repos)  |

### Op chain

When a source is present, the step generates a DAG of operations:

```
1. download GPG key → cache       (if key_url is set)
2. install signing key on target   (if key_url is set)
3. write repo config on target     (+ cache refresh)
4. install packages
```

Each op has independent Check/Execute — already-satisfied ops are skipped.

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
pkg(packages=["nginx", "curl", "jq"], source=system())
```

### Ensure latest

```python
pkg(
    desc = "keep security tools up to date",
    packages = ["openssl", "ca-certificates"],
    state = "latest",
    source = system(),
)
```

### Remove packages

```python
pkg(packages=["apache2"], state="absent", source=system())
```

### Install from APT repository

```python
pkg(
    desc = "install Docker Engine",
    packages = ["docker-ce", "docker-ce-cli", "containerd.io"],
    source = apt_repo(
        url = "https://download.docker.com/linux/debian",
        key_url = "https://download.docker.com/linux/debian/gpg",
        components = ["stable"],
    ),
)
```

### Install from DNF repository

```python
pkg(
    desc = "install from custom repo",
    packages = ["my-package"],
    source = dnf_repo(
        url = "https://repo.example.com/el9",
        key_url = "https://repo.example.com/RPM-GPG-KEY",
    ),
)
```

### OS-neutral configuration with variables

```python
# debian.star
docker_src = apt_repo(
    url = "https://download.docker.com/linux/debian",
    key_url = "https://download.docker.com/linux/debian/gpg",
    components = ["stable"],
)

# rhel.star
docker_src = dnf_repo(
    url = "https://download.docker.com/linux/rhel",
    key_url = "https://download.docker.com/linux/rhel/gpg",
)

# shared.star (loaded after OS-specific file)
pkg(packages=["docker-ce"], source=docker_src)
```
