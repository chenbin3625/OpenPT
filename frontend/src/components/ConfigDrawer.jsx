import { useEffect, useState } from 'react';
import { Drawer, Descriptions, Spin } from 'antd';
import { fetchConfig } from '../api';

export default function ConfigDrawer({ open, onClose }) {
    const [items, setItems] = useState([]);
    const [loading, setLoading] = useState(false);

    useEffect(() => {
        if (!open) return;
        let cancelled = false;
        setLoading(true);
        fetchConfig()
            .then(data => {
                if (!cancelled) setItems(data);
            })
            .catch(err => console.error(err))
            .finally(() => {
                if (!cancelled) setLoading(false);
            });
        return () => {
            cancelled = true;
        };
    }, [open]);

    return (
        <Drawer
            title="当前配置"
            open={open}
            onClose={onClose}
            placement="right"
            width={560}
        >
            <Spin spinning={loading}>
                <Descriptions column={1} size="small">
                    {items.map(it => (
                        <Descriptions.Item key={it.key} label={it.label}>
                            {it.value}
                        </Descriptions.Item>
                    ))}
                </Descriptions>
            </Spin>
        </Drawer>
    );
}
