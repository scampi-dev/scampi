---
title: Matchers
weight: 2
---

Matchers declare expected end-state on a test target. They live inside the
`expect` field of `test.target_in_memory(...)` and in the `body` field of
`test.request(...)`. All matchers return the opaque `matchers.Matcher` type.

```scampi
import "std/test/matchers"

expect = test.ExpectedState {
  files    = {"/etc/app.conf": matchers.has_substring("listen 8080")}
  services = {"nginx": matchers.has_svc_status(posix.ServiceState.running)}
  dirs     = {"/var/www/mysite": matchers.is_present()}
  packages = {"nginx": matchers.has_pkg_status(posix.PkgState.present)}
}
```

## Content matchers

Match against string content (file bodies, request bodies, header values).

| Matcher                           | Checks                                 |
| --------------------------------- | -------------------------------------- |
| `matchers.has_exact_content(str)` | Byte-for-byte content match            |
| `matchers.has_substring(str)`     | Content contains substring             |
| `matchers.matches_regex(pattern)` | Content matches Go regular expression  |
| `matchers.is_empty()`             | Content exists but is the empty string |

## Existence matchers

Work in any keyed slot — files, packages, services, dirs, symlinks.

| Matcher                 | Checks                           |
| ----------------------- | -------------------------------- |
| `matchers.is_present()` | Slot must exist (any content)    |
| `matchers.is_absent()`  | Slot must NOT exist after deploy |

## Status matchers

Parameterized by existing posix enums so the test vocabulary matches the
production vocabulary one-for-one.

| Matcher                           | Checks                                       |
| --------------------------------- | -------------------------------------------- |
| `matchers.has_svc_status(status)` | Service matches a `posix.ServiceState` value |
| `matchers.has_pkg_status(status)` | Package matches a `posix.PkgState` value     |

### Examples

```scampi
// Service should be running
matchers.has_svc_status(posix.ServiceState.running)

// Package should be installed
matchers.has_pkg_status(posix.PkgState.present)

// Package should be removed
matchers.has_pkg_status(posix.PkgState.absent)
```
