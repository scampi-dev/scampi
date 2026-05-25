# Help
# -----------------------------------------------------------------------------

[default]
[doc("Show this help message")]
@help *args:
  just --unsorted --list release {{args}}

# Releases
# -----------------------------------------------------------------------------

[doc("Generate release notes from issues since last tag")]
[positional-arguments]
notes *args:
  #!/usr/bin/env bash
  exec ./scripts/release-notes.sh "$@"

[doc("Calculate next version tag (alpha|beta|rc or omit for stable)")]
[positional-arguments]
next *args:
  #!/usr/bin/env bash
  exec ./scripts/next-version.sh "$@"

[doc("Prepare a release: changelog, commit, tag (alpha|beta|rc or omit for stable)")]
[positional-arguments]
cut *args:
  #!/usr/bin/env bash
  set -euo pipefail
  ./scripts/release.sh "$@"
  echo ""
  echo "run 'just release push' when ready"

[doc("Push the release commit and tag to origin")]
push:
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
  echo "done -- $tag pushed to origin"

[doc("Undo the last unpushed release (delete tag and reset release commit)")]
undo:
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
    echo "error: $tag has already been pushed to origin -- cannot undo" >&2
    exit 1
  fi
  upstream=$(git rev-parse --abbrev-ref '@{upstream}' 2>/dev/null || true)
  if [[ -n "$upstream" ]]; then
    remote_head=$(git rev-parse "$upstream" 2>/dev/null || true)
    local_head=$(git rev-parse HEAD)
    if [[ "$remote_head" == "$local_head" ]]; then
      echo "error: release commit has already been pushed -- cannot undo" >&2
      exit 1
    fi
  fi
  echo "undoing: $tag"
  git tag -d "$tag"
  git reset --hard HEAD~1
  echo "done -- tag deleted, release commit removed"
