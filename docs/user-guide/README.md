# User Guide

This guide covers everything accessible through the YAAMon web interface.

![Login page](../images/login.png)

## First-time setup

When you open YAAMon for the first time — before any users exist — you are redirected to `/setup`. Enter a username and password for your initial superuser account. This account has full administrative access.

After creating the account you are taken to the login page. Sign in with the credentials you just created.

> To bootstrap a fresh install non-interactively (e.g., in Docker), set `YAAMON_STATE_FILE` to the path of a state file. See [Declarative State](../configuration/declarative-state.md).

## Signing in with a passkey

If passkeys are configured, click **Sign in with passkey** below the password form. The browser presents any stored credentials for the site — no username or password is needed.

## Topics

- [User Roles](roles.md) — permission levels
- [Administration](administration.md) — managing users, nodes, backup and restore
- [Your Profile](profile.md) — avatar, password, callsign lookup, passkeys, themes
- [Overview Page](overview.md) — multi-node summary
- [Dashboards](dashboards.md) — node dashboard, favorites, network graph
- [Callsign Lookup](callsign-lookup.md) — ASL stats, callook.info, QRZ.com
