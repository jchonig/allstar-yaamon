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
    echo "Open http://$(hostname).local:8080/ to create your admin account."
    echo ""
    echo "To expose YAAMon via Apache reverse proxy, see:"
    echo "  /usr/share/doc/yaamon/apache2-yaamon.conf"
else
    echo ""
    echo "YAAMon installed. Start it with: yaamon serve"
fi
