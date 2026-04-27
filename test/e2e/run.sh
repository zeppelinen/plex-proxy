#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
E2E_DIR="$ROOT/test/e2e"

mkdir -p "$E2E_DIR/.tmp"
if [[ ! -f "$E2E_DIR/.tmp/id_ed25519" ]]; then
  ssh-keygen -t ed25519 -N '' -C plex-proxy-e2e@example -f "$E2E_DIR/.tmp/id_ed25519" >/dev/null
fi
chmod 600 "$E2E_DIR/.tmp/id_ed25519"

docker compose -f "$E2E_DIR/docker-compose.yml" up --build --abort-on-container-exit --exit-code-from test
