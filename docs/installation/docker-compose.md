# docker-compose

```yaml
services:
  yaamon:
    image: ghcr.io/jchonig/yaamon:latest
    restart: unless-stopped
    ports:
      - "8080:8080"
    volumes:
      - ./config:/etc/yaamon       # config.yaml lives here
      - yaamon-data:/var/lib/yaamon  # named volume for the SQLite database

volumes:
  yaamon-data:
```

```bash
docker compose up -d
docker compose logs -f
```

## Named volume vs bind-mount

A named volume (`yaamon-data:`) is created and owned by Docker — no permission setup needed. A bind-mount (`./data:/var/lib/yaamon`) gives you a known path on the host but requires the directory to be writable by the container process (uid 1000 by default).

## Bind-mount ownership (PUID / PGID)

When using a bind-mount for `/var/lib/yaamon`, the host directory must be writable by the container process. By default YAAMon runs as uid/gid 1000. Set `PUID` and `PGID` to match your host directory owner:

```yaml
services:
  yaamon:
    image: ghcr.io/jchonig/yaamon:latest
    volumes:
      - ./data:/var/lib/yaamon
    environment:
      - PUID=1000   # run: id -u
      - PGID=1000   # run: id -g
```

The container entrypoint starts as root, adjusts the internal `yaamon` user to the specified uid/gid, re-owns `/var/lib/yaamon`, then drops privileges before starting the server.

> **Note**: `PUID`/`PGID` have no effect when the container is started with `--user`. In that case Docker assigns the uid directly and the privilege-drop is skipped.

## Non-standard ports

Map host ports in `ports` and set matching values in `config.yaml` (or via environment variables):

```yaml
ports:
  - "8080:8080"
  - "8443:8443"
environment:
  - YAAMON_SERVER_HTTP_PORT=8080
  - YAAMON_SERVER_HTTPS_PORT=8443
```

## Automatic state apply on startup

Set `YAAMON_STATE_FILE` to apply a declarative state file every time the container starts:

```yaml
environment:
  - YAAMON_STATE_FILE=/etc/yaamon/state.yaml
```

See [Declarative State](../configuration/declarative-state.md) for the state file format.

## Development: fast binary swap

YAAMon's `docker-compose.yml` supports a `develop.watch` mode that swaps the binary without a full image rebuild:

```bash
make compile   # cross-compiles the binary and injects it into the running container
```

See [Building from source](building.md) for the dev workflow.
