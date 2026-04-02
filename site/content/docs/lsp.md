---
title: LSP
weight: 8
---

`scampls` is the Language Server Protocol (LSP) server for scampi configuration
files. It provides real-time diagnostics, completion, hover documentation, and
signature help in any editor that supports LSP.

## Features

- **Full diagnostics** — runs the real Starlark evaluation pipeline, catching
  unknown kwargs, missing required fields, type errors, invalid enum values,
  and syntax errors with precise source spans
- **Completion** — step builtins, kwargs inside calls, module members
  (`target.`, `rest.`, `container.`)
- **Hover documentation** — function signatures with parameter tables, kwarg
  docs with type/default/examples
- **Signature help** — parameter list with active parameter tracking
- **Test file support** — `*_test.scampi` files get `test.*` builtins

## Installation

If you installed scampi via `get.scampi.dev`, scampls is already on your
PATH. To install just the LSP:

```bash
curl get.scampi.dev/lsp | sh
```

Or via Go:

```bash
go install scampi.dev/scampi/cmd/scampls@latest
```

For development against the source tree, use `go run` directly — it
recompiles on every editor restart so you always get the latest:

```text
go run ./cmd/scampls
```

## Editor setup

### Neovim (LazyVim)

Add to your LazyVim plugin config (e.g. `lua/plugins/scampi.lua`):

```lua
return {
  {
    "nvim-treesitter/nvim-treesitter",
    opts = function(_, opts)
      vim.filetype.add({ extension = { scampi = "scampi" } })
      vim.treesitter.language.register("python", "scampi")
    end,
  },
  {
    "neovim/nvim-lspconfig",
    opts = {
      servers = {
        scampi_lsp = {
          cmd = { "go", "run", "<path-to-scampi>/cmd/scampls" },
          filetypes = { "scampi" },
          root_dir = function(bufnr, cb)
            local fname = vim.api.nvim_buf_get_name(bufnr)
            local root = require("lspconfig.util").root_pattern(
              "scampi.mod"
            )(fname)
            cb(root or vim.fn.fnamemodify(fname, ":h"))
          end,
        },
      },
    },
  },
}
```

Replace `<path-to-scampi>` with the absolute path to your scampi checkout.

### Other editors

Any editor with LSP support can use scampls. Configure it as a stdio language
server with the command `scampls` (or `go run ./cmd/scampls` for development).
The server communicates over stdin/stdout using the standard LSP JSON-RPC
transport.

## Debugging

Pass `--log` to write debug output to a file:

```text
scampls --log /tmp/scampls.log
```

Or in your editor config:

```lua
cmd = { "go", "run", "<path>/cmd/scampls", "--log", "/tmp/scampls.log" },
```

The log shows every request/response: initialize, didOpen, didChange,
completion, hover, signatureHelp, and publishDiagnostics with full detail.
