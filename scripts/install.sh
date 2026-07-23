#!/usr/bin/env bash
set -Eeuo pipefail

REPOSITORY="${MINI_UBUNTU_SERVER_REPOSITORY:-kantaevsherhan/mini-ubuntu-server-panel}"
VERSION="latest"
PORT="8080"
ADMIN_USERNAME="admin"
DATA_DIR="/var/lib/mini-ubuntu-server"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --version) VERSION="$2"; shift 2 ;;
    --port) PORT="$2"; shift 2 ;;
    --username) ADMIN_USERNAME="$2"; shift 2 ;;
    --data-dir) DATA_DIR="$2"; shift 2 ;;
    --update) exec bash <(curl -fsSL "https://raw.githubusercontent.com/$REPOSITORY/main/scripts/update.sh") ;;
    *) echo "Unknown option: $1" >&2; exit 2 ;;
  esac
done

[[ ${EUID} -eq 0 ]] || { echo "Run as root" >&2; exit 1; }
source /etc/os-release
[[ ${ID:-} == ubuntu ]] || { echo "Ubuntu is required" >&2; exit 1; }
case "$(dpkg --print-architecture)" in
  amd64 | arm64) ARCH="$(dpkg --print-architecture)" ;;
  *) echo "Unsupported architecture" >&2; exit 1 ;;
esac

apt-get update -qq
apt-get install -y -qq ca-certificates curl openssl tar
getent group mini-ubuntu-server >/dev/null || groupadd --system mini-ubuntu-server
id mini-ubuntu-server >/dev/null 2>&1 || useradd --system --gid mini-ubuntu-server --home-dir "$DATA_DIR" --shell /usr/sbin/nologin mini-ubuntu-server
install -d -o root -g mini-ubuntu-server -m 0750 /opt/mini-ubuntu-server/bin /etc/mini-ubuntu-server
install -d -o mini-ubuntu-server -g mini-ubuntu-server -m 0750 "$DATA_DIR" "$DATA_DIR/backups" /var/log/mini-ubuntu-server

if [[ "$VERSION" == latest ]]; then
  VERSION="$(curl -fsSL "https://api.github.com/repos/$REPOSITORY/releases/latest" | sed -n 's/.*"tag_name": "\([^"]*\)".*/\1/p')"
fi
[[ -n "$VERSION" ]] || { echo "Unable to resolve release version" >&2; exit 1; }

BASE="https://github.com/$REPOSITORY/releases/download/$VERSION"
ARCHIVE="mini-ubuntu-server-linux-$ARCH.tar.gz"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT
curl -fsSL "$BASE/$ARCHIVE" -o "$TMP_DIR/$ARCHIVE"
curl -fsSL "$BASE/checksums.txt" -o "$TMP_DIR/checksums.txt"
(cd "$TMP_DIR" && grep " $ARCHIVE\$" checksums.txt | sha256sum -c -)
tar -xzf "$TMP_DIR/$ARCHIVE" -C "$TMP_DIR"

install -o root -g root -m 0755 "$TMP_DIR/mini-ubuntu-server" /opt/mini-ubuntu-server/bin/mini-ubuntu-server
install -o root -g root -m 0644 "$TMP_DIR/mini-ubuntu-server.service" /etc/systemd/system/mini-ubuntu-server.service
if [[ ! -f /etc/mini-ubuntu-server/config.yml ]]; then
  sed -e "s/:8080/:$PORT/" -e "s#/var/lib/mini-ubuntu-server#$DATA_DIR#" "$TMP_DIR/config.example.yml" > /etc/mini-ubuntu-server/config.yml
fi

TEMP_PASSWORD="$(openssl rand -base64 24 | tr -d '\n')"
JWT_SECRET="$(openssl rand -hex 32)"
install -o root -g mini-ubuntu-server -m 0640 /dev/null /etc/mini-ubuntu-server/secrets.env
printf 'MINI_UBUNTU_SERVER_JWT_SECRET=%s\nMINI_UBUNTU_SERVER_BOOTSTRAP_USERNAME=%s\nMINI_UBUNTU_SERVER_BOOTSTRAP_PASSWORD=%s\n' \
  "$JWT_SECRET" "$ADMIN_USERNAME" "$TEMP_PASSWORD" > /etc/mini-ubuntu-server/secrets.env
chown root:mini-ubuntu-server /etc/mini-ubuntu-server/secrets.env
chmod 0640 /etc/mini-ubuntu-server/secrets.env

systemctl daemon-reload
systemctl enable --now mini-ubuntu-server.service
sleep 2
curl -fsS "http://127.0.0.1:$PORT/api/v1/health" >/dev/null
sed -i '/^MINI_UBUNTU_SERVER_BOOTSTRAP_/d' /etc/mini-ubuntu-server/secrets.env

IP="$(hostname -I | awk '{print $1}')"
printf '\nMini Ubuntu Server Panel installed.\nURL: http://%s:%s\nUsername: %s\nTemporary password: %s\nChange it after first login.\n' \
  "$IP" "$PORT" "$ADMIN_USERNAME" "$TEMP_PASSWORD"
