import { Fragment } from 'react';
import { Flex, Divider, Typography, theme } from 'antd';
import {
    InboxOutlined,
    WarningOutlined,
    ArrowUpOutlined,
    CloudUploadOutlined,
    ClockCircleOutlined,
} from '@ant-design/icons';
import { formatBytes, formatSpeed, formatRelative } from '../format';

const { Text } = Typography;

const CONN_META = {
    connected: { text: '实时同步中', cls: 'connected' },
    reconnecting: { text: '正在重连...', cls: 'reconnecting' },
    connecting: { text: '连接中...', cls: 'connecting' },
};

export default function StatsBar({ stats, conn }) {
    const { token } = theme.useToken();
    const connMeta = CONN_META[conn] || CONN_META.connecting;

    const next =
        stats.nextTs === Number.MAX_SAFE_INTEGER
            ? '暂无'
            : stats.nextTs <= Date.now()
            ? '即将上报'
            : formatRelative(new Date(stats.nextTs).toISOString());

    const items = [
        {
            key: 'active',
            label: '活跃种子',
            value: stats.total,
            icon: <InboxOutlined />,
            color: token.colorPrimary,
            bg: token.colorPrimaryBg,
        },
        {
            key: 'issues',
            label: '异常种子',
            value: stats.issues,
            icon: <WarningOutlined />,
            color: stats.issues > 0 ? token.colorError : token.colorTextTertiary,
            bg: stats.issues > 0 ? token.colorErrorBg : 'transparent',
            valueColor: stats.issues > 0 ? token.colorError : undefined,
        },
        {
            key: 'speed',
            label: '上传速率',
            value: formatSpeed(stats.totalSpeed),
            icon: <ArrowUpOutlined />,
            color: token.colorSuccess,
            bg: token.colorSuccessBg,
        },
        {
            key: 'uploaded',
            label: '总上传量',
            value: formatBytes(stats.totalUploaded),
            icon: <CloudUploadOutlined />,
            color: token.colorInfo,
            bg: token.colorInfoBg,
        },
        {
            key: 'next',
            label: '下次上报',
            value: next,
            icon: <ClockCircleOutlined />,
            color: token.colorWarning,
            bg: token.colorWarningBg,
        },
        {
            key: 'status',
            label: '连接状态',
            value: connMeta.text,
            isStatus: true,
            statusCls: connMeta.cls,
        },
    ];

    return (
        <Flex align="center" wrap gap={0} style={{ flex: 1, minWidth: 0, justifyContent: 'flex-start' }}>
            {items.map((s, i) => (
                <Fragment key={s.key}>
                    {i > 0 && (
                        <Divider
                            type="vertical"
                            style={{
                                height: 28,
                                marginInline: 4,
                                borderColor: token.colorBorderSecondary,
                            }}
                        />
                    )}
                    <div className="stat-pill">
                        {s.isStatus ? (
                            <div className="status-indicator" style={{ marginRight: 2 }}>
                                <div className={`status-dot ${s.statusCls}`} />
                            </div>
                        ) : (
                            <div
                                style={{
                                    width: 28,
                                    height: 28,
                                    borderRadius: 6,
                                    background: s.bg,
                                    color: s.color,
                                    display: 'flex',
                                    alignItems: 'center',
                                    justifyContent: 'center',
                                    fontSize: 14,
                                    flexShrink: 0,
                                }}
                            >
                                {s.icon}
                            </div>
                        )}
                        <Flex vertical gap={0}>
                            <Text type="secondary" style={{ fontSize: 11, lineHeight: 1.2 }}>
                                {s.label}
                            </Text>
                            <Text
                                strong
                                style={{
                                    fontSize: 13,
                                    lineHeight: 1.3,
                                    fontVariantNumeric: 'tabular-nums',
                                    ...(s.valueColor ? { color: s.valueColor } : {}),
                                }}
                            >
                                {s.value}
                            </Text>
                        </Flex>
                    </div>
                </Fragment>
            ))}
        </Flex>
    );
}
