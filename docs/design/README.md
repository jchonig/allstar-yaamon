# Design

Technical documentation for YAAMon's architecture and internals.

## Package layout

| Package | Responsibility |
|---|---|
| `internal/db` | SQLite access, migrations, typed query functions |
| `internal/auth` | Password hashing, session encoding/signing, middleware |
| `internal/config` | Viper-based config loading and validation |
| `internal/server` | HTTP handlers, routing, middleware, template rendering |
| `internal/ami` | Asterisk Manager Interface client and connection manager |
| `internal/aslstats` | AllStarLink stats API fetcher |
| `internal/astdb` | AllStar node database (astdb.txt) reader and updater |
| `internal/backup` | Backup/restore, Allmon3 INI import, encryption |
| `internal/sse` | Server-Sent Events broker for live dashboard updates |
| `internal/tls` | TLS config (self-signed, provided cert, ACME) |
| `internal/state` | Node state model (connections, links) |
| `internal/mdns` | Optional mDNS announcement |
| `internal/version` | Build-time version string |

## Topics

- [AMI Interface](ami.md) — actions sent, events consumed, wire format, manager.conf requirements
- [Database](database.md) — schema, tables, migrations
- [Authentication](authentication.md) — auth modes, sessions, middleware chain, plaintext safety check
- [API](api.md) — HTTP endpoint inventory, active links enrichment
- [CI/CD](cicd.md) — GitHub Actions, release process, branch image builds
- [Testing](testing.md) — unit, integration, and end-to-end test framework

## Server configuration (`ServerConfig`)

Key fields in `server:` config (`internal/config/config.go`):

| Field | Default | Purpose |
|---|---|---|
| `http_port` | `80` | HTTP listen port |
| `https_port` | `443` | HTTPS listen port (when TLS enabled) |
| `redirect_http` | `false` | Redirect HTTP → HTTPS |
| `bind_address` | `""` (all) | Interface to bind (IP or hostname) |
| `base_path` | `""` | URL prefix when hosted under a sub-path (e.g. `/yaamon`) |
| `allow_public_plaintext` | `false` | Bypass public-IP plaintext safety check |
