#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
RESULTS_DIR="${RESULTS_DIR:-$ROOT/test-results}"
GO="${GO:-go}"

mkdir -p "$RESULTS_DIR"

set +e
"$GO" test -json ./... | tee "$RESULTS_DIR/unit.json"
status=${PIPESTATUS[0]}
set -e

exit "$status"
