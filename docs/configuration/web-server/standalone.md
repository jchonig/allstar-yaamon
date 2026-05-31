# Standalone Web Server

When YAAMon is the only web application on the host, it can run directly on ports 80 and 443 without a reverse proxy.

## TLS modes

Set `tls.mode` in `config.yaml`:

| Mode | When to use |
|------|-------------|
| `disabled` | HTTP only — local LAN, or when TLS is handled by an upstream proxy |
| `self_signed` | Generates a self-signed certificate on first run — instant setup, browser warning |
| `provided` | Supply your own certificate and key files |
| `acme` | Automatic Let's Encrypt certificate via ACME — requires a public domain name |

## `disabled` (HTTP only)

```yaml
tls:
  mode: disabled
server:
  http_port: 8080
```

## `self_signed`

```yaml
tls:
  mode: self_signed
server:
  http_port: 80
  https_port: 443
  redirect_http: true
```

YAAMon generates a self-signed certificate on first run and stores it in the database. The certificate is reused on subsequent starts. Browsers will show a security warning — click through or add a permanent exception.

## `provided`

```yaml
tls:
  mode: provided
  cert_file: /etc/yaamon/fullchain.pem
  key_file:  /etc/yaamon/privkey.pem
server:
  https_port: 443
  redirect_http: true
```

Provide a PEM-format certificate chain and private key. Restart YAAMon after renewing the certificate.

> **Planned**: automatic hot-reload when the certificate file changes (tracked in [issue #11](https://github.com/jchonig/allstar-yaamon/issues/11)) — no restart will be required.

## `acme` (Let's Encrypt)

```yaml
tls:
  mode: acme
  acme_domain: yaamon.example.com
  acme_cache_dir: /etc/yaamon/acme
server:
  http_port: 80    # ACME HTTP-01 challenge requires port 80
  https_port: 443
  redirect_http: true
```

Requirements:
- A registered public domain name pointing to this host
- Port 80 reachable from the internet (for the HTTP-01 ACME challenge)
- `acme_cache_dir` writable by the `yaamon` user

> **Planned**: DNS-01 ACME challenge support (no port 80 required, wildcard certificates) — tracked in [issue #11](https://github.com/jchonig/allstar-yaamon/issues/11).

## HTTP/3 (QUIC)

When TLS is active (`tls.mode` is anything other than `disabled`), YAAMon automatically opens a QUIC (UDP) listener on the same port as HTTPS and injects an `Alt-Svc` header on every response so browsers discover and upgrade to HTTP/3. No configuration is required.

To opt out (e.g. when UDP is blocked by a firewall):

```yaml
server:
  quic: false
```

**Firewall / Docker note**: QUIC runs over UDP. Make sure UDP on the HTTPS port is reachable:

```bash
# firewalld (AllStarLink Appliance or any firewalld host)
sudo firewall-cmd --permanent --add-port=443/udp
sudo firewall-cmd --reload
```

```yaml
# docker-compose.yml — add the UDP mapping alongside TCP
ports:
  - "443:443"
  - "443:443/udp"
```

> **Behind a proxy?** QUIC is not needed in YAAMon when running behind Caddy (or any modern reverse proxy) — the proxy handles QUIC termination itself. This feature is only relevant for standalone deployments where YAAMon is the TLS endpoint. See [Behind Caddy](caddy.md) for proxy setup.

## Running on port 80 without root

Port 80 (and 443) are privileged ports. To run as a non-root user, grant the binary the `CAP_NET_BIND_SERVICE` capability:

```bash
sudo setcap cap_net_bind_service=+ep /usr/local/bin/yaamon
```

This survives reboots but must be re-applied after each upgrade. Alternatively, run behind a reverse proxy that listens on port 80 and forwards to YAAMon on 8080.
