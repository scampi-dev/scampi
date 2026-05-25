% SCAMPI-MOD 1 "" scampi

# NAME

scampi-mod - manage scampi module dependencies

# SYNOPSIS

**scampi mod init** \[*module-path*\]

**scampi mod tidy**

**scampi mod add** *module*\[**@***version*\]

**scampi mod add** *name* *local-path*

**scampi mod update** *module*

**scampi mod download**

**scampi mod verify**

**scampi mod cache**

**scampi mod clean**

# DESCRIPTION

Manages scampi module dependencies. Modules are reusable scampi
configurations published to Git forges and referenced by **load()**
calls in your configuration files.

Dependencies are tracked in *scampi.mod* (module declarations and
versions) and *scampi.sum* (content hashes for integrity).

# SUBCOMMANDS

## init

Create a *scampi.mod* file in the current directory. The optional
*module-path* sets the module's import path (e.g.
*github.com/user/infra*).

## tidy

Sync the require block in *scampi.mod* with the **load()** calls found
in *.scampi* files. Adds missing dependencies and removes unused ones.

## add

Add a dependency. In the one-argument form, fetches the module from
its Git forge. Append **@***version* to pin a specific version; otherwise
the latest stable version is used.

The two-argument form adds a local path dependency, useful during
development.

## update

Update a dependency to its latest stable version.

## download

Download all dependencies listed in *scampi.mod*, resolve transitive
dependencies, and update *scampi.sum*.

## verify

Verify that cached modules match their checksums in *scampi.sum*.

## cache

Print the module cache directory (typically *~/.cache/scampi/mod*).

## clean

Remove all cached modules.

# EXAMPLES

Initialize a new module:

    $ scampi mod init github.com/myorg/infra

Add a dependency:

    $ scampi mod add github.com/scampi-dev/std

Add a pinned version:

    $ scampi mod add github.com/scampi-dev/std@v0.2.0

Use a local module during development:

    $ scampi mod add mylib ../scampi-mylib

Sync dependencies after editing load() calls:

    $ scampi mod tidy

Download everything and verify integrity:

    $ scampi mod download
    $ scampi mod verify

Update a dependency to the latest version:

    $ scampi mod update github.com/scampi-dev/std

Clean the module cache:

    $ scampi mod clean

# FILES

*scampi.mod*
  Module file in the project root. Lists the module path and
  dependencies with versions.

*scampi.sum*
  Checksum file in the project root. Content hashes for all
  downloaded modules.

# SEE ALSO

**scampi**(1), **scampi-gen**(1)
