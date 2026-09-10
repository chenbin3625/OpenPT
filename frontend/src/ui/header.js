// 顶部吸顶条：品牌 + 指标带 + 主题切换 / 配置入口。
import { h, clearNode } from '../lib/dom.js';
import { icon } from './icons.js';
import { createStatsBar } from './stats.js';

export function createHeader({ theme, onOpenConfig }) {
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
