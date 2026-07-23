#!/usr/bin/env bash
set -Eeuo pipefail

BINARY="/opt/mini-ubuntu-server/bin/mini-ubuntu-server"
[[ -x "$BINARY" ]] || { echo "Mini Ubuntu Server Panel is not installed" >&2; exit 1; }
exec "$BINARY" update "$@"
