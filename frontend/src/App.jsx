import { useMemo, useState } from 'react';
import { Flex, Button, Tooltip, Space } from 'antd';
import {
    SunOutlined,
    MoonOutlined,
    SettingOutlined,
} from '@ant-design/icons';
import StatsBar from './components/StatsBar';
import TorrentTable from './components/TorrentTable';
import ConfigDrawer from './components/ConfigDrawer';
import { useTorrentFeed } from './api';
import { useAppTheme } from './ThemeContext';

export default function App() {
    const { torrents, conn } = useTorrentFeed();
    const [configOpen, setConfigOpen] = useState(false);
    const { isDark, toggleMode } = useAppTheme();

    const stats = useMemo(() => {
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
    }, [torrents]);

    return (
        <div className="app">
            <header className="app-header">
                <div className="app-header-inner">
                    <div className="brand-logo-container">
                        <img
                            src="/openpt-icon.svg"
                            width="28"
                            height="28"
                            alt="OpenPT Logo"
                            className="brand-icon"
                        />
                        <span className="brand-title">OpenPT</span>
                    </div>

                    <StatsBar stats={stats} conn={conn} />

                    <Space size={8} style={{ flex: '0 0 auto' }}>
                        <Tooltip title={isDark ? '切换为亮色模式' : '切换为暗色模式'}>
                            <Button
                                type="text"
                                icon={isDark ? <SunOutlined /> : <MoonOutlined />}
                                onClick={toggleMode}
                                aria-label="切换主题"
                                style={{ borderRadius: 8 }}
                            />
                        </Tooltip>
                        <Tooltip title="系统配置">
                            <Button
                                type="text"
                                icon={<SettingOutlined />}
                                onClick={() => setConfigOpen(true)}
                                aria-label="查看配置"
                                style={{ borderRadius: 8 }}
                            />
                        </Tooltip>
                    </Space>
                </div>
            </header>
            <main className="main-container">
                <TorrentTable torrents={torrents} onOpenConfig={() => setConfigOpen(true)} />
            </main>
            <ConfigDrawer open={configOpen} onClose={() => setConfigOpen(false)} />
        </div>
    );
}
