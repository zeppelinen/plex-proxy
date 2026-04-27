#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 1 ]]; then
  echo "usage: $0 <tag>" >&2
  exit 2
fi

tag="$1"
date_utc="$(date -u +%Y-%m-%d)"
changelog="CHANGELOG.md"
previous_tag="$(git tag --sort=-creatordate | grep -E '^v' | grep -v -F "$tag" | head -n 1 || true)"

if [[ -n "$previous_tag" ]]; then
  range="${previous_tag}..${tag}"
else
  range="$tag"
fi

commits="$(git log --no-merges --pretty=format:'- %s (%h)' "$range")"
if [[ -z "$commits" ]]; then
  commits="- No non-merge commits."
fi

tmp="$(mktemp)"
{
  if [[ -f "$changelog" ]]; then
    head -n 1 "$changelog"
  else
    echo "# Changelog"
  fi
  echo
  echo "## ${tag} - ${date_utc}"
  echo
  echo "$commits"
  echo
  if [[ -f "$changelog" ]]; then
    tail -n +2 "$changelog" | sed '/./,$!d'
  fi
} > "$tmp"

mv "$tmp" "$changelog"
