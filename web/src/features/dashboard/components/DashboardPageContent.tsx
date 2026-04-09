'use client';

import { useMemo, type ReactNode } from 'react';
import {
    Button,
    Typography,
    Badge,
    Spin,
} from 'antd';
import { useRouter, useSearchParams } from 'next/navigation';
import { useTranslation } from 'react-i18next';
import {
    SystemsIcon,
    ServicesIcon,
    VMsIcon,
    RequestsIcon,
} from '@/components/layouts/MenuIcons';
import { PageHeader, PageSurface } from '@/components/layouts/PageSection';
import { SetupGuideCard } from '@/features/setup-guide/components/SetupGuideCard';
import type { SetupResumeAction } from '@/features/setup-guide/flow';
import { approvalSummaryMeta, approvalSummaryTitle } from '@/features/approval-shared/summary';
import { useApiGet, type ApiErrorResponse } from '@/hooks/useApiQuery';
import { api } from '@/lib/api/client';
import type { components } from '@/types/api.gen';

const { Text } = Typography;

type SystemList = components['schemas']['SystemList'];
type ServiceList = components['schemas']['ServiceList'];
type VMList = components['schemas']['VMList'];
type TicketList = components['schemas']['TicketList'];
type Ticket = components['schemas']['Ticket'];
type Health = components['schemas']['Health'];
type ApiFetchResult<T> = { data?: T; error?: ApiErrorResponse; response: Response };

type DashboardPreviewItem = {
    key: string;
    title: string;
    meta?: string;
    aside?: string;
    titleTone?: 'default' | 'accent';
    facts?: Array<{
        key: string;
        value: string;
        label?: string;
        tone?: 'default' | 'accent' | 'network' | 'identity';
    }>;
    tone?: 'default' | 'positive' | 'warning' | 'critical';
};

type SystemServicePreview = {
    total: number;
    names: string[];
};

const DASHBOARD_CARD_PREVIEW_LIMIT = 3;

const HEALTH_STATUS_MAP: Record<string, { color: string; badge: 'success' | 'warning' | 'error' | 'default' }> = {
    ok: { color: '#52c41a', badge: 'success' },
    degraded: { color: '#faad14', badge: 'warning' },
    error: { color: '#ff4d4f', badge: 'error' },
};
const SETUP_RESUME_ACTIONS: SetupResumeAction[] = [
    'create-system',
    'create-service',
    'create-namespace',
    'create-template',
    'create-instance-size',
    'open-vm-request',
];

function joinDefined(parts: Array<string | undefined | null>, separator = ' · '): string | undefined {
    const values = parts.filter((part): part is string => typeof part === 'string' && part.trim() !== '');
    return values.length > 0 ? values.join(separator) : undefined;
}

function truncateUtf8(value: string, maxBytes: number): string {
    const encoder = new TextEncoder();
    if (encoder.encode(value).length <= maxBytes) {
        return value;
    }

    let truncated = '';
    for (const char of value) {
        const next = `${truncated}${char}`;
        if (encoder.encode(next).length > maxBytes) {
            break;
        }
        truncated = next;
    }

    return `${truncated.trimEnd()}…`;
}

function dashboardDescriptionPreview(value: string | undefined, maxBytes = 36): string | undefined {
    if (!value) {
        return undefined;
    }
    const normalized = value
        .replace(/\[([^\]]+)\]\([^)]+\)/g, '$1')
        .replace(/^[#>*-\s]+/gm, '')
        .replace(/[`*_~]/g, '')
        .replace(/\s+/g, ' ')
        .trim();
    if (!normalized) {
        return undefined;
    }
    return truncateUtf8(normalized, maxBytes);
}

function statusTone(status: Ticket['status'] | undefined): DashboardPreviewItem['tone'] {
    switch (status) {
        case 'SUCCESS':
        case 'APPROVED':
            return 'positive';
        case 'PENDING':
        case 'EXECUTING':
            return 'warning';
        case 'FAILED':
        case 'REJECTED':
        case 'CANCELLED':
            return 'critical';
        default:
            return 'default';
    }
}

function renderStatusChip(
    label: string,
    value: string,
    badgeStatus: 'success' | 'warning' | 'error' | 'default',
    accentColor?: string,
) {
    return (
        <div className="dashboard-status-chip">
            <span className="dashboard-status-chip__label">{label}</span>
            <span className="dashboard-status-chip__value" style={accentColor ? { color: accentColor } : undefined}>
                <Badge status={badgeStatus} />
                {value}
            </span>
        </div>
    );
}

function DashboardOverviewCard({
    title,
    description,
    total,
    icon,
    actionLabel,
    onAction,
    items,
    emptyLabel,
    badgeText,
    workspaceHint,
    renderOverflowLabel,
}: {
    title: string;
    description: string;
    total: number;
    icon: ReactNode;
    actionLabel: string;
    onAction: () => void;
    items: DashboardPreviewItem[];
    emptyLabel: string;
    badgeText?: string;
    workspaceHint?: string;
    renderOverflowLabel?: (count: number) => string;
}) {
    const visibleItems = items.slice(0, DASHBOARD_CARD_PREVIEW_LIMIT);
    const overflowCount = Math.max(items.length - visibleItems.length, 0);

    return (
        <PageSurface className="dashboard-overview-card" styles={{ body: { padding: 0 } }}>
            <div className="dashboard-overview-card__shell">
                <div className="dashboard-overview-card__header">
                    <div className="dashboard-overview-card__header-main">
                        <div className="dashboard-overview-card__icon">
                            {icon}
                        </div>
                        <div className="dashboard-overview-card__copy">
                            <div className="dashboard-overview-card__eyebrow">
                                <Text strong>{title}</Text>
                                {badgeText ? <span className="dashboard-overview-card__badge">{badgeText}</span> : null}
                            </div>
                            <Text type="secondary" className="dashboard-overview-card__description">
                                {description}
                            </Text>
                        </div>
                    </div>
                    <div className="dashboard-overview-card__metric">
                        <Text className="dashboard-overview-card__metric-label">
                            {title}
                        </Text>
                        <Text strong className="dashboard-overview-card__metric-value">
                            {total}
                        </Text>
                    </div>
                </div>

                <div className="dashboard-overview-card__list">
                    {visibleItems.length > 0 ? visibleItems.map((item) => (
                        <div key={item.key} className="dashboard-overview-card__item">
                            <div className="dashboard-overview-card__item-main">
                                <Text
                                    strong
                                    className={`dashboard-overview-card__item-title dashboard-overview-card__item-title--${item.titleTone ?? 'default'}`}
                                >
                                    {item.title}
                                </Text>
                                {item.meta ? (
                                    <Text type="secondary" className="dashboard-overview-card__item-meta">
                                        {item.meta}
                                    </Text>
                                ) : null}
                                {item.facts && item.facts.length > 0 ? (
                                    <div className="dashboard-overview-card__facts">
                                        {item.facts.map((fact) => (
                                            <span
                                                key={fact.key}
                                                className={`dashboard-overview-card__fact dashboard-overview-card__fact--${fact.tone ?? 'default'}`}
                                            >
                                                {fact.label ? (
                                                    <span className="dashboard-overview-card__fact-label">{fact.label}</span>
                                                ) : null}
                                                <span className="dashboard-overview-card__fact-value">{fact.value}</span>
                                            </span>
                                        ))}
                                    </div>
                                ) : null}
                            </div>
                            {item.aside ? (
                                <span className={`dashboard-overview-card__item-aside dashboard-overview-card__item-aside--${item.tone ?? 'default'}`}>
                                    {item.aside}
                                </span>
                            ) : null}
                        </div>
                    )) : (
                        <div className="dashboard-overview-card__empty">
                            <Text type="secondary">{emptyLabel}</Text>
                        </div>
                    )}
                    {overflowCount > 0 ? (
                        <div className="dashboard-overview-card__more">
                            <Text strong className="dashboard-overview-card__more-count">
                                {renderOverflowLabel ? renderOverflowLabel(overflowCount) : `+${overflowCount}`}
                            </Text>
                            {workspaceHint ? (
                                <Text type="secondary" className="dashboard-overview-card__more-hint">
                                    {workspaceHint}
                                </Text>
                            ) : null}
                        </div>
                    ) : null}
                </div>

                <div className="dashboard-overview-card__footer">
                    <Button className="app-shell-action-button" onClick={onAction}>
                        {actionLabel}
                    </Button>
                </div>
            </div>
        </PageSurface>
    );
}

export function DashboardPageContent() {
    const { t } = useTranslation(['common', 'approval', 'vm']);
    const router = useRouter();
    const searchParams = useSearchParams();
    const rawSetupAction = searchParams.get('setup_action');
    const setupAction = SETUP_RESUME_ACTIONS.includes(rawSetupAction as SetupResumeAction)
        ? rawSetupAction as SetupResumeAction
        : null;

    const fetchReadiness = async (): Promise<ApiFetchResult<Health>> => {
        const result = await api.GET('/health/ready');
        // /health/ready uses 503 + Health payload for degraded state.
        if (result.error && result.response.status === 503) {
            return { data: result.error as unknown as Health, response: result.response };
        }
        return result as unknown as ApiFetchResult<Health>;
    };

    const fetchLiveness = async (): Promise<ApiFetchResult<Health>> => {
        return api.GET('/health/live') as unknown as ApiFetchResult<Health>;
    };

    // Fetch health status
    const { data: health, isLoading: healthLoading } = useApiGet<Health>(
        ['health'],
        fetchReadiness,
        { refetchInterval: 30000 }
    );

    const { data: liveness, isLoading: livenessLoading } = useApiGet<Health>(
        ['health-live'],
        fetchLiveness,
        { refetchInterval: 30000 }
    );

    // Fetch aggregated data for stats
    const { data: systems, isLoading: systemsLoading } = useApiGet<SystemList>(
        ['systems', 'dashboard'],
        () => api.GET('/systems', { params: { query: { per_page: 4 } } })
    );

    const { data: services, isLoading: servicesLoading } = useApiGet<ServiceList>(
        ['services', 'dashboard'],
        () => api.GET('/services', { params: { query: { per_page: 4 } } })
    );

    const systemIds = useMemo(
        () => (systems?.items ?? []).map((system) => system.id),
        [systems],
    );

    const { data: systemServicePreviews, isLoading: systemServicePreviewsLoading } = useApiGet<Record<string, SystemServicePreview>>(
        ['dashboard', 'system-service-previews', systemIds.join(',')],
        async () => {
            const results = await Promise.all(
                systemIds.map((systemId) => api.GET('/systems/{system_id}/services', {
                    params: {
                        path: { system_id: systemId },
                        query: { per_page: 3 },
                    },
                })),
            );
            const failed = results.find((result) => result.error);
            if (failed?.error) {
                return { error: failed.error, response: failed.response };
            }
            return {
                data: Object.fromEntries(
                    results.map((result, index) => [
                        systemIds[index],
                        {
                            total: result.data?.pagination?.total ?? 0,
                            names: (result.data?.items ?? []).map((service) => service.name).filter(Boolean).slice(0, 2),
                        },
                    ]),
                ),
                response: new Response(),
            };
        },
        { enabled: systemIds.length > 0 },
    );

    const { data: vms, isLoading: vmsLoading } = useApiGet<VMList>(
        ['vms', 'dashboard'],
        () => api.GET('/vms', { params: { query: { per_page: 4 } } })
    );

    const { data: myRequests, isLoading: requestsLoading } = useApiGet<TicketList>(
        ['tickets', 'dashboard', 'mine'],
        () => api.GET('/tickets', { params: { query: { mine: true, per_page: 4 } } })
    );

    const { data: pendingTickets, isLoading: ticketsLoading } = useApiGet<TicketList>(
        ['tickets', 'dashboard', 'mine', 'pending'],
        () => api.GET('/tickets', { params: { query: { mine: true, status: 'PENDING', per_page: 1 } } })
    );

    const isLoading =
        healthLoading ||
        livenessLoading ||
        systemsLoading ||
        servicesLoading ||
        vmsLoading ||
        requestsLoading ||
        ticketsLoading ||
        systemServicePreviewsLoading;

    const healthStatus = useMemo(() => {
        if (!health || typeof health.status !== 'string') {
            return { status: 'unknown', color: '#d9d9d9', badge: 'default' as const };
        }
        const normalized = health.status.toLowerCase();
        const mapped = HEALTH_STATUS_MAP[normalized] ?? { color: '#ff4d4f', badge: 'error' as const };
        return { status: normalized, ...mapped };
    }, [health]);

    const systemPreviewItems = useMemo<DashboardPreviewItem[]>(() => (
        (systems?.items ?? []).map((system) => ({
            key: system.id,
            title: system.name,
            meta: dashboardDescriptionPreview(system.description),
            facts: (() => {
                const preview = systemServicePreviews?.[system.id];
                if (!preview || preview.total === 0) {
                    return [];
                }

                const facts: DashboardPreviewItem['facts'] = [
                    {
                        key: `${system.id}-services-total`,
                        value: t('dashboard.overview.service_total', { count: preview.total }),
                        tone: 'accent',
                    },
                ];

                preview.names.forEach((name, index) => {
                    facts.push({
                        key: `${system.id}-service-${index}`,
                        value: name,
                        tone: 'default',
                    });
                });

                const remaining = preview.total - preview.names.length;
                if (remaining > 0) {
                    facts.push({
                        key: `${system.id}-service-more`,
                        value: t('dashboard.overview.more_items', { count: remaining }),
                        tone: 'default',
                    });
                }

                return facts;
            })(),
        }))
    ), [systemServicePreviews, systems, t]);

    const servicePreviewItems = useMemo<DashboardPreviewItem[]>(() => (
        (services?.items ?? []).map((service) => ({
            key: service.id,
            title: service.name,
            meta: dashboardDescriptionPreview(service.description) ?? service.system_name,
            facts: [
                ...(service.system_name ? [{
                    key: `${service.id}-system`,
                    label: t('vm:field.system'),
                    value: service.system_name,
                    tone: 'default' as const,
                }] : []),
                ...(typeof service.next_instance_index === 'number' ? [{
                    key: `${service.id}-next-vm`,
                    label: t('dashboard.overview.next_vm_number_label'),
                    value: String(service.next_instance_index),
                    tone: 'accent' as const,
                }] : []),
            ],
        }))
    ), [services, t]);

    const vmPreviewItems = useMemo<DashboardPreviewItem[]>(() => (
        (vms?.items ?? []).map((vm) => ({
            key: vm.id,
            title: vm.name,
            titleTone: 'accent',
            meta: joinDefined([
                vm.hostname && vm.hostname !== vm.name ? vm.hostname : undefined,
                vm.os_name,
                vm.ip_address,
            ]),
            aside: t(`vm:status.${vm.status}`),
            tone: vm.status === 'RUNNING' ? 'positive' : vm.status === 'FAILED' ? 'critical' : 'default',
        }))
    ), [t, vms]);

    const myRequestPreviewItems = useMemo<DashboardPreviewItem[]>(() => (
        (myRequests?.items ?? []).map((ticket) => ({
            key: ticket.id,
            title: approvalSummaryTitle(ticket, t),
            meta: approvalSummaryMeta(ticket, t).slice(0, 2).join(' · '),
            aside: t(`approval:status.${ticket.status}`),
            tone: statusTone(ticket.status),
        }))
    ), [myRequests, t]);

    const pendingRequestCount = pendingTickets?.pagination?.total ?? 0;

    if (isLoading) {
        return (
            <div style={{ display: 'flex', justifyContent: 'center', alignItems: 'center', minHeight: 400 }}>
                <Spin size="large" />
            </div>
        );
    }

    return (
        <div className="dashboard-page">
            <PageHeader
                title={t('nav.dashboard')}
                subtitle={t('dashboard.subtitle')}
                actions={(
                    <div className="dashboard-status-strip">
                        {renderStatusChip(
                            t('dashboard.status.ready'),
                            String(healthStatus.status || 'unknown').toUpperCase(),
                            healthStatus.badge,
                            healthStatus.color,
                        )}
                        {renderStatusChip(
                            t('dashboard.status.live'),
                            String(liveness?.status || 'unknown').toUpperCase(),
                            typeof liveness?.status === 'string' && liveness.status.toLowerCase() === 'ok' ? 'success' : 'default',
                        )}
                        {health?.version ? renderStatusChip(
                            t('dashboard.status.version'),
                            `v${health.version}`,
                            'default',
                        ) : null}
                    </div>
                )}
            />

            <div className="dashboard-page__overview-grid">
                <DashboardOverviewCard
                    title={t('nav.systems')}
                    description={t('dashboard.overview.systems_description')}
                    total={systems?.pagination?.total ?? 0}
                    icon={<SystemsIcon className="dashboard-overview-card__icon-art" />}
                    actionLabel={t('dashboard.action.open_systems')}
                    onAction={() => router.push('/systems')}
                    items={systemPreviewItems}
                    emptyLabel={t('dashboard.overview.empty_systems')}
                    renderOverflowLabel={(count) => t('dashboard.overview.more_items', { count })}
                    workspaceHint={t('dashboard.overview.workspace_hint')}
                />
                <DashboardOverviewCard
                    title={t('nav.services')}
                    description={t('dashboard.overview.services_description')}
                    total={services?.pagination?.total ?? 0}
                    icon={<ServicesIcon className="dashboard-overview-card__icon-art" />}
                    actionLabel={t('dashboard.action.open_services')}
                    onAction={() => router.push('/services')}
                    items={servicePreviewItems}
                    emptyLabel={t('dashboard.overview.empty_services')}
                    renderOverflowLabel={(count) => t('dashboard.overview.more_items', { count })}
                    workspaceHint={t('dashboard.overview.workspace_hint')}
                />
                <DashboardOverviewCard
                    title={t('nav.vms')}
                    description={t('dashboard.overview.vms_description')}
                    total={vms?.pagination?.total ?? 0}
                    icon={<VMsIcon className="dashboard-overview-card__icon-art" />}
                    actionLabel={t('dashboard.action.open_vms')}
                    onAction={() => router.push('/vms')}
                    items={vmPreviewItems}
                    emptyLabel={t('dashboard.overview.empty_vms')}
                    renderOverflowLabel={(count) => t('dashboard.overview.more_items', { count })}
                    workspaceHint={t('dashboard.overview.workspace_hint')}
                />
                <DashboardOverviewCard
                    title={t('nav.my_requests')}
                    description={t('dashboard.overview.requests_description')}
                    total={myRequests?.pagination?.total ?? 0}
                    icon={<RequestsIcon className="dashboard-overview-card__icon-art" />}
                    actionLabel={t('dashboard.action.open_requests')}
                    onAction={() => router.push('/tickets')}
                    items={myRequestPreviewItems}
                    emptyLabel={t('dashboard.overview.empty_requests')}
                    badgeText={pendingRequestCount > 0 ? t('dashboard.overview.pending_badge', { count: pendingRequestCount }) : undefined}
                    renderOverflowLabel={(count) => t('dashboard.overview.more_items', { count })}
                    workspaceHint={t('dashboard.overview.workspace_hint')}
                />
            </div>

            <div className="dashboard-page__setup-shell">
                <SetupGuideCard
                    variant="dashboard"
                    focusAction={setupAction}
                    snapshot={{
                        systemsTotal: systems?.pagination?.total ?? 0,
                        servicesTotal: services?.pagination?.total ?? 0,
                        vmsTotal: vms?.pagination?.total ?? 0,
                    }}
                />
            </div>
        </div>
    );
}
