# scampi-lang specification

Draft v0.1 — 2026-04-03

## Overview

scampi-lang is a statically-typed, declarative-first configuration language for
the scampi convergence engine. A scampi-lang program evaluates to a set of
desired-state declarations and one or more targets. The engine consumes these
declarations — no language code runs during execution.

The language has three conceptual layers:

1. **Declarations** — step blocks that describe desired state
2. **Types** — enums, structs, and step signatures
3. **Generation logic** — loops, conditionals, functions, and data transforms
   that exist solely to produce declarations

**The language core is minimal.** ~16 keywords: declarations (`type`,
`enum`, `step`, `func`), bindings (`let`), modules (`import`), control
flow (`for`, `in`, `if`, `else`, `return`), values (`true`, `false`,
`none`, `self`). Logic via operators (`&&`, `||`, `!`). No special
syntax for hooks, targets, deploys, or any other scampi concept —
everything domain-specific lives in `std`.

## Design principles

- Simple configs must be simple. A basic webserver setup is 15 lines, not 50.
- The language is a generation tool. It produces a flat set of step
  declarations. Loops and conditionals shape that set — they do not control
  execution order.
- Types exist for the LSP. Every function signature, every field, every enum
  value is statically known. Rename-across-workspace, exhaustive match
  checking, and full completion are first-class goals.
- No feature without a use case. If it's not needed to express real
  infrastructure configs, it doesn't go in.
- Composition is the design pattern. Small typed values (sources, auth
  configs, TLS configs, checks) compose into larger declarations. This
  is the API style — not inheritance, not mixins, not middleware chains.
- **The type system is king.** If solving a problem requires a special
  rule, sigil, or construct that exists only to handle one case, the
  design is wrong. Every user-facing concept — targets, deploys, hooks,
  references, composables — must fit the regular type rules. No
  escape hatches.

## File extension

`.scampi`

---

## 1. Lexical grammar

### 1.1 Encoding

Source files are UTF-8. No BOM.

### 1.2 Comments

```
# This is a line comment. There are no block comments.
```

### 1.3 Keywords

```
import  let     func    step    struct  enum
for     in      if      else    true    false
none    self    return
```

### 1.4 Identifiers

```
identifier = letter (letter | digit | '_')*
letter     = 'a'..'z' | 'A'..'Z' | '_'
digit      = '0'..'9'
```

Identifiers are case-sensitive. Convention is `snake_case` for everything
except enum variants which are `lowercase`.

### 1.5 Literals

**Integers:**
```
42
0xFF
0b1010
0o755
```

**Strings:**
```
"double quoted"
"with escapes: \n \t \\ \""
"with interpolation: ${expr}"
```

Interpolation uses `${expr}` inside double-quoted strings. The expression
is evaluated and converted to its string representation. For literal `${`,
escape the dollar: `\${`.

**Multi-line strings:**
```
"""
multi-line string
  preserves indentation relative to closing quotes
no escape processing except for ${}
"""
```

The common leading whitespace (determined by the closing `"""`) is stripped.

**Booleans:**
```
true
false
```

**None:**
```
none
```

Used only with optional types. `none` is not a general-purpose null.

### 1.6 Operators

```
+   -   *   /   %          # arithmetic
==  !=  <   >   <=  >=     # comparison
&&  ||  !                  # boolean
=                          # assignment / field binding
:                          # type annotation
.                          # member access
```

### 1.7 Delimiters

```
{  }        # blocks and maps
[  ]        # lists
(  )        # function calls and grouping
,           # separator (optional trailing comma allowed everywhere)
```

### 1.8 Whitespace and semicolons

Whitespace is not significant. Newlines act as statement terminators where
unambiguous. No semicolons. A statement continues to the next line if it
ends with an operator, open bracket, or comma.

---

## 2. Type system

### 2.1 Primitive types

| Type     | Description             | Literal examples       |
| -------- | ----------------------- | ---------------------- |
| `string` | UTF-8 text              | `"hello"`, `"""..."""` |
| `int`    | Arbitrary-precision int | `42`, `0o755`          |
| `bool`   | Boolean                 | `true`, `false`        |

### 2.2 Optional types

Any type suffixed with `?` accepts `none`:

```
shell: string? = none
```

Fields without `?` never accept `none`. This is enforced statically.

### 2.3 Collection types

```
list[string]                # homogeneous list
map[string, string]         # homogeneous map
map[string, any]            # heterogeneous values (for REST payloads etc.)
```

List and map literals:

```
let names = ["alice", "bob", "carol"]
let env = {"PATH": "/usr/bin", "HOME": "/root"}
```

### 2.4 Enum types

```
enum PkgState {
    present
    absent
    latest
}
```

Enum variants are accessed qualified: `PkgState.present`. When the expected
type is unambiguous, bare variants are allowed:

```
# both valid:
std.pkg { name = "nginx", state = PkgState.present }
std.pkg { name = "nginx", state = present }
```

The LSP resolves bare variants by checking the expected type of the field.
If ambiguous (two enums in scope with the same variant name), the compiler
requires qualification.

### 2.5 Struct types

```
struct User {
    name:   string
    groups: list[string] = []
    shell:  string = "/bin/bash"
}
```

Structs have named fields with types, optional defaults, and no methods.
Instantiation uses the same block syntax as steps:

```
let alice = User { name = "alice", groups = ["wheel", "dev"] }
```

Fields with defaults may be omitted. Fields without defaults are
required.

**Expected-type inference for struct literals**: when the expected
type of an expression is a struct type and the expression is a bare
block `{ field = value, ... }` (using `=`, distinct from map literals
which use `:`), the block is a struct literal for the expected type.
The type name can be omitted:

```
# with expected type TemplateData, this
data = { values = {...}, env = {...} }
# is equivalent to
data = TemplateData { values = {...}, env = {...} }
```

This keeps common patterns terse without losing type safety. The `=`
vs `:` distinction tells the parser (and the reader) whether they're
looking at a struct literal or a map literal:

```
struct_lit = { name = "alice", age = 30 }      # TeamMember struct
map_lit    = { "name": "alice", "age": 30 }    # map[string, any]
```

### 2.6 Type aliases

Not in v1. If needed later, syntax would be `struct Name = OtherType`
or similar.

---

## 3. Module system

### 3.1 Imports

```
import "std"
import "std/container"
import "std/rest"
import "codeberg.org/scampi-dev/modules/unifi"
import "codeberg.org/scampi-dev/modules/unifi/network"
```

Imports use string-literal paths, Go-style. Each import brings the
module in under its **leaf name** as a namespace:

```
import "std"                   # available as `std`
import "std/container"         # available as `container`
import "std/rest"              # available as `rest`
import "codeberg.org/scampi-dev/modules/unifi"  # available as `unifi`
```

Call sites use the leaf namespace:

```
std.pkg { ... }
std.copy { ... }
container.instance { ... }
rest.request { ... }
unifi.Network(...)
```

Import declarations appear at the top of the file, before any other
declarations or statements.

**Module system**: scampi-lang uses scampi's existing module system
(see `mod/` package and `scampi.mod` manifests). The system is
Go-inspired:

- Projects have a `scampi.mod` manifest declaring the module path and
  dependencies
- Every import path is a canonical module path — no aliasing. If you
  use `codeberg.org/scampi-dev/unifi`, you import it by that path
  everywhere in your code
- All modules (local and remote) resolve through a unified module
  cache. Local deps use a replace-style mechanism to point at a
  directory, but the import path is still canonical
- The cache location is configurable (defaults to user cache dir; can
  be pointed at `./vendor/` for airgapped deploys)

There is no selective import (`import foo.{a, b, c}`), no wildcard
import (`import foo.*`), and no aliasing in the `import` statement
itself. One path, one namespace, consistent everywhere. If two
imports would collide on leaf name, the compiler emits an error and
the user restructures their imports (or we revisit aliasing).

### 3.2 Standard library layout

```
std                            # core steps: copy, dir, symlink, template,
                               # unarchive, pkg, service, user, group, run,
                               # sysctl, mount, firewall
std/container                  # container.instance, container.healthcheck
std/rest                       # rest.request, rest.resource, rest.body.*,
                               # rest.status, rest.jq, rest.bearer, etc.
```

A typical config imports `"std"` for the core steps and adds `"std/rest"`
or `"std/container"` when needed.

All standard library steps are defined as scampi-lang stubs (generated
from Go struct tags). See §7.

There is no auto-import. Every file that uses builtins must
`import "std"` explicitly. Call sites are always namespaced:
`std.pkg {}`, `std.copy {}`, `std.dir {}`.

### 3.3 Exports

All top-level declarations in a file are exported. There is no
public/private distinction in v1. Files are the privacy boundary — don't
put it in the file if you don't want it used by another module.

If a visibility model becomes necessary later, it will be added
explicitly (e.g. a `pub` keyword, an underscore convention, or something
else). Deliberately unsolved for v1.

---

## 4. Declarations

### 4.1 Step instantiation

The core construct. A step block declares desired state:

```
std.pkg {
    name  = "nginx"
    state = present
}
```

Step names are resolved from imports. Builtins live in `std` and must
be imported and namespaced. User-defined steps declared in the current
file can be called unqualified; user-defined steps from another module
are called through that module's namespace (e.g. `myteam.create_user`).

Fields are key-value pairs. Values are expressions. All fields are
validated against the step's type signature at compile time.

### 4.2 Syntactic sugar (deferred to post-v1)

These shorthands are planned but not in the initial implementation.

**State shorthand** — for any step with a `state` field backed by an enum:

```
# sugar
std.pkg.present "nginx"

# desugars to
std.pkg { name = "nginx", state = PkgState.present }
```

**Bulk form:**

```
# sugar
std.pkg.present ["nginx", "certbot", "curl", "htop"]

# desugars to one std.pkg block per item
```

**Single-arg composable shorthand** — see §6.3.

**Scope-local unqualified imports** — inspired by D's `with` / `using`.
For blocks dominated by calls to a single module, a scoped `using` block
allows dropping the namespace prefix:

```
std.deploy {
    name    = "bootstrap"
    targets = [vps_root]

    using std {
        pkg { packages = ["sudo"], source = system {} }
        copy {
            src  = local { path = "./files/sudoers" }
            dest = "/etc/sudoers.d/hal9000"
            perm = "0440"
        }
        dir { path = "/var/log/app", perm = "0755" }
    }
}
```

Inside the `using std { ... }` block, `pkg`, `copy`, `dir`, `system`,
`local`, etc. are resolved against `std`. Outside the block, namespacing
is still required. The boundary is explicit and scope-local — no
file-wide pollution like Go's `import . "pkg"`. Deferred to post-v1.

### 4.3 Step and function declarations

`step` and `func` declarations share the same shape:

```
keyword NAME(field: type, ...) ReturnType { body }
```

Only the keyword differs. Both take typed parameters, both declare a
return/output type, both have a body.

**Functions:**

```
func build_url(host: string, path: string = "/") string {
    return "https://${host}${path}"
}
```

**User-defined step with body:**

```
step create_user(
    name:   string,
    groups: list[string] = [],
    shell:  string = "/bin/bash",
) StepInstance {
    std.pkg { packages = ["shadow-utils"], source = std.system {} }

    std.user {
        name   = self.name
        groups = self.groups
        shell  = self.shell
        state  = present
    }
}
```

`self` refers to the step's own field values inside the body. The body
produces a sequence of `StepInstance` values through bare step
invocations (see §4.7).

**Builtin stub (no body):**

Steps in `std` are declarations with no body — just the signature:

```
step pkg(
    packages: list[string],
    source:   PkgSource,
    state:    PkgState = PkgState.present,
    desc:     string?,
) StepInstance

step ssh(
    name:     string,
    host:     string,
    user:     string,
    port:     int = 22,
    key:      string?,
    insecure: bool = false,
    timeout:  string = "5s",
) Target

step deploy(
    name:    string,
    targets: list[Target],
) Deploy { /* body accepts step invocations */ }

step secrets(
    backend: string,
    path:    string,
) SecretsConfig
```

No body means the Go engine provides the implementation. The stub
gives the LSP everything it needs: field names, types, defaults,
documentation, and output type.

**Call-site syntax differs from declaration:**

Step declarations use parens with colons (matching func declarations).
Step **invocations** use braces with equals (matching struct literals):

```
// declaration: parens, colons
step pkg(packages: list[string], source: PkgSource) StepInstance

// invocation: braces, equals
std.pkg { packages = ["nginx"], source = std.system {} }
```

This reflects steps' dual nature — parameterized declarations (like
funcs) that produce typed records (constructed like struct literals).

**Output type rules (v0):**

- If omitted from a user-defined step, output type is `StepInstance`
- User-defined steps must produce `StepInstance` (no custom output
  types in v0)
- Builtin steps in std can produce any value type defined in std
  (`Target`, `Deploy`, `SecretsConfig`, or `StepInstance`)
- A step invocation's expression has the step's output type: e.g.
  `let v = std.target.ssh { ... }` gives `v` the type `Target`

### 4.4 Top-level scope and the engine

A scampi-lang project evaluates to a flat collection of typed values.
The engine consumes specific value types from the top-level scope:

| Value type      | Cardinality    | Meaning                             |
| --------------- | -------------- | ----------------------------------- |
| `SecretsConfig` | 0 or 1         | Configures the secrets backend      |
| `Target`        | 0 or more      | Execution environment registrations |
| `Deploy`        | 1 or more      | Deployment specifications           |
| `StepInstance`  | 0 at top-level | Only valid inside deploy bodies     |

A compile-time error is raised when a `StepInstance` expression
appears at top-level. The compiler traces this back to a typed
expression (e.g. `std.pkg { ... }`) and suggests wrapping it in a
`std.deploy` body.

An engine-level error is raised post-evaluation when the program
produces no `Deploy` values.

### 4.5 Targets (from `std/target`)

Targets are `let`-bound step invocations that produce `Target` values:

```
import "std/target"

let vps = target.ssh {
    name = "vps"
    host = std.secret("vps.host")
    user = "hal9000"
}

let dev = target.local { name = "dev" }

let api = target.rest {
    name     = "api"
    base_url = "https://api.example.com"
    auth     = rest.bearer {
        token_endpoint = "/oauth/token"
        identity       = std.secret("api.id")
        secret         = std.secret("api.secret")
    }
    tls = rest.tls.secure {}
}
```

Deploys reference targets by their `let` binding names — no
string-name registry. Type-checked end to end.

### 4.6 Deploy (from `std`)

A deploy is a step invocation that produces a `Deploy` value. Its body
accepts step invocations (as statements for desired state, or as
let-bound values for reuse) and arbitrary `let` bindings:

```
std.deploy {
    name    = "site"
    targets = [vps]

    let reload_caddy = std.service { name = "caddy", state = reloaded }

    std.pkg { packages = ["caddy"], source = std.system {} }
    std.dir { path = "/var/www/scampi.dev", perm = "0755" }

    std.copy {
        desc      = "install Caddyfile"
        src       = std.local { path = "./files/Caddyfile" }
        dest      = "/etc/caddy/Caddyfile"
        perm      = "0644"
        owner     = "root"
        group     = "root"
        verify    = "caddy validate --config %s"
        on_change = [reload_caddy]
    }

    std.service { name = "caddy", state = running, enabled = true }
}
```

The deploy body is just a block scope — it contains `let` bindings
and step invocations. Bare step invocations become desired state;
let-bound ones are values you can reference from `on_change` lists
(see §4.7).

### 4.7 Statements vs values in body scopes

Inside a body scope (a user-defined `step` body, a `std.deploy` body,
or any nested block), step invocations behave differently depending
on whether they appear as **statements** or **expressions**:

- **Statement (bare invocation)** — the step is emitted as desired
  state. The engine collects it as part of the enclosing deploy's
  convergence work.
- **Expression (let-bound or used in another value)** — the step
  invocation produces a `StepInstance` (or other output type) value
  you can reuse. It is NOT automatically emitted as desired state.

This positional semantics is what lets reactive hooks work without
any special language machinery:

```
std.deploy {
    name    = "bootstrap"
    targets = [vps_root]

    # let-bound — these are values, not desired state
    let restart_sshd = std.service { name = "sshd", state = restarted }
    let reload_caddy = std.service { name = "caddy", state = reloaded }

    # statement — this IS desired state
    std.copy {
        src       = std.local { path = "./files/sshd_hardened.conf" }
        dest      = "/etc/ssh/sshd_config.d/hardened.conf"
        verify    = "sshd -t -f %s"
        on_change = [restart_sshd]       # reactive reference (value reuse)
    }

    std.copy {
        src       = std.local { path = "./files/Caddyfile" }
        dest      = "/etc/caddy/Caddyfile"
        on_change = [reload_caddy]
    }
}
```

Types:

- `restart_sshd: StepInstance` (from `std.service`)
- `on_change: list[StepInstance]`

The same `StepInstance` value can be emitted as desired state AND
referenced from one or more `on_change` lists — the engine handles
the unification at runtime.

**Step output references (deferred to post-v0)**

Extracting a field from another step's runtime output — e.g. "use
the `id` field of the resource we just created" — is a separate
concern. For v0, use `std.rest.jq` and related composables (same as
current scampi), which bind runtime outputs via JQ expressions. A
dedicated cross-step output reference construct will be designed
when we have more examples of the pattern.

### 4.8 Secrets (from `std`)

```
std.secrets {
    backend = "age"
    path    = "secrets.age.json"
}

let host = std.secret("vps.host")
```

`std.secrets` is a step invocation producing a `SecretsConfig` value.
At most one may appear at top-level across the project. `std.secret(key)`
is a scalar function that returns a string resolved at evaluation time.

---

## 5. Generation logic

All constructs in this section exist to produce declarations. They run
during evaluation and are invisible to the engine.

### 5.1 Variables and mutability

```
let version = "12.7.2"
let url = "https://example.com/v${version}/app.tar.gz"
```

`let` bindings are immutable — you cannot reassign a name after binding.
Shadowing is allowed in inner scopes.

**Mutability rules depend on context:**

- **Inside `func` bodies**: `let` names are immutable, but collection
  *contents* are mutable. You can do `my_map["key"] = value` and
  `my_list.append(item)`. This is where imperative data-building logic
  lives.
- **Inside `step` blocks, `deploy` blocks, and top-level scope**:
  everything is recursively frozen. Once a value is bound, it and all its
  contents are immutable. You can call a function that builds a map, but
  once you have the result, no further mutation is possible.

This creates a clean boundary: functions are where you *compute*, step
blocks are where you *declare*. No side effects leak into declarations.

```
func build_state(name: string, extras: map[string, any]) map[string, any] {
    # inside func — mutation allowed on collection contents
    let state = {"name": name, "enabled": true}
    for k, v in extras {
        state[k] = v
    }
    return state
}

std.deploy {
    name    = "example"
    targets = [vps]
    let s = build_state("web", {"port": 8080})
    # s is frozen here — s["port"] = 9090 would be a compile error
    rest.request { state = s }
}
```

### 5.2 For loops

```
let users = [
    User { name = "alice", groups = ["wheel", "dev"] },
    User { name = "bob",   groups = ["dev"] },
]

for u in users {
    create_user {
        name   = u.name
        groups = u.groups
    }
}
```

`for` generates declarations — one set per iteration. The loop variable
is scoped to the block.

### 5.3 Conditionals

```
if "wheel" in u.groups {
    sudo.rule { user = u.name, commands = "ALL" }
}
```

```
let shell = if u.admin { "/bin/zsh" } else { "/bin/bash" }
```

`if` works both as a statement (generating declarations) and as an
expression (producing a value). The `else` branch is required in
expression form, optional in statement form.

### 5.4 Functions

Functions are for data transformation, string manipulation, and any logic
that builds up values for use in declarations:

```
func base_packages(extra: list[string] = []) list[string] {
    let base = ["curl", "htop", "vim", "tmux"]
    return base + extra
}

func build_dhcp_config(
    dhcp: map[string, string],
    dns: list[string]? = none,
    domain: string? = none,
) map[string, any] {
    let cfg = {
        "mode": "SERVER",
        "ipAddressRange": {"start": dhcp["start"], "end": dhcp["end"]},
        "leaseTimeSeconds": dhcp.get("lease_time", 86400),
    }
    if dns != none {
        cfg["dnsServerIpAddressesOverride"] = dns
    }
    if domain != none {
        cfg["domainName"] = domain
    }
    return cfg
}
```

Functions **cannot** contain step declarations. They take values and return
values. For reusable step bundles, use `step` definitions.

Collection contents are mutable inside function bodies (see §5.1). This is
where imperative data-building logic lives — conditionally inserting map
keys, appending to lists, computing derived values.

### 5.5 List comprehensions

```
let admins = [u.name for u in users if "wheel" in u.groups]
```

### 5.6 Map comprehensions

```
let env = {k: v for k, v in pairs if v != none}
```

### 5.7 Membership

```
"wheel" in u.groups          # true if list contains value
"key" in some_map            # true if map contains key
```

### 5.8 String interpolation

```
let msg = "installing Go ${go_version} to ${dest}"
```

Expressions inside `${}` are evaluated and stringified.

---

## 6. Builtin functions and composable values

### 6.1 Scalar functions (from `std`)

Function-call syntax (parens, positional args) is reserved for scalar
computations:

| Function                 | Type                           | Description                         |
| ------------------------ | ------------------------------ | ----------------------------------- |
| `std.env(name)`          | `(string) -> string`           | Read environment variable           |
| `std.env(name, default)` | `(string, string) -> string`   | With fallback                       |
| `std.secret(name)`       | `(string) -> string`           | Read secret from configured backend |
| `len(coll)`              | `(list[T] \| map[K,V]) -> int` | Collection length                   |
| `int(s)`                 | `(string) -> int`              | String → int parse                  |

### 6.2 Composable types

Composable types are small typed values that plug into step fields. They
use block syntax (like steps and structs), not function-call syntax.

**Source resolvers:**

```
src = std.local { path = "./files/config.yaml" }
src = std.inline { content = "hello world\n" }
src = std.remote { url = "https://example.com/file.tar.gz", checksum = "sha256:abc123" }
```

**Package sources:**

```
source = std.system {}
source = std.apt_repo {
    url        = "https://download.docker.com/linux/debian"
    key_url    = "https://download.docker.com/linux/debian/gpg"
    components = ["stable"]
}
source = std.dnf_repo { url = "https://example.com/repo/el9" }
```

**REST authentication:**

```
auth = rest.no_auth {}
auth = rest.basic { user = "admin", password = std.secret("api.pass") }
auth = rest.bearer {
    token_endpoint = "/oauth/token"
    identity       = std.secret("api.id")
    secret         = std.secret("api.secret")
}
auth = rest.header { name = "X-API-Key", value = std.secret("api.key") }
```

**REST TLS:**

```
tls = rest.tls.secure {}
tls = rest.tls.insecure {}
tls = rest.tls.ca_cert { path = "/etc/ssl/custom-ca.pem" }
```

**REST body:**

```
body = rest.body.json { data = {"name": "example", "count": 42} }
body = rest.body.string { content = "plain text payload" }
```

**REST checks:**

```
check = rest.status { code = 200 }
check = rest.jq { expr = ".data[] | select(.name == \"test\")" }
```

### 6.3 Future sugar for composables

Single-field composables are verbose in block form. A future sugar pass
may allow omitting the field name when there is exactly one argument:

```
# sugar (not in v1)
src = local "./files/config.yaml"
src = inline "hello world\n"
check = rest.jq ".data[]"
check = rest.status 200

# desugars to
src = std.local { path = "./files/config.yaml" }
src = std.inline { content = "hello world\n" }
check = rest.jq { expr = ".data[]" }
check = rest.status { code = 200 }
```

The compiler knows which field is the primary one from the type
definition. This sugar is deferred to post-v1 to keep the initial
implementation simple.

### 6.5 References

scampi-lang has no dedicated reference operator. Names are resolved
through normal scoping rules:

- Targets, variables, functions, types: use their bare name
- Reactive steps (hooks): let-bind them and use the binding name in
  `on_change` lists (see §4.7)
- Cross-step runtime output extraction: use `std.rest.jq` and similar
  composables (deferred post-v0)

---

## 7. Standard library stubs

These are generated from Go struct tags. They define every builtin step's
type signature. The LSP reads these as the authoritative source.

### 7.1 Enums

```
enum PkgState    { present, absent, latest }
enum SvcState    { running, stopped, restarted, reloaded }
enum UserState   { present, absent }
enum GroupState  { present, absent }
enum CtrState    { running, stopped, absent }
enum CtrRestart  { always, on_failure, unless_stopped, no }
enum MountState  { mounted, unmounted, absent }
enum FsType      { nfs, nfs4, cifs, ext4, xfs, btrfs, tmpfs, glusterfs, ceph }
enum FwAction    { allow, deny, reject }
enum HttpMethod  { GET, POST, PUT, PATCH, DELETE, HEAD, OPTIONS }
```

### 7.2 Top-level value types

Defined in `std`, consumed by the engine from top-level scope:

```
struct Target        { ... }   # opaque, produced by target.* steps
struct Deploy        { ... }   # opaque, produced by std.deploy
struct SecretsConfig { ... }   # opaque, produced by std.secrets
struct StepInstance  { ... }   # opaque, produced by desired-state steps
```

### 7.3 Target, deploy, secrets stubs

```
# std/target.scampi

step ssh(
    name:     string,
    host:     string,
    user:     string,
    port:     int = 22,
    key:      string?,
    insecure: bool = false,
    timeout:  string = "5s",
) Target

step local(name: string) Target

step rest(
    name:     string,
    base_url: string,
    auth:     Auth = rest.no_auth {},
    tls:      TLS  = rest.tls.secure {},
) Target
```

```
# std/deploy.scampi

step deploy(
    name:    string,
    targets: list[Target],
) Deploy {
    # body: let bindings + bare step invocations
    # bare step invocations produce desired state
}
```

```
# std/secrets.scampi

step secrets(
    backend: string,
    path:    string,
) SecretsConfig
```

### 7.4 Desired-state step stubs

All produce `StepInstance` (the default output type). Every desired-
state step implicitly has `on_change: list[StepInstance] = []` —
reactive steps to fire when this step changes. `desc: string?` is
shown on each stub.

```
# File operations
# ---------------------------------------------------------------------------

step copy(
    src:    Source,
    dest:   string,
    perm:   string,
    owner:  string,
    group:  string,
    verify: string?,
    desc:   string?,
) StepInstance

step dir(
    path:  string,
    perm:  string?,
    owner: string?,
    group: string?,
    desc:  string?,
) StepInstance

step symlink(
    target: string,
    link:   string,
    desc:   string?,
) StepInstance

struct TemplateData {
    values: map[string, any] = {}
    env:    map[string, string] = {}
}

step template(
    src:    Source,
    dest:   string,
    data:   TemplateData?,
    perm:   string,
    owner:  string,
    group:  string,
    verify: string?,
    desc:   string?,
) StepInstance

step unarchive(
    src:   Source,
    dest:  string,
    depth: int = 0,
    owner: string?,
    group: string?,
    perm:  string?,
    desc:  string?,
) StepInstance

# Package management
# ---------------------------------------------------------------------------

step pkg(
    packages: list[string],
    source:   PkgSource,
    state:    PkgState = PkgState.present,
    desc:     string?,
) StepInstance

# Service management
# ---------------------------------------------------------------------------

step service(
    name:    string,
    state:   SvcState = SvcState.running,
    enabled: bool = true,
    desc:    string?,
) StepInstance

# User and group management
# ---------------------------------------------------------------------------

step user(
    name:     string,
    state:    UserState = UserState.present,
    shell:    string?,
    home:     string?,
    system:   bool = false,
    password: string?,
    groups:   list[string] = [],
    desc:     string?,
) StepInstance

step group(
    name:   string,
    state:  GroupState = GroupState.present,
    gid:    int?,
    system: bool = false,
    desc:   string?,
) StepInstance

# System configuration
# ---------------------------------------------------------------------------

step sysctl(
    key:     string,
    value:   string,
    persist: bool = true,
    desc:    string?,
) StepInstance

step mount(
    src:   string,
    dest:  string,
    type:  FsType,
    opts:  string = "defaults",
    state: MountState = MountState.mounted,
    desc:  string?,
) StepInstance

step firewall(
    port:   string,
    action: FwAction = FwAction.allow,
    desc:   string?,
) StepInstance

# Command execution
# ---------------------------------------------------------------------------

step run(
    apply:  string,
    check:  string?,       # mutually exclusive with always
    always: bool = false,  # mutually exclusive with check
    desc:   string?,
) StepInstance

# Container management (in std/container)
# ---------------------------------------------------------------------------

step container.instance(
    name:        string,
    image:       string,
    state:       CtrState = CtrState.running,
    restart:     CtrRestart = CtrRestart.unless_stopped,
    ports:       list[string]?,
    env:         map[string, string]?,
    mounts:      list[string]?,
    args:        list[string]?,
    labels:      map[string, string]?,
    healthcheck: Healthcheck?,
    desc:        string?,
) StepInstance

# REST (in std/rest)
# ---------------------------------------------------------------------------

step rest.request(
    method:  HttpMethod,
    path:    string,
    headers: map[string, string]?,
    body:    Body?,
    check:   Check?,
    desc:    string?,
) StepInstance

step rest.resource(
    query:    rest.request,
    missing:  rest.request?,
    found:    rest.request?,
    bindings: map[string, Check]?,
    state:    map[string, any]?,
    desc:     string?,
) StepInstance
```

---

## 8. Full example: real infrastructure

The following translates the existing `.infra/` configs to scampi-lang.
Assumes the project's `scampi.mod` declares:

```
module codeberg.org/scampi-dev/infra

require (
    codeberg.org/scampi-dev/scampi v0.1.0
)
```

Intra-project imports use the full canonical path (no `./` shortcut,
matching Go).

### 8.1 targets.scampi — shared target definitions

```
import "std"
import "std/target"

std.secrets {
    backend = "age"
    path    = "secrets.age.json"
}

let vps_host = std.secret("vps.host")

let vps = target.ssh {
    name = "vps"
    host = vps_host
    user = "hal9000"
}

let vps_root = target.ssh {
    name = "vps-root"
    host = vps_host
    user = "root"
}
```

### 8.2 bootstrap.scampi — initial server setup

```
import "std"
import "codeberg.org/scampi-dev/infra/targets"

std.deploy {
    name    = "bootstrap"
    targets = [targets.vps_root]

    let restart_sshd = std.service { name = "sshd", state = restarted }

    std.user {
        desc   = "automation user with sudo"
        name   = "hal9000"
        shell  = "/bin/bash"
        home   = "/home/hal9000"
        groups = ["sudo"]
    }

    std.pkg { packages = ["sudo"], source = std.system {} }

    std.copy {
        desc   = "passwordless sudo for hal9000"
        src    = std.inline { content = "hal9000 ALL=(ALL) NOPASSWD:ALL\n" }
        dest   = "/etc/sudoers.d/hal9000"
        perm   = "0440"
        owner  = "root"
        group  = "root"
        verify = "visudo -cf %s"
    }

    std.dir {
        path  = "/home/hal9000/.ssh"
        perm  = "0700"
        owner = "hal9000"
        group = "hal9000"
    }

    std.copy {
        desc  = "authorize SSH key for hal9000"
        src   = std.local { path = "./files/hal9000_authorized_keys" }
        dest  = "/home/hal9000/.ssh/authorized_keys"
        perm  = "0600"
        owner = "hal9000"
        group = "hal9000"
    }

    std.copy {
        desc      = "harden sshd config"
        src       = std.local { path = "./files/sshd_hardened.conf" }
        dest      = "/etc/ssh/sshd_config.d/hardened.conf"
        perm      = "0644"
        owner     = "root"
        group     = "root"
        verify    = "sshd -t -f %s"
        on_change = [restart_sshd]
    }
}
```

### 8.3 harden.scampi — system hardening

```
import "std"
import "codeberg.org/scampi-dev/infra/targets"

std.deploy {
    name    = "harden"
    targets = [targets.vps]

    let restart_fail2ban = std.service {
        name = "fail2ban", state = restarted
    }
    let restart_unattended_upgrades = std.service {
        name = "unattended-upgrades", state = restarted
    }

    std.pkg {
        desc     = "install hardening packages"
        packages = ["fail2ban", "ufw", "unattended-upgrades"]
        source   = std.system {}
    }

    std.copy {
        desc      = "fail2ban SSH jail config"
        src       = std.local { path = "./files/fail2ban-sshd.conf" }
        dest      = "/etc/fail2ban/jail.d/sshd.conf"
        perm      = "0644"
        owner     = "root"
        group     = "root"
        on_change = [restart_fail2ban]
    }
    std.service { name = "fail2ban", state = running, enabled = true }

    std.firewall { port = "22/tcp",  desc = "allow SSH" }
    std.firewall { port = "80/tcp",  desc = "allow HTTP" }
    std.firewall { port = "443/tcp", desc = "allow HTTPS" }
    std.service { name = "ufw", state = running, enabled = true }

    std.copy {
        desc      = "configure unattended-upgrades"
        src       = std.local { path = "./files/50unattended-upgrades" }
        dest      = "/etc/apt/apt.conf.d/50unattended-upgrades"
        perm      = "0644"
        owner     = "root"
        group     = "root"
        on_change = [restart_unattended_upgrades]
    }
    std.copy {
        desc      = "enable auto-upgrades"
        src       = std.local { path = "./files/20auto-upgrades" }
        dest      = "/etc/apt/apt.conf.d/20auto-upgrades"
        perm      = "0644"
        owner     = "root"
        group     = "root"
        on_change = [restart_unattended_upgrades]
    }
    std.service { name = "unattended-upgrades", state = running, enabled = true }
}
```

### 8.4 runner.scampi — CI runner setup

```
import "std"
import "codeberg.org/scampi-dev/infra/targets"

let runner_version = "12.7.2"
let runner_url = "https://code.forgejo.org/forgejo/runner/releases/download/v${runner_version}/forgejo-runner-${runner_version}-linux-amd64"

let go_version = "1.26.1"
let go_url = "https://go.dev/dl/go${go_version}.linux-amd64.tar.gz"

let just_version = "1.46.0"
let just_url = "https://github.com/casey/just/releases/download/${just_version}/just-${just_version}-x86_64-unknown-linux-musl.tar.gz"

std.deploy {
    name    = "runner"
    targets = [targets.vps]

    let restart_runner = std.service { name = "forgejo-runner", state = restarted }
    let restart_docker = std.service { name = "docker", state = restarted }

    std.pkg {
        desc     = "install build tools"
        packages = ["git", "shellcheck", "curl", "nodejs", "npm", "gcc", "libc6-dev", "jq"]
        source   = std.system {}
    }

    # Go
    std.unarchive {
        desc = "install Go ${go_version}"
        src  = std.remote { url = go_url }
        dest = "/usr/local"
    }
    std.copy {
        desc  = "Go PATH profile"
        src   = std.inline { content = "export PATH=\"/usr/local/go/bin:\$PATH\"\n" }
        dest  = "/etc/profile.d/go.sh"
        perm  = "0644"
        owner = "root"
        group = "root"
    }
    std.symlink { target = "/usr/local/go/bin/go", link = "/usr/local/bin/go" }

    # just
    std.unarchive {
        desc = "install just ${just_version}"
        src  = std.remote { url = just_url }
        dest = "/usr/local/bin"
    }

    # Docker
    std.pkg {
        desc     = "install Docker Engine"
        packages = [
            "docker-ce", "docker-ce-cli", "containerd.io",
            "docker-buildx-plugin", "docker-compose-plugin",
        ]
        source = std.apt_repo {
            url        = "https://download.docker.com/linux/debian"
            key_url    = "https://download.docker.com/linux/debian/gpg"
            components = ["stable"]
        }
    }
    std.copy {
        desc      = "enable Docker BuildKit"
        src       = std.inline { content = "{\"features\":{\"buildkit\":true}}\n" }
        dest      = "/etc/docker/daemon.json"
        perm      = "0644"
        owner     = "root"
        group     = "root"
        on_change = [restart_docker]
    }
    std.service { name = "docker", state = running, enabled = true }

    # Runner user
    std.user {
        desc   = "forgejo-runner service account"
        name   = "forgejo-runner"
        shell  = "/bin/bash"
        home   = "/home/forgejo-runner"
        system = true
        groups = ["docker"]
    }

    # Runner binary
    std.copy {
        desc  = "install forgejo-runner ${runner_version}"
        src   = std.remote { url = runner_url }
        dest  = "/usr/local/bin/forgejo-runner"
        perm  = "0755"
        owner = "root"
        group = "root"
    }

    std.dir {
        path  = "/var/lib/forgejo-runner"
        perm  = "0755"
        owner = "forgejo-runner"
        group = "forgejo-runner"
    }

    std.copy {
        desc      = "forgejo-runner config"
        src       = std.local { path = "./files/forgejo-runner-config.yml" }
        dest      = "/var/lib/forgejo-runner/config.yml"
        perm      = "0640"
        owner     = "forgejo-runner"
        group     = "forgejo-runner"
        on_change = [restart_runner]
    }

    std.copy {
        desc      = "forgejo-runner systemd unit"
        src       = std.local { path = "./files/forgejo-runner.service" }
        dest      = "/etc/systemd/system/forgejo-runner.service"
        perm      = "0644"
        owner     = "root"
        group     = "root"
        on_change = [restart_runner]
    }

    std.run {
        desc  = "disable tmpfs for /tmp"
        check = "systemctl is-enabled tmp.mount 2>/dev/null | grep -q masked"
        apply = "sudo systemctl mask tmp.mount"
    }

    std.service { name = "forgejo-runner", state = running, enabled = true }
}
```

### 8.5 tools.scampi — quality-of-life packages

```
import "std"
import "codeberg.org/scampi-dev/infra/targets"

std.deploy {
    name    = "tools"
    targets = [targets.vps]

    std.pkg {
        desc     = "install user tools"
        packages = ["neovim", "htop", "btop"]
        source   = std.system {}
    }
}
```

### 8.6 User-defined step with iteration

```
import "std"
import "codeberg.org/scampi-dev/infra/targets"

struct TeamMember {
    name:   string
    groups: list[string]
    shell:  string = "/bin/bash"
    admin:  bool = false
}

step create_user(member: TeamMember) StepInstance {
    std.user {
        name   = self.member.name
        groups = self.member.groups
        shell  = self.member.shell
    }

    std.dir {
        path  = "/home/${self.member.name}/.ssh"
        perm  = "0700"
        owner = self.member.name
        group = self.member.name
    }

    if self.member.admin {
        std.copy {
            src   = std.inline { content = "${self.member.name} ALL=(ALL) NOPASSWD:ALL\n" }
            dest  = "/etc/sudoers.d/${self.member.name}"
            perm  = "0440"
            owner = "root"
            group = "root"
            verify = "visudo -cf %s"
        }
    }
}

let team = [
    TeamMember { name = "alice", groups = ["wheel", "dev"], admin = true },
    TeamMember { name = "bob",   groups = ["dev"], shell = "/bin/zsh" },
    TeamMember { name = "carol", groups = ["ops", "dev"] },
]

std.deploy {
    name    = "users"
    targets = [targets.vps]

    std.pkg.present ["shadow-utils"]

    for m in team {
        create_user { member = m }
    }
}
```

### 8.7 UniFi module — network management

This example shows `func` with mutable collection contents — the imperative
data-building pattern that is only allowed inside function bodies.

```
import "codeberg.org/scampi-dev/modules/unifi/api"

func network(
    site_id:   string,
    name:      string,
    vlan_id:   int,
    subnet:    string,
    gateway:   string,
    dhcp:      map[string, string]? = none,
    dns:       list[string]? = none,
    domain:    string? = none,
    isolation: bool = false,
    internet:  bool = true,
    mdns:      bool = false,
    enabled:   bool = true,
    desc:      string? = none,
) map[string, any] {
    let parts = subnet.split("/")
    let prefix = if parts.len() > 1 { int(parts[1]) } else { 24 }

    # mutable map — building up state imperatively inside fn
    let ipv4 = {
        "autoScaleEnabled": false,
        "hostIpAddress": gateway,
        "prefixLength": prefix,
    }

    if dhcp != none {
        let dhcp_cfg = {
            "mode": "SERVER",
            "ipAddressRange": {
                "start": dhcp["start"],
                "end": dhcp["end"],
            },
            "leaseTimeSeconds": dhcp.get("lease_time", 86400),
            "pingConflictDetectionEnabled": true,
        }
        if dns != none {
            dhcp_cfg["dnsServerIpAddressesOverride"] = dns
        }
        if domain != none {
            dhcp_cfg["domainName"] = domain
        }
        ipv4["dhcpConfiguration"] = dhcp_cfg
    }

    return {
        "management": "GATEWAY",
        "name": name,
        "enabled": enabled,
        "vlanId": vlan_id,
        "isolationEnabled": isolation,
        "internetAccessEnabled": internet,
        "mdnsForwardingEnabled": mdns,
        "cellularBackupEnabled": false,
        "ipv4Configuration": ipv4,
    }
}

# Consumer uses the func result in a frozen deploy context
import "std"
import "std/rest"
import "std/target"

let udm = target.rest {
    name     = "udm"
    base_url = "https://udm.local/proxy/network"
    auth     = rest.bearer {
        token_endpoint = "/integration/v1/auth"
        identity       = std.secret("udm.id")
        secret         = std.secret("udm.secret")
    }
}

std.deploy {
    name    = "network"
    targets = [udm]

    let state = network(
        site_id = "default",
        name    = "Servers",
        vlan_id = 2,
        subnet  = "192.0.2.0/24",
        gateway = "192.0.2.1",
        dhcp    = {"start": "192.0.2.100", "end": "192.0.2.254"},
    )
    # state is frozen here — no mutation possible

    rest.resource {
        desc    = "network: Servers"
        query   = api.get_networks_overview_page {
            site_id = "default"
            check   = rest.jq { expr = ".data[] | select(.name == \"Servers\")" }
        }
        missing = api.create_network { site_id = "default" }
        found   = api.update_network { site_id = "default", network_id = "{id}" }
        bindings = {"id": rest.jq { expr = ".id" }}
        state   = state
    }
}
```

---

## 9. Evaluation model

1. The compiler parses all `.scampi` files in the project, resolves
   imports, and type-checks the entire program.
2. The evaluator runs the program top-to-bottom. Variables are bound,
   functions are called, loops are unrolled, conditionals are evaluated.
   Step invocations produce typed values.
3. After evaluation, the engine collects top-level values by type:
   - Exactly zero or one `SecretsConfig`
   - Zero or more `Target`
   - One or more `Deploy` (each carrying its collected desired-state
     `StepInstance` values and any reactive steps referenced from
     `on_change` lists)
4. The engine receives this collection. No language code runs after
   this point.

The evaluation is deterministic and hermetic. No filesystem access, no
network access, no randomness. The only external inputs are environment
variables (`std.env()`) and secrets (`std.secret()`).

---

## 10. Error model

Errors are compile-time or eval-time. There are no runtime errors (the
engine has its own error model for execution failures).

**Compile-time errors:**
- Type mismatches
- Unknown fields in step blocks
- Missing required fields
- Unknown imports
- Ambiguous bare enum variants

**Eval-time errors:**
- Division by zero
- Index out of bounds
- Missing environment variable (no default)
- Missing secret key
- `none` used where non-optional expected

All errors carry source location (file, line, column) and produce a hint
suggesting the fix.

---

## 11. What's not in the language

Explicitly excluded:

| Feature                  | Why                                                                   |
| ------------------------ | --------------------------------------------------------------------- |
| Exceptions / try-catch   | Steps converge or fail — the engine handles failure                   |
| Concurrency              | The DAG scheduler handles parallelism                                 |
| Classes / inheritance    | Structs + step compositions cover all use cases                       |
| Dynamic attribute access | Prevents sound rename/refactor in LSP                                 |
| Eval / exec / reflection | Breaks static analysis                                                |
| Null (general purpose)   | `none` only exists for optional types                                 |
| Generics                 | Not needed — collection types are built-in                            |
| Pattern matching         | `if`/`else` chains are sufficient for v1                              |
| Operator overloading     | Complexity without benefit for config language                        |
| Mutable bindings         | `let` names are immutable. Collection mutation only in `func` bodies. |

---

## 12. Grammar (EBNF sketch)

```ebnf
file           = (use_decl | declaration | statement)* ;

use_decl       = 'use' use_path ('as' IDENT)? ;
use_path       = (IDENT '.')* (IDENT | '*')
               | STRING ('.' (IDENT | '*'))? ;

declaration    = step_decl | struct_decl | enum_decl | fn_decl ;

step_decl      = 'step' dotted_name '(' params ')' type_expr?
                 ('{' block_body '}')? ;
struct_decl    = 'struct' IDENT '{' field_defs '}' ;
enum_decl      = 'enum' IDENT '{' (IDENT ',')* '}' ;

field_defs     = (IDENT ':' type_expr ('=' expr)?)* ;
field_assigns  = (IDENT '=' expr)* ;


step_inst      = dotted_name '{' field_assigns '}'
               | dotted_name STRING
               | dotted_name list_expr ;

statement      = let_stmt | for_stmt | if_stmt | step_inst | expr ;

let_stmt       = 'let' IDENT ('=' expr) ;
for_stmt       = 'for' IDENT 'in' expr '{' block_body '}' ;
if_stmt        = 'if' expr '{' block_body '}' ('else' (if_stmt | '{' block_body '}'))? ;

block_body     = (statement | step_inst)* ;

fn_decl        = 'func' IDENT '(' params ')' type_expr? '{' fn_body '}' ;
params         = (IDENT ':' type_expr ('=' expr)? ',')* ;
fn_body        = (let_stmt | for_stmt | if_stmt | return_stmt | expr)* ;
return_stmt    = 'return' expr ;

type_expr      = IDENT
               | IDENT '[' type_expr (',' type_expr)* ']'
               | type_expr '?' ;

expr           = literal | IDENT | dotted_name | expr '.' IDENT
               | expr '[' expr ']' | expr '(' args ')'
               | expr binop expr | unop expr
               | if_expr | list_expr | map_expr | struct_lit
               | list_comp | map_comp | '(' expr ')' ;

if_expr        = 'if' expr '{' expr '}' 'else' '{' expr '}' ;
list_expr      = '[' (expr ',')* ']' ;
map_expr       = '{' (expr ':' expr ',')* '}' ;
struct_lit     = IDENT? '{' (IDENT '=' expr ',')* '}' ;
list_comp      = '[' expr 'for' IDENT 'in' expr ('if' expr)? ']' ;
map_comp       = '{' expr ':' expr 'for' IDENT (',' IDENT)? 'in' expr ('if' expr)? '}' ;

dotted_name    = IDENT ('.' IDENT)* ;
args           = (IDENT '=' expr ',')* ;

literal        = INT | STRING | BOOL | 'none' ;
binop          = '+' | '-' | '*' | '/' | '%'
               | '==' | '!=' | '<' | '>' | '<=' | '>='
               | '&&' | '||' | 'in' ;
unop           = '!' | '-' ;
```

---

## 13. Implementation notes

### 13.1 Parser

Hand-rolled recursive descent in Go. The grammar is ~30 productions and
fits comfortably in a single file. Error recovery is critical for LSP
support — the parser must produce a partial AST from broken source.

### 13.2 Type checker

Single-pass type checker operating on the AST. All type information is
resolved from explicit annotations (function signatures, step definitions,
struct fields). Local variable types are inferred from their initializer.

### 13.3 Evaluator

Tree-walking evaluator. The language is simple enough that compilation to
bytecode is unnecessary. Evaluation produces a list of step declarations
consumed by the engine.

### 13.4 LSP integration

The LSP operates on the type-checked AST. Core features:

- **Completion**: field names in step blocks, enum variants, imported symbols
- **Hover**: type signatures, step documentation, field descriptions
- **Go to definition**: across files, for steps, types, enums, functions
- **Rename**: sound cross-file rename (no dynamic dispatch, no reflection)
- **Diagnostics**: type errors, missing fields, unknown symbols — all at
  keystroke speed
- **Signature help**: function parameters with types and defaults

### 13.5 Stub generation

A `go generate` command reads the Go step/target config structs (using
struct tags) and emits `.scampi` stub files for the standard library. This
ensures the language-side type signatures always match the Go
implementations.

### 13.6 Tree-sitter grammar

A separate tree-sitter grammar for syntax highlighting in editors. This
replaces the current tree-sitter-starlark extension.

---

## 14. Migration path

1. **Phase 0**: This spec. Iterate until the syntax is locked.
2. **Phase 1**: Parser + type checker + evaluator. Can parse and evaluate
   `.scampi` files into the existing `spec.StepInstance` format.
3. **Phase 2**: Stub generation from Go struct tags. Standard library
   available.
4. **Phase 3**: LSP on the new language. Replaces scampls' Starlark
   evaluation.
5. **Phase 4**: Tree-sitter grammar. Replaces tree-sitter-starlark
   extension.
6. **Phase 5**: Migrate `.infra/` and `.modules/` configs. Old Starlark
   evaluator removed.

The engine, steps, targets, and everything below the evaluation layer are
unchanged. The language replacement is purely a frontend swap.
