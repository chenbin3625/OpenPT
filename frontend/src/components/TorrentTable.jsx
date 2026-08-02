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
} from 'antd';
import {
    SettingOutlined,
    CheckCircleFilled,
    ExclamationCircleFilled,
    CloseCircleFilled,
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

const { Text } = Typography;

const ordinal = t => (t.tracker_count > 0 ? ((t.tracker_index || 0) + 1) + '/' + t.tracker_count : '-');

// 速度/分享率 -> antd 文本类型
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
                text: '失败 ' + t.failures,
                icon: <CloseCircleFilled />,
            };
        }
        return { color: 'warning', text: '异常', icon: <ExclamationCircleFilled /> };
    }
    return { color: 'success', text: '正常', icon: <CheckCircleFilled /> };
};

// 两行单元格：主文本 + 次要文本
const CellText = ({ primary, sub }) => (
    <div style={{ overflow: 'hidden' }}>
        <Text
            strong
            style={{ display: 'block', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}
        >
            {primary}
        </Text>
        <Text type="secondary" style={{ display: 'block', fontSize: 11, whiteSpace: 'nowrap' }}>
            {sub}
        </Text>
    </div>
);

// 状态悬浮卡内容（仅异常/失败行展示，用 Popover 白底承载）
const StatusDetail = ({ t }) => {
    const items = [
        { key: 'status', label: '状态', children: statusMeta(t).text },
        {
            key: 'last',
            label: '上次上报',
            children: formatTime(t.last_announce_at) + '（' + formatRelative(t.last_announce_at) + '）',
        },
        {
            key: 'next',
            label: '下次上报',
            children: formatTime(t.next_announce_at) + '（' + formatRelative(t.next_announce_at) + '）',
        },
        {
            key: 'event',
            label: '上报类型',
            children: eventLabel(t.next_event) + '，间隔 ' + formatDuration(t.last_interval_seconds),
        },
        { key: 'tracker', label: 'Tracker', children: (t.tracker_host || '-') + '（' + ordinal(t) + '）' },
        { key: 'peers', label: 'Peers', children: '做种 ' + (t.seeders || 0) + ' / 下载 ' + (t.leechers || 0) },
    ];
    return (
        <div style={{ width: 380, maxWidth: 'calc(100vw - 32px)' }}>
            <Descriptions column={1} size="small" items={items} />
            {t.last_error && (
                <Text type="danger" style={{ display: 'block', marginTop: 10, fontFamily: 'monospace', fontSize: 12, wordBreak: 'break-all' }}>
                    {t.last_error}
                </Text>
            )}
        </div>
    );
};

// 搜索输入防抖
function useDebouncedValue(value, delay = 150) {
    const [debounced, setDebounced] = useState(value);
    // 在 effect 中调度定时器：value 每次变化（含清空回空串）都会取消上一个
    // 待触发定时器，卸载时也会清理，避免残留旧值或对已卸载组件 setState。
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

    const rows = useMemo(() => {
        const kw = search.trim().toLowerCase();
        return torrents
            .map(t => {
                if (kw) {
                    const hay = (t.name + ' ' + t.info_hash + ' ' + t.tracker_host + ' ' + t.issue_reason).toLowerCase();
                    if (!hay.includes(kw)) return null;
                }
                if (filter === 'issue' && !t.has_issue) return null;
                if (filter === 'ok' && t.has_issue) return null;
                return {
                    ...t,
                    peers_total: (t.seeders || 0) + (t.leechers || 0),
                    next_announce_ts: t.next_announce_at ? new Date(t.next_announce_at).getTime() : Number.MAX_SAFE_INTEGER,
                    _sortName: String(t.name || '').toLowerCase(),
                };
            })
            .filter(Boolean);
    }, [torrents, filter, search]);

    const issueCount = useMemo(() => torrents.filter(t => t.has_issue).length, [torrents]);

    const emptyText = useMemo(() => {
        if (filter === 'issue') return <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="没有异常种子" />;
        if (filter === 'ok') return <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="没有正常种子" />;
        if (query) return <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="未找到匹配" />;
        return <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="请将 .torrent 文件放入 torrents 目录" />;
    }, [filter, query]);

    const columns = useMemo(() => [
        {
            title: '名称',
            dataIndex: 'name',
            defaultSortOrder: 'ascend',
            sorter: (a, b) => a._sortName.localeCompare(b._sortName),
            width: 300,
            render: (name, t) => (
                <CellText primary={name} sub={`Tracker ${ordinal(t)} · ${eventLabel(t.next_event)}`} />
            ),
        },
        {
            title: '状态',
            key: 'status',
            align: 'right',
            sorter: (a, b) => a.failures - b.failures,
            width: 110,
            render: (_, t) => {
                const m = statusMeta(t);
                const tag = <Tag color={m.color} icon={m.icon} bordered={false}>{m.text}</Tag>;
                return (
                    <Flex vertical align="end" gap={2}>
                        {t.has_issue ? (
                            <Popover
                                title={t.name}
                                content={<StatusDetail t={t} />}
                                trigger="hover"
                                placement="bottom"
                                overlayStyle={{ maxWidth: 'calc(100vw - 32px)' }}
                                overlayInnerStyle={{ width: 380 }}
                            >
                                {tag}
                            </Popover>
                        ) : tag}
                        <Text type="secondary" style={{ fontSize: 11, whiteSpace: 'nowrap' }}>
                            {t.failures > 0 ? '重试 ' + formatRelative(t.next_announce_at) : eventLabel(t.next_event)}
                        </Text>
                    </Flex>
                );
            },
        },
        {
            title: '速度',
            dataIndex: 'speed_bps',
            align: 'right',
            sorter: (a, b) => a.speed_bps - b.speed_bps,
            width: 100,
            render: v => <Text type={speedType(v)} strong>{formatSpeed(v)}</Text>,
        },
        {
            title: '已上传',
            dataIndex: 'uploaded',
            align: 'right',
            sorter: (a, b) => a.uploaded - b.uploaded,
            width: 110,
            render: v => formatBytes(v),
        },
        {
            title: '大小',
            dataIndex: 'size',
            align: 'right',
            sorter: (a, b) => a.size - b.size,
            width: 100,
            render: v => formatBytes(v),
        },
        {
            title: 'Peers',
            key: 'peers',
            align: 'right',
            sorter: (a, b) => a.peers_total - b.peers_total,
            width: 90,
            render: (_, t) => (
                <CellText
                    primary={t.peers_total}
                    sub={`S ${t.seeders || 0} / L ${t.leechers || 0}`}
                />
            ),
        },
        {
            title: '分享率',
            dataIndex: 'ratio',
            align: 'right',
            sorter: (a, b) => a.ratio - b.ratio,
            width: 130,
            render: (ratio, t) => {
                const grade = ratioType(ratio);
                const pct = ratio >= 1 ? Math.min(100, (ratio / 2) * 100) : ratio >= 0.5 ? Math.min(100, ratio * 100) : Math.max(0, ratio * 100);
                return (
                    <Flex align="center" justify="flex-end" gap={7}>
                        <Progress percent={pct} showInfo={false} size="small" strokeColor={grade === 'success' ? '#047857' : grade === 'warning' ? '#b45309' : '#dc2626'} trailColor="#e9eef4" style={{ width: 44 }} />
                        <Text type={grade} strong>{formatRatio(ratio)}</Text>
                    </Flex>
                );
            },
        },
        {
            title: '下次上报',
            dataIndex: 'next_announce_at',
            align: 'right',
            sorter: (a, b) => a.next_announce_ts - b.next_announce_ts,
            width: 150,
            render: (v, t) => <CellText primary={formatRelative(v)} sub={formatTime(v)} />,
        },
        {
            title: '间隔',
            dataIndex: 'last_interval_seconds',
            align: 'right',
            responsive: ['md'],
            sorter: (a, b) => a.last_interval_seconds - b.last_interval_seconds,
            width: 90,
            render: v => formatDuration(v),
        },
        {
            title: 'Tracker',
            dataIndex: 'tracker_host',
            responsive: ['md'],
            sorter: (a, b) => (a.tracker_host || '').localeCompare(b.tracker_host || ''),
            width: 160,
            render: (v, t) => <CellText primary={v || '-'} sub={ordinal(t)} />,
        },
        {
            title: 'Info Hash',
            dataIndex: 'info_hash',
            responsive: ['md'],
            width: 130,
            render: v => (
                <Text style={{ fontFamily: 'monospace', fontSize: 12 }} ellipsis title={v}>
                    {v.slice(0, 16)}...
                </Text>
            ),
        },
    ], []);

    return (
        <Card
            variant="borderless"
            className="table-card"
            title={
                <span>
                    种子列表 <Text type="secondary" style={{ fontSize: 12 }}>{rows.length} 个</Text>
                </span>
            }
            extra={
                <Space wrap>
                    <Segmented
                        value={filter}
                        onChange={setFilter}
                        options={[
                            { label: '全部', value: 'all' },
                            { label: `异常 (${issueCount})`, value: 'issue' },
                            { label: '正常', value: 'ok' },
                        ]}
                    />
                    <Button icon={<SettingOutlined />} onClick={onOpenConfig} aria-label="查看配置" />
                    <Input.Search
                        placeholder="搜索种子..."
                        allowClear
                        value={query}
                        onChange={e => setQuery(e.target.value)}
                        style={{ width: 220 }}
                    />
                </Space>
            }
            styles={{ body: { padding: 0 } }}
        >
            <Table
                rowKey="info_hash"
                dataSource={rows}
                columns={columns}
                size="middle"
                scroll={{ x: 1200 }}
                rowClassName={t => (t.has_issue ? 'issue' : '')}
                locale={{ emptyText }}
                pagination={{
                    defaultPageSize: 8,
                    showSizeChanger: true,
                    pageSizeOptions: [8, 10, 20, 50, 100],
                    showQuickJumper: true,
                    showTotal: (total, range) => `${range[0]}-${range[1]} / 共 ${total} 个`,
                }}
            />
        </Card>
    );
}
