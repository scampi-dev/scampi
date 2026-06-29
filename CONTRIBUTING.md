# Contributing to scampi

Hi, and thanks for considering a contribution to scampi!

scampi is a small, solo-developed project. PRs and issues are very
welcome, but a few notes will save us both some friction.

## Filing issues

Pick an issue template that matches what you're filing:

- **Bug** — something is broken or behaves wrong
- **Feature** — you want scampi to do something it currently doesn't

If neither fits (a question, a vague idea, a "should we?"), file a
plain issue and we'll figure out the right shape together.

For **security issues**, please don't file a public issue —
[`SECURITY.md`](./SECURITY.md) has the disclosure path.

## Pull requests

A PR template will pre-fill a few sections (summary, linked issue,
test plan, checklist). Fill them in honestly; "test plan: not sure how
to test this" is a fine answer that opens a conversation.

A few load-bearing expectations:

- **Link an issue.** New PRs without an issue tend to stall while
  we figure out whether the change is wanted. Open the issue first if
  there isn't one.
- **One concern per PR.** Combined "fix bug + refactor module + tweak
  unrelated lint" PRs are hard to review and harder to revert.
- **Tests when reasonable.** Behavior changes usually want an
  integration test under `test/`. Pure-function tweaks usually want a
  small unit test next to them. "I didn't add tests because X" is also
  a reasonable answer if X is honest.

## Building and testing

You'll need Go (the version in `go.mod` or newer) and [`just`](https://github.com/casey/just)
on your `PATH`. The repo is a single Go module — `go build ./...`
works, but `just` recipes do the right thing without you having to
remember flags.

```bash
just build       # build scampi and scampls binaries to ./build/bin/
just test all    # fast tests, no containers, no race detector
just lint        # golangci-lint
just fmt         # format Go + markdown tables
```

Three test gates, in order of fanciness:

```bash
just test all            # inner loop, run after every meaningful change
just test nocontainers   # pre-commit gate — race + integration + bench smoke
just test everything     # pre-push gate — adds container-gated suites
```

CI runs `everything`. Hitting `nocontainers` locally before commit and
`everything` before push catches regressions where the feedback loop is
tight rather than waiting for CI to fail.

`just test` (bare) shows the full list of test recipes.

## Code style

The mechanical bits are enforced by `just lint` — that's the source
of truth, no need to memorise rules. A couple of conventions the
linter doesn't catch:

- **Comments earn their place** by explaining *why*, not *what*.
  Don't restate what the code already says.
- **Section banners look like this:**
  ```go
  // Title
  // -----------------------------------------------------------------------------
  ```
- **Naming** — the project's terminology is StepKind, Step, Op,
  Target; the linker emits `Declared*` types (`DeclaredConfig`,
  `DeclaredStep`, …), the engine runs the bare execution nouns.
  Avoid `Impl`, `Handler`, `Spec`, and `Instance`/`Type` suffixes.

If you're unsure whether a change fits the project style, file the
issue first and we'll talk it through before you write code.

## Commit messages

Short, title-only, no body. We use GitHub magic keywords in
parentheses at the end:

```
feat(std): @size attribute for human-readable byte amounts (closes #250)
fix(lang/format): emit attributes on type-decl fields (refs #244)
```

- `(closes #N)` for everything that resolves an issue
- `(fixes #N)` for bug fixes specifically (label `Kind/Bug`)
- `(refs #N)` if the commit relates to an issue but doesn't close it

`<type>` matches conventional commits (`feat`, `fix`, `docs`, `chore`,
`refactor`, `test`, `ci`). Scopes are package-shaped (`(std)`,
`(lang/format)`, `(engine)`).

No commit bodies, no signed-off-by.

## Adding a new step type

If you're adding a new step kind:

1. Create `step/<kind>/<kind>.go` implementing the `spec.StepType`
   interface.
2. Add a config struct with `step` / `summary` / `optional` /
   `default` / `example` field tags.
3. Register in `engine/registry.go`.
4. Cover it with an integration test under `test/` exercising the
   full plan → check → apply pipeline.
5. Update site docs under `site/content/docs/steps/`.

## A note on scope

scampi is opinionated. The feature surface is intentionally narrow —
"batteries included" steps over plugin sprawl, deterministic
fail-fast execution over best-effort orchestration, the project's own
language over YAML/HCL templating.

A well-written PR can still get a "not in scope" response. Examples
of what's likely to bounce:

- **Plugin / extension APIs.** New step kinds belong in the standard
  library, not behind a runtime extension point.
- **Alternative config formats** alongside scampi (YAML, HCL,
  templating layers). One language, one mental model.
- **Best-effort retry / recovery** that masks underlying failures.
  scampi prefers fail-fast with a clear diagnostic over silent
  partial success.
- **Direct ports** of step implementations from other tools without
  rethinking the model in scampi terms.

If you're unsure, **file the issue first.** A short discussion before
you spend time coding is the cheapest possible alignment.

## Thank you

File the issue, send the PR, ask the question. The worst that can
happen is "no, but here's why," and even that's useful information
for both of us.

---

For questions that don't fit issues, my email is on
[my GitHub profile](https://github.com/pskry).
