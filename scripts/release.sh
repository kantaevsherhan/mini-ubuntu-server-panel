#!/usr/bin/env bash
set -Eeuo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd -- "$SCRIPT_DIR/.." && pwd)"
VERSION="${1:-dev}"
DIST_DIR="$PROJECT_DIR/dist"
PACKAGE_DIR="$DIST_DIR/package"

command -v bun >/dev/null || { echo "Bun is required" >&2; exit 1; }
command -v go >/dev/null || { echo "Go is required" >&2; exit 1; }

cd "$PROJECT_DIR/frontend"
bun install --frozen-lockfile
rm -rf "$PROJECT_DIR/backend/cmd/mini-ubuntu-server/web/assets" "$PROJECT_DIR/backend/cmd/mini-ubuntu-server/web/index.html"
bun run check

rm -rf "$PACKAGE_DIR"
mkdir -p "$PACKAGE_DIR"
for arch in amd64 arm64; do
  GOOS=linux GOARCH="$arch" CGO_ENABLED=0 go build \
    -C "$PROJECT_DIR/backend" -trimpath \
    -ldflags "-s -w -X main.version=$VERSION" \
    -o "$PACKAGE_DIR/mini-ubuntu-server" ./cmd/mini-ubuntu-server
  cp "$PROJECT_DIR/packaging/mini-ubuntu-server.service" "$PROJECT_DIR/packaging/config.example.yml" "$PACKAGE_DIR/"
  tar -C "$PACKAGE_DIR" -czf "$DIST_DIR/mini-ubuntu-server-linux-$arch.tar.gz" \
    mini-ubuntu-server mini-ubuntu-server.service config.example.yml
done

cd "$DIST_DIR"
sha256sum mini-ubuntu-server-linux-amd64.tar.gz mini-ubuntu-server-linux-arm64.tar.gz > checksums.txt
cp "$PROJECT_DIR/scripts/install.sh" "$PROJECT_DIR/scripts/uninstall.sh" .
echo "Release artifacts created in $DIST_DIR"
