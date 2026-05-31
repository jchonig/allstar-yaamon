# Web Security

## TLS

YAAMon supports four TLS modes. Choose the one that fits your deployment:

| Mode | Use case |
|------|----------|
| `disabled` | Local LAN only, or TLS handled by an upstream proxy |
| `self_signed` | Quick local setup; browser warning accepted once |
| `provided` | Your own certificate (certbot, Let's Encrypt manual, etc.) |
| `acme` | Automatic Let's Encrypt; requires a public domain and port 80 |

See [Web Server — Standalone](../configuration/web-server/standalone.md) for configuration details.

### Let's Encrypt (ACME)

When `tls.mode: acme`, YAAMon handles certificate issuance and renewal automatically via the HTTP-01 ACME challenge. Requirements:

- A public domain name pointing to the YAAMon host
- Port 80 reachable from the internet (for the ACME challenge)
- `acme_cache_dir` writable by the `yaamon` user

Certificates are renewed automatically before expiry.

> **Planned**: DNS-01 ACME challenge (no port 80 required, wildcard certs) — see [issue #11](https://github.com/jchonig/allstar-yaamon/issues/11).

### Reverse proxy TLS (Caddy, Apache, nginx)

When TLS is handled by a reverse proxy, run YAAMon with `tls.mode: disabled` on a local-only port (8080 or 80 on the Docker bridge). The proxy handles TLS termination and forwards plain HTTP to YAAMon.

## Authentication

YAAMon supports three authentication methods that can coexist:

| Method | Description |
|--------|-------------|
| **Local password** | bcrypt-hashed password stored in the database |
| **Passkey (WebAuthn/FIDO2)** | FIDO2 resident key — Touch ID, YubiKey, Bitwarden, etc. |
| **Tailscale auth** | Automatic login from Tailscale identity headers |
| **OAuth2 proxy auth** | Automatic login from oauth2-proxy headers (OIDC/Kanidm) |

### Passkeys

Passkeys are FIDO2 resident credentials — phishing-resistant and not reusable across sites. See [Passkeys configuration](../configuration/passkeys.md) and [Profile — Passkeys](../user-guide/profile.md#passkeys).

### Password security

- Passwords are bcrypt-hashed (cost factor 12) — never stored in plain text.
- Failed login attempts are rate-limited per username.
- Accounts created by OAuth2 proxy auth use a `*` sentinel that disables local login.

## Session cookies

Sessions are encoded as JSON, base64-encoded, and HMAC-SHA256-signed. The resulting `<payload>.<sig>` string is stored in a cookie with:

- `HttpOnly` — not accessible to JavaScript
- `SameSite=Strict` — not sent on cross-site requests (CSRF mitigation)
- `Secure` — sent only over HTTPS (when TLS is enabled)
- 7-day TTL

The HMAC key is generated once and stored in the database (`configs.session_secret`).

## CSRF protection

All state-changing requests (POST, PUT, PATCH, DELETE) require a valid CSRF token. The token is embedded in HTML pages and sent as a request header by the JavaScript client. Requests without a valid token are rejected with 403.

## Container security

The Docker container runs as a non-root user (`yaamon`, uid 1000). The config directory (`/etc/yaamon`) is mounted read-only. The systemd unit (for `.deb` installs) runs as the `yaamon` system user with a minimal capability set.
