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
- [Authentication](authentication.md) — auth modes, sessions, middleware chain
- [API](api.md) — HTTP endpoint inventory
- [CI/CD](cicd.md) — GitHub Actions, release process, branch image builds
- [Testing](testing.md) — unit, integration, and end-to-end test framework
