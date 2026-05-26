# YAAMon Documentation

- [First-Time Setup](#first-time-setup)
- [User Roles](#user-roles)
- [Managing Users](#managing-users)
- [Adding Nodes](#adding-nodes)
- [Remote Nodes](#remote-nodes)
- [Favorites](#favorites)
- [The Dashboard](#the-dashboard)
- [Overview Page](#overview-page)
- [Node Pages](#node-pages)
- [Your Profile](#your-profile)
- [Themes](#themes)
- [Backup and Restore](#backup-and-restore)
- [Declarative State (yaamon apply)](#declarative-state-yaamon-apply)

---

## First-Time Setup

When you open YAAMon for the first time — before any users exist — you are redirected to `/setup`. Enter a username and password for your initial superuser account. This account has full administrative access.

After creating the account you are taken to the login page. Sign in with the credentials you just created.

> If you need to bootstrap a fresh install non-interactively (e.g., in a Docker environment), set `YAAMON_STATE_FILE` to the path of a state file — it will be applied automatically on container start. See [Declarative State](#declarative-state-yaamon-apply).

---

## User Roles

YAAMon has four permission levels, from least to most access:

| Role | Can do |
|------|--------|
| **readonly** | View the dashboard, node stats, and connection graph. Cannot change anything. |
| **readwrite** | Everything readonly can do, plus connect/disconnect nodes and manage favorites. |
| **admin** | Everything readwrite can do, plus add/edit/remove nodes and users, upload a favicon, and take backups. |
| **superuser** | Everything admin can do. Superusers cannot be demoted or deleted by admins — only by other superusers. |

A newly installed system requires at least one superuser. You cannot delete or demote the last superuser account.

---

## Managing Users

Go to **Admin → Users** (top-right menu, admin and superuser accounts only).

### Adding a user

Click **Add User**, enter a username, password, and choose a role. The user can log in immediately and change their own password from their profile.

### Changing a user's role

Click the role badge next to the user's name and select a new role from the dropdown. Changes take effect on the user's next page load — their existing session is not invalidated, but the new permission level is checked on every request.

### Deleting a user

Click the trash icon next to the user. You will be asked to confirm. If the user is currently logged in their session becomes invalid on the next request.

> You cannot delete or demote the last superuser account.

### Passwords

Users can change their own password from [My Profile](#your-profile). Admins and superusers can reset any user's password from the Users page.

---

## Adding Nodes

Go to **Admin → Nodes** (top-right menu, admin and superuser accounts only).

Click **Add Node** and fill in:

| Field | Description |
|-------|-------------|
| **Name** | A friendly label shown in the UI — use your call sign or a descriptive name |
| **Node number** | Your AllStarLink node number (digits only) |
| **AMI host** | Hostname or IP of the Asterisk server. Use `localhost` if YAAMon runs on the same machine as Asterisk |
| **AMI port** | Asterisk Manager port — default is `5038` |
| **AMI username** | The `user` defined in `/etc/asterisk/manager.conf` |
| **AMI password** | The `secret` for that manager user |
| **Enabled** | Uncheck to disable the AMI connection without deleting the node |

After saving, YAAMon connects to the AMI immediately. A green dot on the node card indicates a live connection; a red dot means the connection failed (check the AMI credentials and firewall).

### Minimum manager.conf entry

**When YAAMon runs on the same machine as Asterisk** (AMI host = `localhost`):

```ini
[general]
enabled = yes
bindaddr = 127.0.0.1        ; listen on loopback only — safest default

[yaamon]
secret = your-secret-here
read = system,call,log,verbose,agent,user,config,dtmf,reporting,cdr,dialplan
write = system,call,agent,user,config,command,reporting,originate
permit = 127.0.0.1/255.255.255.255
```

**When YAAMon runs on a different machine** (e.g., a Docker host or separate server), Asterisk must listen on a network interface and permit connections from the YAAMon host's address:

```ini
[general]
enabled = yes
bindaddr = 0.0.0.0          ; or the specific interface IP facing YAAMon

[yaamon]
secret = your-secret-here
read = system,call,log,verbose,agent,user,config,dtmf,reporting,cdr,dialplan
write = system,call,agent,user,config,command,reporting,originate
permit = 192.168.1.50/255.255.255.255   ; replace with YAAMon host's IP
deny = 0.0.0.0/0.0.0.0
```

Reload the manager module after any changes:

```bash
sudo asterisk -rx "module reload manager"
```

> **Security**: AMI has no encryption. If YAAMon is not on the same machine, use a VPN or SSH tunnel and keep `bindaddr` restricted to the VPN/tunnel interface rather than `0.0.0.0`. See [Remote Nodes](#remote-nodes).

---

## Remote Nodes

YAAMon can manage nodes on other machines over the network by pointing the AMI host at a remote IP address. **AMI transmits credentials in plain text** — never expose port 5038 directly to the internet.

### Option A — VPN (recommended)

Put the YAAMon host and the remote node on the same VPN (WireGuard or OpenVPN). Use the remote node's VPN IP address as the AMI host. No firewall holes needed in the remote node's public interface.

### Option B — SSH tunnel

On the YAAMon host, open a persistent tunnel to the remote node:

```bash
ssh -N -L 5038:localhost:5038 youruser@remote-node-ip
```

Then set the AMI host to `127.0.0.1` and port `5038` in YAAMon. The tunnel forwards the local port to the remote Asterisk.

For a persistent tunnel, use `autossh`:

```bash
autossh -M 0 -N -L 5038:localhost:5038 youruser@remote-node-ip
```

Or configure it as a systemd service.

### manager.conf on the remote node

When allowing remote connections, restrict the AMI `permit` line to the YAAMon host's VPN or tunnel address:

```ini
[yaamon]
secret = your-secret-here
read = system,call,log,verbose,agent,user,config,dtmf,reporting,cdr,dialplan
write = system,call,agent,user,config,command,reporting,originate
permit = 10.0.0.2/255.255.255.255    ; YAAMon host VPN address only
```

Restart Asterisk after editing `manager.conf`:

```bash
sudo asterisk -rx "module reload manager"
```

---

## Favorites

Favorites are the nodes you frequently connect to, organized per node. They appear as quick-connect buttons on the dashboard.

### Adding favorites

Go to **Favorites** (top-right menu, readwrite and above). Select the node you want to manage favorites for. Click **Add Favorite** and fill in:

| Field | Description |
|-------|-------------|
| **Node number** | The remote AllStarLink node number to connect to |
| **Callsign** | Optional — shown on the button |
| **Description** | A longer label for the node |
| **Location** | City, state, or other location info |
| **Group** | Organize favorites into named groups (tabs on the dashboard) |

### Reordering

Drag and drop favorites within a group to reorder them. The order is saved immediately.

### Copying favorites

You can copy all favorites from one node to another using the **Copy from node** button at the top of the Favorites page. Useful when you add a second node and want the same set of favorites.

### Importing from AllScan

Use the `yaamon apply` command with a state file to bulk-import favorites. See [Declarative State](#declarative-state-yaamon-apply).

---

## The Dashboard

The dashboard is the main view. It shows live stats for your connected node(s) and lets you connect and disconnect.

### Selecting a node

If you have more than one node, the navbar shows either a button group (on wider screens) or a dropdown (on narrow screens) with **Overview** and each of your nodes. Click a node name to switch to its dashboard. Click **Overview** for the multi-node summary.

### Connecting to a favorite

Click any favorite button to send a connect command to that node via AMI. The button highlights when the connection is active. Click it again (or click **Disconnect**) to disconnect.

### Live updates

The dashboard uses Server-Sent Events (SSE) to push live stats — you do not need to refresh the page. The connection indicator in the corner shows whether the live feed is active.

---

## Overview Page

The Overview page is shown when you have more than one node and click **Overview** in the nav. It displays a summary card for each node showing:

- Connection status (green/red dot)
- Node number and name
- Number of active connections
- Whether the node is currently keyed

Click a node card to jump to that node's full dashboard.

---

## Node Pages

Each node's dashboard shows:

### Connection list

The active connections table lists every node currently linked, with:
- Node number (click to open the AllStarLink page for that node)
- Callsign and location (pulled from the AllStarLink node database)
- Direction (inbound / outbound)
- Duration of the current connection
- Whether the remote node is currently keyed

Hovering over a node number shows a tooltip with additional AllStarLink stats when available.

### Favorites panel

Your favorites for this node are shown as buttons, organized by group. Buttons for active connections are highlighted. Click to connect; click again to disconnect.

### Network graph

Click the graph icon on any connection row to open an interactive network graph showing how the connected nodes link to each other. The graph is also available as a full-page view.

---

## Your Profile

Click your name or avatar in the top-right corner and choose **My Profile**.

### Avatar

You can set an avatar two ways:

- **Upload an image** — click **Choose…**, pick a PNG, JPEG, GIF, or WebP image (max 2 MB), then click **Upload**. The image is stored in YAAMon's database.
- **Link to a URL** — paste an external image URL into the "Or link to an avatar URL" field and save. The browser fetches the image directly from that URL on each page load.

Click **Remove** to clear the avatar entirely.

### Full name

Enter your name in the **Full Name** field. It appears in the navbar dropdown in place of your username.

### Password

Enter your current password and a new password to change it. Passwords must be at least 8 characters. Leave both fields blank to keep the current password.

---

## Themes

Click the half-circle icon in the top-right corner to switch themes:

| Theme | Description |
|-------|-------------|
| **System** | Follows your OS dark/light mode preference |
| **Dark** | Dark background (default) |
| **Light** | Light background |
| **Solarized** | Solarized color palette |
| **High Contrast** | Maximum contrast for accessibility |

Your choice is saved in browser local storage and persists across sessions.

---

## Backup and Restore

Go to **Admin → Backup** (admin and superuser accounts only).

### Creating a backup

Click **Download Backup**. YAAMon exports the entire database — users, nodes, favorites, and configuration — as a compressed `.owbackup` file. Optionally enter a passphrase to encrypt the backup before download.

From the command line:

```bash
yaamon backup -o /path/to/backup.owbackup
yaamon backup -o /path/to/backup.owbackup --passphrase "your passphrase"
```

### Inspecting a backup

Before restoring, you can inspect a backup file to see what it contains without applying it:

```bash
yaamon inspect /path/to/backup.owbackup
```

### Restoring

On the Backup page, click **Restore** and upload a `.owbackup` file. If the backup is encrypted, enter the passphrase. YAAMon takes an automatic safety backup of the current database before overwriting it.

From the command line:

```bash
yaamon restore /path/to/backup.owbackup
yaamon restore /path/to/backup.owbackup --passphrase "your passphrase"
```

> **Warning**: Restore replaces the entire database. All current users, nodes, and favorites are overwritten.

---

## Declarative State (yaamon apply)

`yaamon apply` lets you define users, nodes, and favorites in a YAML file and apply them to the database non-interactively. This is useful for:

- Bootstrapping a fresh Docker container
- Version-controlling your YAAMon configuration
- Bulk-importing favorites from AllScan

```bash
yaamon apply state.yaml
yaamon apply state.yaml --dry-run          # preview changes without applying
yaamon apply state.yaml --reset-passwords  # overwrite existing user passwords
```

### Automatic apply on Docker startup

Set the `YAAMON_STATE_FILE` environment variable to the path of your state file inside the container. The entrypoint will apply it automatically every time the container starts, before the server comes up:

```yaml
services:
  yaamon:
    image: ghcr.io/jchonig/allstar-yaamon:latest
    volumes:
      - ./config:/etc/yaamon
      - yaamon-data:/data
    environment:
      - YAAMON_STATE_FILE=/etc/yaamon/state.yaml
```

Because `apply` is idempotent by default (it only adds or updates, never deletes unless `purge` is enabled), running it on every start is safe — existing data is not overwritten unless the state file changes.

See [`state.yaml.example`](state.yaml.example) for the full format. Environment variable substitution is supported — any value beginning with `$` is resolved from the environment at apply time, which keeps secrets out of the file:

```yaml
users:
  - username: admin
    permission: superuser
    password: $ADMIN_PASSWORD    # set ADMIN_PASSWORD in the environment

nodes:
  - name: "Home Node"
    node_number: "12345"
    ami_host: localhost
    ami_user: yaamon
    ami_pass: $AMI_PASSWORD
    enabled: true
    favorites:
      - node_number: "29840"
        callsign: W1AW
        description: ARRL HQ
        location: Newington, CT
```

### purge controls

By default `apply` only adds or updates — it never deletes. Set `purge: true` for any section to delete objects not present in the state file:

```yaml
purge:
  users: false      # keep DB users not listed here
  nodes: false      # keep DB nodes not listed here
  favorites: false  # keep favorites not listed under their node
```
