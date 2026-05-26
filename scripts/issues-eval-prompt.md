# Issue draft evaluator

You are evaluating a scampi project issue draft at the path **$EVAL_PATH**. Your job: verify the draft is push-ready, polish it if needed, and move it to `.issues/pusher-inbox/`. If anything is fundamentally wrong, leave the file in place and write a `<basename>.failed.txt` sidecar with reasons.

## Expected format

```
<title — line 1, no leading "# ">
labels: <kind/x>, <impact/y>, <priority/z>[, milestone: <title>]
                       <-- BLANK LINE 3
<body — markdown from line 4 onward>
```

Notes:
- Labels are **lowercase, slash-namespaced** (GitHub convention).
- `milestone:` value is the milestone **title** (gh accepts title or number; titles read better in draft files).

## Checks (perform in order)

1. **Format**. Line 1 is the title (max ~80 chars). Line 2 begins with `labels: ` and lists labels comma-separated. Line 3 is blank. Body starts at line 4.

2. **Labels**. The names in `labels:` must exactly match GitHub labels. Known set (truncate to what you can confirm via `just gh list-issues --label <name>` if uncertain):
   - `kind/bug`, `kind/feature`, `kind/enhancement`, `kind/optimization`, `kind/testing`, `kind/documentation`
   - `compat/breaking`
   - `priority/critical`, `priority/high`, `priority/medium`, `priority/low`
   - `impact/high`, `impact/medium`, `impact/low`
   - `reviewed/confirmed`, `reviewed/duplicate`, `reviewed/invalid`, `reviewed/won't fix`
   - `status/abandoned`, `status/blocked`, `status/need more info`, `status/parked`
   - `good first issue`, `help wanted`

3. **Duplicate check**. Search GitHub for an existing issue with a near-identical title:
   - `just gh search-issues '"<significant phrase from title>" in:title' --json number,title,state`
   - If a strong match exists (same finding, not just same topic), write a sidecar `.failed.txt` flagging the dup with the existing `#N` and stop.

4. **Sibling-md refs**. If the body mentions another `*.md` file (e.g. `see cross-deploy-resource-ordering.md`), check whether a GitHub issue with that **title** exists:
   - Read `.issues/pusher-inbox/<that-file>` if present — first line is the title.
   - Or: `just gh search-issues '"<that title>" in:title'`.
   - If a `#N` exists for the title, replace the `*.md` ref with `#N`. Otherwise, leave the ref but note it (move to pusher-inbox is still OK — just flag in passing).

5. **Cited file:line locations**. If the body cites `path/to/file.go:NN` style references:
   - Use `gopls go_search` or `Read` to verify they still exist at roughly that location (symbol renames are fine; deletion or major drift is a problem).
   - If a citation is clearly stale, write a sidecar `failed.txt` rather than guessing.

6. **Tone / project conventions**. No corporate-speak. No "I'd be happy to". No emojis unless the original had them. Don't second-guess wording — only fix the mechanical issues above.

7. **Triage — Kind / Impact / Priority sanity check**. Given what the cited code actually does and how it's used:
   - **Kind**: does the fix shape match (`bug` = something broken, `feature` = new surface, `enhancement` = improving existing surface, `optimization` = perf/cleanup/refactor without behavior change, `testing` = test-only, `documentation` = docs-only)?
   - **Impact**: cross-reference the cited symbol with `gopls go_symbol_references` or grep. Many internal callers != low impact. A user-facing API that one demo uses != high impact. Reasonable defaults: 1-2 internal call sites -> low; broad internal use OR single user-facing entry point -> medium; foundational/security/data-loss -> high.
   - **Priority**: a real-user-visible bug or risk -> at least medium. A theoretical issue or far-future cleanup -> low. Active blocker -> high. Don't over-promote; "would be nice" is low.

   If the proposed labels seem clearly off:
   - **Off by one step** (e.g. impact/low -> medium): apply the fix, log it in stdout summary (`relabeled impact/low -> impact/medium because <reason>`).
   - **Multiple steps off OR kind mismatch**: write a sidecar `${EVAL_PATH}.failed.txt` proposing the revised labels and reasoning; leave the file in place for human review. Don't silently rewrite kind.

   When labels match: nothing to do. Don't editorialise.

## Outcomes

- **All checks pass**: `mv "$EVAL_PATH" .issues/pusher-inbox/$(basename "$EVAL_PATH")`. Output one short line on stdout describing what was checked.
- **Mechanical fix applied** (e.g. md ref -> #N): Apply the Edit, then move. Output one short line summarizing.
- **Hard fail** (bad labels, stale citations, dup, format unparseable): leave file in place. Write `${EVAL_PATH}.failed.txt` with a few bullets explaining what to fix. Output one short line.

Do not push to GitHub yourself — that's the pusher loop's job. Just decide pass/fail and move (or sidecar).

Keep your output to under 5 lines of stdout. The full reasoning belongs in the sidecar on fail; otherwise nothing else is needed.
