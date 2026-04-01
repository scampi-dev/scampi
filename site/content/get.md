---
title: ""
linkTitle: Install
---

<style>
/* Kill spacing from blank title */
#content > br, #content > h1 { display: none; }
#content > .content { margin-top: 0; }
#content > div.hx\:mb-16 { margin-bottom: 1.9rem; }
/* Mascot in left margin */
.get-mascot {
  position: sticky;
  top: 6rem;
  float: left;
  margin-left: -200px;
  width: 160px;
}
@media (max-width: 1280px) { .get-mascot { display: none; } }
</style>

<img src="/scampi-get.png" alt="scampi mascot" class="get-mascot">

{{< hextra/hero-badge link="https://codeberg.org/scampi-dev/scampi/releases" >}}
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
curl -fsSL get.scampi.dev | sh
```

Downloads the latest release of **both `scampi` and `scampls`** (the LSP
server), verifies SHA256 checksums, and installs to `~/.local/bin` (or
`/usr/local/bin` if it doesn't exist).

**Just the CLI** (e.g. CI runners):

```bash
curl -fsSL get.scampi.dev/cli | sh
```

**Just the LSP:**

```bash
curl -fsSL get.scampi.dev/lsp | sh
```

**Custom path:**

```bash
curl -fsSL get.scampi.dev | sh -s -- -o ~/.local/bin
```

Supported platforms: Linux, macOS, and FreeBSD (amd64/arm64).

## Go

```bash
go install scampi.dev/scampi/cmd/scampi@latest
go install scampi.dev/scampi/cmd/scampls@latest
```

Requires Go {{< go-version >}}+.

## Manual download

Prebuilt binaries for all supported platforms are available on
[Codeberg releases](https://codeberg.org/scampi-dev/scampi/releases).

Download the binary for your platform, verify against `SHA256SUMS`, and place it on your `PATH`.

## Build from source

```bash
git clone https://codeberg.org/scampi-dev/scampi.git
cd scampi
just build
```

Produces `./build/bin/scampi` and `./build/bin/scampls`. Requires Go {{< go-version >}}+ and [just](https://just.systems).
