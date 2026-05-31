# REST API

YAAMon's HTTP API is consumed by its own frontend. All endpoints return JSON unless noted. State-changing requests require a valid CSRF token in the `X-CSRF-Token` header.

## Public (no authentication required)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/health` | Health check — returns 200 OK |
| GET | `/setup` | Setup page (redirected to on first run) |
| POST | `/setup` | Create initial superuser account |
| GET | `/login` | Login page |
| POST | `/login` | Authenticate with username + password |
| GET | `/logout` | Clear session cookie |
| POST | `/api/passkeys/login/begin` | Start WebAuthn discoverable login ceremony |
| POST | `/api/passkeys/login/finish` | Complete WebAuthn login; sets session cookie |

## Readonly (readwrite, admin, superuser)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/` | Dashboard (redirects to first node or overview) |
| GET | `/dashboard` | Dashboard page |
| GET | `/dashboard/overview` | Overview page |
| GET | `/dashboard/{nodeID}` | Node dashboard page |
| GET | `/sse/{nodeID}` | Server-Sent Events stream for live stats |
| GET | `/graph/{nodeNumber}` | Network graph page |
| GET | `/api/nodes` | List all nodes |
| GET | `/api/nodes/{id}/favorites` | List favorites for a node |
| GET | `/api/nodes/{id}/stats` | Current stats for a node |
| GET | `/api/nodes/{id}/connections/{nodeNumber}` | Connection detail |
| GET | `/api/profile` | Get current user's profile |
| PUT | `/api/profile` | Update profile (name, avatar URL, lookup source, Tailscale usernames) |
| POST | `/api/profile/avatar` | Upload avatar image |
| DELETE | `/api/profile/avatar` | Delete uploaded avatar |
| GET | `/api/profile/qrz` | Get QRZ.com credentials status |
| PUT | `/api/profile/qrz` | Save QRZ.com credentials |
| DELETE | `/api/profile/qrz` | Remove QRZ.com credentials |
| DELETE | `/api/profile/qrz/cache` | Clear personal QRZ lookup cache |
| GET | `/api/users/{id}/avatar` | Serve a user's uploaded avatar image |
| GET | `/api/qrz/{callsign}` | Look up a callsign (callook.info or QRZ.com) |
| GET | `/api/passkeys` | List current user's registered passkeys |
| POST | `/api/passkeys/register/begin` | Start WebAuthn registration ceremony |
| POST | `/api/passkeys/register/finish` | Complete WebAuthn registration |
| PATCH | `/api/passkeys/{id}` | Rename a passkey |
| DELETE | `/api/passkeys/{id}` | Delete a passkey |

## Readwrite

| Method | Path | Description |
|--------|------|-------------|
| GET | `/settings/favorites` | Favorites management page |
| GET | `/settings/favorites/{nodeID}` | Favorites page for a specific node |
| POST | `/api/nodes/{id}/connect` | Connect to a node |
| POST | `/api/nodes/{id}/disconnect` | Disconnect from a node |
| POST | `/api/nodes/{id}/favorites` | Create a favorite |
| PUT | `/api/nodes/{id}/favorites/{fid}` | Update a favorite |
| PUT | `/api/nodes/{id}/favorites/reorder` | Reorder favorites (drag and drop) |
| POST | `/api/nodes/{id}/favorites/copy` | Copy favorites from another node |
| DELETE | `/api/nodes/{id}/favorites/{fid}` | Delete a favorite |

## Admin

| Method | Path | Description |
|--------|------|-------------|
| GET | `/admin/nodes` | Nodes admin page |
| POST | `/api/nodes` | Create a node |
| PUT | `/api/nodes/{id}` | Update a node |
| DELETE | `/api/nodes/{id}` | Delete a node |
| POST | `/api/nodes/{id}/test` | Test AMI connectivity |
| GET | `/api/nodes/{id}/secret` | Get AMI secret (masked) |
| POST | `/api/admin/import/allmon3/preview` | Preview Allmon3 INI import |
| POST | `/api/admin/import/allmon3` | Execute Allmon3 INI import |
| POST | `/api/admin/favicon` | Upload custom favicon |
| DELETE | `/api/admin/favicon` | Remove custom favicon |
| GET | `/admin/users` | Users admin page |
| GET | `/api/users` | List all users |
| POST | `/api/users` | Create a user |
| PUT | `/api/users/{id}` | Update a user (role, Tailscale usernames, etc.) |
| DELETE | `/api/users/{id}` | Delete a user |
| GET | `/admin/backup` | Backup/restore page |
| POST | `/api/backup` | Download a backup |
| POST | `/api/backup/inspect` | Inspect a backup file |
| POST | `/api/backup/restore` | Restore from a backup |
| DELETE | `/api/admin/integrations/qrz/cache` | Clear shared QRZ lookup cache (all users) |

## Superuser

| Method | Path | Description |
|--------|------|-------------|
| *(same as admin — no superuser-only API endpoints; role difference is enforced at data level)* | | |

## Static assets

| Path | Description |
|------|-------------|
| `/static/*` | Embedded static files (CSS, JS, fonts) |
| `/favicon.ico` | Favicon (custom or default) |
| `/favicon.png` | 256×256 PNG favicon |
