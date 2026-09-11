// UI 回归断言表。每条断言默认轮询到通过或超时，所以不依赖固定 sleep，
// 在慢机器（CI）上也不会抖动。
//
// 字段：
//   name    断言名
//   act     可选，执行交互（driver API 见 run.mjs），返回 Promise
//   expr    在页面里执行的表达式体，需 return 一个可 JSON 序列化的值
//   expect  期望值（严格相等；对象/数组按 JSON 比较）
//   settle  可选，设为毫秒数时改为"等待 N ms 后只判定一次"。
//           用于反向断言（例如"弹窗不应出现"）——轮询对这类断言会立刻通过，
//           必须先给足时间让它有机会出错。
//   timeout 可选，轮询超时（默认 5000ms）
//
// 断言按顺序执行且共享页面状态：搜索 / 筛选 / 排序 / 分页类断言各自负责收尾复位。

// 只取主列表，避免命中配置抽屉里的同名 class
const ROWS = '.card > .table-scroll tbody tr';
const EMPTY = '.card > .empty';
const PAGINATION = '.card > .pagination';
const SEARCH = '.card .search .input';

const boot = [
    {
        name: 'boot 应用挂载到 #root',
        expr: `return document.querySelectorAll('#root .app').length`,
        expect: 1,
    },
    {
        name: 'boot 表头 10 列且标题顺序正确',
        expr: `return [...document.querySelectorAll('.table thead th .th-inner > span:first-child')]
            .map(e => e.textContent).join('|')`,
        expect: '种子名称|状态|上传速度|已上传|种子大小|Peers (S/L)|分享率|下次上报|Tracker 地址|Info Hash',
    },
    {
        name: 'boot SSE 首帧后渲染 10 行（第 1 页）',
        expr: `return document.querySelectorAll('${ROWS}').length`,
        expect: 10,
        timeout: 15000,
    },
    {
        name: 'boot 卡片标题显示总数',
        expr: `return document.querySelector('.card-title .tag-primary').textContent`,
        expect: '共 24 个',
    },
];

const stats = [
    {
        name: 'stats 活跃种子数',
        expr: `return document.querySelectorAll('.stats .stat')[0].querySelector('.stat-value').textContent`,
        expect: '24',
    },
    {
        name: 'stats 异常种子数并带危险色',
        expr: `const s = document.querySelectorAll('.stats .stat')[1];
            const v = s.querySelector('.stat-value');
            return v.textContent + '|' + v.classList.contains('is-danger') + '|' + s.querySelector('.stat-icon').className`,
        expect: '6|true|stat-icon tone-danger',
    },
    {
        name: 'stats 上传速率汇总',
        expr: `return document.querySelectorAll('.stats .stat')[2].querySelector('.stat-value').textContent`,
        expect: '2.98 MB/s',
    },
    {
        name: 'stats 总上传量汇总',
        expr: `return document.querySelectorAll('.stats .stat')[3].querySelector('.stat-value').textContent`,
        expect: '10.84 GB',
    },
    {
        name: 'stats 下次上报为相对时间',
        expr: `const t = document.querySelectorAll('.stats .stat')[4].querySelector('.stat-value').textContent;
            return /(后|即将上报)$/.test(t)`,
        expect: true,
    },
    {
        name: 'stats 连接状态为已连接',
        expr: `return document.querySelector('.status-dot').className + '|'
            + document.querySelectorAll('.stats .stat')[5].querySelector('.stat-value').textContent`,
        expect: 'status-dot connected|实时同步中',
        timeout: 15000,
    },
];

const filters = [
    {
        name: 'filter 四个分段计数正确',
        expr: `return [...document.querySelectorAll('.segmented .seg-label')].map(e => e.textContent).join('|')`,
        expect: '全部 (24)|正常 (18)|上传中 (16)|异常 (6)',
    },
    {
        name: 'filter 异常段标红，全部段为选中态',
        expr: `const b = [...document.querySelectorAll('.segmented .seg-btn')];
            return b[3].classList.contains('is-danger') + '|' + b[0].getAttribute('aria-selected')`,
        expect: 'true|true',
    },
];

const rows = [
    {
        name: 'row 首行名称与 Tracker 序号标签',
        expr: `const r = document.querySelector('${ROWS}');
            return r.querySelector('.name-main').textContent + '||' + r.querySelector('.name-sub .tag').textContent`,
        expect: '示例种子 01 - Release.Name.2026.1080p||Tracker 1/3',
    },
    {
        name: 'row 首行速度 / 已上传 / 大小格式化',
        expr: `const c = document.querySelector('${ROWS}').children;
            return c[2].textContent + '|' + c[3].textContent + '|' + c[4].textContent`,
        expect: '0 B/s|37.00 MB|700.00 MB',
    },
    {
        name: 'row 速度为 0 时闪电图标隐藏且用中性色',
        expr: `const w = document.querySelector('${ROWS} .speed');
            return w.className + '|' + getComputedStyle(w.querySelector('.bolt')).display`,
        expect: 'speed tone-muted|none',
    },
    {
        name: 'row 速度不为 0 时闪电图标可见',
        expr: `const w = document.querySelectorAll('${ROWS}')[1].querySelector('.speed');
            const b = w.querySelector('.bolt');
            return w.className + '|' + (getComputedStyle(b).display !== 'none') + '|' + w.textContent`,
        expect: 'speed tone-warning|true|29.30 KB/s',
    },
    {
        name: 'row 首行 Peers 合计与明细',
        expr: `const c = document.querySelector('${ROWS}').children[5];
            return c.querySelector('b').textContent + '|' + c.querySelector('.sub').textContent`,
        expect: '0|做种 0 / 下 0',
    },
    {
        name: 'row 首行分享率文本与色调',
        expr: `const v = document.querySelector('${ROWS} .ratio b');
            return v.textContent + '|' + v.className`,
        expect: '0.000|num-text tone-danger',
    },
    {
        name: 'row 分享率 >= 1 时进度条用绿色',
        expr: `const rs = [...document.querySelectorAll('${ROWS}')];
            const hit = rs.map(r => r.querySelector('.ratio b')).find(b => Number(b.textContent) >= 1);
            return hit ? hit.className : 'not-found'`,
        expect: 'num-text tone-success',
    },
    {
        name: 'row 首行 Tracker 主机与节点数',
        expr: `const c = document.querySelector('${ROWS}').children[8];
            return c.querySelector('.host').textContent + '|' + c.querySelector('.sub').textContent`,
        expect: 'tracker0.example.org|共 3 个节点',
    },
    {
        name: 'row 首行 InfoHash 截断显示且带复制按钮',
        expr: `const c = document.querySelector('${ROWS}').children[9];
            return c.querySelector('.hash-text').textContent + '|' + Boolean(c.querySelector('.copy-btn'))`,
        expect: '0000000000…|true',
    },
    {
        name: 'row 正常行状态为绿色且非交互态',
        expr: `const p = document.querySelector('${ROWS} .pill');
            return p.className + '|' + p.textContent`,
        expect: 'pill tone-success|正常',
    },
    {
        name: 'row 失败行（第 4 行）显示失败次数并标 is-issue',
        expr: `const r = document.querySelectorAll('${ROWS}')[3];
            const p = r.querySelector('.pill');
            return r.classList.contains('is-issue') + '|' + p.className + '|' + p.textContent`,
        expect: 'true|pill tone-danger is-interactive|失败 3',
    },
    {
        name: 'row 失败行副标题显示重试倒计时',
        expr: `return /^重试 .+后$/.test(document.querySelectorAll('${ROWS}')[3].querySelector('.sub').textContent)`,
        expect: true,
    },
    {
        name: 'row 无 peers 行（第 6 行）为警告态而非失败态',
        expr: `const r = document.querySelectorAll('${ROWS}')[5];
            const p = r.querySelector('.pill');
            return r.classList.contains('is-issue') + '|' + p.className + '|' + p.textContent`,
        expect: 'true|pill tone-warning is-interactive|异常',
    },
    {
        name: 'row 上报类型标签落在预期集合内',
        expr: `const set = new Set(['启动上报','停止上报','完成上报','常规上报']);
            return [...document.querySelectorAll('${ROWS} .name-sub .muted')].every(e => set.has(e.textContent))`,
        expect: true,
    },
    {
        name: 'row 按 info_hash 复用：SSE 刷新后不重建 DOM 节点',
        act: d => d.evalJS(`window.__probe = document.querySelector('${ROWS}');
            window.__probe.dataset.probe = 'kept'; return true`),
        settle: 2500, // 跨 2 帧 SSE（mock 每秒一帧）
        expr: `const r = document.querySelector('${ROWS}');
            return (r === window.__probe) + '|' + (r.dataset.probe || '')`,
        expect: 'true|kept',
    },
];

// ---- 可见性回归 ----
// v0.3.0 的 bug：这些元素靠 el.hidden 隐藏，但浏览器的 [hidden] 属于 UA 样式，
// 会被 .empty / .pagination / .input-clear 自身的作者级 display 规则盖掉，
// 于是"hidden 为 true 但仍然显示"。因此这里一律断言 computed display 与实际尺寸，
// 而不是只看 hidden 属性；并且正反两个方向都要覆盖。
const visibility = [
    {
        name: 'vis 有数据时 empty 块不可见',
        expr: `const e = document.querySelector('${EMPTY}');
            return e.hidden + '|' + getComputedStyle(e).display + '|' + e.offsetHeight`,
        expect: 'true|none|0',
    },
    {
        name: 'vis 有数据时分页条可见',
        expr: `const p = document.querySelector('${PAGINATION}');
            return p.hidden + '|' + (getComputedStyle(p).display !== 'none') + '|' + (p.offsetHeight > 0)`,
        expect: 'false|true|true',
    },
    {
        name: 'vis 搜索框为空时清空按钮不可见',
        expr: `const c = document.querySelector('.card .input-clear');
            return c.hidden + '|' + getComputedStyle(c).display + '|' + c.offsetWidth`,
        expect: 'true|none|0',
    },
    {
        name: 'vis 搜索框有内容时清空按钮可见',
        act: d => d.typeText(SEARCH, 'tracker1'),
        expr: `const c = document.querySelector('.card .input-clear');
            return c.hidden + '|' + (getComputedStyle(c).display !== 'none') + '|' + (c.offsetWidth > 0)`,
        expect: 'false|true|true',
    },
];

const search = [
    {
        name: 'search 关键字命中 8 行并显示筛选说明',
        expr: `return document.querySelectorAll('${ROWS}').length + '|'
            + document.querySelector('.card-title .note').textContent`,
        expect: '8|筛选出 8 个',
    },
    {
        name: 'search 命中行的 Tracker 全部匹配关键字',
        expr: `return [...document.querySelectorAll('${ROWS} .host')].every(e => e.textContent === 'tracker1.example.org')`,
        expect: true,
    },
    {
        name: 'vis 零结果时 empty 块可见且文案为"未找到匹配的种子"',
        act: async d => {
            await d.click('.card .input-clear');
            await d.typeText(SEARCH, 'zzz-no-such-torrent');
        },
        expr: `const e = document.querySelector('${EMPTY}');
            return (getComputedStyle(e).display !== 'none') + '|' + (e.offsetHeight > 0) + '|'
                + e.querySelector('.empty-text').textContent`,
        expect: 'true|true|未找到匹配的种子',
    },
    {
        name: 'vis 零结果时分页条不可见',
        expr: `const p = document.querySelector('${PAGINATION}');
            return p.hidden + '|' + getComputedStyle(p).display + '|' + p.offsetHeight`,
        expect: 'true|none|0',
    },
    {
        name: 'search 零结果时行清空',
        expr: `return document.querySelectorAll('${ROWS}').length`,
        expect: 0,
    },
    {
        name: 'search 点击清空按钮恢复全量并复位筛选说明',
        act: d => d.click('.card .input-clear'),
        expr: `return document.querySelectorAll('${ROWS}').length + '|'
            + document.querySelector('.card-title .note').textContent + '|'
            + document.querySelector('${SEARCH}').value`,
        expect: '10||',
    },
    {
        name: 'vis 清空后 empty 块重新隐藏',
        expr: `const e = document.querySelector('${EMPTY}');
            return getComputedStyle(e).display + '|' + e.offsetHeight`,
        expect: 'none|0',
    },
];

const filtering = [
    {
        name: 'filter 切到异常：6 行且全部带 is-issue',
        act: d => d.clickNth('.segmented .seg-btn', 3),
        expr: `const rs = [...document.querySelectorAll('${ROWS}')];
            return rs.length + '|' + rs.every(r => r.classList.contains('is-issue'))`,
        expect: '6|true',
    },
    {
        name: 'filter 异常视图分页条隐藏（单页）',
        expr: `return document.querySelector('${PAGINATION} .page-total').textContent`,
        expect: '1-6 / 共 6 个',
    },
    {
        name: 'filter 异常 + 无匹配搜索时显示"未找到匹配的种子"',
        act: d => d.typeText(SEARCH, 'zzz-no-such-torrent'),
        expr: `return document.querySelector('${EMPTY} .empty-text').textContent + '|'
            + (document.querySelector('${EMPTY}').offsetHeight > 0)`,
        expect: '未找到匹配的种子|true',
    },
    {
        name: 'filter 异常视图清空搜索后回到 6 行',
        // 说明：emptyHint() 里 filter 专属文案（如"当前没有异常种子"）需要该筛选
        // 结果为空，而本 mock 每个筛选都有数据，因此这些分支不在浏览器层覆盖。
        act: d => d.click('.card .input-clear'),
        expr: `return document.querySelectorAll('${ROWS}').length`,
        expect: 6,
    },
    {
        name: 'filter 切到上传中：16 条，首页 10 行且速度均 > 0',
        act: d => d.clickNth('.segmented .seg-btn', 2),
        expr: `const rs = [...document.querySelectorAll('${ROWS}')];
            const allUp = rs.every(r => r.children[2].textContent !== '0 B/s');
            return rs.length + '|' + allUp + '|' + document.querySelector('${PAGINATION} .page-total').textContent`,
        expect: '10|true|1-10 / 共 16 个',
    },
    {
        name: 'filter 切到正常：18 条且无 is-issue 行',
        act: d => d.clickNth('.segmented .seg-btn', 1),
        expr: `const rs = [...document.querySelectorAll('${ROWS}')];
            return rs.length + '|' + rs.some(r => r.classList.contains('is-issue')) + '|'
                + document.querySelector('${PAGINATION} .page-total').textContent`,
        expect: '10|false|1-10 / 共 18 个',
    },
    {
        name: 'filter 切回全部：恢复 24 条',
        act: d => d.clickNth('.segmented .seg-btn', 0),
        expr: `return document.querySelector('${PAGINATION} .page-total').textContent`,
        expect: '1-10 / 共 24 个',
    },
];

const sorting = [
    {
        name: 'sort 点击上传速度：升序，首行为 0 B/s 且表头标记 ascending',
        act: d => d.click('.table thead th.col-speed'),
        expr: `const th = document.querySelector('.table thead th.col-speed');
            return document.querySelector('${ROWS}').children[2].textContent + '|'
                + th.classList.contains('is-sorted') + '|' + th.getAttribute('aria-sort')`,
        expect: '0 B/s|true|ascending',
    },
    {
        name: 'sort 再点一次：降序，首行为最大速率',
        act: d => d.click('.table thead th.col-speed'),
        expr: `const th = document.querySelector('.table thead th.col-speed');
            return document.querySelector('${ROWS}').children[2].textContent + '|'
                + th.classList.contains('is-desc') + '|' + th.getAttribute('aria-sort')`,
        expect: '351.56 KB/s|true|descending',
    },
    {
        name: 'sort 第三次点击回到默认（名称升序）',
        act: d => d.click('.table thead th.col-speed'),
        expr: `const speed = document.querySelector('.table thead th.col-speed');
            const name = document.querySelector('.table thead th.col-name');
            return document.querySelector('${ROWS} .name-main').textContent.slice(0, 7) + '|'
                + speed.classList.contains('is-sorted') + '|' + name.getAttribute('aria-sort')`,
        expect: '示例种子 01|false|ascending',
    },
    {
        name: 'sort 分享率降序：首行为最高分享率',
        act: async d => {
            await d.click('.table thead th.col-ratio');
            await d.click('.table thead th.col-ratio');
        },
        expr: `return document.querySelector('${ROWS} .ratio b').textContent`,
        expect: '4.600',
    },
    {
        name: 'sort Info Hash 列不可排序（无 caret、无 tabindex）',
        expr: `const th = document.querySelector('.table thead th.col-hash');
            return th.classList.contains('is-sortable') + '|' + Boolean(th.querySelector('.sort-caret'))
                + '|' + th.hasAttribute('tabindex')`,
        expect: 'false|false|false',
    },
    {
        name: 'sort 复位为默认排序',
        act: d => d.click('.table thead th.col-ratio'),
        expr: `return document.querySelector('${ROWS} .name-main').textContent.slice(0, 7)`,
        expect: '示例种子 01',
    },
];

const paging = [
    {
        name: 'page 24 条 / 每页 10：3 个页码按钮且首页禁用上一页',
        expr: `return document.querySelectorAll('${PAGINATION} .page-btn').length + '|'
            + document.querySelector('${PAGINATION} .page-btn.is-active').textContent + '|'
            + document.querySelector('${PAGINATION} [aria-label="上一页"]').disabled`,
        expect: '3|1|true',
    },
    {
        name: 'page 下一页：显示 11-20 且首行为第 11 个种子',
        act: d => d.click(`${PAGINATION} [aria-label="下一页"]`),
        expr: `return document.querySelector('${PAGINATION} .page-total').textContent + '|'
            + document.querySelector('${ROWS} .name-main').textContent.slice(0, 7)`,
        expect: '11-20 / 共 24 个|示例种子 11',
    },
    {
        name: 'page 末页：4 行且禁用下一页',
        act: d => d.clickNth(`${PAGINATION} .page-btn`, 2),
        expr: `return document.querySelectorAll('${ROWS}').length + '|'
            + document.querySelector('${PAGINATION} .page-total').textContent + '|'
            + document.querySelector('${PAGINATION} [aria-label="下一页"]').disabled`,
        expect: '4|21-24 / 共 24 个|true',
    },
    {
        name: 'page 跳转输入框回车跳到第 1 页',
        act: d => d.jumpToPage(1),
        expr: `return document.querySelector('${PAGINATION} .page-total').textContent + '|'
            + document.querySelector('${PAGINATION} .jumper .input').value`,
        expect: '1-10 / 共 24 个|',
    },
    {
        name: 'page 每页 50：单页渲染 24 行且分页条仍可见',
        act: d => d.selectValue(`${PAGINATION} .select`, '50'),
        expr: `const p = document.querySelector('${PAGINATION}');
            return document.querySelectorAll('${ROWS}').length + '|'
                + p.querySelector('.page-total').textContent + '|'
                + (getComputedStyle(p).display !== 'none')`,
        expect: '24|1-24 / 共 24 个|true',
    },
    {
        name: 'page 复位为每页 10',
        act: d => d.selectValue(`${PAGINATION} .select`, '10'),
        expr: `return document.querySelectorAll('${ROWS}').length`,
        expect: 10,
    },
];

const drawer = [
    {
        name: 'drawer 点击顶栏配置按钮打开抽屉',
        act: d => d.click('.header-actions button[aria-label="查看运行时配置"]'),
        expr: `const p = document.querySelector('.drawer-panel');
            return Boolean(p) + '|' + p?.getAttribute('aria-modal') + '|'
                + document.documentElement.classList.contains('scroll-locked')`,
        expect: 'true|true|true',
    },
    {
        name: 'drawer 入场动画类 is-open 落上（依赖 rAF）',
        expr: `return document.querySelector('.drawer-overlay').classList.contains('is-open')`,
        expect: true,
    },
    {
        name: 'drawer 打开后关闭按钮获得焦点',
        expr: `return document.activeElement?.getAttribute('aria-label')`,
        expect: '关闭',
    },
    {
        name: 'drawer 渲染 7 个配置分组（6 个已知 + 其它配置）',
        expr: `return document.querySelectorAll('.drawer-panel .cfg-card').length`,
        expect: 7,
    },
    {
        name: 'drawer 分组标题与项数标签正确',
        expr: `return [...document.querySelectorAll('.drawer-panel .cfg-card-head')]
            .map(h => h.querySelector('span:not(.tag)').textContent + '=' + h.querySelector('.tag').textContent)
            .join('|')`,
        expect: '核心保种与客户端=3 项|目录与状态持久化=2 项|上传速率与策略=1 项|Announce 网络配置=2 项'
            + '|Tracker 连接与重试=1 项|监控与指标服务=1 项|其它配置=1 项',
    },
    {
        name: 'drawer 配置行渲染 11 项且值正确',
        expr: `const rs = [...document.querySelectorAll('.drawer-panel .cfg-row')];
            const client = rs.find(r => r.querySelector('.cfg-label').textContent === '模拟客户端');
            return rs.length + '|' + client.querySelector('.cfg-value span').textContent`,
        expect: '11|qBittorrent 4.6.5',
    },
    {
        name: 'drawer 占位值（自动检测 / 无）不给复制按钮',
        expr: `return document.querySelectorAll('.drawer-panel .cfg-row .copy-btn').length`,
        expect: 9,
    },
    {
        name: 'drawer 目录类键名用等宽字体渲染',
        expr: `const rs = [...document.querySelectorAll('.drawer-panel .cfg-row')];
            const dir = rs.find(r => r.querySelector('.cfg-label').textContent === '种子目录');
            return Boolean(dir.querySelector('.cfg-value .mono'))`,
        expect: true,
    },
    {
        name: 'drawer 顶部提示条存在',
        expr: `return document.querySelector('.drawer-panel .alert-info .alert-body strong').textContent`,
        expect: '提示',
    },
    {
        name: 'drawer ESC 关闭并解除滚动锁',
        act: d => d.pressKey('Escape'),
        expr: `return Boolean(document.querySelector('.drawer-panel')) + '|'
            + document.documentElement.classList.contains('scroll-locked')`,
        expect: 'false|false',
        timeout: 3000,
    },
];

const popover = [
    {
        name: 'popover 悬浮失败行状态标签：展开并带标题与错误框',
        act: d => d.hoverNth(`${ROWS} .pill`, 3),
        expr: `const p = document.querySelector('.popover');
            if (!p || !p.classList.contains('is-visible')) return 'not-visible';
            return p.querySelector('.popover-title').textContent.slice(0, 7) + '|'
                + p.querySelectorAll('.detail-row').length + '|'
                + Boolean(p.querySelector('.error-box'))`,
        expect: '示例种子 04|7|true',
    },
    {
        name: 'popover 错误详情文本来自 last_error',
        expr: `return document.querySelector('.popover .error-text').textContent`,
        expect: 'dial tcp 10.0.0.3:443: i/o timeout',
    },
    {
        name: 'popover 展开时定位在视口内',
        expr: `const r = document.querySelector('.popover').getBoundingClientRect();
            return (r.top >= 0 && r.left >= 0 && r.right <= window.innerWidth && r.bottom <= window.innerHeight)`,
        expect: true,
    },
    {
        name: 'popover ESC 关闭',
        act: d => d.pressKey('Escape'),
        expr: `return document.querySelector('.popover').classList.contains('is-visible')`,
        expect: false,
    },
    {
        name: 'popover 无 last_error 的警告行不渲染错误框',
        act: d => d.hoverNth(`${ROWS} .pill`, 5),
        expr: `const p = document.querySelector('.popover');
            if (!p.classList.contains('is-visible')) return 'not-visible';
            const reason = [...p.querySelectorAll('.detail-row')]
                .find(r => r.querySelector('.detail-label').textContent === '异常原因');
            return Boolean(p.querySelector('.error-box')) + '|' + reason.querySelector('.detail-value').textContent`,
        expect: 'false|无 peers 返回',
    },
    {
        name: 'popover 正常行不展开（canOpen 为 false）',
        act: async d => {
            await d.pressKey('Escape');
            await d.hoverNth(`${ROWS} .pill`, 0);
        },
        settle: 600, // 反向断言：必须给足 120ms 展开延时 + 渲染时间
        expr: `return document.querySelector('.popover').classList.contains('is-visible')`,
        expect: false,
    },
];

const tooltip = [
    {
        name: 'tooltip 悬浮带 data-tip 的元素后显示对应文案',
        act: d => d.hover('.card .toolbar button[data-tip="运行时配置"]'),
        expr: `const t = document.querySelector('.tooltip');
            if (!t || !t.classList.contains('is-visible')) return 'not-visible';
            return t.textContent + '|' + (t.getBoundingClientRect().width > 0)`,
        expect: '运行时配置|true',
    },
];

const clipboard = [
    {
        name: 'copy 点击 InfoHash 复制按钮弹出成功提示',
        act: d => d.clickNth(`${ROWS} .copy-btn`, 0),
        expr: `const t = document.querySelector('.toast');
            if (!t) return 'no-toast';
            // is-in 由 rAF 添加，一并断言，避免动画类静默失效
            return t.className + '|' + t.textContent`,
        expect: 'toast toast-success is-in|InfoHash 已复制到剪贴板',
    },
];

const theme = [
    {
        name: 'theme 默认跟随系统（headless 为亮色）',
        expr: `return document.documentElement.dataset.theme`,
        expect: 'light',
    },
    {
        name: 'theme 点击切换为暗色并写入 localStorage',
        act: d => d.click('.header-actions button:first-child'),
        expr: `return document.documentElement.dataset.theme + '|'
            + localStorage.getItem('openpt-theme-mode')`,
        expect: 'dark|dark',
    },
    {
        name: 'theme 暗色下背景色确实变深',
        expr: `const bg = getComputedStyle(document.body).backgroundColor;
            const m = bg.match(/\\d+/g).slice(0, 3).map(Number);
            return m.every(v => v < 90)`,
        expect: true,
    },
    {
        name: 'theme 再次点击切回亮色',
        act: d => d.click('.header-actions button:first-child'),
        expr: `return document.documentElement.dataset.theme + '|'
            + localStorage.getItem('openpt-theme-mode')`,
        expect: 'light|light',
    },
];

export const CHECKS = [
    ...boot, ...stats, ...filters, ...rows, ...visibility, ...search, ...filtering,
    ...sorting, ...paging, ...drawer, ...popover, ...tooltip, ...clipboard, ...theme,
];
