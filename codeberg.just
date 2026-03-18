codeberg_api := "https://codeberg.org/api/v1"
codeberg_repo := "scampi-dev/scampi"

[doc("Create issues from file or dir (--dry-run to preview)")]
[positional-arguments]
create-issue path *flags:
  #!/usr/bin/env bash
  shift  # path
  exec ./scripts/codeberg-create-issue.sh \
    "{{codeberg_api}}" "{{codeberg_repo}}" "{{path}}" "$@"

[doc("Update an issue (--body, --state, --labels)")]
[positional-arguments]
update-issue number *args:
  #!/usr/bin/env bash
  shift  # number
  exec ./scripts/codeberg-update-issue.sh \
    "{{codeberg_api}}" "{{codeberg_repo}}" "{{number}}" "$@"

[doc("Close an issue")]
close-issue number:
  just cb update-issue {{number}} --state closed

[doc("Reopen an issue")]
reopen-issue number:
  just cb update-issue {{number}} --state open

[doc("List open issues (optional: --label 'Kind/Bug')")]
[positional-arguments]
list-issues *args:
  #!/usr/bin/env bash
  exec ./scripts/codeberg-list-issues.sh \
    "{{codeberg_api}}" "{{codeberg_repo}}" "$@"

[doc("Assign an issue to a user")]
assign-issue number user:
  ./scripts/codeberg-assign-issue.sh \
    "{{codeberg_api}}" "{{codeberg_repo}}" \
    "{{number}}" "{{user}}"

[doc("Comment on an issue")]
comment-issue number body:
  ./scripts/codeberg-comment-issue.sh \
    "{{codeberg_api}}" "{{codeberg_repo}}" \
    "{{number}}" "{{body}}"

[doc("Generate release notes from issues since last tag")]
[positional-arguments]
release-notes *args:
  #!/usr/bin/env bash
  exec ./scripts/release-notes.sh \
    "{{codeberg_api}}" "{{codeberg_repo}}" "$@"

[doc("Calculate next version tag from issue labels")]
[positional-arguments]
next-version *args:
  #!/usr/bin/env bash
  exec ./scripts/next-version.sh \
    "{{codeberg_api}}" "{{codeberg_repo}}" "$@"

[doc("Show full issue details (--refresh to bypass cache)")]
[positional-arguments]
show-issue number *args:
  #!/usr/bin/env bash
  shift  # number
  exec ./scripts/codeberg-show-issue.sh \
    "{{codeberg_api}}" "{{codeberg_repo}}" "{{number}}" "$@"
