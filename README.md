# OpenPT

OpenPT 是一个无 WebUI、完全配置驱动的 tracker announce 程序，用于让已有 `.torrent` 以完整 seeder 身份定期向 tracker 汇报状态。

默认行为偏保守：`left=0`，`downloaded=0`，`uploaded` 默认不增长。上传量统计可以通过配置关闭，也可以按低速或固定速率策略累计。

## 功能

- 无 WebUI，无管理端口，所有运行参数来自配置文件
- 支持 HTTP/HTTPS tracker announce
- 支持 `started`、regular、`stopped` announce 流程
- 自动解析 `.torrent` 的 tracker、名称、大小和 `info_hash`
- 扫描并监听 torrent 目录，新增 `.torrent` 后自动加载
- 无效 torrent 会移动到 archive 目录
- 收到 `SIGINT` 或 `SIGTERM` 时优雅退出，并发送 `stopped`
- 支持同时保种数量限制
- 支持 tracker 超时、代理和最大连续失败次数配置
- 支持上传量策略：`none`、`conservative_rate`、`configured_rate`
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

准备目录：

```sh
mkdir -p openpt-data/torrents openpt-data/torrents/archived openpt-data/clients
cp config.example.json openpt-data/config.json
cp clients/*.client openpt-data/clients/
```

编辑 `openpt-data/config.json`，建议保持：

```json
{
  "torrents_dir": "/data/torrents",
  "archive_dir": "/data/torrents/archived",
  "clients_dir": "/data/clients",
  "client": "qbittorrent-5.1.4.client"
}
```

创建 `compose.yml`：

```yaml
services:
  openpt:
    image: chenbin3625/openpt:0.0.1
    container_name: openpt
    restart: unless-stopped
    volumes:
      - ./openpt-data:/data
    command: ["--config", "/data/config.json"]
```

启动：

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
- `configured_rate`：按配置的固定速率累计上传量

无论使用哪种策略，OpenPT 都不会让 `downloaded` 随时间增长，完整保种场景下 `left` 始终为 `0`。

## 当前限制

- 仅支持 HTTP/HTTPS tracker，暂不支持 UDP tracker
- 运行状态保存在内存中，重启后不会恢复进程内生成的临时状态
- 不提供 WebUI、HTTP 管理端或远程控制接口
