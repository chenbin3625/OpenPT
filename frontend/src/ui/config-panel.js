// 运行时配置（只读）：按主题分组展示 /api/config 返回的键值对。
import { h, clearNode } from '../lib/dom.js';
import { fetchRuntimeConfig } from '../lib/api.js';
import { copyText } from '../lib/clipboard.js';
import { icon } from './icons.js';
import { showToast } from './toast.js';

const CONFIG_GROUPS = [
    {
        title: '核心保种与客户端', icon: 'control',
        keys: ['client', 'simultaneous_seed', 'scan_interval_seconds', 'shutdown_stop_timeout_seconds'],
    },
    {
        title: '目录与状态持久化', icon: 'folder',
        keys: ['torrents_dir', 'archive_dir', 'archive_retries', 'clients_dir', 'state_file', 'logging.file'],
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

export function createConfigPanel() {
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
