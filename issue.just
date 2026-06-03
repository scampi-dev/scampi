# Help
# -----------------------------------------------------------------------------

[default]
[doc("Show this help message")]
@help *args:
  just --unsorted --list issue {{args}}

# Issue
# -----------------------------------------------------------------------------

[doc("Push a draft markdown file (issue or comment) to GitHub")]
@push file:
  ./scripts/gh-issue.sh {{file}}

[doc("Close an issue as not-planned and add a reason label (wontfix|duplicate|invalid)")]
@close issue label:
  gh issue edit {{issue}} --add-label {{label}}
  gh issue close {{issue}} --reason "not planned"
