# Behind Apache

The `.deb` package installs two Apache reverse-proxy config templates into
`/etc/apache2/conf-available/`. The right approach depends on whether your
Apache site uses HTTPS:

| Scenario | Approach |
|----------|----------|
| HTTP only (port 80) | `a2enconf yaamon-subfolder` or `a2enconf yaamon-subdomain` |
| HTTPS (port 443, ASL3 with Let's Encrypt) | Add directives inside the SSL VirtualHost (see below) |

Enable proxy modules first (one-time):

```bash
sudo a2enmod proxy proxy_http
```

> **Why the difference?** Apache's `conf-enabled` snippets are included at the
> global server level. They apply to the default HTTP VirtualHost, but
> explicitly defined `<VirtualHost *:443>` blocks do not inherit them.
> ProxyPass directives must be inside the VirtualHost block to take effect
> over HTTPS.

---

## Subfolder on an HTTPS site (ASL3 coexistence — most common)

ASL3 nodes typically have HTTPS enabled via Let's Encrypt with a VirtualHost
in `/etc/apache2/sites-available/default-ssl.conf`. Add the proxy directives
directly inside that block.

**1. Edit `/etc/yaamon/config.yaml`:**

```yaml
server:
  bind_address: 127.0.0.1
  http_port: 8080
  base_path: /yaamon
tls:
  mode: disabled
```

**2. Restart YAAMon:**

```bash
sudo systemctl restart yaamon
```

**3. Add proxy directives to the SSL VirtualHost:**

```bash
sudo nano /etc/apache2/sites-available/default-ssl.conf
```

Inside `<VirtualHost *:443>`, add:

```apache
ProxyPreserveHost On
ProxyPass        /yaamon/ http://127.0.0.1:8080/yaamon/ flushpackets=on
ProxyPassReverse /yaamon/ http://127.0.0.1:8080/yaamon/
```

**4. Reload Apache:**

```bash
sudo systemctl reload apache2
```

YAAMon is now at `https://<node-hostname>/yaamon/`.

`flushpackets=on` is required for live dashboard updates (SSE).

---

## Subfolder on an HTTP-only site

If Apache is HTTP only (no SSL VirtualHost), the `conf-enabled` snippet works:

```bash
sudo a2enconf yaamon-subfolder
sudo systemctl reload apache2
```

The installed template (`/etc/apache2/conf-available/yaamon-subfolder.conf`):

```apache
ProxyPreserveHost On
ProxyPass        /yaamon/ http://127.0.0.1:8080/yaamon/ flushpackets=on
ProxyPassReverse /yaamon/ http://127.0.0.1:8080/yaamon/
```

---

## Subdomain (dedicated virtual host)

Serves YAAMon on its own hostname, e.g. `yaamon.example.com`. This creates a
new VirtualHost so it works correctly with HTTPS.

**1. Edit the template** — replace `yaamon.example.com` with your hostname:

```bash
sudo nano /etc/apache2/conf-available/yaamon-subdomain.conf
```

**2. Edit `/etc/yaamon/config.yaml`:**

```yaml
server:
  bind_address: 127.0.0.1
  http_port: 8080
  # no base_path needed
tls:
  mode: disabled   # Apache handles TLS
```

**3. Restart YAAMon:**

```bash
sudo systemctl restart yaamon
```

**4. Enable the config:**

```bash
sudo a2enconf yaamon-subdomain
sudo systemctl reload apache2
```

**5. (Optional) Add TLS with Certbot:**

```bash
sudo certbot --apache -d yaamon.example.com
```

The installed template (`/etc/apache2/conf-available/yaamon-subdomain.conf`):

```apache
<VirtualHost *:80>
    ServerName yaamon.example.com

    ProxyPreserveHost On
    ProxyPass        / http://127.0.0.1:8080/ flushpackets=on
    ProxyPassReverse / http://127.0.0.1:8080/

    ErrorLog  ${APACHE_LOG_DIR}/yaamon-error.log
    CustomLog ${APACHE_LOG_DIR}/yaamon-access.log combined
</VirtualHost>
```

---

## Simple: YAAMon on port 8080 (no Apache needed)

Access YAAMon directly at `http://<node-ip>:8080/` — no Apache configuration required.
