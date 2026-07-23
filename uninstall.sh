#!/usr/bin/env bash
set -Eeuo pipefail
[[ ${EUID} -eq 0 ]] || { echo "Run as root" >&2;exit 1; }
read -r -p "Remove application? [y/N] " answer;[[ "$answer" =~ ^[Yy]$ ]] || exit 0
systemctl disable --now mini-ubuntu-server.service 2>/dev/null || true
rm -f /etc/systemd/system/mini-ubuntu-server.service /etc/sudoers.d/mini-ubuntu-server
rm -rf /opt/mini-ubuntu-server
read -r -p "Remove config? [y/N] " answer;[[ "$answer" =~ ^[Yy]$ ]] && rm -rf /etc/mini-ubuntu-server
read -r -p "Remove SQLite and backups? [y/N] " answer;[[ "$answer" =~ ^[Yy]$ ]] && rm -rf /var/lib/mini-ubuntu-server
systemctl daemon-reload
echo "Application removed. Data was preserved unless explicitly selected."
