# YAAMon Design

## Database

YAAMon uses a single SQLite file (WAL mode, `PRAGMA foreign_keys = ON`). All
schema changes are applied at startup through a versioned migration table.
Each migration runs inside its own transaction together with the
`schema_version` insert, so a failed migration is rolled back cleanly and
retried on the next start. Complex migrations that cannot be expressed as a
single SQL string use a Go `migrationFn` instead.

### Tables

#### `schema_version`
Tracks applied migrations. `migrate()` in `internal/db/db.go` runs each
migration whose version exceeds `MAX(version)`.

| Column | Type | Notes |
|---|---|---|
| `version` | INTEGER PK | Migration number |
| `applied_at` | DATETIME | When it was applied |

#### `nodes`
One row per AllStar node managed by this instance.

| Column | Type | Notes |
|---|---|---|
| `id` | INTEGER PK AUTOINCREMENT | |
| `name` | TEXT NOT NULL | Display name |
| `node_number` | TEXT NOT NULL | AllStar node number |
| `ami_host` | TEXT | Default `localhost` |
| `ami_port` | INTEGER | Default `5038` |
| `ami_user` | TEXT | AMI username |
| `ami_pass` | TEXT | AMI password (stored in plaintext) |
| `enabled` | INTEGER | `1` = active, `0` = disabled |
| `description` | TEXT | Free-form |
| `location` | TEXT | Free-form |
| `created_at` | DATETIME | |

#### `favorites`
Saved node numbers (quick-connect shortcuts) scoped to a parent node.
`position` drives the user-visible sort order and is maintained by the
reorder API; `sort_order` is a secondary sort within groups.

| Column | Type | Notes |
|---|---|---|
| `id` | INTEGER PK AUTOINCREMENT | |
| `node_id` | INTEGER NOT NULL | FK → `nodes(id)` ON DELETE CASCADE |
| `node_number` | TEXT NOT NULL | AllStar target node |
| `callsign` | TEXT | |
| `description` | TEXT | |
| `location` | TEXT | |
| `cmd` | TEXT | AMI command to issue on connect |
| `sort_order` | INTEGER | Secondary sort within a group |
| `group_name` | TEXT | Default `default` |
| `position` | INTEGER | Primary display order |
| `created_at` | DATETIME | |

#### `users`
Local user accounts. Password is a bcrypt hash; `*` means local login is
disabled (OAuth2/proxy-created accounts).

| Column | Type | Notes |
|---|---|---|
| `id` | INTEGER PK AUTOINCREMENT | |
| `username` | TEXT UNIQUE NOT NULL | |
| `password` | TEXT NOT NULL | bcrypt hash or `*` |
| `permission` | TEXT NOT NULL | `superuser`, `admin`, `readwrite`, `readonly`, `none` |
| `full_name` | TEXT | Display name |
| `avatar_url` | TEXT | External URL or `/api/users/{id}/avatar` |
| `created_at` | DATETIME | |

#### `tailscale_logins`
Maps Tailscale user identities to local accounts. The PRIMARY KEY on `login`
enforces that each Tailscale identity belongs to at most one account — the DB
constraint replaces any application-level duplicate check. ON DELETE CASCADE
removes entries when the user is deleted.

| Column | Type | Notes |
|---|---|---|
| `login` | TEXT PRIMARY KEY | Full Tailscale identity (`user@tailnet`) |
| `user_id` | INTEGER NOT NULL | FK → `users(id)` ON DELETE CASCADE |

#### `configs`
Key-value store for runtime settings (session secret, uploaded avatar data,
custom favicon, etc.).

| Column | Type | Notes |
|---|---|---|
| `key` | TEXT PRIMARY KEY | |
| `value` | TEXT | |
| `updated_at` | DATETIME | |

Known config keys:

| Key | Contents |
|---|---|
| `session_secret` | Base64-encoded 32-byte HMAC key |
| `user_avatar_{id}` | Base64-encoded raw image bytes for uploaded avatars |
| `favicon` / `favicon-256.png` | Base64-encoded custom favicon images |

#### `stats_cache`
Persists the last-known ASL stats for each node across server restarts. On startup the
cache is pre-loaded from this table so the UI shows values immediately while fresh data
is fetched in the background. Only successful (non-error) fetch results are stored. Rows
whose `node_number` no longer appears in `nodes` or `favorites` are pruned at startup.

| Column | Type | Notes |
|---|---|---|
| `node_number` | TEXT PRIMARY KEY | AllStar node number |
| `stats_json` | TEXT NOT NULL | JSON-encoded `NodeStats` |
| `updated_at` | DATETIME | Last successful fetch time |

#### `tls_certs`
Stores a self-signed TLS certificate and private key generated at first start
when `tls.mode = self_signed`.

| Column | Type | Notes |
|---|---|---|
| `id` | INTEGER PK | Always 1 |
| `cert_pem` | TEXT | PEM certificate |
| `key_pem` | TEXT | PEM private key |
| `generated_at` | DATETIME | |

### Relationships

```
nodes ──< favorites
users ──< tailscale_logins
nodes, favorites ──> stats_cache (pruned by cross-reference)
```

### Migration history

| Version | Change |
|---|---|
| 1 | Initial schema: `nodes`, `favorites`, `users`, `configs`, `tls_certs` |
| 2 | Add `favorites.position`; seed from `id` |
| 3 | Add `nodes.description`, `nodes.location` |
| 4 | Add `users.full_name`, `users.avatar_url` |
| 5 | Add `users.tailscale_usernames` (comma-separated — superseded by migration 6) |
| 6 | Create `tailscale_logins` join table; migrate comma-separated data; drop `tailscale_usernames` column |
| 7 | Add `stats_cache` table for persisting ASL stats across server restarts |

---

## Authentication

Three authentication modes can coexist. Proxy auth runs first; if it
produces a session the cookie middleware is skipped.

### Local login

- Password stored as a bcrypt hash in `users.password`.
- `*` sentinel disables local login (used for OAuth2-provisioned accounts).
- Failed attempts are rate-limited per username by `loginLimiter`
  (`internal/server/handlers.go`).
- On success, a signed session cookie is written (7-day TTL).

### Proxy auth (OAuth2 / oauth2-proxy)

Enabled by `proxy_auth.enabled: true`. Reads `X-Auth-Request-*` headers set
by an upstream reverse proxy (e.g., oauth2-proxy).

- `username_header` identifies the user (`X-Auth-Request-Preferred-Username`).
- `groups_header` carries comma-separated group memberships.
- `group_roles` maps group names to YAAMon permission levels; the
  highest-ranked group wins.
- If the user does not exist in the DB and `create_users: true`, a new account
  is created with password `*`.
- If `update_db_role: true`, the DB permission is updated to match the header
  on every request.
- Sessions are stateless — no cookie is written.

### Tailscale auth

Enabled by `tailscale_auth.enabled: true`. Reads headers injected by
`caddy-tailscale` via `header_up` directives in the Caddyfile.

- `user_header` carries the full Tailscale identity
  (`Tailscale-User-Login`, value like `user@tailnet`). Always configure
  the Caddyfile to use `{http.auth.user.tailscale_user}` (the fully-qualified
  form) rather than `tailscale_login` (username only).
- The identity is looked up in `tailscale_logins`; no match → fall through to
  login.
- `name_header` and `avatar_header` fill the session display name and avatar
  URL only when the DB fields are empty.
- Sessions are stateless — no cookie is written.
- A Tailscale identity can be assigned to exactly one account (enforced by
  `tailscale_logins` PRIMARY KEY).

### Middleware chain

```
RealIP → Logger → Recoverer → CSRF → setupGuard
    → proxyAuthMiddleware   (sets session in context; Tailscale first, then OAuth2)
    → sessions.Middleware   (reads cookie only if context has no session yet)
    → validateSessionUser   (checks DB for non-proxy sessions; clears stale cookies)
    → RequirePermission(…)  (per-route group)
```

---

## Sessions

Sessions are encoded as JSON, base64-encoded, and HMAC-SHA256-signed.  The
resulting `<payload>.<sig>` string is stored in an `HttpOnly`, `SameSite=Strict`
cookie (`yaamon_session`). The HMAC key is generated once and stored in
`configs.session_secret`.

Session fields:

| JSON key | Go field | Notes |
|---|---|---|
| `uid` | `UserID` | |
| `u` | `Username` | |
| `p` | `Permission` | |
| `fn` | `FullName` | omitempty |
| `av` | `AvatarURL` | omitempty |
| `exp` | `Expires` | Unix timestamp |
| `am` | `AuthMethod` | `""` = local, `"tailscale"`, `"OAuth2"` |
| `eu` | `ExternalUsername` | Tailscale login when it differs from username |

Proxy-auth sessions (`AuthMethod != ""`) are re-derived from headers on every
request and are never written to a cookie. `validateSessionUser` skips the
DB existence check for proxy sessions.

---

## Permission levels

Ranked from lowest to highest:

| Level | Capabilities |
|---|---|
| `none` | No access |
| `readonly` | View dashboard, stats, favorites |
| `readwrite` | `readonly` + connect/disconnect, manage favorites |
| `admin` | `readwrite` + manage nodes, users, backup/restore |
| `superuser` | `admin` + create/modify superuser accounts |

Only superusers can delete the last superuser account.

---

## Avatar storage

Uploaded avatars are stored base64-encoded in `configs` under key
`user_avatar_{id}`. `GET /api/users/{id}/avatar` decodes and serves the
image with `Content-Type` derived from the magic bytes (PNG, JPEG, GIF,
WebP). When avatar data is present, `effectiveAvatarURL` returns the
`/api/users/{id}/avatar` path; otherwise it returns `users.avatar_url`
(which may be an external URL set in the profile).

---

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
