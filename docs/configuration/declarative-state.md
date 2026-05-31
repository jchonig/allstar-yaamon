# Declarative State (yaamon apply)

`yaamon apply` lets you define users, nodes, and favorites in a YAML file and apply them to the database non-interactively. This is useful for:

- Bootstrapping a fresh Docker container
- Version-controlling your YAAMon configuration
- Bulk-importing favorites

```bash
yaamon apply state.yaml
yaamon apply state.yaml --dry-run          # preview changes without applying
yaamon apply state.yaml --reset-passwords  # overwrite existing user passwords
```

`apply` is idempotent by default — it only adds or updates, never deletes unless `purge` is enabled. Running it on every start is safe.

## State file format

```yaml
users:
  - username: admin
    permission: superuser
    password: $ADMIN_PASSWORD    # resolved from environment at apply time

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

Environment variable substitution is supported — any value beginning with `$` is resolved from the environment at apply time, keeping secrets out of the file.

See [`state.yaml.example`](../../state.yaml.example) in the repository for the full format.

## purge controls

By default `apply` only adds or updates — it never deletes. Set `purge: true` for any section to delete objects not present in the state file:

```yaml
purge:
  users: false      # keep DB users not listed here
  nodes: false      # keep DB nodes not listed here
  favorites: false  # keep favorites not listed under their node
```

## Automatic apply on Docker startup

Set the `YAAMON_STATE_FILE` environment variable to apply the state file every time the container starts:

```yaml
services:
  yaamon:
    image: ghcr.io/jchonig/yaamon:latest
    volumes:
      - ./config:/etc/yaamon
      - yaamon-data:/var/lib/yaamon
    environment:
      - YAAMON_STATE_FILE=/etc/yaamon/state.yaml
```

The entrypoint applies the state file before starting the server. Because `apply` is idempotent, existing data is not overwritten unless the state file changes (or `--reset-passwords` is set).

## CLI reference

See [CLI Reference — apply](../reference/cli.md#apply) for all flags.
