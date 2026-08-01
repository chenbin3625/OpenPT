import { useMemo, useState } from 'react';
import { Flex, Typography } from 'antd';
import StatsBar from './components/StatsBar';
import TorrentTable from './components/TorrentTable';
import ConfigDrawer from './components/ConfigDrawer';
import { useTorrentFeed } from './api';

const { Text } = Typography;

export default function App() {
    const { torrents, conn } = useTorrentFeed();
    const [configOpen, setConfigOpen] = useState(false);

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
                    <Flex align="center" gap={9} style={{ flex: '0 0 auto' }}>
                        <img src="/openpt-icon.svg" width="26" height="26" alt="" style={{ borderRadius: 6 }} />
                        <Text strong style={{ fontSize: 15, whiteSpace: 'nowrap' }}>OpenPT 监控面板</Text>
                    </Flex>
                    <StatsBar stats={stats} conn={conn} />
                </div>
            </header>
            <main className="main-container">
                <TorrentTable torrents={torrents} onOpenConfig={() => setConfigOpen(true)} />
            </main>
            <ConfigDrawer open={configOpen} onClose={() => setConfigOpen(false)} />
        </div>
    );
}
