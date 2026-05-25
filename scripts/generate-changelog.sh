#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-only
#
# Regenerate CHANGELOG.md from all version tags.
# Usage: generate-changelog.sh
#
# Walks tags newest-first, generates release notes for each range,
# and outputs a complete changelog to stdout. Reads issue metadata
# from GitHub via the `gh` CLI.
set -euo pipefail

# Collect all version tags, newest first
tags=$(git tag -l 'v*' --sort=-version:refname)

if [[ -z "$tags" ]]; then
  echo "# Changelog"
  exit 0
fi

echo "# Changelog"
echo ""

# Build a simple list we can index into
set -f  # disable globbing
tag_list=""
for t in $tags; do
  tag_list="${tag_list}${t} "
done
set +f

emit_section() {
  local tag="$1" range="$2"

  tag_date=$(git log -1 --format='%cs' "$tag")

  issues=$(git log "$range" --format='%s%n%b' \
    | grep -ioE '(fixes|closes|fix|close|resolves|resolve) #[0-9]+' \
    | grep -oE '[0-9]+' \
    | sort -u || true)

  if [[ -z "$issues" ]]; then
    echo "## ${tag} — ${tag_date}"
    echo ""
    return
  fi

  features=""
  enhancements=""
  bugs=""
  breaking=""
  security=""
  other=""

  for issue in $issues; do
    json=$(gh issue view "$issue" --json title,labels 2>/dev/null || echo '{}')

    title=$(echo "$json" | jq -r '.title // "???"')
    labels=$(echo "$json" | jq -r '.labels[].name' 2>/dev/null || true)
    entry="- ${title} (#${issue})"

    classified=false
    for label in $labels; do
      case "$label" in
        compat/breaking)  breaking="${breaking}${entry}\n"; classified=true ;;
        kind/security)    security="${security}${entry}\n"; classified=true ;;
        kind/feature)     features="${features}${entry}\n"; classified=true ;;
        kind/enhancement) enhancements="${enhancements}${entry}\n"; classified=true ;;
        kind/bug)         bugs="${bugs}${entry}\n"; classified=true ;;
      esac
    done

    if [[ "$classified" == false ]]; then
      other="${other}${entry}\n"
    fi
  done

  echo "## ${tag} — ${tag_date}"
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
}

# Walk tags pairwise: each tag's range is prev_tag..tag (or just tag for oldest)
prev=""
for tag in $tags; do
  if [[ -n "$prev" ]]; then
    emit_section "$prev" "${tag}..${prev}"
  fi
  prev="$tag"
done

# Oldest tag — range is everything up to it
if [[ -n "$prev" ]]; then
  emit_section "$prev" "$prev"
fi
