# Behind Apache

The `.deb` package installs two ready-to-use Apache reverse-proxy configs into
`/etc/apache2/conf-available/`. Enable whichever fits your deployment:

| Config | Use case | Enabled with |
|--------|----------|--------------|
| `yaamon-subfolder` | ASL3 coexistence — YAAMon at `/yaamon/` on the existing site | `a2enconf yaamon-subfolder` |
| `yaamon-subdomain` | Dedicated subdomain — `yaamon.example.com` | `a2enconf yaamon-subdomain` |

Enable the required modules first (one-time):

```bash
sudo a2enmod proxy proxy_http substitute
```

---

## Subfolder (ASL3 coexistence)

Keeps the existing ASL3/Apache site at `/` and adds YAAMon under `/yaamon/`.

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

**3. Enable the Apache config:**

```bash
sudo a2enconf yaamon-subfolder
sudo systemctl reload apache2
```

YAAMon is now at `https://<node-hostname>/yaamon/`.

The installed config (`/etc/apache2/conf-available/yaamon-subfolder.conf`):

```apache
Redirect permanent /yaamon /yaamon/

ProxyPreserveHost On
ProxyPass        /yaamon/ http://127.0.0.1:8080/yaamon/ flushpackets=on
ProxyPassReverse /yaamon/ http://127.0.0.1:8080/yaamon/

# Inject a YAAMon card into the ASL3 dashboard without modifying index.html.
<IfModule mod_substitute.c>
  <Directory /var/www/html>
    <Files "index.html">
      AddOutputFilterByType SUBSTITUTE text/html
      Substitute "s|</body>|<script src=\"/js/yaamon-asl3-card.js\"></script></body>|i"
    </Files>
  </Directory>
</IfModule>
```

`flushpackets=on` is required for live dashboard updates (SSE).

The config also uses `mod_substitute` to inject a YAAMon card into the ASL3 web dashboard automatically — no changes to ASL3's `index.html` are needed. The `.deb` installs the required JavaScript to `/var/www/html/js/yaamon-asl3-card.js`.

---

## Subdomain (dedicated virtual host)

Serves YAAMon on its own hostname, e.g. `yaamon.example.com`. This creates a
new VirtualHost so it works correctly alongside the existing ASL3 site.

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
