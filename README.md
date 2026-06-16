# OpenPT

OpenPT 是一个无 WebUI、完全配置驱动的 tracker announce 程序，用于让已有 `.torrent` 以完整 seeder 身份定期向 tracker 汇报状态。

项目默认采用保守行为：`left=0`，`downloaded=0`，`uploaded` 默认不增长。上传量统计可以通过配置关闭或按低速策略累计。

## 基础功能

- 通过配置文件启动，只保留必要 CLI 参数 `--config`
- 扫描并监听 torrent 目录，支持运行时热加载新增 `.torrent`
- 自动解析 `.torrent` 的 tracker、名称、大小和 `info_hash`
- 支持 HTTP/HTTPS tracker announce
- 支持 `started`、regular、`stopped` announce 流程
- 收到 `SIGINT` 或 `SIGTERM` 时优雅退出，并发送 `stopped`
- 无效 torrent 会移动到 archive 目录
- 支持配置 tracker timeout、proxy、最大连续失败次数
- 支持上传统计策略：`none`、`conservative_rate`、`configured_rate`
- 输出清晰运行日志，包含配置加载、torrent 加载、announce 事件、tracker 响应和归档原因

## 运行方式

```sh
cd OpenPT
go run ./cmd/openpt --config examples/config.example.json
```

构建二进制：

```sh
go build -o openpt ./cmd/openpt
./openpt --config examples/config.example.json
```

## 配置示例

配置示例位于：

[examples/config.example.json](examples/config.example.json)

常用字段：

- `torrents_dir`：torrent 文件目录
- `archive_dir`：无效或归档 torrent 的目录
- `clients_dir`：客户端伪装配置目录
- `client`：使用的客户端配置文件名
- `simultaneous_seed`：同时保种数量
- `keep_torrent_with_zero_leechers`：tracker 返回 0 leechers 时是否继续保留
- `announce.port`：announce 上报端口
- `tracker.timeout_seconds`：tracker 请求超时
- `tracker.proxy`：代理地址，例如 `http://127.0.0.1:7890`
- `max_consecutive_failures`：连续失败归档阈值
- `uploaded.strategy`：上传量策略

## 验证

```sh
gofmt -w cmd internal
go test ./...
go build -o /tmp/openpt-check ./cmd/openpt
```

## 发布

推送版本标签会触发自动发布流程：

```sh
git tag v0.0.1
git push origin v0.0.1
```

发布流程会自动构建常见平台二进制，并创建 GitHub Release。Docker Hub 推送需要在 GitHub 仓库中配置：

- `DOCKERHUB_USERNAME`
- `DOCKERHUB_TOKEN`

默认镜像仓库为 `chenbin3625/openpt`。如需修改，可设置仓库变量 `DOCKERHUB_REPOSITORY`。

## 当前限制

- 仅支持 HTTP/HTTPS tracker，暂不支持 UDP tracker
- 运行状态保存在内存中，重启后不会恢复进程内生成的临时状态
- 不提供 WebUI、HTTP 管理端或远程控制接口
