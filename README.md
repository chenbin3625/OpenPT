# OpenPT

OpenPT 是一个轻量级、配置驱动的 BitTorrent Tracker Announce 工具，面向 PT 站保种场景。它只需要 `.torrent` 文件，不需要真实下载内容，通过模拟客户端向 Tracker 定期汇报做种状态、上传量、端口、客户端标识等信息。

OpenPT 内置 Web UI，可实时查看活跃种子、上传速度、分享率、Tracker 状态、上次上报时间、下次上报时间和错误详情。默认配置会在每次启动时随机生成 Announce 端口，避免继续使用常见的固定默认端口。

**English Summary**

OpenPT is a lightweight, configuration-driven BitTorrent tracker announce tool for private tracker seeding workflows. It works with `.torrent` files only, simulates client announce requests, exposes a Web UI and Prometheus metrics, and does not require downloading the real content.

## 主要功能

- 自动扫描 `torrents` 目录并加载 `.torrent` 文件
- 按配置限制同时保种数量
- 支持 qBittorrent、Transmission、Deluge、uTorrent 等客户端伪装文件
- 支持不累计上传量、保守速率、自定义速率三种上传策略
- 根据 Tracker 返回的 peers 动态分配上传速度
- 支持目标分享率，达到后自动停止该种子
- Tracker 请求失败后自动指数退避重试
- 删除种子文件后自动发送 `stopped` 上报并释放槽位
- Web UI 实时展示状态、错误详情和当前配置
- 暴露 Prometheus 格式指标
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
min_rate_bps = 30000
max_rate_bps = 170000
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
- Tracker 超时、代理和退避配置

需要重启后生效：

- 客户端伪装文件
- 种子目录
- 客户端配置目录
- 日志文件
- Web UI 监听地址和指标路径

## 使用提示

- OpenPT 不需要真实文件，只需要 `.torrent` 文件。
- OpenPT 不会真实上传数据，只会向 Tracker 汇报上传量数字。
- 默认 `announce.port = 0` 会在启动时随机端口；如需固定端口再手动设置具体值。
- 建议从较保守的上传策略开始，确认站点表现正常后再调整速率。
- 暂不支持 UDP Tracker，仅支持 HTTP/HTTPS Tracker。
