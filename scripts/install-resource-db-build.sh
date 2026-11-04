#!/usr/bin/env bash
# Install the reviewed local resources/database build; preserve config and panel data.
set -Eeuo pipefail
[[ ${EUID} -eq 0 ]] || { echo 'Run this script with sudo.' >&2; exit 1; }
PROJECT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
DIST_DIR="$PROJECT_DIR/dist"
BINARY=/opt/mini-ubuntu-server/bin/mini-ubuntu-server
[[ -x "$BINARY" && -f "$DIST_DIR/mini-ubuntu-server-resources-db" ]] || { echo 'Installed panel or new build is missing.' >&2; exit 1; }
(cd "$DIST_DIR" && sha256sum -c resources-db.sha256)
"$DIST_DIR/mini-ubuntu-server-resources-db" --version
BACKUP_DIR="/var/backups/mini-ubuntu-server/resources-db-$(date -u +%Y%m%dT%H%M%SZ)"
install -d -m 0700 "$BACKUP_DIR"
cp -a -- "$BINARY" "$BACKUP_DIR/mini-ubuntu-server"
cp -a -- /etc/mini-ubuntu-server "$BACKUP_DIR/config"
stopped=0
rollback() {
  local code=$?
  trap - ERR
  if [[ "$stopped" -eq 1 ]]; then
    install -o root -g root -m 0755 "$BACKUP_DIR/mini-ubuntu-server" "$BINARY.rollback"
    mv -f -- "$BINARY.rollback" "$BINARY"
    systemctl restart mini-ubuntu-server.service || true
  fi
  echo "Update failed; previous binary restored. Backup: $BACKUP_DIR" >&2
  exit "$code"
}
trap rollback ERR
systemctl stop mini-ubuntu-server.service
stopped=1
tar --exclude=./backups -czf "$BACKUP_DIR/data.tar.gz" -C /var/lib/mini-ubuntu-server .
install -o root -g root -m 0755 "$DIST_DIR/mini-ubuntu-server-resources-db" "$BINARY.new"
mv -f -- "$BINARY.new" "$BINARY"
systemctl start mini-ubuntu-server.service
healthy=0
for attempt in {1..20}; do
  if curl --max-time 2 -fsS http://127.0.0.1:8080/api/v1/health | grep -Fq 'v0.0.0-local.resources-db.20261102'; then healthy=1; break; fi
  sleep 1
done
[[ "$healthy" -eq 1 ]]
systemctl is-active --quiet mini-ubuntu-server.service
trap - ERR
if [[ -f /etc/sudoers.d/99-shrk-panel-update ]] && grep -Fxq 'shrk ALL=(root) NOPASSWD: ALL' /etc/sudoers.d/99-shrk-panel-update; then
  rm -f -- /etc/sudoers.d/99-shrk-panel-update
fi
echo "Panel updated successfully. Backup: $BACKUP_DIR"
