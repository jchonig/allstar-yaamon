# Configuration

YAAMon is configured via a single YAML file, typically `/etc/yaamon/config.yaml`.

## File format

```yaml
server:
  http_port: 8080
  https_port: 443
  redirect_http: true

tls:
  mode: disabled   # disabled | self_signed | provided | acme

db:
  path: /var/lib/yaamon/yaamon.db

astdb:
  path: /var/lib/asterisk/astdb.txt
  update: true

log:
  level: info      # debug | info | warn | error

ui:
  footer_text: "Yet Another Allstarlink MONitor (and favorites)"
  footer_url: ""
  footer_attribution: N2VLV
  footer_attribution_url: https://n2vlv.net
```

A fully commented example is included as [`examples/config.yaml.example`](../../examples/config.yaml.example) in the repository and every release tarball.

## Environment variable overrides

Any config value can be overridden with an environment variable using the pattern `YAAMON_<SECTION>_<KEY>` (uppercase, underscores):

| Config key | Environment variable |
|---|---|
| `db.path` | `YAAMON_DB_PATH` |
| `log.level` | `YAAMON_LOG_LEVEL` |
| `server.http_port` | `YAAMON_SERVER_HTTP_PORT` |
| `astdb.path` | `YAAMON_ASTDB_PATH` |
| `astdb.update` | `YAAMON_ASTDB_UPDATE` |

Environment variables take precedence over the config file.

## Full reference

| Section | Key | Default | Description |
|---------|-----|---------|-------------|
| `server` | `http_port` | `8080` | HTTP listen port |
| `server` | `https_port` | `443` | HTTPS listen port |
| `server` | `redirect_http` | `true` | 301 redirect HTTP → HTTPS when TLS is enabled |
| `server` | `allow_public_plaintext` | `false` | Allow plaintext HTTP on public (non-RFC-1918) addresses. YAAMon refuses to start if `tls.mode: disabled` and the bound address is publicly routable. Set to `true` only if you have an external security layer and understand the risk. |
| `tls` | `mode` | `disabled` | TLS mode: `disabled`, `self_signed`, `provided`, `acme` |
| `tls` | `cert_file` | — | PEM certificate path (required for `mode: provided`) |
| `tls` | `key_file` | — | PEM private key path (required for `mode: provided`) |
| `tls` | `acme_domain` | — | Domain for Let's Encrypt (required for `mode: acme`) |
| `tls` | `acme_cache_dir` | `/etc/yaamon/acme` | ACME cache directory |
| `db` | `path` | `/var/lib/yaamon/yaamon.db` | SQLite database path |
| `astdb` | `path` | `/var/lib/asterisk/astdb.txt` | AllStarLink node database path |
| `astdb` | `update` | `true` | Download/refresh the node database automatically |
| `log` | `level` | `info` | Log level: `debug`, `info`, `warn`, `error` |
| `ui` | `footer_text` | (YAAMon name) | Left footer label |
| `ui` | `footer_url` | — | URL for the footer label link |
| `ui` | `footer_attribution` | `N2VLV` | Right footer attribution name |
| `ui` | `footer_attribution_url` | `https://n2vlv.net` | Right footer attribution URL |
| `commands` | `commands` | (5 defaults) | List of node commands in the Functions menu. See [Node Commands](commands.md). |

## Configuration topics

- [Web Server](web-server/README.md) — ports, TLS, reverse proxy setup
- [AMI Configuration](ami.md) — Asterisk Manager Interface
- [AllStarLink Node Database](astdb.md) — astdb.txt path and auto-update
- [Passkeys](passkeys.md) — WebAuthn/FIDO2 RPID and origins
- [Tailscale Authentication](tailscale.md) — header-based Tailscale login
- [Proxy Authentication](proxy-auth.md) — OAuth2 / oauth2-proxy header auth
- [Declarative State](declarative-state.md) — `yaamon apply` state files
- [Node Commands](commands.md) — Functions menu, custom commands, role filtering

## mDNS

YAAMon optionally announces itself via mDNS (Bonjour/Avahi) so it is discoverable on the local network as `yaamon.local`. This is enabled automatically when the binary is built with mDNS support and the host supports it. No configuration is required.
