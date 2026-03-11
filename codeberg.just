codeberg_api := "https://codeberg.org/api/v1"
codeberg_repo := "pskry/scampi"

[doc("Create issues from a file or all *.md in a directory (--dry-run to preview)")]
create-issue path *flags:
  #!/usr/bin/env bash
  set -euo pipefail
  : "${CODEBERG_TOKEN:?Set CODEBERG_TOKEN to a Codeberg personal access token}"

  dry_run=false
  for flag in {{flags}}; do
    case "$flag" in
      --dry-run) dry_run=true ;;
      *) echo "unknown flag: $flag"; exit 1 ;;
    esac
  done

  files=()
  if [[ -d "{{path}}" ]]; then
    for f in "{{path}}"/*.md; do
      [[ -f "$f" ]] && files+=("$f")
    done
  else
    files=("{{path}}")
  fi

  if [[ ${#files[@]} -eq 0 ]]; then
    echo "No .md files found in {{path}}"
    exit 1
  fi

  # Fetch labels and existing issue titles once
  all_labels=$(curl -sf \
    -H "Authorization: token $CODEBERG_TOKEN" \
    "{{codeberg_api}}/repos/{{codeberg_repo}}/labels?limit=50")
  existing_titles=$(curl -sf \
    -H "Authorization: token $CODEBERG_TOKEN" \
    "{{codeberg_api}}/repos/{{codeberg_repo}}/issues?type=issues&state=open&limit=50" \
    | jq -r '.[].title')

  for f in "${files[@]}"; do
    title=$(sed -n '1p' "$f")
    labels_line=$(sed -n '2p' "$f")
    body=$(tail -n +4 "$f")

    label_ids="[]"
    label_names=""
    if [[ "$labels_line" == labels:* ]]; then
      label_names=$(echo "$labels_line" | sed 's/^labels: *//')
      label_ids=$(echo "$all_labels" | jq --arg names "$label_names" '
        ($names | split(", ")) as $want |
        [.[] | select(.name as $n | $want | any(. == $n)) | .id]')
    fi

    if echo "$existing_titles" | grep -qxF "$title"; then
      echo "skip: $title (already exists)"
      continue
    fi

    if [[ "$dry_run" == true ]]; then
      echo "--- $f"
      echo "  title:  $title"
      echo "  labels: $label_names"
      echo "  body:   $(echo "$body" | wc -l | tr -d ' ') lines"
      continue
    fi

    resp=$(curl -sf \
      -H "Authorization: token $CODEBERG_TOKEN" \
      -H "Content-Type: application/json" \
      -d "$(jq -n --arg t "$title" --arg b "$body" --argjson l "$label_ids" \
        '{title: $t, body: $b, labels: $l}')" \
      "{{codeberg_api}}/repos/{{codeberg_repo}}/issues")
    echo "$resp" | jq -r '"#\(.number): \(.title) → \(.html_url)"'

    if [[ ${#files[@]} -gt 1 ]]; then
      sleep 3
    fi
  done

[doc("Update an issue: just codeberg update-issue 9 --body file.md --state closed --labels 'Kind/Bug, Priority/High'")]
update-issue number *args:
  #!/usr/bin/env bash
  set -euo pipefail
  : "${CODEBERG_TOKEN:?Set CODEBERG_TOKEN to a Codeberg personal access token}"

  body_file="" state="" labels="" title=""
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --body)   body_file="$2"; shift 2 ;;
      --state)  state="$2"; shift 2 ;;
      --labels) labels="$2"; shift 2 ;;
      --title)  title="$2"; shift 2 ;;
      *) echo "unknown arg: $1"; exit 1 ;;
    esac
  done

  patch='{}'

  if [[ -n "$title" ]]; then
    patch=$(echo "$patch" | jq --arg t "$title" '. + {title: $t}')
  fi

  if [[ -n "$body_file" ]]; then
    body=$(<"$body_file")
    patch=$(echo "$patch" | jq --arg b "$body" '. + {body: $b}')
  fi

  if [[ -n "$state" ]]; then
    patch=$(echo "$patch" | jq --arg s "$state" '. + {state: $s}')
  fi

  if [[ -n "$labels" ]]; then
    all_labels=$(curl -sf \
      -H "Authorization: token $CODEBERG_TOKEN" \
      "{{codeberg_api}}/repos/{{codeberg_repo}}/labels?limit=50")
    label_ids=$(echo "$all_labels" | jq --arg names "$labels" '
      ($names | split(", ")) as $want |
      [.[] | select(.name as $n | $want | any(. == $n)) | .id]')
    patch=$(echo "$patch" | jq --argjson l "$label_ids" '. + {labels: $l}')
  fi

  if [[ "$patch" == '{}' ]]; then
    echo "nothing to update — pass --title, --body, --state, or --labels"
    exit 1
  fi

  resp=$(curl -sf -X PATCH \
    -H "Authorization: token $CODEBERG_TOKEN" \
    -H "Content-Type: application/json" \
    -d "$patch" \
    "{{codeberg_api}}/repos/{{codeberg_repo}}/issues/{{number}}")
  echo "$resp" | jq -r '"#\(.number): \(.title) [\(.state)] → \(.html_url)"'

[doc("Close an issue")]
close-issue number:
  just codeberg update-issue {{number}} --state closed

[doc("Reopen an issue")]
reopen-issue number:
  just codeberg update-issue {{number}} --state open

[doc("List open issues (optional: --label 'Kind/Bug')")]
list-issues *args:
  #!/usr/bin/env bash
  set -euo pipefail
  : "${CODEBERG_TOKEN:?Set CODEBERG_TOKEN to a Codeberg personal access token}"

  label=""
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --label) label="$2"; shift 2 ;;
      *) echo "unknown arg: $1"; exit 1 ;;
    esac
  done

  url="{{codeberg_api}}/repos/{{codeberg_repo}}/issues?type=issues&state=open&limit=50&sort=created&direction=asc"
  if [[ -n "$label" ]]; then
    url="${url}&labels=$(python3 -c "import urllib.parse; print(urllib.parse.quote('$label'))")"
  fi

  curl -sf \
    -H "Authorization: token $CODEBERG_TOKEN" \
    "$url" \
    | jq -r '.[] | "#\(.number)  \(.title)  [\(.labels | map(.name) | join(", "))]"'

[doc("Assign an issue to a user")]
assign-issue number user:
  #!/usr/bin/env bash
  set -euo pipefail
  : "${CODEBERG_TOKEN:?Set CODEBERG_TOKEN to a Codeberg personal access token}"

  resp=$(curl -sf -X PATCH \
    -H "Authorization: token $CODEBERG_TOKEN" \
    -H "Content-Type: application/json" \
    -d "$(jq -n --arg u "{{user}}" '{assignees: [$u]}')" \
    "{{codeberg_api}}/repos/{{codeberg_repo}}/issues/{{number}}")
  echo "$resp" | jq -r '"#\(.number): \(.title) → assigned to \(.assignees | map(.login) | join(", "))"'

[doc("Show full issue details")]
show-issue number:
  #!/usr/bin/env bash
  set -euo pipefail
  : "${CODEBERG_TOKEN:?Set CODEBERG_TOKEN to a Codeberg personal access token}"

  curl -sf \
    -H "Authorization: token $CODEBERG_TOKEN" \
    "{{codeberg_api}}/repos/{{codeberg_repo}}/issues/{{number}}" \
    | jq -r '"#\(.number): \(.title)\nState: \(.state)\nLabels: \(.labels | map(.name) | join(", "))\nURL: \(.html_url)\n\n\(.body)"'

  comments=$(curl -sf \
    -H "Authorization: token $CODEBERG_TOKEN" \
    "{{codeberg_api}}/repos/{{codeberg_repo}}/issues/{{number}}/comments")

  count=$(echo "$comments" | jq 'length')
  if [[ "$count" -gt 0 ]]; then
    echo ""
    echo "--- comments ($count) ---"
    echo "$comments" | jq -r '.[] | "\n@\(.user.login) (\(.created_at)):\n\(.body)"'
  fi
