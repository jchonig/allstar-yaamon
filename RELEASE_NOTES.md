## What's new

- **Default port changed to 8080** — avoids conflict with ASL3's Apache on port 80. Existing installs with an explicit `http_port` in `config.yaml` are unaffected.
- **Apache reverse-proxy documentation** — new "Changing the port" section in the README covers running on port 80, fronting with Apache, and Docker port mapping.
- **`ami_user` defaults to `admin`** — when creating a node without specifying `ami_user`, it now defaults to `admin` (matching `ami_host` → `localhost` and `ami_port` → `5038`) rather than returning an error.
