# Database

YAAMon uses a single SQLite file (WAL mode, `PRAGMA foreign_keys = ON`). All schema changes are applied at startup through a versioned migration table. Each migration runs inside its own transaction together with the `schema_version` insert, so a failed migration is rolled back cleanly and retried on the next start. Complex migrations that cannot be expressed as a single SQL string use a Go `migrationFn` instead.

## Tables

### `schema_version`

Tracks applied migrations. `migrate()` in `internal/db/db.go` runs each migration whose version exceeds `MAX(version)`.

| Column | Type | Notes |
|---|---|---|
| `version` | INTEGER PK | Migration number |
| `applied_at` | DATETIME | When it was applied |

### `nodes`

One row per AllStar node managed by this instance.

| Column | Type | Notes |
|---|---|---|
| `id` | INTEGER PK AUTOINCREMENT | |
| `name` | TEXT NOT NULL | Display name |
| `node_number` | TEXT NOT NULL | AllStar node number |
| `ami_host` | TEXT | Default `localhost` |
| `ami_port` | INTEGER | Default `5038` |
| `ami_user` | TEXT | AMI username |
| `ami_pass` | TEXT | AMI password (plaintext — secured by AMI-level access control) |
| `enabled` | INTEGER | `1` = active, `0` = disabled |
| `description` | TEXT | Free-form |
| `location` | TEXT | Free-form |
| `created_at` | DATETIME | |

### `favorites`

Saved node numbers (quick-connect shortcuts) scoped to a parent node. `position` drives the user-visible sort order; `sort_order` is a secondary sort within groups.

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

### `users`

Local user accounts. Password is a bcrypt hash; `*` means local login is disabled (OAuth2/proxy-created accounts).

| Column | Type | Notes |
|---|---|---|
| `id` | INTEGER PK AUTOINCREMENT | |
| `username` | TEXT UNIQUE NOT NULL | |
| `password` | TEXT NOT NULL | bcrypt hash or `*` |
| `permission` | TEXT NOT NULL | `superuser`, `admin`, `readwrite`, `readonly`, `none` |
| `full_name` | TEXT | Display name |
| `avatar_url` | TEXT | External URL or `/api/users/{id}/avatar` |
| `webauthn_id` | BLOB | 64-byte stable WebAuthn user handle |
| `created_at` | DATETIME | |

### `tailscale_logins`

Maps Tailscale user identities to local accounts. The PRIMARY KEY on `login` enforces that each Tailscale identity belongs to at most one account. ON DELETE CASCADE removes entries when the user is deleted.

| Column | Type | Notes |
|---|---|---|
| `login` | TEXT PRIMARY KEY | Full Tailscale identity (`user@tailnet`) |
| `user_id` | INTEGER NOT NULL | FK → `users(id)` ON DELETE CASCADE |

### `webauthn_credentials`

FIDO2 passkey credentials for each user.

| Column | Type | Notes |
|---|---|---|
| `id` | INTEGER PK AUTOINCREMENT | |
| `user_id` | INTEGER NOT NULL | FK → `users(id)` ON DELETE CASCADE |
| `credential_id` | BLOB UNIQUE NOT NULL | WebAuthn credential ID |
| `name` | TEXT NOT NULL | User-assigned name (e.g. "MacBook Pro Touch ID") |
| `credential_json` | TEXT NOT NULL | Serialised `webauthn.Credential` |
| `created_at` | DATETIME | |
| `last_used_at` | DATETIME | Updated on each successful authentication |

### `webauthn_sessions`

Single-use ceremony sessions for WebAuthn Begin/Finish flows (login and registration). Stored as Unix integer timestamps to avoid SQLite datetime string format ambiguity.

| Column | Type | Notes |
|---|---|---|
| `session_id` | TEXT PRIMARY KEY | Random session ID (stored in HttpOnly cookie) |
| `ceremony` | TEXT NOT NULL | `login` or `registration` |
| `user_id` | INTEGER | NULL for discoverable login |
| `session_json` | TEXT NOT NULL | Serialised `webauthn.SessionData` |
| `expires_at` | INTEGER NOT NULL | Unix timestamp (5-minute TTL) |
| `rpid` | TEXT NOT NULL | Relying Party ID at ceremony start |
| `origin` | TEXT NOT NULL | Origin at ceremony start |

### `configs`

Key-value store for runtime settings.

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
| `qrz_username` | QRZ.com account username |
| `qrz_password_enc` | AES-256-GCM encrypted QRZ password (key derived from session secret) |

### `stats_cache`

Persists the last-known ASL stats for each node across server restarts. Rows are pruned at startup for node numbers not referenced by any node or favorite.

| Column | Type | Notes |
|---|---|---|
| `node_number` | TEXT PRIMARY KEY | AllStar node number |
| `stats_json` | TEXT NOT NULL | JSON-encoded `NodeStats` |
| `updated_at` | DATETIME | Last successful fetch time |

### `qrz_cache`

Persists callsign records fetched from QRZ.com. Records are valid for 30 days.

| Column | Type | Notes |
|---|---|---|
| `callsign` | TEXT PRIMARY KEY | Amateur radio callsign (uppercase) |
| `record_json` | TEXT NOT NULL | JSON-encoded `qrz.Record` |
| `fetched_at` | DATETIME NOT NULL | When the QRZ API returned this record |

### `tls_certs`

Stores a self-signed TLS certificate and private key generated at first start when `tls.mode = self_signed`.

| Column | Type | Notes |
|---|---|---|
| `id` | INTEGER PK | Always 1 |
| `cert_pem` | TEXT | PEM certificate |
| `key_pem` | TEXT | PEM private key |
| `generated_at` | DATETIME | |

## Relationships

```
nodes ──< favorites
users ──< tailscale_logins
users ──< webauthn_credentials
nodes, favorites ──> stats_cache (pruned by cross-reference)
```

## Migration history

| Version | Change |
|---|---|
| 1 | Initial schema: `nodes`, `favorites`, `users`, `configs`, `tls_certs` |
| 2 | Add `favorites.position`; seed from `id` |
| 3 | Add `nodes.description`, `nodes.location` |
| 4 | Add `users.full_name`, `users.avatar_url` |
| 5 | Add `users.tailscale_usernames` (comma-separated — superseded by migration 6) |
| 6 | Create `tailscale_logins` join table; migrate comma-separated data; drop `tailscale_usernames` column |
| 7 | Add `stats_cache` table |
| 8 | Add `qrz_cache` table |
| 9 | Add `users.qrz_username`, `users.qrz_password_enc`, `users.lookup_source` |
| 10 | Add `users.webauthn_id`; create `webauthn_credentials` and `webauthn_sessions` |
| 11 | Add `webauthn_sessions.rpid` and `webauthn_sessions.origin` |
