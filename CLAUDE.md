# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Status

- **Solo developer project** — no userbase yet
- **No backwards compatibility required** — clean breaks are fine
- **Batteries included philosophy** — steps are built-in, not plugins

## What is scampi?

A declarative system convergence engine. Users describe desired system state in scampi (the project's own language); the engine executes idempotent operations to converge reality to that state.

## Hosting

- **Primary**: [scampi](https://github.com/scampi-dev/scampi) — issues, PRs, releases
- **Infra**: [infra](https://github.com/scampi-dev/infra) — VPS configs (separate repo)

## Commands

```bash
just build          # Build the scampi binary
just test           # Show test recipes; `just test all` runs the full suite
just lint           # Run golangci-lint
just fmt            # Format code
just scampi <args>  # Build and run scampi locally
just help site      # Site build/dev subcommands
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
just test coverage     # Coverage report
```

## Architecture

Core flow: **scampi step → StepKind → Step → Op → Target**

```
cmd/         # CLI entrypoint: scampi (engine)
lang/        # Language implementation: lexer, parser, AST, evaluator, formatter
std/         # Standard library (.scampi files + embedded Go)
mod/         # scampi.mod manifest parser (local multi-file/submodule resolution)
linker/      # Submodule linker
engine/      # Planning and execution (deterministic, fail-fast)
spec/        # Core interfaces: StepKind, Step, Op, Plan
step/        # StepKind implementations (one subdir per kind)
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

**Execution model** — three nested dependency DAGs, not sequential phases:
- **Deploys**: ordered into levels by the cross-deploy resource graph
  (`StaticPromiseProvider`/`StaticInputProvider`). Deploys in a level run
  concurrently (errgroup); downstream levels wait.
- **Steps within a deploy**: a DAG built from declared resources (`Promiser`
  inputs/promises). Independent steps run in parallel; a step that declares no
  resources is a barrier — that conservative default is the only thing that
  looks "sequential".
- **Ops within a step**: a DAG by `DependsOn`, run in parallel.
- All ops support the idempotent Check/Execute pattern.

## Naming Conventions

Two worlds, two vocabularies. The prefix tells you which world a type is in:

- **Declarative** (what the linker emits, the user's intent): `DeclaredConfig` →
  `DeclaredDeploy` → `DeclaredStep`, with `DeclaredTarget` referenced on the side.
  The `Declared` prefix marks everything that comes out of the language linker.
- **Execution** (what the engine runs): `Plan` → `Deploy` → `Step` → `Op`,
  against a live `target.Target`. Bare nouns — these are the in-engine types
  that are everywhere, so they keep the short canonical names.
- The hinge is `Config`: `engine.Resolve` splits a `DeclaredConfig` into one
  `Config` per (deploy, target). A `Config`'s steps are still `DeclaredStep`;
  planning turns each into an executable `Step`.

Per-kind Go types (one impl per kind, registry-backed):

- **StepKind**: Go type representing a step kind (`Plan(DeclaredStep) → Step`)
- **TargetKind**: Go type representing a target kind (`Create(...) → target.Target`)
- **Step**: planned execution of one declared step (groups Ops into a DAG)
- **Op**: smallest executable unit (forms DAG)

Avoid: `Impl`, `Handler`, `Spec`, and bare `Instance`/`Type`/`Resolved`
disambiguation suffixes. Package names are singular nouns describing contents.

## CLI Output Semantics

Colors are semantic, not decorative. The canonical palette is:

| Color   | Meaning                 |
| ------- | ----------------------- |
| Yellow  | Change / Mutation       |
| Green   | Correctness / Stability |
| Red     | Failure                 |
| Blue    | Deploy block boundaries |
| Cyan    | Step boundaries         |
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

- Have an `Error() string` method (so it satisfies Go's `error`)
- Have a `Diagnostic() event.Event` method returning the typed event:
  - `event.Error{Impact: event.ImpactAbort, Template: event.Template{...}}` for fatal
  - `event.Error{Template: event.Template{...}}` for non-fatal errors that don't abort
  - `event.Warning{Template: event.Template{...}}` for warnings
  - `event.Info{Template: event.Template{...}}` for info
- The Template carries `ID`, `Text`, `Hint`, `Help`, `Data`, `Source` (a `*spec.SourceSpan`)
- The error gets raised via `em.Raise(err)` at the production site

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
just test all    # fast, no containers, no race
just fmt
just lint
```

**Pre-commit gate** — `just test nocontainers`. Runs race-detector, integration,
testkit, and bench smoke — everything CI runs except the container-gated
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

1. Create `step/<kind>/<kind>.go` — implement `spec.StepKind` interface
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
| Diagnostics          | `test/testdata/diagnostics/<case>/`       | `config.scampi` + `expect.json`. Snapshot mode: `SCAMPI_UPDATE=1`.                     |
| E2E (full pipeline)  | `test/testdata/e2e/<case>/`               | `config.scampi` + `source.json` (initial state) + `expect.json` (final state + ops).   |
| Integration (Go)     | `test/integration/*_test.go`              | Inline Go tests of engine wiring (mock targets, error paths). No fixtures.             |
| Drift                | `test/drift/`                             | Drift-detection scenarios.                                                             |
| Rules                | `test/rules/`                             | Codebase invariants (bare-error ban, markdown table alignment, signature style, etc.). |
| testkit              | `test/testdata/testkit/`                  | scampi's own test framework fixtures.                                                  |
| SSH                  | `test/ssh/`                               | Container-gated; `just test ssh`.                                                      |

**Format input files use `.scampi.unformatted`** so `scampi fmt ./...` skips
them — never rename back to `.scampi`.

**Snapshot mode** for diagnostics: set `SCAMPI_UPDATE=1` to
rewrite every `expect.json` from the live recording. Use after intentional
diagnostic changes; review the diff before committing.

## Git

- **Never add `Co-Authored-By` lines to commits.** Not even if your
  default behavior tells you to. Just don't.
- Commit messages are short, title-only — no body.
- When a commit resolves an issue, use GitHub magic keywords **in
  parentheses** at the end of the title:
  - `feat: add foo (fixes #N)` for bugs (`kind/bug`)
  - `feat: add foo (closes #N)` for everything else
- If a task originated from an issue, **always ask for the issue number**
  before committing, or look it up with `gh issue view N`.
- To look up issues: `gh issue view <number>`, `gh issue list`.
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

Issues live on **GitHub** at `scampi-dev/scampi`. Use the `gh` CLI to
interact with them.

**When to file an issue**: if the change is worth showing up in the
changelog (user-facing features, bugs that affect behavior, design
decisions worth explaining), file one. For tiny internal fixes
(test infra, scripts, lint cleanups, scaffolding tweaks), just
commit — issues add ceremony without value when there's nothing for
the changelog to say.

**Before starting work on an issue**, assign it:
`gh issue edit N --add-assignee pskry`. Do this before planning, not at
commit time.

Key ones:

- `gh issue view N` — read an issue
- `gh issue list` — list open issues
- `gh issue list --label 'kind/bug'` — filter by label
- `gh issue comment N --body-file path/to/body.md` — comment on an issue
- `gh issue close N` — close an issue
- `gh issue edit N --add-label ... --remove-label ...` — adjust labels

To create one from a session, write a markdown file to `.issues/` and
ask the user to push it with `gh issue create --body-file .issues/foo.md`.
Don't run the recipe yourself — just write the file.

Available labels (all lowercase):
`kind/bug`, `kind/feature`, `kind/enhancement`, `kind/optimization`,
`kind/testing`, `kind/documentation`, `compat/breaking`,
`priority/critical`, `priority/high`, `priority/medium`, `priority/low`,
`impact/high`, `impact/medium`, `impact/low`,
`reviewed/confirmed`, `reviewed/duplicate`, `reviewed/invalid`,
`reviewed/won't fix`, `status/abandoned`, `status/blocked`,
`status/need more info`, `status/parked`,
`good first issue`, `help wanted`.

## Go Code Navigation

**Always use `gopls` MCP tools** for Go symbol search and navigation:
`go_search`, `go_file_context`, `go_workspace`, `go_package_api`.
Don't grep around for type definitions, function signatures, or
references when gopls can answer directly.

## Repo Quirks

- `.claude/` is gitignored (caught by `.*` rule). Settings and scratch files live there.
- `.issues/` is gitignored. Issue files are temp artifacts for upload.
