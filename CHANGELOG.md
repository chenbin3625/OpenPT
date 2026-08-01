# 更新记录

本项目以 Git tag 发布版本。每次发布都会在 GitHub Release 中附上对应说明。

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
- 开发流程：`cd web && npm ci && npm run build` 后执行 `go build`

> 说明：本文件为首次建立，v0.2.0 之前的版本变更记录见各 Git tag 的发布说明。
