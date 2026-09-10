// 事件委托式 tooltip：任何带 data-tip 属性的元素都会自动获得提示。
import { h } from '../lib/dom.js';

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

export function initTooltips() {
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
