---
title: ""
linkTitle: Install
weight: 5
type: docs
sidebar:
  open: true
---

<style>
/* Kill spacing from blank title */
#content > br, #content > h1 { display: none; }
#content > .content { margin-top: 0; }
#content > div.hx\:mb-16 { margin-bottom: 1.9rem; }
</style>

{{< hextra/hero-badge link="https://github.com/scampi-dev/scampi/releases" >}}
  <span>All releases</span>
  {{< icon name="arrow-circle-right" attributes="height=14" >}}
{{< /hextra/hero-badge >}}

{{< hextra/hero-headline >}}
  Get scampi
{{< /hextra/hero-headline >}}

{{< hextra/hero-subtitle >}}
  Pick your method. Be converging in seconds.
{{< /hextra/hero-subtitle >}}

## One-liner

```bash
curl get.scampi.dev | sh
```

<img src="/scampi-get.png" alt="scampi mascot" style="float: right; width: 160px; margin: 0 0 1rem 1.5rem;">

This fetches [`install.sh`](/get/install-source/) from `get.scampi.dev`
and pipes it into POSIX `sh`. The script is short enough to read in
full before you trust it; it downloads the latest release of **both
`scampi` and `scampls`** (the LSP server), verifies the release SSH
signature on `SHA256SUMS`, checks each binary's SHA256, and installs
them to `~/.local/bin` (or `/usr/local/bin` if it doesn't exist).

**Just the CLI** (e.g. CI runners):

```bash
curl get.scampi.dev/cli | sh
```

**Just the LSP:**

```bash
curl get.scampi.dev/lsp | sh
```

**Custom path:**

```bash
curl get.scampi.dev | sh -s -- -d ~/.local/bin
```

Supported platforms: Linux, macOS, and FreeBSD (amd64/arm64).

## Go

```bash
go install scampi.dev/scampi/cmd/scampi@latest
go install scampi.dev/scampi/cmd/scampls@latest
```

Requires Go {{< go-version >}}+.

## Manual download

Prebuilt binaries for all supported platforms are available on the
[GitHub releases page](https://github.com/scampi-dev/scampi/releases).

Download the binary for your platform, verify against `SHA256SUMS` (and ideally
the [signature](#verify-a-release) too), and place it on your `PATH`.

## Verify a release

<img src="/scampi-sec.png" alt="security scampi mascot" style="float: right; width: 140px; margin: 0 0 1rem 1.5rem;">

The install one-liner already verifies the release signature for you.
This section is for the rest — the people who'd rather download
manually and check before running.

Every release after `v0.1.0-alpha.7` ships a signed `SHA256SUMS`
manifest.[^pre-signing] The signing identity is `releases@scampi.dev`.
Verification needs only `ssh-keygen` — the binary that comes with
OpenSSH on every Linux, macOS, and BSD install — no gpg, no cosign,
no minisign.

That's a deliberate choice. Every release-signing scheme grounds its
trust in *some* tool the user already has. Schemes built on minisign,
cosign, or gpg-on-macOS extend the trust loop one step further: you
have to install the verifier before you can verify, which means the
package manager that delivered the verifier is now part of the chain
too. SSH skips that step. `ssh-keygen` is on every machine that talks
to a remote server, which is every machine you'd plausibly use scampi
from. The trust chain stops at the shortest possible point.

[^pre-signing]: Earlier alpha releases (`v0.1.0-alpha.1` through
    `v0.1.0-alpha.7`) ship the `SHA256SUMS` file but no signature.
    They predate the signing key and won't be retroactively signed.

### One-time setup

Save the scampi release signer to your SSH config:

```bash
mkdir -p ~/.ssh && chmod 700 ~/.ssh
cat >> ~/.ssh/scampi_allowed_signers <<'EOF'
releases@scampi.dev ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIEDBbJOSWyfk9kJhHjUmSJVIax9lxGnOjwpL4dSheQfu
EOF
chmod 600 ~/.ssh/scampi_allowed_signers
```

This is a one-shot. From here on, verification reuses the saved entry —
same place you'd keep any other SSH peer trust decision.

### Per release

```bash
TAG={{< scampi-version >}}
URL="https://dl.scampi.dev/${TAG}"

curl -fLO "${URL}/SHA256SUMS" "${URL}/SHA256SUMS.sig"
ssh-keygen -Y verify -I releases@scampi.dev -n file \
  -f ~/.ssh/scampi_allowed_signers \
  -s SHA256SUMS.sig < SHA256SUMS

# Then for each binary you grabbed:
sha256sum --ignore-missing -c SHA256SUMS
```

> [!WARNING]
> **If the signature verification fails, DO NOT RUN THE BINARY.**
>
> Cross-check the line you saved against the same one in the
> [`install.sh`](/get/install-source/) we serve and in
> [`SECURITY.md`](https://github.com/scampi-dev/scampi/blob/main/SECURITY.md)
> on the canonical GitHub repo — those three places are deliberately
> kept in sync, so a mismatch anywhere is a red flag worth reporting
> via the
> [security policy](https://github.com/scampi-dev/scampi/blob/main/SECURITY.md).

## Build from source

```bash
git clone https://github.com/scampi-dev/scampi.git
cd scampi
just build
```

Produces `./build/bin/scampi` and `./build/bin/scampls`. Requires Go {{< go-version >}}+ and [just](https://just.systems).
