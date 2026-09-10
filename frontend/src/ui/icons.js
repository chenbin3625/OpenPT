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
export function icon(name, opts = {}) {
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
