# Komari Probe

<img src="docs/images/logo.svg" alt="Komari Probe logo" width="120">

[English](./README.md) | [简体中文](./README_zh-cn.md)

Komari Probe is a pure, lightweight, and secure self-hosted server monitoring solution. It provides a simple and efficient way to track server performance through a modern web interface, with metrics collected by a lightweight agent.

> [!WARNING]
> Komari Probe is a self-hosted monitoring and control application. Deploy it only on systems you own or are authorized to manage. You are solely responsible for how you deploy and use Komari Probe. The developers accept no liability for unauthorized access, persistence, command execution, other misuse, or any resulting consequences.

[Documentation](https://www.komari.wiki/) | [Telegram Group](https://t.me/komari_probe)

---

## Features

- **Real-time Monitoring**: Sub-second real-time metrics and network latency monitoring.
- **Lightweight & Efficient**: Minimal system footprint, ideal for small VPS instances and large bare-metal servers.
- **Self-Hosted & Private**: Total control over your metrics data with local persistent SQLite time-series storage.
- **Adaptive Dashboard**: Responsive web interface seamlessly adapted for both desktop and mobile screens.
- **Extensible Architecture**: Fully independent theme customization and plugin ecosystem.

---

## Release Channels

Choose the appropriate release channel based on your environment:

| Channel | Description | Docker Tag | Use Case |
| :--- | :--- | :--- | :--- |
| **Stable** | Thoroughly tested official release for maximum stability | `:latest` or `:1.0.0` | Recommended for production |
| **Preview / Beta** | Public beta testing releases with the latest refactored UI and features (e.g., `v1.0.0-beta.1`) | `:v1.0.0-beta.1` | Early testing and feature evaluation |
| **Snapshot** | Automated builds from the `main` branch containing recent commits and fixes | `:snapshot` | Developers and urgent bug testing |

---

## Fresh Installation

### 1. Server Installation

#### Method A: Host / systemd (Recommended)
Supported on Ubuntu, Debian, CentOS, Alpine, etc. Runs as a systemd service. Default data directory is `/opt/komari/data`, and default port is `25774`.

- **Stable Release**:
  ```bash
  curl -fsSL https://raw.githubusercontent.com/komari-probe/komari-probe/main/install-komari.sh | sudo bash
  ```
- **Preview / Beta Release (e.g., `v1.0.0-beta.1`)**:
  ```bash
  curl -fsSL https://raw.githubusercontent.com/komari-probe/komari-probe/main/install-komari.sh | sudo VERSION=v1.0.0-beta.1 bash
  ```
- **Snapshot Release**:
  ```bash
  curl -fsSL https://raw.githubusercontent.com/komari-probe/komari-probe/main/install-komari.sh | sudo CHANNEL=snapshot bash
  ```

#### Method B: Docker Compose
Create a `docker-compose.yml` file:

```yaml
services:
  komari:
    # Use :v1.0.0-beta.1 during the beta phase, or :latest once stable is published
    image: ghcr.io/komari-probe/komari-probe:v1.0.0-beta.1
    container_name: komari
    restart: unless-stopped
    ports:
      - "25774:25774"
    volumes:
      - ./data:/app/data
```

Start the service:
```bash
docker compose up -d
```

#### Method C: Docker CLI Single Container
```bash
docker run -d \
  --name komari \
  --restart unless-stopped \
  -p 25774:25774 \
  -v /opt/komari/data:/app/data \
  ghcr.io/komari-probe/komari-probe:v1.0.0-beta.1
```

> Once installed, access `http://<YOUR_SERVER_IP>:25774/` in your browser and complete the initial administrator setup.

---

### 2. Agent Installation

After deploying the Server, add a node in the Admin Panel (`/admin` -> Node Management) to generate an **Agent Token**. Then install the Agent on target client machines:

#### Method A: Host Installation (Recommended)
- **Stable Release**:
  ```bash
  curl -fsSL https://raw.githubusercontent.com/komari-probe/komari-probe-agent/main/install.sh | sudo bash -s -- \
    -e "http://<SERVER_IP>:25774" \
    -t "<YOUR_AGENT_TOKEN>"
  ```
- **Preview / Beta Release**:
  ```bash
  curl -fsSL https://raw.githubusercontent.com/komari-probe/komari-probe-agent/main/install.sh | sudo bash -s -- \
    -e "http://<SERVER_IP>:25774" \
    -t "<YOUR_AGENT_TOKEN>" \
    -v "v1.0.0-beta.1"
  ```

#### Method B: Docker Container
> [!IMPORTANT]
> The Agent container **must run with `--net=host`**, otherwise metrics will be collected from Docker's virtual bridge instead of the physical host.

```bash
docker run -d \
  --name komari-agent \
  --restart unless-stopped \
  --net=host \
  ghcr.io/komari-probe/komari-probe-agent:v1.0.0-beta.1 \
  -e "http://<SERVER_IP>:25774" -t "<YOUR_AGENT_TOKEN>"
```

For more details on CLI options and configuration files, see the [komari-probe-agent repository](https://github.com/komari-probe/komari-probe-agent).

---

## Migration from Komari Monitor

If you are already running an upstream **Komari Monitor** instance, automated, non-destructive migration scripts are provided to upgrade smoothly to Komari Probe.

### Safety & Guardrails
1. **Automated Full Backup**: Persistent data, databases, and ping tasks are tar-gzipped to `/var/backups/` with a SHA-256 manifest before any modification.
2. **Checksum Integrity Enforcement**: All downloaded binaries are strictly verified against the release `checksums.txt`. Any hash mismatch triggers a fail-safe abort without touching running services.
3. **Health Check & Auto-Rollback**: The script probes the local health endpoint after starting the upgraded service. If health checks fail, the previous binary and data archive are restored automatically.

---

### 1. Server Migration

#### Scenario A: Host Migration (systemd)
For instances deployed directly on bare-metal or VPS systems:

```bash
# 1. Download migration script
curl -fsSL https://raw.githubusercontent.com/komari-probe/komari-probe/main/scripts/migrate-server-host.sh -o migrate-server-host.sh

# 2. Dry run (validates environment and outputs plan without making changes)
sudo bash migrate-server-host.sh --tag v1.0.0-beta.1 --dry-run

# 3. Perform migration
sudo bash migrate-server-host.sh --tag v1.0.0-beta.1
```

> **Options**:
> - `--tag TAG`: Target release tag (e.g., `v1.0.0-beta.1`; defaults to `latest`).
> - `--service NAME`: systemd service name (default: `komari`).
> - `--data-dir PATH`: Data directory (default: `/opt/komari/data`).
> - `--port PORT`: Health check port (default: `25774`).
> - `--cleanup-backup <ID>`: Safely delete a specific timestamped backup directory after verifying migration.

#### Scenario B: Docker Compose Migration
For instances running via `docker compose`:

```bash
# 1. Download Docker migration script
curl -fsSL https://raw.githubusercontent.com/komari-probe/komari-probe/main/scripts/migrate-server-docker.sh -o migrate-server-docker.sh

# 2. Perform migration (pass existing compose file and service name)
sudo bash migrate-server-docker.sh \
  --compose-file /path/to/docker-compose.yml \
  --service komari \
  --target-image ghcr.io/komari-probe/komari-probe:v1.0.0-beta.1
```

> **Note**: Backs up the data mount to `/var/backups/komari-server-docker-migration/` and generates a non-destructive `.komari-probe-<SERVICE>.override.yml` override file. Your original `docker-compose.yml` remains untouched.

---

### 2. Agent Migration

To migrate existing upstream Agent nodes while **preserving `auto-discovery.json` and active tokens**:

#### Host Agent:
```bash
curl -fsSL https://raw.githubusercontent.com/komari-probe/komari-probe-agent/main/scripts/migrate-agent-host.sh -o migrate-agent-host.sh
sudo bash migrate-agent-host.sh --tag v1.0.0-beta.1
```

#### Docker Agent:
```bash
curl -fsSL https://raw.githubusercontent.com/komari-probe/komari-probe-agent/main/scripts/migrate-agent-docker.sh -o migrate-agent-docker.sh
sudo bash migrate-agent-docker.sh \
  --container komari-agent \
  --target-image ghcr.io/komari-probe/komari-probe-agent:v1.0.0-beta.1
```

---

## Service Management

### systemd Management
```bash
sudo systemctl status komari   # Check status
sudo systemctl restart komari  # Restart server
sudo journalctl -u komari -f   # View live logs
```

### Docker Management
```bash
docker compose ps              # Check container status
docker compose logs -f komari  # View live logs
docker compose restart komari  # Restart container
```

---

## Screenshots

| Page | Screenshot |
| :--- | :--- |
| Home Dashboard | <img src="https://b2.akz.moe/awesome-pictures/komari-screenshot/%E4%B8%BB%E9%A1%B5%E4%BB%AA%E8%A1%A8%E7%9B%98-en.webp" width="800" alt="Home Dashboard"> |
| Admin Dashboard | <img src="https://b2.akz.moe/awesome-pictures/komari-screenshot/%E5%90%8E%E5%8F%B0%E4%BB%AA%E8%A1%A8%E7%9B%98-en.webp" width="800" alt="Admin Dashboard"> |
| History Charts | <img src="https://b2.akz.moe/awesome-pictures/komari-screenshot/%E5%8E%86%E5%8F%B2%E5%9B%BE%E8%A1%A8-en.webp" width="800" alt="History Charts"> |
| Web Terminal | <img src="https://b2.akz.moe/awesome-pictures/komari-screenshot/%E7%BD%91%E9%A1%B5%E7%BB%88%E7%AB%AF.webp" width="800" alt="Web Terminal"> |
| Customizable Themes | <img src="https://b2.akz.moe/awesome-pictures/komari-screenshot/%E4%B8%BB%E9%A2%98%E5%8F%AF%E8%87%AA%E5%AE%9A%E4%B9%89-en.webp" width="800" alt="Customizable Themes"> |
| Theme Market | <img src="https://b2.akz.moe/awesome-pictures/komari-screenshot/%E4%B8%BB%E9%A2%98%E5%B8%82%E5%9C%BA-en.webp" width="800" alt="Theme Market"> |

---

## Contributors & Credits

Komari Probe began as a fork of [Komari Monitor](https://github.com/komari-monitor/komari). Thanks to everyone who built and contributed to the original project.

Thanks also to everyone who has contributed code, themes, plugins, documentation, translations, bug reports, or feedback to Komari Probe.

<a href="https://github.com/komari-probe/komari-probe/graphs/contributors"><img src="https://contributors-img.web.app/image?repo=komari-probe/komari-probe" alt="Komari Probe contributors" width="600"></a>
