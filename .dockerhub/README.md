# OpenPT

OpenPT is a lightweight, configuration-driven BitTorrent Tracker announce tool aimed at private tracker (PT) seeding scenarios. It only needs `.torrent` files and does not require downloading the real content; it simulates a client periodically reporting seeding status, uploaded amount, port and client identity to the Tracker. It ships with a built-in Web UI and exposes Prometheus-format metrics. It is for users who want to keep torrents "seeding" on a tracker without storing the actual data.

## Quick Start

Create `compose.yml`:

```yaml
services:
  openpt:
    image: chenbin3625/openpt:latest
    container_name: openpt
    restart: unless-stopped
    ports:
      - "127.0.0.1:9090:9090"
    environment:
      PUID: "1000"
      PGID: "1000"
    volumes:
      - ./data:/data
```

Start it:

```bash
docker compose up -d
```

If the current user on the host machine is not `1000:1000`, check the actual UID/GID first and change `PUID`/`PGID` to the matching values:

```sh
id -u
id -g
```

OpenPT runs as this user, which makes it easier to read `.torrent` files bind-mounted in. The container does not modify the ownership of directories or files that already exist on the host at startup.

Put `.torrent` files into:

```text
./data/torrents
```

Then open the Web UI at `http://127.0.0.1:9090`.

Restart the container after changing the configuration:

```sh
docker compose restart
```

## Image Tags

| Tag | Description |
| --- | --- |
| `latest` | Most recent tagged release; published when a `v*` tag is pushed |
| `<version>` | Full semantic version of a release, for example `1.2.3` |
| `<major>.<minor>` | Minor-version stream of a release, for example `1.2` |

Multi-platform images are published for `linux/amd64`, `linux/arm64` and `linux/arm/v7`. Pin a specific version tag instead of `latest` if you want reproducible upgrades.

## Key Features

- Automatically scans the `torrents` directory and loads `.torrent` files
- Limits the number of simultaneous seeds according to the configuration
- Supports client spoofing files for qBittorrent, Transmission, Deluge, uTorrent and others
- Three upload strategies: no accumulated upload, conservative rate, and custom rate
- Dynamically allocates upload speed based on the peers returned by the Tracker
- Supports a target share ratio, automatically stopping that torrent once it is reached
- Automatically retries with exponential backoff after a Tracker request fails
- Supports HTTP, HTTPS and UDP Trackers, plus BitTorrent v1, hybrid and v2-only info hashes
- Persists accumulated upload and completed status, so it can keep running after a restart
- Web UI for status, error details and the current configuration; Prometheus metrics and a health check endpoint

## Configuration

OpenPT is entirely configuration-driven by a single TOML file. In the Docker image the config lives at `/data/config.toml` (the container's default command is `--config /data/config.toml`). On first start, if `/data/config.toml` does not exist, the entrypoint creates it from the bundled `examples/config.docker.toml`, whose paths already point at the container layout (`/data/torrents`, `/data/clients`, `/data/torrents_archive`, `/data/openpt_state.json`).

A commonly used configuration example:

```toml
simultaneous_seed = 200
client = "qbittorrent-5.1.4.client"
torrents_dir = "./torrents"
clients_dir = "./clients"
scan_interval_seconds = 5
shutdown_stop_timeout_seconds = 20

[uploaded]
strategy = "configured_rate"
configured_rate_bps = 170000
# Set to 0 when you do not want a separate fluctuation range: random_jitter_percent generates one around configured_rate_bps
min_rate_bps = 0
max_rate_bps = 0
conservative_rate_bps = 1024
random_jitter_percent = 10
random_refresh_seconds = 1200
ratio_target = 0

[announce]
port = 0
ip = ""
ipv6 = ""

[tracker]
timeout_seconds = 15
proxy = ""
reuse_connections = true
max_idle_conns = 100
max_idle_conns_per_host = 10
idle_conn_timeout_seconds = 90
failure_backoff_min_seconds = 5
failure_backoff_max_seconds = 300

[metrics]
enabled = true
listen = "127.0.0.1:9090"
path = "/metrics"
webui = true

[logging]
file = ""
```

Key options:

- `simultaneous_seed`: number of simultaneous seeds. `0` means no limit, and OpenPT will try to load all torrents in the directory.
- `client`: the client spoofing file name, which must exist in the `clients_dir` directory.
- `torrents_dir`: the torrent file directory; OpenPT scans the `.torrent` files here.
- `clients_dir`: the client spoofing configuration directory.
- `archive_dir`: the archive directory for problematic torrents. Corrupted or unparsable `.torrent` files are moved here once the write is confirmed complete; files with the same name are not overwritten.
- `state_file`: the state persistence file, which stores accumulated upload and the status of torrents that have reached the target share ratio.
- `scan_interval_seconds`: the interval for scanning the torrent directory.
- `shutdown_stop_timeout_seconds`: the longest time to wait for the `stopped` announce to complete on exit.
- `announce.port`: recommended to be `0`, which generates a random dynamic port between `49152-65535` on every startup, avoiding common default ports such as `6881`. Set a specific value between `1-65535` only if you really need a fixed port. `announce.ip` and `announce.ipv6` can be left empty, letting the Tracker detect the address automatically.
- `uploaded.strategy`: `none` (no accumulated upload, the most conservative), `conservative_rate` (low-rate conservative upload) or `configured_rate` (custom upload rate). Common rate conversions: `100 KB/s` = `102400`, `500 KB/s` = `512000`, `1 MB/s` = `1048576`.
- `ratio_target`: the target share ratio. `0` means never stopping because of the share ratio; `2.0` means stopping that torrent after reaching a 2.0 share ratio.
- `tracker.timeout_seconds`: the Tracker request timeout.
- `tracker.proxy`: the proxy address, which may be empty. HTTP and SOCKS5 proxies are supported, for example `proxy = "http://127.0.0.1:7890"`. Leaving it empty connects to the Tracker directly: environment variables such as `HTTP_PROXY` / `HTTPS_PROXY` do not affect Tracker announce traffic, so configure this option explicitly when a proxy is needed. The proxy is only used for HTTP/HTTPS Trackers; with a proxy configured, UDP Trackers return a clear error and the scheduler keeps trying other Trackers in the torrent.
- `failure_backoff_min_seconds` and `failure_backoff_max_seconds`: control the exponential backoff retry range after a failure.
- `metrics.enabled`, `metrics.listen`, `metrics.path`, `metrics.webui`: enable the metrics endpoint and Web UI, and set where they listen. For LAN access inside a container, use `listen = "0.0.0.0:9090"` (as the bundled Docker example does).
- `logging.file`: log file path; empty means default logging behaviour.

Part of the configuration can be hot reloaded with `SIGHUP` (simultaneous seeds, upload strategy and rate, ratio target, shutdown `stopped` timeout, announce port/IP/IPv6, Tracker timeout/proxy/backoff). Changing client spoofing files, the torrents directory, the archive directory, the clients directory, the state file, the scan interval, the log file, or the metrics service switch, listen address, metrics path and Web UI switch requires a restart — in a container, `docker compose restart`.

## Volumes

The image declares `VOLUME ["/data"]` and everything persistent lives under `/data`, so a single bind mount is enough:

```yaml
volumes:
  - ./data:/data
```

| Path | Purpose |
| --- | --- |
| `/data/config.toml` | The TOML configuration file; created from `examples/config.docker.toml` on first start if absent |
| `/data/torrents` | Put your `.torrent` files here; scanned automatically |
| `/data/clients` | Client spoofing files; seeded from the image's bundled `clients` directory if absent |
| `/data/torrents_archive` | Corrupted or unparsable `.torrent` files are archived here |
| `/data/openpt_state.json` | State persistence: accumulated upload and completed status |

`torrents`, `clients`, `torrents_archive` and the parent `/data` directory are created automatically by the entrypoint when missing. Only the files and directories the entrypoint created are chowned to `PUID`/`PGID`; existing host files are left untouched. If existing files produce `permission denied`, make sure they are readable and writable by the user corresponding to `PUID/PGID`:

```sh
ls -ld ./data ./data/torrents ./data/torrents_archive
ls -l ./data/torrents
```

`PUID` and `PGID` (equivalently `OPENPT_UID` / `OPENPT_GID`, default `1000:1000`) set the user the process drops to. `OPENPT_DATA_DIR` and `OPENPT_APP_DIR` override the container data and application directories (`/data` and `/app` by default).

## Metrics

Metrics and the Web UI are served on port `9090` (the image `EXPOSE`s it and the entrypoint/Web UI use `metrics.listen`):

```text
Web UI:            http://127.0.0.1:9090
Prometheus metrics: http://127.0.0.1:9090/metrics
Health check:      http://127.0.0.1:9090/healthz
```

The health check address is always `/healthz`. `GET` and `HEAD` requests return `200 OK` on success, and the Docker image uses this endpoint for its container health check (`HEALTHCHECK` runs `wget -q -T 2 -O /dev/null http://127.0.0.1:9090/healthz` with an interval of 30s, a timeout of 3s and a start period of 10s). `/healthz` does not disappear when `metrics.enabled` is turned off: even with `enabled = false`, the program still listens on `metrics.listen` and serves only that endpoint (`/metrics` and the Web UI stop being served), so container health checks remain available at all times.

## Links

- GitHub: https://github.com/chenbin3625/OpenPT
- Releases: https://github.com/chenbin3625/OpenPT/releases

---

# 中文

OpenPT 是一个轻量级、配置驱动的 BitTorrent Tracker Announce 工具，面向 PT 站保种场景。它只需要 `.torrent` 文件，不需要真实下载内容，通过模拟客户端向 Tracker 定期汇报做种状态、上传量、端口、客户端标识等信息。它内置 Web UI，并暴露 Prometheus 格式指标。适合希望在不存储真实数据的情况下让种子在站点上保持"做种"状态的用户。

## 快速开始

创建 `compose.yml`：

```yaml
services:
  openpt:
    image: chenbin3625/openpt:latest
    container_name: openpt
    restart: unless-stopped
    ports:
      - "127.0.0.1:9090:9090"
    environment:
      PUID: "1000"
      PGID: "1000"
    volumes:
      - ./data:/data
```

启动：

```bash
docker compose up -d
```

如果宿主机当前用户不是 `1000:1000`，先查看实际 UID/GID，再把 `PUID`、`PGID` 改成对应数值：

```sh
id -u
id -g
```

OpenPT 会以这个用户运行，便于读取 bind mount 进来的 `.torrent` 文件。容器启动时不会修改宿主机上已经存在的目录或文件所有权。

将 `.torrent` 文件放入：

```text
./data/torrents
```

然后打开 Web UI：`http://127.0.0.1:9090`。

修改配置后重启容器：

```sh
docker compose restart
```

## 镜像标签

| 标签 | 说明 |
| --- | --- |
| `latest` | 最新的正式发布版本；推送 `v*` 标签时发布 |
| `<version>` | 完整的语义化版本号，例如 `1.2.3` |
| `<major>.<minor>` | 某个次版本系列的标签，例如 `1.2` |

镜像为多平台构建，覆盖 `linux/amd64`、`linux/arm64` 和 `linux/arm/v7`。如需可复现的升级，请固定使用具体版本标签，而不是 `latest`。

## 主要功能

- 自动扫描 `torrents` 目录并加载 `.torrent` 文件
- 按配置限制同时保种数量
- 支持 qBittorrent、Transmission、Deluge、uTorrent 等客户端伪装文件
- 支持不累计上传量、保守速率、自定义速率三种上传策略
- 根据 Tracker 返回的 peers 动态分配上传速度
- 支持目标分享率，达到后自动停止该种子
- Tracker 请求失败后自动指数退避重试
- 支持 HTTP、HTTPS 和 UDP Tracker，以及 BitTorrent v1、hybrid 和 v2-only 种子 info hash
- 持久化累计上传量和已完成状态，重启后可继续运行
- Web UI 展示状态、错误详情和当前配置；提供 Prometheus 指标和健康检查接口

## 配置方法

OpenPT 完全由单个 TOML 配置文件驱动。在 Docker 镜像中配置文件位于 `/data/config.toml`（容器默认启动命令为 `--config /data/config.toml`）。首次启动时如果 `/data/config.toml` 不存在，入口程序会从镜像内置的 `examples/config.docker.toml` 创建它，其中的路径已对应容器目录结构（`/data/torrents`、`/data/clients`、`/data/torrents_archive`、`/data/openpt_state.json`）。

一个常用配置示例：

```toml
simultaneous_seed = 200
client = "qbittorrent-5.1.4.client"
torrents_dir = "./torrents"
clients_dir = "./clients"
scan_interval_seconds = 5
shutdown_stop_timeout_seconds = 20

[uploaded]
strategy = "configured_rate"
configured_rate_bps = 170000
# 不单独限制波动范围时置 0：由 random_jitter_percent 在 configured_rate_bps 周围生成
min_rate_bps = 0
max_rate_bps = 0
conservative_rate_bps = 1024
random_jitter_percent = 10
random_refresh_seconds = 1200
ratio_target = 0

[announce]
port = 0
ip = ""
ipv6 = ""

[tracker]
timeout_seconds = 15
proxy = ""
reuse_connections = true
max_idle_conns = 100
max_idle_conns_per_host = 10
idle_conn_timeout_seconds = 90
failure_backoff_min_seconds = 5
failure_backoff_max_seconds = 300

[metrics]
enabled = true
listen = "127.0.0.1:9090"
path = "/metrics"
webui = true

[logging]
file = ""
```

主要配置项：

- `simultaneous_seed`：同时保种数量。设置为 `0` 表示不限制数量，会尽量加载目录中的全部种子。
- `client`：客户端伪装文件名，需要存在于 `clients_dir` 目录中。
- `torrents_dir`：种子文件目录，OpenPT 会扫描这里的 `.torrent` 文件。
- `clients_dir`：客户端伪装配置目录。
- `archive_dir`：问题种子归档目录。损坏或无法解析的 `.torrent` 文件会在确认写入完成后移动到这里；同名文件不会被覆盖。
- `state_file`：状态持久化文件，保存累计上传量和已达到目标分享率的种子状态。
- `scan_interval_seconds`：扫描种子目录的间隔。
- `shutdown_stop_timeout_seconds`：退出时等待 `stopped` 上报完成的最长时间。
- `announce.port`：推荐设置为 `0`，会在每次启动时随机生成 `49152-65535` 之间的动态端口，避免使用 `6881` 等常见默认端口。确实需要固定端口时再设置为 `1-65535` 之间的具体值。`announce.ip` 和 `announce.ipv6` 默认留空即可，让 Tracker 自动识别地址。
- `uploaded.strategy`：支持 `none`（不累计上传量，最保守）、`conservative_rate`（低速保守上传量）和 `configured_rate`（自定义上传速率）三种模式。常见速率换算：`100 KB/s` = `102400`，`500 KB/s` = `512000`，`1 MB/s` = `1048576`。
- `ratio_target`：目标分享率。设置为 `0` 表示永不因分享率停止；设置为 `2.0` 表示达到 2.0 分享率后停止该种子。
- `tracker.timeout_seconds`：Tracker 请求超时时间。
- `tracker.proxy`：代理地址，可为空。支持 HTTP 代理或 SOCKS5 代理，例如 `proxy = "http://127.0.0.1:7890"`。留空表示直连 Tracker：`HTTP_PROXY` / `HTTPS_PROXY` 等环境变量不会影响 Tracker 上报流量，需要代理时请显式配置此项。代理仅用于 HTTP/HTTPS Tracker；配置代理后，UDP Tracker 会返回明确错误，调度器会继续尝试种子中的其它 Tracker。
- `failure_backoff_min_seconds` 和 `failure_backoff_max_seconds`：控制失败后的指数退避重试范围。
- `metrics.enabled`、`metrics.listen`、`metrics.path`、`metrics.webui`：启用监控接口和 Web UI，并设置监听地址。容器内如需局域网访问，可改为 `listen = "0.0.0.0:9090"`（镜像内置的 Docker 示例即如此配置）。
- `logging.file`：日志文件路径，留空使用默认日志行为。

部分配置支持通过 `SIGHUP` 热重载（同时保种数量、上传策略和速率、分享率目标、关闭时等待 `stopped` 上报的超时时间、Announce 端口/IP/IPv6、Tracker 超时/代理/退避配置）。客户端伪装文件、种子目录、问题种子归档目录、客户端配置目录、状态文件、种子扫描间隔、日志文件，以及监控服务开关、监听地址、指标路径和 Web UI 开关需重启后生效 —— 在容器中即执行 `docker compose restart`。

## 卷挂载

镜像声明了 `VOLUME ["/data"]`，所有需要持久化的内容都在 `/data` 下，因此一个 bind mount 即可：

```yaml
volumes:
  - ./data:/data
```

| 路径 | 用途 |
| --- | --- |
| `/data/config.toml` | TOML 配置文件；首次启动时若不存在，会从 `examples/config.docker.toml` 创建 |
| `/data/torrents` | 放置 `.torrent` 文件，会被自动扫描 |
| `/data/clients` | 客户端伪装文件；缺失时会从镜像内置的 `clients` 目录复制 |
| `/data/torrents_archive` | 损坏或无法解析的 `.torrent` 文件归档目录 |
| `/data/openpt_state.json` | 状态持久化：累计上传量和已完成状态 |

`torrents`、`clients`、`torrents_archive` 及上级 `/data` 目录不存在时会由入口程序自动创建。只有入口程序创建的文件和目录会被 chown 为 `PUID`/`PGID`，宿主机上已存在的文件不会被改动。如果已有文件出现 `permission denied`，确认它们对 `PUID/PGID` 对应的用户可读写：

```sh
ls -ld ./data ./data/torrents ./data/torrents_archive
ls -l ./data/torrents
```

`PUID` 与 `PGID`（等价写法为 `OPENPT_UID` / `OPENPT_GID`，默认 `1000:1000`）决定进程降权后使用的用户。`OPENPT_DATA_DIR` 与 `OPENPT_APP_DIR` 可覆盖容器内的数据目录和应用目录（默认为 `/data` 和 `/app`）。

## 指标

指标和 Web UI 都监听 `9090` 端口（镜像已 `EXPOSE` 该端口，入口程序与 Web UI 使用 `metrics.listen`）：

```text
Web UI:             http://127.0.0.1:9090
Prometheus 指标:    http://127.0.0.1:9090/metrics
健康检查:           http://127.0.0.1:9090/healthz
```

健康检查地址始终为 `/healthz`。`GET` 和 `HEAD` 请求成功时返回 `200 OK`，Docker 镜像也使用此接口执行容器健康检查（`HEALTHCHECK` 执行 `wget -q -T 2 -O /dev/null http://127.0.0.1:9090/healthz`，间隔 30s、超时 3s、启动等待 10s）。`/healthz` 不随 `metrics.enabled` 关闭而消失：即使 `enabled = false`，程序仍会监听 `metrics.listen` 并只提供该接口（`/metrics` 与 Web UI 停止服务），以保证容器健康检查始终可用。

## 链接

- GitHub: https://github.com/chenbin3625/OpenPT
- Releases: https://github.com/chenbin3625/OpenPT/releases
