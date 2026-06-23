---
title: install.sh
linkTitle: install.sh source
description: The install script served at get.scampi.dev.
type: docs
---

`curl get.scampi.dev | sh` pipes [`/install.sh`](/install.sh) into your
shell. `get.scampi.dev` returns the script body to CLI clients (curl,
wget, fetch); browsers get redirected back to this page.

Read it before you trust it — it's short, downloads the latest release
of `scampi`, verifies the SSH signature on `SHA256SUMS`, checks the
binary's SHA256, and installs to `~/.local/bin` (or `/usr/local/bin`):

- served raw at [/install.sh](/install.sh)
- canonical source: [scripts/install.sh](https://github.com/scampi-dev/scampi/blob/main/scripts/install.sh)

The served copy is the canonical script verbatim — there's one
`install.sh`, copied into the site at build time.
