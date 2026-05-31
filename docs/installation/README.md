# Installation

Choose the method that fits your environment:

| Method | Best for |
|--------|----------|
| [Debian / Ubuntu package](deb.md) | ASL3 nodes (Raspberry Pi, x86 server) — recommended |
| [Pre-built binary (tarball)](binary.md) | Non-Debian Linux, manual systemd setup |
| [Docker](docker.md) | Quick start, isolated environment |
| [docker-compose](docker-compose.md) | Production Docker deployments |
| [Building from source](building.md) | Development, custom builds |

After installation, open `http://<your-host>:8080/` in a browser. On first visit you will be taken to the setup page to create your admin account.

## Migrating from AllScan or Allmon3?

See [Migration](migration.md) — YAAMon has built-in import support with no conversion scripts needed.
