# 更新记录

本项目以 Git tag 发布版本。每次发布都会在 GitHub Release 中附上对应说明。

## v0.2.6 - 2026-09-10

本版无功能变更，集中于依赖安全与发布流程本身。

### 安全
- **Go 工具链升级到 1.26.6**：用仓库自身工具链执行 `govulncheck` 时发现 5 个**可达**的标准库漏洞（net/http、crypto/tls、net/url、encoding/asn1），均在 1.26.6 修复；`go.mod` 与 Docker 构建镜像（golang:1.26.6-alpine）同步升级，发布的二进制不再携带这些漏洞
- **漏洞扫描与构建工具链对齐**：此前 `golang/govulncheck-action` 固定使用自带的 stable Go 并忽略 `go-version-file`，扫描的工具链与实际发布二进制所用的并不一致，标准库漏洞因此被掩盖

### 发布流程
- **发布产物附带 sha256 校验**：15 个平台的 tar.gz/zip 都生成同名 `.sha256` 并随 Release 一并发布
- **交叉编译冒烟纳入 CI**：15 个发布平台在每次 CI 都会交叉编译一遍，平台相关问题不再拖到打 tag 时才暴露，也不会让整次发布因单个平台失败而卡住
- **预发布不再覆盖 latest**：`v0.3.0-rc1` 这类预发布 tag 不再生成 `latest` 与次版本别名
- **Docker Hub secret 缺失直接失败**：此前只打 warning 并跳过推送，tag 发了但镜像没发、流水线仍是绿灯
- 解包前端产物前先清空 `internal/web/dist`，避免旧 hash 文件被 `go:embed` 打进二进制
- 权限最小化（`contents: write` 仅限发布 job）、增加并发控制与 job 超时

### 依赖与构建
- 前端构建工具链升级到 Vite 8 / @vitejs/plugin-react 6
- 重新构建并入库 Web UI 静态资源（internal/web/dist）

版本号 frontend/package.json -> 0.2.6

## v0.2.4 - 2026-08-30

### 正确性与健壮性
- **重复种子副本不再清零上传状态**：同一 infohash 在 torrents 目录存在多份副本时，删除其中一份不再向调度器发送 Removed 事件（此前会停止该种子并把持久化上传量与 completed 状态当作"文件被删除"清零），仅当最后一份副本被删除时才停止
- **Announce 间隔防御性下限**：tracker 响应缺失 interval（为 0）或显式给出过小间隔时，钳制到至少 30 秒（上限仍为 7 天），避免以初始 5 秒间隔无限高频上报触发站点反作弊封禁
- **健康检查与 metrics.enabled 解耦**：`/healthz` 始终可用（容器 HEALTHCHECK 依赖），即使 `enabled = false`，程序仍监听 metrics.listen 并只提供该接口（`/metrics` 与 Web UI 停止服务）；`metrics.listen` 无论开关都校验为合法监听地址
- **环境变量代理不再劫持上报**：`tracker.proxy` 未配置时显式直连，`HTTP_PROXY`/`HTTPS_PROXY` 等环境变量不再影响 Tracker 上报流量，需要代理请显式配置 `tracker.proxy`
- **周期扫描按文件指纹短路**：周期扫描对 mtime+size 未变化的种子文件跳过重新解析与 info 哈希，显著降低大种子库下的持续 CPU/磁盘开销（fsnotify 主路径行为不变）

### 一致性与细节
- **「无下载者」不再输出为异常原因**：仅有做种者属于正常做种状态（此时带宽权重为 0），下载者数量在列表中有独立展示，消除 has_issue 与 issue_reason 不一致的问题
- **keyGenerator.shouldUrlEncode 生效**：与 peerIdGenerator 的 shouldUrlEncode 语义一致（未配置时不做百分号编码）；内置客户端 profile 的 key 均为纯十六进制/字母数字，实际行为不变
- 修正 `config.Load` 过期注释（JSON 配置 v0.2.0 起已不支持）；为 `complete - 1`（排除自身的做种者计数）补充设计说明
- README：注明本地 `node_modules` 中 flatted 附带的 Go 参考实现会被 `go ./...` 通配编译（已被 .gitignore 排除，不影响 CI）；补充 tracker.proxy 留空即直连、`/healthz` 始终可用的说明

## v0.2.3 - 2026-08-20

### 健壮性与正确性
- **BTv2 信息哈希**：改用 anacrolix 参考实现计算 v2-only 种子的 20 字节 trackable infohash（原始 info 字典字节的 SHA-256 截断），避免算法细节与生态实现漂移；补充 hybrid 种子走 v1 哈希的测试
- **带宽数值溢出防御**：对上传速率/波动区间上限做钳制（1 TiB/s），消除极端配置下 `Int63n` 溢出引发的 panic；新增超大速率配置测试
- **UDP Tracker 超时统一**：UDP 会话（connect + announce）改为与 HTTP 路径一致的 timeout 约束（`tracker.timeout_seconds`），避免无响应时无限阻塞；新增代理拒绝与超时测试
- **HTTP 服务加固**：监控服务增加 `ReadTimeout`，并明确 `WriteTimeout` 因 SSE 长连接而刻意不设置
- **静态分析清零**：删除未使用函数与死赋值（staticcheck 0 告警）；热路径正则提升为包级变量，避免每次上报重复编译

### 体验与一致性
- **状态语义**：种子在收到首次 Tracker 响应前不再被误标为“无 peers 异常”（新增 `has_response` 状态）
- **SSE 自适应轮询**：数据无变化时轮询间隔向 15s 退避，空闲时显著降低全量状态遍历开销
- **日志统一**：web 层改用 slog（此前混用标准库 log）
- **路由清单单源化**：`metrics.path` 冲突校验复用 `config.ReservedWebUIRoutes()`，避免与 Web 路由注册清单漂移
- **前端**：配置抽屉增加加载失败提示；总上传量配色改走主题 token；字节格式化对负数/NaN 归零；前端版本与项目对齐

### 工程与发布
- **前端产物入库**：`internal/web/dist` 纳入版本控制，全新 clone 后可直接 `go build`/`go test`（原需先手动构建前端）
- **开源许可**：新增 MIT LICENSE 并在 README 标注
- **审查闭环**：新增 `CODE_REVIEW.md`（全量代码审查与修复状态总览）

## v0.2.2 - 2026-08-20

### 修复
- **PT 反作弊防封**：修复种子在 0 Leechers（无下载者）状态下依然被分配非零带宽权重并虚增上传的问题，增加 `seeders > 0 && leechers == 0` 时零权重判定，消除反作弊脚本封号隐患
- **时间格式化**：修复前端相对时间与持续时间格式化算法（解决 35 秒显示为“1 分钟后”、35 分钟显示为“1 小时后”的四舍五入偏差），并补充天数格式化支持
- **调度性能**：状态落盘机制增加防抖与异步触发，避免批量停止/删除种子时频繁执行同步 `fsync` 阻塞事件处理主循环
- **UDP Tracker**：修复 UDP Announce 参数解析将 `+` 错误解码为空格及二进制哈希被截断的隐患，改用无损百分号解码
- **容器安全**：`openpt-entrypoint` 在降权时补充 `syscall.Setgroups([]int{})` 清除 root 附加组，并改用标准库 `os/exec.LookPath`
- **微小速率精度**：在带宽统计中增加浮点余数记录，避免低速率模式下每秒强制整数转换丢失微小上传量

### Web UI 增强
- **主题切换**：新增深色模式（Dark Mode）、浅色模式及跟随系统自适应切换，支持状态持久化
- **数据看板**：升级 StatsBar 视觉层级，增加 SSE 连接动态呼吸状态指示灯与等宽数字排版
- **列表交互**：种子列表增加“上传中”等快捷分类筛选、InfoHash 与错误日志一键复制、结构化 Popover 详情卡片
- **配置抽屉**：ConfigDrawer 重构为 6 大业务卡片分组，支持参数值一键复制
- **构建优化**：优化 Vite 打包分包规则（manualChunks），消除包体积告警，提升首屏加载性能

## v0.2.1 - 2026-08-03

### 修复
- **安全性/鲁棒性**：拦截 bencode 字符串长度整数溢出，避免恶意或异常的 Tracker 响应导致进程崩溃
- **上传丢失**：修复同 infohash 种子文件替换（如更新 passkey/换 tracker）后累计上传量与完成状态被清零的问题，替换时保留持久化状态并改用新 announce 列表重启
- **上报节流**：防御 Tracker 返回异常大的 interval 导致 `time.Duration` 溢出为负、失去节流地疯狂上报
- **启动 panic**：修正 `metrics.path` 与 Web 路由的冲突校验列表（移除已删除的 `/styles.css`、补上 `/assets/`），避免 `/assets/` 重复注册导致启动 panic
- **优雅退出**：SSE 长连接不再阻塞 `http.Server.Shutdown`，SIGTERM 停机时间大幅缩短
- **并发**：修复停止种子与成功上报之间的竞态（孤儿带宽条目持续累计上传）、停机时 WaitGroup Add/Wait 数据竞争导致漏发 stopped
- **持久化**：状态文件写入增加 fsync，防止掉电/强杀后状态丢失
- **边界**：修复极端客户端配置下 peer_id 生成溢出、随机端口回退可能越界的问题
- **前端**：修复大文件上传量单位显示 `undefined`、搜索防抖残留定时器、“下次上报”误显示为已过期
- **开发体验**：前端开发模式补充 Vite 代理到 Go 后端 API（此前 `npm run dev` 无法访问接口）；示例配置调整 min/max 速率默认值使 `random_jitter_percent` 的 ±10% 波动生效

## v0.2.0 - 2026-08-01

### Web UI 全面重写
- 前端由原生 HTML/CSS/JS 迁移到 **React 18 + Ant Design 5** 组件库
- 统计栏改为紧凑数据带（图标颜色取自主题 token），整体更简约紧凑
- 种子列表使用 antd Table：排序、全部/异常/正常过滤、搜索防抖
- 状态详情改为悬浮卡片（Popover），异常种子悬停即查看上报详情与错误信息
- 配置面板改为右侧抽屉（Drawer）
- 种子列表支持分页（每页 8/10/20/50/100）
- 移动端响应式适配（窄屏隐藏 Info Hash / Tracker / 间隔列）

### 构建链路
- 前端产物通过 `go:embed` 内嵌进二进制，无需额外资源目录
- Dockerfile 改为多阶段构建（Node 构建前端 → Go 编译内嵌）
- CI 增加前端构建 job，测试与各平台二进制均使用内嵌产物
- 开发流程：`cd frontend && npm ci && npm run build` 后执行 `go build`

> 说明：本文件为首次建立，v0.2.0 之前的版本变更记录见各 Git tag 的发布说明。
