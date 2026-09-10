// 右侧抽屉：遮罩 + 面板，支持 ESC / 遮罩点击关闭，关闭后归还焦点。
import { h } from '../lib/dom.js';
import { icon } from './icons.js';

/**
 * @param {{title: Node|string, width?: number, onOpen?: () => void}} opts
 */
export function createDrawer(opts) {
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
