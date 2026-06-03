#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-only

# gh-issue.sh - push an issue draft from .issues/<name>.md
#
# The markdown file carries YAML frontmatter with `title:` and
# `labels:` fields. Body is everything after the second `---`.

set -euo pipefail

if [[ $# -ne 1 ]]; then
    printf 'usage: %s <issue-name>\n' "$0" >&2
    exit 1
fi

name="$1"
file=".issues/${name}.md"

if [[ ! -f "$file" ]]; then
    printf 'error: %s not found\n' "$file" >&2
    exit 1
fi

title=$(awk '/^title: / { sub(/^title: */, ""); gsub(/^"|"$/, ""); print; exit }' "$file")
labels=$(awk '/^labels: / { sub(/^labels: */, ""); gsub(/^"|"$/, ""); print; exit }' "$file")
body=$(awk '/^---$/{c++; next} c>=2' "$file")

if [[ -z "$title" ]]; then
    printf 'error: no title in frontmatter\n' >&2
    exit 1
fi

label_arg=()
if [[ -n "$labels" ]]; then
    label_arg=(--label "$labels")
fi

gh issue create --title "$title" --body "$body" "${label_arg[@]}"
rm "$file"
