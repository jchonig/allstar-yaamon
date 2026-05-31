# Debian / Ubuntu Package

> **Port note**: ASL3 already runs Apache2 on port 80. YAAMon defaults to port 8080 to avoid the conflict. Access it at `http://<your-node-ip>:8080/` (or `http://nodeXXXXX.local:8080/`). See [web server configuration](../configuration/web-server/README.md) if you want a different port or to front it with Apache.

## Download and install

Download the `.deb` for your architecture from the [Releases page](https://github.com/jchonig/allstar-yaamon/releases/latest):

| Platform | File |
|----------|------|
| Raspberry Pi 3B+ / Zero 2 W / Pi 4 / Pi 5 | `yaamon_*_linux_arm64.deb` |
| x86-64 server / VM | `yaamon_*_linux_amd64.deb` |

```bash
# Example — replace version and arch as appropriate
wget https://github.com/jchonig/allstar-yaamon/releases/download/v1.0.0/yaamon_1.0.0_linux_arm64.deb
sudo dpkg -i yaamon_1.0.0_linux_arm64.deb
```

The package installs:
- `/usr/local/bin/yaamon` — the binary
- `/lib/systemd/system/yaamon.service` — systemd unit
- `/etc/yaamon/config.yaml` — starter configuration (not overwritten on upgrade)

The service starts automatically on install.

## Verify

```bash
sudo systemctl status yaamon
sudo journalctl -u yaamon -f
```

## Systemd quick reference

```bash
sudo systemctl start yaamon
sudo systemctl stop yaamon
sudo systemctl restart yaamon
sudo systemctl status yaamon
sudo journalctl -u yaamon -f          # live logs
sudo journalctl -u yaamon --since today
```

## Configuration

Edit `/etc/yaamon/config.yaml` then restart:

```bash
sudo systemctl restart yaamon
```

See [Configuration](../configuration/README.md) for all available settings.
