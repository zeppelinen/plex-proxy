#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

VERSION="${VERSION:-${GITHUB_REF_NAME:-dev}}"
COMMIT="${COMMIT:-$(git rev-parse --short HEAD 2>/dev/null || echo unknown)}"
DATE="${DATE:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}"
LDFLAGS="-s -w -X github.com/justnodes/plex-proxy/internal/version.Version=${VERSION} -X github.com/justnodes/plex-proxy/internal/version.Commit=${COMMIT} -X github.com/justnodes/plex-proxy/internal/version.Date=${DATE}"

rm -rf dist
mkdir -p dist

build_one() {
  local goos="$1"
  local goarch="$2"
  local name="plex-proxy_${VERSION}_${goos}_${goarch}"
  local out="dist/${name}/plex-proxy"
  mkdir -p "dist/${name}"
  GOOS="$goos" GOARCH="$goarch" CGO_ENABLED=0 go build -trimpath -ldflags "$LDFLAGS" -o "$out" ./cmd/plex-proxy
  cp README.md examples/config.yaml examples/plex-proxy.service "dist/${name}/"
  tar -C dist -czf "dist/${name}.tar.gz" "$name"
}

build_one linux amd64
build_one linux arm64
build_one darwin arm64

(cd dist && sha256sum *.tar.gz > checksums.txt)
