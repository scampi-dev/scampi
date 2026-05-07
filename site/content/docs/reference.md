---
title: Quick Reference
weight: 10
---

A single-page cheatsheet for writing scampi configs. For full explanations, see
the [Language]({{< relref "language" >}}) and [Step Reference]({{< relref "steps" >}}) pages.

## File structure

```scampi
module main

import "std"
import "std/posix"
import "std/ssh"
import "std/secrets"
```

Every file starts with `module` + imports. Comments are `//` line and `/* … */` block.

## Standard library namespaces

| Import            | Namespace   | Contents                                          |
| ----------------- | ----------- | ------------------------------------------------- |
| `"std"`           | `std`       | Core: `deploy`, `env`, `range`, `ref`, attributes |
| `"std/local"`     | `local`     | `local.target` — execute on local machine         |
| `"std/ssh"`       | `ssh`       | `ssh.target` — execute over SSH                   |
| `"std/posix"`     | `posix`     | All POSIX steps + source/pkg composables          |
| `"std/rest"`      | `rest`      | REST target, request/resource steps, auth, TLS    |
| `"std/container"` | `container` | Container lifecycle (Docker, Podman)              |
| `"std/secrets"`   | `secrets`   | Secret resolvers (`from_age`, `from_file`)        |
| `"std/test"`      | `test`      | Test framework: mock targets, matchers            |

## Bindings and types

```scampi
let version = "1.2.3"                         // immutable
let url = "https://example.com/v${version}"   // interpolation
let pkgs = ["nginx", "curl"]                  // list
let env = {"PORT": "8080"}                    // map

type User {
  name:   string
  groups: list[string] = []       // default
  shell:  string       = "/bin/bash"
  admin:  bool         = false
  bio:    string?                 // optional (accepts none)
}

enum PkgState { present, absent, latest }

let alice = User { name = "alice", admin = true }
```

## Two call syntaxes

```scampi
// Decl call — braces + field assignments (steps, targets, sources)
posix.dir { path = "/srv/app", perm = "0755" }

// Function call — parentheses (computed values)
let key = age.get("vps.host")

// Trailing-block function — block[T] return
std.deploy(name = "web", targets = [vps]) {
  // steps here
}
```

## User-defined decl and func

```scampi
decl create_user(name: string, shell: string = "/bin/bash") std.Step {
  posix.user { name = self.name, shell = self.shell }
}

func base_packages(extra: list[string] = []) list[string] {
  let base = ["curl", "htop", "vim"]
  return base + extra
}
```

## Control flow

```scampi
for u in users { create_user { name = u.name } }
for k, v in env_map { posix.sysctl { key = k, value = v } }

if u.admin { posix.copy { ... } }

let shell = if u.admin { "/bin/zsh" } else { "/bin/bash" }

let names = [u.name for u in users if u.admin]
let by_name = {u.name: u for u in users}
```

## UFCS

Any `f(x, y)` can be written `x.f(y)`:
```scampi
let token = age.get("key")  // same as secrets.get(age, "key")
```

## Deploy blocks

```scampi
std.deploy(name = "webserver", targets = [web]) {
  // bare invocation = desired state
  posix.pkg { packages = ["nginx"], source = posix.pkg_system {} }

  // let-bound = value for on_change (not desired state itself)
  let reload = posix.service { name = "nginx", state = posix.ServiceState.reloaded }

  posix.copy {
    src = posix.source_local { path = "./nginx.conf" }
    dest = "/etc/nginx/nginx.conf"
    perm = "0644", owner = "root", group = "root"
    on_change = [reload]
  }
}
```

## Targets

```scampi
// Local
let machine = local.target { name = "my-machine" }

// SSH
let vps = ssh.target {
  name = "vps", host = "192.168.1.10", user = "deploy"
  port = 22, key = "~/.ssh/id_ed25519", timeout = "5s"
}

// REST
let api = rest.target {
  name = "api"
  base_url = "https://api.example.com"
  auth = rest.bearer { token_endpoint = "/oauth/token", identity = "...", secret = "..." }
  tls = rest.tls_secure {}
}
```

**Auth:** `rest.no_auth {}`, `rest.basic { user, password }`,
`rest.bearer { token_endpoint, identity, secret }`, `rest.header { name, value }`

**TLS:** `rest.tls_secure {}`, `rest.tls_insecure {}`, `rest.tls_ca_cert { path }`

## Secrets

```scampi
let age = secrets.from_age(path = "secrets.age.json")
let host = age.get("vps.host")
```

Identity: `$SCAMPI_AGE_KEY`, `$SCAMPI_AGE_KEY_FILE`, or `~/.config/scampi/age.key`.

## Source resolvers

```scampi
src = posix.source_local { path = "./files/config.yaml" }
src = posix.source_inline { content = "key = value\n" }
src = posix.source_remote { url = "https://...", checksum = "sha256:abc..." }
```

## Built-in functions

| Function                      | Description                         |
| ----------------------------- | ----------------------------------- |
| `std.env(name)`               | Read env var (error if unset)       |
| `std.env(name, default = "")` | Read env var with fallback          |
| `std.range(n)`                | `[0, 1, ..., n-1]`                  |
| `std.parse_int(s)`            | Parse base-10 int from string       |
| `std.unique(items)`           | Order-preserving dedupe of a list   |
| `std.join(items, sep = " ")`  | Join list of strings with separator |
| `std.trim_prefix(s, prefix)`  | Strip leading prefix if present     |
| `std.trim_suffix(s, suffix)`  | Strip trailing suffix if present    |
| `std.ref(step, expr)`         | Cross-step jq reference             |
| `std.deploy(name, targets)`   | Declare a deploy block              |
| `secrets.from_age(path)`      | Age-encrypted JSON resolver         |
| `secrets.from_file(path)`     | Plain JSON resolver                 |
| `resolver.get(key)`           | Look up secret key (UFCS)           |
| `len(coll)`                   | Collection length                   |

## Step quick-reference

| Step                 | Purpose                   | Key fields                                         |
| -------------------- | ------------------------- | -------------------------------------------------- |
| `posix.copy`         | Deploy a file             | `src`, `dest`, `perm`, `owner`, `group`, `verify`  |
| `posix.template`     | Render Go template        | `src`, `dest`, `data`, `perm`, `owner`, `group`    |
| `posix.dir`          | Ensure directory          | `path`, `perm`, `owner`, `group`                   |
| `posix.symlink`      | Manage symlink            | `target`, `link`                                   |
| `posix.pkg`          | Package management        | `packages`, `source`, `state`                      |
| `posix.service`      | Systemd service           | `name`, `state`, `enabled`                         |
| `posix.user`         | User account              | `name`, `state`, `shell`, `home`, `groups`         |
| `posix.group`        | System group              | `name`, `state`, `gid`, `system`                   |
| `posix.firewall`     | Firewall rule             | `port`, `end_port`, `proto`, `action`              |
| `posix.mount`        | Filesystem mount          | `src`, `dest`, `fs_type`, `opts`, `state`          |
| `posix.sysctl`       | Kernel parameter          | `key`, `value`, `persist`                          |
| `posix.run`          | Arbitrary command         | `apply`, `check` xor `always`                      |
| `posix.unarchive`    | Extract archive           | `src`, `dest`, `depth`, `owner`, `group`, `perm`   |
| `container.instance` | Container lifecycle       | `name`, `image`, `state`, `ports`, `env`, `mounts` |
| `rest.request`       | HTTP request              | `method`, `path`, `headers`, `body`, `check`       |
| `rest.resource`      | Declarative REST resource | `query`, `missing`, `found`, `bindings`, `state`   |

## State enums

| Enum                   | Values                                                                      |
| ---------------------- | --------------------------------------------------------------------------- |
| `posix.PkgState`       | `present`, `absent`, `latest`                                               |
| `posix.ServiceState`   | `running`, `stopped`, `restarted`, `reloaded`                               |
| `posix.UserState`      | `present`, `absent`                                                         |
| `posix.GroupState`     | `present`, `absent`                                                         |
| `posix.MountState`     | `mounted`, `unmounted`, `absent`                                            |
| `posix.MountType`      | `nfs`, `nfs4`, `cifs`, `ext4`, `xfs`, `btrfs`, `tmpfs`, `glusterfs`, `ceph` |
| `posix.FirewallAction` | `allow`, `deny`, `reject`                                                   |
| `posix.FirewallProto`  | `tcp`, `udp`                                                                |
| `container.State`      | `running`, `stopped`, `absent`                                              |
| `container.Restart`    | `unless_stopped`, `always`, `on_failure`, `no`                              |

## Attributes

| Attribute               | Purpose                           |
| ----------------------- | --------------------------------- |
| `@std.nonempty`         | String must not be empty          |
| `@std.filemode`         | Validate as file permissions      |
| `@std.path`             | Validate as filesystem path       |
| `@std.pattern`          | Match a regex                     |
| `@std.oneof`            | Must be one of a fixed set        |
| `@std.min` / `@std.max` | Integer bounds                    |
| `@secrets.secretkey`    | Secret-store key (LSP completion) |
| `@std.deprecated`       | Emit warning at every use         |
