---
title: Language
weight: 3
---

scampi has its own configuration language. This page is a guided tour: enough
to read every example in the rest of the docs and write your first config from
scratch.

If you've used Python, HCL, or Bicep before, scampi will feel familiar — but
it's its own thing, with a real type system, declarative struct literals, and
no significant whitespace.

## A complete file at a glance

```scampi {filename="webserver.scampi"}
module main

import "std"
import "std/local"
import "std/posix"

let machine = local.target { name = "my-machine" }

let reload_nginx = posix.service {
  name = "nginx"
  state = posix.ServiceState.reloaded
}

std.deploy(name = "webserver", targets = [machine]) {
  posix.pkg {
    packages = ["nginx", "certbot"]
    source = posix.pkg_system {}
  }

  posix.copy {
    src = posix.source_local { path = "./nginx.conf" }
    dest = "/etc/nginx/nginx.conf"
    perm = "0644"
    owner = "root"
    group = "root"
    on_change = [reload_nginx]
  }

  posix.service { name = "nginx", state = posix.ServiceState.running, enabled = true }
  posix.firewall { port = "80/tcp" }
  posix.firewall { port = "443/tcp" }
}
```

That's a real, working scampi config. Every concept on this page shows up in
those 30 lines.

## Modules and imports

Every scampi file starts with a `module` declaration and zero or more `import`
statements:

```scampi
module main

import "std"
import "std/posix"
import "std/ssh"
```

`module main` declares the entry point. Library modules use their own name
(`module mymodule`) and live in importable paths.

`import "std/posix"` brings the `posix` namespace into scope. After importing
you reference its contents with `posix.copy`, `posix.ServiceState`, etc. The
last path segment is the binding name.

The standard library is split into focused namespaces — see [std vs posix vs
…](#whats-in-std-vs-posix) below.

## Let bindings

Use `let` to bind a value to a name:

```scampi
let version = "1.2.3"
let url = "https://example.com/v${version}/app.tar.gz"
let admins = ["alice", "bob"]
```

Strings support `${}` interpolation. Bindings are immutable — you can't
reassign a `let`, but you can bind a new name.

`let` works for steps too. This is how you reference the same step from
multiple places (the classic "reload nginx when config changes" pattern):

```scampi
let reload_nginx = posix.service {
  name = "nginx"
  state = posix.ServiceState.reloaded
}

posix.copy {
  src = posix.source_local { path = "./nginx.conf" }
  dest = "/etc/nginx/nginx.conf"
  perm = "0644", owner = "root", group = "root"
  on_change = [reload_nginx]
}
```

## Two ways to call things

scampi has two distinct call syntaxes. The choice tells you what kind of thing
you're calling.

### Decl calls — struct literal syntax

A `decl` produces a value of some declarative type (a step, a target, a source
resolver). You call it with **braces and field assignments**, like a struct
literal:

```scampi
posix.dir { path = "/srv/app", perm = "0755" }

posix.copy {
  src = posix.source_local { path = "./config.yaml" }
  dest = "/etc/app/config.yaml"
  perm = "0644"
  owner = "app"
  group = "app"
}
```

Field separators are flexible: line breaks, commas, or both. Order doesn't
matter — fields are named, not positional.

### Function calls — parentheses

A `func` returns a computed value. You call it with **parentheses**, the way
you'd expect from any other language:

```scampi
let pkgs = base_packages(extra = ["nginx"])
let key  = std.secret("vps.host")
let nums = std.range(10)
```

### Trailing-block functions

Some functions return a `block[T]` — a structured block whose body you write
at the call site, after the `)`:

```scampi
std.deploy(name = "webserver", targets = [machine]) {
  posix.pkg { packages = ["nginx"], source = posix.pkg_system {} }
  posix.service { name = "nginx", state = posix.ServiceState.running }
}
```

The `{ … }` after `std.deploy(...)` is the deploy body. Inside it you write
expressions whose results — typically steps — get attached to the deploy.

This is a first-class language feature. Any function with a `block[T]` return
type uses this trailing-block syntax. You can write your own.

## Types and the type system

scampi is statically typed. You can declare your own types:

```scampi
type User {
  name:   string
  groups: list[string] = []
  shell:  string       = "/bin/bash"
  admin:  bool         = false
}
```

Each field has a type. Defaults are written with `=`. Optional fields use a
`?` suffix on the type (`string?`).

You instantiate a typed value with — surprise — struct literal syntax:

```scampi
let alice = User { name = "alice", groups = ["sudo"], admin = true }
let bob   = User { name = "bob", shell = "/bin/zsh" }
let chuck = User { name = "charlie" }
```

Generic types use bracket syntax: `list[string]`, `map[string, int]`.

Enums are closed sets of named values:

```scampi
enum PkgState { present, absent, latest }

let s = PkgState.present
```

## Attributes

Attributes annotate types and parameters with extra meaning — validation
rules, deprecation notices, documentation hints. Built-in attributes from
`std` cover the common cases:

| Attribute       | Purpose                                                  |
| --------------- | -------------------------------------------------------- |
| `@std.nonempty` | String parameter must not be empty                       |
| `@std.path`     | Validate as a filesystem path (`absolute=true` optional) |
| `@std.filemode` | Validate as octal/ls/posix file permissions              |
| `@std.pattern`  | Match a regex (`regex="..."`)                            |
| `@std.oneof`    | Must be one of a fixed set of strings                    |
| `@std.secretkey`| String is a secret-store key (LSP completion enabled)    |
| `@std.deprecated` | Emit a warning at every use                            |
| `@std.since`    | Records the version a parameter was introduced          |

You'll see them sprinkled across the standard library:

```scampi
decl copy(
  src: Source,
  @std.path(absolute=true)
  dest: string,
  @std.filemode
  perm: string,
  @std.nonempty
  owner: string,
  // ...
) std.Step
```

Validation runs at link time, so a config with bad values fails fast with a
typed diagnostic — long before anything touches a target.

## Declarations: `let`, `func`, `decl`, `type`, `enum`, `attribute`

scampi has a small set of top-level declaration kinds:

| Kind         | Purpose                                                            |
| ------------ | ------------------------------------------------------------------ |
| `let`        | Bind a value to a name                                             |
| `func`       | Define a function (called with parens)                             |
| `decl`       | Define a declarative constructor (called with struct-literal braces) |
| `type`       | Define a struct type                                               |
| `enum`       | Define a closed set of named values                                |
| `attribute`  | Define an attribute usable on parameters                           |

You can write your own `func` and `decl` — they're first-class. A user-defined
`decl` is a great way to compose a reusable step pattern:

```scampi
decl create_user(name: string, shell: string = "/bin/bash") std.Step {
  posix.user { name = self.name, shell = self.shell }
}

// later:
create_user { name = "alice", shell = "/bin/zsh" }
```

Inside the body, `self` refers to the call-site struct literal.

## Expressions

scampi has the usual expression palette:

```scampi
// Arithmetic and comparison
let x = (a + b) * 2
let ok = x > 0 && y < 10

// String interpolation
let path = "/srv/${app}/v${version}"

// List and map literals
let pkgs = ["nginx", "certbot"]
let env  = {"NODE_ENV": "production", "PORT": "3000"}

// Comprehensions
let admin_names = [u.name for u in users if u.admin]
let by_name     = {u.name: u for u in users}

// If-expressions
let extra = if needs_sudo { ["sudo"] } else { [] }
```

## Statements inside blocks

Trailing-block bodies (like a `std.deploy(...) { … }`) are blocks of
statements. Most statements are just expressions whose results get attached to
the block, but you can also use control flow:

```scampi
std.deploy(name = "users", targets = [vps]) {
  for u in users {
    create_user { name = u.name, shell = u.shell }
  }

  if needs_sudo {
    posix.pkg { packages = ["sudo"], source = posix.pkg_system {} }
  }
}
```

## UFCS — uniform function call syntax

Any function call `f(x, y)` can also be written `x.f(y)`. The two are
identical:

```scampi
func double(n: int) int { return n + n }

let a = double(5)         // bare call
let b = (5).double()      // UFCS — same function
let c = std.range(5)      // module-qualified — also a function call
```

This makes method-chain-style code possible without the language having actual
methods. It's also why `std.range` and `std.secret` look like methods on `std`
— they're functions, and the dot is just UFCS reaching across the import.

## What's in `std` vs `posix` vs …

The standard library is split into focused namespaces. You import only what
you need.

| Import              | What's inside                                                                                      |
| ------------------- | -------------------------------------------------------------------------------------------------- |
| `std`               | Core types (`Step`, `Target`), validation attributes, `deploy`, `secret`, `env`, `range`, secrets config |
| `std/local`         | `local.target` — execute steps on the local machine                                                |
| `std/ssh`           | `ssh.target` — execute steps on a remote host over SSH                                             |
| `std/posix`         | All POSIX steps (copy, dir, template, pkg, service, user, group, mount, firewall, sysctl, run, …) and source/pkg composables |
| `std/rest`          | REST target, request/resource steps, auth and TLS composables                                      |
| `std/container`    | Container management (Docker, Podman)                                                              |
| `std/test`          | Test framework: mock targets, assertions, matchers                                                 |

A typical "real" config imports `std` plus one target module plus the step
modules it uses. The webserver example at the top of this page imports `std`,
`std/local`, and `std/posix` — that's enough for everything POSIX targets do.

## Where to next

- [Getting Started]({{< relref "../getting-started" >}}) — write and run your first config
- [Concepts]({{< relref "../concepts" >}}) — the execution model: steps, actions, ops, convergence
- [Step Reference]({{< relref "../steps" >}}) — every built-in step with examples
- [Target Reference]({{< relref "../targets" >}}) — local, SSH, REST
