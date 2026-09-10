(function () {
'use strict';

// ==================== src/lib/dom.js ====================
// 极简 DOM 构建工具：替代 JSX，保持声明式写法但不引入运行时依赖。

// 这些键在 DOM 上必须按属性（property）赋值，setAttribute 语义不同或无效。
const ATTR_AS_PROP = new Set(['value', 'checked', 'selected', 'disabled', 'textContent']);

function applyProps(el, props) {
    for (const [key, value] of Object.entries(props)) {
        if (value === null || value === undefined || value === false) continue;

        if (key === 'class' || key === 'className') {
            el.className = Array.isArray(value) ? value.filter(Boolean).join(' ') : String(value);
        } else if (key === 'style' && typeof value === 'object') {
            Object.assign(el.style, value);
        } else if (key === 'dataset') {
            Object.assign(el.dataset, value);
        } else if (key === 'text') {
            el.textContent = String(value);
        } else if (key === 'html') {
            // 仅用于内置的静态 SVG 图标字符串，不接受外部数据
            el.innerHTML = value;
        } else if (key.startsWith('on') && typeof value === 'function') {
            el.addEventListener(key.slice(2).toLowerCase(), value);
        } else if (ATTR_AS_PROP.has(key)) {
            el[key] = value;
        } else {
            el.setAttribute(key, value === true ? '' : String(value));
        }
    }
}

function appendChildren(el, children) {
    for (const child of children) {
        if (child === null || child === undefined || child === false || child === '') continue;
        if (Array.isArray(child)) {
            appendChildren(el, child);
        } else if (child instanceof Node) {
            el.appendChild(child);
        } else {
            el.appendChild(document.createTextNode(String(child)));
        }
    }
}

// h('div', { class: 'x' }, '文本', h('span', null, '子节点'))
// 第二个参数可以直接省略成子节点（当它不是纯对象时）。
function h(tag, props, ...children) {
    const el = document.createElement(tag);
    const isProps = props && typeof props === 'object' && !Array.isArray(props) && !(props instanceof Node);
    if (isProps) applyProps(el, props);
    appendChildren(el, isProps ? children : [props, ...children]);
    return el;
}

function clearNode(el) {
    while (el.firstChild) el.removeChild(el.firstChild);
}

// ==================== src/lib/store.js ====================
// 最小可订阅状态容器：set 做浅合并并通知订阅者。
function createStore(initial) {
    let state = { ...initial };
    const subscribers = new Set();

    return {
        get: () => state,
        set(patch) {
            state = { ...state, ...patch };
            for (const fn of subscribers) fn(state);
        },
        subscribe(fn) {
            subscribers.add(fn);
            fn(state);
            return () => subscribers.delete(fn);
        },
    };
}

// ==================== src/lib/api.js ====================
// 后端接口：SSE 实时状态流 + 运行时配置快照。

/**
 * 订阅 /api/events 的实时状态推送。EventSource 自带自动重连，
 * 这里只把连接状态归一为 connecting / connected / reconnecting 交给 UI 展示。
 *
 * @param {(payload: {torrents: object[]}) => void} onData
 * @param {(state: string) => void} onState
 * @returns {() => void} 取消订阅
 */
function subscribeTorrentFeed(onData, onState) {
    const es = new EventSource('/api/events');

    es.onopen = () => onState('connected');
    es.onmessage = e => {
        try {
            const data = JSON.parse(e.data);
            onData({ torrents: Array.isArray(data.torrents) ? data.torrents : [] });
        } catch (err) {
            console.error('解析 SSE 数据失败', err);
        }
    };
    es.onerror = () => onState('reconnecting');

    return () => es.close();
}

/** 读取运行时配置（只读视图）。 */
async function fetchRuntimeConfig() {
    const res = await fetch('/api/config', { headers: { accept: 'application/json' } });
    if (!res.ok) {
        throw new Error(`配置接口返回 ${res.status}`);
    }
    const json = await res.json();
    return Array.isArray(json.items) ? json.items : [];
}

// ==================== src/lib/theme.js ====================
// 主题控制：auto 跟随系统，light / dark 为显式选择，选择结果持久化到 localStorage。
const THEME_STORAGE_KEY = 'openpt-theme-mode';

function createThemeController() {
    const media = window.matchMedia ? window.matchMedia('(prefers-color-scheme: dark)') : null;
    const listeners = new Set();

    let mode = 'auto';
    try {
        const saved = localStorage.getItem(THEME_STORAGE_KEY);
        if (saved === 'light' || saved === 'dark' || saved === 'auto') mode = saved;
    } catch {
        // 隐私模式下 localStorage 可能不可用，退回默认值
    }

    const isDark = () => (mode === 'auto' ? Boolean(media && media.matches) : mode === 'dark');

    const apply = () => {
        const dark = isDark();
        document.documentElement.dataset.theme = dark ? 'dark' : 'light';
        for (const fn of listeners) fn(dark, mode);
    };

    if (media) {
        media.addEventListener('change', () => {
            if (mode === 'auto') apply();
        });
    }

    const setMode = next => {
        mode = next;
        try {
            localStorage.setItem(THEME_STORAGE_KEY, next);
        } catch {
            // 忽略写入失败，仅本次会话生效
        }
        apply();
    };

    apply();

    return {
        get mode() {
            return mode;
        },
        get isDark() {
            return isDark();
        },
        setMode,
        toggle: () => setMode(isDark() ? 'light' : 'dark'),
        subscribe(fn) {
            listeners.add(fn);
            fn(isDark(), mode);
            return () => listeners.delete(fn);
        },
    };
}

// ==================== src/ui/tooltip.js ====================
// 事件委托式 tooltip：任何带 data-tip 属性的元素都会自动获得提示。

let tipEl = null;
let tipTimer = 0;
let tipAnchor = null;

function ensureTip() {
    if (!tipEl) {
        tipEl = h('div', { class: 'tooltip', role: 'tooltip' });
        document.body.appendChild(tipEl);
    }
    return tipEl;
}

function place(anchor) {
    const el = ensureTip();
    el.textContent = anchor.dataset.tip || '';
    el.classList.add('is-visible');

    const a = anchor.getBoundingClientRect();
    const t = el.getBoundingClientRect();
    const gap = 8;

    let top = a.top - t.height - gap;
    let below = false;
    if (top < 8) {
        top = a.bottom + gap;
        below = true;
    }
    let left = a.left + a.width / 2 - t.width / 2;
    left = Math.max(8, Math.min(left, window.innerWidth - t.width - 8));

    el.style.top = `${Math.round(top)}px`;
    el.style.left = `${Math.round(left)}px`;
    el.classList.toggle('is-below', below);
}

function hideTip() {
    clearTimeout(tipTimer);
    tipAnchor = null;
    if (tipEl) tipEl.classList.remove('is-visible');
}

function showTip(anchor) {
    if (!anchor.dataset.tip) return;
    tipAnchor = anchor;
    clearTimeout(tipTimer);
    tipTimer = setTimeout(() => {
        if (tipAnchor === anchor && anchor.isConnected) place(anchor);
    }, 140);
}

function initTooltips() {
    document.addEventListener('pointerover', e => {
        const anchor = e.target.closest?.('[data-tip]');
        if (anchor && anchor !== tipAnchor) showTip(anchor);
    });
    document.addEventListener('pointerout', e => {
        const anchor = e.target.closest?.('[data-tip]');
        if (anchor && anchor === tipAnchor) hideTip();
    });
    document.addEventListener('focusin', e => {
        const anchor = e.target.closest?.('[data-tip]');
        if (anchor) showTip(anchor);
    });
    document.addEventListener('focusout', hideTip);
    document.addEventListener('pointerdown', hideTip, true);
    window.addEventListener('scroll', hideTip, true);
    window.addEventListener('resize', hideTip);
}

// ==================== src/ui/icons.js ====================
// 内置图标：统一 24x24 线性风格，按需克隆，避免引入图标库。
const SVG_NS = 'http://www.w3.org/2000/svg';

const ICONS = {
    seeds: '<rect x="3" y="4" width="18" height="6.5" rx="2"/><rect x="3" y="13.5" width="18" height="6.5" rx="2"/><path d="M7 7.25h.01M7 16.75h.01"/>',
    warning: '<path d="M10.3 3.9 2.5 17.4A2 2 0 0 0 4.2 20.4h15.6a2 2 0 0 0 1.7-3L13.7 3.9a2 2 0 0 0-3.4 0Z"/><path d="M12 9.5v4M12 17h.01"/>',
    speed: '<path d="M12 20V5"/><path d="m5.5 11.5 6.5-6.5 6.5 6.5"/>',
    uploaded: '<path d="M21 15.5V19a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-3.5"/><path d="M12 3v12"/><path d="m7.5 7.5 4.5-4.5 4.5 4.5"/>',
    clock: '<circle cx="12" cy="12" r="8.5"/><path d="M12 7.5V12l3.5 2"/>',
    sun: '<circle cx="12" cy="12" r="4"/><path d="M12 2v2.5M12 19.5V22M2 12h2.5M19.5 12H22M4.9 4.9l1.8 1.8M17.3 17.3l1.8 1.8M19.1 4.9l-1.8 1.8M6.7 17.3l-1.8 1.8"/>',
    moon: '<path d="M21 12.9A9 9 0 1 1 11.1 3a7.2 7.2 0 0 0 9.9 9.9Z"/>',
    settings: '<path d="M4 6.5h16M4 12h16M4 17.5h16"/><circle cx="9" cy="6.5" r="2.2"/><circle cx="15" cy="12" r="2.2"/><circle cx="8" cy="17.5" r="2.2"/>',
    search: '<circle cx="11" cy="11" r="7"/><path d="m20 20-3.6-3.6"/>',
    ok: '<circle cx="12" cy="12" r="8.5"/><path d="m8.2 12.3 2.6 2.6 5-5.8"/>',
    fail: '<circle cx="12" cy="12" r="8.5"/><path d="m9.2 9.2 5.6 5.6M14.8 9.2l-5.6 5.6"/>',
    alert: '<circle cx="12" cy="12" r="8.5"/><path d="M12 7.8v4.6M12 16.2h.01"/>',
    info: '<circle cx="12" cy="12" r="8.5"/><path d="M12 11v5.2M12 7.8h.01"/>',
    copy: '<rect x="9" y="9" width="12" height="12" rx="2"/><path d="M15 5.5V5a2 2 0 0 0-2-2H5a2 2 0 0 0-2 2v8a2 2 0 0 0 2 2h.5"/>',
    bolt: '<path d="M13.4 2.5 4.6 13.2a.6.6 0 0 0 .5 1h4.6l-1.1 7.3a.6.6 0 0 0 1 .5l8.8-10.7a.6.6 0 0 0-.5-1h-4.6l1.1-7.3a.6.6 0 0 0-1-.5Z"/>',
    tracker: '<path d="M9 2.5v6M15 2.5v6"/><path d="M6.5 8.5h11v2.8a5.5 5.5 0 0 1-11 0V8.5Z"/><path d="M12 16.8v4.7"/>',
    folder: '<path d="M3 7.5a2 2 0 0 1 2-2h3.8l2 2.2H19a2 2 0 0 1 2 2v7.8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V7.5Z"/>',
    gauge: '<circle cx="12" cy="12" r="8.5"/><path d="m15.2 8.8-3.2 3.2"/><path d="M12 5.5v1.5M18.5 12H17M12 18.5V17M5.5 12H7"/>',
    globe: '<circle cx="12" cy="12" r="8.5"/><path d="M3.5 12h17"/><path d="M12 3.5c2.4 2.3 3.6 5.1 3.6 8.5S14.4 18.2 12 20.5c-2.4-2.3-3.6-5.1-3.6-8.5S9.6 5.8 12 3.5Z"/>',
    close: '<path d="M18 6 6 18M6 6l12 12"/>',
    left: '<path d="m14.5 18-6-6 6-6"/>',
    right: '<path d="m9.5 6 6 6-6 6"/>',
    caret: '<path d="m6 9.5 6-5 6 5"/>',
    inbox: '<path d="M3.5 12.5h4l1.5 3h6l1.5-3h4"/><path d="M5.5 5.5h13l2.5 7v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-6l2.5-7Z"/>',
    control: '<circle cx="12" cy="12" r="3"/><path d="M12 2.5V6M12 18v3.5M2.5 12H6M18 12h3.5M5.3 5.3l2.5 2.5M16.2 16.2l2.5 2.5M18.7 5.3l-2.5 2.5M7.8 16.2l-2.5 2.5"/>',
};

/**
 * 生成图标节点。
 * @param {string} name ICONS 中的键
 * @param {{size?: number, class?: string, fill?: boolean}} [opts]
 */
function icon(name, opts = {}) {
    const paths = ICONS[name];
    if (!paths) throw new Error(`未知图标: ${name}`);

    const svg = document.createElementNS(SVG_NS, 'svg');
    svg.setAttribute('viewBox', '0 0 24 24');
    svg.setAttribute('width', String(opts.size || 16));
    svg.setAttribute('height', String(opts.size || 16));
    svg.setAttribute('aria-hidden', 'true');
    svg.setAttribute('focusable', 'false');
    if (opts.fill) {
        svg.setAttribute('fill', 'currentColor');
        svg.setAttribute('stroke', 'none');
    } else {
        svg.setAttribute('fill', 'none');
        svg.setAttribute('stroke', 'currentColor');
        svg.setAttribute('stroke-width', '1.7');
        svg.setAttribute('stroke-linecap', 'round');
        svg.setAttribute('stroke-linejoin', 'round');
    }
    svg.setAttribute('class', ['icon', opts.class].filter(Boolean).join(' '));
    svg.innerHTML = paths;
    return svg;
}

// ==================== src/lib/format.js ====================
// 展示层格式化：字节 / 速率 / 分享率 / 时间，全部按中文习惯输出。

const BYTE_UNITS = ['B', 'KB', 'MB', 'GB', 'TB', 'PB'];

function formatBytes(n) {
    n = Number(n || 0);
    // 负数或 NaN 一律按 0 处理，避免输出 "-0.00 B" 或异常单位
    if (!(n > 0)) return '0 B';
    let i = Math.floor(Math.log(n) / Math.log(1024));
    if (!Number.isFinite(i) || i < 0) i = 0;
    if (i >= BYTE_UNITS.length) i = BYTE_UNITS.length - 1;
    return (n / Math.pow(1024, i)).toFixed(2) + ' ' + BYTE_UNITS[i];
}

function formatSpeed(n) {
    return formatBytes(n) + '/s';
}

function formatRatio(n) {
    return Number(n || 0).toFixed(3);
}

function formatTime(s) {
    if (!s) return '未上报';
    const d = new Date(s);
    if (Number.isNaN(d.getTime())) return '未知';
    return d.toLocaleString('zh-CN', { hour12: false });
}

function formatRelative(s) {
    if (!s) return '暂无';
    const d = new Date(s);
    if (Number.isNaN(d.getTime())) return '未知';
    const diff = d.getTime() - Date.now();
    const sec = Math.floor(Math.abs(diff) / 1000);
    let text;
    if (sec < 60) {
        text = sec + ' 秒';
    } else if (sec < 3600) {
        text = Math.floor(sec / 60) + ' 分钟';
    } else if (sec < 86400) {
        text = Math.floor(sec / 3600) + ' 小时';
    } else {
        text = Math.floor(sec / 86400) + ' 天';
    }
    return diff >= 0 ? text + '后' : text + '前';
}

function formatDuration(seconds) {
    seconds = Number(seconds || 0);
    if (seconds <= 0) return '-';
    if (seconds < 60) return seconds + ' 秒';
    if (seconds < 3600) {
        const min = Math.floor(seconds / 60);
        const sec = seconds % 60;
        return sec ? min + ' 分 ' + sec + ' 秒' : min + ' 分';
    }
    if (seconds < 86400) {
        const hour = Math.floor(seconds / 3600);
        const min = Math.floor((seconds % 3600) / 60);
        return min ? hour + ' 小时 ' + min + ' 分' : hour + ' 小时';
    }
    const day = Math.floor(seconds / 86400);
    const hour = Math.floor((seconds % 86400) / 3600);
    return hour ? day + ' 天 ' + hour + ' 小时' : day + ' 天';
}

function eventLabel(event) {
    if (event === 'started') return '启动上报';
    if (event === 'stopped') return '停止上报';
    if (event === 'completed') return '完成上报';
    return '常规上报';
}

// ==================== src/ui/stats.js ====================
// 顶栏指标带：活跃 / 异常 / 速率 / 总量 / 下次上报 / 连接状态。

const CONN_META = {
    connected: { text: '实时同步中', cls: 'connected' },
    reconnecting: { text: '正在重连…', cls: 'reconnecting' },
    connecting: { text: '连接中…', cls: 'connecting' },
};

const STAT_DEFS = [
    { key: 'total', label: '活跃种子', icon: 'seeds', tone: 'primary' },
    { key: 'issues', label: '异常种子', icon: 'warning', tone: 'muted' },
    { key: 'speed', label: '上传速率', icon: 'speed', tone: 'success' },
    { key: 'uploaded', label: '总上传量', icon: 'uploaded', tone: 'info' },
    { key: 'next', label: '下次上报', icon: 'clock', tone: 'warning' },
];

/** 把 nextTs 渲染成人类可读文本。 */
function nextText(nextTs) {
    if (!nextTs || nextTs === Number.MAX_SAFE_INTEGER) return '暂无';
    if (nextTs <= Date.now()) return '即将上报';
    return formatRelative(new Date(nextTs).toISOString());
}

function createStatsBar() {
    const refs = {};
    const chips = STAT_DEFS.map(def => {
        const iconEl = h('span', { class: `stat-icon tone-${def.tone}` }, icon(def.icon, { size: 15 }));
        const valueEl = h('span', { class: 'stat-value', text: '—' });
        refs[def.key] = { iconEl, valueEl };
        return h('div', { class: 'stat' },
            iconEl,
            h('span', { class: 'stat-text' },
                h('span', { class: 'stat-label', text: def.label }),
                valueEl,
            ),
        );
    });

    const dot = h('span', { class: 'status-dot connecting' });
    const connValue = h('span', { class: 'stat-value', text: CONN_META.connecting.text });
    chips.push(h('div', { class: 'stat' },
        h('span', { class: 'stat-dot-wrap' }, dot),
        h('span', { class: 'stat-text' },
            h('span', { class: 'stat-label', text: '连接状态' }),
            connValue,
        ),
    ));

    const el = h('div', { class: 'stats' }, chips);
    let lastStats = null;

    function update(stats, conn) {
        lastStats = stats;
        refs.total.valueEl.textContent = String(stats.total);
        refs.speed.valueEl.textContent = formatSpeed(stats.totalSpeed);
        refs.uploaded.valueEl.textContent = formatBytes(stats.totalUploaded);
        refs.next.valueEl.textContent = nextText(stats.nextTs);

        // 异常数为 0 时保持中性色，避免顶栏一直挂着红色
        refs.issues.valueEl.textContent = String(stats.issues);
        refs.issues.valueEl.classList.toggle('is-danger', stats.issues > 0);
        refs.issues.iconEl.className = `stat-icon tone-${stats.issues > 0 ? 'danger' : 'muted'}`;

        const meta = CONN_META[conn] || CONN_META.connecting;
        dot.className = `status-dot ${meta.cls}`;
        connValue.textContent = meta.text;
    }

    // 秒级刷新相对时间，避免空闲期（SSE 无推送）显示停滞
    function refreshTimes() {
        if (lastStats) refs.next.valueEl.textContent = nextText(lastStats.nextTs);
    }

    return { el, update, refreshTimes };
}

// ==================== src/ui/header.js ====================
// 顶部吸顶条：品牌 + 指标带 + 主题切换 / 配置入口。

function createHeader({ theme, onOpenConfig }) {
    const stats = createStatsBar();

    const themeBtn = h('button', { class: 'icon-btn', type: 'button', onClick: () => theme.toggle() });
    theme.subscribe(isDark => {
        clearNode(themeBtn);
        themeBtn.appendChild(icon(isDark ? 'sun' : 'moon', { size: 17 }));
        const label = isDark ? '切换为亮色模式' : '切换为暗色模式';
        themeBtn.dataset.tip = label;
        themeBtn.setAttribute('aria-label', label);
    });

    const el = h('header', { class: 'app-header' },
        h('div', { class: 'app-header-inner' },
            h('div', { class: 'brand' },
                h('img', { class: 'brand-icon', src: '/openpt-icon.svg', width: '26', height: '26', alt: 'OpenPT' }),
                h('span', { class: 'brand-name', text: 'OpenPT' }),
            ),
            stats.el,
            h('div', { class: 'header-actions' },
                themeBtn,
                h('button', {
                    class: 'icon-btn',
                    type: 'button',
                    'aria-label': '查看运行时配置',
                    'data-tip': '运行时配置',
                    onClick: onOpenConfig,
                }, icon('settings', { size: 17 })),
            ),
        ),
    );

    return { el, stats };
}

// ==================== src/lib/clipboard.js ====================
// 复制到剪贴板。navigator.clipboard 仅在安全上下文（https / localhost）可用，
// 而本面板常以 http 形式部署在内网，因此保留 execCommand 兜底路径。
async function copyText(text) {
    const value = String(text ?? '');
    if (!value) return false;

    if (navigator.clipboard && window.isSecureContext) {
        try {
            await navigator.clipboard.writeText(value);
            return true;
        } catch {
            // 继续走兜底
        }
    }

    const ta = document.createElement('textarea');
    ta.value = value;
    ta.setAttribute('readonly', '');
    ta.style.cssText = 'position:fixed;top:-1000px;opacity:0;';
    document.body.appendChild(ta);
    ta.select();
    let ok = false;
    try {
        ok = document.execCommand('copy');
    } catch {
        ok = false;
    }
    document.body.removeChild(ta);
    return ok;
}

// ==================== src/ui/toast.js ====================
// 轻量全局提示（替代组件库的 message）。

const TOAST_ICON = { success: 'ok', error: 'fail', info: 'info' };
let toastHost = null;

function showToast(text, type = 'success') {
    if (!toastHost) {
        toastHost = h('div', { class: 'toast-host', role: 'status', 'aria-live': 'polite' });
        document.body.appendChild(toastHost);
    }

    const el = h('div', { class: `toast toast-${type}` },
        icon(TOAST_ICON[type] || 'info', { size: 15 }),
        h('span', { text }),
    );
    toastHost.appendChild(el);

    // 入场动画结束后再计时，退场动画结束后移除节点
    requestAnimationFrame(() => el.classList.add('is-in'));
    setTimeout(() => {
        el.classList.remove('is-in');
        setTimeout(() => el.remove(), 200);
    }, 2400);
}

// ==================== src/ui/popover.js ====================
// 悬浮详情卡片。内容里可能有可点击元素（复制按钮），
// 因此离开锚点后留出宽限时间，允许指针移入卡片本体。

let panel = null;
let currentAnchor = null;
let openTimer = 0;
let closeTimer = 0;

function ensurePanel() {
    if (!panel) {
        panel = h('div', { class: 'popover', role: 'dialog' });
        panel.addEventListener('pointerenter', () => clearTimeout(closeTimer));
        panel.addEventListener('pointerleave', () => scheduleClose());
        document.body.appendChild(panel);
    }
    return panel;
}

function position(anchor) {
    const el = ensurePanel();
    const a = anchor.getBoundingClientRect();
    const p = el.getBoundingClientRect();
    const gap = 8;
    const margin = 12;

    let top = a.bottom + gap;
    if (top + p.height > window.innerHeight - margin) {
        const above = a.top - p.height - gap;
        top = above >= margin ? above : Math.max(margin, window.innerHeight - p.height - margin);
    }

    // 右对齐锚点，超出视口时收回
    let left = a.right - p.width;
    left = Math.max(margin, Math.min(left, window.innerWidth - p.width - margin));

    el.style.top = `${Math.round(top)}px`;
    el.style.left = `${Math.round(left)}px`;
}

function closePopover() {
    clearTimeout(openTimer);
    clearTimeout(closeTimer);
    currentAnchor = null;
    if (panel) {
        panel.classList.remove('is-visible');
        clearNode(panel);
    }
}

function scheduleClose() {
    clearTimeout(closeTimer);
    closeTimer = setTimeout(closePopover, 180);
}

function openFor(anchor, build, title) {
    const el = ensurePanel();
    clearNode(el);
    if (title) el.appendChild(h('div', { class: 'popover-title', text: title }));
    el.appendChild(build());
    el.style.visibility = 'hidden';
    el.classList.add('is-visible');
    // 先渲染再测量，否则拿不到真实高度
    requestAnimationFrame(() => {
        if (currentAnchor !== anchor) return;
        position(anchor);
        el.style.visibility = '';
    });
    currentAnchor = anchor;
}

/**
 * 给锚点挂载悬浮卡片。
 * @param {HTMLElement} anchor
 * @param {() => Node} build 每次展开时重新构建内容，保证数据最新
 * @param {{title?: () => string, canOpen?: () => boolean}} [opts]
 */
function attachPopover(anchor, build, opts = {}) {
    const show = () => {
        if (opts.canOpen && !opts.canOpen()) return;
        clearTimeout(closeTimer);
        clearTimeout(openTimer);
        openTimer = setTimeout(() => {
            if (anchor.isConnected) openFor(anchor, build, opts.title ? opts.title() : '');
        }, 120);
    };

    anchor.addEventListener('pointerenter', show);
    anchor.addEventListener('focus', show);
    anchor.addEventListener('pointerleave', scheduleClose);
    anchor.addEventListener('blur', scheduleClose);
}

document.addEventListener('keydown', e => {
    if (e.key === 'Escape') closePopover();
});
window.addEventListener('scroll', () => closePopover(), true);
window.addEventListener('resize', () => closePopover());

// ==================== src/ui/row.js ====================
// 单行种子渲染。行元素按 info_hash 复用：SSE 每次推送只更新文本与状态类，
// 不重建节点，这样悬浮卡片、tooltip 与滚动位置都不会被打断。

function trackerOrdinal(t) {
    return t.tracker_count > 0 ? `${(t.tracker_index || 0) + 1}/${t.tracker_count}` : '-';
}

function statusMeta(t) {
    if (t.has_issue) {
        if (t.failures > 0) return { tone: 'danger', text: `失败 ${t.failures}`, icon: 'fail' };
        return { tone: 'warning', text: '异常', icon: 'alert' };
    }
    return { tone: 'success', text: '正常', icon: 'ok' };
}

function speedTone(bps) {
    if (!(bps > 0)) return 'muted';
    if (bps < 10240) return 'default';
    if (bps < 102400) return 'warning';
    return 'success';
}

function ratioTone(r) {
    if (r >= 1) return 'success';
    if (r >= 0.5) return 'warning';
    return 'danger';
}

// 分享率进度：达标（>=1）时以 2.0 为满格，未达标时直接按百分比铺满
function ratioPercent(r) {
    const ratio = Number(r || 0);
    if (ratio >= 1) return Math.min(100, (ratio / 2) * 100);
    return Math.max(0, Math.min(100, ratio * 100));
}

function detailRow(label, value) {
    return h('div', { class: 'detail-row' },
        h('span', { class: 'detail-label', text: label }),
        h('span', { class: 'detail-value' }, value),
    );
}

/** 悬浮卡片内容：异常种子的完整上报状态与最近错误。 */
function buildStatusDetail(t) {
    const meta = statusMeta(t);
    const box = h('div', { class: 'detail' },
        detailRow('运行状态', h('span', { class: `pill tone-${meta.tone}` },
            icon(meta.icon, { size: 13 }), h('span', { text: meta.text }))),
        detailRow('上次上报', `${formatTime(t.last_announce_at)}（${formatRelative(t.last_announce_at)}）`),
        detailRow('下次上报', `${formatTime(t.next_announce_at)}（${formatRelative(t.next_announce_at)}）`),
        detailRow('上报类型', `${eventLabel(t.next_event)} · 周期 ${formatDuration(t.last_interval_seconds)}`),
        detailRow('Tracker 节点', h('span', { class: 'detail-inline' },
            icon('tracker', { size: 13, class: 'tone-primary' }),
            h('code', { text: t.tracker_host || '-' }),
            h('span', { class: 'tag', text: `序号 ${trackerOrdinal(t)}` }),
        )),
        detailRow('Peers 节点', h('span', { class: 'detail-inline' },
            h('span', null, '做种 ', h('b', { class: 'tone-success', text: String(t.seeders || 0) })),
            h('span', null, '下载 ', h('b', {
                class: t.leechers > 0 ? 'tone-warning' : 'tone-muted',
                text: String(t.leechers || 0),
            })),
        )),
    );

    if (t.issue_reason) {
        box.appendChild(detailRow('异常原因', h('span', { class: 'tone-warning', text: t.issue_reason })));
    }

    if (t.last_error) {
        const copyBtn = h('button', {
            class: 'icon-btn icon-btn-sm',
            type: 'button',
            'data-tip': '复制错误信息',
            'aria-label': '复制错误信息',
            onClick: async () => {
                const ok = await copyText(t.last_error);
                showToast(ok ? '错误信息已复制到剪贴板' : '复制失败，请手动选择文本', ok ? 'success' : 'error');
            },
        }, icon('copy', { size: 13 }));

        box.appendChild(h('div', { class: 'error-box' },
            h('div', { class: 'error-head' },
                h('span', { class: 'error-title', text: '最新错误详情' }),
                copyBtn,
            ),
            h('code', { class: 'error-text', text: t.last_error }),
        ));
    }

    return box;
}

/**
 * 创建一行，并返回可复用的更新句柄。
 * @param {object} torrent
 */
function createTorrentRow(torrent) {
    let current = torrent;

    const nameMain = h('span', { class: 'name-main' });
    const nameTag = h('span', { class: 'tag' });
    const nameEvent = h('span', { class: 'muted', text: '' });

    const pill = h('span', { class: 'pill', tabindex: '0' });
    const statusSub = h('span', { class: 'sub muted' });

    const speedBolt = icon('bolt', { size: 11, fill: true, class: 'bolt' });
    const speedText = h('span');
    const speedWrap = h('span', { class: 'speed' }, speedBolt, speedText);

    const uploaded = h('span', { class: 'num-text' });
    const size = h('span', { class: 'num-text muted' });

    const peersTotal = h('b', { class: 'num-text' });
    const peersSub = h('span', { class: 'sub muted' });

    const ratioFill = h('i');
    const ratioValue = h('b', { class: 'num-text' });

    const nextRel = h('b', { class: 'num-text' });
    const nextAbs = h('span', { class: 'sub muted' });

    const trackerHost = h('span', { class: 'host' });
    const trackerCount = h('span', { class: 'sub muted' });

    const hashText = h('code', { class: 'hash-text' });
    const hashCopy = h('button', {
        class: 'icon-btn icon-btn-sm copy-btn',
        type: 'button',
        'data-tip': '复制 InfoHash',
        'aria-label': '复制 InfoHash',
        onClick: async () => {
            const ok = await copyText(current.info_hash);
            showToast(ok ? 'InfoHash 已复制到剪贴板' : '复制失败，请手动选择文本', ok ? 'success' : 'error');
        },
    }, icon('copy', { size: 13 }));

    const tr = h('tr', { class: 'row' },
        h('td', { class: 'col-name' },
            h('div', { class: 'name' },
                nameMain,
                h('span', { class: 'name-sub' }, nameTag, nameEvent),
            ),
        ),
        h('td', { class: 'col-status num' }, h('div', { class: 'stack-end' }, pill, statusSub)),
        h('td', { class: 'col-speed num' }, speedWrap),
        h('td', { class: 'col-num num' }, uploaded),
        h('td', { class: 'col-num num' }, size),
        h('td', { class: 'col-peers num' }, h('div', { class: 'stack-end' }, peersTotal, peersSub)),
        h('td', { class: 'col-ratio num' },
            h('div', { class: 'ratio' }, h('span', { class: 'ratio-bar' }, ratioFill), ratioValue),
        ),
        h('td', { class: 'col-time num' }, h('div', { class: 'stack-end' }, nextRel, nextAbs)),
        h('td', { class: 'col-tracker' }, h('div', { class: 'stack' }, trackerHost, trackerCount)),
        h('td', { class: 'col-hash' }, h('div', { class: 'hash' }, hashText, hashCopy)),
    );

    // 悬浮详情只对异常种子开放，与正常行的静态标签保持一致的交互预期
    attachPopover(pill, () => buildStatusDetail(current), {
        title: () => current.name || '',
        canOpen: () => Boolean(current.has_issue),
    });

    function renderTimes() {
        const t = current;
        statusSub.textContent = t.failures > 0
            ? `重试 ${formatRelative(t.next_announce_at)}`
            : eventLabel(t.next_event);
        nextRel.textContent = formatRelative(t.next_announce_at);
        nextAbs.textContent = formatTime(t.next_announce_at);
    }

    function update(t) {
        current = t;
        tr.classList.toggle('is-issue', Boolean(t.has_issue));

        nameMain.textContent = t.name || '-';
        nameMain.dataset.tip = t.name || '';
        nameTag.textContent = `Tracker ${trackerOrdinal(t)}`;
        nameEvent.textContent = eventLabel(t.next_event);

        const meta = statusMeta(t);
        pill.className = `pill tone-${meta.tone}${t.has_issue ? ' is-interactive' : ''}`;
        clearNode(pill);
        pill.appendChild(icon(meta.icon, { size: 13 }));
        pill.appendChild(h('span', { text: meta.text }));
        pill.setAttribute('aria-label', t.has_issue && t.issue_reason ? `${meta.text}：${t.issue_reason}` : meta.text);

        speedWrap.className = `speed tone-${speedTone(t.speed_bps || 0)}`;
        speedBolt.style.display = (t.speed_bps || 0) > 0 ? '' : 'none';
        speedText.textContent = formatSpeed(t.speed_bps);

        uploaded.textContent = formatBytes(t.uploaded);
        size.textContent = formatBytes(t.size);

        peersTotal.textContent = String((t.seeders || 0) + (t.leechers || 0));
        peersSub.textContent = `做种 ${t.seeders || 0} / 下 ${t.leechers || 0}`;

        const tone = ratioTone(Number(t.ratio || 0));
        ratioFill.className = `tone-${tone}`;
        ratioFill.style.width = `${ratioPercent(t.ratio)}%`;
        ratioValue.className = `num-text tone-${tone}`;
        ratioValue.textContent = formatRatio(t.ratio);

        trackerHost.textContent = t.tracker_host || '-';
        trackerHost.dataset.tip = t.tracker_host || '';
        trackerCount.textContent = `共 ${t.tracker_count || 1} 个节点`;

        hashText.textContent = t.info_hash ? `${t.info_hash.slice(0, 10)}…` : '-';
        hashCopy.style.display = t.info_hash ? '' : 'none';

        renderTimes();
    }

    update(torrent);
    return { tr, key: torrent.info_hash, update, refreshTimes: renderTimes };
}

// ==================== src/ui/pagination.js ====================
// 分页条：总数说明 + 每页条数 + 页码 + 快速跳转。

const PAGE_SIZES = [10, 20, 50, 100];

/** 页码窗口：始终保留首尾页，中间围绕当前页，其余折叠为省略号。 */
function pageWindow(page, totalPages) {
    if (totalPages <= 7) return Array.from({ length: totalPages }, (_, i) => i + 1);
    const pages = new Set([1, totalPages, page, page - 1, page + 1]);
    if (page <= 3) [2, 3, 4].forEach(p => pages.add(p));
    if (page >= totalPages - 2) [totalPages - 3, totalPages - 2, totalPages - 1].forEach(p => pages.add(p));

    const sorted = [...pages].filter(p => p >= 1 && p <= totalPages).sort((a, b) => a - b);
    const out = [];
    let prev = 0;
    for (const p of sorted) {
        if (prev && p - prev > 1) out.push('…');
        out.push(p);
        prev = p;
    }
    return out;
}

function createPagination({ onChange }) {
    const totalText = h('span', { class: 'page-total' });
    const pagesEl = h('div', { class: 'page-numbers' });

    const sizeSelect = h('select', {
        class: 'select',
        'aria-label': '每页条数',
        onChange: e => onChange({ page: 1, pageSize: Number(e.target.value) }),
    }, PAGE_SIZES.map(n => h('option', { value: String(n), text: `${n} 条/页` })));

    const jumper = h('input', {
        class: 'input input-sm',
        type: 'number',
        min: '1',
        'aria-label': '跳至页码',
        placeholder: '页码',
        onKeydown: e => {
            if (e.key !== 'Enter') return;
            const value = Number(e.target.value);
            if (Number.isFinite(value) && value >= 1) onChange({ page: Math.floor(value) });
            e.target.value = '';
        },
    });

    const prevBtn = h('button', {
        class: 'icon-btn icon-btn-sm', type: 'button', 'aria-label': '上一页', 'data-tip': '上一页',
    }, icon('left', { size: 15 }));
    const nextBtn = h('button', {
        class: 'icon-btn icon-btn-sm', type: 'button', 'aria-label': '下一页', 'data-tip': '下一页',
    }, icon('right', { size: 15 }));

    const el = h('div', { class: 'pagination' },
        totalText,
        h('div', { class: 'pagination-controls' },
            sizeSelect,
            h('div', { class: 'page-group' }, prevBtn, pagesEl, nextBtn),
            h('label', { class: 'jumper' }, '跳至', jumper, '页'),
        ),
    );

    let state = { total: 0, page: 1, pageSize: 10 };

    prevBtn.addEventListener('click', () => onChange({ page: state.page - 1 }));
    nextBtn.addEventListener('click', () => onChange({ page: state.page + 1 }));

    function update(next) {
        state = { ...state, ...next };
        const { total, page, pageSize } = state;
        const totalPages = Math.max(1, Math.ceil(total / pageSize));
        const from = total === 0 ? 0 : (page - 1) * pageSize + 1;
        const to = Math.min(total, page * pageSize);

        totalText.textContent = total === 0 ? '共 0 个' : `${from}-${to} / 共 ${total} 个`;
        if (sizeSelect.value !== String(pageSize)) sizeSelect.value = String(pageSize);
        prevBtn.disabled = page <= 1;
        nextBtn.disabled = page >= totalPages;
        jumper.max = String(totalPages);

        clearNode(pagesEl);
        for (const item of pageWindow(page, totalPages)) {
            if (item === '…') {
                pagesEl.appendChild(h('span', { class: 'page-gap', text: '…' }));
                continue;
            }
            pagesEl.appendChild(h('button', {
                class: `page-btn${item === page ? ' is-active' : ''}`,
                type: 'button',
                'aria-current': item === page ? 'page' : null,
                onClick: () => onChange({ page: item }),
            }, String(item)));
        }
    }

    return { el, update };
}

// ==================== src/ui/table.js ====================
// 种子列表：筛选 / 搜索 / 排序 / 分页 + 按 key 复用行的增量更新。

const COLUMNS = [
    { key: 'name', title: '种子名称', cls: 'col-name', sort: (a, b) => a._name.localeCompare(b._name) },
    { key: 'status', title: '状态', cls: 'col-status num', sort: (a, b) => a.failures - b.failures },
    { key: 'speed', title: '上传速度', cls: 'col-speed num', sort: (a, b) => a.speed_bps - b.speed_bps },
    { key: 'uploaded', title: '已上传', cls: 'col-num num', sort: (a, b) => a.uploaded - b.uploaded },
    { key: 'size', title: '种子大小', cls: 'col-num num', sort: (a, b) => a.size - b.size },
    { key: 'peers', title: 'Peers (S/L)', cls: 'col-peers num', sort: (a, b) => a._peers - b._peers },
    { key: 'ratio', title: '分享率', cls: 'col-ratio num', sort: (a, b) => a.ratio - b.ratio },
    { key: 'next', title: '下次上报', cls: 'col-time num', sort: (a, b) => a._nextTs - b._nextTs },
    {
        key: 'tracker', title: 'Tracker 地址', cls: 'col-tracker',
        sort: (a, b) => (a.tracker_host || '').localeCompare(b.tracker_host || ''),
    },
    { key: 'hash', title: 'Info Hash', cls: 'col-hash', sort: null },
];

const FILTERS = [
    { key: 'all', label: '全部' },
    { key: 'ok', label: '正常' },
    { key: 'uploading', label: '上传中' },
    { key: 'issue', label: '异常' },
];

const DEFAULT_SORT = { key: 'name', dir: 'asc' };

function emptyHint(filter, query) {
    if (query) return '未找到匹配的种子';
    if (filter === 'issue') return '当前没有异常种子，运行良好';
    if (filter === 'ok') return '没有正常状态的种子';
    if (filter === 'uploading') return '当前暂无正在上传的种子';
    return '请将 .torrent 种子文件放入 torrents 目录';
}

/** 派生排序与搜索需要的字段，避免在比较器里反复计算。 */
function decorate(t) {
    return {
        ...t,
        _name: String(t.name || '').toLowerCase(),
        _peers: (t.seeders || 0) + (t.leechers || 0),
        _nextTs: t.next_announce_at ? new Date(t.next_announce_at).getTime() : Number.MAX_SAFE_INTEGER,
    };
}

function matchesFilter(t, filter) {
    if (filter === 'issue') return Boolean(t.has_issue);
    if (filter === 'ok') return !t.has_issue;
    if (filter === 'uploading') return (t.speed_bps || 0) > 0;
    return true;
}

function matchesQuery(t, kw) {
    if (!kw) return true;
    return `${t.name || ''} ${t.info_hash || ''} ${t.tracker_host || ''} ${t.issue_reason || ''}`
        .toLowerCase().includes(kw);
}

/**
 * @param {{onOpenConfig: () => void}} opts
 */
function createTorrentTable({ onOpenConfig }) {
    const state = { filter: 'all', query: '', sort: { ...DEFAULT_SORT }, page: 1, pageSize: 10 };
    const rows = new Map(); // info_hash -> 行句柄，跨刷新复用
    let torrents = [];
    let searchTimer = 0;

    // ---- 卡片头部 ----
    const countTag = h('span', { class: 'tag tag-primary' });
    const filteredNote = h('span', { class: 'muted note' });

    const filterBtns = new Map();
    const segmented = h('div', { class: 'segmented', role: 'tablist', 'aria-label': '状态筛选' },
        FILTERS.map(f => {
            const label = h('span', { class: 'seg-label' });
            const btn = h('button', {
                class: 'seg-btn',
                type: 'button',
                role: 'tab',
                onClick: () => {
                    state.filter = f.key;
                    state.page = 1;
                    render();
                },
            }, label);
            filterBtns.set(f.key, { btn, label });
            return btn;
        }),
    );

    const clearBtn = h('button', {
        class: 'input-clear',
        type: 'button',
        'aria-label': '清空搜索',
        onClick: () => {
            searchInput.value = '';
            state.query = '';
            state.page = 1;
            render();
        },
    }, icon('close', { size: 13 }));

    const searchInput = h('input', {
        class: 'input',
        type: 'search',
        placeholder: '搜索名称 / Tracker / Hash…',
        'aria-label': '搜索种子',
        onInput: e => {
            const value = e.target.value;
            clearTimeout(searchTimer);
            // 输入防抖，避免每个字符都触发整表重排
            searchTimer = setTimeout(() => {
                state.query = value.trim().toLowerCase();
                state.page = 1;
                render();
            }, 150);
            clearBtn.hidden = value === '';
        },
    });
    clearBtn.hidden = true;

    const toolbar = h('div', { class: 'toolbar' },
        segmented,
        h('div', { class: 'search' }, icon('search', { size: 15, class: 'search-icon' }), searchInput, clearBtn),
        h('button', {
            class: 'icon-btn icon-btn-bordered',
            type: 'button',
            'aria-label': '查看运行时配置',
            'data-tip': '运行时配置',
            onClick: onOpenConfig,
        }, icon('settings', { size: 16 })),
    );

    // ---- 表格骨架 ----
    const headCells = new Map();
    const headRow = h('tr', null, COLUMNS.map(col => {
        const caret = col.sort ? icon('caret', { size: 12, class: 'sort-caret' }) : null;
        const th = h('th', {
            class: [col.cls, col.sort ? 'is-sortable' : ''].filter(Boolean).join(' '),
            scope: 'col',
            ...(col.sort ? { tabindex: '0', role: 'columnheader' } : {}),
        }, h('span', { class: 'th-inner' }, h('span', { text: col.title }), caret));

        if (col.sort) {
            const toggle = () => {
                if (state.sort.key !== col.key) {
                    state.sort = { key: col.key, dir: 'asc' };
                } else if (state.sort.dir === 'asc') {
                    state.sort = { key: col.key, dir: 'desc' };
                } else {
                    state.sort = { ...DEFAULT_SORT }; // 第三次点击回到默认排序
                }
                render();
            };
            th.addEventListener('click', toggle);
            th.addEventListener('keydown', e => {
                if (e.key === 'Enter' || e.key === ' ') {
                    e.preventDefault();
                    toggle();
                }
            });
        }
        headCells.set(col.key, th);
        return th;
    }));

    const tbody = h('tbody');
    const emptyEl = h('div', { class: 'empty' }, icon('inbox', { size: 30 }), h('p', { class: 'empty-text' }));
    const scroller = h('div', { class: 'table-scroll' },
        h('table', { class: 'table' }, h('thead', null, headRow), tbody),
    );

    const pagination = createPagination({
        onChange: patch => {
            Object.assign(state, patch);
            render();
        },
    });

    const el = h('section', { class: 'card' },
        h('div', { class: 'card-head' },
            h('div', { class: 'card-title' }, h('h2', { text: '种子列表' }), countTag, filteredNote),
            toolbar,
        ),
        scroller,
        emptyEl,
        pagination.el,
    );

    function render() {
        const counts = {
            all: torrents.length,
            ok: 0,
            uploading: 0,
            issue: 0,
        };
        for (const t of torrents) {
            if (t.has_issue) counts.issue++; else counts.ok++;
            if ((t.speed_bps || 0) > 0) counts.uploading++;
        }
        for (const f of FILTERS) {
            const ref = filterBtns.get(f.key);
            ref.label.textContent = `${f.label} (${counts[f.key]})`;
            ref.btn.classList.toggle('is-active', state.filter === f.key);
            ref.btn.classList.toggle('is-danger', f.key === 'issue' && counts.issue > 0);
            ref.btn.setAttribute('aria-selected', String(state.filter === f.key));
        }

        const items = torrents
            .filter(t => matchesFilter(t, state.filter) && matchesQuery(t, state.query))
            .map(decorate);

        const column = COLUMNS.find(c => c.key === state.sort.key);
        if (column?.sort) {
            const dir = state.sort.dir === 'desc' ? -1 : 1;
            items.sort((a, b) => column.sort(a, b) * dir);
        }

        const totalPages = Math.max(1, Math.ceil(items.length / state.pageSize));
        state.page = Math.min(Math.max(1, state.page), totalPages);
        const start = (state.page - 1) * state.pageSize;
        const pageItems = items.slice(start, start + state.pageSize);

        // 行复用：按 info_hash 命中已有行只更新文本，未命中才新建
        const liveKeys = new Set(torrents.map(t => t.info_hash));
        const visibleKeys = new Set();
        for (const t of pageItems) {
            visibleKeys.add(t.info_hash);
            let row = rows.get(t.info_hash);
            if (row) {
                row.update(t);
            } else {
                row = createTorrentRow(t);
                rows.set(t.info_hash, row);
            }
            tbody.appendChild(row.tr); // 顺序追加即完成重排
        }

        let detached = false;
        for (const [key, row] of rows) {
            if (visibleKeys.has(key)) continue;
            if (row.tr.parentNode) {
                row.tr.remove();
                detached = true;
            }
            if (!liveKeys.has(key)) rows.delete(key);
        }
        if (detached) closePopover(); // 锚点行已移除，避免悬浮卡片留在页面上

        countTag.textContent = `共 ${torrents.length} 个`;
        filteredNote.textContent = state.query ? `筛选出 ${items.length} 个` : '';

        for (const col of COLUMNS) {
            const th = headCells.get(col.key);
            const active = col.sort && state.sort.key === col.key;
            th.classList.toggle('is-sorted', Boolean(active));
            th.classList.toggle('is-desc', Boolean(active) && state.sort.dir === 'desc');
            if (col.sort) th.setAttribute('aria-sort', active ? (state.sort.dir === 'asc' ? 'ascending' : 'descending') : 'none');
        }

        emptyEl.hidden = pageItems.length > 0;
        emptyEl.querySelector('.empty-text').textContent = emptyHint(state.filter, state.query);
        pagination.el.hidden = items.length === 0;
        pagination.update({ total: items.length, page: state.page, pageSize: state.pageSize });
    }

    return {
        el,
        update(next) {
            torrents = next;
            render();
        },
        // 秒级刷新可见行里的相对时间文本
        refreshTimes() {
            for (const row of rows.values()) {
                if (row.tr.parentNode) row.refreshTimes();
            }
        },
    };
}

// ==================== src/ui/drawer.js ====================
// 右侧抽屉：遮罩 + 面板，支持 ESC / 遮罩点击关闭，关闭后归还焦点。

/**
 * @param {{title: Node|string, width?: number, onOpen?: () => void}} opts
 */
function createDrawer(opts) {
    let lastFocused = null;

    const closeBtn = h('button', {
        class: 'icon-btn',
        type: 'button',
        'aria-label': '关闭',
        'data-tip': '关闭',
        onClick: () => close(),
    }, icon('close', { size: 17 }));

    const body = h('div', { class: 'drawer-body' });
    const panel = h('aside', {
        class: 'drawer-panel',
        role: 'dialog',
        'aria-modal': 'true',
        'aria-label': typeof opts.title === 'string' ? opts.title : '抽屉',
        style: opts.width ? { '--drawer-width': `${opts.width}px` } : null,
    },
        h('header', { class: 'drawer-head' },
            h('div', { class: 'drawer-title' }, opts.title),
            closeBtn,
        ),
        body,
    );

    const overlay = h('div', {
        class: 'drawer-overlay',
        onClick: e => {
            if (e.target === overlay) close();
        },
    }, panel);

    const onKeydown = e => {
        if (e.key === 'Escape') {
            e.stopPropagation();
            close();
        }
    };

    function open() {
        if (overlay.isConnected) return;
        lastFocused = document.activeElement;
        document.body.appendChild(overlay);
        document.documentElement.classList.add('scroll-locked');
        document.addEventListener('keydown', onKeydown);
        requestAnimationFrame(() => overlay.classList.add('is-open'));
        closeBtn.focus({ preventScroll: true });
        opts.onOpen?.();
    }

    function close() {
        if (!overlay.isConnected) return;
        overlay.classList.remove('is-open');
        document.removeEventListener('keydown', onKeydown);
        document.documentElement.classList.remove('scroll-locked');
        // 等退场动画结束再摘除节点
        setTimeout(() => overlay.remove(), 220);
        lastFocused?.focus?.({ preventScroll: true });
    }

    return { body, open, close, toggle: () => (overlay.isConnected ? close() : open()) };
}

// ==================== src/ui/config-panel.js ====================
// 运行时配置（只读）：按主题分组展示 /api/config 返回的键值对。

const CONFIG_GROUPS = [
    {
        title: '核心保种与客户端', icon: 'control',
        keys: ['client', 'simultaneous_seed', 'scan_interval_seconds', 'shutdown_stop_timeout_seconds'],
    },
    {
        title: '目录与状态持久化', icon: 'folder',
        keys: ['torrents_dir', 'archive_dir', 'clients_dir', 'state_file', 'logging.file'],
    },
    {
        title: '上传速率与策略', icon: 'uploaded',
        keys: [
            'uploaded.strategy', 'uploaded.configured_rate_bps', 'uploaded.min_rate_bps',
            'uploaded.max_rate_bps', 'uploaded.conservative_rate_bps', 'uploaded.random_jitter_percent',
            'uploaded.random_refresh_seconds', 'uploaded.ratio_target',
        ],
    },
    { title: 'Announce 网络配置', icon: 'globe', keys: ['announce.port', 'announce.ip', 'announce.ipv6'] },
    {
        title: 'Tracker 连接与重试', icon: 'tracker',
        keys: [
            'tracker.timeout_seconds', 'tracker.proxy', 'tracker.reuse_connections',
            'tracker.max_idle_conns', 'tracker.max_idle_conns_per_host', 'tracker.idle_conn_timeout_seconds',
            'tracker.failure_backoff_min_seconds', 'tracker.failure_backoff_max_seconds',
        ],
    },
    {
        title: '监控与指标服务', icon: 'gauge',
        keys: ['metrics.enabled', 'metrics.listen', 'metrics.path', 'metrics.webui'],
    },
];

// 这些占位值没有复制价值
const NON_COPYABLE = new Set(['', '无', '自动检测', '标准输出']);
const MONO_HINTS = ['dir', 'file', 'ip', 'client', 'listen', 'path', 'proxy'];

function isMono(key) {
    return MONO_HINTS.some(hint => key.includes(hint));
}

/** 按 CONFIG_GROUPS 分组，未命中的键归入“其它配置”。 */
function groupItems(items) {
    const map = new Map(items.map(it => [it.key, it]));
    const used = new Set();
    const sections = [];

    for (const group of CONFIG_GROUPS) {
        const bucket = [];
        for (const key of group.keys) {
            if (map.has(key)) {
                bucket.push(map.get(key));
                used.add(key);
            }
        }
        if (bucket.length) sections.push({ ...group, items: bucket });
    }

    const rest = items.filter(it => !used.has(it.key));
    if (rest.length) sections.push({ title: '其它配置', icon: 'control', items: rest });
    return sections;
}

function configRow(item) {
    const copyable = !NON_COPYABLE.has(String(item.value || '').trim());
    const copyBtn = copyable
        ? h('button', {
            class: 'icon-btn icon-btn-sm copy-btn',
            type: 'button',
            'data-tip': '复制值',
            'aria-label': `复制 ${item.label}`,
            onClick: async () => {
                const ok = await copyText(item.value);
                showToast(ok ? `${item.label} 已复制` : '复制失败，请手动选择文本', ok ? 'success' : 'error');
            },
        }, icon('copy', { size: 12 }))
        : null;

    return h('div', { class: 'cfg-row' },
        h('span', { class: 'cfg-label', text: item.label || item.key, title: item.key }),
        h('span', { class: 'cfg-value' },
            h('span', { class: isMono(item.key) ? 'mono' : '', text: item.value || '-' }),
            copyBtn,
        ),
    );
}

function alertBox(type, title, text, onClose) {
    return h('div', { class: `alert alert-${type}`, role: type === 'error' ? 'alert' : null },
        icon(type === 'error' ? 'fail' : 'info', { size: 15, class: 'alert-icon' }),
        h('div', { class: 'alert-body' },
            h('strong', { text: title }),
            h('p', { text }),
        ),
        onClose
            ? h('button', { class: 'icon-btn icon-btn-sm', type: 'button', 'aria-label': '关闭提示', onClick: onClose },
                icon('close', { size: 13 }))
            : null,
    );
}

function createConfigPanel() {
    const sectionsEl = h('div', { class: 'cfg-sections' });
    const statusEl = h('div', { class: 'cfg-status' });

    const el = h('div', { class: 'cfg' },
        alertBox('info', '提示', '修改 config.toml 后，可发送 SIGHUP 信号或重启服务以应用新配置。'),
        statusEl,
        sectionsEl,
    );

    async function load() {
        clearNode(statusEl);
        statusEl.appendChild(h('div', { class: 'loading' }, h('span', { class: 'spinner' }), '正在读取配置…'));

        try {
            const items = await fetchRuntimeConfig();
            clearNode(statusEl);
            clearNode(sectionsEl);
            for (const section of groupItems(items)) {
                sectionsEl.appendChild(h('section', { class: 'cfg-card' },
                    h('header', { class: 'cfg-card-head' },
                        icon(section.icon, { size: 15, class: 'tone-primary' }),
                        h('span', { text: section.title }),
                        h('span', { class: 'tag', text: `${section.items.length} 项` }),
                    ),
                    h('div', { class: 'cfg-card-body' }, section.items.map(configRow)),
                ));
            }
            if (!items.length) {
                sectionsEl.appendChild(h('div', { class: 'empty' },
                    icon('inbox', { size: 28 }), h('p', { class: 'empty-text', text: '后端未返回配置项' })));
            }
        } catch (err) {
            console.error(err);
            clearNode(statusEl);
            statusEl.appendChild(alertBox(
                'error', '加载失败', '配置加载失败，请确认后端服务状态后重试',
                () => clearNode(statusEl),
            ));
        }
    }

    return { el, load };
}

// ==================== src/main.js ====================
// 应用入口：装配顶栏 / 列表 / 配置抽屉，并把 SSE 数据分发给各部件。

/** 汇总顶栏需要的整体指标。 */
function summarize(torrents) {
    let issues = 0;
    let totalSpeed = 0;
    let totalUploaded = 0;
    let nextTs = Number.MAX_SAFE_INTEGER;

    for (const t of torrents) {
        if (t.has_issue) issues++;
        totalSpeed += t.speed_bps || 0;
        totalUploaded += t.uploaded || 0;
        const ts = t.next_announce_at ? new Date(t.next_announce_at).getTime() : Number.MAX_SAFE_INTEGER;
        if (ts < nextTs) nextTs = ts;
    }

    return { total: torrents.length, issues, totalSpeed, totalUploaded, nextTs };
}

function bootstrap() {
    const root = document.getElementById('root');
    const theme = createThemeController();
    const store = createStore({ torrents: [], conn: 'connecting' });

    initTooltips();

    const configPanel = createConfigPanel();
    const drawer = createDrawer({
        title: h('span', { class: 'drawer-title-inner' },
            icon('control', { size: 16, class: 'tone-primary' }),
            h('span', { text: '运行时配置' }),
            h('span', { class: 'tag tag-primary', text: '只读视图' }),
        ),
        width: 580,
        // 每次打开都重新拉取，保证看到的是当前进程的配置
        onOpen: () => configPanel.load(),
    });
    drawer.body.appendChild(configPanel.el);

    const table = createTorrentTable({ onOpenConfig: () => drawer.open() });
    const header = createHeader({ theme, onOpenConfig: () => drawer.open() });

    root.appendChild(h('div', { class: 'app' },
        header.el,
        h('main', { class: 'app-main' }, table.el),
    ));

    store.subscribe(state => {
        header.stats.update(summarize(state.torrents), state.conn);
        table.update(state.torrents);
    });

    subscribeTorrentFeed(
        payload => store.set({ torrents: payload.torrents }),
        conn => {
            if (store.get().conn !== conn) store.set({ conn });
        },
    );

    // SSE 空闲时最长 15s 才推送一次，本地秒级刷新相对时间，避免"下次上报"看起来停滞
    setInterval(() => {
        header.stats.refreshTimes();
        table.refreshTimes();
    }, 1000);
}

bootstrap();

})();
