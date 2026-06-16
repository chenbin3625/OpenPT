# OpenPT

OpenPT 是一个无 WebUI、完全配置驱动的 tracker announce 程序，用于让已有 `.torrent` 以完整 seeder 身份定期向 tracker 汇报状态。

默认行为偏保守：`left=0`，`downloaded=0`，`uploaded` 默认不增长。上传量统计可以通过配置关闭，也可以按低速或配置速率策略累计；启用上传统计后支持随机速率、按 peers 权重分配带宽和 ratio target 自动完成。

## 功能

- 无 WebUI，无管理端口，所有运行参数来自配置文件
- 支持 HTTP/HTTPS tracker announce
- 支持 `started`、regular、`stopped` announce 流程
- 自动解析 `.torrent` 的 tracker、名称、大小和 `info_hash`
- 扫描并监听 torrent 目录，新增 `.torrent` 后自动加载
- 无效 torrent 会移动到 archive 目录
- 收到 `SIGINT` 或 `SIGTERM` 时优雅退出，并发送 `stopped`
- 支持同时保种数量限制，并在归档、停止或热更新调大并发后自动补位
- 支持 tracker 超时、代理和最大连续失败次数配置
- 支持 tracker 连接池复用和失败指数退避
- 支持上传量策略：`none`、`conservative_rate`、`configured_rate`
- 支持基于 tracker 返回的 seeders/leechers 做上传带宽权重分配
- 支持 `ratio_target`，达到目标分享率后自动发送 `stopped` 并归档
- 支持可选文件日志、SIGHUP 配置热更新和 Prometheus 文本指标
- 内置多种客户端伪装配置，位于 `clients/` 目录

## 使用方法

从 Release 下载对应系统的压缩包，解压后进入目录。

## 配置说明

### 基础配置

| 配置项 | 类型 | 默认值 | 说明 |
|--------|------|--------|------|
| `torrents_dir` | string | `"./torrents"` | 放置 `.torrent` 文件的目录，新增 torrent 会自动加载 |
| `archive_dir` | string | `"./torrents/archived"` | 无效或归档 torrent 的移动目录 |
| `clients_dir` | string | `"./clients"` | 客户端伪装配置文件目录 |
| `client` | string | `utorrent-3.5.0_43916.client` | 要使用的客户端配置文件名 |
| `simultaneous_seed` | int | `200` | 同时保种的最大数量，调大时自动补满槽位 |
| `keep_torrent_with_zero_leechers` | bool | `true` | 是否保留无 leecher 的 torrent；`false` 时 seeders 或 leechers 为 0 即归档 |
| `scan_interval_seconds` | int | `5` | 扫描 torrent 目录的间隔（秒） |
| `shutdown_stop_timeout_seconds` | int | `20` | 优雅退出时等待 stopped announce 的超时（秒） |
| `max_consecutive_failures` | int | `5` | 最大连续 announce 失败次数，达到后归档 torrent |

### Announce 配置

| 配置项 | 类型 | 默认值 | 说明 |
|--------|------|--------|------|
| `announce.port` | int | `6881` | announce 上报端口，范围 1-65535 |
| `announce.ip` | string | `""` | 强制上报的 IPv4 地址，留空自动检测 |
| `announce.ipv6` | string | `""` | 强制上报的 IPv6 地址，留空自动检测 |

### Tracker 配置

| 配置项 | 类型 | 默认值 | 说明 |
|--------|------|--------|------|
| `tracker.timeout_seconds` | int | `15` | tracker HTTP 请求超时（秒） |
| `tracker.proxy` | string | `""` | HTTP/HTTPS 代理地址，如 `http://127.0.0.1:7890`，留空直连 |
| `tracker.reuse_connections` | bool | `true` | 是否复用 HTTP 连接池 |
| `tracker.max_idle_conns` | int | `100` | 最大空闲连接数 |
| `tracker.max_idle_conns_per_host` | int | `10` | 每个 host 最大空闲连接数 |
| `tracker.idle_conn_timeout_seconds` | int | `90` | 空闲连接超时（秒） |
| `tracker.failure_backoff_min_seconds` | int | `5` | 失败退避最小等待时间（秒） |
| `tracker.failure_backoff_max_seconds` | int | `300` | 失败退避最大等待时间（秒） |

### 上传策略配置

| 配置项 | 类型 | 默认值 | 说明 |
|--------|------|--------|------|
| `uploaded.strategy` | string | `"configured_rate"` | 上传量策略：`none`、`conservative_rate`、`configured_rate` |
| `uploaded.conservative_rate_bps` | int64 | `1024` | `conservative_rate` 策略的上传速率（字节/秒） |
| `uploaded.configured_rate_bps` | int64 | `170000` | `configured_rate` 策略的上传速率（字节/秒） |
| `uploaded.min_rate_bps` | int64 | `30000` | 全局上传速率随机区间最小值（字节/秒），`0` 表示不启用 |
| `uploaded.max_rate_bps` | int64 | `170000` | 全局上传速率随机区间最大值（字节/秒），`> 0` 时启用随机速率 |
| `uploaded.random_jitter_percent` | int | `10` | 未配置随机区间时，围绕配置速率的上下浮动百分比（0-100） |
| `uploaded.random_refresh_seconds` | int | `1200` | 随机速率刷新间隔（秒），默认 20 分钟 |
| `uploaded.ratio_target` | float64 | `0` | 目标分享率，达到后自动 stopped 并归档；`0` 表示禁用 |

### 日志配置

| 配置项 | 类型 | 默认值 | 说明 |
|--------|------|--------|------|
| `logging.file` | string | `""` | 文件日志路径，留空仅输出到 stdout |

### Prometheus 指标配置

| 配置项 | 类型 | 默认值 | 说明 |
|--------|------|--------|------|
| `metrics.enabled` | bool | `false` | 是否启用 Prometheus 指标端点 |
| `metrics.listen` | string | `"127.0.0.1:9090"` | 指标服务监听地址 |
| `metrics.path` | string | `"/metrics"` | 指标 HTTP 路径 |

### 完整配置示例

```json
{
  "torrents_dir": "./torrents",
  "archive_dir": "./torrents/archived",
  "clients_dir": "./clients",
  "client": "utorrent-3.5.0_43916.client",
  "simultaneous_seed": 200,
  "keep_torrent_with_zero_leechers": true,
  "announce": {
    "port": 6881,
    "ip": "",
    "ipv6": ""
  },
  "tracker": {
    "timeout_seconds": 15,
    "proxy": "",
    "reuse_connections": true,
    "max_idle_conns": 100,
    "max_idle_conns_per_host": 10,
    "idle_conn_timeout_seconds": 90,
    "failure_backoff_min_seconds": 5,
    "failure_backoff_max_seconds": 300
  },
  "logging": {
    "file": ""
  },
  "metrics": {
    "enabled": false,
    "listen": "127.0.0.1:9090",
    "path": "/metrics"
  },
  "max_consecutive_failures": 5,
  "uploaded": {
    "strategy": "configured_rate",
    "conservative_rate_bps": 1024,
    "configured_rate_bps": 170000,
    "min_rate_bps": 30000,
    "max_rate_bps": 170000,
    "random_jitter_percent": 10,
    "random_refresh_seconds": 1200,
    "ratio_target": 0
  },
  "scan_interval_seconds": 5,
  "shutdown_stop_timeout_seconds": 20
}
```

## 快速开始

```sh
./openpt --config config.example.json
```

Windows 下运行：

```powershell
.\openpt.exe --config config.example.json
```

停止程序时按 `Ctrl+C`，OpenPT 会尽量向已启动的 torrent 发送 `stopped` announce。

## Docker Compose

创建 `compose.yml`：

```yaml
services:
  openpt:
    image: chenbin3625/openpt
    container_name: openpt
    restart: unless-stopped
    volumes:
      - ./openpt-data:/data
```

启动：

```sh
docker compose up -d
```

首次启动会在 `openpt-data` 中生成基础文件和目录：

- `config.json`
- `torrents/`
- `torrents/archived/`
- `clients/`

编辑 `openpt-data/config.json`，默认客户端为 `qbittorrent-5.1.4.client`。把需要保种的 `.torrent` 文件放入 `openpt-data/torrents`。

再次启动或重载：

```sh
docker compose up -d
```

查看日志：

```sh
docker compose logs -f openpt
```

停止：

```sh
docker compose down
```

## 上传量策略

- `none`：不累计上传量，默认推荐
- `conservative_rate`：按较低速率累计上传量
- `configured_rate`：按配置速率累计上传量

当 `uploaded.max_rate_bps` 大于 0 时，OpenPT 会在区间内随机选择当前全局速度，并按 `uploaded.random_refresh_seconds` 定期刷新；这不要求 `uploaded.configured_rate_bps` 非 0。没有显式区间但设置了 `uploaded.random_jitter_percent` 时，会围绕当前策略速率生成随机上下浮动。

OpenPT 会根据 tracker 返回的 peers 分配每个 torrent 的上传速度：有更多 leechers、且 leechers 占比更高的 torrent 会获得更多带宽。

`keep_torrent_with_zero_leechers=false` 时，只要 tracker 返回的 seeders 或 leechers 任一为 0，OpenPT 都会发送 `stopped` 并归档该 torrent。

无论使用哪种策略，OpenPT 都不会让 `downloaded` 随时间增长，完整保种场景下 `left` 始终为 `0`。

## 运行期维护

发送 `SIGHUP` 会重载配置，并应用 tracker 超时/连接池、失败退避、上传速率、ratio target、同时保种数量等运行期参数；如果调大 `simultaneous_seed`，OpenPT 会立即补满可用槽位。更换 client 文件、torrent/client 目录、日志文件路径或 metrics 监听地址/路径需要重启。

## 当前限制

- 仅支持 HTTP/HTTPS tracker，暂不支持 UDP tracker
- 运行状态保存在内存中，重启后不会恢复进程内生成的临时状态
- 不提供 WebUI、HTTP 管理端或远程控制接口
