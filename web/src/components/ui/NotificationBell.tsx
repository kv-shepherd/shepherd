'use client';

/**
 * NotificationBell — Header notification indicator with dropdown.
 *
 * AGENTS.md §2.1: Direct imports (antd is in optimizePackageImports).
 * ADR-0015 §20: Platform Inbox notification display.
 * master-flow.md Stage 5.F: API endpoints for List, UnreadCount, MarkRead, MarkAllRead.
 *
 * Features:
 * - Badge with unread count (auto-refreshes via polling).
 * - Dropdown list of recent notifications.
 * - Click to mark as read + navigate to related resource.
 * - "Mark all read" action.
 */
import React, { useState, useCallback } from 'react';
import {
    Badge,
    Popover,
    List,
    Button,
    Typography,
    Space,
    Tag,
    Empty,
    Spin,
    Tooltip,
} from 'antd';
import {
    BellOutlined,
    CheckOutlined,
    CheckCircleOutlined,
    CloseCircleOutlined,
    ClockCircleOutlined,
    DesktopOutlined,
} from '@ant-design/icons';
import { useRouter } from 'next/navigation';
import { useTranslation } from 'react-i18next';
import { useApiGet, useApiAction } from '@/hooks/useApiQuery';
import type { components } from '@/types/api.gen';
import { api } from '@/lib/api/client';

const { Text, Paragraph } = Typography;

type Notification = components['schemas']['Notification'];
type NotificationType = Notification['type'];

/** Notification type → UI config mapping */
const typeConfig: Record<NotificationType, { color: string; icon: React.ReactNode; label: string }> = {
    APPROVAL_PENDING: {
        color: 'orange',
        icon: <ClockCircleOutlined />,
        label: 'Pending Approval',
    },
    APPROVAL_COMPLETED: {
        color: 'green',
        icon: <CheckCircleOutlined />,
        label: 'Approved',
    },
    APPROVAL_REJECTED: {
        color: 'red',
        icon: <CloseCircleOutlined />,
        label: 'Rejected',
    },
    VM_STATUS_CHANGE: {
        color: 'blue',
        icon: <DesktopOutlined />,
        label: 'VM Status',
    },
};

/** Relative time formatter */
function formatRelativeTime(dateStr: string): string {
    const now = Date.now();
    const date = new Date(dateStr).getTime();
    const diff = now - date;
    const minutes = Math.floor(diff / 60000);
    if (minutes < 1) return 'just now';
    if (minutes < 60) return `${minutes}m ago`;
    const hours = Math.floor(minutes / 60);
    if (hours < 24) return `${hours}h ago`;
    const days = Math.floor(hours / 24);
    return `${days}d ago`;
}

export default function NotificationBell() {
    const router = useRouter();
    const { t } = useTranslation('common');
    const [open, setOpen] = useState(false);

    // Fetch unread count (poll every 30s).
    const { data: unreadData } = useApiGet(
        ['notifications', 'unread-count'],
        () => api.GET('/notifications/unread-count'),
        { refetchInterval: 30_000 }
    );

    // Fetch recent notifications when popover is open.
    const { data: listData, isLoading: listLoading } = useApiGet(
        ['notifications', 'list'],
        () => api.GET('/notifications', { params: { query: { per_page: 10 } } }),
        { enabled: open }
    );

    // Mark single notification as read.
    const markRead = useApiAction<string>(
        (notificationId: string) =>
            api.PATCH('/notifications/{notification_id}/read', {
                params: { path: { notification_id: notificationId } },
            }),
        {
            invalidateKeys: [['notifications', 'unread-count'], ['notifications', 'list']],
        }
    );

    // Mark all as read.
    const markAllRead = useApiAction(
        () => api.POST('/notifications/mark-all-read'),
        {
            invalidateKeys: [['notifications', 'unread-count'], ['notifications', 'list']],
        }
    );

    const handleNotificationClick = useCallback(
        (notification: Notification) => {
            // Mark as read if unread.
            if (!notification.read) {
                markRead.mutate(notification.id);
            }

            // Navigate to resource.
            if (notification.resource_type === 'approval_ticket' && notification.resource_id) {
                router.push('/admin/approvals');
            } else if (notification.resource_type === 'vm' && notification.resource_id) {
                router.push('/vms');
            }

            setOpen(false);
        },
        [markRead, router]
    );

    const unreadCount = unreadData?.count ?? 0;
    const notifications = listData?.items ?? [];

    const content = (
        <div style={{ width: 380, maxHeight: 460 }}>
            {/* Header */}
            <div
                style={{
                    display: 'flex',
                    justifyContent: 'space-between',
                    alignItems: 'center',
                    padding: '8px 12px',
                    borderBottom: '1px solid #f0f0f0',
                }}
            >
                <Text strong style={{ fontSize: 15 }}>
                    {t('notification.title', 'Notifications')}
                </Text>
                {unreadCount > 0 && (
                    <Button
                        type="link"
                        size="small"
                        icon={<CheckOutlined />}
                        onClick={() => markAllRead.mutate()}
                        loading={markAllRead.isPending}
                    >
                        {t('notification.markAllRead', 'Mark all read')}
                    </Button>
                )}
            </div>

            {/* Notification List */}
            {listLoading ? (
                <div style={{ textAlign: 'center', padding: 40 }}>
                    <Spin size="small" />
                </div>
            ) : notifications.length === 0 ? (
                <Empty
                    image={Empty.PRESENTED_IMAGE_SIMPLE}
                    description={t('notification.empty', 'No notifications')}
                    style={{ padding: '24px 0' }}
                />
            ) : (
                <List
                    dataSource={notifications}
                    renderItem={(item: Notification) => {
                        const config = typeConfig[item.type];
                        return (
                            <List.Item
                                onClick={() => handleNotificationClick(item)}
                                style={{
                                    cursor: 'pointer',
                                    padding: '10px 12px',
                                    backgroundColor: item.read ? 'transparent' : '#f6ffed',
                                    transition: 'background-color 0.2s',
                                }}
                                onMouseEnter={(e) => {
                                    (e.currentTarget as HTMLElement).style.backgroundColor = '#fafafa';
                                }}
                                onMouseLeave={(e) => {
                                    (e.currentTarget as HTMLElement).style.backgroundColor = item.read
                                        ? 'transparent'
                                        : '#f6ffed';
                                }}
                            >
                                <List.Item.Meta
                                    avatar={
                                        <div
                                            style={{
                                                width: 32,
                                                height: 32,
                                                borderRadius: '50%',
                                                backgroundColor: `${config.color}15`,
                                                display: 'flex',
                                                alignItems: 'center',
                                                justifyContent: 'center',
                                                fontSize: 16,
                                                color: config.color,
                                            }}
                                        >
                                            {config.icon}
                                        </div>
                                    }
                                    title={
                                        <Space size={4}>
                                            <Text
                                                strong={!item.read}
                                                ellipsis
                                                style={{ maxWidth: 220, fontSize: 13 }}
                                            >
                                                {item.title}
                                            </Text>
                                            <Tag
                                                color={config.color}
                                                style={{ fontSize: 10, lineHeight: '16px', padding: '0 4px' }}
                                            >
                                                {config.label}
                                            </Tag>
                                        </Space>
                                    }
                                    description={
                                        <div>
                                            <Paragraph
                                                type="secondary"
                                                ellipsis={{ rows: 1 }}
                                                style={{ marginBottom: 2, fontSize: 12 }}
                                            >
                                                {item.message}
                                            </Paragraph>
                                            <Text type="secondary" style={{ fontSize: 11 }}>
                                                {formatRelativeTime(item.created_at)}
                                            </Text>
                                        </div>
                                    }
                                />
                                {!item.read && (
                                    <div
                                        style={{
                                            width: 8,
                                            height: 8,
                                            borderRadius: '50%',
                                            backgroundColor: '#1677ff',
                                            flexShrink: 0,
                                        }}
                                    />
                                )}
                            </List.Item>
                        );
                    }}
                />
            )}
        </div>
    );

    return (
        <Popover
            content={content}
            trigger="click"
            open={open}
            onOpenChange={setOpen}
            placement="bottomRight"
            arrow={false}
            styles={{ body: { padding: 0 } }}
        >
            <Tooltip title={t('notification.title', 'Notifications')}>
                <Badge count={unreadCount} size="small" offset={[-2, 4]}>
                    <BellOutlined
                        style={{
                            fontSize: 18,
                            cursor: 'pointer',
                            padding: '4px 8px',
                            color: '#595959',
                        }}
                    />
                </Badge>
            </Tooltip>
        </Popover>
    );
}
