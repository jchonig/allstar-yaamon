# CI/CD

YAAMon uses GitHub Actions for continuous integration, branch image builds, and releases. All build steps run inside Docker — no Go installation is required on the host.

## Workflows

### `ci.yml` — Continuous Integration

Triggers on pushes to `main` and on pull requests, but **only when build-relevant files change**. Pushes that touch only `docs/**`, `*.md`, `contrib/**`, `examples/**`, or other non-code files skip CI entirely — GitHub marks the workflow as passed so docs-only PRs still merge cleanly.

The path filter covers: `**/*.go`, `go.mod`, `go.sum`, `Dockerfile`, `Makefile`, `web/**`, `scripts/**`, `e2e/**`, `integration_tests/**`, `.github/workflows/**`.

Stages (in order):

1. **Pre-commit checks** — whitespace validation + `go mod tidy` verification
2. **Unit tests** — `go test ./...` inside a Docker container
3. **Docker image build** — multi-arch build (amd64 + arm64) verification
4. **Integration tests** — start the SUT container, run Go integration tests against it
5. **PUID/PGID test** — verify the entrypoint uid/gid handling
6. **GoReleaser snapshot** — build `.deb` packages without publishing
7. **Deb integration tests** — install the `.deb` on Ubuntu 22.04 and run integration tests

### `branch.yml` — Branch Image Builds

Triggers on pushes to any branch **except `main`**.

1. Builds multi-arch Docker images (amd64 + arm64)
2. Pushes to GHCR with the branch name as the tag:
   - `ghcr.io/jchonig/yaamon:<branch-name>`
   - `ghcr.io/jchonig/yaamon:<branch-name>-<sha7>` (pinned SHA for reproducibility)
3. No GitHub Release or `latest` tag update

Branch names become image tags; slashes are converted to dashes:

| Branch | Image tag |
|---|---|
| `feature/passkeys` | `ghcr.io/jchonig/yaamon:feature-passkeys` |
| `fix/auth-bug` | `ghcr.io/jchonig/yaamon:fix-auth-bug` |

**Testing a branch image in production**: temporarily change the image tag in your `docker-compose.yml`:

```yaml
image: ghcr.io/jchonig/yaamon:feature-passkeys
```

Restart and test. Revert to `latest` when done.

### `release.yml` — Release

Triggers on `v*` tag pushes (e.g. `v1.2.3`).

1. Runs the same pre-release tests as CI
2. Executes GoReleaser:
   - Cross-compiles Linux binaries for amd64 + arm64
   - Builds `.deb` packages (with systemd unit, post/pre-install scripts)
   - Builds multi-arch Docker images
   - Creates tar.gz archives with binary, README, LICENSE, example config
3. Publishes to GitHub Releases (with `RELEASE_NOTES.md` as release notes)
4. Pushes Docker images to GHCR:
   - `ghcr.io/jchonig/yaamon:<version>` (e.g. `v1.2.3`)
   - `ghcr.io/jchonig/yaamon:latest` (multi-arch manifest, skipped for pre-releases)

## GoReleaser configuration

Configured in `.goreleaser.yml`:

- **builds**: cross-compile to `linux/amd64` and `linux/arm64`
- **nfpms**: Debian package — maintainer, license, config file, systemd service, post-install/pre-remove scripts
- **archives**: tar.gz with README, LICENSE, example config, systemd unit
- **dockers**: multi-arch manifest push to GHCR
- **changelog**: auto-generated from commit messages (excludes docs/test/chore commits)

## Deployment topology (reference)

The author's production deployment:

```
Internet / Tailscale
        │
        ▼
  Caddy (macvlan IP, ports 80 + 443)
  ├── TLS termination (Let's Encrypt)
  ├── HTTP → HTTPS redirect
  ├── oauth2-proxy sidecar (forward auth, Kanidm OIDC)
  └── reverse proxy → yaamon:80 (bridge network, TLS disabled)
```

YAAMon is on the Docker bridge network and not directly reachable from outside. All auth headers are injected by Caddy/oauth2-proxy.
