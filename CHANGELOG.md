# 更新记录

本项目以 Git tag 发布版本。每次发布都会在 GitHub Release 中附上对应说明。

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
