<p align="center">
  <img src="internal/web/openpt-icon.svg" width="104" alt="OpenPT logo">
</p>

<h1 align="center">OpenPT</h1>

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE) [![Release](https://img.shields.io/github/v/release/chenbin3625/OpenPT)](https://github.com/chenbin3625/OpenPT/releases) [![Docker Pulls](https://img.shields.io/docker/pulls/chenbin3625/openpt)](https://hub.docker.com/r/chenbin3625/openpt)

OpenPT is a lightweight, configuration-driven BitTorrent Tracker announce tool aimed at private tracker (PT) seeding scenarios. It only needs `.torrent` files and does not require downloading the real content; it simulates a client periodically reporting seeding status, uploaded amount, port, client identity and other information to the Tracker.

OpenPT has a built-in Web UI that shows active torrents, upload speed, share ratio, Tracker status, last announce time, next announce time and error details in real time. The default configuration generates a random announce port on every startup, avoiding continued use of common fixed default ports.

## Key Features

- Automatically scans the `torrents` directory and loads `.torrent` files
- Limits the number of simultaneous seeds according to the configuration
- Supports client spoofing files for qBittorrent, Transmission, Deluge, uTorrent and others
- Supports three upload strategies: no accumulated upload, conservative rate, and custom rate
- Dynamically allocates upload speed based on the peers returned by the Tracker
- Supports a target share ratio, automatically stopping that torrent once it is reached
- Automatically retries with exponential backoff after a Tracker request fails
- Supports HTTP, HTTPS and UDP Trackers
- Supports BitTorrent v1, hybrid and v2-only torrent info hashes
- Automatically archives corrupted or unparsable torrent files, avoiding repeated load failures
- Persists accumulated upload and completed status, so it can keep running after a restart
- Automatically sends a `stopped` announce and releases the slot when a torrent file is deleted
- Web UI shows status, error details and the current configuration in real time
- Exposes Prometheus-format metrics and a health check endpoint
- Supports hot reloading of part of the configuration via `SIGHUP`

## Quick Start

### Running Locally

1. Prepare the program files and the configuration file:

```sh
cp examples/config.example.toml config.toml
```

2. Edit `config.toml`:

```sh
nano config.toml
```

3. Create the torrent directory and put `.torrent` files in it:

```sh
mkdir -p torrents
```

4. Start OpenPT:

```sh
./openpt --config config.toml
```

5. Open the Web UI:

```text
http://127.0.0.1:9090
```

### Running with Docker

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

If the current user on the host machine is not `1000:1000`, first check the actual UID/GID:

```sh
id -u
id -g
```

Then change `PUID` and `PGID` to the corresponding values. OpenPT will run as this user, which makes it easier to read `.torrent` files bind-mounted in. The container does not modify the ownership of directories or files that already exist on the host at startup.

Start it:

```sh
docker compose up -d
```

Put `.torrent` files into:

```text
./data/torrents
```

If existing files produce `permission denied`, make sure they are readable and writable by the user corresponding to `PUID/PGID`. For example, to check permissions only:

```sh
ls -ld ./data ./data/torrents ./data/torrents_archive
ls -l ./data/torrents
```

Restart the container after changing the configuration:

```sh
docker compose restart
```

## Building from Source

The Web UI is built with Vite + React + antd, and the build output is embedded into the binary via `go:embed`.

**The embedded frontend output (`internal/web/dist/`) is committed to the repository**, so after a fresh clone you can run
`go build` / `go test` directly, without building the frontend manually first:

```sh
go build -o openpt ./cmd/openpt
```

Only after modifying `frontend/src/**` do you need to rebuild the frontend output (output to `internal/web/dist`,
and committed together with the change, keeping it in sync with the source):

```sh
# 1. Build the frontend (output to internal/web/dist)
cd frontend
npm ci
npm run build
cd ..

# 2. Build the Go binary (with the embedded frontend output)
go build -o openpt ./cmd/openpt
```

> For frontend development, you can run `npm run dev` in the `frontend/` directory; Vite starts a local dev server
> and proxies to the OpenPT API. Release builds use the embedded output directly, with no extra asset directory needed.

> Note: after running `npm install` locally, `frontend/node_modules/flatted` ships with a Go
> reference implementation, which gets compiled by wildcard-driven tools such as `go build ./...` and `go test ./...`
> (showing up as `openpt/frontend/node_modules/...` packages). That directory is excluded by `.gitignore`,
> so it does not affect CI or release artifacts, and the risk of a build failure is zero; if you want to avoid it, run the Go commands after building the frontend.

## Configuration

OpenPT uses a TOML configuration file. It is recommended to copy it from the example file:

```sh
cp examples/config.example.toml config.toml
```

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

### Core Configuration

`simultaneous_seed`: number of simultaneous seeds. Setting it to `0` means no limit, and OpenPT will try to load all torrents in the directory.

`client`: the client spoofing file name, which must exist in the `clients_dir` directory.

`torrents_dir`: the torrent file directory; OpenPT scans the `.torrent` files here.

`clients_dir`: the client spoofing configuration directory.

`archive_dir`: the archive directory for problematic torrents. Corrupted or unparsable `.torrent` files are moved here once the write is confirmed complete; files with the same name are not overwritten.

`state_file`: the state persistence file, which stores accumulated upload and the status of torrents that have reached the target share ratio.

`scan_interval_seconds`: the interval for scanning the torrent directory.

`shutdown_stop_timeout_seconds`: the longest time to wait for the `stopped` announce to complete on exit.

### Announce Configuration

`announce.port` is recommended to be set to `0` by default:

```toml
[announce]
port = 0
```

When the port is `0` or not configured, OpenPT generates a random dynamic port between `49152-65535` on every startup. This avoids using common default ports such as `6881`.

If you really need a fixed port, you can set it to a specific value between `1-65535`:

```toml
[announce]
port = 51413
```

`announce.ip` and `announce.ipv6` can be left empty by default, letting the Tracker detect the address automatically.

### Upload Strategies

`uploaded.strategy` supports three modes:

- `none`: no accumulated upload, the most conservative
- `conservative_rate`: use a low-rate conservative upload
- `configured_rate`: use a custom upload rate

Common rate conversions:

- `100 KB/s` = `102400`
- `500 KB/s` = `512000`
- `1 MB/s` = `1048576`

`ratio_target` is the target share ratio. Setting it to `0` means never stopping because of the share ratio; setting it to `2.0` means stopping that torrent after reaching a 2.0 share ratio.

### Tracker Configuration

`tracker.timeout_seconds`: the Tracker request timeout.

`tracker.proxy`: the proxy address, which may be empty. HTTP proxies and SOCKS5 proxies are supported, for example:

```toml
proxy = "http://127.0.0.1:7890"
```

Leaving it empty means connecting to the Tracker directly: environment variables such as `HTTP_PROXY` / `HTTPS_PROXY` do not affect Tracker announce traffic, so configure this option explicitly when a proxy is needed.

The proxy is only used for HTTP/HTTPS Trackers. After a proxy is configured, UDP Trackers return a clear error, and the scheduler keeps trying other Trackers in the torrent.

`failure_backoff_min_seconds` and `failure_backoff_max_seconds` control the exponential backoff retry range after a failure.

### Web UI and Metrics

Enable the Web UI:

```toml
[metrics]
enabled = true
listen = "127.0.0.1:9090"
path = "/metrics"
webui = true
```

Access address:

```text
http://127.0.0.1:9090
```

Prometheus metrics address:

```text
http://127.0.0.1:9090/metrics
```

The health check address is always:

```text
http://127.0.0.1:9090/healthz
```

`GET` and `HEAD` requests return `200 OK` on success. The Docker image also uses this endpoint to run container health checks.
`/healthz` does not disappear when `metrics.enabled` is turned off: even with `enabled = false`, the program still listens on
`metrics.listen` and serves only that endpoint (`/metrics` and the Web UI stop being served), so that container health checks remain available at all times.

If you need LAN access, you can change the listen address to:

```toml
listen = "0.0.0.0:9090"
```

## Using the Web UI

The Web UI shows in real time:

- Number of active torrents
- Number of abnormal torrents
- Current upload speed
- Total uploaded
- Next announce time
- Size, uploaded, speed, peers and share ratio of each torrent
- Last announce time, next announce time, announce interval
- Tracker host and Tracker switch index
- Status and error details

You can sort by column, filter by status, and search by name or Tracker. Hovering over the status column shows the full error message, announce time, Tracker, peers and other context.

## Hot Reload

Sending `SIGHUP` hot reloads part of the configuration:

```sh
kill -HUP $(pidof openpt)
```

Hot reloadable:

- Number of simultaneous seeds
- Upload strategy and rate
- Share ratio target
- Timeout for waiting for the `stopped` announce on shutdown
- Announce port, IP and IPv6
- Tracker timeout, proxy and backoff configuration

When `announce.port = 0`, hot reload keeps using the port generated when this process started, and does not randomize again because of `SIGHUP`.

Require a restart to take effect:

- Client spoofing files
- Torrent directory
- Problematic torrent archive directory
- Client configuration directory
- State file
- Torrent scan interval
- Log file
- Metrics service switch, listen address, metrics path and Web UI switch

## Usage Tips

- OpenPT does not need real files, only `.torrent` files.
- OpenPT does not actually upload data; it only reports upload numbers to the Tracker.
- The default `announce.port = 0` randomizes the port at startup; set a specific value manually if you need a fixed port.
- It is recommended to start with a more conservative upload strategy, and adjust the rate after confirming the site behaves normally.
- HTTP, HTTPS and UDP Trackers are supported; when a proxy is configured, UDP Trackers report a clear error and switch to other Trackers.

## License

This project is released under the [MIT License](LICENSE).

---

# 中文

OpenPT 是一个轻量级、配置驱动的 BitTorrent Tracker Announce 工具，面向 PT 站保种场景。它只需要 `.torrent` 文件，不需要真实下载内容，通过模拟客户端向 Tracker 定期汇报做种状态、上传量、端口、客户端标识等信息。

OpenPT 内置 Web UI，可实时查看活跃种子、上传速度、分享率、Tracker 状态、上次上报时间、下次上报时间和错误详情。默认配置会在每次启动时随机生成 Announce 端口，避免继续使用常见的固定默认端口。

## 主要功能

- 自动扫描 `torrents` 目录并加载 `.torrent` 文件
- 按配置限制同时保种数量
- 支持 qBittorrent、Transmission、Deluge、uTorrent 等客户端伪装文件
- 支持不累计上传量、保守速率、自定义速率三种上传策略
- 根据 Tracker 返回的 peers 动态分配上传速度
- 支持目标分享率，达到后自动停止该种子
- Tracker 请求失败后自动指数退避重试
- 支持 HTTP、HTTPS 和 UDP Tracker
- 支持 BitTorrent v1、hybrid 和 v2-only 种子 info hash
- 自动归档损坏或无法解析的种子文件，避免反复加载失败
- 持久化累计上传量和已完成状态，重启后可继续运行
- 删除种子文件后自动发送 `stopped` 上报并释放槽位
- Web UI 实时展示状态、错误详情和当前配置
- 暴露 Prometheus 格式指标和健康检查接口
- 支持 `SIGHUP` 热重载部分配置

## 快速开始

### 本地运行

1. 准备程序文件和配置文件：

```sh
cp examples/config.example.toml config.toml
```

2. 编辑 `config.toml`：

```sh
nano config.toml
```

3. 创建种子目录，并放入 `.torrent` 文件：

```sh
mkdir -p torrents
```

4. 启动 OpenPT：

```sh
./openpt --config config.toml
```

5. 打开 Web UI：

```text
http://127.0.0.1:9090
```

### Docker 运行

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

如果宿主机当前用户不是 `1000:1000`，先查看实际 UID/GID：

```sh
id -u
id -g
```

再把 `PUID`、`PGID` 改成对应数值。OpenPT 会以这个用户运行，便于读取 bind mount 进来的 `.torrent` 文件。容器启动时不会修改宿主机上已经存在的目录或文件所有权。

启动：

```sh
docker compose up -d
```

将 `.torrent` 文件放入：

```text
./data/torrents
```

如果已有文件出现 `permission denied`，确认它们对 `PUID/PGID` 对应的用户可读写。例如只检查权限：

```sh
ls -ld ./data ./data/torrents ./data/torrents_archive
ls -l ./data/torrents
```

修改配置后重启容器：

```sh
docker compose restart
```

## 从源码构建

Web UI 使用 Vite + React + antd 构建，产物通过 `go:embed` 内嵌进二进制。

**内嵌前端产物（`internal/web/dist/`）已随仓库入库**，因此全新 clone 后可直接
`go build` / `go test`，无需先手动构建前端：

```sh
go build -o openpt ./cmd/openpt
```

只有修改了 `frontend/src/**` 后，才需要重新构建前端产物（输出到 `internal/web/dist`，
并随改动一起提交，保持与源码同步）：

```sh
# 1. 构建前端（输出到 internal/web/dist）
cd frontend
npm ci
npm run build
cd ..

# 2. 构建 Go 二进制（内嵌前端产物）
go build -o openpt ./cmd/openpt
```

> 前端开发时可在 `frontend/` 目录运行 `npm run dev`，Vite 会启动本地开发服务器
> 并代理到 OpenPT 的 API。发布版本直接使用内嵌产物，无需额外资源目录。

> 说明：本地执行 `npm install` 后，`frontend/node_modules/flatted` 内附带一段 Go
> 参考实现，会被 `go build ./...`、`go test ./...` 等按通配符执行的工具一并编译
> （表现为 `openpt/frontend/node_modules/...` 包）。该目录已被 `.gitignore` 排除，
> 不影响 CI 与发布产物，编译失败风险为零；如需规避，可在前端构建后再执行 Go 命令。

## 配置方法

OpenPT 使用 TOML 配置文件。推荐从示例文件复制：

```sh
cp examples/config.example.toml config.toml
```

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

### 核心配置

`simultaneous_seed`：同时保种数量。设置为 `0` 表示不限制数量，会尽量加载目录中的全部种子。

`client`：客户端伪装文件名，需要存在于 `clients_dir` 目录中。

`torrents_dir`：种子文件目录，OpenPT 会扫描这里的 `.torrent` 文件。

`clients_dir`：客户端伪装配置目录。

`archive_dir`：问题种子归档目录。损坏或无法解析的 `.torrent` 文件会在确认写入完成后移动到这里；同名文件不会被覆盖。

`state_file`：状态持久化文件，保存累计上传量和已达到目标分享率的种子状态。

`scan_interval_seconds`：扫描种子目录的间隔。

`shutdown_stop_timeout_seconds`：退出时等待 `stopped` 上报完成的最长时间。

### Announce 配置

`announce.port` 默认推荐设置为 `0`：

```toml
[announce]
port = 0
```

当端口为 `0` 或未配置时，OpenPT 会在每次启动时随机生成 `49152-65535` 之间的动态端口。这样可以避免使用 `6881` 等常见默认端口。

如果你确实需要固定端口，可以设置为 `1-65535` 之间的具体值：

```toml
[announce]
port = 51413
```

`announce.ip` 和 `announce.ipv6` 默认留空即可，让 Tracker 自动识别地址。

### 上传策略

`uploaded.strategy` 支持三种模式：

- `none`：不累计上传量，最保守
- `conservative_rate`：使用低速保守上传量
- `configured_rate`：使用自定义上传速率

常见速率换算：

- `100 KB/s` = `102400`
- `500 KB/s` = `512000`
- `1 MB/s` = `1048576`

`ratio_target` 为目标分享率。设置为 `0` 表示永不因分享率停止；设置为 `2.0` 表示达到 2.0 分享率后停止该种子。

### Tracker 配置

`tracker.timeout_seconds`：Tracker 请求超时时间。

`tracker.proxy`：代理地址，可为空。支持 HTTP 代理或 SOCKS5 代理，例如：

```toml
proxy = "http://127.0.0.1:7890"
```

留空表示直连 Tracker：`HTTP_PROXY` / `HTTPS_PROXY` 等环境变量不会影响 Tracker 上报流量，需要代理时请显式配置此项。

代理仅用于 HTTP/HTTPS Tracker。配置代理后，UDP Tracker 会返回明确错误，调度器会继续尝试种子中的其它 Tracker。

`failure_backoff_min_seconds` 和 `failure_backoff_max_seconds` 控制失败后的指数退避重试范围。

### Web UI 与指标

启用 Web UI：

```toml
[metrics]
enabled = true
listen = "127.0.0.1:9090"
path = "/metrics"
webui = true
```

访问地址：

```text
http://127.0.0.1:9090
```

Prometheus 指标地址：

```text
http://127.0.0.1:9090/metrics
```

健康检查地址始终为：

```text
http://127.0.0.1:9090/healthz
```

`GET` 和 `HEAD` 请求成功时返回 `200 OK`。Docker 镜像也使用此接口执行容器健康检查。
`/healthz` 不随 `metrics.enabled` 关闭而消失：即使 `enabled = false`，程序仍会监听
`metrics.listen` 并只提供该接口（`/metrics` 与 Web UI 停止服务），以保证容器健康检查始终可用。

如果需要局域网访问，可将监听地址改成：

```toml
listen = "0.0.0.0:9090"
```

## Web UI 使用

Web UI 会实时展示：

- 活跃种子数量
- 异常种子数量
- 当前上传速度
- 总上传量
- 下次上报时间
- 每个种子的大小、上传量、速度、Peers、分享率
- 上次上报时间、下次上报时间、上报间隔
- Tracker 主机和 Tracker 切换序号
- 状态和错误详情

可以按列排序、按状态筛选、按名称或 Tracker 搜索。鼠标悬停在状态列上，可以查看完整错误信息、上报时间、Tracker、Peers 等上下文。

## 热重载

发送 `SIGHUP` 可热重载部分配置：

```sh
kill -HUP $(pidof openpt)
```

可热重载：

- 同时保种数量
- 上传策略和速率
- 分享率目标
- 关闭时等待 `stopped` 上报的超时时间
- Announce 端口、IP 和 IPv6
- Tracker 超时、代理和退避配置

当 `announce.port = 0` 时，热重载会继续使用本次进程启动时生成的端口，不会因 `SIGHUP` 再次随机。

需要重启后生效：

- 客户端伪装文件
- 种子目录
- 问题种子归档目录
- 客户端配置目录
- 状态文件
- 种子扫描间隔
- 日志文件
- 监控服务开关、监听地址、指标路径和 Web UI 开关

## 使用提示

- OpenPT 不需要真实文件，只需要 `.torrent` 文件。
- OpenPT 不会真实上传数据，只会向 Tracker 汇报上传量数字。
- 默认 `announce.port = 0` 会在启动时随机端口；如需固定端口再手动设置具体值。
- 建议从较保守的上传策略开始，确认站点表现正常后再调整速率。
- 支持 HTTP、HTTPS 和 UDP Tracker；配置代理时 UDP Tracker 会明确报错并切换到其它 Tracker。

## 许可证

本项目以 [MIT License](LICENSE) 发布。
