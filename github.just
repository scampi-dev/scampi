repo := "scampi-dev/scampi"

# Help
# -----------------------------------------------------------------------------

[default]
[doc("Show this help message")]
@help *args:
  just --unsorted --list gh {{args}}

# Issues
# -----------------------------------------------------------------------------

[group('issues')]
[doc("List open issues (extra args pass through: --label kind/bug, --milestone N, --state closed, ...)")]
[positional-arguments]
list-issues *args:
  #!/usr/bin/env bash
  exec gh issue list --repo {{repo}} "$@"

[group('issues')]
[doc("Show full issue details (extras: --comments, --web)")]
[positional-arguments]
show-issue number *args:
  #!/usr/bin/env bash
  exec gh issue view --repo {{repo}} "$@"

[group('issues')]
[doc("Search issues (free-text query, gh search filter flags accepted)")]
[positional-arguments]
search-issues *args:
  #!/usr/bin/env bash
  exec gh search issues --repo {{repo}} "$@"

[group('issues')]
[doc("Create an issue (--title T --body-file path | --body text, --label L, --milestone M, --assignee U)")]
[positional-arguments]
create-issue *args:
  #!/usr/bin/env bash
  exec gh issue create --repo {{repo}} "$@"

[group('issues')]
[doc("Edit an issue (--add-label, --remove-label, --title, --body, --milestone, ...)")]
[positional-arguments]
update-issue number *args:
  #!/usr/bin/env bash
  exec gh issue edit --repo {{repo}} "$@"

[group('issues')]
[doc("Close an issue (extras: --reason completed|not-planned, --comment text)")]
[positional-arguments]
close-issue number *args:
  #!/usr/bin/env bash
  exec gh issue close --repo {{repo}} "$@"

[group('issues')]
[doc("Reopen an issue")]
reopen-issue number:
  gh issue reopen --repo {{repo}} {{number}}

[group('issues')]
[doc("Assign an issue to a user")]
assign-issue number user:
  gh issue edit --repo {{repo}} {{number}} --add-assignee {{user}}

[group('issues')]
[doc("Add a label to an issue (without removing existing labels)")]
add-label number label:
  gh issue edit --repo {{repo}} {{number}} --add-label {{label}}

[group('issues')]
[doc("Comment on an issue (--body text | --body-file path)")]
[positional-arguments]
comment-issue number *args:
  #!/usr/bin/env bash
  exec gh issue comment --repo {{repo}} "$@"

# Milestones
# -----------------------------------------------------------------------------
# gh has no first-class milestone subcommand, so these go through
# `gh api`. set-milestone uses --milestone with the TITLE string (gh
# accepts either title or number, but title reads better in scripts).

[group('milestones')]
[doc("List open milestones (--state closed for closed ones)")]
[positional-arguments]
list-milestones *args:
  #!/usr/bin/env bash
  state="open"
  for a in "$@"; do
    case "$a" in --state) shift; state="$1"; shift ;; esac
  done
  exec gh api "repos/{{repo}}/milestones?state=${state}" \
    --jq '.[] | "\(.number)\t\(.title)\t\(.open_issues) open / \(.closed_issues) closed"'

[group('milestones')]
[doc("Create a milestone")]
create-milestone title description="":
  gh api "repos/{{repo}}/milestones" -f title="{{title}}" -f description="{{description}}"

[group('milestones')]
[doc("Set milestone on an issue (by title)")]
set-milestone number title:
  gh issue edit --repo {{repo}} {{number}} --milestone "{{title}}"

[group('milestones')]
[doc("Close a milestone by number")]
close-milestone number:
  gh api -X PATCH "repos/{{repo}}/milestones/{{number}}" -f state=closed

# Pull requests
# -----------------------------------------------------------------------------

[group('prs')]
[doc("List PRs (--state open|closed|merged, --label L, --author U)")]
[positional-arguments]
list-prs *args:
  #!/usr/bin/env bash
  exec gh pr list --repo {{repo}} "$@"

[group('prs')]
[doc("Show PR details (extras: --comments, --web)")]
[positional-arguments]
show-pr number *args:
  #!/usr/bin/env bash
  exec gh pr view --repo {{repo}} "$@"

[group('prs')]
[doc("List inline review comments on a PR")]
pr-comments number:
  gh api "repos/{{repo}}/pulls/{{number}}/comments"

[group('prs')]
[doc("Show a PR's diff (--name-only for just the file list)")]
[positional-arguments]
pr-diff number *args:
  #!/usr/bin/env bash
  exec gh pr diff --repo {{repo}} "$@"

[group('prs')]
[doc("Show merge readiness of all open PRs (number, mergeable, merge state)")]
pr-status:
  #!/usr/bin/env bash
  set -euo pipefail
  gh pr list --repo {{repo}} --json number,mergeable,mergeStateStatus,title \
    --jq '.[] | [.number, .mergeable, .mergeStateStatus, .title] | @tsv'

[group('prs')]
[doc("Merge a PR (--squash|--merge|--rebase, --delete-branch, --auto)")]
[positional-arguments]
merge-pr number *args:
  #!/usr/bin/env bash
  exec gh pr merge --repo {{repo}} "$@"

[group('prs')]
[doc("Show PR checks / CI status")]
[positional-arguments]
pr-checks number *args:
  #!/usr/bin/env bash
  exec gh pr checks --repo {{repo}} "$@"

# Security
# -----------------------------------------------------------------------------

[group('security')]
[doc("Summarize open Dependabot alerts by severity, package and manifest")]
alerts:
  #!/usr/bin/env bash
  set -euo pipefail
  gh api "repos/{{repo}}/dependabot/alerts?state=open&per_page=100" --paginate \
    --jq '.[] | [.security_advisory.severity, .dependency.package.name, .dependency.manifest_path] | @tsv' \
    | sort | uniq -c | sort -rn

[group('security')]
[doc("Show one Dependabot alert in full (number from `just gh alerts`)")]
alert number:
  gh api "repos/{{repo}}/dependabot/alerts/{{number}}"

[group('security')]
[doc("List open Dependabot alerts with their alert numbers")]
list-alerts:
  #!/usr/bin/env bash
  set -euo pipefail
  gh api "repos/{{repo}}/dependabot/alerts?state=open&per_page=100" --paginate \
    --jq '.[] | [.number, .security_advisory.severity, .dependency.package.name, .dependency.manifest_path] | @tsv'
