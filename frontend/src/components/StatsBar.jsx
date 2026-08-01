import { Fragment } from 'react';
import { Flex, Divider, Badge, Typography, theme } from 'antd';
import {
    InboxOutlined,
    WarningOutlined,
    ArrowUpOutlined,
    UploadOutlined,
    ClockCircleOutlined,
} from '@ant-design/icons';
import { formatBytes, formatSpeed, formatRelative } from '../format';

const { Text } = Typography;

const CONN_META = {
    connected: { text: '已连接', badge: 'success' },
    reconnecting: { text: '重连中...', badge: 'warning' },
    connecting: { text: '连接中...', badge: 'processing' },
};

// 紧凑的数据带：图标 + 标签 + 数值，颜色取自主题 token，零自定义样式
export default function StatsBar({ stats, conn }) {
    const { token } = theme.useToken();
    const connMeta = CONN_META[conn] || CONN_META.connecting;
    const next = stats.nextTs === Number.MAX_SAFE_INTEGER ? '暂无' : formatRelative(new Date(stats.nextTs).toISOString());

    const items = [
        { label: '活跃种子', value: stats.total, icon: <InboxOutlined />, color: token.colorPrimary },
        {
            label: '异常种子',
            value: stats.issues,
            icon: <WarningOutlined />,
            color: token.colorError,
            valueColor: stats.issues > 0 ? token.colorError : undefined,
        },
        { label: '上传速度', value: formatSpeed(stats.totalSpeed), icon: <ArrowUpOutlined />, color: token.colorSuccess },
        { label: '总上传量', value: formatBytes(stats.totalUploaded), icon: <UploadOutlined />, color: '#4f46e5' },
        { label: '下次上报', value: next, icon: <ClockCircleOutlined />, color: token.colorWarning },
        { label: '状态', value: connMeta.text, badge: connMeta.badge },
    ];

    return (
        <Flex align="center" wrap gap={0} style={{ flex: 1, minWidth: 0 }}>
            {items.map((s, i) => (
                <Fragment key={s.label}>
                    {i > 0 && <Divider type="vertical" style={{ height: 26 }} />}
                    <Flex align="center" gap={9} style={{ paddingInline: 14 }}>
                        {s.badge ? (
                            <Badge status={s.badge} />
                        ) : (
                            <span style={{ color: s.color, fontSize: 15, display: 'inline-flex' }}>{s.icon}</span>
                        )}
                        <Flex vertical gap={0}>
                            <Text type="secondary" style={{ fontSize: 11, lineHeight: 1.3 }}>{s.label}</Text>
                            <Text strong style={{ fontSize: 14, lineHeight: 1.3, ...(s.valueColor ? { color: s.valueColor } : {}) }}>
                                {s.value}
                            </Text>
                        </Flex>
                    </Flex>
                </Fragment>
            ))}
        </Flex>
    );
}
