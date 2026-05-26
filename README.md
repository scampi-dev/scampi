<img src="scampi.png" alt="scampi" width="200" align="right">

# scampi

[![License](https://img.shields.io/badge/license-GPL--3.0-blue)](https://github.com/scampi-dev/scampi/blob/main/LICENSE)
[![Security Policy](https://img.shields.io/badge/security-policy-critical?logo=gnuprivacyguard&logoColor=white)](SECURITY.md)
[![Latest Release](https://img.shields.io/github/v/release/scampi-dev/scampi?include_prereleases&label=release&color=blue)](https://github.com/scampi-dev/scampi/releases)
[![Platforms](https://img.shields.io/badge/platforms-linux%20%7C%20macos%20%7C%20freebsd-lightgrey)](https://github.com/scampi-dev/scampi/releases/latest)
[![Install](https://img.shields.io/badge/install-get.scampi.dev-2ea44f)](https://get.scampi.dev)

> [!IMPORTANT]
> **Found a security issue?** Please follow the [security policy](SECURITY.md) — don't open a public issue. Sensitive reports can be PGP-encrypted; the key is published in `SECURITY.md`.

Hi, I'm **scampi** — your friendly infrastructure crustacean. I do IaC convergence, garlic buttery smooth.

## What is scampi?

A declarative system convergence engine. You describe desired system state in scampi; **scampi** executes idempotent operations to converge reality to that state.

Think Ansible or Terraform, but with:
- **Own language** instead of YAML/HCL (actual programming, not templating hell)
- **Built-in steps** instead of plugin sprawl (batteries included)
- **Deterministic execution** with fail-fast semantics (no half-applied mystery states)

## Quick Start

Install scampi (one-time):

```bash
curl https://get.scampi.dev | sh
```

Then write a config and run it:

```bash
# Create a config that renders a template
cat > hello.scampi <<EOF
module main

import "std"
import "std/posix"
import "std/local"

let my_machine = local.target { name = "local" }

std.deploy(name = "hello", targets = [my_machine]) {
  posix.template {
    desc  = "render a greeting"
    src   = posix.source_inline { content = "Hello {{ .my_val }}!" }
    dest  = "/tmp/scampi-hello.txt"
    data  = { "values": { "my_val": "world" } }
    perm  = "0644"
    owner = "$(id -un)"
    group = "$(id -gn)"
  }
}
EOF

# Show scampi's execution plan
scampi plan hello.scampi

# Check what would happen
scampi check hello.scampi

# Punch it
scampi apply hello.scampi

# Apply a second time — idempotent, nothing to do
scampi apply hello.scampi

# Verify
cat /tmp/scampi-hello.txt
```

Stack `-v` flags on any of those for more detail — `-v` (why), `-vv` (how), `-vvv` (everything). Quiet → `-vvv` is a *brutal* jump; add as many `v`s as you can stomach.

## From source

Prefer building from a checkout? You'll need [Go](https://go.dev) and [`just`](https://github.com/casey/just):

```bash
git clone https://github.com/scampi-dev/scampi
cd scampi
just build           # produces ./build/bin/scampi and ./build/bin/scampls
```

For ongoing development, `just scampi <args>` is a rebuild-on-change wrapper that always runs the latest source.

## Why does this exist?

Because configuration drift is inevitable, but suffering through YAML isn't.

**scampi** ensures your systems stay in the state you declared — idempotent, deterministic, and with error messages that actually help you fix things instead of sending you on a documentation scavenger hunt.

## Why "scampi"?

Because naming things is hard, and sometimes the best names have nothing to do with what they do.

**scampi** is garlic butter shrimp. It's delicious, it's memorable, and it's absurd enough that you'll never forget it. In the grand tradition of infrastructure tools with nonsensical names (git, ansible, terraform), **scampi** joins the pantheon.

The command is `scampi`. You're welcome to alias it to whatever you want — `scam` or `scm` if you're lazy:

```bash
scampi apply
scampi plan
scampi check
```

### Corporate Translation Guide

If you're a tech lead who needs to sell your favorite IaC tool to corporate suits, here are some handy backronyms for **SCAMPI**:

- **S**ystem **C**onfiguration **A**nd **M**anagement **P**latform for **I**nfrastructure
- **S**ecure **C**ontinuous **A**utomation & **M**ulti-cloud **P**rovisioning for **I**nfrastructure
- **S**caled **C**onvergence **A**ssurance **M**iddleware **P**latform **I**ntegration
- **S**oftware-defined **C**ompliance **A**utomation **M**onitoring **P**ipeline for **I**T

Or just own it: "We've standardized on **scampi** for infrastructure convergence." Watch them nod seriously while you're internally screaming about shrimp.

## Development

Want to hack on scampi? See [`CONTRIBUTING.md`](CONTRIBUTING.md) — covers the build/test workflow, the design docs under `doc/design/`, the commit style, and the project's code conventions.

## License

See [LICENSE](LICENSE).

---

*🍤 Made with garlic butter and strong opinions*
