# Building from Source

Builds run inside Docker — Go does not need to be installed on the host. Only Docker and Make are required.

```bash
git clone https://github.com/jchonig/allstar-yaamon.git
cd allstar-yaamon
```

## Common make targets

| Target | Description |
|--------|-------------|
| `make build` | Build Docker image for the current platform |
| `make build-multi` | Multi-arch build (amd64 + arm64, requires Docker buildx) |
| `make run` | Start the dev container via docker-compose |
| `make compile` | Fast cross-compile — injects binary into a running dev container |
| `make watch` | Start with docker-compose watch mode (auto-rebuild on source changes) |
| `make test` | Full test suite: pre-commit → unit → build → integration → e2e |
| `make test-unit` | Unit tests only (runs inside Docker) |
| `make test-integration` | Integration tests against a running SUT container |
| `make e2e` | Playwright end-to-end browser tests |
| `make snapshot` | GoReleaser cross-compile snapshot (all platforms + .deb, no publish) |
| `make test-deb` | Build snapshot .deb and run integration tests against the installed package |
| `make lint` | golangci-lint |
| `make coverage` | Go test coverage report |
| `make deps` | `go mod tidy` + verify |
| `make install-hooks` | Install git pre-commit hook |

## Development workflow

```bash
make run          # start the dev container
# edit source files...
make compile      # rebuild binary and hot-swap into the container (fast)
make test-unit    # run unit tests
```

The pre-commit hook (`make install-hooks`) runs whitespace checks and `go mod tidy` verification before each commit.

## Release builds

Releases use [GoReleaser](https://goreleaser.com/) configured in `.goreleaser.yml`:

- Linux binaries for amd64 + arm64
- `.deb` packages (with systemd unit, post-install script)
- Multi-arch Docker images pushed to `ghcr.io/jchonig/yaamon`

```bash
make snapshot     # build all artifacts locally without publishing
```

See [CI/CD](../design/cicd.md) for the full pipeline description.
