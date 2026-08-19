# OpenPT 全量代码审查与修改建议

- 审查范围：`cmd/`、`internal/`（config、clientemu、tracker、bandwidth、store、torrent、scheduler、web）、`frontend/src/`、构建与 CI 相关文件（Dockerfile、`.github/workflows/release.yml`）、文档。
- 审查方式：逐文件人工阅读 + 静态工具验证。
- 验证结果：
  - `go vet ./...` ✅ 无告警
  - `go test -race ./...` ✅ 10 个包全部通过（含 scheduler/store/tracker 并发测试）
  - `gofmt -l .` ✅ 无未格式化文件
  - `staticcheck ./...` ⚠️ 2 项（详见下文 P3）
  - `cd frontend && npm run build` ✅ 构建通过
  - CI 已配置 `govulncheck` 对依赖做漏洞扫描

**总体结论**：代码质量整体较高。并发边界（锁顺序统一为 `s.mu → a.mu`，无死锁）、持久化（fsync + rename + 目录 fsync）、Tracker 响应防溢出、优雅停机等关键路径处理得相当细致，且有较完整（约 60 个测试）的测试覆盖。本次审查未发现 P0（必现崩溃/数据损坏）级问题。主要建议集中在：少数**正确性需要核对**的隐患、若干**健壮性防御缺失**、以及**可维护性与一致性**改进。

---

## 修复状态总览（已完成，2026-08-20）

| 编号 | 问题 | 状态 | 修复方式 |
|---|---|---|---|
| P1-1 | dist 未入库导致克隆即构建失败 | ✅ | `.gitignore` 取消忽略 `internal/web/dist`，真实内嵌产物已入库；README 构建章节更新 |
| P1-2 | v2 infohash 需与参考实现对齐 | ✅ | 改用 anacrolix `infohash_v2.HashBytes` 权威实现；新增 hybrid 用例，v2 用例改用库函数做独立断言 |
| P2-1 | `Int63n` 溢出可 panic | ✅ | `normalizeConfig` 对速率/max/min 钳制到 `1<<40`；`refreshCurrentRateLocked` 增加 span 防御；新增 `TestNormalizeConfigClampsHugeRates` |
| P2-2 | UDP 超时口径与 HTTP 不一致 | ✅ | UDP 会话改为 `context.WithTimeout` 统一受 `tracker.timeout_seconds` 约束；新增 proxy 拒绝与超时两个测试 |
| P2-3 | HTTP 服务器缺读超时 | ✅ | 增加 `ReadTimeout: 30s`；`WriteTimeout` 因 SSE 长连接仍刻意不设并注释说明 |
| P3-1 | 死代码（staticcheck） | ✅ | 删除未使用 `randInt64`；移除 scheduler 死赋值；staticcheck 现 0 告警 |
| P3-2 | 热路径重复编译正则 | ✅ | `unresolvedPlaceholder` 提升为包级变量 |
| P3-3 | 路由冲突清单双处硬编码 | ✅ | 新增 `config.ReservedWebUIRoutes()` 单源清单，Validate 复用；web 注册处注释保持同步 |
| P3-4 | 新种子误标“异常（无 peers）” | ✅ | `TorrentStatus` 新增 `has_response`；未收到首次响应前不判无 peers |
| P3-5 | SSE 每客户端全量轮询 | ✅ | 自适应轮询：无变化时向 `maxSSEPollInterval(15s)` 指数退避，有变化复位 |
| P3-6 | 日志体系混用 | ✅ | web.go 统一 slog（Handler 注入 logger） |
| P3-7 | 前端工程问题 | ✅ | package.json 版本 → 0.2.2；`formatBytes` 负数/NaN 归零；`fetchConfig` 校验 res.ok；ConfigDrawer 增加加载失败 Alert；StatsBar 硬编码色改 theme token |
| P3-8 | 其它杂项 | ✅ | 新增 MIT LICENSE + README 许可证节；`randomAnnouncePort` 重写为 uint64 无符号算法；`RegisterRoutes` 由 panic 改为返回 error；带宽随机源与 store.emit 阻塞告警加注释/日志 |

> 说明：P3-7 中“接入 ESLint”未落地——前端代码已按要求清理，但未引入 ESLint 工具链（需新增依赖且 CI 未接线，避免半成品配置）。如需可另行接入。

---

## P1 —— 建议尽快处理

### P1-1 开发/CI 一致性：`internal/web/dist` 未入库，而 `go:embed` 强依赖它
- 位置：`internal/web/web.go:18`（`//go:embed all:dist`）；`.gitignore` 中 `dist/` 命中该目录，`git ls-files internal/web/dist` 为 0 个文件。
- 问题：`go:embed all:dist` 要求 `dist` 目录在编译时存在。当前该目录被 `.gitignore` 忽略，**全新 clone 后直接 `go build ./...` / `go test ./...` 会编译失败**（`pattern all:dist: no matching files found`），必须先手动 `cd frontend && npm ci && npm run build`。CI 通过单独的前端 job 注入产物解决了 CI 问题，但本地贡献者/使用源码包的用户仍会踩坑。
- 建议（任选其一）：
  1. 提交一个最小的 `internal/web/dist/index.html` 占位文件（推送时保留真实构建产物即可覆盖），保证克隆即能编译测试；
  2. 或在 README/`Makefile` 增加 `make frontend build` 前置校验并给出清晰报错；
  3. 更稳妥：`go:embed` 改用可容忍缺失的写法（如单独 `//go:embed dist/index.html` + 运行时从可配置目录回退），但这会改变现有架构，非必须。

### P1-2 正确性核对：v2-only 种子信息哈希的计算需要与 BEP-52 对齐
- 位置：`internal/torrent/torrent.go:38-41`
  ```go
  case info.HasV2():
      v2Hash := sha256.Sum256(mi.InfoBytes)
      copy(hash[:], v2Hash[:20])
  ```
- 问题：BEP-52 对 v2 种子的 trackenable infohash 定义较为特殊——并不简单等于“对整份 `InfoBytes` 做 SHA-256 截断”，且 **bencoded 的 info 字典如包含 `padding` 字段时应先剔除再哈希**。这里直接用 `mi.InfoBytes`（文件里原始编码的完整 info 字典）可能存在偏差，会导致纯 v2-only Tracker 端认不出种子（README 声明“支持 v2-only 种子 info hash”）。
- 建议：用真实 v2-only / hybrid `.torrent` 样本与 qBittorrent/libtorrent 生成的 URL 对比校验；必要时按 BEP-52 对 info 字典做规范化（剔除 padding、遵循 canonical bencode）后再截断 SHA-256，并补充 `torrent_test.go` 的 v2 用例（当前 `TestTorrentV2UsesTruncatedSHA256InfoHash` 只是自证当前实现，非独立正确性验证）。若确认 PT 场景均为 v1，可在 README 明确标注“v2-only 支持为最佳努力”。

---

## P2 —— 健壮性/稳定性建议

### P2-1 带宽随机范围 `Int63n` 溢出可致 panic
- 位置：`internal/bandwidth/bandwidth.go:280`
  ```go
  d.currentRate = minRate + d.rng.Int63n(maxRate-minRate+1)
  ```
- 问题：`maxRate-minRate+1` 为 `int64` 运算，当配置的 `uploaded.max_rate_bps` 接近 `math.MaxInt64`（配置校验未对上限做约束）时 `maxRate-minRate+1` 会回绕为负，`math/rand.Rand.Int63n` 收到 `<=0` 参数会 panic：`panic: invalid argument to Int63n`。
- 建议：在 `bandwidth.normalizeConfig` 中对速率上限做防御（如钳制到 `1<<40` 字节/秒）或改用 `big.Int`/`rand.Int64` 区间算法（项目已在 `clientemu/types.go` 的 `digitHexAlgorithm` 中采用过 `big.Int` 方案，可复用思路）。

### P2-2 UDP Tracker 依赖 DNS 的阻塞行为与超时体系不统一
- 位置：`internal/tracker/client.go:171`（`(&net.Dialer{}).DialContext(ctx, "udp", ...)`）。
- 问题：UDP announce 的读写超时走 `conn.SetDeadline`（受 `timeout` 约束），而连接建立依赖 `DialContext`（受 ctx 约束）。本地解析/DNS 阻塞时，若 `timeout` 很小而 ctx 永不取消，行为尚可；但整体超时口径与 HTTP 路径（`http.Client.Timeout`）不一致，且**单个 UDP 失败即整次 announce 失败并切换 tracker**，缺少与 HTTP 路径一致的“失败退避+多 tracker 轮询中继续”语义观察点。
- 建议：为 UDP 路径显式传入带超时的 `context.WithTimeout`（或在 `Dialer` 上设置 `Timeout`/`Deadline`），并补充“配置 proxy 后 UDP 返回明确错误”的单元测试（当前 `client_test.go` 只有成功路径 `TestAnnounceUDP`）。

### P2-3 HTTP 服务器缺少整体读/写超时（除 `ReadHeaderTimeout`）
- 位置：`cmd/openpt/main.go:226-232`
- 现状：已设置 `ReadHeaderTimeout: 5s`、`IdleTimeout: 60s`、`MaxHeaderBytes: 1MB`，但未设 `ReadTimeout`；`WriteTimeout` 因会与 SSE 长连接冲突而刻意未设（合理）。
- 建议：保持不设 `WriteTimeout`（SSE 需要），但可补充对 API 端点（非 SSE）的按路由超时，或使用中间件对 `/api/*` 加等待超时，防止慢客户端占用连接。

---

## P3 —— 清理与体验改进（按性价比排序）

### P3-1 静态检查死代码（staticcheck 已抓出，共 2 处）
1. `internal/clientemu/types.go:563` `func randInt64` 完全未使用，删除（连同错误提示里对它的引用）。
2. `internal/scheduler/scheduler.go:508` `interval := a.lastInterval` 是死赋值：无论 `intervalSeconds > 0` 与否，后续 `:546` 都会重新声明并读取 `a.lastInterval`，第 508 行的值从未被消费。删除该行即可（行为不变），顺手消除 SA4006。

### P3-2 热路径重复编译正则
- `internal/clientemu/types.go:266` 每次 `RenderQuery` 都执行 `regexp.MustCompile(\`\{.*?\}\`)`。建议提升为包级 `var`，或先 `strings.Index(q, "{")` 短路（客户端模板多数没有未知占位符）。

### P3-3 `metrics.path` 与 Web 路由冲突清单双处硬编码
- `internal/config/config.go:289-292`（校验）与 `internal/web/web.go:82-88`（注册）各维护一份“/ 、/assets/、/openpt-icon.svg、/api/status …”清单，已出现一次失同步（见 v0.2.1 修复记录）。
- 建议：把路由清单抽成一个包级常量（如 `web.ReservedPaths()`），config 校验直接复用，从机制上防止再次漂移。

### P3-4 新加载种子被误标“异常（无 peers）”
- `internal/scheduler/scheduler.go:917-922`：`Seeders==0 && Leechers==0` 即判定 `hasIssue=true`，而刚调度的种子在首次 announce 返回前种子/做种者都是 0，前端立即显示橙色“异常”，噪声大、易误判。
- 建议：增加“尚无数据”中性状态（如增加 `hasData`/`announceCount` 字段，仅在收到过至少一次响应后才把“无 peers”算作问题）。

### P3-5 SSE 采用每客户端全量轮询，规模扩展时线性放大
- `internal/web/web.go:244,262`：每个 SSE 连接每 2s 都调用 `scheduler.Status()`（遍历全部 active 种子）+ `json.Marshal`。单用户无碍，多浏览器/多面板时成本随连接数×种子数放大。
- 建议：短期可把轮询间隔可配置/随连接数退避；长期可改为单协程广播 + 订阅者集合，或让 `scheduler` 提供增量事件。

### P3-6 日志体系混用
- `internal/web/web.go:8` 用标准库 `log.Printf`，其余均用 `slog`。建议统一为 slog（带结构化字段），便于日志采集。

### P3-7 前端工程与一致性
- `frontend/package.json` 版本仍为 `0.1.0`，与项目 tag（0.2.2）不一致，建议同步。
- 无 ESLint/Prettier 配置；代码基本整洁，但建议接入以固定团队风格。
- `frontend/src/components/StatsBar.jsx:62-63` 硬编码主题色 `#6366f1`，建议改走 antd token。
- `frontend/src/format.js:1-9` `formatBytes` 对负值会输出负数字节（`Math.log` 为 NaN 后落到 0 单位），可在入口对 `n<0` 取 `Math.abs` 或显示 0 B。
- `frontend/src/api.js` 对 `/api/config` fetch 无状态/错误兜底（失败时 ConfigDrawer 仅 console.error），建议给出加载失败提示。

### P3-8 其它小项
- `internal/config/config.go:197-205` 端口回退分支的位运算可读性一般，可改为显式 `uint64(seed)>>16` 计算并在注释说明。
- `internal/clientemu`（关键生成）+ `internal/bandwidth`（速率抖动）使用两套随机源（crypto/rand 与 `math/rand`+时间种子）。带宽抖动非安全敏感，可接受；但建议注释说明这是有意为之。
- `store.emit`（`internal/store/store.go:449`）在事件缓冲满时阻塞 watcher goroutine（有界），现有注释已说明是有意取舍；建议再加一条日志记录被阻塞时长，便于诊断极端批量场景。
- 仓库无 `LICENSE` 文件：若将发布到 Docker Hub/对外分发，建议明确开源许可证（MIT/Apache-2 等），并在 `go.mod`/README 标注，避免下游使用者合规风险。
- `internal/web/web.go:83` 对 dist 缺失直接 `panic("web: embedded dist missing")`——改为带说明/可配置降级会更好（与 P1-1 相关）。

---

## 已确认无问题（审查重点，但结论良好）

- **并发安全**：`scheduler`/`bandwidth`/`store`/`tracker` 各锁无嵌套反向（仅 `s.mu → a.mu` 单向嵌套），未发现死锁；成功上报与并发停止之间的竞态、停机 `WaitGroup.Add/Wait` 顺序均有专门测试与注释。
- **持久化**：状态文件写临时文件 → `fsync` → `rename` → 目录 `fsync`，掉电安全；写入有防抖，避免批量删除时高频 fsync。
- **Tracker 响应安全**：bencode 解析有深度限制（200）与字符串长度溢出防御；HTTP 响应有 1MB 读取上限与 gzip/deflate（含 zlib 探测）解码；UDP 有交易 ID 校验。
- **上报节流**：对不可信 `interval` 有 7 天上限与 min-interval 合并逻辑，防疯狂上报。
- **反作弊相关**：`seeders>0 && leechers==0` 时上传权重归零；挂起恢复后单次 tick 累计上限 2s，避免尖峰；peer/key 均用 crypto/rand。
- **优雅停机**：先并发发 stopped → 关闭 stopped 入口 → 等待在途 → 最终落盘；`http.Server.Shutdown` 因 SSE 注入停机信号而立即退出。
- **配置校验**：proxy URL、metrics.listen/path 冲突、`archive_dir` 是否落入 `torrents_dir`、速率/退避/比值的边界均做了校验；`unknown config field` 严格拒绝。
- **CI**：编排合理（frontend → test(-race+vet+govulncheck) → 多平台 binaries/docker），带 `-race` 与格式校验。

---

## 建议的落地顺序

1. **P1-1**：提交 `internal/web/dist` 最小占位（或增加本地构建前置校验说明）——影响所有源码使用/贡献者。
2. **P1-2**：构造 v2/hybrid 样本核对 infohash，补齐 BEP-52 处理与独立测试。
3. **P2-1**：`normalizeConfig` 对速率上限做钳制，堵住 `Int63n` panic 路径。
4. **P3-1 / P3-2**：清理 staticcheck 2 处 + 正则热路径（低风险小改动，可顺手合入）。
5. **P3-3 ~ P3-8**：按团队节奏渐进优化（路由单源、UI 中性状态、SSE 广播、Lint、LICENSE 等）。
