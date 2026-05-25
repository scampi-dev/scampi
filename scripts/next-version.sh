#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-only
#
# Calculate the next semver tag based on issue labels since the last tag.
# Usage: next-version.sh [alpha|beta|rc]
#
# Inspects git log since the last tag, extracts issue numbers from
# "fixes #N" / "closes #N", reads issue labels from GitHub via the `gh`
# CLI, and determines the bump level:
#
#   compat/breaking  -> major
#   kind/feature     -> minor
#   kind/enhancement -> minor
#   kind/bug         -> patch
#
# Stage argument:
#   alpha, beta, rc  Pre-release tag with auto-incrementing suffix.
#   (omitted)        Stable release.
#
# Pre-release ordering is enforced: alpha < beta < rc. Going backwards
# (e.g. requesting alpha when beta tags exist) is an error.
set -euo pipefail

pre_stage=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    alpha|beta|rc) pre_stage="$1"; shift ;;
    *) echo "usage: next-version.sh [alpha|beta|rc]" >&2; exit 1 ;;
  esac
done

# Pre-release stage ordering (lower index = earlier stage).
stage_rank() {
  case "$1" in
    alpha) echo 0 ;;
    beta)  echo 1 ;;
    rc)    echo 2 ;;
  esac
}

# Find the last tag (stable or pre-release)
last_tag=$(git describe --tags --abbrev=0 2>/dev/null || echo "")

# Extract the stable base from the last tag (strip pre-release suffix)
if [[ -n "$last_tag" ]]; then
  base="${last_tag%%-*}"  # v0.1.0-alpha.1 -> v0.1.0
else
  base="v0.0.0"
fi

major="${base#v}"          # 0.1.0
major="${major%%.*}"       # 0
rest="${base#v"$major".}"  # 1.0
minor="${rest%%.*}"        # 1
patch="${rest#"$minor".}"  # 0

# Collect issue numbers from commits since last tag
if [[ -n "$last_tag" ]]; then
  range="${last_tag}..HEAD"
else
  range="HEAD"
fi

issues=$(git log "$range" --format='%s%n%b' \
  | grep -ioE '(fixes|closes|fix|close|resolves|resolve) #[0-9]+' \
  | grep -oE '[0-9]+' \
  | sort -u || true)

if [[ -z "$issues" ]]; then
  echo "no issues found since ${last_tag:-inception}" >&2
  bump="patch"
else
  bump="patch"
  summary=""

  for issue in $issues; do
    json=$(gh issue view "$issue" --json title,labels 2>/dev/null || echo '{}')

    title=$(echo "$json" | jq -r '.title // "???"')
    labels=$(echo "$json" | jq -r '.labels[].name' 2>/dev/null || true)
    summary="${summary}  #${issue} ${title}\n"

    for label in $labels; do
      case "$label" in
        compat/breaking)
          bump="major"
          ;;
        kind/feature|kind/enhancement)
          [[ "$bump" != "major" ]] && bump="minor"
          ;;
      esac
    done
  done

  echo "issues since ${last_tag:-inception}:" >&2
  printf '%b' "$summary" >&2
  echo "bump: $bump" >&2
fi

if [[ -n "$pre_stage" ]]; then
  next="v${major}.${minor}.${patch}"

  if [[ "$last_tag" == *-* ]]; then
    # Already on a pre-release — check for regression
    highest_pre=$(git tag -l "${next}-*" | sort -V | tail -1)
    if [[ -n "$highest_pre" ]]; then
      highest_stage="${highest_pre#"${next}"-}"  # alpha.2 / beta.1 / rc.3
      highest_stage="${highest_stage%%.*}"        # alpha / beta / rc

      requested_rank=$(stage_rank "$pre_stage")
      highest_rank=$(stage_rank "$highest_stage")

      if (( requested_rank < highest_rank )); then
        echo "error: cannot go back from $highest_stage to $pre_stage" >&2
        echo "  highest tag for $next: $highest_pre" >&2
        exit 1
      fi
    fi
  else
    # Coming from stable — apply the semver bump
    case "$bump" in
      major) major=$((major + 1)); minor=0; patch=0 ;;
      minor) minor=$((minor + 1)); patch=0 ;;
      patch) patch=$((patch + 1)) ;;
    esac
    next="v${major}.${minor}.${patch}"
  fi

  existing=$(git tag -l "${next}-${pre_stage}.*" | sort -V | tail -1)
  if [[ -n "$existing" ]]; then
    last_num="${existing##*.}"
    next_num=$((last_num + 1))
  else
    next_num=1
  fi
  next="${next}-${pre_stage}.${next_num}"
else
  # Stable release
  if [[ "$last_tag" == *-* ]]; then
    # Coming from pre-release — promote the base version as-is
    next="v${major}.${minor}.${patch}"
  else
    # Coming from stable — apply the semver bump
    case "$bump" in
      major) major=$((major + 1)); minor=0; patch=0 ;;
      minor) minor=$((minor + 1)); patch=0 ;;
      patch) patch=$((patch + 1)) ;;
    esac
    next="v${major}.${minor}.${patch}"
  fi
fi

echo "$next"
