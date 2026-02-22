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
import { Button, Card, Space, Table, Tag, Typography } from 'antd';
import { ThunderboltOutlined, ReloadOutlined } from '@ant-design/icons';
import dayjs from 'dayjs';
import { useTranslation } from 'react-i18next';

import { useApiGet } from '@/lib/api/useApiGet';
import { api } from '@/lib/api/client';

const { Title, Text } = Typography;

interface RateLimitStatus {
    user_id: string;
    operation: string;
    count: number;
    window_seconds: number;
    limit: number;
    is_exempted: boolean;
}

interface RateLimitStatusList {
    items: RateLimitStatus[];
    pagination?: { total: number; page: number; per_page: number };
}

interface RateLimitExemption {
    user_id: string;
    exempted_by: string;
    reason?: string;
    expires_at?: string;
    created_at: string;
}

interface RateLimitExemptionList {
    items: RateLimitExemption[];
    pagination?: { total: number; page: number; per_page: number };
}

export default function AdminRateLimitsPage() {
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

    const columns = [
        {
            title: 'User',
            dataIndex: 'user_id',
            key: 'user_id',
            render: (id: string) => <Text code>{id}</Text>,
        },
        {
            title: 'Operation',
            dataIndex: 'operation',
            key: 'operation',
            render: (op: string) => <Tag color="purple">{op}</Tag>,
        },
        {
            title: 'Count / Limit',
            key: 'count',
            render: (_: unknown, record: RateLimitStatus) => (
                <Text type={record.count >= record.limit ? 'danger' : 'secondary'}>
                    {record.count} / {record.limit}
                </Text>
            ),
        },
        {
            title: 'Window (s)',
            dataIndex: 'window_seconds',
            key: 'window_seconds',
            width: 110,
        },
        {
            title: 'Exempted',
            dataIndex: 'is_exempted',
            key: 'is_exempted',
            width: 100,
            render: (exempted: boolean) => (
                <Tag color={exempted ? 'green' : 'default'}>{exempted ? 'Yes' : 'No'}</Tag>
            ),
        },
    ];

    const exemptionColumns = [
        {
            title: t('rate_limits.exemptions.user', { defaultValue: 'User' }),
            dataIndex: 'user_id',
            key: 'user_id',
            render: (id: string) => <Text code>{id}</Text>,
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
            render: (value?: string) => (value ? dayjs(value).format('YYYY-MM-DD HH:mm') : '-'),
        },
        {
            title: t('rate_limits.exemptions.created_at', { defaultValue: 'Created At' }),
            dataIndex: 'created_at',
            key: 'created_at',
            render: (value: string) => dayjs(value).format('YYYY-MM-DD HH:mm'),
        },
    ];

    return (
        <div data-testid="rate-limit-status-page">
            <div
                style={{
                    display: 'flex',
                    justifyContent: 'space-between',
                    alignItems: 'center',
                    marginBottom: 24,
                }}
            >
                <div>
                    <Title level={4} style={{ margin: 0 }}>
                        <ThunderboltOutlined style={{ marginRight: 8, color: '#fa8c16' }} />
                        {t('rate_limits.title', { defaultValue: 'Rate Limits' })}
                    </Title>
                    <Text type="secondary">
                        {t('rate_limits.subtitle', {
                            defaultValue: 'Platform-wide request rate limiting status.',
                        })}
                    </Text>
                </div>
                <Space>
                    <Button
                        icon={<ReloadOutlined />}
                        onClick={() => {
                            void refetch();
                            void refetchExemptions();
                        }}
                    >
                        {t('common:button.refresh')}
                    </Button>
                </Space>
            </div>

            <Card style={{ borderRadius: 12 }} styles={{ body: { padding: 0 } }}>
                {loadError ? (
                    <div style={{ padding: 16 }}>
                        <Text type="danger">
                            {loadError instanceof Error ? loadError.message : t('common:message.error')}
                        </Text>
                    </div>
                ) : null}
                <Table
                    dataSource={data?.items ?? []}
                    columns={columns}
                    rowKey={(r) => `${r.user_id}-${r.operation}`}
                    loading={isLoading}
                    pagination={false}
                    size="middle"
                    locale={{ emptyText: t('rate_limits.empty', { defaultValue: 'No rate limit data' }) }}
                />
            </Card>

            <Card
                style={{ borderRadius: 12, marginTop: 16 }}
                title={t('rate_limits.exemptions.title', { defaultValue: 'Exemptions' })}
                styles={{ body: { padding: 0 } }}
            >
                <Table
                    dataSource={exemptionData?.items ?? []}
                    columns={exemptionColumns}
                    rowKey="user_id"
                    loading={exemptionsLoading}
                    pagination={false}
                    size="middle"
                    locale={{
                        emptyText: t('rate_limits.exemptions.empty', { defaultValue: 'No exemptions configured' }),
                    }}
                />
            </Card>
        </div>
    );
}
