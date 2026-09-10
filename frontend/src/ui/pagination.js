// 分页条：总数说明 + 每页条数 + 页码 + 快速跳转。
import { h, clearNode } from '../lib/dom.js';
import { icon } from './icons.js';

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

export function createPagination({ onChange }) {
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
