'use client';

/**
 * /admin/rate-limits — Platform-wide rate limiting status and configuration.
 * master-flow.md §10: Rate Limit management.
 *
 * API contracts:
 *   GET  /admin/rate-limits/status             → RateLimitStatusList
 *   GET  /admin/rate-limits/exemptions         → RateLimitExemptionList
 *   POST /admin/rate-limits/exemptions         → RateLimitExemption
 *   DELETE /admin/rate-limits/exemptions/{id}  → 204
 *
 * E2E data-testid requirements:
 *   rate-limit-status-page
 */
import { ReloadOutlined } from '@ant-design/icons';
import { Alert, Button, Space, Table, Tag, Typography } from 'antd';
import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';

import { ActionEmptyState } from '@/components/feedback/ActionEmptyState';
import { SummaryMetricCard } from '@/components/feedback/SummaryMetricCard';
import {
    HealthOverviewGlyph,
    NotificationInboxGlyph,
    QueueReviewGlyph,
    RequestsOverviewGlyph,
} from '@/components/illustrations/DashboardIllustrations';
import { PageHeader, PageSurface } from '@/components/layouts/PageSection';
import { LocalDateTimeText } from '@/components/ui/LocalDateTimeText';
import { api } from '@/lib/api/client';
import { translateApiError } from '@/lib/api/errorMessage';
import { useApiGet } from '@/lib/api/useApiGet';
import type { components } from '@/types/api.gen';

const { Text } = Typography;

type RateLimitStatus = components['schemas']['RateLimitUserStatus'];
type RateLimitStatusList = components['schemas']['RateLimitStatusList'];
type RateLimitExemption = components['schemas']['RateLimitExemption'];
type RateLimitExemptionList = components['schemas']['RateLimitExemptionList'];

function renderUserIdentity(
    t: (key: string, options?: Record<string, unknown>) => string,
    userId: string,
    displayName?: string,
    username?: string,
    email?: string,
) {
    const primary = displayName?.trim() || username?.trim() || userId;
    const secondary = username?.trim() && username !== primary ? username : userId;

    return (
        <Space direction="vertical" size={0}>
            <Text strong>{primary}</Text>
            <Text type="secondary" style={{ fontSize: 12 }}>{secondary}</Text>
            <Text type="secondary" style={{ fontSize: 12 }}>
                {email?.trim() || t('users.directory.no_email', { defaultValue: 'No contact email' })}
            </Text>
        </Space>
    );
}

export default function AdminRateLimitsPageContent() {
    const { t } = useTranslation(['admin', 'common']);

    const { data, isLoading, error: statusError, refetch } = useApiGet<RateLimitStatusList>(
        ['admin-rate-limits-status'],
        () => api.GET('/admin/rate-limits/status', {}) as Promise<{ data?: RateLimitStatusList; error?: unknown; response?: Response }>
    );

    const {
        data: exemptionData,
        isLoading: exemptionsLoading,
        error: exemptionsError,
        refetch: refetchExemptions,
    } = useApiGet<RateLimitExemptionList>(
        ['admin-rate-limits-exemptions'],
        () => api.GET('/admin/rate-limits/exemptions', { params: { query: { page: 1, per_page: 100 } } }) as Promise<{ data?: RateLimitExemptionList; error?: unknown; response?: Response }>
    );

    const loadError = statusError ?? exemptionsError;
    const statusItems = useMemo(() => data?.items ?? [], [data?.items]);
    const exemptionItems = useMemo(() => exemptionData?.items ?? [], [exemptionData?.items]);
    const rateLimitSummary = useMemo(() => {
        const trackedUsers = new Set(statusItems.map((item) => item.user_id).filter(Boolean)).size;
        const coolingDownUsers = statusItems.filter((item) => (item.cooldown_remaining_seconds ?? 0) > 0).length;
        const exemptedUsers = statusItems.filter((item) => item.exempted).length;
        return {
            trackedUsers,
            coolingDownUsers,
            exemptedUsers,
            exemptionsTotal: exemptionItems.length,
        };
    }, [exemptionItems.length, statusItems]);

    const columns = [
        {
            title: t('rate_limits.table.user', { defaultValue: 'User' }),
            dataIndex: 'user_id',
            key: 'user_id',
            render: (_: string, record: RateLimitStatus) =>
                renderUserIdentity(t, record.user_id, record.display_name, record.username, record.email),
        },
        {
            title: t('rate_limits.table.pending_work', { defaultValue: 'Pending work' }),
            key: 'pending',
            render: (_: unknown, record: RateLimitStatus) => (
                <Space direction="vertical" size={4}>
                    <Tag color={(record.current_pending_parents ?? 0) >= (record.effective_max_pending_parents ?? 0) ? 'red' : 'blue'}>
                        {t('rate_limits.table.pending_parents', {
                            defaultValue: 'Parent requests: {{count}} / {{limit}}',
                            count: record.current_pending_parents ?? 0,
                            limit: record.effective_max_pending_parents ?? 0,
                        })}
                    </Tag>
                    <Tag color={(record.current_pending_children ?? 0) >= (record.effective_max_pending_children ?? 0) ? 'red' : 'purple'}>
                        {t('rate_limits.table.pending_children', {
                            defaultValue: 'Child requests: {{count}} / {{limit}}',
                            count: record.current_pending_children ?? 0,
                            limit: record.effective_max_pending_children ?? 0,
                        })}
                    </Tag>
                </Space>
            ),
        },
        {
            title: t('rate_limits.table.policy', { defaultValue: 'Effective policy' }),
            key: 'policy',
            render: (_: unknown, record: RateLimitStatus) => (
                <Space direction="vertical" size={0}>
                    <Text>{t('rate_limits.table.policy_parents', { defaultValue: 'Parent queue limit: {{count}}', count: record.effective_max_pending_parents ?? 0 })}</Text>
                    <Text>{t('rate_limits.table.policy_children', { defaultValue: 'Child queue limit: {{count}}', count: record.effective_max_pending_children ?? 0 })}</Text>
                    <Text type="secondary">
                        {t('rate_limits.table.policy_cooldown', { defaultValue: 'Cooldown: {{seconds}}s', seconds: record.effective_cooldown_seconds ?? 0 })}
                    </Text>
                </Space>
            ),
        },
        {
            title: t('rate_limits.table.cooldown', { defaultValue: 'Cooldown' }),
            dataIndex: 'cooldown_remaining_seconds',
            key: 'cooldown_remaining_seconds',
            width: 180,
            render: (seconds: number) => (
                <Tag color={seconds > 0 ? 'gold' : 'green'}>
                    {seconds > 0
                        ? t('rate_limits.table.cooldown_active', { defaultValue: '{{seconds}}s remaining', seconds })
                        : t('rate_limits.table.cooldown_ready', { defaultValue: 'Ready' })}
                </Tag>
            ),
        },
        {
            title: t('rate_limits.table.exemption', { defaultValue: 'Exemption' }),
            dataIndex: 'exempted',
            key: 'exempted',
            width: 220,
            render: (_: boolean, record: RateLimitStatus) => (
                <Space direction="vertical" size={0}>
                    <Tag color={record.exempted ? 'green' : 'default'}>
                        {record.exempted
                            ? t('rate_limits.table.exempted_yes', { defaultValue: 'Exempted' })
                            : t('rate_limits.table.exempted_no', { defaultValue: 'Standard policy' })}
                    </Tag>
                    {record.exempted && record.exemption_expires_at ? (
                        <Text type="secondary" style={{ fontSize: 12 }}>
                            {t('rate_limits.table.exemption_expires_at', { defaultValue: 'Expires' })}: <LocalDateTimeText value={record.exemption_expires_at} />
                        </Text>
                    ) : null}
                </Space>
            ),
        },
    ];

    const exemptionColumns = [
        {
            title: t('rate_limits.exemptions.user', { defaultValue: 'User' }),
            dataIndex: 'user_id',
            key: 'user_id',
            render: (_: string, record: RateLimitExemption) =>
                renderUserIdentity(t, record.user_id, record.display_name, record.username, record.email),
        },
        {
            title: t('rate_limits.exemptions.updated_by', { defaultValue: 'Updated by' }),
            dataIndex: 'exempted_by',
            key: 'exempted_by',
            width: 160,
        },
        {
            title: t('rate_limits.exemptions.reason', { defaultValue: 'Reason' }),
            dataIndex: 'reason',
            key: 'reason',
            render: (value?: string) => value || '-',
        },
        {
            title: t('rate_limits.exemptions.expires_at', { defaultValue: 'Expires At' }),
            dataIndex: 'expires_at',
            key: 'expires_at',
            render: (value?: string) => <LocalDateTimeText value={value} />,
        },
        {
            title: t('rate_limits.exemptions.created_at', { defaultValue: 'Created At' }),
            dataIndex: 'created_at',
            key: 'created_at',
            render: (value: string) => <LocalDateTimeText value={value} />,
        },
    ];

    return (
        <div data-testid="rate-limit-status-page">
            <PageHeader
                title={t('rate_limits.title', { defaultValue: 'Rate Limits' })}
                subtitle={t('rate_limits.subtitle', {
                    defaultValue: 'Platform-wide request rate limiting status.',
                })}
                actions={(
                    <Space>
                        <Button
                            icon={<ReloadOutlined />}
                            onClick={() => {
                                void refetch()
                                void refetchExemptions()
                            }}
                        >
                            {t('common:button.refresh')}
                        </Button>
                    </Space>
                )}
            />
            <div className="summary-card-grid">
                <SummaryMetricCard
                    title={t('rate_limits.summary.users_title', { defaultValue: 'Tracked users' })}
                    value={rateLimitSummary.trackedUsers}
                    description={t('rate_limits.summary.users_description', { defaultValue: 'Unique identities currently present in the status window.' })}
                    visual={<HealthOverviewGlyph className="summary-metric-card__art" />}
                    accentColor="#1D5BFF"
                    surfaceColor="#E6F4FF"
                />
                <SummaryMetricCard
                    title={t('rate_limits.summary.cooldown_title', { defaultValue: 'Cooling down' })}
                    value={rateLimitSummary.coolingDownUsers}
                    description={t('rate_limits.summary.cooldown_description', { defaultValue: 'Users who still need to wait before opening another batch request.' })}
                    visual={<QueueReviewGlyph className="summary-metric-card__art" />}
                    accentColor="#CF1322"
                    surfaceColor="#FFF1F0"
                />
                <SummaryMetricCard
                    title={t('rate_limits.summary.status_exemptions_title', { defaultValue: 'Exempt status rows' })}
                    value={rateLimitSummary.exemptedUsers}
                    description={t('rate_limits.summary.status_exemptions_description', { defaultValue: 'Rate-limit rows already covered by an exemption flag.' })}
                    visual={<RequestsOverviewGlyph className="summary-metric-card__art" />}
                    accentColor="#0F8F57"
                    surfaceColor="#E8FFF2"
                />
                <SummaryMetricCard
                    title={t('rate_limits.summary.exemptions_title', { defaultValue: 'Exemption records' })}
                    value={rateLimitSummary.exemptionsTotal}
                    description={t('rate_limits.summary.exemptions_description', { defaultValue: 'Explicit exemption entries currently configured.' })}
                    visual={<NotificationInboxGlyph className="summary-metric-card__art" />}
                    accentColor="#6D4DE3"
                    surfaceColor="#F5EDFF"
                />
            </div>

            <PageSurface flush={true}>
                {loadError ? (
                    <Alert
                        type="error"
                        showIcon
                        style={{ margin: 16, marginBottom: 0 }}
                        message={t('common:message.error')}
                        description={translateApiError(t, loadError)}
                    />
                ) : null}
                <Table
                    dataSource={statusItems}
                    columns={columns}
                    rowKey="user_id"
                    loading={isLoading}
                    pagination={false}
                    size="middle"
                    scroll={{ x: 'max-content' }}
                    locale={{
                        emptyText: (
                            <ActionEmptyState
                                compact={true}
                                title={t('rate_limits.empty', { defaultValue: 'No rate limit data' })}
                                description={t('rate_limits.empty_description', { defaultValue: 'Active rate-limit windows will appear here once requests start flowing through the platform.' })}
                                visual={<QueueReviewGlyph className="action-empty-state__art action-empty-state__art--compact" />}
                            />
                        ),
                    }}
                />
            </PageSurface>

            <PageSurface
                style={{ marginTop: 16 }}
                title={t('rate_limits.exemptions.title', { defaultValue: 'Exemptions' })}
                styles={{ body: { padding: 0 } }}
            >
                <Table
                    dataSource={exemptionItems}
                    columns={exemptionColumns}
                    rowKey="user_id"
                    loading={exemptionsLoading}
                    pagination={false}
                    size="middle"
                    scroll={{ x: 'max-content' }}
                    locale={{
                        emptyText: (
                            <ActionEmptyState
                                compact={true}
                                title={t('rate_limits.exemptions.empty', { defaultValue: 'No exemptions configured' })}
                                description={t('rate_limits.exemptions.empty_description', { defaultValue: 'Add exemptions only for well-understood automation or operational break-glass scenarios.' })}
                                visual={<NotificationInboxGlyph className="action-empty-state__art action-empty-state__art--compact" />}
                            />
                        ),
                    }}
                />
            </PageSurface>
        </div>
    );
}
