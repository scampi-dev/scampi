#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-only
#
# Prepare a release: generate changelog, update CHANGELOG.md, and tag.
# Usage: release.sh <api> <repo> [alpha|beta|rc] [--dry-run]
#
# Steps:
#   1. Compute next version via next-version.sh
#   2. Generate release notes for the new version
#   3. Tag the release
#   4. Regenerate CHANGELOG.md from all tags
#   5. Amend the release commit with the full changelog
#
# With --dry-run, prints what would happen without modifying anything.
set -euo pipefail

api="$1"
repo="$2"
shift 2
dir="$(cd "$(dirname "$0")" && pwd)"
root="$(cd "$dir/.." && pwd)"

stage=""
dry_run=false

while [[ $# -gt 0 ]]; do
  case "$1" in
  alpha | beta | rc)
    stage="$1"
    shift
    ;;
  --dry-run)
    dry_run=true
    shift
    ;;
  *)
    echo "usage: release.sh <api> <repo> [alpha|beta|rc] [--dry-run]" >&2
    exit 1
    ;;
  esac
done

# Ensure clean working tree
if [[ -n "$(git status --porcelain)" ]]; then
  echo "error: working tree is dirty — commit or stash first" >&2
  exit 1
fi

# Compute version
# shellcheck disable=SC2086
version=$("$dir/next-version.sh" "$api" "$repo" $stage --refresh)
echo ""
echo "next version: $version"
echo ""

# Generate release notes for HEAD (pre-tag, so use --head)
# shellcheck disable=SC2086
notes=$("$dir/release-notes.sh" "$api" "$repo" --head --refresh)

# Strip the header and whitespace — if nothing remains, there are no notes.
notes_body=$(echo "$notes" | sed '1d' | tr -d '[:space:]')
if [[ -z "$notes_body" ]]; then
  echo "error: no release notes generated (no issues found since last tag)" >&2
  exit 1
fi

echo "$notes"
echo ""

if [[ "$dry_run" == true ]]; then
  echo "--- dry run: would commit CHANGELOG.md and tag $version ---"
  exit 0
fi

# Create an empty release commit and tag it so the changelog generator
# can see this version in the tag list.
changelog="$root/CHANGELOG.md"
touch "$changelog"
git add "$changelog"
git commit --allow-empty -m "release: $version"
git tag -a "$version" -m "$version"

# Now regenerate the full changelog from all tags.
"$dir/generate-changelog.sh" "$api" "$repo" --refresh >"$changelog"

# Amend the release commit with the actual changelog.
git add "$changelog"
git commit --amend --no-edit

# Move the tag to the amended commit.
git tag -f -a "$version" -m "$version"

echo ""
echo "tagged $version"
