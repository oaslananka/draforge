#!/usr/bin/env bash
set -euo pipefail

workflow_dir=${1:-.github/workflows}
violations=0

while IFS= read -r entry; do
  file=${entry%%:*}
  remainder=${entry#*:}
  line=${remainder%%:*}
  text=${remainder#*:}
  action_ref=${text#*@}
  action_ref=${action_ref%%[[:space:]#]*}

  case "$text" in
    *"uses: ./"*|*"uses: docker://"*)
      continue
      ;;
  esac

  if [[ ! "$action_ref" =~ ^[0-9a-f]{40}$ ]]; then
    printf 'Unpinned GitHub Action: %s:%s: %s\n' "$file" "$line" "${text#*[![:space:]]}" >&2
    violations=$((violations + 1))
  fi
done < <(grep -RInE '^[[:space:]-]*uses:[[:space:]]+[^[:space:]#]+@[^[:space:]#]+' "$workflow_dir" || true)

if (( violations > 0 )); then
  printf 'Found %d GitHub Action reference(s) not pinned to a full commit SHA.\n' "$violations" >&2
  exit 1
fi

printf 'All GitHub Action references are pinned to full commit SHAs.\n'
