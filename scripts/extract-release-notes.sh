#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 2 ]]; then
  echo "usage: $0 <tag> <output>" >&2
  exit 2
fi

tag="$1"
output="$2"

awk -v tag="$tag" '
  index($0, "## " tag " - ") == 1 {
    print
    in_section = 1
    next
  }
  in_section && /^## / {
    exit
  }
  in_section {
    print
  }
' CHANGELOG.md > "$output"
