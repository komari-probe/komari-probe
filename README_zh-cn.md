# Komari Probe

<img src="docs/images/logo.svg" alt="Komari Probe logo" width="120">

[English](./README.md) | [简体中文](./README_zh-cn.md)

Komari Probe 是一款纯粹、轻量、安全的自托管服务器监控工具，旨在提供简单、高效的服务器性能监控解决方案。它支持通过 Web 界面查看服务器状态，并通过轻量级 Agent 收集数据。

> [!WARNING]
> Komari Probe 是一款自托管的监控/控制程序，仅应部署在你拥有或已获得授权管理的系统上。在未获授权的情况下部署、访问、持久化、执行命令及从事其他滥用行为，用户需要自行承担部署和使用 Komari Probe 的责任。开发者不对未经授权或滥用行为及其后果承担责任。

[文档](https://www.komari.wiki/) | [Telegram 群](https://t.me/komari_probe)

---

## 特性

- **实时监控**：秒级实时数据展示与网络延迟监测。
- **轻量高效**：极低系统资源占用，适合轻量 VPS 及各类物理服务器。
- **数据自主**：完全掌控数据隐私，历史时序指标本地持久化存储。
- **自适应界面**：现代响应式监控仪表盘，移动端/桌面端完美适配。
- **生态扩展**：支持独立主题定制与插件生态系统。

---

## 发布通道说明 (Release Channels)

在部署或迁移前，请根据场景选择合适的发布通道：

| 通道 | 说明 | Docker 标签 | 适用场景 |
| :--- | :--- | :--- | :--- |
| **正式稳定版 (Stable)** | 经过充分测试验证的正式版本，追求极致稳定性 | `:latest` 或 `:1.0.0` | 生产环境推荐 |
| **预览体验版 (Preview / Beta)** | 包含最新重构界面与最新功能体验（如当前 `v1.0.0-beta.1`） | `:v1.0.0-beta.1` | 公测尝鲜与新特性验证 |
| **开发快照版 (Snapshot)** | 主分支自动构建的代码快照，包含最新即时修复 | `:snapshot` | 开发者与抢先排错 |

---

## 全新安装指南 (Fresh Installation)

### 1. 服务端全新安装（Server）

#### 方式 A：宿主机一键安装（推荐）
适用于 Ubuntu / Debian / CentOS / Alpine 等常见 Linux 发行版，以 systemd 服务运行。默认数据目录为 `/opt/komari/data`，默认端口为 `25774`。

- **正式稳定版（Stable）**：
  ```bash
  curl -fsSL https://raw.githubusercontent.com/komari-probe/komari-probe/main/install-komari.sh | sudo bash
  ```
- **预览体验版（Pre-release / Beta，如 `v1.0.0-beta.1`）**：
  ```bash
  curl -fsSL https://raw.githubusercontent.com/komari-probe/komari-probe/main/install-komari.sh | sudo VERSION=v1.0.0-beta.1 bash
  ```
- **开发快照版（Snapshot）**：
  ```bash
  curl -fsSL https://raw.githubusercontent.com/komari-probe/komari-probe/main/install-komari.sh | sudo CHANNEL=snapshot bash
  ```

#### 方式 B：Docker Compose 部署
编写 `docker-compose.yml` 文件：

```yaml
services:
  komari:
    # 预览版使用 :v1.0.0-beta.1；正式版发布后可使用 :latest
    image: ghcr.io/komari-probe/komari-probe:v1.0.0-beta.1
    container_name: komari
    restart: unless-stopped
    ports:
      - "25774:25774"
    volumes:
      - ./data:/app/data
```

启动服务：
```bash
docker compose up -d
```

#### 方式 C：Docker CLI 单容器运行
```bash
docker run -d \
  --name komari \
  --restart unless-stopped \
  -p 25774:25774 \
  -v /opt/komari/data:/app/data \
  ghcr.io/komari-probe/komari-probe:v1.0.0-beta.1
```

> 安装完成后，在浏览器中访问 `http://<服务器IP>:25774/`，按照初始引导设置管理员账号密码即可开始使用。

---

### 2. 探针客户端全新接入（Agent）

部署并启动服务端后，接入被监控节点只需三步：

1. **进入管理后台**：浏览器打开 `http://<服务器IP>:25774/`，登录后进入后台管理（`/admin`）；
2. **添加节点**：点击左侧导航栏的 **【节点管理】** $\rightarrow$ **【添加节点】**；
3. **一键复制部署**：在弹出的安装窗口中，**系统已自动拼接好当前面板地址与专属 Token 的一键安装指令**，直接点击**【复制】**并粘贴到被监控机器终端运行即可，无需手动替换任何参数！

*(后台已原生支持 Linux 一键命令、Windows PowerShell、macOS 及 Docker 指令的自动生成与复制)*

---

<details>
<summary><b>🛠️ 附：手动安装命令参考（供自动化脚本 / 无面板环境备查）</b></summary>

若需在 CI/CD 或批量运维脚本中静默安装，可手动拼接参数执行：

#### 宿主机安装（Linux / macOS）
- **正式稳定版（Stable）**：
  ```bash
  curl -fsSL https://raw.githubusercontent.com/komari-probe/komari-probe-agent/main/install.sh | sudo bash -s -- \
    -e "http://<服务端IP或域名>:25774" \
    -t "<AGENT_TOKEN>"
  ```
- **预览体验版（Beta）**：
  ```bash
  curl -fsSL https://raw.githubusercontent.com/komari-probe/komari-probe-agent/main/install.sh | sudo bash -s -- \
    -e "http://<服务端IP或域名>:25774" \
    -t "<AGENT_TOKEN>" \
    -v "v1.0.0-beta.1"
  ```

#### Docker 容器运行
> [!IMPORTANT]
> Agent 容器**必须使用 `--net=host`**，否则采集到的将是 Docker 虚拟网桥指标而非物理主机的真实 CPU、内存及网络数据。

```bash
docker run -d \
  --name komari-agent \
  --restart unless-stopped \
  --net=host \
  ghcr.io/komari-probe/komari-probe-agent:v1.0.0-beta.1 \
  -e "http://<服务端IP或域名>:25774" -t "<AGENT_TOKEN>"
```

更多配置项与 CLI 字典请参阅 [komari-probe-agent 仓库说明](https://github.com/komari-probe/komari-probe-agent)。
</details>

---

## 从原版 Komari Monitor 平滑迁移指南 (Migration)

如果你此前已经部署了原版 **Komari Monitor**，本项目提供经过全量数据实测验证的**全自动无感迁移工具**，助你无损切换至 Komari Probe。

### 安全保障机制
1. **自动归档备份**：迁移前自动打包全部节点数据、Ping 任务、配置及历史时序数据库 `metrics.db` 至 `/var/backups/`，并生成 SHA-256 校验和；
2. **防篡改安全熔断**：下载 Release 产物时严格校验 GitHub Release 的 `checksums.txt`，哈希不匹配时立即中断，绝不篡改已有系统；
3. **就绪探测与自动回滚**：新版本启动后执行本地健康探测，若启动失败会自动还原旧二进制、解压恢复数据卷并重启旧服务。

---

### 1. 服务端迁移（Server Migration）

#### 场景一：宿主机原版迁移（Host Migration）
适用于原版通过 systemd 服务运行的实例：

```bash
# 1. 下载服务端宿主机迁移脚本
curl -fsSL https://raw.githubusercontent.com/komari-probe/komari-probe/main/scripts/migrate-server-host.sh -o migrate-server-host.sh

# 2. 预检执行（Dry-Run，仅评估升级路径与备份目录，不作修改）
sudo bash migrate-server-host.sh --tag v1.0.0-beta.1 --dry-run

# 3. 正式执行平滑迁移
sudo bash migrate-server-host.sh --tag v1.0.0-beta.1
```

> **常用参数**：
> - `--tag TAG`：指定目标版本（如 `v1.0.0-beta.1`；正式版发布后默认最新稳定版）。
> - `--service NAME`：systemd 服务名称（默认 `komari`）。
> - `--data-dir PATH`：数据目录（默认 `/opt/komari/data`）。
> - `--port PORT`：本地健康探测端口（默认 `25774`）。
> - `--cleanup-backup <ID>`：迁移验证完成后，按需清理指定时间戳备份目录。

#### 场景二：Docker Compose 原版迁移（Docker Migration）
适用于原版使用 `docker compose` 运行的实例：

```bash
# 1. 下载 Docker 迁移脚本
curl -fsSL https://raw.githubusercontent.com/komari-probe/komari-probe/main/scripts/migrate-server-docker.sh -o migrate-server-docker.sh

# 2. 执行平滑迁移（指定原始 docker-compose.yml 路径与 service 名称）
sudo bash migrate-server-docker.sh \
  --compose-file /path/to/docker-compose.yml \
  --service komari \
  --target-image ghcr.io/komari-probe/komari-probe:v1.0.0-beta.1
```

> **说明**：脚本会自动归档数据卷至 `/var/backups/komari-server-docker-migration/`，并在 Compose 目录生成轻量级 `.komari-probe-<SERVICE>.override.yml` 覆盖文件。原始的 `docker-compose.yml` 保持原样零污染，后续 Compose 指令将自动平滑运行 Komari Probe 镜像。

---

### 2. 客户端迁移（Agent Migration）

原版已在运行的探针节点，使用客户端迁移脚本升级，**会自动完整保留原有的 `auto-discovery.json` 与已连接 Token**：

#### 宿主机 Agent 迁移：
```bash
curl -fsSL https://raw.githubusercontent.com/komari-probe/komari-probe-agent/main/scripts/migrate-agent-host.sh -o migrate-agent-host.sh
sudo bash migrate-agent-host.sh --tag v1.0.0-beta.1
```

#### Docker Agent 迁移：
```bash
curl -fsSL https://raw.githubusercontent.com/komari-probe/komari-probe-agent/main/scripts/migrate-agent-docker.sh -o migrate-agent-docker.sh
sudo bash migrate-agent-docker.sh \
  --container komari-agent \
  --target-image ghcr.io/komari-probe/komari-probe-agent:v1.0.0-beta.1
```

---

## 常用服务管理命令

### 宿主机管理（systemd）
```bash
sudo systemctl status komari   # 查看服务端状态
sudo systemctl restart komari  # 重启服务端
sudo journalctl -u komari -f   # 跟踪实时日志
```

### Docker 容器管理
```bash
docker compose ps              # 查看运行状态
docker compose logs -f komari  # 查看实时日志
docker compose restart komari  # 重启服务容器
```

---

## 截图

| 页面         | 截图                                                                                                                                                         |
| ------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| 主页仪表盘   | <img src="https://b2.akz.moe/awesome-pictures/komari-screenshot/%E4%B8%BB%E9%A1%B5%E4%BB%AA%E8%A1%A8%E7%9B%98.webp" width="800" alt="主页仪表盘">            |
| 后台仪表盘   | <img src="https://b2.akz.moe/awesome-pictures/komari-screenshot/%E5%90%8E%E5%8F%B0%E4%BB%AA%E8%A1%A8%E7%9B%98.webp" width="800" alt="后台仪表盘">            |
| 历史图表     | <img src="https://b2.akz.moe/awesome-pictures/komari-screenshot/%E5%8E%86%E5%8F%B2%E5%9B%BE%E8%A1%A8.webp" width="800" alt="历史图表">                       |
| 网页终端     | <img src="https://b2.akz.moe/awesome-pictures/komari-screenshot/%E7%BD%91%E9%A1%B5%E7%BB%88%E7%AB%AF.webp" width="800" alt="网页终端">                       |
| 主题可自定义 | <img src="https://b2.akz.moe/awesome-pictures/komari-screenshot/%E4%B8%BB%E9%A2%98%E5%8F%AF%E8%87%AA%E5%AE%9A%E4%B9%89.webp" width="800" alt="主题可自定义"> |
| 主题市场     | <img src="https://b2.akz.moe/awesome-pictures/komari-screenshot/%E4%B8%BB%E9%A2%98%E5%B8%82%E5%9C%BA.webp" width="800" alt="主题市场">                       |

---

## 贡献者与致谢

Komari Probe 最初 fork 自 [Komari Monitor](https://github.com/komari-monitor/komari)，衷心感谢所有构建和贡献过原项目的朋友。

也感谢所有为 Komari Probe 贡献代码、主题、插件、文档、翻译、问题报告或反馈的朋友。

<a href="https://github.com/komari-probe/komari-probe/graphs/contributors"><img src="https://contributors-img.web.app/image?repo=komari-probe/komari-probe" alt="Komari Probe 贡献者" width="600"></a>
