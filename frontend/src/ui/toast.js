// 轻量全局提示（替代组件库的 message）。
import { h } from '../lib/dom.js';
import { icon } from './icons.js';

const TOAST_ICON = { success: 'ok', error: 'fail', info: 'info' };
let toastHost = null;

export function showToast(text, type = 'success') {
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
    el.getBoundingClientRect();
    el.classList.add('is-in');
    setTimeout(() => {
        el.classList.remove('is-in');
        setTimeout(() => el.remove(), 200);
    }, 2400);
}
