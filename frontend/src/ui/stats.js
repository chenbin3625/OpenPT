// 顶栏指标带：活跃 / 异常 / 速率 / 总量 / 下次上报 / 连接状态。
import { h } from '../lib/dom.js';
import { formatBytes, formatSpeed, formatRelative } from '../lib/format.js';
import { icon } from './icons.js';

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

export function createStatsBar() {
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
