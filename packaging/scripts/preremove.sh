#!/bin/sh
# gone-server: before removal. Stop and disable the service, but not on a
# Debian upgrade ("upgrade") or an rpm upgrade (a count above 0).
set -e
case "${1:-}" in
	upgrade | 1) exit 0 ;;
esac
command -v systemctl >/dev/null 2>&1 || exit 0
[ -d /run/systemd/system ] || exit 0
systemctl disable --now gone >/dev/null 2>&1 || true
