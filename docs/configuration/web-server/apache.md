# Behind Apache (ASL3 Coexistence)

When YAAMon runs on an ASL3 node alongside Apache2, the simplest setup is to keep YAAMon on port 8080 and optionally add an Apache reverse proxy to expose it on port 80 under a path prefix.

## Simple: YAAMon on port 8080

No Apache configuration needed. Access YAAMon at `http://<node-ip>:8080/`.

## Reverse proxy at root

To serve YAAMon at `http://<node-ip>/` via Apache, enable the proxy modules and add a vhost or conf snippet:

```bash
sudo a2enmod proxy proxy_http
```

Create `/etc/apache2/conf-available/yaamon.conf`:

```apache
ProxyPreserveHost On
ProxyPass        / http://127.0.0.1:8080/
ProxyPassReverse / http://127.0.0.1:8080/
```

```bash
sudo a2enconf yaamon
sudo systemctl reload apache2
```

## Reverse proxy at a path prefix

> **Note**: Full subfolder support requires a base-path feature tracked in [issue #14](https://github.com/jchonig/allstar-yaamon/issues/14). Until that ships, proxy at root (`/` → `http://localhost:8080/`) is the recommended approach — links and redirects work correctly.

When the feature is available, the configuration will be:

```apache
ProxyPreserveHost On
ProxyPass        /yaamon/ http://127.0.0.1:8080/
ProxyPassReverse /yaamon/ http://127.0.0.1:8080/
```

## TLS with Apache

Let Apache handle TLS (via `mod_ssl` or `certbot`) and forward plain HTTP to YAAMon:

```yaml
# config.yaml — TLS disabled, plain HTTP to localhost
tls:
  mode: disabled
server:
  http_port: 8080
```

Configure Apache's SSL vhost to proxy to `http://127.0.0.1:8080/`.
