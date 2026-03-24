'use client';

import {
    Badge,
    Button,
    Segmented,
    Space,
    Table,
    Tag,
    Typography,
} from 'antd';
import type { ColumnsType } from 'antd/es/table';
import { ReloadOutlined } from '@ant-design/icons';
import dayjs from 'dayjs';
import { useTranslation } from 'react-i18next';

import { ActionEmptyState } from '@/components/feedback/ActionEmptyState';
import { SummaryMetricCard } from '@/components/feedback/SummaryMetricCard';
import {
    DecisionRejectedGlyph,
    NotificationInboxGlyph,
    QueueReviewGlyph,
    RequestsOverviewGlyph,
    VirtualMachinesOverviewGlyph,
} from '@/components/illustrations/DashboardIllustrations';
import { PageHeader, PageSurface } from '@/components/layouts/PageSection';
import { useNotificationsController } from '../hooks/useNotificationsController';
import type { Notification } from '../types';

const { Text } = Typography;

const typeConfig: Record<string, { color: string; icon: React.ReactNode; labelKey: string }> = {
    APPROVAL_PENDING: {
        color: 'orange',
        icon: <QueueReviewGlyph style={{ width: 18, height: 18, display: 'block' }} />,
        labelKey: 'notification.type.approval_pending',
    },
    APPROVAL_COMPLETED: {
        color: 'green',
        icon: <RequestsOverviewGlyph style={{ width: 18, height: 18, display: 'block' }} />,
        labelKey: 'notification.type.approval_completed',
    },
    APPROVAL_REJECTED: {
        color: 'red',
        icon: <DecisionRejectedGlyph style={{ width: 18, height: 18, display: 'block' }} />,
        labelKey: 'notification.type.approval_rejected',
    },
    VM_STATUS_CHANGE: {
        color: 'blue',
        icon: <VirtualMachinesOverviewGlyph style={{ width: 18, height: 18, display: 'block' }} />,
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
                    data-testid={`notification-action-read-${record.id}`}
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
    const pendingVisible = listItems.filter((item) => item.type === 'APPROVAL_PENDING').length;
    const resolvedVisible = listItems.filter((item) => item.type === 'APPROVAL_COMPLETED' || item.type === 'APPROVAL_REJECTED').length;
    const vmEventsVisible = listItems.filter((item) => item.type === 'VM_STATUS_CHANGE').length;

    return (
        <div>
            {notifications.messageContextHolder}
            <PageHeader
                title={t('notification.title')}
                subtitle={t('notification.subtitle')}
                actions={(
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
                        data-testid="notifications-mark-all-read-button"
                        onClick={notifications.markAllRead}
                        loading={notifications.markAllReadPending}
                    >
                        {t('notification.markAllRead')}
                    </Button>
                    </Space>
                )}
            />
            <div className="summary-card-grid">
                <SummaryMetricCard
                    title={t('notification.summary.unread_title', 'Unread inbox')}
                    value={notifications.unreadCount}
                    description={t('notification.summary.unread_description', 'Unread items across your notification inbox.')}
                    visual={<NotificationInboxGlyph className="summary-metric-card__art" />}
                    accentColor="#1D5BFF"
                    surfaceColor="#E6F4FF"
                />
                <SummaryMetricCard
                    title={t('notification.summary.pending_title', 'Pending approvals')}
                    value={pendingVisible}
                    description={t('notification.summary.pending_description', 'Approval reminders visible in the current list.')}
                    visual={<QueueReviewGlyph className="summary-metric-card__art" />}
                    accentColor="#D97706"
                    surfaceColor="#FFF4E5"
                />
                <SummaryMetricCard
                    title={t('notification.summary.resolved_title', 'Resolved items')}
                    value={resolvedVisible}
                    description={t('notification.summary.resolved_description', 'Approved and rejected results visible right now.')}
                    visual={<RequestsOverviewGlyph className="summary-metric-card__art" />}
                    accentColor="#0F8F57"
                    surfaceColor="#E8FFF2"
                />
                <SummaryMetricCard
                    title={t('notification.summary.vm_title', 'VM events')}
                    value={vmEventsVisible}
                    description={t('notification.summary.vm_description', 'Lifecycle updates visible in the current page.')}
                    visual={<VirtualMachinesOverviewGlyph className="summary-metric-card__art" />}
                    accentColor="#6D4DE3"
                    surfaceColor="#F5EDFF"
                />
            </div>

            <PageSurface flush={true}>
                {listItems.length === 0 && !notifications.isLoading ? (
                    <div style={{ padding: 48 }}>
                        <ActionEmptyState
                            title={t('notification.empty')}
                            description={t('notification.empty_description', 'Approval decisions and VM lifecycle changes will appear here.')}
                            visual={<NotificationInboxGlyph className="action-empty-state__art" />}
                        />
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
            </PageSurface>
        </div>
    );
}
