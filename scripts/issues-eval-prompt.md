# Issue draft evaluator

You are evaluating a scampi project issue draft at the path **$EVAL_PATH**. Your job: verify the draft is push-ready, polish it if needed, and move it to `.issues/pusher-inbox/`. If anything is fundamentally wrong, leave the file in place and write a `<basename>.failed.txt` sidecar with reasons.

## Expected format

```
<title — line 1, no leading "# ">
labels: <Kind/X>, <Impact/Y>, <Priority/Z>[, milestone: <id>]
                       <-- BLANK LINE 3
<body — markdown from line 4 onward>
```

## Checks (perform in order)

1. **Format**. Line 1 is the title (max ~80 chars). Line 2 begins with `labels: ` and lists labels comma-separated. Line 3 is blank. Body starts at line 4.

2. **Labels**. The names in `labels:` must exactly match Codeberg labels — case- and slash-sensitive. Known set (truncate to what you can confirm via `just cb` if uncertain):
   - `Kind/Bug`, `Kind/Feature`, `Kind/Enhancement`, `Kind/Optimization`, `Kind/Testing`, `Kind/Documentation`
   - `Impact/Low`, `Impact/Medium`, `Impact/High`
   - `Priority/Low`, `Priority/Medium`, `Priority/High`, `Priority/Critical`
   - `Compat/Breaking`
   - `Reviewed/Confirmed`, `Reviewed/Duplicate`, `Reviewed/Invalid`, `Reviewed/Won't Fix`
   - `Status/Blocked`, `Status/Parked`, `Status/Need More Info`, `Status/Abandoned`

3. **Sibling-md refs**. If the body mentions another `*.md` file (e.g. `see cross-deploy-resource-ordering.md`), check whether a Codeberg issue with that **title** exists:
   - Read `.issues/pusher-inbox/<that-file>` if present — first line is the title.
   - Or: `just cb list-issues` and grep for the title.
   - If a `#N` exists for the title, replace the `*.md` ref with `#N`. Otherwise, leave the ref but flag it (move to pusher-inbox is still OK — but note in passing).

4. **Cited file:line locations**. If the body cites `path/to/file.go:NN` style references:
   - Use `gopls go_search` or `Read` to verify they still exist at roughly that location (symbol renames are fine; deletion or major drift is a problem).
   - If a citation is clearly stale, write a sidecar `failed.txt` rather than guessing.

5. **Tone / project conventions**. No corporate-speak. No "I'd be happy to". No emojis unless the original had them. Don't second-guess wording — only fix the mechanical issues above.

## Outcomes

- **All checks pass**: `mv "$EVAL_PATH" .issues/pusher-inbox/$(basename "$EVAL_PATH")`. Output one short line on stdout describing what was checked.
- **Mechanical fix applied** (e.g. md ref → #N): Apply the Edit, then move. Output one short line summarizing.
- **Hard fail** (bad labels, stale citations, format unparseable): leave file in place. Write `${EVAL_PATH}.failed.txt` with a few bullets explaining what to fix. Output one short line.

Do not push to Codeberg yourself — that's the pusher loop's job. Just decide pass/fail and move (or sidecar).

Keep your output to under 5 lines of stdout. The full reasoning belongs in the sidecar on fail; otherwise nothing else is needed.
