# CLAUDE.md

Guidance for Claude Code when working in this repository.

## What is scampi?

A decentralized reconciler for bare-metal infrastructure. Statically-linked
binary, runs as peers in a gossip mesh, reconciles declared desired state
against live observation of reality. The architectural spec lives in
`doc/design/2026-05-28-decentralized-reconciler.md`; treat it as a
compass, not a contract.

## Project status

Solo developer project.
No userbase.
No backwards-compatibility requirements.
Clean breaks are fine.

## Tone

- Talk like a coworker, not a consultant. Banter is welcome.
- No corporate-speak, no "I'd be happy to", no "Great question!".
- Be direct and casual. Swear if the moment calls for it.
- If you screw up, just own it. No preamble apology essays.

## Code style

- ASCII-only everywhere. No em-dashes, en-dashes, ellipses, smart
  quotes, fancy arrows. Substitutions:
  - Em-dash / en-dash: restructure to normal English (period, comma,
    parens, rephrase). If a separator is genuinely needed, single
    ASCII `-`. **Never use `--`.** Humans don't write that many
    dashes in flowtext.
  - Ellipsis: `...`
  - Smart quotes: ASCII `"` `'`
  - Arrows: `->`, `<-`, `=>`, or written word.
- Section comments use the banner style:
  ```
  // Title
  // -----------------------------------------------------------------------------
  ```
- Markdown tables must have aligned columns. Pad cells so pipes line
  up. Applies to all `.md` in this repo.
- No transient labels like "new shape", "legacy", "during migration"
  in code or comments. Future-me asks "where's the old one?" and the
  answer is nothing.

### Comments

Audience: technical professional. No yapping.

- **Why, not what.** A comment that restates the declaration is
  noise. Comments earn their place by capturing WHY: hidden
  invariants, non-obvious trade-offs, traps a reader will hit,
  reasons something exists.
- **Terse.** State the operation; if a WHY is needed, one short
  sentence. Stop. "With prepends fields to Event.Fields" beats
  "With returns a Diag whose every emitted Event has fields
  prepended to Event.Fields".
- **No tautology / no "satisfies X".** "Foo is the foo type",
  "Write satisfies Sink", "New returns a new Bar" all delete.
- **No usage guides.** Don't write "Use when ..." on a definition.
  Usage belongs at the call site; on the definition it rots when
  callers change.
- **No call-site references.** Describe what the thing IS, not who
  uses it. "the engine calls this each tick", "for the action log
  to stay small", "used by SlogSink to ..." all leak coupling and
  rot.
- **Use industry terms directly.** "JSONL" not "newline-delimited
  JSON". "topo-sort" not "topological ordering". "mutex" not "a
  lock to prevent concurrent writes". The paraphrase isn't kinder,
  it's noisier.

## Git

- **Never add `Co-Authored-By` lines to commits.** Just don't.
- Commit messages are short, title-only. No body.
- When a commit resolves an issue, use GitHub magic keywords in
  parentheses at the end of the title:
  - `feat: add foo (fixes #N)` for bugs (`kind/bug`)
  - `feat: add foo (closes #N)` for everything else
- Amending is fine for unpushed commits when the change clearly belongs
  with the original. Never amend pushed commits.
- Never chain git commands with `&&`. Run `git add` and `git commit` as
  separate tool calls.
- Never use destructive git commands (`reset --hard`, `push --force`,
  `branch -D`, `clean -f`) without explicit ask. User does history
  rewrites in lazygit.

## Issue tracking

Issues live on GitHub at `scampi-dev/scampi`. Use the `gh` CLI.

Before starting work on an issue, assign it:
`gh issue edit N --add-assignee pskry`. Do this before planning, not at
commit time.

Labels: stick to GitHub's defaults (`bug`, `documentation`,
`duplicate`, `enhancement`, `good first issue`, `help wanted`,
`invalid`, `question`, `wontfix`). Add a custom label only when
filtering by it would actually inform a decision; solo project
without users hasn't earned a richer taxonomy.

When to file: if the change is worth showing up in the changelog
(user-facing features, bugs that affect behavior, design decisions
worth explaining), file one. Tiny internal fixes just commit.

To create from a session, write a markdown file to `.issues/` and ask the
user to push it via `gh issue create --body-file .issues/foo.md`. Don't
run the create command yourself.

## Hosting

- **Primary**: GitHub `scampi-dev/scampi`. Issues, PRs, releases.
- **Mirror**: Codeberg `scampi-dev/scampi`. Read-only mirror.
- **Infra**: `scampi-dev/scampi-infra`. VPS / homelab configs (separate).

## Repo quirks

- `.claude/` is gitignored. Settings and scratch files live there.
- `.issues/` is gitignored. Issue files are temp artifacts for upload.
- `.sandbox/` is gitignored. Repro / scratch files live there.
  Never bash heredoc to `/tmp/`; always `Write` into `.sandbox/`.

## Go code navigation

Use `gopls` MCP tools for Go symbol search and navigation:
`go_search`, `go_file_context`, `go_workspace`, `go_package_api`.
Don't grep around for type definitions, function signatures, or
references when gopls can answer directly.
