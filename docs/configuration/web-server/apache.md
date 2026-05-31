# Behind Apache (ASL3 Coexistence)

When YAAMon runs on an ASL3 node alongside Apache2, the simplest setup is to keep YAAMon on port 8080 and access it directly. Optionally you can add an Apache reverse proxy to expose it on port 80 — either at the root path or under a sub-path.

## Simple: YAAMon on port 8080 (no Apache integration)

No Apache configuration needed. Access YAAMon at `http://<node-ip>:8080/`.

## Reverse proxy setup — prerequisites

Enable the proxy modules and install the example config:

```bash
sudo a2enmod proxy proxy_http
sudo cp /usr/share/doc/yaamon/apache2-yaamon.conf /etc/apache2/conf-available/yaamon.conf
```

When running behind any reverse proxy, restrict YAAMon to localhost only:

```yaml
# /etc/yaamon/config.yaml
server:
  bind_address: 127.0.0.1
  http_port: 8080
```

## Variant 1 — Proxy at root (`http://<host>/`)

Use this when Apache serves a virtual host dedicated to YAAMon, or when YAAMon should answer all requests at the top level.

Edit `/etc/apache2/conf-available/yaamon.conf` (uncomment the variant 1 block):

```apache
ProxyPreserveHost On
ProxyPass        / http://127.0.0.1:8080/
ProxyPassReverse / http://127.0.0.1:8080/
```

No `base_path` setting needed — YAAMon remains at `/`.

```bash
sudo a2enconf yaamon
sudo systemctl reload apache2
```

## Variant 2 — Proxy at a sub-path (`http://<host>/yaamon/`)

Use this when Apache already serves other content at the root (e.g. the ASL3/Asterisk status page) and you want YAAMon under `/yaamon`.

Set `base_path` in YAAMon's config to match:

```yaml
# /etc/yaamon/config.yaml
server:
  bind_address: 127.0.0.1
  http_port: 8081          # use a port distinct from any other YAAMon instance
  base_path: /yaamon
```

Edit `/etc/apache2/conf-available/yaamon.conf` (uncomment the variant 2 block):

```apache
ProxyPreserveHost On
ProxyPass        /yaamon/ http://127.0.0.1:8081/yaamon/
ProxyPassReverse /yaamon/ http://127.0.0.1:8081/yaamon/
```

```bash
sudo a2enconf yaamon
sudo systemctl reload apache2
```

YAAMon is now accessible at `http://<node-ip>/yaamon/`.

## TLS with Apache

Let Apache handle TLS (via `mod_ssl` or `certbot`) and forward plain HTTP to YAAMon. Disable TLS in YAAMon — Apache terminates it:

```yaml
# /etc/yaamon/config.yaml
tls:
  mode: disabled
server:
  bind_address: 127.0.0.1
  http_port: 8080
```

Configure Apache's SSL virtual host to proxy to `http://127.0.0.1:8080/`.

> **Note**: When Apache (or any proxy) handles TLS, set `quic: false` in YAAMon's config — the proxy is responsible for HTTP/3, not YAAMon.
