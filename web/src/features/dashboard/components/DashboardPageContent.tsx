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
import { LocalDateTimeText } from '@/components/ui/LocalDateTimeText';
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
    title: ReactNode;
    meta?: ReactNode;
    aside?: ReactNode;

    titleTone?: 'default' | 'accent';
    facts?: Array<{
        key: string;
        value: ReactNode;
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
    const { t } = useTranslation();
    const visibleItems = items.slice(0, DASHBOARD_CARD_PREVIEW_LIMIT);
    const overflowCount = Math.max(items.length - visibleItems.length, 0);

    return (
        <PageSurface className="dashboard-overview-card" styles={{ body: { padding: 0 } }}>
            <div className="dashboard-overview-card__shell">
                <div className="dashboard-overview-card__header" style={{
                    padding: '24px',
                    background: 'linear-gradient(135deg, rgba(248, 250, 252, 0.9) 0%, rgba(241, 245, 249, 0.4) 100%)',
                    borderBottom: '1px solid rgba(15, 23, 42, 0.04)',
                }}>
                    <div className="dashboard-overview-card__header-main" style={{ alignItems: 'center' }}>
                        <div className="dashboard-overview-card__icon" style={{
                            background: '#ffffff',
                            boxShadow: '0 2px 8px rgba(15, 23, 42, 0.04), 0 1px 2px rgba(15, 23, 42, 0.02)',
                            borderRadius: 12,
                            padding: 10,
                            border: '1px solid rgba(15, 23, 42, 0.03)'
                        }}>
                            {icon}
                        </div>
                        <div className="dashboard-overview-card__copy">
                            <div className="dashboard-overview-card__eyebrow" style={{ marginBottom: 4 }}>
                                <Text strong style={{ fontSize: 16, color: '#0f172a', letterSpacing: '-0.01em' }}>{title}</Text>
                                {badgeText ? <span className="dashboard-overview-card__badge" style={{ marginLeft: 8 }}>{badgeText}</span> : null}
                            </div>
                            <Text type="secondary" className="dashboard-overview-card__description" style={{ fontSize: 13, color: '#64748b' }}>
                                {description}
                            </Text>
                        </div>
                    </div>
                    <div className="dashboard-overview-card__metric" style={{ flexDirection: 'row', alignItems: 'center', gap: 10 }}>
                        <Text className="dashboard-overview-card__metric-label" style={{
                            display: 'inline-block',
                            fontSize: 12,
                            fontWeight: 700,
                            color: '#64748b',
                            letterSpacing: '0.04em',
                            textTransform: 'uppercase',
                        }}>
                            {t('dashboard.overview.total_metric', { defaultValue: 'TOTAL' })}
                        </Text>
                        <div style={{
                            display: 'flex',
                            alignItems: 'center',
                            justifyContent: 'center',
                            minWidth: 44,
                            height: 44,
                            background: '#ffffff',
                            border: '1px solid #e2e8f0',
                            borderRadius: 12,
                            boxShadow: '0 2px 4px rgba(15, 23, 42, 0.02), inset 0 1px 0 rgba(255, 255, 255, 1)',
                            padding: '0 12px'
                        }}>
                            <Text strong className="dashboard-overview-card__metric-value" style={{
                                fontSize: 22,
                                color: '#0f172a',
                                lineHeight: 1,
                                letterSpacing: '-0.02em',
                                fontFamily: 'Inter, -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif'
                            }}>
                                {total}
                            </Text>
                        </div>
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
                        value: <Text code style={{ fontSize: '13px', backgroundColor: '#f1f5f9' }}>{name}</Text>,
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
            titleTone: 'accent',
            meta: dashboardDescriptionPreview(service.description) ?? service.system_name,
            facts: [
                ...(service.system_name ? [{
                    key: `${service.id}-system`,
                    label: t('vm:field.system'),
                    value: <Text strong style={{ fontSize: '13px', color: '#334155' }}>{service.system_name}</Text>,
                    tone: 'default' as const,
                }] : []),
            ],
            aside: <LocalDateTimeText value={service.created_at} />,
        }))
    ), [services, t]);

    const vmPreviewItems = useMemo<DashboardPreviewItem[]>(() => (
        (vms?.items ?? []).map((vm) => ({
            key: vm.id,
            title: vm.name,
            titleTone: 'accent',
            meta: (
                <div style={{ display: 'flex', alignItems: 'center', gap: '8px', flexWrap: 'wrap', marginTop: '2px' }}>
                    {vm.hostname && vm.hostname !== vm.name ? (
                        <Text code style={{ fontSize: '13px', color: '#475569', backgroundColor: '#f1f5f9', border: 'none' }}>{vm.hostname}</Text>
                    ) : null}
                    {vm.os_name ? <Text type="secondary" style={{ fontSize: '13px' }}>{vm.os_name}</Text> : null}
                    {vm.ip_address ? <Text style={{ fontFamily: 'monospace', fontSize: '13px', color: '#64748b' }}>{vm.ip_address}</Text> : null}
                </div>
            ),
            aside: vm.status === 'RUNNING' ? (
                <div style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
                    <span style={{
                        width: 6, height: 6, borderRadius: '50%',
                        backgroundColor: '#10b981',
                        boxShadow: '0 0 0 2px rgba(16, 185, 129, 0.2)'
                    }} className="app-pulse-dot" />
                    <span>{t(`vm:status.${vm.status}`)}</span>
                </div>
            ) : t(`vm:status.${vm.status}`),
            tone: vm.status === 'RUNNING' ? 'positive' : vm.status === 'FAILED' ? 'critical' : 'default',
        }))
    ), [t, vms]);

    const myRequestPreviewItems = useMemo<DashboardPreviewItem[]>(() => (
        (myRequests?.items ?? []).map((ticket) => ({
            key: ticket.id,
            title: <Text strong style={{ color: '#1e293b' }}>{approvalSummaryTitle(ticket, t)}</Text>,
            meta: approvalSummaryMeta(ticket, t).slice(0, 2).join(' · '),
            aside: t(`approval:status.${ticket.status}`),
            tone: statusTone(ticket.status),
        }))
    ), [myRequests, t]);

    const pendingRequestCount = pendingTickets?.pagination?.total ?? 0;
    const systemsTotal = systems?.pagination?.total ?? 0;
    const servicesTotal = services?.pagination?.total ?? 0;
    const vmsTotal = vms?.pagination?.total ?? 0;
    const shouldPromoteSetupGuideShell = vmsTotal === 0;

    if (isLoading) {
        return (
            <div style={{ display: 'flex', justifyContent: 'center', alignItems: 'center', minHeight: 400 }}>
                <Spin size="large" />
            </div>
        );
    }

    return (
        <div className="dashboard-page" style={{ position: 'relative', zIndex: 0 }}>
            <PageHeader
                title={t('nav.dashboard')}
                subtitle={t('dashboard.subtitle')}
            />
            <PageSurface className="dashboard-command-deck" styles={{ body: { padding: 0 } }}>
                <div className="dashboard-command-deck__shell" style={{ position: 'relative', overflow: 'hidden' }}>
                    <div className="dashboard-command-deck__main" style={{ position: 'relative', zIndex: 1 }}>
                        <div className="dashboard-command-deck__copy">
                            <Text className="dashboard-command-deck__eyebrow">{t('app.subtitle')}</Text>
                            <Text className="dashboard-command-deck__title">{t('app.name')}</Text>
                            <Text className="dashboard-command-deck__subtitle">{t('app.description')}</Text>
                        </div>
                        <div className="dashboard-command-deck__actions">
                            <Button
                                type="primary"
                                className="app-shell-action-button app-shell-action-button--primary"
                                onClick={() => router.push('/vms?request=create')}
                            >
                                {t('quick_actions.new_vm_request')}
                            </Button>
                            <Button
                                className="app-shell-action-button"
                                onClick={() => router.push('/tickets')}
                            >
                                {t('dashboard.action.open_requests')}
                            </Button>
                            <Button
                                className="app-shell-action-button"
                                onClick={() => router.push('/services')}
                            >
                                {t('dashboard.action.open_services')}
                            </Button>
                        </div>
                    </div>
                    <div className="dashboard-command-deck__aside" style={{ position: 'relative', zIndex: 1 }}>
                        <div className="dashboard-command-deck__aside-section">
                            <Text className="dashboard-command-deck__aside-eyebrow">{t('dashboard.status.ready')}</Text>
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
                        </div>
                    </div>
                </div>
            </PageSurface>

            {shouldPromoteSetupGuideShell ? (
                <div className="dashboard-page__setup-shell">
                    <SetupGuideCard
                        variant="dashboard"
                        focusAction={setupAction}
                        snapshot={{
                            systemsTotal,
                            servicesTotal,
                            vmsTotal,
                        }}
                    />
                </div>
            ) : null}

            <div className="dashboard-page__overview-grid">
                <DashboardOverviewCard
                    title={t('nav.systems')}
                    description={t('dashboard.overview.systems_description')}
                    total={systemsTotal}
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
                    total={servicesTotal}
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
                    total={vmsTotal}
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

            {!shouldPromoteSetupGuideShell ? (
                <div className="dashboard-page__setup-shell">
                    <SetupGuideCard
                        variant="dashboard"
                        focusAction={setupAction}
                        snapshot={{
                            systemsTotal,
                            servicesTotal,
                            vmsTotal,
                        }}
                    />
                </div>
            ) : null}
        </div>
    );
}
