import { useEffect, useState } from 'react';

// 通过 SSE 订阅实时状态；EventSource 自带自动重连
export function useTorrentFeed() {
    const [torrents, setTorrents] = useState([]);
    const [conn, setConn] = useState('connecting'); // connecting | connected | reconnecting

    useEffect(() => {
        const es = new EventSource('/api/events');
        es.onopen = () => setConn('connected');
        es.onmessage = e => {
            try {
                const data = JSON.parse(e.data);
                setTorrents(data.torrents || []);
            } catch (err) {
                console.error(err);
            }
        };
        es.onerror = () => setConn('reconnecting');
        return () => es.close();
    }, []);

    return { torrents, conn };
}

export async function fetchConfig() {
    const res = await fetch('/api/config');
    if (!res.ok) {
        throw new Error(`配置接口返回 ${res.status}`);
    }
    const json = await res.json();
    return json.items || [];
}
