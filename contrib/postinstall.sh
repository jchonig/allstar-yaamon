#!/bin/bash
set -e

if ! id -u yaamon >/dev/null 2>&1; then
    useradd --system --no-create-home --shell /usr/sbin/nologin yaamon
fi

mkdir -p /etc/yaamon /var/lib/yaamon
chown yaamon:yaamon /etc/yaamon /var/lib/yaamon

if [ -d /run/systemd/system ]; then
    systemctl daemon-reload
    systemctl enable yaamon
    systemctl start yaamon || true
    echo ""
    echo "YAAMon installed and started."
    echo "Open http://$(hostname).local/ to create your admin account."
else
    echo ""
    echo "YAAMon installed. Start it with: yaamon serve"
fi
