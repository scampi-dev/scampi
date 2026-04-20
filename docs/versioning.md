# Versioning

scampi follows [Semantic Versioning 2.0.0](https://semver.org/).

## Version Format

```
v<major>.<minor>.<patch>[-<pre-release>]
```

- **Major** — breaking changes to the public interface
- **Minor** — new features, backwards-compatible
- **Patch** — bug fixes, backwards-compatible

## Public Interface

The public interface covers everything users interact with directly:

- **Step definitions** — field names, types, defaults, and behavior
- **Language API** — builtins, functions, and configuration model
- **CLI commands and flags** — subcommands, arguments, exit codes

Not considered public interface (can change without a major bump):

- CLI output formatting and rendering
- Diagnostic messages and wording
- Internal Go APIs

## Pre-release Tags

Pre-release versions use dotted numeric suffixes:

```
v0.2.0-alpha.1    unstable, incomplete — expect rough edges
v0.2.0-beta.1     feature-complete — testing and feedback
v0.2.0-rc.1       release candidate — believed ready, final validation
v0.2.0            stable
```

Not every stage is required. Skip from alpha straight to rc if the scope is
small. Tags containing a hyphen are marked as pre-releases on the Codeberg
releases page.

## Major Zero

While on `0.x.y`, the public interface is unstable. Anything can change
between minor versions without it being considered a breaking change.

## Determining the Next Version

The bump level is derived from issue labels in the git history since the
last tag. Each commit that closes an issue (via `fixes #N` or `closes #N`)
contributes that issue's labels to the decision. The highest-impact label
wins:

| Label              | Bump  |
| ------------------ | ----- |
| `Compat/Breaking`  | major |
| `Kind/Feature`     | minor |
| `Kind/Enhancement` | minor |
| `Kind/Bug`         | patch |

If no qualifying issues are found, the bump defaults to patch.

Run `just cb next-version` to calculate the next tag automatically.
It inspects the git log since the last tag, queries Codeberg for issue
labels, and prints the next version. Pass `--pre-release alpha` (or
`beta`, `rc`) to append a pre-release suffix with auto-incrementing
number.
