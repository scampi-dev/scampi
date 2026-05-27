# Help
# -----------------------------------------------------------------------------

[default]
[doc("Show this help message")]
@help *args:
  just --unsorted --list release {{args}}

# Releases
# -----------------------------------------------------------------------------

[doc("Calculate next version tag (alpha|beta|rc or omit for stable)")]
[positional-arguments]
next *args:
  #!/usr/bin/env bash
  exec ./scripts/next-version.sh "$@"

[doc("Stage a release: tag the current HEAD (alpha|beta|rc or omit for stable)")]
[positional-arguments]
stage *args:
  #!/usr/bin/env bash
  set -euo pipefail
  ./scripts/release.sh "$@"
  echo ""
  echo "run 'just release push' when ready"

[doc("Push the staged tag to origin")]
push:
  #!/usr/bin/env bash
  set -euo pipefail
  tag=$(git describe --tags --abbrev=0 2>/dev/null || true)
  if [[ -z "$tag" ]]; then
    echo "error: no tags found" >&2
    exit 1
  fi
  # The tag must point at a commit reachable from HEAD; otherwise we'd
  # be publishing a release tag for code that's not on the current branch.
  if ! git merge-base --is-ancestor "$tag" HEAD; then
    echo "error: tag $tag is not an ancestor of HEAD -- refusing to push" >&2
    exit 1
  fi
  echo "pushing $tag..."
  # Atomic: main + tag in one push operation = one GH webhook event = one
  # each of CI / Mirror / Release runs (instead of double-firing).
  git push --atomic origin main "refs/tags/$tag"
  echo "done -- $tag pushed to origin"

[doc("Undo the last unpushed release tag")]
undo:
  #!/usr/bin/env bash
  set -euo pipefail
  tag=$(git describe --tags --abbrev=0 2>/dev/null || true)
  if [[ -z "$tag" ]]; then
    echo "error: no tags found" >&2
    exit 1
  fi
  # Refuse to undo pushed releases
  remote_tag=$(git ls-remote --tags origin "refs/tags/$tag" 2>/dev/null || true)
  if [[ -n "$remote_tag" ]]; then
    echo "error: $tag has already been pushed to origin -- cannot undo" >&2
    exit 1
  fi
  echo "undoing: $tag"
  git tag -d "$tag"
  echo "done -- tag deleted"
