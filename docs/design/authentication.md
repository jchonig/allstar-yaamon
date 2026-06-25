# Authentication

Three authentication modes can coexist. Proxy auth runs first; if it produces a session the cookie middleware is skipped.

## Local login

- Password stored as a bcrypt hash in `users.password`.
- `*` sentinel disables local login (used for OAuth2-provisioned accounts).
- Failed attempts are rate-limited per username by `loginLimiter` (`internal/server/handlers.go`).
- On success, a signed session cookie is written (7-day TTL).

## Proxy auth (OAuth2 / oauth2-proxy)

Enabled by `proxy_auth.enabled: true`. Reads `X-Auth-Request-*` headers set by an upstream reverse proxy (e.g., oauth2-proxy).

- `username_header` identifies the user (`X-Auth-Request-Preferred-Username`).
- `groups_header` carries comma-separated group memberships.
- `group_roles` maps group names to YAAMon permission levels; the highest-ranked group wins.
- If the user does not exist in the DB and `create_users: true`, a new account is created with password `*`.
- If `update_db_role: true`, the DB permission is updated to match the header on every request.
- Sessions are stateless — no cookie is written.

## Tailscale auth

Enabled by `tailscale_auth.enabled: true`. Reads headers injected by `caddy-tailscale` via `header_up` directives in the Caddyfile.

- `user_header` carries the full Tailscale identity (`Tailscale-User-Login`, value like `user@tailnet`). Always configure the Caddyfile to use `{http.auth.user.tailscale_user}` (the fully-qualified form) rather than `tailscale_login` (username only) — the full ID avoids ambiguity when users from different Tailscale tailnets might share the same short username.
- The identity is looked up in `tailscale_logins`; no match → fall through to login.
- `name_header` and `avatar_header` fill the session display name and avatar URL only when the DB fields are empty.
- Sessions are stateless — no cookie is written.
- A Tailscale identity can be assigned to exactly one account (enforced by `tailscale_logins` PRIMARY KEY).

## WebAuthn / Passkey auth

Enabled when `webauthn` is configured (or by default with per-request derivation). Implements FIDO2 discoverable credentials.

- Begin: generates a challenge, stores a single-use session in `webauthn_sessions` with the ceremony's RPID and origin, sets a 5-minute HttpOnly cookie.
- Finish: fetches and deletes the session (single-use), verifies the authenticator response against the stored session using the same RPID/origin, updates `webauthn_credentials.last_used_at`, writes a full session cookie.
- RPID and origin are derived from the `Origin` request header per-ceremony (or from explicit config), stored in the DB session, and matched at Finish time — ensuring the validation works regardless of which hostname the user accesses.

## Middleware chain

```
RealIP → Logger → Recoverer → CSRF → setupGuard
    → proxyAuthMiddleware   (Tailscale first, then OAuth2; sets session in context)
    → sessions.Middleware   (reads cookie only if context has no session yet)
    → validateSessionUser   (checks DB for non-proxy sessions; clears stale cookies)
    → RequirePermission(…)  (per-route group)
```

## Sessions

Sessions are encoded as JSON, base64-encoded, and HMAC-SHA256-signed. The resulting `<payload>.<sig>` string is stored in an `HttpOnly`, `SameSite=Strict` cookie (`yaamon_session`). The HMAC key is generated once and stored in `configs.session_secret`.

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

Proxy-auth sessions (`AuthMethod != ""`) are re-derived from headers on every request and are never written to a cookie. `validateSessionUser` skips the DB existence check for proxy sessions.

## Permission levels

| Level | Capabilities |
|---|---|
| `none` | No access |
| `readonly` | View dashboard, stats, favorites |
| `readwrite` | `readonly` + connect/disconnect, manage favorites |
| `admin` | `readwrite` + manage nodes, users, backup/restore |
| `superuser` | `admin` + create/modify superuser accounts |

Only superusers can delete the last superuser account.

## Plaintext HTTP safety check

`checkPlaintextSafety` (`internal/server/plaintext_check.go`) runs at startup and refuses to serve plaintext HTTP if the server would be reachable on a public IP. The check is skipped entirely if any of the following are true:

- TLS is active (any `tls.mode` other than `""` or `"none"`)
- `proxy_auth.enabled: true` (upstream proxy handles TLS)
- `tailscale_auth.enabled: true` (traffic is confined to the Tailscale network)
- `server.allow_public_plaintext: true` (operator opt-out)

If none of those apply, `resolveBindIPs` enumerates all interfaces that the configured `bind_address` maps to (wildcard addresses expand to all interfaces). If any resolved IP falls outside private ranges (RFC 1918, loopback, link-local, ULA, RFC 6598 CGNAT / Tailscale `100.64.0.0/10`, `fd7a:115c:a1e0::/48`), the server logs a fatal error and exits.

This prevents accidental exposure of unauthenticated HTTP to the public internet.
