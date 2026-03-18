#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-only
#
# Generate release notes from closed issues since the last tag.
# Usage: release-notes.sh <api> <repo> [--head] [--refresh]
#
# Groups issues by label into sections:
#   Kind/Feature     -> Features
#   Kind/Enhancement -> Enhancements
#   Kind/Bug         -> Bug Fixes
#   Kind/Breaking    -> Breaking Changes
#   Kind/Security    -> Security
#   (unlabelled)     -> Other
set -euo pipefail

api="$1"; repo="$2"; shift 2
dir="$(cd "$(dirname "$0")" && pwd)"

refresh_flag=""
head_flag=false
while [[ $# -gt 0 ]]; do
  case "$1" in
    --refresh) refresh_flag="--refresh"; shift ;;
    --head) head_flag=true; shift ;;
    *) echo "unknown option: $1" >&2; exit 1 ;;
  esac
done

# Find tag range
current_tag=$(git describe --tags --abbrev=0 2>/dev/null || echo "HEAD")

if [[ "$head_flag" == true ]]; then
  range="${current_tag}..HEAD"
else
  # Pipeline mode: range between the two most recent tags
  prev_tag=$(git tag -l 'v*' --sort=-version:refname \
    | awk -v cur="$current_tag" 'found{print;exit} $0==cur{found=1}')

  if [[ -n "$prev_tag" ]]; then
    range="${prev_tag}..${current_tag}"
  else
    range="$current_tag"
  fi
fi

# Collect issue numbers
issues=$(git log "$range" --format='%s%n%b' \
  | grep -ioE '(fixes|closes|fix|close|resolves|resolve) #[0-9]+' \
  | grep -oE '[0-9]+' \
  | sort -u || true)

# Classify issues by label
features=""
enhancements=""
bugs=""
breaking=""
security=""
other=""

for issue in $issues; do
  json=$("$dir/codeberg-fetch.sh" "$api/repos/$repo/issues/$issue" $refresh_flag)

  title=$(echo "$json" | jq -r '.title // "???"')
  labels=$(echo "$json" | jq -r '.labels[].name' 2>/dev/null || true)
  entry="- ${title} (#${issue})"

  classified=false
  for label in $labels; do
    case "$label" in
      Compat/Breaking)
        breaking="${breaking}${entry}\n"
        classified=true
        ;;
      Kind/Security)
        security="${security}${entry}\n"
        classified=true
        ;;
      Kind/Feature)
        features="${features}${entry}\n"
        classified=true
        ;;
      Kind/Enhancement)
        enhancements="${enhancements}${entry}\n"
        classified=true
        ;;
      Kind/Bug)
        bugs="${bugs}${entry}\n"
        classified=true
        ;;
    esac
  done

  if [[ "$classified" == false ]]; then
    other="${other}${entry}\n"
  fi
done

# Build output
echo "## What's Changed"
echo ""

if [[ -n "$breaking" ]]; then
  echo "### Breaking Changes"
  printf '%b' "$breaking"
  echo ""
fi

if [[ -n "$security" ]]; then
  echo "### Security"
  printf '%b' "$security"
  echo ""
fi

if [[ -n "$features" ]]; then
  echo "### Features"
  printf '%b' "$features"
  echo ""
fi

if [[ -n "$enhancements" ]]; then
  echo "### Enhancements"
  printf '%b' "$enhancements"
  echo ""
fi

if [[ -n "$bugs" ]]; then
  echo "### Bug Fixes"
  printf '%b' "$bugs"
  echo ""
fi

if [[ -n "$other" ]]; then
  echo "### Other"
  printf '%b' "$other"
  echo ""
fi
