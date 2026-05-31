# Tailscale Authentication

When YAAMon sits behind [caddy-tailscale](https://github.com/tailscale/caddy-tailscale), it can automatically log in users arriving from the Tailscale network without a password.

## How it works

caddy-tailscale's `tailscale_auth` directive identifies the Tailscale user making each request and populates Caddy auth user fields. Those fields are mapped to HTTP headers by the `reverse_proxy` block and forwarded to YAAMon. YAAMon reads the login name from the configured header and looks for a YAAMon DB user whose **Tailscale Usernames** profile field contains that login. If found, the user is logged in with their DB role. If not found, YAAMon falls through to the cookie session (if any) or the login page.

Unlike OAuth2 proxy auth, Tailscale auth **does not create users automatically**. The mapping must be configured in each user's profile.

> **Note**: `tailscale_auth` identifies the connecting user's personal device. It does not work for tagged (service) devices — only user-owned devices.

## Caddyfile configuration

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

> Use `tailscale_user` (not `tailscale_login`) for the login header — `tailscale_user` is the fully-qualified identity (e.g. `jchonig@github`) which avoids ambiguity when users from different Tailscale tailnets might share the same short username.

## YAAMon configuration

```yaml
tailscale_auth:
  enabled: true

  # Header carrying the Tailscale login name (must match the header_up name in Caddyfile).
  user_header: Tailscale-User-Login

  # Optional headers for display name and avatar.
  name_header:   Tailscale-User-Name
  avatar_header: Tailscale-User-Profile-Pic
```

## Mapping users to Tailscale logins

Each YAAMon user must have their Tailscale login name listed in their profile before Tailscale auth will work for them.

**Self-service**: Open **My Profile** from the top-right dropdown. Enter your Tailscale login name (e.g. `jch@honig.net`) in the **Tailscale Usernames** field. Separate multiple logins with commas.

**Admin**: Admins and superusers can set the Tailscale Usernames field for any user from **Admin → Users**.

When connecting through caddy-tailscale, an **Add \<login\>** button appears automatically in the profile when the login is not yet mapped.

## Display name and avatar

If the user's YAAMon profile has no display name or avatar, they are filled opportunistically from the Tailscale headers for that request. To make the values permanent, save them in the profile.

## Auth indicator

When a session is established via Tailscale, a shield icon (🛡) appears next to the username in the top-right dropdown. Hover to see `Authenticated via tailscale`. If the Tailscale login differs from the YAAMon username, the login is shown in parentheses: `Authenticated via tailscale (jch@honig.net)`.

## Troubleshooting

See [Troubleshooting — Tailscale auth checklist](../troubleshooting/README.md#checklist-for-tailscale-auth).
