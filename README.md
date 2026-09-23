# Sonar

<img src="docs/images/logo.svg" alt="Sonar logo" width="120">

[English](./README.md) | [简体中文](./README_zh-cn.md)

Sonar is a pure, lightweight, and secure self-hosted server monitoring solution. It provides a simple and efficient way to track server performance through a modern web interface, with metrics collected by a lightweight agent.

> [!WARNING]
> Sonar is a self-hosted monitoring and control application. Deploy it only on systems you own or are authorized to manage. You are solely responsible for how you deploy and use Sonar. The developers accept no liability for unauthorized access, persistence, command execution, other misuse, or any resulting consequences.

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
| **Preview / Beta** | Public beta testing releases with the latest refactored UI and features (e.g., `v1.1.0-beta.3`, check the [Releases](https://github.com/sonar-probe/sonar/releases) page for the current one) | `:v1.1.0-beta.3` | Early testing and feature evaluation |
| **Snapshot** | Automated builds from the `main` branch containing recent commits and fixes | `:snapshot` | Developers and urgent bug testing |

---

## Fresh Installation

### 1. Server Installation

#### Method A: Host / systemd (Recommended)
Supported on Ubuntu, Debian, CentOS, Alpine, etc. Runs as a systemd service. Default data directory is `/opt/komari/data`, and default port is `25774`.

> [!IMPORTANT]
> `install-sonar.sh` is interactive (it asks you to pick a language, release channel, and listen port) — **download it first, then run it**. Don't pipe it straight into `sudo bash`: the pipe consumes stdin, so the script can never read your keystrokes and gets stuck re-printing "invalid option" at the selection menu.

```bash
curl -fsSL https://raw.githubusercontent.com/sonar-probe/sonar/main/install-sonar.sh -o install-sonar.sh
sudo bash install-sonar.sh
```

The script will prompt you to pick a release channel (stable / snapshot) interactively. You can also skip the prompt with an environment variable:

- **Install a specific version**:
  ```bash
  curl -fsSL https://raw.githubusercontent.com/sonar-probe/sonar/main/install-sonar.sh -o install-sonar.sh
  sudo VERSION=v1.1.0-beta.3 bash install-sonar.sh
  ```
- **Snapshot Release**:
  ```bash
  curl -fsSL https://raw.githubusercontent.com/sonar-probe/sonar/main/install-sonar.sh -o install-sonar.sh
  sudo CHANNEL=snapshot bash install-sonar.sh
  ```

#### Method B: Docker Compose
Create a `docker-compose.yml` file:

```yaml
services:
  sonar:
    # Use :v1.1.0-beta.3 during the beta phase (check Releases for the current tag), or :latest once stable is published
    image: ghcr.io/sonar-probe/sonar:v1.1.0-beta.3
    container_name: sonar
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
  --name sonar \
  --restart unless-stopped \
  -p 25774:25774 \
  -v /opt/komari/data:/app/data \
  ghcr.io/sonar-probe/sonar:v1.1.0-beta.3
```

> Once installed, access `http://<YOUR_SERVER_IP>:25774/` in your browser and complete the initial administrator setup.

---

### 2. Agent Installation

Connecting client nodes to your server takes only three simple steps:

1. **Open Admin Panel**: Navigate to `http://<YOUR_SERVER_IP>:25774/` and log in to the Admin Dashboard (`/admin`);
2. **Add Node**: Click **Node Management** in the left sidebar $\rightarrow$ click **Add Node**;
3. **One-Click Copy & Deploy**: In the install modal, **the system automatically generates the complete command containing your panel URL and token**. Simply click **Copy** and paste it into your target machine terminal — no manual configuration needed!

*(The Admin Panel natively supports auto-generating commands for Linux, Windows PowerShell, macOS, and Docker)*

---

<details>
<summary><b>🛠️ Manual Command Reference (For CI/CD & Automated Scripts)</b></summary>

If you need to install agents silently via automation scripts without using the web UI:

#### Host Installation (Linux / macOS)
- **Stable Release**:
  ```bash
  curl -fsSL https://raw.githubusercontent.com/sonar-probe/sonar-agent/main/install.sh | sudo bash -s -- \
    -e "http://<SERVER_IP>:25774" \
    -t "<AGENT_TOKEN>"
  ```
- **Preview / Beta Release**:
  ```bash
  curl -fsSL https://raw.githubusercontent.com/sonar-probe/sonar-agent/main/install.sh | sudo bash -s -- \
    -e "http://<SERVER_IP>:25774" \
    -t "<AGENT_TOKEN>" \
    -v "v1.1.0-beta.2"
  ```

#### Docker Container
> [!IMPORTANT]
> The Agent container **must run with `--net=host`**, otherwise metrics will be collected from Docker's virtual bridge rather than the physical host.

```bash
docker run -d \
  --name sonar-agent \
  --restart unless-stopped \
  --net=host \
  ghcr.io/sonar-probe/sonar-agent:v1.1.0-beta.2 \
  -e "http://<SERVER_IP>:25774" -t "<AGENT_TOKEN>"
```

For configuration files and parameter details, see the [komari-probe-agent repository](https://github.com/sonar-probe/sonar-agent).
</details>

---

## Migration from Komari Monitor

If you are already running an upstream **Komari Monitor** instance, automated, non-destructive migration scripts are provided to upgrade smoothly to Sonar.

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
curl -fsSL https://raw.githubusercontent.com/sonar-probe/sonar/main/scripts/migrate-server-host.sh -o migrate-server-host.sh

# 2. Dry run (validates environment and outputs plan without making changes)
sudo bash migrate-server-host.sh --tag v1.1.0-beta.3 --dry-run

# 3. Perform migration
sudo bash migrate-server-host.sh --tag v1.1.0-beta.3
```

> **Options**:
> - `--tag TAG`: Target release tag (e.g., `v1.1.0-beta.3`, check [Releases](https://github.com/sonar-probe/sonar/releases) for the current one; defaults to `latest`).
> - `--service NAME`: systemd service name (default: `komari`).
> - `--data-dir PATH`: Data directory (default: `/opt/komari/data`).
> - `--port PORT`: Health check port (default: `25774`).
> - `--cleanup-backup <ID>`: Safely delete a specific timestamped backup directory after verifying migration.

#### Scenario B: Docker Compose Migration
For instances running via `docker compose`:

```bash
# 1. Download Docker migration script
curl -fsSL https://raw.githubusercontent.com/sonar-probe/sonar/main/scripts/migrate-server-docker.sh -o migrate-server-docker.sh

# 2. Perform migration (pass existing compose file and service name)
sudo bash migrate-server-docker.sh \
  --compose-file /path/to/docker-compose.yml \
  --service sonar \
  --target-image ghcr.io/sonar-probe/sonar:v1.1.0-beta.3
```

> **Note**: Backs up the data mount to `/var/backups/sonar-server-docker-migration/` and generates a non-destructive `.sonar-<SERVICE>.override.yml` override file. Your original `docker-compose.yml` remains untouched.

---

### 2. Agent Migration

To migrate existing upstream Agent nodes while **preserving `auto-discovery.json` and active tokens**:

#### Host Agent:
```bash
curl -fsSL https://raw.githubusercontent.com/sonar-probe/sonar-agent/main/scripts/migrate-agent-host.sh -o migrate-agent-host.sh
sudo bash migrate-agent-host.sh --tag v1.1.0-beta.2
```

#### Docker Agent:
```bash
curl -fsSL https://raw.githubusercontent.com/sonar-probe/sonar-agent/main/scripts/migrate-agent-docker.sh -o migrate-agent-docker.sh
sudo bash migrate-agent-docker.sh \
  --container sonar-agent \
  --target-image ghcr.io/sonar-probe/sonar-agent:v1.1.0-beta.2
```

---

## Service Management

### systemd Management
```bash
sudo systemctl status sonar   # Check status
sudo systemctl restart sonar  # Restart server
sudo journalctl -u sonar -f   # View live logs
```

### Docker Management
```bash
docker compose ps              # Check container status
docker compose logs -f sonar  # View live logs
docker compose restart sonar  # Restart container
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

Sonar began as a fork of [Komari Monitor](https://github.com/komari-monitor/komari). Thanks to everyone who built and contributed to the original project.

Thanks also to everyone who has contributed code, themes, plugins, documentation, translations, bug reports, or feedback to Sonar.

<a href="https://github.com/sonar-probe/sonar/graphs/contributors"><img src="https://contributors-img.web.app/image?repo=sonar-probe/sonar" alt="Sonar contributors" width="600"></a>
