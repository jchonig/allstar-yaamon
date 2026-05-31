# Behind Caddy

Caddy is the recommended reverse proxy for YAAMon when you need automatic TLS, Tailscale authentication, or OAuth2/OIDC integration.

## Basic reverse proxy with automatic TLS

```caddyfile
yaamon.example.com {
    reverse_proxy yaamon:80
}
```

Caddy obtains a Let's Encrypt certificate automatically. YAAMon runs with TLS disabled on port 80 (Docker bridge, not reachable directly):

```yaml
# config.yaml
tls:
  mode: disabled
server:
  http_port: 80
  redirect_http: false
```

## With Tailscale authentication

Use [caddy-tailscale](https://github.com/tailscale/caddy-tailscale) to authenticate users on your Tailscale network automatically:

```caddyfile
https://yaamon.example.ts.net {
    bind tailscale/yaamon

    tailscale_auth

    reverse_proxy yaamon:80 {
        header_up Tailscale-User-Login       {http.auth.user.tailscale_user}
        header_up Tailscale-User-Name        {http.auth.user.tailscale_name}
        header_up Tailscale-User-Profile-Pic {http.auth.user.tailscale_profile_picture}
    }
}
```

> Use `tailscale_user` (not `tailscale_login`) — it is the fully-qualified identity (e.g. `jchonig@github`) which avoids ambiguity across tailnets.

Enable Tailscale auth in YAAMon:

```yaml
tailscale_auth:
  enabled: true
  user_header:   Tailscale-User-Login
  name_header:   Tailscale-User-Name
  avatar_header: Tailscale-User-Profile-Pic
```

See [Tailscale Authentication](../tailscale.md) for the full setup guide.

## With oauth2-proxy (OIDC / Kanidm)

Use an `oauth2-proxy` sidecar for full OIDC authentication:

```
Internet
    │
    ▼
  Caddy (TLS termination, ports 80 + 443)
    ├── oauth2-proxy (forward auth, port 4180)
    │     └── OIDC provider (Kanidm, Authentik, etc.)
    └── reverse proxy → yaamon:80
```

Caddy configuration (forward-auth pattern):

```caddyfile
yaamon.example.com {
    forward_auth oauth2-proxy:4180 {
        uri /oauth2/auth
        copy_headers X-Auth-Request-User X-Auth-Request-Preferred-Username X-Auth-Request-Groups X-Auth-Request-Email
    }
    reverse_proxy yaamon:80
}
```

Enable proxy auth in YAAMon:

```yaml
proxy_auth:
  enabled: true
  username_header: X-Auth-Request-Preferred-Username
  groups_header:   X-Auth-Request-Groups
  group_roles:
    yaamon_superadmin: superuser
    yaamon_admin:      admin
    yaamon_rw:         readwrite
    yaamon_access:     readonly
```

See [Proxy Authentication](../proxy-auth.md) for the full setup guide.
