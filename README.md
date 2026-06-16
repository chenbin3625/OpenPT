# OpenPT

OpenPT 是一个轻量级、配置驱动的 BitTorrent Tracker Announce 工具，专为 PT 站保种设计。

## 核心特性

### 🚀 智能做种管理
- **自动失败处理**：连续失败 10 次后自动停止损坏的种子，释放槽位
- **分享率控制**：达到目标分享率后自动停止，避免过度上传
- **槽位自动填充**：种子停止后立即填充新种子，保持满载运行
- **热配置重载**：`SIGHUP` 信号热重载配置，无需重启

### 📊 实时监控面板
- **SSE 实时更新**：种子状态、速度、分享率实时刷新
- **异常检测**：自动标记失败和无 peer 的种子
- **搜索过滤**：按名称搜索，按状态筛选
- **配置查看**：Web UI 查看当前运行配置

### 🔧 灵活配置
- **TOML 格式**：简洁易读，支持中文注释
- **客户端伪装**：内置多种 BitTorrent 客户端配置
- **上传策略**：`none`、`conservative_rate`、`configured_rate` 三种模式
- **智能带宽分配**：根据 peers 数量动态分配上传速度

### 🛡️ 安全可靠
- **优雅退出**：`Ctrl+C` 时向所有 tracker 发送 `stopped`
- **无需真实文件**：仅需 `.torrent` 文件，无需下载内容
- **保守默认值**：`left=0`，`downloaded=0`，`uploaded` 默认不增长

## 快速开始

### 本地运行

1. 从 [Releases](https://github.com/your-repo/OpenPT/releases) 下载对应系统的压缩包并解压

2. 创建并编辑配置文件：
   ```sh
   cp examples/config.example.toml config.toml
   nano config.toml
   ```

3. 将 `.torrent` 文件放入 `torrents/` 目录

4. 启动程序：
   ```sh
   ./openpt --config config.toml
   ```

5. 访问 Web UI：`http://127.0.0.1:9090`

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
    volumes:
      - ./data:/data
```

启动：
```sh
docker compose up -d
```

首次启动自动生成 `data/config.toml`，编辑后重启容器：
```sh
docker compose restart
```

## 配置说明

### 核心配置

```toml
# 同时保种数量（设为 0 可暂停所有做种）
simultaneous_seed = 200

# 客户端伪装文件
client = "qbittorrent-5.1.4.client"
```

### 上传策略

```toml
[uploaded]
# 策略：none（不增长） / conservative_rate（低速） / configured_rate（自定义）
strategy = "configured_rate"

# 上传速率（字节/秒）
configured_rate_bps = 170000  # ≈ 166 KB/s

# 速率范围（用于随机波动）
min_rate_bps = 30000   # 29 KB/s
max_rate_bps = 170000  # 166 KB/s

# 随机抖动百分比（0-100）
random_jitter_percent = 10

# 分享率目标（0 = 永不停止）
ratio_target = 2.0
```

**速率计算参考**：
- 100 KB/s = 102400
- 500 KB/s = 512000
- 1 MB/s = 1048576

### Tracker 配置

```toml
[tracker]
timeout_seconds = 15
proxy = ""  # HTTP/SOCKS5 代理，如 "http://127.0.0.1:7890"
reuse_connections = true
failure_backoff_min_seconds = 5
failure_backoff_max_seconds = 300
```

### 监控配置

```toml
[metrics]
enabled = true
listen = "127.0.0.1:9090"  # 改为 "0.0.0.0:9090" 允许外部访问
webui = true
```

## 工作原理

### 种子生命周期

1. **加载**：扫描 `torrents/` 目录，解析 `.torrent` 文件
2. **启动**：发送 `started` announce，注册到 tracker
3. **定期汇报**：按 tracker 返回的 `interval` 定期 announce
4. **智能停止**：以下情况自动停止
   - 达到分享率目标 (`ratio_target`)
   - 连续失败 10 次（损坏/无效种子）
   - 种子文件被删除
5. **停止通知**：发送 `stopped` announce，释放槽位

### 失败重试机制

使用指数退避策略，最多重试 10 次：

```
失败 1 次 → 等待 5 秒
失败 2 次 → 等待 10 秒
失败 3 次 → 等待 20 秒
失败 4 次 → 等待 40 秒
...
失败 10 次 → 停止种子
```

### 带宽分配

OpenPT 根据 tracker 返回的 peers 动态分配上传速度：

- 有更多 **leechers** 的种子获得更多带宽
- 无 peers 的种子分配最低带宽
- 总带宽不超过 `max_rate_bps`

## 配置热重载

发送 `SIGHUP` 信号重载配置（无需重启）：

```sh
kill -HUP $(pidof openpt)
```

**可热重载**：
- ✅ 同时保种数量
- ✅ 上传速度配置
- ✅ 分享率目标
- ✅ Tracker 配置

**需要重启**：
- ❌ 客户端伪装文件
- ❌ 目录路径
- ❌ 监控服务地址

## Web UI 功能

访问 `http://127.0.0.1:9090` 查看：

| 功能 | 说明 |
|------|------|
| 🟢 实时状态 | SSE 自动更新种子列表 |
| 📊 统计信息 | 活跃种子数、总速度、总上传量 |
| 🔍 搜索过滤 | 按名称搜索，按状态筛选 |
| ⚠️ 异常提示 | 自动标记失败和无 peer 种子 |
| 📋 配置查看 | 查看当前运行配置 |
| 🔗 Prometheus | 导出指标供 Grafana 监控 |

## 版本更新日志

### v0.0.7 (2024-01-XX)

**重大改进**：
- ✅ 移除最大失败次数限制，恢复无限重试机制
- ✅ 修复 GitHub Actions 构建失败问题
- ✅ 修复 WebUI 状态列显示 bug
- ✅ 优化异常种子提示，显示具体失败原因
- ✅ 添加配置查看功能，支持 WebUI 查看运行配置

**配置格式迁移**：
- ⚠️ **停止支持 JSON 配置格式，统一使用 TOML**
- ✅ 提供详细的 TOML 配置示例和注释
- ✅ 简化配置文件结构，更易读易维护

**性能优化**：
- 🚀 优化带宽分配算法，无 peers 时使用默认权重
- 🚀 优化 WebUI 实时更新性能
- 🚀 减少不必要的锁竞争

### v0.0.6 (2024-01-XX)

**重大改进**：
- ✅ 修复达到分享率目标后无限重启的 bug
- ✅ 修复 stopped announce 可能在进程退出前未完成的问题
- ✅ 允许 `simultaneous_seed = 0` 暂停所有做种
- ✅ 同步等待 stopped announce 完成，确保 tracker 收到通知

**功能移除**：
- ❌ 删除 `keep_torrent_with_zero_leechers` 配置（无实际作用）
- ❌ 完全移除归档功能，简化代码逻辑

## 常见问题

### Q: 需要真实的文件吗？
不需要。OpenPT 只需要 `.torrent` 文件，向 tracker 汇报时声明已下载完成（`left=0`）。

### Q: 会真的上传数据吗？
不会。OpenPT 只向 tracker 汇报上传量数字，不实际上传文件数据。

### Q: 安全吗？
相对安全。OpenPT 行为保守（默认上传量不增长），但仍存在被检测的理论可能。建议：
- 使用 `none` 或 `conservative_rate` 策略
- 不要设置过高的上传速度
- 定期更换客户端伪装

### Q: 支持 UDP tracker 吗？
暂不支持。仅支持 HTTP/HTTPS tracker。

### Q: 重启后会丢失数据吗？
会。运行状态（上传量、失败次数等）保存在内存中，重启后重新开始。但这通常不是问题，因为 tracker 会维护真实数据。

## 技术架构

- **语言**：Go 1.23+
- **配置**：TOML（github.com/BurntSushi/toml）
- **Torrent 解析**：github.com/anacrolix/torrent/metainfo
- **文件监控**：github.com/fsnotify/fsnotify
- **Web UI**：原生 HTML/CSS/JavaScript + SSE

## 许可证

MIT License

## 致谢

本项目受 [JOAL](https://github.com/anthonyraymond/joal) 启发。
