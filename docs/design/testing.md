# Testing

YAAMon has three layers of automated tests, all runnable via `make`.

## Unit tests

```bash
make test-unit
# or directly:
go test ./...
```

Run inside a Docker container (Go not required on the host). Cover:

- Database operations (`internal/db/`)
- Authentication logic (`internal/auth/`)
- HTTP handlers (`internal/server/`) — using `httptest.NewRecorder` and a real in-memory SQLite DB
- Configuration loading (`internal/config/`)
- Backup/restore (`internal/backup/`)
- State management (`internal/state/`)

Handler tests use real database instances (via `t.TempDir()`) rather than mocks, so SQL, migrations, and business logic are all exercised together.

```bash
go test ./internal/db/...      # database tests
go test ./internal/server/...  # handler tests
go test ./...                  # all packages
go test -run TestHandleAPI...  # specific test
make coverage                  # HTML coverage report
```

## Integration tests

```bash
make test-integration
```

Starts a real YAAMon Docker container (SUT), then runs Go tests in `integration/` against it over HTTP. Tests exercise:

- First-run setup flow
- Login / logout
- Node creation and AMI connection
- Backup and restore
- Proxy auth header handling
- PUID/PGID entrypoint behaviour

```bash
make test-puid   # verify uid/gid entrypoint handling in isolation
make test-deb    # build snapshot .deb, install on Ubuntu 22.04, run integration tests
```

## End-to-end tests (Playwright)

```bash
make e2e
```

Full browser-based tests using [Playwright](https://playwright.dev/) in the `e2e/` directory. Cover:

- Login and navigation
- Dashboard live updates
- Favorite management (add, reorder, connect)
- Admin user management
- Profile editing
- Theme switching

```bash
make e2e-dev    # run Playwright against an already-running dev server (faster iteration)
```

## Pre-commit checks

```bash
make check
```

Runs:
1. Whitespace check — trailing whitespace in staged files
2. `go mod tidy` verification — ensures `go.mod` and `go.sum` are up to date

Install as a git hook:

```bash
make install-hooks
```

## Test coverage

```bash
make coverage
```

Generates an HTML coverage report for all packages and opens it in the browser.
