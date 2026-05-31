# Pre-built Binary (Tarball)

Download the tarball for your platform from the [Releases page](https://github.com/jchonig/allstar-yaamon/releases/latest):

| Platform | File |
|----------|------|
| Raspberry Pi / ARM64 | `yaamon_*_linux_arm64.tar.gz` |
| x86-64 | `yaamon_*_linux_amd64.tar.gz` |

```bash
tar xzf yaamon_*_linux_arm64.tar.gz
sudo mv yaamon /usr/local/bin/
```

## Running manually

```bash
yaamon serve --config /etc/yaamon/config.yaml
```

## Persistent operation with systemd

Copy the bundled unit file and enable it:

```bash
sudo cp contrib/yaamon.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now yaamon
```

## Systemd quick reference

```bash
sudo systemctl start yaamon
sudo systemctl stop yaamon
sudo systemctl restart yaamon
sudo systemctl status yaamon
sudo journalctl -u yaamon -f
```

## Configuration

Create `/etc/yaamon/config.yaml` (see [Configuration](../configuration/README.md)). The tarball includes a `config.yaml.example` as a starting point.
