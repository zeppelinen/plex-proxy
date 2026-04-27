#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
RESULTS_DIR="${RESULTS_DIR:-$ROOT/test-results}"
COVERAGE_PROFILE="${COVERAGE_PROFILE:-$RESULTS_DIR/coverage.out}"
BASE_REF="${BASE_REF:-${GITHUB_BASE_REF:-}}"
GO="${GO:-go}"

coverage_percent() {
  "$GO" tool cover -func="$1" | awk '/^total:/ { gsub(/%/, "", $3); print $3 }'
}

write_summary() {
  local body="$1"
  if [[ -n "${GITHUB_STEP_SUMMARY:-}" ]]; then
    printf '%s\n' "$body" >>"$GITHUB_STEP_SUMMARY"
  fi
}

mkdir -p "$RESULTS_DIR"

if [[ ! -f "$COVERAGE_PROFILE" ]]; then
  "$GO" test -count=1 -covermode=atomic -coverprofile="$COVERAGE_PROFILE" ./...
fi

head_coverage="$(coverage_percent "$COVERAGE_PROFILE")"
if [[ -z "$head_coverage" ]]; then
  echo "Could not read total coverage from $COVERAGE_PROFILE" >&2
  exit 1
fi

if [[ -z "$BASE_REF" ]]; then
  summary="$(printf '## Coverage\n\nCurrent coverage: `%s%%`\n\nNo base ref was provided, so regression comparison was skipped.' "$head_coverage")"
  write_summary "$summary"
  echo "Current coverage: $head_coverage%"
  exit 0
fi

git fetch --no-tags --depth=1 origin "$BASE_REF"

baseline_dir="$(mktemp -d)"
cleanup() {
  git -C "$ROOT" worktree remove -f "$baseline_dir" >/dev/null 2>&1 || true
  rm -rf "$baseline_dir"
}
trap cleanup EXIT

git -C "$ROOT" worktree add --detach "$baseline_dir" "origin/$BASE_REF" >/dev/null

baseline_profile="$RESULTS_DIR/coverage-base.out"
(
  cd "$baseline_dir"
  "$GO" test -count=1 -covermode=atomic -coverprofile="$baseline_profile" ./...
)

base_coverage="$(coverage_percent "$baseline_profile")"
if [[ -z "$base_coverage" ]]; then
  echo "Could not read total coverage from baseline profile" >&2
  exit 1
fi

delta="$(awk -v head="$head_coverage" -v base="$base_coverage" 'BEGIN { printf "%.1f", head - base }')"
summary="$(printf '## Coverage\n\nBase `%s`: `%s%%`\n\nCurrent: `%s%%`\n\nDelta: `%+.1f%%`' "$BASE_REF" "$base_coverage" "$head_coverage" "$delta")"
write_summary "$summary"

if awk -v head="$head_coverage" -v base="$base_coverage" 'BEGIN { exit !(head + 0.00001 < base) }'; then
  echo "Coverage decreased from $base_coverage% to $head_coverage%" >&2
  exit 1
fi

echo "Coverage check passed: $head_coverage% (base $base_coverage%, delta ${delta}%)"
