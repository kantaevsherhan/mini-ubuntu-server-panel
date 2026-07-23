#!/usr/bin/env bash
set -Eeuo pipefail
REPOSITORY="OWNER/mini-ubuntu-server-panel"
VERSION="latest"; PORT="8080"; ADMIN_USERNAME="admin"; DATA_DIR="/var/lib/mini-ubuntu-server"
while [[ $# -gt 0 ]]; do case "$1" in --version) VERSION="$2";shift 2;;--port) PORT="$2";shift 2;;--username) ADMIN_USERNAME="$2";shift 2;;--data-dir) DATA_DIR="$2";shift 2;;--update) exec /opt/mini-ubuntu-server/bin/mini-ubuntu-server update;;*) echo "Unknown option: $1" >&2;exit 2;;esac;done
[[ ${EUID} -eq 0 ]] || { echo "Run as root" >&2;exit 1; }
source /etc/os-release
[[ ${ID:-} == ubuntu ]] || { echo "Ubuntu is required" >&2;exit 1; }
case "$(dpkg --print-architecture)" in amd64|arm64) ARCH="$(dpkg --print-architecture)";;*) echo "Unsupported architecture" >&2;exit 1;;esac
apt-get update -qq; apt-get install -y -qq ca-certificates curl openssl tar
getent group mini-ubuntu-server >/dev/null || groupadd --system mini-ubuntu-server
id mini-ubuntu-server >/dev/null 2>&1 || useradd --system --gid mini-ubuntu-server --home-dir "$DATA_DIR" --shell /usr/sbin/nologin mini-ubuntu-server
install -d -o root -g mini-ubuntu-server -m 0750 /opt/mini-ubuntu-server/bin /etc/mini-ubuntu-server
install -d -o mini-ubuntu-server -g mini-ubuntu-server -m 0750 "$DATA_DIR" "$DATA_DIR/backups" /var/log/mini-ubuntu-server
if [[ "$VERSION" == latest ]]; then VERSION="$(curl -fsSL "https://api.github.com/repos/$REPOSITORY/releases/latest" | sed -n 's/.*"tag_name": "\([^"]*\)".*/\1/p')";fi
BASE="https://github.com/$REPOSITORY/releases/download/$VERSION"; ARCHIVE="mini-ubuntu-server-linux-$ARCH.tar.gz"; TMP="$(mktemp -d)";trap 'rm -rf "$TMP"' EXIT
curl -fsSL "$BASE/$ARCHIVE" -o "$TMP/$ARCHIVE";curl -fsSL "$BASE/checksums.txt" -o "$TMP/checksums.txt"
(cd "$TMP" && grep " $ARCHIVE\$" checksums.txt | sha256sum -c -)
tar -xzf "$TMP/$ARCHIVE" -C "$TMP";install -o root -g root -m 0755 "$TMP/mini-ubuntu-server" /opt/mini-ubuntu-server/bin/mini-ubuntu-server
install -o root -g root -m 0644 "$TMP/mini-ubuntu-server.service" /etc/systemd/system/mini-ubuntu-server.service
[[ -f /etc/mini-ubuntu-server/config.yml ]] || sed -e "s/:8080/:$PORT/" -e "s#/var/lib/mini-ubuntu-server#$DATA_DIR#" "$TMP/config.example.yml" > /etc/mini-ubuntu-server/config.yml
TEMP_PASSWORD="$(openssl rand -base64 24 | tr -d '\n')";JWT_SECRET="$(openssl rand -hex 32)"
install -o root -g mini-ubuntu-server -m 0640 /dev/null /etc/mini-ubuntu-server/secrets.env
printf 'MINI_UBUNTU_SERVER_JWT_SECRET=%s\nMINI_UBUNTU_SERVER_BOOTSTRAP_USERNAME=%s\nMINI_UBUNTU_SERVER_BOOTSTRAP_PASSWORD=%s\n' "$JWT_SECRET" "$ADMIN_USERNAME" "$TEMP_PASSWORD" > /etc/mini-ubuntu-server/secrets.env
chown root:mini-ubuntu-server /etc/mini-ubuntu-server/secrets.env;chmod 0640 /etc/mini-ubuntu-server/secrets.env
systemctl daemon-reload;systemctl enable --now mini-ubuntu-server.service
sleep 2;curl -fsS "http://127.0.0.1:$PORT/api/v1/health" >/dev/null
sed -i '/^MINI_UBUNTU_SERVER_BOOTSTRAP_/d' /etc/mini-ubuntu-server/secrets.env
IP="$(hostname -I | awk '{print $1}')";printf '\nMini Ubuntu Server Panel installed.\nURL: http://%s:%s\nUsername: %s\nTemporary password: %s\nChange it after first login.\n' "$IP" "$PORT" "$ADMIN_USERNAME" "$TEMP_PASSWORD"
