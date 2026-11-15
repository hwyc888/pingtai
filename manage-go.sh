#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")" && pwd)"
COMMAND="start"
USERNAME="admin"
if [ "$#" -gt 0 ]; then COMMAND="$1"; fi
if [ "$#" -gt 1 ]; then USERNAME="$2"; fi

case "$COMMAND" in
  start)
    cd "$ROOT"
    exec ./pingtai-linux-amd64
    ;;
  create-admin)
    cd "$ROOT"
    exec ./pingtai-linux-amd64 create-admin --username "$USERNAME"
    ;;
  *)
    echo "Usage: $0 {start|create-admin [username]}"
    exit 2
    ;;
esac
