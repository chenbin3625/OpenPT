// 悬浮详情卡片。内容里可能有可点击元素（复制按钮），
// 因此离开锚点后留出宽限时间，允许指针移入卡片本体。
import { h, clearNode } from '../lib/dom.js';

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

export function closePopover() {
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
    currentAnchor = anchor;
    // 先渲染再测量，否则拿不到真实高度
    position(anchor);
    if (currentAnchor === anchor) el.style.visibility = '';
}

/**
 * 给锚点挂载悬浮卡片。
 * @param {HTMLElement} anchor
 * @param {() => Node} build 每次展开时重新构建内容，保证数据最新
 * @param {{title?: () => string, canOpen?: () => boolean}} [opts]
 */
export function attachPopover(anchor, build, opts = {}) {
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
