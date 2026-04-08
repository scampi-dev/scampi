codeberg_api := "https://codeberg.org/api/v1"
codeberg_repo := "scampi-dev/scampi"

# Issues
# ##############################################################################

[doc("List open issues (optional: --label 'Kind/Bug', --milestone ID)")]
[positional-arguments]
list-issues *args:
  #!/usr/bin/env bash
  exec ./scripts/codeberg-list-issues.sh \
    "{{codeberg_api}}" "{{codeberg_repo}}" "$@"

[doc("Show full issue details (--refresh to bypass cache)")]
[positional-arguments]
show-issue number *args:
  #!/usr/bin/env bash
  shift  # number
  exec ./scripts/codeberg-show-issue.sh \
    "{{codeberg_api}}" "{{codeberg_repo}}" "{{number}}" "$@"

[doc("Create issues from file or dir (--dry-run to preview)")]
[positional-arguments]
create-issue path *flags:
  #!/usr/bin/env bash
  shift  # path
  exec ./scripts/codeberg-create-issue.sh \
    "{{codeberg_api}}" "{{codeberg_repo}}" "{{path}}" "$@"

[doc("Update an issue (--body, --state, --labels, --title)")]
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

[doc("Assign an issue to a user")]
assign-issue number user:
  ./scripts/codeberg-assign-issue.sh \
    "{{codeberg_api}}" "{{codeberg_repo}}" \
    "{{number}}" "{{user}}"

[doc("Add a label to an issue (without removing existing labels)")]
add-label number label:
  ./scripts/codeberg-add-label.sh \
    "{{codeberg_api}}" "{{codeberg_repo}}" \
    "{{number}}" "{{label}}"

[doc("Comment on an issue")]
comment-issue number body:
  ./scripts/codeberg-comment-issue.sh \
    "{{codeberg_api}}" "{{codeberg_repo}}" \
    "{{number}}" "{{body}}"

# Milestones
# ##############################################################################

[doc("List milestones (--closed for closed ones)")]
[positional-arguments]
list-milestones *args:
  #!/usr/bin/env bash
  exec ./scripts/codeberg-list-milestones.sh \
    "{{codeberg_api}}" "{{codeberg_repo}}" "$@"

[doc("Create a milestone")]
create-milestone title description="":
  ./scripts/codeberg-create-milestone.sh \
    "{{codeberg_api}}" "{{codeberg_repo}}" \
    "{{title}}" "{{description}}"

[doc("Set milestone on an issue")]
set-milestone issue_number milestone_id:
  ./scripts/codeberg-set-milestone.sh \
    "{{codeberg_api}}" "{{codeberg_repo}}" \
    "{{issue_number}}" "{{milestone_id}}"

# Releases
# ##############################################################################

[doc("Generate release notes from issues since last tag")]
[positional-arguments]
release-notes *args:
  #!/usr/bin/env bash
  exec ./scripts/release-notes.sh \
    "{{codeberg_api}}" "{{codeberg_repo}}" "$@"

[doc("Calculate next version tag (alpha|beta|rc or omit for stable)")]
[positional-arguments]
next-version *args:
  #!/usr/bin/env bash
  exec ./scripts/next-version.sh \
    "{{codeberg_api}}" "{{codeberg_repo}}" "$@"

[doc("Prepare a release: changelog, commit, tag (alpha|beta|rc or omit for stable)")]
[positional-arguments]
release *args:
  #!/usr/bin/env bash
  set -euo pipefail
  ./scripts/release.sh \
    "{{codeberg_api}}" "{{codeberg_repo}}" "$@"
  echo ""
  echo "run 'just cb push-release' when ready"

[doc("Push the release commit and tag to origin")]
push-release:
  #!/usr/bin/env bash
  set -euo pipefail
  tag=$(git describe --tags --abbrev=0 2>/dev/null || true)
  if [[ -z "$tag" ]]; then
    echo "error: no tags found" >&2
    exit 1
  fi
  last_msg=$(git log -1 --format='%s')
  if [[ "$last_msg" != "release: $tag" ]]; then
    echo "error: HEAD is not a release commit (expected 'release: $tag', got '$last_msg')" >&2
    exit 1
  fi
  echo "pushing $tag..."
  git push
  git push origin "$tag"
  echo "done — $tag pushed to origin"

[doc("Undo the last unpushed release (delete tag and reset release commit)")]
undo-release:
  #!/usr/bin/env bash
  set -euo pipefail
  tag=$(git describe --tags --abbrev=0 2>/dev/null || true)
  if [[ -z "$tag" ]]; then
    echo "error: no tags found" >&2
    exit 1
  fi
  last_msg=$(git log -1 --format='%s')
  if [[ "$last_msg" != "release: $tag" ]]; then
    echo "error: HEAD is not a release commit (expected 'release: $tag', got '$last_msg')" >&2
    exit 1
  fi
  # Refuse to undo pushed releases
  remote_tag=$(git ls-remote --tags origin "refs/tags/$tag" 2>/dev/null || true)
  if [[ -n "$remote_tag" ]]; then
    echo "error: $tag has already been pushed to origin — cannot undo" >&2
    exit 1
  fi
  upstream=$(git rev-parse --abbrev-ref '@{upstream}' 2>/dev/null || true)
  if [[ -n "$upstream" ]]; then
    remote_head=$(git rev-parse "$upstream" 2>/dev/null || true)
    local_head=$(git rev-parse HEAD)
    if [[ "$remote_head" == "$local_head" ]]; then
      echo "error: release commit has already been pushed — cannot undo" >&2
      exit 1
    fi
  fi
  echo "undoing: $tag"
  git tag -d "$tag"
  git reset --hard HEAD~1
  echo "done — tag deleted, release commit removed"
