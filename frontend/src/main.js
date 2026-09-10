// 应用入口：装配顶栏 / 列表 / 配置抽屉，并把 SSE 数据分发给各部件。
import { h } from './lib/dom.js';
import { createStore } from './lib/store.js';
import { subscribeTorrentFeed } from './lib/api.js';
import { createThemeController } from './lib/theme.js';
import { initTooltips } from './ui/tooltip.js';
import { createHeader } from './ui/header.js';
import { createTorrentTable } from './ui/table.js';
import { createDrawer } from './ui/drawer.js';
import { createConfigPanel } from './ui/config-panel.js';
import { icon } from './ui/icons.js';

/** 汇总顶栏需要的整体指标。 */
function summarize(torrents) {
    let issues = 0;
    let totalSpeed = 0;
    let totalUploaded = 0;
    let nextTs = Number.MAX_SAFE_INTEGER;

    for (const t of torrents) {
        if (t.has_issue) issues++;
        totalSpeed += t.speed_bps || 0;
        totalUploaded += t.uploaded || 0;
        const ts = t.next_announce_at ? new Date(t.next_announce_at).getTime() : Number.MAX_SAFE_INTEGER;
        if (ts < nextTs) nextTs = ts;
    }

    return { total: torrents.length, issues, totalSpeed, totalUploaded, nextTs };
}

function bootstrap() {
    const root = document.getElementById('root');
    const theme = createThemeController();
    const store = createStore({ torrents: [], conn: 'connecting' });

    initTooltips();

    const configPanel = createConfigPanel();
    const drawer = createDrawer({
        title: h('span', { class: 'drawer-title-inner' },
            icon('control', { size: 16, class: 'tone-primary' }),
            h('span', { text: '运行时配置' }),
            h('span', { class: 'tag tag-primary', text: '只读视图' }),
        ),
        width: 580,
        // 每次打开都重新拉取，保证看到的是当前进程的配置
        onOpen: () => configPanel.load(),
    });
    drawer.body.appendChild(configPanel.el);

    const table = createTorrentTable({ onOpenConfig: () => drawer.open() });
    const header = createHeader({ theme, onOpenConfig: () => drawer.open() });

    root.appendChild(h('div', { class: 'app' },
        header.el,
        h('main', { class: 'app-main' }, table.el),
    ));

    store.subscribe(state => {
        header.stats.update(summarize(state.torrents), state.conn);
        table.update(state.torrents);
    });

    subscribeTorrentFeed(
        payload => store.set({ torrents: payload.torrents }),
        conn => {
            if (store.get().conn !== conn) store.set({ conn });
        },
    );

    // SSE 空闲时最长 15s 才推送一次，本地秒级刷新相对时间，避免"下次上报"看起来停滞
    setInterval(() => {
        header.stats.refreshTimes();
        table.refreshTimes();
    }, 1000);
}

bootstrap();
