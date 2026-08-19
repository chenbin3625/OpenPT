import { useMemo, useEffect, useState } from 'react';
import {
    Card,
    Table,
    Popover,
    Tag,
    Progress,
    Input,
    Button,
    Space,
    Segmented,
    Empty,
    Descriptions,
    Flex,
    Typography,
    Tooltip,
    message,
    theme,
} from 'antd';
import {
    SettingOutlined,
    CheckCircleFilled,
    ExclamationCircleFilled,
    CloseCircleFilled,
    CopyOutlined,
    SearchOutlined,
    ThunderboltFilled,
    ApiOutlined,
} from '@ant-design/icons';
import {
    formatBytes,
    formatSpeed,
    formatRatio,
    formatTime,
    formatRelative,
    formatDuration,
    eventLabel,
} from '../format';

const { Text, Paragraph } = Typography;

const ordinal = t => (t.tracker_count > 0 ? ((t.tracker_index || 0) + 1) + '/' + t.tracker_count : '-');

const speedType = bps => {
    if (bps <= 0) return 'secondary';
    if (bps < 10240) return undefined;
    if (bps < 102400) return 'warning';
    return 'success';
};

const ratioType = r => {
    if (r >= 1) return 'success';
    if (r >= 0.5) return 'warning';
    return 'danger';
};

const statusMeta = t => {
    if (t.has_issue) {
        if (t.failures > 0) {
            return {
                color: 'error',
                text: `失败 ${t.failures}`,
                icon: <CloseCircleFilled />,
            };
        }
        return { color: 'warning', text: '异常', icon: <ExclamationCircleFilled /> };
    }
    return { color: 'success', text: '正常', icon: <CheckCircleFilled /> };
};

// 状态卡片 Popover
const StatusDetail = ({ t }) => {
    const { token } = theme.useToken();
    const meta = statusMeta(t);

    const copyError = () => {
        if (t.last_error) {
            navigator.clipboard.writeText(t.last_error);
            message.success('错误信息已复制到剪贴板');
        }
    };

    const items = [
        { key: 'status', label: '运行状态', children: <Tag color={meta.color} icon={meta.icon} bordered={false}>{meta.text}</Tag> },
        {
            key: 'last',
            label: '上次上报',
            children: `${formatTime(t.last_announce_at)} (${formatRelative(t.last_announce_at)})`,
        },
        {
            key: 'next',
            label: '下次上报',
            children: `${formatTime(t.next_announce_at)} (${formatRelative(t.next_announce_at)})`,
        },
        {
            key: 'event',
            label: '上报类型',
            children: `${eventLabel(t.next_event)} · 周期 ${formatDuration(t.last_interval_seconds)}`,
        },
        {
            key: 'tracker',
            label: 'Tracker 节点',
            children: (
                <Space size={4}>
                    <ApiOutlined style={{ color: token.colorPrimary }} />
                    <Text orientation="left" style={{ fontFamily: 'monospace', fontSize: 12 }}>{t.tracker_host || '-'}</Text>
                    <Tag bordered={false} style={{ fontSize: 11 }}>序号 {ordinal(t)}</Tag>
                </Space>
            ),
        },
        {
            key: 'peers',
            label: 'Peers 节点',
            children: (
                <Space size={8}>
                    <span>做种: <Text strong style={{ color: token.colorSuccess }}>{t.seeders || 0}</Text></span>
                    <span>下载: <Text strong style={{ color: t.leechers > 0 ? token.colorWarning : token.colorTextSecondary }}>{t.leechers || 0}</Text></span>
                </Space>
            ),
        },
    ];

    return (
        <div style={{ width: 380, maxWidth: 'calc(100vw - 32px)' }}>
            <Descriptions column={1} size="small" items={items} />
            {t.last_error && (
                <div
                    style={{
                        marginTop: 10,
                        padding: '8px 10px',
                        background: token.colorErrorBg,
                        border: `1px solid ${token.colorErrorBorder}`,
                        borderRadius: 6,
                    }}
                >
                    <Flex justify="space-between" align="center" style={{ marginBottom: 4 }}>
                        <Text type="danger" strong style={{ fontSize: 12 }}>最新错误详情</Text>
                        <Tooltip title="复制错误">
                            <Button
                                type="text"
                                size="small"
                                icon={<CopyOutlined />}
                                onClick={copyError}
                                style={{ height: 20, paddingInline: 4 }}
                            />
                        </Tooltip>
                    </Flex>
                    <Text
                        type="danger"
                        style={{
                            display: 'block',
                            fontFamily: 'monospace',
                            fontSize: 11,
                            wordBreak: 'break-all',
                            maxHeight: 120,
                            overflowY: 'auto',
                        }}
                    >
                        {t.last_error}
                    </Text>
                </div>
            )}
        </div>
    );
};

function useDebouncedValue(value, delay = 150) {
    const [debounced, setDebounced] = useState(value);
    useEffect(() => {
        const id = setTimeout(() => setDebounced(value), delay);
        return () => clearTimeout(id);
    }, [value, delay]);
    return debounced;
}

export default function TorrentTable({ torrents, onOpenConfig }) {
    const [filter, setFilter] = useState('all');
    const [query, setQuery] = useState('');
    const search = useDebouncedValue(query);
    const { token } = theme.useToken();

    const activeUploadCount = useMemo(() => torrents.filter(t => (t.speed_bps || 0) > 0).length, [torrents]);
    const issueCount = useMemo(() => torrents.filter(t => t.has_issue).length, [torrents]);
    const okCount = useMemo(() => torrents.filter(t => !t.has_issue).length, [torrents]);

    const rows = useMemo(() => {
        const kw = search.trim().toLowerCase();
        return torrents
            .map(t => {
                if (kw) {
                    const hay = `${t.name || ''} ${t.info_hash || ''} ${t.tracker_host || ''} ${t.issue_reason || ''}`.toLowerCase();
                    if (!hay.includes(kw)) return null;
                }
                if (filter === 'issue' && !t.has_issue) return null;
                if (filter === 'ok' && t.has_issue) return null;
                if (filter === 'uploading' && (t.speed_bps || 0) <= 0) return null;
                return {
                    ...t,
                    peers_total: (t.seeders || 0) + (t.leechers || 0),
                    next_announce_ts: t.next_announce_at ? new Date(t.next_announce_at).getTime() : Number.MAX_SAFE_INTEGER,
                    _sortName: String(t.name || '').toLowerCase(),
                };
            })
            .filter(Boolean);
    }, [torrents, filter, search]);

    const emptyText = useMemo(() => {
        if (filter === 'issue') return <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="当前没有异常种子，运行良好" />;
        if (filter === 'ok') return <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="没有正常状态的种子" />;
        if (filter === 'uploading') return <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="当前暂无正在上传的种子" />;
        if (query) return <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="未找到匹配的种子" />;
        return <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="请将 .torrent 种子文件放入 torrents 目录" />;
    }, [filter, query]);

    const columns = useMemo(() => [
        {
            title: '种子名称',
            dataIndex: 'name',
            defaultSortOrder: 'ascend',
            sorter: (a, b) => a._sortName.localeCompare(b._sortName),
            width: 320,
            render: (name, t) => (
                <div style={{ overflow: 'hidden' }}>
                    <Tooltip title={name} placement="topLeft">
                        <Text
                            strong
                            style={{
                                display: 'block',
                                overflow: 'hidden',
                                textOverflow: 'ellipsis',
                                whiteSpace: 'nowrap',
                                cursor: 'default',
                            }}
                        >
                            {name}
                        </Text>
                    </Tooltip>
                    <Flex align="center" gap={6} style={{ marginTop: 2 }}>
                        <Tag bordered={false} style={{ margin: 0, fontSize: 11, paddingInline: 4 }}>
                            Tracker {ordinal(t)}
                        </Tag>
                        <Text type="secondary" style={{ fontSize: 11, whiteSpace: 'nowrap' }}>
                            {eventLabel(t.next_event)}
                        </Text>
                    </Flex>
                </div>
            ),
        },
        {
            title: '状态',
            key: 'status',
            align: 'right',
            sorter: (a, b) => a.failures - b.failures,
            width: 120,
            render: (_, t) => {
                const m = statusMeta(t);
                const tag = (
                    <Tag
                        color={m.color}
                        icon={m.icon}
                        bordered={false}
                        style={{ cursor: t.has_issue ? 'pointer' : 'default', margin: 0 }}
                    >
                        {m.text}
                    </Tag>
                );
                return (
                    <Flex vertical align="end" gap={2}>
                        {t.has_issue ? (
                            <Popover
                                title={t.name}
                                content={<StatusDetail t={t} />}
                                trigger="hover"
                                placement="bottomRight"
                                overlayStyle={{ maxWidth: 'calc(100vw - 32px)' }}
                            >
                                {tag}
                            </Popover>
                        ) : tag}
                        <Text type="secondary" style={{ fontSize: 11, whiteSpace: 'nowrap' }}>
                            {t.failures > 0 ? `重试 ${formatRelative(t.next_announce_at)}` : eventLabel(t.next_event)}
                        </Text>
                    </Flex>
                );
            },
        },
        {
            title: '上传速度',
            dataIndex: 'speed_bps',
            align: 'right',
            sorter: (a, b) => a.speed_bps - b.speed_bps,
            width: 110,
            render: v => (
                <Flex vertical align="end">
                    <Text
                        type={speedType(v)}
                        strong
                        style={{
                            fontVariantNumeric: 'tabular-nums',
                            display: 'flex',
                            alignItems: 'center',
                            gap: 3,
                        }}
                    >
                        {v > 0 && <ThunderboltFilled style={{ fontSize: 11 }} />}
                        {formatSpeed(v)}
                    </Text>
                </Flex>
            ),
        },
        {
            title: '已上传',
            dataIndex: 'uploaded',
            align: 'right',
            sorter: (a, b) => a.uploaded - b.uploaded,
            width: 110,
            render: v => <Text style={{ fontVariantNumeric: 'tabular-nums' }}>{formatBytes(v)}</Text>,
        },
        {
            title: '种子大小',
            dataIndex: 'size',
            align: 'right',
            sorter: (a, b) => a.size - b.size,
            width: 105,
            render: v => <Text type="secondary" style={{ fontVariantNumeric: 'tabular-nums' }}>{formatBytes(v)}</Text>,
        },
        {
            title: 'Peers (S/L)',
            key: 'peers',
            align: 'right',
            sorter: (a, b) => a.peers_total - b.peers_total,
            width: 100,
            render: (_, t) => (
                <Flex vertical align="end">
                    <Text strong style={{ fontVariantNumeric: 'tabular-nums' }}>
                        {t.peers_total}
                    </Text>
                    <Text type="secondary" style={{ fontSize: 11, whiteSpace: 'nowrap', fontVariantNumeric: 'tabular-nums' }}>
                        做种 {t.seeders || 0} / 下 {t.leechers || 0}
                    </Text>
                </Flex>
            ),
        },
        {
            title: '分享率',
            dataIndex: 'ratio',
            align: 'right',
            sorter: (a, b) => a.ratio - b.ratio,
            width: 135,
            render: (ratio, t) => {
                const grade = ratioType(ratio);
                const pct = ratio >= 1 ? Math.min(100, (ratio / 2) * 100) : ratio >= 0.5 ? Math.min(100, ratio * 100) : Math.max(0, ratio * 100);
                return (
                    <Flex align="center" justify="flex-end" gap={6}>
                        <Progress
                            percent={pct}
                            showInfo={false}
                            size="small"
                            strokeColor={grade === 'success' ? token.colorSuccess : grade === 'warning' ? token.colorWarning : token.colorError}
                            trailColor={token.colorFillAlter}
                            style={{ width: 42, margin: 0 }}
                        />
                        <Text type={grade} strong style={{ fontVariantNumeric: 'tabular-nums' }}>
                            {formatRatio(ratio)}
                        </Text>
                    </Flex>
                );
            },
        },
        {
            title: '下次上报',
            dataIndex: 'next_announce_at',
            align: 'right',
            sorter: (a, b) => a.next_announce_ts - b.next_announce_ts,
            width: 140,
            render: (v, t) => (
                <Flex vertical align="end">
                    <Text strong style={{ fontSize: 12, fontVariantNumeric: 'tabular-nums' }}>
                        {formatRelative(v)}
                    </Text>
                    <Text type="secondary" style={{ fontSize: 11, fontVariantNumeric: 'tabular-nums' }}>
                        {formatTime(v)}
                    </Text>
                </Flex>
            ),
        },
        {
            title: 'Tracker 地址',
            dataIndex: 'tracker_host',
            responsive: ['md'],
            sorter: (a, b) => (a.tracker_host || '').localeCompare(b.tracker_host || ''),
            width: 170,
            render: (v, t) => (
                <Flex vertical align="start">
                    <Text ellipsis title={v} style={{ maxWidth: 150, fontSize: 12 }}>
                        {v || '-'}
                    </Text>
                    <Text type="secondary" style={{ fontSize: 11 }}>
                        共 {t.tracker_count || 1} 个节点
                    </Text>
                </Flex>
            ),
        },
        {
            title: 'Info Hash',
            dataIndex: 'info_hash',
            responsive: ['lg'],
            width: 140,
            render: v => (
                <Paragraph
                    copyable={{ text: v, tooltips: ['复制 InfoHash', '已复制'] }}
                    style={{ margin: 0, fontFamily: 'monospace', fontSize: 12 }}
                >
                    {v ? `${v.slice(0, 10)}...` : '-'}
                </Paragraph>
            ),
        },
    ], [token]);

    return (
        <Card
            variant="borderless"
            className="table-card"
            title={
                <Flex align="center" gap={8}>
                    <Text strong style={{ fontSize: 15 }}>种子列表</Text>
                    <Tag color="blue" bordered={false} style={{ borderRadius: 10, paddingInline: 8 }}>
                        共 {torrents.length} 个
                    </Tag>
                    {search && (
                        <Text type="secondary" style={{ fontSize: 12 }}>
                            (筛选出 {rows.length} 个)
                        </Text>
                    )}
                </Flex>
            }
            extra={
                <Space wrap size={10}>
                    <Segmented
                        className="filter-segmented"
                        value={filter}
                        onChange={setFilter}
                        options={[
                            { label: `全部 (${torrents.length})`, value: 'all' },
                            { label: `正常 (${okCount})`, value: 'ok' },
                            { label: `上传中 (${activeUploadCount})`, value: 'uploading' },
                            {
                                label: (
                                    <span style={{ color: issueCount > 0 ? token.colorError : undefined }}>
                                        异常 ({issueCount})
                                    </span>
                                ),
                                value: 'issue',
                            },
                        ]}
                    />
                    <Input
                        prefix={<SearchOutlined style={{ color: token.colorTextTertiary }} />}
                        placeholder="搜索名称 / Tracker / Hash..."
                        allowClear
                        value={query}
                        onChange={e => setQuery(e.target.value)}
                        style={{ width: 230, borderRadius: 8 }}
                    />
                    <Tooltip title="查看运行时配置">
                        <Button
                            icon={<SettingOutlined />}
                            onClick={onOpenConfig}
                            aria-label="查看配置"
                            style={{ borderRadius: 8 }}
                        />
                    </Tooltip>
                </Space>
            }
            styles={{ body: { padding: 0 } }}
        >
            <Table
                rowKey="info_hash"
                dataSource={rows}
                columns={columns}
                size="middle"
                scroll={{ x: 1260 }}
                rowClassName={t => (t.has_issue ? 'issue' : '')}
                locale={{ emptyText }}
                pagination={{
                    defaultPageSize: 10,
                    showSizeChanger: true,
                    pageSizeOptions: [10, 20, 50, 100],
                    showQuickJumper: true,
                    showTotal: (total, range) => `${range[0]}-${range[1]} / 共 ${total} 个`,
                }}
            />
        </Card>
    );
}
