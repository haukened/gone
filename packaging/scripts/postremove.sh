#!/bin/sh
# gone-server: after removal. Reload systemd. The data in /var/lib/gone is
# kept; delete it by hand if you no longer need it.
set -e
command -v systemctl >/dev/null 2>&1 || exit 0
[ -d /run/systemd/system ] || exit 0
systemctl daemon-reload || true
