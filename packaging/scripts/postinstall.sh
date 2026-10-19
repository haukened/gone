#!/bin/sh
# gone-server: after install or upgrade. Reload systemd; on an upgrade,
# restart the service if it was running. A fresh install doesn't start it:
# Gone should only come up once its settings and TLS proxy are ready.
set -e
if [ -d /run/systemd/system ] && command -v systemctl >/dev/null 2>&1; then
	systemctl daemon-reload || true
	if systemctl is-enabled --quiet gone 2>/dev/null; then
		systemctl try-restart gone || true
		exit 0
	fi
fi
echo "gone-server: edit /etc/default/gone, then: systemctl enable --now gone"
