# Web Server Configuration

YAAMon includes its own HTTP server — no external web server is required. The default HTTP port is **8080** so it can coexist with ASL3's Apache2 on port 80.

## Deployment options

| Scenario | See |
|----------|-----|
| Run standalone (no other web server) | [Standalone](standalone.md) |
| Behind Apache (ASL3 coexistence on port 80) | [Behind Apache](apache.md) |
| Behind nginx | [Behind nginx](nginx.md) |
| Behind Caddy (TLS termination, auth proxy) | [Behind Caddy](caddy.md) |

## Ports

Configure ports in `config.yaml`:

```yaml
server:
  http_port: 8080    # default — change to 80 if standalone
  https_port: 443
  redirect_http: true   # 301 redirect http → https when TLS is enabled
```

Or via environment variables:

```
YAAMON_SERVER_HTTP_PORT=80
YAAMON_SERVER_HTTPS_PORT=443
```

## Reverse proxy options

When running behind a reverse proxy, two additional settings control YAAMon's behaviour:

| Setting | Purpose |
|---------|---------|
| `server.bind_address` | Restrict the listener to a specific interface (e.g. `127.0.0.1`). Default `""` = all interfaces. |
| `server.base_path` | Mount the app under a sub-path prefix (e.g. `/yaamon`). Required when the proxy forwards a sub-path. |

```yaml
server:
  bind_address: 127.0.0.1   # only reachable from the proxy, not the network
  base_path: /yaamon         # omit for root proxy; set to match the proxy location
```
