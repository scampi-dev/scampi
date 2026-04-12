---
title: pkg
---

Ensure packages are present, absent, or at the latest version on the target.

scampi detects the target's package manager automatically. Currently supported
backends: apt (Debian/Ubuntu), dnf (Fedora/RHEL), apk (Alpine), pacman (Arch),
zypper (openSUSE), pkg (FreeBSD), brew (macOS).

## Fields

| Field       | Type          | Required | Default            | Description                                    |
| ----------- | ------------- | :------: | ------------------ | ---------------------------------------------- |
| `packages`  | list\[string] |    ✓     |                    | Package names to manage (`@std.nonempty`)      |
| `source`    | `PkgSource`   |    ✓     |                    | Package source — see [below](#package-sources) |
| `state`     | `PkgState`    |          | `PkgState.present` | Desired state                                  |
| `desc`      | string?       |          |                    | Human-readable description                     |
| `on_change` | list\[Step]   |          |                    | Steps to trigger when packages change          |

## States

`posix.PkgState` is an enum:

| Value              | Behavior                                                                        |
| ------------------ | ------------------------------------------------------------------------------- |
| `PkgState.present` | Install if not already installed. Don't touch if already present (any version). |
| `PkgState.absent`  | Remove if installed. No-op if already absent.                                   |
| `PkgState.latest`  | Install or upgrade to the latest version available in the package index.        |

## Package sources

The `source` field tells scampi where packages come from. Use
`posix.pkg_system {}` for the target's built-in package manager, or a typed
source decl for third-party repositories:

| Source               | Description                                  |
| -------------------- | -------------------------------------------- |
| `posix.pkg_system`   | System package manager (apt, dnf, apk, etc.) |
| `posix.pkg_apt_repo` | Third-party APT repository (Debian/Ubuntu)   |
| `posix.pkg_dnf_repo` | Third-party DNF repository (Fedora/RHEL)     |

For third-party repositories, the step handles GPG key installation, repo
configuration, and cache refresh automatically before installing packages.

### `posix.pkg_apt_repo`

Configure an APT repository (Debian/Ubuntu).

| Field        | Type           | Required | Description                                       |
| ------------ | -------------- | :------: | ------------------------------------------------- |
| `url`        | string         |    ✓     | Repository URL                                    |
| `key_url`    | string         |    ✓     | URL to the GPG signing key                        |
| `components` | list\[string]? |          | Repository components (defaults to `["main"]`)    |
| `suite`      | string?        |          | Distribution codename (auto-detected from target) |

### `posix.pkg_dnf_repo`

Configure a DNF repository (Fedora/RHEL).

| Field     | Type    | Required | Description                                              |
| --------- | ------- | :------: | -------------------------------------------------------- |
| `url`     | string  |    ✓     | Repository base URL                                      |
| `key_url` | string? |          | URL to the GPG signing key (optional for unsigned repos) |

### Op chain

When a third-party source is in use, the step generates a DAG of operations:

```
1. download GPG key → cache       (if key_url is set)
2. install signing key on target  (if key_url is set)
3. write repo config on target    (+ cache refresh)
4. install packages
```

Each op has independent Check/Execute — already-satisfied ops are skipped.

## How it works

The `pkg` step queries the target's package manager to check installed packages.
For `present`, it checks if each package is installed. For `latest`, it checks
if the installed version matches the latest available. For `absent`, it checks
if the package is installed at all.

Only missing or outdated packages trigger an install/upgrade. Already-satisfied
packages are skipped.

## Examples

### Install packages

```scampi
posix.pkg {
  packages = ["nginx", "curl", "jq"]
  source   = posix.pkg_system {}
}
```

### Ensure latest

```scampi
posix.pkg {
  desc     = "keep security tools up to date"
  packages = ["openssl", "ca-certificates"]
  state    = posix.PkgState.latest
  source   = posix.pkg_system {}
}
```

### Remove packages

```scampi
posix.pkg {
  packages = ["apache2"]
  state    = posix.PkgState.absent
  source   = posix.pkg_system {}
}
```

### Install from APT repository

```scampi
posix.pkg {
  desc     = "install Docker Engine"
  packages = ["docker-ce", "docker-ce-cli", "containerd.io"]
  source   = posix.pkg_apt_repo {
    url        = "https://download.docker.com/linux/debian"
    key_url    = "https://download.docker.com/linux/debian/gpg"
    components = ["stable"]
  }
}
```

### Install from DNF repository

```scampi
posix.pkg {
  desc     = "install from custom repo"
  packages = ["my-package"]
  source   = posix.pkg_dnf_repo {
    url     = "https://repo.example.com/el9"
    key_url = "https://repo.example.com/RPM-GPG-KEY"
  }
}
```

### OS-neutral configuration with `let` bindings

```scampi
// Pick the right source based on the target. Both options below
// produce a `PkgSource` value, so a single `pkg` call can use either.

let docker_apt = posix.pkg_apt_repo {
  url        = "https://download.docker.com/linux/debian"
  key_url    = "https://download.docker.com/linux/debian/gpg"
  components = ["stable"]
}

let docker_dnf = posix.pkg_dnf_repo {
  url     = "https://download.docker.com/linux/rhel"
  key_url = "https://download.docker.com/linux/rhel/gpg"
}

// Use one of them in a deploy block:
posix.pkg {
  packages = ["docker-ce"]
  source   = docker_apt
}
```
