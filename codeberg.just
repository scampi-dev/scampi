codeberg_api := "https://codeberg.org/api/v1"
codeberg_repo := "pskry/scampi"

[doc("Create issues from a file or all *.md in a directory (--dry-run to preview)")]
[positional-arguments]
create-issue path *flags:
  #!/usr/bin/env bash
  shift  # path
  exec ./scripts/codeberg-create-issue.sh "{{codeberg_api}}" "{{codeberg_repo}}" "{{path}}" "$@"

[doc("Update an issue: just codeberg update-issue 9 --body file.md --state closed --labels 'Kind/Bug, Priority/High'")]
[positional-arguments]
update-issue number *args:
  #!/usr/bin/env bash
  shift  # number
  exec ./scripts/codeberg-update-issue.sh "{{codeberg_api}}" "{{codeberg_repo}}" "{{number}}" "$@"

[doc("Close an issue")]
close-issue number:
  just codeberg update-issue {{number}} --state closed

[doc("Reopen an issue")]
reopen-issue number:
  just codeberg update-issue {{number}} --state open

[doc("List open issues (optional: --label 'Kind/Bug')")]
[positional-arguments]
list-issues *args:
  #!/usr/bin/env bash
  exec ./scripts/codeberg-list-issues.sh "{{codeberg_api}}" "{{codeberg_repo}}" "$@"

[doc("Assign an issue to a user")]
assign-issue number user:
  ./scripts/codeberg-assign-issue.sh "{{codeberg_api}}" "{{codeberg_repo}}" "{{number}}" "{{user}}"

[doc("Show full issue details")]
show-issue number:
  ./scripts/codeberg-show-issue.sh "{{codeberg_api}}" "{{codeberg_repo}}" "{{number}}"
