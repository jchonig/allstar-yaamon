# YAAMon CLI Reference

`yaamon` is the command-line interface for managing YAAMon. All subcommands
read the same `config.yaml` that the server uses to locate the database.

## Global flags

| Flag | Default | Description |
|---|---|---|
| `--config <path>` | `/etc/yaamon/config.yaml` or `./config.yaml` | Path to config file |

## Commands

- [serve](#serve)
- [apply](#apply)
- [user](#user)
  - [user list](#user-list)
  - [user add](#user-add)
  - [user passwd](#user-passwd)
  - [user permission](#user-permission)
  - [user delete](#user-delete)
- [node](#node)
  - [node list](#node-list)
  - [node add](#node-add)
  - [node edit](#node-edit)
  - [node enable / node disable](#node-enable--node-disable)
  - [node delete](#node-delete)
  - [node test](#node-test)
- [backup](#backup)
- [restore](#restore)
- [inspect](#inspect)

---

## serve

Start the web server.

```
yaamon serve [--config <path>]
```

Reads `config.yaml`, opens the database (running any pending migrations), and
starts listening on the configured HTTP or HTTPS port. Blocks until interrupted
(`SIGINT` / `SIGTERM`), then gracefully shuts down.

---

## apply

Apply a declarative YAML state file to the database.

```
yaamon apply <state-file> [flags]
```

| Flag | Description |
|---|---|
| `--dry-run` | Print planned changes without writing anything |
| `--reset-passwords` | Overwrite existing user passwords from the state file (by default passwords are only set on creation) |

`apply` is idempotent — it only creates or updates records. Records not
mentioned in the state file are left alone unless a `purge` section enables
deletion. See [Declarative State](DOCUMENTATION.md#declarative-state-yaamon-apply)
for the full state file format.

---

## user

Manage local user accounts.

### user list

```
yaamon user list
```

Print all users with their ID, username, and permission level.

```
ID  USERNAME  PERMISSION
1   admin     superuser
2   alice     readwrite
```

### user add

```
yaamon user add <username> [flags]
```

| Flag | Default | Description |
|---|---|---|
| `-P, --permission <level>` | `readonly` | Permission level: `readonly`, `readwrite`, `admin`, or `superuser` |
| `-p, --password <pw>` | *(prompted)* | Password; prompted interactively if omitted |

### user passwd

```
yaamon user passwd <username> [flags]
```

| Flag | Default | Description |
|---|---|---|
| `-p, --password <pw>` | *(prompted)* | New password; prompted interactively if omitted |

Useful for recovering access when the web UI is unavailable. Also resets the
`*` sentinel on OAuth2-created accounts, re-enabling local login.

### user permission

```
yaamon user permission <username> -P <level>
```

| Flag | Required | Description |
|---|---|---|
| `-P, --permission <level>` | yes | New permission level |

Valid levels: `none`, `readonly`, `readwrite`, `admin`, `superuser`.

### user delete

```
yaamon user delete <username>
```

Refuses to delete the last superuser account.

---

## node

Manage AllStar nodes. Node subcommands that take an `<id>` use the numeric
database ID shown by `node list`, not the AllStar node number.

### node list

```
yaamon node list
```

Print all nodes with their ID, name, node number, AMI host, port, username,
and enabled state.

### node add

```
yaamon node add <name> -n <node-number> [flags]
```

| Flag | Default | Description |
|---|---|---|
| `-n, --number <num>` | *(required)* | AllStar node number |
| `-H, --ami-host <host>` | `localhost` | AMI host |
| `-p, --ami-port <port>` | `5038` | AMI port |
| `-u, --ami-user <user>` | | AMI username |
| `-P, --ami-pass <pass>` | | AMI password |
| `-e, --enabled` | `true` | Connect AMI on server start |

### node edit

```
yaamon node edit <id> [flags]
```

Only flags that are explicitly provided are changed; omitted flags leave the
existing value intact.

| Flag | Description |
|---|---|
| `-N, --name <name>` | New display name |
| `-n, --number <num>` | New AllStar node number |
| `-H, --ami-host <host>` | New AMI host |
| `-p, --ami-port <port>` | New AMI port |
| `-u, --ami-user <user>` | New AMI username |
| `-P, --ami-pass <pass>` | New AMI password |

### node enable / node disable

```
yaamon node enable <id>
yaamon node disable <id>
```

Enable or disable the AMI connection for a node without deleting it. Takes
effect after a server restart.

### node delete

```
yaamon node delete <id>
```

Deletes the node and all of its favorites (cascaded).

### node test

```
yaamon node test <id>
```

Opens an AMI connection to the node and immediately closes it, reporting
success or the error. Useful for verifying credentials and network
connectivity without starting the full server.

---

## backup

Create a `.owbackup` archive from the live database.

```
yaamon backup [flags]
```

| Flag | Default | Description |
|---|---|---|
| `-o, --output <file>` | `yaamon-<timestamp>.owbackup` | Output file path |
| `-p, --passphrase <pw>` | *(none)* | Encrypt the backup with a passphrase |

The backup includes all nodes, favorites, users, and config records. TLS
certificates are excluded. The server does not need to be stopped.

---

## restore

Restore the database from a `.owbackup` file.

```
yaamon restore <file.owbackup> [flags]
```

| Flag | Default | Description |
|---|---|---|
| `-p, --passphrase <pw>` | *(none)* | Passphrase for an encrypted backup |

Before overwriting the database, a pre-restore backup is written to the same
directory. The server must be restarted after a successful restore.

---

## inspect

Print the manifest of a `.owbackup` file without restoring it.

```
yaamon inspect <file.owbackup>
```

Outputs format, app version, schema version, creation time, hostname,
encryption status, and record counts. Does not require database access.

Example output:

```
Format:         owbackup v1
App Version:    v0.1.13
Schema Version: 6
Created At:     2026-05-29 21:00:00 EDT
Hostname:       yaamon.home.honig.net
Encrypted:      false
Contents:       3 nodes, 42 favorites, 4 users, 7 configs
```
