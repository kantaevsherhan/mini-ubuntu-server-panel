#!/usr/bin/env bash
set -Eeuo pipefail

REPOSITORY="${MINI_UBUNTU_SERVER_REPOSITORY:-kantaevsherhan/mini-ubuntu-server-panel}"
VERSION="${1:-latest}"
INSTALL_DIR="/opt/mini-ubuntu-server"
DATA_DIR="/var/lib/mini-ubuntu-server"
SERVICE="mini-ubuntu-server.service"

[[ ${EUID} -eq 0 ]] || { echo "Run as root" >&2; exit 1; }
case "$(dpkg --print-architecture)" in
  amd64 | arm64) ARCH="$(dpkg --print-architecture)" ;;
  *) echo "Unsupported architecture" >&2; exit 1 ;;
esac

if [[ "$VERSION" == "latest" ]]; then
  VERSION="$(curl -fsSL "https://api.github.com/repos/$REPOSITORY/releases/latest" | sed -n 's/.*"tag_name": "\([^"]*\)".*/\1/p')"
fi
[[ -n "$VERSION" ]] || { echo "Unable to resolve release version" >&2; exit 1; }

TMP_DIR="$(mktemp -d)"
BACKUP_DIR="$DATA_DIR/backups/update-$(date -u +%Y%m%dT%H%M%SZ)"
ARCHIVE="mini-ubuntu-server-linux-$ARCH.tar.gz"
BASE_URL="https://github.com/$REPOSITORY/releases/download/$VERSION"
trap 'rm -rf "$TMP_DIR"' EXIT

curl -fsSL "$BASE_URL/$ARCHIVE" -o "$TMP_DIR/$ARCHIVE"
curl -fsSL "$BASE_URL/checksums.txt" -o "$TMP_DIR/checksums.txt"
(cd "$TMP_DIR" && grep " $ARCHIVE\$" checksums.txt | sha256sum -c -)
tar -xzf "$TMP_DIR/$ARCHIVE" -C "$TMP_DIR"

install -d -m 0750 "$BACKUP_DIR"
cp -a "$INSTALL_DIR/bin/mini-ubuntu-server" "$BACKUP_DIR/mini-ubuntu-server"
[[ ! -f "$DATA_DIR/mini-ubuntu-server.db" ]] || cp -a "$DATA_DIR/mini-ubuntu-server.db" "$BACKUP_DIR/mini-ubuntu-server.db"

rollback() {
  echo "Update failed; restoring previous binary" >&2
  install -o root -g root -m 0755 "$BACKUP_DIR/mini-ubuntu-server" "$INSTALL_DIR/bin/mini-ubuntu-server"
  systemctl restart "$SERVICE" || true
}
trap rollback ERR

systemctl stop "$SERVICE"
install -o root -g root -m 0755 "$TMP_DIR/mini-ubuntu-server" "$INSTALL_DIR/bin/mini-ubuntu-server"
systemctl start "$SERVICE"
sleep 2
curl -fsS http://127.0.0.1:8080/api/v1/health >/dev/null
trap - ERR
echo "Updated Mini Ubuntu Server Panel to $VERSION"
