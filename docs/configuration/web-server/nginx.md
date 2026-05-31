# Behind nginx

nginx can serve as a reverse proxy in front of YAAMon — useful when TLS termination, rate limiting, or coexistence with other services is needed.

## Prerequisites

When running behind any reverse proxy, restrict YAAMon to localhost only:

```yaml
# /etc/yaamon/config.yaml
server:
  bind_address: 127.0.0.1
  http_port: 8080
tls:
  mode: disabled   # nginx handles TLS
```

> **SSE requirement**: YAAMon's live dashboard uses Server-Sent Events (SSE). nginx must have `proxy_buffering off` on the proxied location or SSE messages will be buffered and the dashboard will not update in real time.

## Variant 1 — Dedicated subdomain

Serve YAAMon at `https://yaamon.example.com/`. No `base_path` needed.

See [`examples/nginx/subdomain.conf`](../../../examples/nginx/subdomain.conf) for a complete example.

Key points:

```nginx
location / {
    proxy_pass       http://127.0.0.1:8080;
    proxy_buffering  off;       # required for SSE
    proxy_cache      off;
    proxy_read_timeout 3600s;   # SSE connections are long-lived
}
```

## Variant 2 — Sub-path of an existing virtual host

Serve YAAMon at `https://example.com/yaamon/` alongside other content.

Set `base_path` in YAAMon to match the nginx location prefix:

```yaml
# /etc/yaamon/config.yaml
server:
  bind_address: 127.0.0.1
  http_port: 8081
  base_path: /yaamon
```

Then add to your existing nginx `server {}` block:

```nginx
location /yaamon/ {
    proxy_pass       http://127.0.0.1:8081/yaamon/;
    proxy_buffering  off;
    proxy_cache      off;
    proxy_read_timeout 3600s;
}
```

See [`examples/nginx/subfolder.conf`](../../../examples/nginx/subfolder.conf) for a complete example.

> **Trailing slash**: Both the `location` block and `proxy_pass` URL must have a trailing slash so nginx strips the prefix correctly before forwarding to YAAMon.

## Reload nginx

```bash
sudo nginx -t && sudo systemctl reload nginx
```
