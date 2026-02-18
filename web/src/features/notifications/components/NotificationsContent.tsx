'use client';

import {
    Badge,
    Button,
    Card,
    Empty,
    Segmented,
    Space,
    Table,
    Tag,
    Typography,
} from 'antd';
import type { ColumnsType } from 'antd/es/table';
import {
    CheckCircleOutlined,
    ClockCircleOutlined,
    CloseCircleOutlined,
    DesktopOutlined,
    ReloadOutlined,
} from '@ant-design/icons';
import dayjs from 'dayjs';
import { useTranslation } from 'react-i18next';

import { useNotificationsController } from '../hooks/useNotificationsController';
import type { Notification } from '../types';

const { Title, Text } = Typography;

const typeConfig: Record<string, { color: string; icon: React.ReactNode; labelKey: string }> = {
    APPROVAL_PENDING: {
        color: 'orange',
        icon: <ClockCircleOutlined />,
        labelKey: 'notification.type.approval_pending',
    },
    APPROVAL_COMPLETED: {
        color: 'green',
        icon: <CheckCircleOutlined />,
        labelKey: 'notification.type.approval_completed',
    },
    APPROVAL_REJECTED: {
        color: 'red',
        icon: <CloseCircleOutlined />,
        labelKey: 'notification.type.approval_rejected',
    },
    VM_STATUS_CHANGE: {
        color: 'blue',
        icon: <DesktopOutlined />,
        labelKey: 'notification.type.vm_status_change',
    },
};

export function NotificationsContent() {
    const { t } = useTranslation('common');
    const notifications = useNotificationsController({ t });

    const columns: ColumnsType<Notification> = [
        {
            title: t('table.status'),
            dataIndex: 'read',
            key: 'read',
            width: 120,
            render: (read: boolean) => (
                <Tag color={read ? 'default' : 'blue'}>
                    {read ? t('notification.read') : t('notification.unread')}
                </Tag>
            ),
        },
        {
            title: t('notification.type'),
            dataIndex: 'type',
            key: 'type',
            width: 220,
            render: (type: Notification['type']) => {
                const cfg = typeConfig[type] ?? typeConfig.APPROVAL_PENDING;
                return (
                    <Space>
                        {cfg.icon}
                        <Tag color={cfg.color}>{t(cfg.labelKey)}</Tag>
                    </Space>
                );
            },
        },
        {
            title: t('table.name'),
            dataIndex: 'title',
            key: 'title',
            render: (title: string, record: Notification) => (
                <Space direction="vertical" size={0}>
                    <Text strong={!record.read}>{title}</Text>
                    <Text type="secondary">{record.message}</Text>
                </Space>
            ),
        },
        {
            title: t('table.created_at'),
            dataIndex: 'created_at',
            key: 'created_at',
            width: 180,
            render: (createdAt: string) => dayjs(createdAt).format('YYYY-MM-DD HH:mm:ss'),
        },
        {
            title: t('table.actions'),
            key: 'actions',
            width: 120,
            render: (_, record) => (
                <Button
                    size="small"
                    disabled={record.read}
                    loading={notifications.markReadPending}
                    onClick={() => notifications.markRead(record.id)}
                >
                    {t('notification.markRead')}
                </Button>
            ),
        },
    ];

    const listItems = notifications.data?.items ?? [];

    return (
        <div>
            {notifications.messageContextHolder}
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 24 }}>
                <div>
                    <Title level={4} style={{ margin: 0 }}>{t('notification.title')}</Title>
                    <Text type="secondary">{t('notification.subtitle')}</Text>
                </div>
                <Space>
                    <Badge count={notifications.unreadCount} showZero color="#1677ff" />
                    <Segmented
                        value={notifications.unreadOnly ? 'unread' : 'all'}
                        options={[
                            { value: 'all', label: t('notification.filter_all') },
                            { value: 'unread', label: t('notification.filter_unread') },
                        ]}
                        onChange={(value) => {
                            notifications.setUnreadOnly(value === 'unread');
                            notifications.setPage(1);
                        }}
                    />
                    <Button
                        icon={<ReloadOutlined />}
                        onClick={() => notifications.refetch()}
                    >
                        {t('button.refresh')}
                    </Button>
                    <Button
                        type="primary"
                        onClick={notifications.markAllRead}
                        loading={notifications.markAllReadPending}
                    >
                        {t('notification.markAllRead')}
                    </Button>
                </Space>
            </div>

            <Card style={{ borderRadius: 12 }} styles={{ body: { padding: 0 } }}>
                {listItems.length === 0 && !notifications.isLoading ? (
                    <div style={{ padding: 48 }}>
                        <Empty description={t('notification.empty')} />
                    </div>
                ) : (
                    <Table<Notification>
                        rowKey="id"
                        columns={columns}
                        dataSource={listItems}
                        loading={notifications.isLoading}
                        pagination={{
                            current: notifications.page,
                            pageSize: notifications.pageSize,
                            total: notifications.data?.pagination?.total ?? 0,
                            showTotal: (total) => t('table.total', { total }),
                            onChange: (page, pageSize) => {
                                notifications.setPage(page);
                                notifications.setPageSize(pageSize);
                            },
                        }}
                    />
                )}
            </Card>
        </div>
    );
}
