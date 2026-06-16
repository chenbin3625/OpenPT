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

编辑 `config.example.json`，至少确认这些路径和选项：

- `torrents_dir`：放置 `.torrent` 文件的目录
- `archive_dir`：无效或归档 torrent 的目录
- `clients_dir`：客户端伪装配置目录，默认可使用 `./clients`
- `client`：要使用的客户端配置文件名，默认示例使用 `qbittorrent-5.1.4.client`
- `simultaneous_seed`：同时保种数量
- `announce.port`：announce 上报端口
- `tracker.timeout_seconds`：tracker 请求超时
- `tracker.proxy`：代理地址，可留空
- `uploaded.strategy`：上传量策略，默认建议使用 `none`
- `uploaded.min_rate_bps` / `uploaded.max_rate_bps`：全局上传速率随机区间，`max_rate_bps > 0` 时启用并定期重抽
- `uploaded.random_jitter_percent`：未配置随机区间时，围绕配置速率生成上下浮动
- `uploaded.ratio_target`：目标分享率，`0` 表示禁用
- `logging.file`：文件日志路径，留空仅输出到 stdout
- `metrics.enabled`：开启后在 `metrics.listen` 的 `metrics.path` 输出 Prometheus 指标

把需要保种的 `.torrent` 文件放入 `torrents_dir`，然后运行：

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
    image: chenbin3625/openpt:0.0.1
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

当 `uploaded.max_rate_bps` 大于 0 时，OpenPT 会像 joal 一样在区间内随机选择当前全局速度，并按 `uploaded.random_refresh_seconds` 定期刷新；这不要求 `uploaded.configured_rate_bps` 非 0。没有显式区间但设置了 `uploaded.random_jitter_percent` 时，会围绕当前策略速率生成随机上下浮动。

OpenPT 会根据 tracker 返回的 peers 分配每个 torrent 的上传速度：有更多 leechers、且 leechers 占比更高的 torrent 会获得更多带宽。

为兼容 JOAL，`keep_torrent_with_zero_leechers=false` 时，只要 tracker 返回的 seeders 或 leechers 任一为 0，OpenPT 都会发送 `stopped` 并归档该 torrent。

无论使用哪种策略，OpenPT 都不会让 `downloaded` 随时间增长，完整保种场景下 `left` 始终为 `0`。

## 运行期维护

发送 `SIGHUP` 会重载配置，并应用 tracker 超时/连接池、失败退避、上传速率、ratio target、同时保种数量等运行期参数；如果调大 `simultaneous_seed`，OpenPT 会立即补满可用槽位。更换 client 文件、torrent/client 目录、日志文件路径或 metrics 监听地址/路径需要重启。

## 当前限制

- 仅支持 HTTP/HTTPS tracker，暂不支持 UDP tracker
- 运行状态保存在内存中，重启后不会恢复进程内生成的临时状态
- 不提供 WebUI、HTTP 管理端或远程控制接口
