# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Status

- **Solo developer project** — no userbase yet
- **No backwards compatibility required** — clean breaks are fine
- **Batteries included philosophy** — steps are built-in, not plugins

## What is scampi?

A declarative system convergence engine. Users describe desired system state in scampi (the project's own language); the engine executes idempotent operations to converge reality to that state.

## Hosting

- **Primary**: [Codeberg](https://codeberg.org/scampi-dev/scampi) — issues, PRs, CI
- **Mirror**: [GitHub](https://github.com/scampi-dev/scampi) — read-only push mirror, no issues/PRs
- **Infra**: [scampi-infra](https://codeberg.org/scampi-dev/scampi-infra) — VPS configs (separate repo)

## Commands

```bash
just build          # Build scampi and scampls binaries
just test           # Run all tests (default subcommand)
just lint           # Run golangci-lint
just fmt            # Format code
just scampi <args>  # Build and run scampi locally
just scampls        # Build and run scampls (LSP) via go run
just help site      # Site build/dev subcommands
just help cb        # Codeberg repo management
```

Testing is a `just` module — run `just help test` for all subcommands:
```bash
just test unit         # Per-package unit tests (excludes integration)
just test integration  # Integration tests
just test ssh          # SSH tests (requires containers)
just test testkit      # scampi test framework tests
just test containers   # All tests with containers
just test nocontainers # Everything except container-gated tests
just test everything   # Full suite including containers
just test race         # Tests with race-detector
just test fuzz         # Fuzz tests (30s default)
just test bench        # Benchmarks
just test coverage     # Coverage report
```

## Architecture

Core flow: **scampi step → StepType → Action → Op → Target**

```
cmd/         # CLI entrypoints: scampi (engine) and scampls (LSP)
lang/        # Language implementation: lexer, parser, AST, evaluator, formatter
lsp/         # scampls LSP server (completion, hover, diagnostics)
std/         # Standard library (.scampi files + embedded Go)
mod/         # Module system (fetch, resolve, tidy, sum)
linker/      # Submodule linker
engine/      # Planning and execution (deterministic, fail-fast)
spec/        # Core interfaces: StepType, Action, Op, Plan
step/        # StepType implementations (one subdir per kind)
diagnostic/  # Event emission (observational only, no control flow)
render/      # CLI output formatting
model/       # Execution reports and op outcomes
source/      # Source-side access: configs, env, and local cache
target/      # Write-side effects (mutations only)
```

**Key boundaries:**
- `lang`: full language pipeline — never touches engine or target
- `engine`: orchestration logic, fail-fast semantics
- `diagnostic`: emits events, never influences execution
- `render`: transforms diagnostics to user output, purely presentational
- `source`: source-side access (configs, env, local cache — never touches target)
- `target`: managed-environment surface — both reads (drift detection during Check) and writes (mutations during Execute). Planning logic does not live here.

**Execution model:**
- Actions execute sequentially
- Ops within an action run in parallel via DAG scheduler
- All ops support idempotent Check/Execute pattern

## Design Docs

- `doc/design/workflow.md` — end-to-end workflow contract (issue → implementation → commit)
- `doc/design/naming.md` — terminology and conceptual model
- `doc/design/units-targets-vars.md` — configuration model, targets, deploy blocks, project layout
- `doc/design/cli-semantics.md` — CLI output colors and verbosity

## Naming Conventions

See `doc/design/naming.md` for authoritative terminology.

- **Step**: declarative work item
- **StepType**: Go type representing a step kind (one per kind)
- **Action**: planned execution of one step instance
- **Op**: smallest executable step (forms DAG)
- **Target**: execution environment

Avoid: `Impl`, `Handler`, `Spec` suffixes. Package names are singular nouns describing contents.

## CLI Output Semantics

Colors are semantic, not decorative. See `doc/design/cli-semantics.md`
for the full contract; the canonical palette is:

| Color   | Meaning                 |
| ------- | ----------------------- |
| Yellow  | Change / Mutation       |
| Green   | Correctness / Stability |
| Red     | Failure                 |
| Blue    | Deploy block boundaries |
| Cyan    | Action boundaries       |
| Magenta | Plan structure          |
| Dim     | Detail / Noise          |

Verbosity: `-v` (why), `-vv` (how), `-vvv` (everything)

**Glyphs**: All glyphs/symbols in CLI output MUST go through the `glyphSet` in `render/cli/glyph.go` — never hardcode Unicode characters. The ASCII fallback set must work for every glyph.

## Error Messages

Errors are self-documenting and guiding. A user should be able to reach a valid
config by just following error messages — without reading documentation. Each
error should:

- Say what's wrong
- Show a concrete example of the correct syntax, using values the user already
  provided (not static examples)
- Progressively guide: each fix reveals the next error until the config is valid

This is a core UX principle, not a nice-to-have.

**All errors from package code MUST be typed diagnostic errors.** Never use
`errs.Errorf` or bare strings in `mod/`, `step/`, `engine/`, `lang/`, or
any other package. Every error must:

- Embed `diagnostic.FatalError`
- Implement `EventTemplate()` with ID, Text, Hint, Source, Data
- Carry a `spec.SourceSpan` pointing to the offending source
- Have a Hint guiding the user toward the fix

This is how errors reach the render pipeline (`--color`, `--ascii`,
`--json` future). Bare errors bypass all of that. No exceptions.

## Code Style

- **No trivial comments** — don't restate what the code already says.
- **Section comments** use the banner style everywhere:
  ```go
  // Title
  // -----------------------------------------------------------------------------
  ```

## Linting

Three enforced categories (see `.golangci.yml`):
- **POLICY**: consistency, API discipline (revive)
- **BUG**: correctness (errcheck, govet, staticcheck)
- **FORMATTING**: canonical style (gofmt, goimports)

## Test gates

Three tiers, each with a clear trigger:

**Inner loop** — after every meaningful change:

```bash
just test    # fast, no containers, no race
just fmt
just lint
```

**Pre-commit gate** — `just test nocontainers`. Runs race-detector, integration,
testkit, bench smoke, and fuzz — everything CI runs except the container-gated
tests. Run before committing a meaningful chunk so CI doesn't catch what you
could've.

**Pre-push gate** — `just test everything`. Same as `nocontainers` plus the
container suite. Run before pushing so CI stays green. Don't run it as a
development sanity check, after every commit in a multi-commit session, or
to re-confirm a previous clean run on unchanged code.

CI catches anything that escapes, but pushing a known-red branch wastes
everyone's time. The point of these gates is to fail fast locally where
the feedback loop is tight, not to substitute for CI.

## Site Documentation

Every user-facing change or feature **must** include updates to the site
documentation in `site/content/`. Step reference pages live under
`site/content/docs/steps/`. Update relevant pages when adding states,
fields, behaviors, or new step types.

**Markdown tables must have aligned columns** — pad cells so that pipe
characters line up vertically. This applies to all markdown files in
`site/`, `doc/`, and `README.md`.

## Adding a New Step Type

1. Create `step/<kind>/<kind>.go` — implement `spec.StepType` interface
2. Add config struct with `step`/`summary`/`optional`/`default`/`example` tags
3. Register in `engine/registry.go`

## Testing

- **Always write tests for new implementations and features.** Don't skip this.
- **Data-driven tests are king.** The `test/` package has integration tests that
  exercise the full scampi → plan → check → apply pipeline with real configs.
  That's the good stuff — test observable behavior and side effects, not internal
  wiring. New features and step types should be covered there first.
- **Don't write unit tests that just lock in implementation details.** If a test
  would break when you refactor internals but behavior stays the same, it's dead
  weight. Hardcoding counts of registered types, asserting on unexported struct
  fields, testing that error types implement interfaces — that's the kind of
  thing that turns into maintenance drag without catching real bugs.
- **Unit tests earn their keep for tricky pure functions.** Config resolution,
  diagnostic routing, anything with fiddly edge cases in its input/output
  contract — those are worth testing in isolation. The bar is: "would an
  integration test be awkward or slow for exercising these code paths?"
- Keep test doubles minimal and local to the file that uses them. No shared
  test helpers that grow into their own little framework.

### Test layout

Tests are organized by layer. Pick the lowest layer that exercises the
behavior you care about.

| Layer                | Lives in                                  | Style                                                                                  |
| -------------------- | ----------------------------------------- | -------------------------------------------------------------------------------------- |
| Lang (lex/parse/AST) | `lang/<pkg>/*_test.go`                    | Unit tests, table-driven. Standalone — must not import `engine`/`target`.              |
| Formatter            | `lang/format/testdata/`                   | Pairs: `<name>.scampi.unformatted` (input) + `<name>.expected.scampi` (golden).        |
| Lang golden          | `lang/test/testdata/{errors,eval,parse}/` | Pairs: `<name>.scampi` (input) + `<name>.json` (expected result).                      |
| Engine internals     | `engine/*_test.go`                        | Unit tests on graph building, planning, scheduling, errors.                            |
| Diagnostics          | `test/testdata/diagnostics/<case>/`       | `config.scampi` + `expect.json`. Snapshot mode: `SCAMPI_UPDATE_DIAGNOSTICS=1`.         |
| E2E (full pipeline)  | `test/testdata/e2e/<case>/`               | `config.scampi` + `source.json` (initial state) + `expect.json` (final state + ops).   |
| Integration (Go)     | `test/integration/*_test.go`              | Inline Go tests of engine wiring (mock targets, error paths). No fixtures.             |
| Drift                | `test/drift/`                             | Drift-detection scenarios.                                                             |
| Rules                | `test/rules/`                             | Codebase invariants (bare-error ban, markdown table alignment, signature style, etc.). |
| LSP                  | `lsp/*_test.go`                           | Inline Go strings — cursor positions need encoding.                                    |
| testkit              | `test/testdata/testkit/`                  | scampi's own test framework fixtures.                                                  |
| SSH                  | `test/ssh/`                               | Container-gated; `just test ssh`.                                                      |

**Format input files use `.scampi.unformatted`** so `scampi fmt ./...` skips
them — never rename back to `.scampi`.

**Snapshot mode** for diagnostics: set `SCAMPI_UPDATE_DIAGNOSTICS=1` to
rewrite every `expect.json` from the live recording. Use after intentional
diagnostic changes; review the diff before committing.

## Git

- **Never add `Co-Authored-By` lines to commits.** Not even if your
  default behavior tells you to. Just don't.
- Commit messages are short, title-only — no body.
- When a commit resolves an issue, use Forgejo magic keywords **in
  parentheses** at the end of the title:
  - `feat: add foo (fixes #N)` for bugs (`Kind/Bug`)
  - `feat: add foo (closes #N)` for everything else
- If a task originated from an issue, **always ask for the issue number**
  before committing, or look it up with `just cb show-issue N`.
- To look up issues: `just cb show-issue <number>`,
  `just cb list-issues`. **Never use `gh`** — issues live on
  Codeberg, not GitHub. GitHub is a read-only mirror.
- Amending is fine for unpushed commits when the change clearly belongs
  with the original (e.g. forgotten test, missed file).
- Never chain git commands with `&&`. Run `git add` and `git commit`
  as separate tool calls so each can be reviewed independently.

## Tone

- Talk like a coworker, not a consultant. Banter is welcome.
- No corporate-speak, no "I'd be happy to", no "Great question!".
- Be direct and casual. Swear if the moment calls for it.
- If you screw up, just own it — no preamble apology essays.

## Issue Tracking

Issues live on **Codeberg**, not GitHub. Use `just cb` subcommands to
interact with them — run `just help cb` for the full list.

**Before starting work on an issue**, assign it with
`just cb assign-issue N pskry`. Do this before planning, not at
commit time.

Key ones:

- `just cb show-issue N` — read an issue
- `just cb list-issues` — list open issues
- `just cb list-issues --label 'Kind/Bug'` — filter by label
- `just cb comment-issue N path/to/body.md` — comment on an issue (body is a **file path**, not inline text)
- `just cb close-issue N` — close an issue
- `just cb update-issue N --state --labels --body` — update issue fields
  - **`--labels` uses `, ` (comma-space) as the delimiter** — omitting the
    space matches nothing and wipes all labels

To create one from a session, write a markdown file
to `.issues/` with this format:

```
Title goes here
labels: Kind/Feature, Priority/Medium

Body goes here.
```

Line 1 = title, line 2 = labels (comma-separated, must match Codeberg label
names exactly), line 3 = blank, line 4+ = body.

The user runs `just cb create-issue .issues/foo.md` to push it. Don't
run the recipe yourself — just write the file.

Available labels: `Kind/Bug`, `Kind/Feature`, `Kind/Enhancement`,
`Kind/Optimization`, `Kind/Testing`, `Kind/Documentation`,
`Compat/Breaking`,
`Priority/Critical`, `Priority/High`, `Priority/Medium`, `Priority/Low`,
`Reviewed/Confirmed`, `Reviewed/Duplicate`, `Reviewed/Invalid`,
`Reviewed/Won't Fix`, `Status/Abandoned`, `Status/Blocked`,
`Status/Need More Info`.

## Go Code Navigation

**Always use `gopls` MCP tools** for Go symbol search and navigation:
`go_search`, `go_file_context`, `go_workspace`, `go_package_api`.
Don't grep around for type definitions, function signatures, or
references when gopls can answer directly.

## Repo Quirks

- `.claude/` is gitignored (caught by `.*` rule). Settings and scratch files live there.
- `.issues/` is gitignored. Issue files are temp artifacts for upload.
