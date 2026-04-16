'use client';

/**
 * /admin/rate-limits — Platform-wide rate limiting status and configuration.
 * master-flow.md §10: Rate Limit management.
 *
 * API contracts:
 *   GET    /admin/rate-limits/status               → RateLimitStatusList
 *   GET    /admin/rate-limits/exemptions           → RateLimitExemptionList
 *   POST   /admin/rate-limits/exemptions           → RateLimitExemption
 *   DELETE /admin/rate-limits/exemptions/{user_id} → 204
 *   PUT    /admin/rate-limits/users/{user_id}      → RateLimitUserOverride
 *
 * E2E data-testid requirements:
 *   rate-limit-status-page
 */
import {
    DeleteOutlined,
    EditOutlined,
    PlusOutlined,
    ReloadOutlined,
    SettingOutlined,
} from '@ant-design/icons';
import {
    Alert,
    App,
    Button,
    DatePicker,
    Form,
    Input,
    InputNumber,
    Modal,
    Popconfirm,
    Select,
    Space,
    Table,
    Tag,
    Typography,
} from 'antd';
import dayjs, { type Dayjs } from 'dayjs';
import { useMemo, useState } from 'react';
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
import { PageSearchToolbar, filterOptionByLabel } from '@/components/ui/PageSearchToolbar';
import { useApiMutation } from '@/hooks/useApiQuery';
import { api } from '@/lib/api/client';
import { translateApiError } from '@/lib/api/errorMessage';
import { useApiGet } from '@/lib/api/useApiGet';
import type { components } from '@/types/api.gen';

const { Text } = Typography;

type RateLimitStatus = components['schemas']['RateLimitUserStatus'];
type RateLimitStatusList = components['schemas']['RateLimitStatusList'];
type RateLimitExemption = components['schemas']['RateLimitExemption'];
type RateLimitExemptionList = components['schemas']['RateLimitExemptionList'];
type RateLimitExemptionCreateRequest = components['schemas']['RateLimitExemptionCreateRequest'];
type RateLimitUserOverride = components['schemas']['RateLimitUserOverride'];
type RateLimitUserOverrideRequest = components['schemas']['RateLimitUserOverrideRequest'];
type UserList = components['schemas']['UserList'];

interface ExemptionFormValues {
    user_id: string;
    reason?: string;
    expires_at?: Dayjs | null;
}

interface OverrideFormValues {
    user_id: string;
    max_pending_parents?: number | null;
    max_pending_children?: number | null;
    cooldown_seconds?: number | null;
    reason?: string;
}

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
            <Text type="secondary" style={{ fontSize: 13 }}>{secondary}</Text>
            <Text type="secondary" style={{ fontSize: 13 }}>
                {email?.trim() || t('users.directory.no_email', { defaultValue: 'No contact email' })}
            </Text>
        </Space>
    );
}

function buildUserOptionLabel(displayName?: string, username?: string, email?: string, fallback?: string) {
    return [displayName?.trim(), username?.trim(), email?.trim()]
        .filter((value, index, array): value is string => Boolean(value) && array.indexOf(value) === index)
        .join(' · ') || fallback || '';
}

export default function AdminRateLimitsPageContent() {
    const { t } = useTranslation(['admin', 'common']);
    const { message: messageApi } = App.useApp();
    const [exemptionForm] = Form.useForm<ExemptionFormValues>();
    const [overrideForm] = Form.useForm<OverrideFormValues>();
    const [quickSearchDraft, setQuickSearchDraft] = useState('');
    const [search, setSearch] = useState('');
    const [userDraft, setUserDraft] = useState('');
    const [exemptionDraft, setExemptionDraft] = useState('');
    const [cooldownDraft, setCooldownDraft] = useState('');
    const [userFilter, setUserFilter] = useState('');
    const [exemptionFilter, setExemptionFilter] = useState('');
    const [cooldownFilter, setCooldownFilter] = useState('');
    const [advancedSearchOpen, setAdvancedSearchOpen] = useState(false);
    const [exemptionModalOpen, setExemptionModalOpen] = useState(false);
    const [overrideModalOpen, setOverrideModalOpen] = useState(false);
    const [exemptionUserLocked, setExemptionUserLocked] = useState(false);
    const [overrideUserLocked, setOverrideUserLocked] = useState(false);

    const { data, isLoading, error: statusError, refetch } = useApiGet<RateLimitStatusList>(
        ['admin-rate-limits-status'],
        () => api.GET('/admin/rate-limits/status', {}) as Promise<{ data?: RateLimitStatusList; error?: unknown; response?: Response }>,
    );

    const {
        data: exemptionData,
        isLoading: exemptionsLoading,
        error: exemptionsError,
        refetch: refetchExemptions,
    } = useApiGet<RateLimitExemptionList>(
        ['admin-rate-limits-exemptions'],
        () =>
            api.GET('/admin/rate-limits/exemptions', {
                params: { query: { page: 1, per_page: 100 } },
            }) as Promise<{ data?: RateLimitExemptionList; error?: unknown; response?: Response }>,
    );

    const { data: userDirectoryData } = useApiGet<UserList>(
        ['admin-rate-limits-users'],
        () =>
            api.GET('/admin/users', {
                params: { query: { page: 1, per_page: 200 } },
            }) as Promise<{ data?: UserList; error?: unknown; response?: Response }>,
        { staleTime: 60_000 },
    );

    const loadError = statusError ?? exemptionsError;
    const statusItems = useMemo(() => data?.items ?? [], [data?.items]);
    const exemptionItems = useMemo(() => exemptionData?.items ?? [], [exemptionData?.items]);

    const refetchAll = async () => {
        await Promise.all([refetch(), refetchExemptions()]);
    };

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

    const userOptions = useMemo(() => {
        const users = new Map<string, string>();
        for (const item of userDirectoryData?.items ?? []) {
            users.set(item.id, buildUserOptionLabel(item.display_name, item.username, item.email, item.id));
        }
        for (const item of statusItems) {
            users.set(item.user_id, buildUserOptionLabel(item.display_name, item.username, item.email, item.user_id));
        }
        for (const item of exemptionItems) {
            users.set(item.user_id, buildUserOptionLabel(item.display_name, item.username, item.email, item.user_id));
        }
        return Array.from(users.entries())
            .sort((left, right) => left[1].localeCompare(right[1]))
            .map(([value, label]) => ({ value, label }));
    }, [exemptionItems, statusItems, userDirectoryData?.items]);

    const exemptionOptions = useMemo(
        () => [
            {
                value: 'exempted',
                label: t('rate_limits.table.exempted_yes', { defaultValue: 'Exempted' }),
            },
            {
                value: 'standard',
                label: t('rate_limits.table.exempted_no', { defaultValue: 'Standard policy' }),
            },
        ],
        [t],
    );

    const cooldownOptions = useMemo(
        () => [
            {
                value: 'cooling',
                label: t('rate_limits.table.cooldown_filter_active', { defaultValue: 'Cooling down' }),
            },
            {
                value: 'ready',
                label: t('rate_limits.table.cooldown_ready', { defaultValue: 'Ready' }),
            },
        ],
        [t],
    );

    const filteredStatusItems = useMemo(() => {
        const normalizedSearch = search.trim().toLowerCase();
        return statusItems.filter((item) => {
            if (userFilter !== '' && item.user_id !== userFilter) {
                return false;
            }
            if (exemptionFilter === 'exempted' && !item.exempted) {
                return false;
            }
            if (exemptionFilter === 'standard' && item.exempted) {
                return false;
            }
            const isCooling = (item.cooldown_remaining_seconds ?? 0) > 0;
            if (cooldownFilter === 'cooling' && !isCooling) {
                return false;
            }
            if (cooldownFilter === 'ready' && isCooling) {
                return false;
            }
            if (normalizedSearch === '') {
                return true;
            }
            return [
                item.user_id,
                item.username ?? '',
                item.display_name ?? '',
                item.email ?? '',
            ].some((value) => value.toLowerCase().includes(normalizedSearch));
        });
    }, [cooldownFilter, exemptionFilter, search, statusItems, userFilter]);

    const filteredExemptionItems = useMemo(() => {
        const normalizedSearch = search.trim().toLowerCase();
        return exemptionItems.filter((item) => {
            if (userFilter !== '' && item.user_id !== userFilter) {
                return false;
            }
            if (exemptionFilter === 'standard') {
                return false;
            }
            if (normalizedSearch === '') {
                return true;
            }
            return [
                item.user_id,
                item.username ?? '',
                item.display_name ?? '',
                item.email ?? '',
                item.reason ?? '',
                item.exempted_by ?? '',
            ].some((value) => value.toLowerCase().includes(normalizedSearch));
        });
    }, [exemptionFilter, exemptionItems, search, userFilter]);

    const createExemptionMutation = useApiMutation<RateLimitExemptionCreateRequest, RateLimitExemption>(
        (body) =>
            api.POST('/admin/rate-limits/exemptions', {
                body,
            }),
        {
            onSuccess: async () => {
                messageApi.success(t('rate_limits.exemptions.save_success', { defaultValue: 'Rate-limit exemption saved.' }));
                setExemptionModalOpen(false);
                exemptionForm.resetFields();
                await refetchAll();
            },
            onError: (error) => {
                messageApi.error(translateApiError(t, error));
            },
        },
    );

    const deleteExemptionMutation = useApiMutation<string, void>(
        (userId) =>
            api.DELETE('/admin/rate-limits/exemptions/{user_id}', {
                params: { path: { user_id: userId } },
            }),
        {
            onSuccess: async () => {
                messageApi.success(t('rate_limits.exemptions.delete_success', { defaultValue: 'Rate-limit exemption removed.' }));
                await refetchAll();
            },
            onError: (error) => {
                messageApi.error(translateApiError(t, error));
            },
        },
    );

    const updateOverrideMutation = useApiMutation<
        { userId: string; body: RateLimitUserOverrideRequest },
        RateLimitUserOverride
    >(
        ({ userId, body }) =>
            api.PUT('/admin/rate-limits/users/{user_id}', {
                params: { path: { user_id: userId } },
                body,
            }),
        {
            onSuccess: async () => {
                messageApi.success(t('rate_limits.overrides.save_success', { defaultValue: 'User rate-limit override saved.' }));
                setOverrideModalOpen(false);
                overrideForm.resetFields();
                await refetchAll();
            },
            onError: (error) => {
                messageApi.error(translateApiError(t, error));
            },
        },
    );

    const openCreateExemptionModal = () => {
        setExemptionUserLocked(false);
        exemptionForm.resetFields();
        setExemptionModalOpen(true);
    };

    const openEditExemptionModal = (record: Pick<RateLimitExemption, 'user_id' | 'reason' | 'expires_at'>) => {
        setExemptionUserLocked(true);
        exemptionForm.setFieldsValue({
            user_id: record.user_id,
            reason: record.reason ?? '',
            expires_at: record.expires_at ? dayjs(record.expires_at) : null,
        });
        setExemptionModalOpen(true);
    };

    const openOverrideModal = (record?: Pick<RateLimitStatus, 'user_id' | 'effective_max_pending_parents' | 'effective_max_pending_children' | 'effective_cooldown_seconds'>) => {
        setOverrideUserLocked(Boolean(record));
        overrideForm.setFieldsValue({
            user_id: record?.user_id ?? '',
            max_pending_parents: record?.effective_max_pending_parents ?? null,
            max_pending_children: record?.effective_max_pending_children ?? null,
            cooldown_seconds: record?.effective_cooldown_seconds ?? null,
            reason: '',
        });
        setOverrideModalOpen(true);
    };

    const submitExemption = async () => {
        const values = await exemptionForm.validateFields();
        createExemptionMutation.mutate({
            user_id: values.user_id,
            reason: values.reason?.trim() || undefined,
            expires_at: values.expires_at ? values.expires_at.toISOString() : null,
        });
    };

    const submitOverride = async () => {
        const values = await overrideForm.validateFields();
        updateOverrideMutation.mutate({
            userId: values.user_id,
            body: {
                max_pending_parents: values.max_pending_parents ?? null,
                max_pending_children: values.max_pending_children ?? null,
                cooldown_seconds: values.cooldown_seconds ?? null,
                reason: values.reason?.trim() || undefined,
            },
        });
    };

    const statusColumns = [
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
                        <Text type="secondary" style={{ fontSize: 13 }}>
                            {t('rate_limits.table.exemption_expires_at', { defaultValue: 'Expires' })}: <LocalDateTimeText value={record.exemption_expires_at} />
                        </Text>
                    ) : null}
                </Space>
            ),
        },
        {
            title: t('common:table.actions', { defaultValue: 'Actions' }),
            key: 'actions',
            width: 240,
            render: (_: unknown, record: RateLimitStatus) => (
                <Space wrap>
                    <Button
                        size="small"
                        icon={<SettingOutlined />}
                        onClick={() => openOverrideModal(record)}
                        data-testid={`rate-limits-edit-override-${record.user_id}`}
                    >
                        {t('rate_limits.overrides.edit_action', { defaultValue: 'Set override' })}
                    </Button>
                    <Button
                        size="small"
                        icon={record.exempted ? <EditOutlined /> : <PlusOutlined />}
                        onClick={() =>
                            openEditExemptionModal({
                                user_id: record.user_id,
                                reason: '',
                                expires_at: record.exemption_expires_at ?? undefined,
                            })
                        }
                        data-testid={`rate-limits-edit-exemption-${record.user_id}`}
                    >
                        {record.exempted
                            ? t('rate_limits.exemptions.edit_action', { defaultValue: 'Edit exemption' })
                            : t('rate_limits.exemptions.add_action', { defaultValue: 'Add exemption' })}
                    </Button>
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
        {
            title: t('common:table.actions', { defaultValue: 'Actions' }),
            key: 'actions',
            width: 180,
            render: (_: unknown, record: RateLimitExemption) => (
                <Space wrap>
                    <Button
                        size="small"
                        icon={<EditOutlined />}
                        onClick={() => openEditExemptionModal(record)}
                        data-testid={`rate-limits-edit-existing-exemption-${record.user_id}`}
                    >
                        {t('common:button.edit', { defaultValue: 'Edit' })}
                    </Button>
                    <Popconfirm
                        title={t('rate_limits.exemptions.delete_confirm_title', { defaultValue: 'Remove exemption?' })}
                        description={t('rate_limits.exemptions.delete_confirm_description', { defaultValue: 'This user will go back to the standard rate-limit policy.' })}
                        okText={t('common:button.delete', { defaultValue: 'Delete' })}
                        cancelText={t('common:button.cancel', { defaultValue: 'Cancel' })}
                        onConfirm={() => deleteExemptionMutation.mutate(record.user_id)}
                    >
                        <Button
                            size="small"
                            danger
                            icon={<DeleteOutlined />}
                            loading={deleteExemptionMutation.isPending}
                            data-testid={`rate-limits-delete-exemption-${record.user_id}`}
                        >
                            {t('common:button.delete', { defaultValue: 'Delete' })}
                        </Button>
                    </Popconfirm>
                </Space>
            ),
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
                                void refetchAll();
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
                <div style={{ padding: 16, paddingBottom: 0 }}>
                    <PageSearchToolbar
                        searchValue={search}
                        searchDraftValue={quickSearchDraft}
                        onSearchDraftChange={setQuickSearchDraft}
                        onSearchChange={(value) => {
                            const nextValue = value.trim();
                            setQuickSearchDraft(nextValue);
                            setSearch(nextValue);
                        }}
                        searchPlaceholder={t('rate_limits.search_placeholder', { defaultValue: 'Search users, emails, usernames, reasons, or paste a user ID' })}
                        searchTestId="rate-limits-quick-search"
                        searchHelp={t('rate_limits.search_help', { defaultValue: 'Press Enter or click Search. Quick search matches users, usernames, emails, exemption reasons, and pasted user IDs.' })}
                        primaryActions={(
                            <Space wrap>
                                <Button
                                    icon={<PlusOutlined />}
                                    onClick={openCreateExemptionModal}
                                    data-testid="rate-limits-create-exemption"
                                >
                                    {t('rate_limits.exemptions.add_action', { defaultValue: 'Add exemption' })}
                                </Button>
                                <Button
                                    icon={<SettingOutlined />}
                                    onClick={() => openOverrideModal()}
                                    data-testid="rate-limits-create-override"
                                >
                                    {t('rate_limits.overrides.add_action', { defaultValue: 'Set user override' })}
                                </Button>
                            </Space>
                        )}
                        advancedSearch={{
                            open: advancedSearchOpen,
                            onToggle: () => setAdvancedSearchOpen((open) => !open),
                            openLabel: t('common:search.advanced', { defaultValue: 'Advanced search' }),
                            closeLabel: t('common:search.hide_advanced', { defaultValue: 'Hide advanced search' }),
                            title: t('common:search.advanced', { defaultValue: 'Advanced search' }),
                            toggleTestId: 'rate-limits-advanced-search-toggle',
                            content: (
                                <Space direction="vertical" size={12} style={{ width: '100%' }}>
                                    <Text type="secondary">
                                        {t('common:search.exact_match_help')}
                                    </Text>
                                    <Space wrap size={[12, 12]}>
                                        <Select
                                            allowClear
                                            showSearch
                                            filterOption={filterOptionByLabel}
                                            optionFilterProp="label"
                                            style={{ minWidth: 280 }}
                                            data-testid="rate-limits-filter-user"
                                            placeholder={t('rate_limits.table.user', { defaultValue: 'User' })}
                                            options={userOptions}
                                            value={userDraft || undefined}
                                            onChange={(value) => setUserDraft(value ?? '')}
                                        />
                                        <Select
                                            allowClear
                                            showSearch
                                            filterOption={filterOptionByLabel}
                                            optionFilterProp="label"
                                            style={{ minWidth: 220 }}
                                            data-testid="rate-limits-filter-exemption"
                                            placeholder={t('rate_limits.table.exemption', { defaultValue: 'Exemption' })}
                                            options={exemptionOptions}
                                            value={exemptionDraft || undefined}
                                            onChange={(value) => setExemptionDraft(value ?? '')}
                                        />
                                        <Select
                                            allowClear
                                            showSearch
                                            filterOption={filterOptionByLabel}
                                            optionFilterProp="label"
                                            style={{ minWidth: 220 }}
                                            data-testid="rate-limits-filter-cooldown"
                                            placeholder={t('rate_limits.table.cooldown', { defaultValue: 'Cooldown' })}
                                            options={cooldownOptions}
                                            value={cooldownDraft || undefined}
                                            onChange={(value) => setCooldownDraft(value ?? '')}
                                        />
                                        <Button
                                            type="primary"
                                            data-testid="rate-limits-advanced-search-submit"
                                            onClick={() => {
                                                setUserFilter(userDraft);
                                                setExemptionFilter(exemptionDraft);
                                                setCooldownFilter(cooldownDraft);
                                            }}
                                        >
                                            {t('common:button.search')}
                                        </Button>
                                    </Space>
                                </Space>
                            ),
                        }}
                        hasActiveFilters={search !== '' || userFilter !== '' || exemptionFilter !== '' || cooldownFilter !== ''}
                        onClear={() => {
                            setQuickSearchDraft('');
                            setSearch('');
                            setUserDraft('');
                            setExemptionDraft('');
                            setCooldownDraft('');
                            setUserFilter('');
                            setExemptionFilter('');
                            setCooldownFilter('');
                            setAdvancedSearchOpen(false);
                        }}
                        clearLabel={t('common:button.clear_filters', { defaultValue: 'Clear filters' })}
                    />
                </div>
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
                    dataSource={filteredStatusItems}
                    columns={statusColumns}
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
                    dataSource={filteredExemptionItems}
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

            <Modal
                open={exemptionModalOpen}
                title={t('rate_limits.exemptions.modal_title', { defaultValue: 'Manage rate-limit exemption' })}
                okText={t('common:button.save', { defaultValue: 'Save' })}
                cancelText={t('common:button.cancel', { defaultValue: 'Cancel' })}
                confirmLoading={createExemptionMutation.isPending}
                onOk={() => void submitExemption()}
                onCancel={() => {
                    setExemptionModalOpen(false);
                    exemptionForm.resetFields();
                }}
                destroyOnHidden
            >
                <Form form={exemptionForm} layout="vertical">
                    <Form.Item
                        name="user_id"
                        label={t('rate_limits.exemptions.user', { defaultValue: 'User' })}
                        rules={[{
                            required: true,
                            message: t('rate_limits.exemptions.user_required', { defaultValue: 'Select a user.' }),
                        }]}
                    >
                        <Select
                            showSearch
                            disabled={exemptionUserLocked}
                            placeholder={t('rate_limits.exemptions.user_placeholder', { defaultValue: 'Search by name, username, or email' })}
                            optionFilterProp="label"
                            filterOption={filterOptionByLabel}
                            options={userOptions}
                        />
                    </Form.Item>
                    <Form.Item
                        name="reason"
                        label={t('rate_limits.exemptions.reason', { defaultValue: 'Reason' })}
                    >
                        <Input.TextArea rows={3} maxLength={240} />
                    </Form.Item>
                    <Form.Item
                        name="expires_at"
                        label={t('rate_limits.exemptions.expires_at', { defaultValue: 'Expires At' })}
                    >
                        <DatePicker
                            showTime
                            format="YYYY-MM-DD HH:mm"
                            style={{ width: '100%' }}
                            placeholder={t('rate_limits.exemptions.never_expires', { defaultValue: 'Never expires' })}
                        />
                    </Form.Item>
                </Form>
            </Modal>

            <Modal
                open={overrideModalOpen}
                title={t('rate_limits.overrides.modal_title', { defaultValue: 'Manage user rate-limit override' })}
                okText={t('common:button.save', { defaultValue: 'Save' })}
                cancelText={t('common:button.cancel', { defaultValue: 'Cancel' })}
                confirmLoading={updateOverrideMutation.isPending}
                onOk={() => void submitOverride()}
                onCancel={() => {
                    setOverrideModalOpen(false);
                    overrideForm.resetFields();
                }}
                destroyOnHidden
            >
                <Form form={overrideForm} layout="vertical">
                    <Form.Item
                        name="user_id"
                        label={t('rate_limits.exemptions.user', { defaultValue: 'User' })}
                        rules={[{
                            required: true,
                            message: t('rate_limits.exemptions.user_required', { defaultValue: 'Select a user.' }),
                        }]}
                    >
                        <Select
                            showSearch
                            disabled={overrideUserLocked}
                            placeholder={t('rate_limits.exemptions.user_placeholder', { defaultValue: 'Search by name, username, or email' })}
                            optionFilterProp="label"
                            filterOption={filterOptionByLabel}
                            options={userOptions}
                        />
                    </Form.Item>
                    <Form.Item
                        name="max_pending_parents"
                        label={t('rate_limits.overrides.max_pending_parents', { defaultValue: 'Parent request limit' })}
                    >
                        <InputNumber min={1} style={{ width: '100%' }} />
                    </Form.Item>
                    <Form.Item
                        name="max_pending_children"
                        label={t('rate_limits.overrides.max_pending_children', { defaultValue: 'Child request limit' })}
                    >
                        <InputNumber min={1} style={{ width: '100%' }} />
                    </Form.Item>
                    <Form.Item
                        name="cooldown_seconds"
                        label={t('rate_limits.overrides.cooldown_seconds', { defaultValue: 'Cooldown (seconds)' })}
                    >
                        <InputNumber min={0} style={{ width: '100%' }} />
                    </Form.Item>
                    <Form.Item
                        name="reason"
                        label={t('rate_limits.exemptions.reason', { defaultValue: 'Reason' })}
                    >
                        <Input.TextArea rows={3} maxLength={240} />
                    </Form.Item>
                </Form>
            </Modal>
        </div>
    );
}
