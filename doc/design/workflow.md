# Workflow

How we work — from first principles to merged code.

## 1. Problem discovery

Start from the user's perspective: what sucks right now, what's missing,
what's confusing. Not "we need to refactor X" but "a user trying to do Y
hits a wall because Z."

## 2. Design the destination

Work backwards: what does the user experience look like when this is
solved? What's the minimal surface that gets them there? This shapes
scope before anything gets filed.

## 3. Milestone

Group related work into a milestone — a shippable increment. Not a
sprint, not a deadline. A milestone is "after these N things land, users
can do X that they couldn't before." It's the release boundary.

## 4. Decompose into issues

Each issue is a **deliverable, releasable unit of work** — not a task in
a checklist. The bar: could you push this one issue and have it make
sense on its own? No half-states, no "part 1 of 3" that breaks without
part 2.

Issues get filed via `.issues/foo.md` → `just cb create-issue`.

## 5. Start a session

User drops an issue number. Claude:
- `just cb assign-issue N pskry`
- `just cb show-issue N` to read it
- Reads any linked context (related issues, design docs, relevant code)

## 6. Clarification & design

Before touching code, ask questions:
- Ambiguities in the issue
- Edge cases that aren't specified
- Design choices where multiple valid approaches exist
- Scope boundaries ("does this include X or is that a separate issue?")

Goal: align on what "done" looks like before writing line one.

## 7. Planning

Lay out a plan — what changes, in what order, why. User signs off or
redirects. Lightweight, not a design doc — just enough to avoid building
the wrong thing.

## 8. Implementation

Code → inner loop (`just test all`, `just fmt`, `just lint`). Iterate until
green. No committing yet — that's the user's call.

## 9. Review

When implementation is complete, say "done, please review." User reviews
the diff. Three outcomes:
- **Changes requested** → fix, re-test, re-present
- **Approved** → `just test nocontainers` (or `everything` if warranted), then commit
- **Needs rethinking** → back to step 6 or 7

## 10. Commit & close

Commit with magic keyword: `feat: thing (closes #N)`. Issue closes on
push. Never manually close issues or push — user handles that.

## Circuit breaker

If during implementation the scope starts ballooning — more files
touched than expected, cascading changes, rabbit holes — stop and flag
it:

> "This is getting bigger than the issue warrants. Here's what's
> happening: [X]. Options: (a) split into a follow-up issue, (b)
> simplify approach to [Y], (c) keep going if you think this is fine."

The trigger isn't a line count — it's when the change stops feeling like
a single coherent unit that you'd want to review as one diff.

## What Claude doesn't do

- Commit without being told
- Push
- Close issues manually
- Start work without assigning the issue first
- Skip clarification on non-trivial issues
- Run `just test everything` as a dev sanity check (that's the pre-push gate)
