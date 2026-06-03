#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-only

# gh-issue.sh - push a draft markdown file to GitHub.
#
# The frontmatter's type field is required and decides what to do:
#   type: issue    -> gh issue create with title + body + labels
#   type: comment  -> gh issue comment <issue> with body
#
# Issue drafts carry title and labels. Comment drafts carry issue
# (the target issue number). On success the file is deleted.

set -euo pipefail

if [[ $# -ne 1 ]]; then
    printf 'usage: %s <draft-file>\n' "$0" >&2
    exit 1
fi

file="$1"

if [[ ! -f "$file" ]]; then
    printf 'error: %s not found\n' "$file" >&2
    exit 1
fi

# Frontmatter scan. Default type is `issue` so legacy drafts without
# the field keep working.
field() {
    awk -v key="$1" 'BEGIN{p="^" key ": "} $0 ~ p {sub(p, ""); gsub(/^"|"$/, ""); print; exit}' "$file"
}

type=$(field type)
if [[ -z "$type" ]]; then
    printf 'error: draft missing type field (issue or comment)\n' >&2
    exit 1
fi
body=$(awk '/^---$/{c++; next} c>=2' "$file")

case "$type" in
    comment)
        issue=$(field issue)
        if [[ -z "$issue" ]]; then
            printf 'error: comment draft missing issue field\n' >&2
            exit 1
        fi
        gh issue comment "$issue" --body "$body"
        ;;
    issue)
        title=$(field title)
        labels=$(field labels)
        if [[ -z "$title" ]]; then
            printf 'error: issue draft missing title field\n' >&2
            exit 1
        fi
        label_arg=()
        if [[ -n "$labels" ]]; then
            label_arg=(--label "$labels")
        fi
        gh issue create --title "$title" --body "$body" "${label_arg[@]}"
        ;;
    *)
        printf 'error: unknown draft type %q\n' "$type" >&2
        exit 1
        ;;
esac

rm "$file"
