import { useEffect, useState, useMemo } from 'react';
import { Drawer, Descriptions, Spin, Tag, Typography, Alert, Card, Space, theme } from 'antd';
import {
    ControlOutlined,
    FolderOpenOutlined,
    CloudUploadOutlined,
    ApiOutlined,
    DashboardOutlined,
    GlobalOutlined,
} from '@ant-design/icons';
import { fetchConfig } from '../api';

const { Text } = Typography;

const GROUP_CONFIG = [
    {
        title: '核心保种与客户端',
        icon: <ControlOutlined />,
        keys: ['client', 'simultaneous_seed', 'scan_interval_seconds', 'shutdown_stop_timeout_seconds'],
    },
    {
        title: '目录与状态持久化',
        icon: <FolderOpenOutlined />,
        keys: ['torrents_dir', 'archive_dir', 'clients_dir', 'state_file', 'logging.file'],
    },
    {
        title: '上传速率与策略',
        icon: <CloudUploadOutlined />,
        keys: [
            'uploaded.strategy',
            'uploaded.configured_rate_bps',
            'uploaded.min_rate_bps',
            'uploaded.max_rate_bps',
            'uploaded.conservative_rate_bps',
            'uploaded.random_jitter_percent',
            'uploaded.random_refresh_seconds',
            'uploaded.ratio_target',
        ],
    },
    {
        title: 'Announce 网络配置',
        icon: <GlobalOutlined />,
        keys: ['announce.port', 'announce.ip', 'announce.ipv6'],
    },
    {
        title: 'Tracker 连接与重试',
        icon: <ApiOutlined />,
        keys: [
            'tracker.timeout_seconds',
            'tracker.proxy',
            'tracker.reuse_connections',
            'tracker.max_idle_conns',
            'tracker.max_idle_conns_per_host',
            'tracker.idle_conn_timeout_seconds',
            'tracker.failure_backoff_min_seconds',
            'tracker.failure_backoff_max_seconds',
        ],
    },
    {
        title: '监控与指标服务',
        icon: <DashboardOutlined />,
        keys: ['metrics.enabled', 'metrics.listen', 'metrics.path', 'metrics.webui'],
    },
];

export default function ConfigDrawer({ open, onClose }) {
    const [items, setItems] = useState([]);
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState(null);
    const { token } = theme.useToken();

    useEffect(() => {
        if (!open) return;
        let cancelled = false;
        setLoading(true);
        setError(null);
        fetchConfig()
            .then(data => {
                if (!cancelled) setItems(data);
            })
            .catch(err => {
                console.error(err);
                if (!cancelled) setError('配置加载失败，请确认后端服务状态后重试');
            })
            .finally(() => {
                if (!cancelled) setLoading(false);
            });
        return () => {
            cancelled = true;
        };
    }, [open]);

    const itemMap = useMemo(() => {
        const map = new Map();
        for (const it of items) {
            map.set(it.key, it);
        }
        return map;
    }, [items]);

    const groupedSections = useMemo(() => {
        const matchedKeys = new Set();
        const sections = GROUP_CONFIG.map(group => {
            const groupItems = [];
            for (const key of group.keys) {
                if (itemMap.has(key)) {
                    groupItems.push(itemMap.get(key));
                    matchedKeys.add(key);
                }
            }
            return {
                ...group,
                items: groupItems,
            };
        }).filter(g => g.items.length > 0);

        // 收集未分类的项
        const unassigned = [];
        for (const it of items) {
            if (!matchedKeys.has(it.key)) {
                unassigned.push(it);
            }
        }
        if (unassigned.length > 0) {
            sections.push({
                title: '其它配置',
                icon: <ControlOutlined />,
                items: unassigned,
            });
        }
        return sections;
    }, [itemMap, items]);

    return (
        <Drawer
            title={
                <Space align="center">
                    <ControlOutlined style={{ color: token.colorPrimary }} />
                    <span>运行时配置</span>
                    <Tag color="blue" bordered={false}>只读视图</Tag>
                </Space>
            }
            open={open}
            onClose={onClose}
            placement="right"
            width={580}
            styles={{ body: { padding: 16 } }}
        >
            <Spin spinning={loading}>
                <Space direction="vertical" size={14} style={{ width: '100%' }}>
                    {error && (
                        <Alert
                            message="加载失败"
                            description={error}
                            type="error"
                            showIcon
                            closable
                            onClose={() => setError(null)}
                            style={{ borderRadius: 8 }}
                        />
                    )}
                    <Alert
                        message="提示"
                        description="修改 config.toml 配置文件后，可发送 SIGHUP 信号或重启服务应用新配置。"
                        type="info"
                        showIcon
                        style={{ borderRadius: 8 }}
                    />

                    {groupedSections.map(sec => (
                        <Card
                            key={sec.title}
                            size="small"
                            title={
                                <Space size={8} style={{ fontSize: 13, fontWeight: 600 }}>
                                    <span style={{ color: token.colorPrimary }}>{sec.icon}</span>
                                    <span>{sec.title}</span>
                                </Space>
                            }
                            styles={{
                                header: { minHeight: 38, background: token.colorFillAlter },
                                body: { padding: '8px 12px' },
                            }}
                            style={{ borderRadius: 8, borderColor: token.colorBorderSecondary }}
                        >
                            <Descriptions column={1} size="small" styles={{ label: { width: 150 } }}>
                                {sec.items.map(it => (
                                    <Descriptions.Item key={it.key} label={<Text type="secondary">{it.label}</Text>}>
                                        <Text
                                            style={{
                                                fontFamily: it.key.includes('dir') || it.key.includes('file') || it.key.includes('ip') || it.key.includes('client')
                                                    ? 'monospace'
                                                    : 'inherit',
                                                fontSize: 12,
                                                wordBreak: 'break-all',
                                            }}
                                            copyable={
                                                it.value && it.value !== '无' && it.value !== '自动检测' && it.value !== '标准输出'
                                                    ? { text: it.value }
                                                    : false
                                            }
                                        >
                                            {it.value}
                                        </Text>
                                    </Descriptions.Item>
                                ))}
                            </Descriptions>
                        </Card>
                    ))}
                </Space>
            </Spin>
        </Drawer>
    );
}
