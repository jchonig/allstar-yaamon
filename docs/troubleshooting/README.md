# Troubleshooting

## Enabling debug logging

Set the log level to `debug` in `config.yaml` or via environment variable:

```yaml
log:
  level: debug
```

```bash
YAAMON_LOG_LEVEL=debug
```

Restart YAAMon and watch the logs:

```bash
sudo journalctl -u yaamon -f          # systemd
docker compose logs -f yaamon         # Docker
```

## Proxy auth / Tailscale auth not working

With debug logging enabled, the auth middleware logs a message on every request. Look for lines with `tailscale auth` or `oauth2 auth`:

| Message | Meaning |
|---------|---------|
| `tailscale auth: header absent` | The configured `user_header` was not present. The Caddyfile is missing `tailscale_auth`, or caddy-tailscale is not in use. |
| `tailscale auth: no matching user` | The header was present but no DB user has that Tailscale login in their **Tailscale Usernames** profile field. |
| `tailscale auth: matched user` | Tailscale auth succeeded. |
| `oauth2 auth: header absent` | The configured `username_header` was not present. The proxy is not injecting auth headers. |
| *(no `oauth2 auth` lines at all)* | `proxy_auth.enabled` is `false`. Check that `proxy_auth.enabled: true` is in `config.yaml`. |
| `oauth2 auth: group_roles is not configured` | `proxy_auth.enabled: true` but `group_roles` is empty. Add a mapping to `config.yaml`. All requests are denied until this is configured. |
| `oauth2 auth: no matching group` | The username header was present but none of the user's groups appear in `group_roles`. |
| `oauth2 auth: matched role` | OAuth2 auth succeeded. |

## Checklist for Tailscale auth

1. `tailscale_auth.enabled: true` is set in `config.yaml`.
2. The Caddyfile has `tailscale_auth` before `reverse_proxy`.
3. The `reverse_proxy` block has `header_up` lines mapping Caddy auth user fields to the headers YAAMon expects:
   ```caddyfile
   header_up Tailscale-User-Login       {http.auth.user.tailscale_user}
   header_up Tailscale-User-Name        {http.auth.user.tailscale_name}
   header_up Tailscale-User-Profile-Pic {http.auth.user.tailscale_profile_picture}
   ```
4. The user has their Tailscale login (e.g. `jch@honig.net`) in their **Tailscale Usernames** profile field.
5. YAAMon is not directly reachable from clients — only Caddy should reach it, otherwise headers can be spoofed.

## Checklist for OAuth2 / oauth2-proxy auth

1. `proxy_auth.enabled: true` is set in `config.yaml`. This key defaults to `false` — if absent or false, no `oauth2 auth` log lines appear even with debug logging.
2. `username_header` and `groups_header` match what oauth2-proxy injects (defaults: `X-Auth-Request-Preferred-Username` and `X-Auth-Request-Groups`).
3. `group_roles` is configured with at least one group → role mapping. If empty, every proxy-auth request is denied (403) and a WARN is logged.
4. The user's OIDC provider groups appear in `group_roles`.
5. oauth2-proxy is configured with `set-xauthrequest = true` to inject the headers.
6. YAAMon is not directly reachable from clients — only the proxy should reach it.

## Docker bind-mount ownership

If YAAMon fails to start with a permission error on the database file, the host directory is not writable by the container process (uid 1000 by default). Set `PUID` and `PGID` to match the host directory owner:

```yaml
environment:
  - PUID=1000   # run: id -u
  - PGID=1000   # run: id -g
```

See [docker-compose installation](../installation/docker-compose.md#bind-mount-ownership-puid--pgid) for details.

## AMI connection failing

- Green dot = live AMI connection
- Red dot = connection failed

Check:
1. Asterisk is running: `sudo systemctl status asterisk`
2. AMI is enabled in `/etc/asterisk/manager.conf` (`enabled = yes`)
3. The `permit` line allows connections from the YAAMon host IP
4. The secret in YAAMon matches `manager.conf`
5. The AMI port (default 5038) is not blocked by a firewall

Test connectivity without starting the full server:

```bash
yaamon node test <id>
```

For remote nodes, see [AMI Security](../security/ami-security.md).

## Lost superuser access

If you have lost access to all superuser accounts, use the CLI to reset a password:

```bash
sudo yaamon user passwd <username>
```

Or create a new superuser:

```bash
sudo yaamon user add recovery -P superuser
```

This requires local access to the server (not via the web UI).
