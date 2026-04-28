'use client';

import {
    Badge,
    Button,
    Collapse,
    Pagination,
    Select,
    Segmented,
    Space,
    Table,
    Tag,
    Typography,
} from 'antd';
import type { ColumnsType } from 'antd/es/table';
import { CheckOutlined, ReloadOutlined } from '@ant-design/icons';
import { useTranslation } from 'react-i18next';
import { useMemo, useState } from 'react';

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
import { LocalDateTimeText } from '@/components/ui/LocalDateTimeText';
import { PageSearchToolbar, filterOptionByLabel } from '@/components/ui/PageSearchToolbar';
import { getNotificationDisplay } from '@/lib/notifications/display';
import { useNotificationsController } from '../hooks/useNotificationsController';
import type { Notification } from '../types';

const { Text } = Typography;

const typeConfig: Record<string, { color: string; icon: React.ReactNode; labelKey: string }> = {
    APPROVAL_PENDING: {
        color: 'orange',
        icon: <QueueReviewGlyph className="notification-type-cell__icon" />,
        labelKey: 'notification.type.approval_pending',
    },
    APPROVAL_COMPLETED: {
        color: 'green',
        icon: <RequestsOverviewGlyph className="notification-type-cell__icon" />,
        labelKey: 'notification.type.approval_completed',
    },
    APPROVAL_REJECTED: {
        color: 'red',
        icon: <DecisionRejectedGlyph className="notification-type-cell__icon" />,
        labelKey: 'notification.type.approval_rejected',
    },
    VM_STATUS_CHANGE: {
        color: 'blue',
        icon: <VirtualMachinesOverviewGlyph className="notification-type-cell__icon" />,
        labelKey: 'notification.type.vm_status_change',
    },
};

export function NotificationsContent() {
    const { t } = useTranslation('common');
    const notifications = useNotificationsController({ t });
    const [quickSearch, setQuickSearch] = useState('');
    const [quickSearchDraft, setQuickSearchDraft] = useState('');
    const [filtersOpen, setFiltersOpen] = useState(false);
    const [typeFilter, setTypeFilter] = useState<'all' | Notification['type']>('all');
    const [typeFilterDraft, setTypeFilterDraft] = useState<'all' | Notification['type']>('all');
    const normalizedQuickSearch = quickSearch.trim().toLowerCase();

    const columns: ColumnsType<Notification> = [
        {
            title: t('table.status'),
            dataIndex: 'read',
            key: 'read',
            width: 104,
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
            width: 180,
            render: (type: Notification['type']) => {
                const cfg = typeConfig[type] ?? typeConfig.APPROVAL_PENDING;
                return (
                    <Space className="notification-type-cell">
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
            width: 360,
            render: (_, record: Notification) => {
                const display = getNotificationDisplay(record, t);
                return (
                    <Space direction="vertical" size={2} className="notification-title-cell">
                        <Text strong={!record.read} className="notification-title-cell__title">{display.title}</Text>
                        <Text type="secondary" className="notification-title-cell__message">{display.message}</Text>
                    </Space>
                );
            },
        },
        {
            title: t('table.created_at'),
            dataIndex: 'created_at',
            key: 'created_at',
            width: 160,
            render: (createdAt: string) => (
                <Text className="workbench-table-date">
                    <LocalDateTimeText value={createdAt} />
                </Text>
            ),
        },
        {
            title: t('table.actions'),
            key: 'actions',
            width: 132,
            render: (_, record) => (
                <Button
                    size="small"
                    icon={<CheckOutlined aria-hidden="true" />}
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

    const filteredItems = useMemo(
        () =>
            (notifications.data?.items ?? []).filter((item) => {
                const display = getNotificationDisplay(item, t);
                const matchesSearch =
                    !normalizedQuickSearch ||
                    [
                        display.title,
                        display.message,
                        t(typeConfig[item.type]?.labelKey ?? 'notification.type.approval_pending'),
                    ]
                        .join(' ')
                        .toLowerCase()
                        .includes(normalizedQuickSearch);
                const matchesType = typeFilter === 'all' || item.type === typeFilter;
                return matchesSearch && matchesType;
            }),
        [normalizedQuickSearch, notifications.data?.items, t, typeFilter],
    );
    const unreadItems = filteredItems.filter((item) => !item.read);
    const readItems = filteredItems.filter((item) => item.read);
    const primaryTableItems = notifications.unreadOnly ? filteredItems : unreadItems;
    const pendingVisible = filteredItems.filter((item) => item.type === 'APPROVAL_PENDING').length;
    const resolvedVisible = filteredItems.filter((item) => item.type === 'APPROVAL_COMPLETED' || item.type === 'APPROVAL_REJECTED').length;
    const vmEventsVisible = filteredItems.filter((item) => item.type === 'VM_STATUS_CHANGE').length;
    const hasActiveFilters = quickSearch.trim().length > 0 || typeFilter !== 'all';
    const paginationTotal = hasActiveFilters
        ? filteredItems.length
        : notifications.data?.pagination?.total ?? filteredItems.length;
    const onPageChange = (page: number, pageSize: number) => {
        notifications.setPage(page);
        notifications.setPageSize(pageSize);
    };
    const applyFilters = (nextSearch = quickSearchDraft) => {
        setQuickSearch(nextSearch);
        setTypeFilter(typeFilterDraft);
        notifications.setPage(1);
    };

    return (
        <div className="notifications-page">
            {notifications.messageContextHolder}
            <PageHeader
                title={t('notification.title')}
                subtitle={t('notification.subtitle')}
                actions={(
                    <Space wrap className="notifications-page__header-actions">
                        <Badge count={notifications.unreadCount} showZero color="#1677ff" />
                        <Button
                            icon={<ReloadOutlined aria-hidden="true" />}
                            onClick={() => notifications.refetch()}
                        >
                            {t('common:button.refresh')}
                        </Button>
                        <Button
                            type="primary"
                            icon={<CheckOutlined aria-hidden="true" />}
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

            <PageSurface className="notifications-page__workspace-surface" flush={true}>
                <div className="notifications-page__search-stack">
                    <PageSearchToolbar
                        searchValue={quickSearch}
                        searchDraftValue={quickSearchDraft}
                        onSearchDraftChange={setQuickSearchDraft}
                        onSearchChange={(value) => {
                            setQuickSearchDraft(value);
                            setQuickSearch(value);
                            notifications.setPage(1);
                        }}
                        searchPlaceholder={t('notification.search_placeholder', 'Search notifications by title, message, or type')}
                        searchTestId="notifications-quick-search"
                        searchHelp={t('notification.search_help', 'Quick search filters notifications visible in the current page.')}
                        secondaryActions={(
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
                        )}
                        advancedSearch={{
                            open: filtersOpen,
                            onToggle: () => setFiltersOpen((current) => !current),
                            openLabel: t('search.advanced', 'Advanced search'),
                            closeLabel: t('search.hide_advanced', 'Hide advanced search'),
                            title: t('search.advanced', 'Advanced search'),
                            content: (
                                <Space direction="vertical" size={12} className="notifications-advanced-search">
                                    <Text type="secondary" className="notifications-advanced-search__help">
                                        {t('notification.advanced_search_help', 'Select exact notification filters here. Options support keyword matching, but the applied filter remains an exact value.')}
                                    </Text>
                                    <Space wrap size={[12, 12]} align="end" className="notifications-advanced-search__controls">
                                        <Select
                                            className="notifications-advanced-search__type"
                                            placeholder={t('notification.type')}
                                            showSearch
                                            filterOption={filterOptionByLabel}
                                            optionFilterProp="label"
                                            value={typeFilterDraft}
                                            onChange={(value) => {
                                                setTypeFilterDraft(value);
                                            }}
                                            options={[
                                                { value: 'all', label: t('filter.all', 'All') },
                                                { value: 'APPROVAL_PENDING', label: t('notification.type.approval_pending') },
                                                { value: 'APPROVAL_COMPLETED', label: t('notification.type.approval_completed') },
                                                { value: 'APPROVAL_REJECTED', label: t('notification.type.approval_rejected') },
                                                { value: 'VM_STATUS_CHANGE', label: t('notification.type.vm_status_change') },
                                            ]}
                                        />
                                        <Button
                                            type="primary"
                                            data-testid="notifications-advanced-search-submit"
                                            onClick={() => applyFilters()}
                                        >
                                            {t('common:button.search')}
                                        </Button>
                                    </Space>
                                </Space>
                            ),
                        }}
                        hasActiveFilters={hasActiveFilters}
                        onClear={() => {
                            setQuickSearch('');
                            setQuickSearchDraft('');
                            setTypeFilter('all');
                            setTypeFilterDraft('all');
                            setFiltersOpen(false);
                            notifications.setPage(1);
                        }}
                        clearLabel={t('common:button.clear_filters', { defaultValue: 'Clear filters' })}
                    />
                </div>
                {filteredItems.length === 0 && !notifications.isLoading ? (
                    <div className="notifications-page__empty">
                        <ActionEmptyState
                            title={hasActiveFilters ? t('notification.empty_filtered', 'No notifications match the current filters') : t('notification.empty')}
                            description={hasActiveFilters ? t('notification.empty_filtered_description', 'Try a broader search or clear the current filters.') : t('notification.empty_description', 'Approval decisions and VM lifecycle changes will appear here.')}
                            visual={<NotificationInboxGlyph className="action-empty-state__art" />}
                        />
                    </div>
                ) : (
                    <>
                        {primaryTableItems.length > 0 || notifications.isLoading ? (
                            <Table<Notification>
                                rowKey="id"
                                columns={columns}
                                dataSource={primaryTableItems}
                                loading={notifications.isLoading}
                                scroll={{ x: 960 }}
                                pagination={notifications.unreadOnly
                                    ? {
                                        current: notifications.page,
                                        pageSize: notifications.pageSize,
                                        total: paginationTotal,
                                        showTotal: (total) => t('table.total', { total }),
                                        onChange: onPageChange,
                                    }
                                    : false}
                            />
                        ) : null}
                        {!notifications.unreadOnly && readItems.length > 0 ? (
                            <Collapse
                                className="notifications-read-collapse"
                                ghost={false}
                                items={[
                                    {
                                        key: 'read',
                                        label: t('notification.read_collapsed', {
                                            count: readItems.length,
                                            defaultValue: '{{count}} read notifications',
                                        }),
                                        children: (
                                            <Table<Notification>
                                                rowKey="id"
                                                columns={columns}
                                                dataSource={readItems}
                                                loading={notifications.isLoading}
                                                pagination={false}
                                                scroll={{ x: 960 }}
                                            />
                                        ),
                                    },
                                ]}
                            />
                        ) : null}
                        {!notifications.unreadOnly && filteredItems.length > 0 ? (
                            <div className="notifications-page__pagination">
                                <Pagination
                                    current={notifications.page}
                                    pageSize={notifications.pageSize}
                                    total={paginationTotal}
                                    showTotal={(total) => t('table.total', { total })}
                                    onChange={onPageChange}
                                />
                            </div>
                        ) : null}
                    </>
                )}
            </PageSurface>
        </div>
    );
}
