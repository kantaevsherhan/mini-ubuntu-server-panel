#!/usr/bin/env bash
set -Eeuo pipefail

REPOSITORY="${MINI_UBUNTU_SERVER_REPOSITORY:-kantaevsherhan/mini-ubuntu-server-panel}"
VERSION="latest"
PORT="8080"
ADMIN_USERNAME="admin"
DATA_DIR="/var/lib/mini-ubuntu-server"
ENABLE_DOCKER=0
UPDATE=0

while [[ $# -gt 0 ]]; do
  case "$1" in
    --version) VERSION="$2"; shift 2 ;;
    --port) PORT="$2"; shift 2 ;;
    --username) ADMIN_USERNAME="$2"; shift 2 ;;
    --data-dir) DATA_DIR="$2"; shift 2 ;;
    --enable-docker) ENABLE_DOCKER=1; shift ;;
    --update) UPDATE=1; shift ;;
    *) echo "Unknown option: $1" >&2; exit 2 ;;
  esac
done

[[ ${EUID} -eq 0 ]] || { echo "Run as root" >&2; exit 1; }
[[ "$PORT" =~ ^[0-9]+$ ]] && ((PORT >= 1 && PORT <= 65535)) || { echo "Invalid --port" >&2; exit 2; }
[[ "$ADMIN_USERNAME" =~ ^[a-z_][a-z0-9_-]{2,31}$ ]] || { echo "Invalid --username" >&2; exit 2; }
[[ "$DATA_DIR" == /* && "$DATA_DIR" != "/" && "$DATA_DIR" =~ ^[A-Za-z0-9/._-]+$ ]] || { echo "Invalid --data-dir" >&2; exit 2; }
[[ "$VERSION" == latest || "$VERSION" =~ ^v[0-9]+\.[0-9]+\.[0-9]+([.-][0-9A-Za-z.-]+)?$ ]] || { echo "Invalid --version" >&2; exit 2; }

if [[ "$UPDATE" -eq 1 ]]; then
  BINARY="/opt/mini-ubuntu-server/bin/mini-ubuntu-server"
  [[ -x "$BINARY" ]] || { echo "Mini Ubuntu Server Panel is not installed" >&2; exit 1; }
  if [[ "$VERSION" == "latest" ]]; then
    exec "$BINARY" update
  fi
  exec "$BINARY" update --version "$VERSION"
fi
source /etc/os-release
[[ ${ID:-} == ubuntu ]] || { echo "Ubuntu is required" >&2; exit 1; }
case "$(dpkg --print-architecture)" in
  amd64 | arm64) ARCH="$(dpkg --print-architecture)" ;;
  *) echo "Unsupported architecture" >&2; exit 1 ;;
esac

apt-get update -qq
apt-get install -y -qq ca-certificates curl openssl sudo tar ufw
getent group mini-ubuntu-server >/dev/null || groupadd --system mini-ubuntu-server
id mini-ubuntu-server >/dev/null 2>&1 || useradd --system --gid mini-ubuntu-server --home-dir "$DATA_DIR" --shell /usr/sbin/nologin mini-ubuntu-server
if [[ "$ENABLE_DOCKER" -eq 1 ]]; then
  getent group docker >/dev/null || { echo "Docker group does not exist; install Docker Engine first" >&2; exit 1; }
  usermod -aG docker mini-ubuntu-server
  echo "Warning: Docker socket access is root-equivalent and was explicitly enabled for mini-ubuntu-server." >&2
fi
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

visudo -cf "$TMP_DIR/mini-ubuntu-server.sudoers" >/dev/null
# Stop a running instance before replacing its binary (re-install over an existing panel).
systemctl stop mini-ubuntu-server.service 2>/dev/null || true
install -o root -g root -m 0755 "$TMP_DIR/mini-ubuntu-server" /opt/mini-ubuntu-server/bin/mini-ubuntu-server
ln -sfn /opt/mini-ubuntu-server/bin/mini-ubuntu-server /usr/local/bin/mini-ubuntu-server
install -o root -g root -m 0644 "$TMP_DIR/mini-ubuntu-server.service" /etc/systemd/system/mini-ubuntu-server.service
install -o root -g root -m 0440 "$TMP_DIR/mini-ubuntu-server.sudoers" /etc/sudoers.d/mini-ubuntu-server
if [[ ! -f /etc/mini-ubuntu-server/config.yml ]]; then
  sed -e "s/:8080/:$PORT/" -e "s#/var/lib/mini-ubuntu-server#$DATA_DIR#" "$TMP_DIR/config.example.yml" > /etc/mini-ubuntu-server/config.yml
fi

SECRETS=/etc/mini-ubuntu-server/secrets.env
TEMP_PASSWORD=""
# Re-running the installer must keep the JWT secret (active sessions) and the Telegram token.
if [[ ! -f "$SECRETS" ]] || ! grep -q '^MINI_UBUNTU_SERVER_JWT_SECRET=' "$SECRETS"; then
  [[ -f "$SECRETS" ]] || install -o root -g mini-ubuntu-server -m 0640 /dev/null "$SECRETS"
  printf 'MINI_UBUNTU_SERVER_JWT_SECRET=%s\n' "$(openssl rand -hex 32)" >> "$SECRETS"
fi
# The bootstrap admin is created only for an empty database, so only then is a password generated.
if [[ ! -f "$DATA_DIR/mini-ubuntu-server.db" ]]; then
  TEMP_PASSWORD="$(openssl rand -base64 24 | tr -d '\n')"
  printf 'MINI_UBUNTU_SERVER_BOOTSTRAP_USERNAME=%s\nMINI_UBUNTU_SERVER_BOOTSTRAP_PASSWORD=%s\n' "$ADMIN_USERNAME" "$TEMP_PASSWORD" >> "$SECRETS"
fi
chown root:mini-ubuntu-server "$SECRETS"
chmod 0640 "$SECRETS"

systemctl daemon-reload
systemctl enable mini-ubuntu-server.service
systemctl restart mini-ubuntu-server.service
HEALTHY=0
for _ in $(seq 1 30); do
  if curl -fsS "http://127.0.0.1:$PORT/api/v1/health" >/dev/null 2>&1; then
    HEALTHY=1
    break
  fi
  sleep 1
done
sed -i '/^MINI_UBUNTU_SERVER_BOOTSTRAP_/d' "$SECRETS"
if [[ "$HEALTHY" -ne 1 ]]; then
  echo "Service did not become healthy; see: journalctl -u mini-ubuntu-server -n 100" >&2
  exit 1
fi

IP="$(hostname -I | awk '{print $1}')"
printf '\nMini Ubuntu Server Panel installed.\nURL: http://%s:%s\n' "$IP" "$PORT"
if [[ -n "$TEMP_PASSWORD" ]]; then
  printf 'Username: %s\nTemporary password: %s\nChange it after first login.\n' "$ADMIN_USERNAME" "$TEMP_PASSWORD"
else
  printf 'Existing database, users and secrets were kept.\n'
fi
