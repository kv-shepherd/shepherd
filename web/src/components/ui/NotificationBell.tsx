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
import type { TFunction } from 'i18next';
import { useApiGet, useApiAction } from '@/hooks/useApiQuery';
import type { components } from '@/types/api.gen';
import { api } from '@/lib/api/client';
import { getNotificationDisplay } from '@/lib/notifications/display';

const { Text, Paragraph } = Typography;

type Notification = components['schemas']['Notification'];
type NotificationType = Notification['type'];

/** Notification type → UI config mapping */
const typeConfig: Record<NotificationType, {
    accent: string;
    surface: string;
    tagColor: string;
    icon: React.ReactNode;
    label: string;
}> = {
    APPROVAL_PENDING: {
        accent: '#D97706',
        surface: '#FFF4E5',
        tagColor: 'orange',
        icon: <ClockCircleOutlined aria-hidden="true" />,
        label: 'notification.type.approval_pending',
    },
    APPROVAL_COMPLETED: {
        accent: '#0F8F57',
        surface: '#E8FFF2',
        tagColor: 'green',
        icon: <CheckCircleOutlined aria-hidden="true" />,
        label: 'notification.type.approval_completed',
    },
    APPROVAL_REJECTED: {
        accent: '#DC2626',
        surface: '#FEF2F2',
        tagColor: 'red',
        icon: <CloseCircleOutlined aria-hidden="true" />,
        label: 'notification.type.approval_rejected',
    },
    VM_STATUS_CHANGE: {
        accent: '#2563EB',
        surface: '#EFF6FF',
        tagColor: 'blue',
        icon: <DesktopOutlined aria-hidden="true" />,
        label: 'notification.type.vm_status_change',
    },
};

const NOTIFICATION_BELL_PREVIEW_LIMIT = 5;
const NOTIFICATION_BELL_FETCH_LIMIT = NOTIFICATION_BELL_PREVIEW_LIMIT + 1;

/** Relative time formatter */
function formatRelativeTime(dateStr: string, t: TFunction<'common'>): string {
    const now = Date.now();
    const date = new Date(dateStr).getTime();
    const diff = now - date;
    const minutes = Math.floor(diff / 60000);
    if (minutes < 1) return t('notification.time.just_now', { defaultValue: 'just now' });
    if (minutes < 60) return t('notification.time.minutes_ago', { count: minutes, defaultValue: `${minutes}m ago` });
    const hours = Math.floor(minutes / 60);
    if (hours < 24) return t('notification.time.hours_ago', { count: hours, defaultValue: `${hours}h ago` });
    const days = Math.floor(hours / 24);
    return t('notification.time.days_ago', { count: days, defaultValue: `${days}d ago` });
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
        ['notifications', 'list', NOTIFICATION_BELL_FETCH_LIMIT],
        () => api.GET('/notifications', { params: { query: { per_page: NOTIFICATION_BELL_FETCH_LIMIT } } }),
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
            if (notification.resource_type === 'ticket' && notification.resource_id) {
                router.push('/admin/approval-tasks');
            } else if (notification.resource_type === 'vm' && notification.resource_id) {
                router.push('/vms');
            }

            setOpen(false);
        },
        [markRead, router]
    );

    const unreadCount = unreadData?.count ?? 0;
    const notifications = listData?.items ?? [];
    const previewNotifications = notifications.slice(0, NOTIFICATION_BELL_PREVIEW_LIMIT);
    const totalNotifications = listData?.pagination?.total ?? notifications.length;
    const hiddenNotificationCount = Math.max(totalNotifications - previewNotifications.length, 0);

    const content = (
        <div className="notification-bell-popover">
            {/* Header */}
            <div className="notification-bell-popover__header">
                <Text strong className="notification-bell-popover__title">
                    {t('notification.title', 'Notifications')}
                </Text>
                {unreadCount > 0 && (
                    <Button
                        type="link"
                        size="small"
                        icon={<CheckOutlined aria-hidden="true" />}
                        onClick={() => markAllRead.mutate()}
                        loading={markAllRead.isPending}
                    >
                        {t('notification.markAllRead', 'Mark all read')}
                    </Button>
                )}
            </div>

            {/* Notification List */}
            {listLoading ? (
                <div className="notification-bell-popover__loading">
                    <Spin size="small" />
                </div>
            ) : notifications.length === 0 ? (
                <Empty
                    image={Empty.PRESENTED_IMAGE_SIMPLE}
                    description={t('notification.empty', 'No notifications')}
                    className="notification-bell-popover__empty"
                />
            ) : (
                <List
                    className="notification-bell-popover__list"
                    dataSource={previewNotifications}
                    renderItem={(item: Notification) => {
                        const config = typeConfig[item.type];
                        const display = getNotificationDisplay(item, t);
                        return (
                            <List.Item
                                onClick={() => handleNotificationClick(item)}
                                className={item.read
                                    ? 'notification-bell-popover__item'
                                    : 'notification-bell-popover__item notification-bell-popover__item--unread'}
                            >
                                <List.Item.Meta
                                    avatar={
                                        <div
                                            className="notification-bell-popover__avatar"
                                            style={{
                                                '--notification-bell-accent': config.accent,
                                                '--notification-bell-surface': config.surface,
                                            } as React.CSSProperties}
                                        >
                                            {config.icon}
                                        </div>
                                    }
                                    title={
                                        <Space size={4} wrap className="notification-bell-popover__item-title">
                                            <Text
                                                strong={!item.read}
                                                ellipsis
                                                className="notification-bell-popover__item-heading"
                                            >
                                                {display.title}
                                            </Text>
                                            <Tag
                                                color={config.tagColor}
                                                className="notification-bell-popover__tag"
                                            >
                                                {t(config.label, { defaultValue: config.label })}
                                            </Tag>
                                        </Space>
                                    }
                                    description={
                                        <div>
                                            <Paragraph
                                                type="secondary"
                                                ellipsis={{ rows: 1 }}
                                                className="notification-bell-popover__message"
                                            >
                                                {display.message}
                                            </Paragraph>
                                            <Text type="secondary" className="notification-bell-popover__time">
                                                {formatRelativeTime(item.created_at, t)}
                                            </Text>
                                        </div>
                                    }
                                />
                                {!item.read && (
                                    <div className="notification-bell-popover__unread-dot" />
                                )}
                            </List.Item>
                        );
                    }}
                />
            )}

            <div className="notification-bell-popover__footer">
                {hiddenNotificationCount > 0 ? (
                    <Text type="secondary" className="notification-bell-popover__more">
                        {t('notification.moreInInbox', {
                            count: hiddenNotificationCount,
                            defaultValue: `${hiddenNotificationCount} more in inbox`,
                        })}
                    </Text>
                ) : <span />}
                <Button
                    type="link"
                    size="small"
                    data-testid="notification-view-all"
                    onClick={() => {
                        setOpen(false);
                        router.push('/notifications');
                    }}
                >
                    {t('notification.viewAll', 'View all')}
                </Button>
            </div>
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
            <Tooltip title={t('notification.title', 'Notifications')} open={open ? false : undefined}>
                <Badge count={unreadCount} size="small" offset={[-4, 6]}>
                    <Button
                        type="text"
                        className="app-shell-icon-action app-shell-notification-trigger"
                        data-testid="notification-bell-trigger"
                        aria-label={t('notification.title', 'Notifications')}
                        icon={<BellOutlined aria-hidden="true" />}
                    />
                </Badge>
            </Tooltip>
        </Popover>
    );
}
