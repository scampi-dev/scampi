<img src="scampi.png" alt="scampi" width="200">

# scampi

[![CI](https://codeberg.org/scampi-dev/scampi/badges/workflows/ci.yml/badge.svg?branch=main)](https://codeberg.org/scampi-dev/scampi/actions?workflow=ci.yml) [![License](https://img.shields.io/badge/license-GPL--3.0-blue)](https://codeberg.org/scampi-dev/scampi/src/branch/main/LICENSE)

Hi, I'm **scampi** — your friendly infrastructure crustacean. I do IaC convergence, garlic buttery smooth.

## What is scampi?

A declarative system convergence engine. You describe desired system state in Starlark; **scampi** executes idempotent operations to converge reality to that state.

Think Ansible or Terraform, but with:
- **Starlark** instead of YAML/HCL (actual programming, not templating hell)
- **Built-in steps** instead of plugin sprawl (batteries included)
- **Deterministic execution** with fail-fast semantics (no half-applied mystery states)

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
- **S**tate **C**onvergence **A**utomation and **M**aintenance **P**latform **I**nterface
- **S**calable **C**onfiguration **A**pplication **M**anagement **P**latform **I**nfrastructure
- **S**erver **C**ompliance **A**nd **M**aintenance **P**latform **I**T

Or just own it: "We've standardized on **scampi** for infrastructure convergence." Watch them nod seriously while you're internally screaming about shrimp.

## Quick Start

```bash
# Create a config that renders a template
cat > hello.star <<EOF
target.local(name="local")

deploy(name="demo", targets=["local"], steps=[
    template(
        src=inline("Hello {{ .user }}! Your shell: {{ .shell }}"),
        dest="/tmp/scampi-hello.txt",
        data={
            "values": {
                "user": "world",
                "shell": "/bin/sh",
            },
            "env": {
                "USER": "user",
                "SHELL": "shell",
            },
        },
        perm="0644",
        owner="$(id -un)",
        group="$(id -gn)",
    ),
])
EOF

# Check what would happen
just scampi plan hello.star

# Make it so
just scampi apply hello.star

# Verify
cat /tmp/scampi-hello.txt
```

## Why does this exist?

Because configuration drift is inevitable, but suffering through YAML isn't.

**scampi** ensures your systems stay in the state you declared — idempotent, deterministic, and with error messages that actually help you fix things instead of sending you on a documentation scavenger hunt.

## Development

See the docs:
- `docs/naming.md` — terminology and concepts
- `docs/units-targets-vars.md` — configuration model
- `docs/cli-semantics.md` — CLI output and verbosity

Commands:
```bash
just build       # Build CLI to ./build/bin/scampi
just test        # Run all tests
just lint        # Run golangci-lint
just fmt         # Format code
```

## License

See [LICENSE](LICENSE)

---

*🍤 Made with garlic butter and strong opinions*
