# AllStarLink Node Database (astdb)

YAAMon automatically downloads the AllStarLink node database from `allmondb.allstarlink.org` and uses it to populate callsign, description, and location fields for nodes and favorites that are not manually configured. The database is refreshed hourly using a conditional HTTP request (If-Modified-Since), so bandwidth use is minimal.

## File location

| Deployment | Default path |
|---|---|
| Standard Asterisk install (co-located) | `/var/lib/asterisk/astdb.txt` |
| Docker (no Asterisk on the host) | `/var/lib/yaamon/astdb.txt` |

The file is written atomically (temp file → rename) so readers never see a partial update. On startup the cache is pre-loaded from the last-known stats so the UI shows values immediately while fresh data fetches in the background.

## Configuration

```yaml
astdb:
  # Path to the AllStarLink node database file.
  path: /var/lib/asterisk/astdb.txt

  # update: true  — download on startup and refresh every hour (default).
  # update: false — read the existing file only; make no network requests.
  update: true
```

Environment-variable equivalents:

```
YAAMON_ASTDB_PATH=/var/lib/yaamon/astdb.txt
YAAMON_ASTDB_UPDATE=false
```

## Docker — dedicated copy

When there is no Asterisk on the Docker host, let YAAMon manage its own copy:

```yaml
services:
  yaamon:
    image: ghcr.io/jchonig/yaamon:latest
    environment:
      - YAAMON_ASTDB_PATH=/var/lib/yaamon/astdb.txt
    volumes:
      - ./data:/var/lib/yaamon
```

## Docker — share Asterisk's copy

If Asterisk runs on the same host, bind-mount its data directory and disable updates (Asterisk keeps the file current):

```yaml
services:
  yaamon:
    image: ghcr.io/jchonig/yaamon:latest
    environment:
      - YAAMON_ASTDB_PATH=/asterisk/astdb.txt
      - YAAMON_ASTDB_UPDATE=false
    volumes:
      - /var/lib/asterisk:/asterisk:ro
      - ./data:/var/lib/yaamon
```
