# Developer environments

This directory contains developer environment configurations for working on this repository.

The contents reflect **personal preferences**: editor setups, debugger configurations, terminal layouts, and similar workflow choices. These are not standards, recommendations, or requirements — they are concrete examples of how individual developers choose to work with the codebase.

Nothing under this directory is required to build, test, or contribute to the project.

## Structure

Environment configurations are grouped **by tool**, not by person.

```
.dev/
├─ nvim/
├─ tmux/
├─ vscode/
├─ scripts/
├─ …/
└─ README.md
```

This makes it easy to browse and compare different approaches to the same tool without having to navigate through individual developer directories.

The `scripts/` directory may contain small, developer-specific helper scripts related to local environment setup (for example, creating symlinks). These scripts are convenience tooling only and do not impose requirements on other contributors.

## Naming

Files are named **by author**, typically using initials, to make ownership and intent explicit.

Example structure:

```
.dev/
├─ nvim/
│  ├─ lazy-pskry.lua
│  └─ dap-alice.lua
├─ vscode/
│  └─ launch-bob.yml
└─ tmux/
   ├─ tmuxinator-pskry.yml
   └─ tmuxinator-alice.yml
```

This signals that:

* the configuration reflects one person’s workflow
* it is not a shared or canonical setup
* multiple parallel configurations are expected

Unnamed or generic files (for example `default.lua` or `config.yml`) are intentionally avoided to prevent the appearance of an "officially supported" environment.

## Usage

Developers who wish to use one of these configurations may copy or symlink it locally as appropriate for their setup.

For example, creating symlinks manually:

```
ln -s .dev/tmux/tmuxinator-pskry.yml .tmuxinator.yml
ln -s .dev/nvim/lazy-pskry.lua .lazy.lua
```

Alternatively, a developer may provide a **personal init script** under `.dev/scripts/` that performs this setup (for example, creating or updating local symlinks). Such scripts are:

* developer-specific
* explicitly opt-in
* safe to ignore

Running an init script is a convenience, not a requirement.

Local files or symlinks created in the project root (such as `.tmuxinator.yml` or `.lazy.lua`) shall not be committed to the repository.

## Adding new environments

Contributors are welcome to add their own environment configurations under `.dev/` provided they:

* follow the existing tool-based directory structure
* use a name that clearly identifies the author
* avoid introducing assumptions or requirements for other contributors

The goal is to share knowledge and working examples without imposing workflow choices on others.
