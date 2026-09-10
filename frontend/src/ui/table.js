// 种子列表：筛选 / 搜索 / 排序 / 分页 + 按 key 复用行的增量更新。
import { h, clearNode } from '../lib/dom.js';
import { icon } from './icons.js';
import { createTorrentRow } from './row.js';
import { createPagination } from './pagination.js';
import { closePopover } from './popover.js';

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
export function createTorrentTable({ onOpenConfig }) {
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
