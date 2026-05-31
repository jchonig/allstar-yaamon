# User Roles

YAAMon has four permission levels, from least to most access:

| Role | Can do |
|------|--------|
| **readonly** | View the dashboard, node stats, and connection graph. Cannot change anything. |
| **readwrite** | Everything readonly can do, plus connect/disconnect nodes and manage favorites. |
| **admin** | Everything readwrite can do, plus add/edit/remove nodes and users, upload a favicon, and take backups. |
| **superuser** | Everything admin can do. Superusers cannot be demoted or deleted by admins — only by other superusers. |

A newly installed system requires at least one superuser. You cannot delete or demote the last superuser account.

## Role assignment

Roles are assigned when creating a user and can be changed by an admin or superuser at any time. See [Administration — Managing Users](administration.md#managing-users).

## Proxy auth and Tailscale auth

When [proxy authentication](../configuration/proxy-auth.md) is enabled, a user's role for the session comes from the OAuth2 group mapping, not the DB record (unless `update_db_role: true`).

When [Tailscale authentication](../configuration/tailscale.md) is enabled, the user logs in with their DB role — Tailscale auth does not modify roles.
