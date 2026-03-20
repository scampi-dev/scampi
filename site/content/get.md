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

```sh
curl -fsSL get.scampi.dev | sh
```

Downloads the latest release, verifies the SHA256 checksum, and installs to `~/.local/bin`.

Supported platforms: Linux, macOS, and FreeBSD (amd64/arm64).

## Go

```sh
go install scampi.dev/scampi/cmd/scampi@latest
```

Requires Go {{< go-version >}}+.

## Manual download

Prebuilt binaries for all supported platforms are available on
[Codeberg releases](https://codeberg.org/scampi-dev/scampi/releases).

Download the binary for your platform, verify against `SHA256SUMS`, and place it on your `PATH`.

## Build from source

```sh
git clone https://codeberg.org/scampi-dev/scampi.git
cd scampi
just build
```

The binary lands at `./build/bin/scampi`. Requires Go {{< go-version >}}+ and [just](https://just.systems).
