#!/usr/bin/env bash
set -euo pipefail

repo="zeppelinen/plex-proxy"
bin_dir="${BIN_DIR:-/usr/local/bin}"
api_url="https://api.github.com/repos/${repo}/releases/latest"

log() {
  printf '%s\n' "$*"
}

fail() {
  printf 'error: %s\n' "$*" >&2
  exit 1
}

need() {
  command -v "$1" >/dev/null 2>&1 || fail "missing required command: $1"
}

need curl
need tar
need install

os="$(uname -s | tr '[:upper:]' '[:lower:]')"
arch="$(uname -m)"

case "$os/$arch" in
  linux/x86_64|linux/amd64) target="linux_amd64" ;;
  linux/aarch64|linux/arm64) target="linux_arm64" ;;
  darwin/arm64) target="darwin_arm64" ;;
  *)
    fail "unsupported platform: $os/$arch"
    ;;
esac

tmp_dir="$(mktemp -d)"
cleanup() {
  rm -rf "$tmp_dir"
}
trap cleanup EXIT

release_json="$tmp_dir/release.json"
curl -fsSL "$api_url" -o "$release_json"

tag="$(
  sed -nE 's/.*"tag_name"[[:space:]]*:[[:space:]]*"([^"]+)".*/\1/p' "$release_json" |
    head -n 1
)"
asset_url="$(
  sed -nE "s#.*\"browser_download_url\"[[:space:]]*:[[:space:]]*\"([^\"]*/plex-proxy_[^\"]*_${target}\\.tar\\.gz)\".*#\\1#p" "$release_json" |
    head -n 1
)"

if [[ -z "$tag" ]]; then
  fail "could not detect latest release tag"
fi
if [[ -z "$asset_url" ]]; then
  fail "could not find latest release artifact for $target"
fi

archive="$tmp_dir/plex-proxy.tar.gz"
log "Downloading plex-proxy $tag for $target..."
curl -fsSL "$asset_url" -o "$archive"
tar -xzf "$archive" -C "$tmp_dir"

binary="$(find "$tmp_dir" -type f -name plex-proxy -perm -111 | head -n 1)"
if [[ -z "$binary" ]]; then
  fail "downloaded artifact did not contain a plex-proxy binary"
fi

mkdir -p "$bin_dir" 2>/dev/null || true
if [[ -w "$bin_dir" ]]; then
  install -m 0755 "$binary" "$bin_dir/plex-proxy"
elif command -v sudo >/dev/null 2>&1; then
  log "Installing to $bin_dir requires elevated privileges."
  sudo install -m 0755 "$binary" "$bin_dir/plex-proxy"
else
  fail "$bin_dir is not writable and sudo is not available"
fi

log "Installed $bin_dir/plex-proxy"
"$bin_dir/plex-proxy" version

if [[ "$os" == "darwin" ]]; then
  cat <<EOF

macOS Privacy & Security note:
If macOS blocks plex-proxy after the first launch, open System Settings,
go to Privacy & Security, scroll to the security message for plex-proxy,
and choose Open Anyway. Then run plex-proxy again and confirm Open.

You can also remove quarantine from the installed binary:
  xattr -d com.apple.quarantine "$bin_dir/plex-proxy"
EOF
fi
