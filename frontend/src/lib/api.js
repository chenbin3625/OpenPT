// 后端接口：SSE 实时状态流 + 运行时配置快照。

/**
 * 订阅 /api/events 的实时状态推送。EventSource 自带自动重连，
 * 这里只把连接状态归一为 connecting / connected / reconnecting 交给 UI 展示。
 *
 * @param {(payload: {torrents: object[]}) => void} onData
 * @param {(state: string) => void} onState
 * @returns {() => void} 取消订阅
 */
export function subscribeTorrentFeed(onData, onState) {
    const es = new EventSource('/api/events');

    es.onopen = () => onState('connected');
    es.onmessage = e => {
        try {
            const data = JSON.parse(e.data);
            onData({ torrents: Array.isArray(data.torrents) ? data.torrents : [] });
        } catch (err) {
            console.error('解析 SSE 数据失败', err);
        }
    };
    es.onerror = () => onState('reconnecting');

    return () => es.close();
}

/** 读取运行时配置（只读视图）。 */
export async function fetchRuntimeConfig() {
    const res = await fetch('/api/config', { headers: { accept: 'application/json' } });
    if (!res.ok) {
        throw new Error(`配置接口返回 ${res.status}`);
    }
    const json = await res.json();
    return Array.isArray(json.items) ? json.items : [];
}
