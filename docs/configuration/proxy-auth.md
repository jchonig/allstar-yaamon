# Proxy Authentication (OAuth2 / oauth2-proxy)

When YAAMon sits behind an [oauth2-proxy](https://oauth2-proxy.github.io/oauth2-proxy/) reverse proxy (or any proxy that injects `X-Auth-Request-*` headers), it can derive a session automatically from those headers without requiring users to log in through the YAAMon login page.

## How it works

On every request the proxy injects headers identifying the authenticated user. YAAMon reads the username and group membership from those headers, maps the user's groups to a YAAMon role, and establishes a stateless session — no cookie is written, the session is re-derived from headers on every request.

If the user does not yet have a YAAMon account and `create_users: true` (the default), an account is created automatically. The password is stored as `*`, which prevents local login — the account is only usable via proxy auth.

## Configuration

```yaml
proxy_auth:
  enabled: false

  # Header that carries the authenticated username.
  username_header: X-Auth-Request-Preferred-Username

  # Header that carries the comma-separated group claim values.
  groups_header: X-Auth-Request-Groups

  # Map group claim values to YAAMon roles (highest role wins).
  # MUST be configured — if empty, every proxy-auth request is denied.
  group_roles:
    yaamon_superadmin: superuser
    yaamon_admin:      admin
    yaamon_rw:         readwrite
    yaamon_access:     readonly

  # Create a DB account on first proxy-auth login (default: true).
  create_users: true

  # If true, update the DB account's stored role to the OAuth-mapped role
  # on every successful auth. If false (default), the DB role is ignored
  # while the proxy is active.
  update_db_role: false
```

## Headers injected by oauth2-proxy

| Header | Content |
|--------|---------|
| `X-Auth-Request-User` | OIDC subject / username |
| `X-Auth-Request-Preferred-Username` | Human-readable preferred username |
| `X-Auth-Request-Email` | User's email address |
| `X-Auth-Request-Groups` | Comma-separated list of group names |

Configure oauth2-proxy with `set-xauthrequest = true` to enable these headers.

## Kanidm group mapping example

| Kanidm group | YAAMon role |
|---|---|
| `yaamon_access` | `readonly` |
| `yaamon_rw` | `readwrite` |
| `yaamon_admin` | `admin` |
| `yaamon_superadmin` | `superuser` |

A user may belong to more than one group — YAAMon grants the highest role present.

## Security considerations

YAAMon trusts the proxy headers unconditionally. Ensure that:

- YAAMon is **not directly reachable** from the internet — only the proxy should be able to reach it.
- The proxy **strips** any incoming `X-Auth-Request-*` headers from external requests before adding its own.

When `proxy_auth.enabled: true`, the local login page still works for accounts without the `*` password sentinel. This allows a fallback superuser to log in directly during testing or emergencies.

## Auth indicator

When a session is established via proxy auth, a shield icon (🛡) appears next to the username in the top-right dropdown. Hover to see `Authenticated via OAuth2`.

## Troubleshooting

See [Troubleshooting — OAuth2 / oauth2-proxy checklist](../troubleshooting/README.md#checklist-for-oauth2--oauth2-proxy-auth).
