// UI 测试用的 mock 后端：提供确定性的 /api/events、/api/config 与图标。
// 数值刻意取整齐的值，方便断言格式化结果（如 i=1 时速率恰为 29.30 KB/s）。
import http from 'node:http';

export const TORRENT_COUNT = 24;
// 每 7 个里 1 个失败、1 个无 peers：24 个中共 6 个异常（i=3,10,17 与 i=5,12,19）
export const ISSUE_COUNT = 6;

function buildTorrents() {
    const now = Date.now();
    return Array.from({ length: TORRENT_COUNT }, (_, i) => {
        const failing = i % 7 === 3;
        const noPeers = i % 7 === 5;
        return {
            info_hash: String(i).padStart(2, '0').repeat(20).slice(0, 40),
            name: `示例种子 ${String(i + 1).padStart(2, '0')} - Release.Name.2026.1080p`,
            size: (i + 1) * 1024 * 1024 * 700,
            uploaded: (i + 1) * 1024 * 1024 * 37,
            speed_bps: i % 3 === 0 ? 0 : (i + 1) * 15000,
            seeders: i * 2,
            leechers: i % 4,
            ratio: i / 5,
            tracker_host: `tracker${i % 3}.example.org`,
            tracker_index: i % 3,
            tracker_count: 3,
            failures: failing ? i : 0,
            has_issue: failing || noPeers,
            issue_reason: failing ? 'announce 连续失败' : noPeers ? '无 peers 返回' : '',
            last_error: failing ? `dial tcp 10.0.0.${i}:443: i/o timeout` : '',
            last_announce_at: new Date(now - (i + 1) * 60000).toISOString(),
            next_announce_at: new Date(now + (i + 1) * 90000).toISOString(),
            last_interval_seconds: 1800,
            retry_in_seconds: failing ? 300 : 0,
            next_event: i % 4 === 0 ? 'started' : i % 4 === 1 ? 'completed' : 'none',
            has_response: true,
        };
    });
}

// 6 个已分组段 + 1 个未分组项（用于验证"其它配置"兜底段）
export const CONFIG_ITEMS = [
    ['client', '模拟客户端', 'qBittorrent 4.6.5'],
    ['simultaneous_seed', '并发保种数', '20'],
    ['scan_interval_seconds', '扫描间隔', '60'],
    ['torrents_dir', '种子目录', '/data/torrents'],
    ['state_file', '状态文件', '/data/state.json'],
    ['uploaded.strategy', '上传策略', 'random'],
    ['announce.port', '监听端口', '45678'],
    ['announce.ip', '对外 IP', '自动检测'],
    ['tracker.proxy', '代理', '无'],
    ['metrics.listen', '监控监听', '0.0.0.0:9090'],
    ['unknown.extra_key', '未分组项', 'hello'],
].map(([key, label, value]) => ({ key, label, value }));

export const CONFIG_SECTION_COUNT = 7;

const ICON = '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24">'
    + '<title>OpenPT</title><circle cx="12" cy="12" r="10" fill="#2563eb"/></svg>';

/** 创建 mock 后端。SSE 每秒推一帧，用于验证行复用与相对时间刷新。 */
export function createMockBackend() {
    // 每次推送重新生成：相对时间始终相对"现在"，断言不会随测试时长漂移。
    // info_hash 与速率只由下标决定，所以行复用的 key 依然稳定。
    return http.createServer((req, res) => {
        const url = (req.url || '/').split('?')[0];

        if (url === '/api/events') {
            res.writeHead(200, {
                'content-type': 'text/event-stream',
                'cache-control': 'no-cache',
                connection: 'keep-alive',
            });
            const push = () => res.write(`data: ${JSON.stringify({ torrents: buildTorrents() })}\n\n`);
            push();
            const timer = setInterval(push, 1000);
            req.on('close', () => clearInterval(timer));
            return;
        }
        if (url === '/api/status') {
            res.writeHead(200, { 'content-type': 'application/json' });
            res.end(JSON.stringify({ torrents: buildTorrents() }));
            return;
        }
        if (url === '/api/config') {
            res.writeHead(200, { 'content-type': 'application/json' });
            res.end(JSON.stringify({ items: CONFIG_ITEMS }));
            return;
        }
        if (url === '/openpt-icon.svg') {
            res.writeHead(200, { 'content-type': 'image/svg+xml' });
            res.end(ICON);
            return;
        }
        res.writeHead(404, { 'content-type': 'text/plain' });
        res.end('not found');
    });
}
