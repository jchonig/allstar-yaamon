# YAAMon APT Repository

This is the YAAMon Debian/Ubuntu APT package repository.

It is intended to be used with `apt`, not browsed directly. See the
[installation instructions](../installation/deb.md) to add it to your system.

## Quick setup

```bash
curl -fsSL https://yaamon.n2vlv.net/debian/gpg.key \
  | sudo gpg --no-default-keyring \
      --keyring gnupg-ring:/usr/share/keyrings/yaamon-archive-keyring.gpg \
      --import
sudo chmod 644 /usr/share/keyrings/yaamon-archive-keyring.gpg
echo "deb [signed-by=/usr/share/keyrings/yaamon-archive-keyring.gpg] https://yaamon.n2vlv.net/debian stable main" \
  | sudo tee /etc/apt/sources.list.d/yaamon.list
sudo apt update && sudo apt install yaamon
```
