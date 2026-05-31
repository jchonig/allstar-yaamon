# Security

## Overview

YAAMon's security surface has two main areas:

1. **Web security** — how the web interface is protected (TLS, authentication, session cookies)
2. **AMI security** — how connections to Asterisk are secured

See the topic pages for details:

- [Web Security](web-security.md) — TLS, Let's Encrypt, passkeys, session hardening, CSRF
- [AMI Security](ami-security.md) — securing AMI connections, VPN, SSH tunnels

## Defence in depth

A well-secured YAAMon deployment uses multiple layers:

| Layer | Mechanism |
|-------|-----------|
| Transport | TLS (ACME, provided cert, or reverse proxy) |
| Authentication | Password + optional passkey / Tailscale / OAuth2 |
| Sessions | HMAC-signed HttpOnly SameSite=Strict cookies |
| CSRF | Per-request CSRF token on all state-changing requests |
| AMI | VPN or SSH tunnel; `permit`/`deny` in manager.conf |
| Container | Non-root user (uid 1000), read-only config mount |
