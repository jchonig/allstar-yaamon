# Docker

```bash
docker run -d \
  --name yaamon \
  --restart unless-stopped \
  -p 8080:8080 \
  -v /etc/yaamon:/etc/yaamon \
  -v /var/lib/yaamon:/var/lib/yaamon \
  ghcr.io/jchonig/yaamon:latest
```

Mount your config file at `/etc/yaamon/config.yaml` and a persistent data directory at `/var/lib/yaamon`. The database is created at `/var/lib/yaamon/yaamon.db` on first run.

Access at `http://<host>:8080/`.

## Configuration

Any `config.yaml` value can be overridden with an environment variable using the pattern `YAAMON_<SECTION>_<KEY>`:

```bash
docker run -d \
  --name yaamon \
  -p 8080:8080 \
  -v /etc/yaamon:/etc/yaamon \
  -v /var/lib/yaamon:/var/lib/yaamon \
  -e YAAMON_LOG_LEVEL=debug \
  ghcr.io/jchonig/yaamon:latest
```

## Health check

The container includes a built-in health check (`GET /health`). Docker will mark the container unhealthy if the server stops responding.

## Image tags

| Tag | Description |
|-----|-------------|
| `latest` | Latest stable release (multi-arch: amd64 + arm64) |
| `v1.2.3` | Specific version |
| `branch-name` | Feature branch build (pushed by CI on every branch commit) |

See [Configuration](../configuration/README.md) for all available settings.
