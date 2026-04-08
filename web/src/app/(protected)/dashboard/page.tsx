'use client';

/**
 * Dashboard page — system overview with real API data.
 *
 * Fetches health status and aggregated statistics from backend.
 * Uses TanStack Query for caching and automatic refetch.
 */
import { useMemo } from 'react';
import type { CSSProperties } from 'react';
import {
    Row,
    Col,
    Statistic,
    Typography,
    Badge,
    Spin,
    Alert,
} from 'antd';
import { useSearchParams } from 'next/navigation';
import { useTranslation } from 'react-i18next';
import {
    SystemsIcon,
    VMsIcon,
    RequestsIcon,
    DashboardIcon,
} from '@/components/layouts/MenuIcons';
import { PageHeader, PageSurface } from '@/components/layouts/PageSection';
import { SetupGuideCard } from '@/features/setup-guide/components/SetupGuideCard';
import type { SetupResumeAction } from '@/features/setup-guide/flow';
import { useApiGet, type ApiErrorResponse } from '@/hooks/useApiQuery';
import { api } from '@/lib/api/client';
import type { components } from '@/types/api.gen';

const { Text } = Typography;

type SystemList = components['schemas']['SystemList'];
type VMList = components['schemas']['VMList'];
type TicketList = components['schemas']['TicketList'];
type Health = components['schemas']['Health'];
type ApiFetchResult<T> = { data?: T; error?: ApiErrorResponse; response: Response };

const HEALTH_STATUS_MAP: Record<string, { color: string }> = {
    ok: { color: '#52c41a' },
    degraded: { color: '#faad14' },
    error: { color: '#ff4d4f' },
};
const SETUP_RESUME_ACTIONS: SetupResumeAction[] = [
    'create-system',
    'create-service',
    'create-namespace',
    'create-template',
    'create-instance-size',
    'open-vm-request',
];

export default function DashboardPage() {
    const { t } = useTranslation(['common', 'approval']);
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
        () => api.GET('/systems', { params: { query: { per_page: 1 } } })
    );

    const { data: vms, isLoading: vmsLoading } = useApiGet<VMList>(
        ['vms', 'dashboard'],
        () => api.GET('/vms', { params: { query: { per_page: 1 } } })
    );

    const { data: pendingTickets, isLoading: ticketsLoading } = useApiGet<TicketList>(
        ['tickets', 'dashboard', 'mine', 'pending'],
        () => api.GET('/tickets', { params: { query: { mine: true, status: 'PENDING', per_page: 1 } } })
    );

    const isLoading = healthLoading || livenessLoading || systemsLoading || vmsLoading || ticketsLoading;

    const healthStatus = useMemo(() => {
        if (!health || typeof health.status !== 'string') {
            return { status: 'unknown', color: '#d9d9d9' };
        }
        const normalized = health.status.toLowerCase();
        const mapped = HEALTH_STATUS_MAP[normalized] ?? HEALTH_STATUS_MAP.error;
        return { status: normalized, ...mapped };
    }, [health]);

    const stats = useMemo(() => [
        {
            title: t('nav.systems'),
            value: systems?.pagination?.total ?? 0,
            icon: <SystemsIcon className="dashboard-stat__icon-art" />,
            color: 'linear-gradient(180deg, rgba(94, 106, 210, 0.16) 0%, rgba(94, 106, 210, 0.08) 100%)',
            iconColor: '#5E6AD2',
        },
        {
            title: t('nav.vms'),
            value: vms?.pagination?.total ?? 0,
            icon: <VMsIcon className="dashboard-stat__icon-art" />,
            color: 'linear-gradient(180deg, rgba(17, 183, 138, 0.16) 0%, rgba(17, 183, 138, 0.08) 100%)',
            iconColor: '#109A73',
        },
        {
            title: t('nav.my_requests'),
            value: pendingTickets?.pagination?.total ?? 0,
            icon: <RequestsIcon className="dashboard-stat__icon-art" />,
            color: 'linear-gradient(180deg, rgba(214, 106, 31, 0.16) 0%, rgba(214, 106, 31, 0.08) 100%)',
            iconColor: '#C45B17',
            suffix: <span style={{ whiteSpace: 'nowrap', marginLeft: 8, fontSize: 13, fontWeight: 500, color: 'rgba(15, 23, 42, 0.55)' }}>{t('approval:status.PENDING')}</span>,
        },
    ], [t, systems, vms, pendingTickets]);

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
            />

            {/* Health Status Banner */}
            <PageSurface className="dashboard-page__health-surface" style={{ marginBottom: 24 }} styles={{ body: { padding: '16px 24px' } }}>
                <div className="dashboard-health">
                    <div className="dashboard-health__meta">
                        <Badge
                            status={healthStatus.status === 'ok' ? 'success' : healthStatus.status === 'degraded' ? 'warning' : 'error'}
                            style={{ transform: 'scale(1.5)', marginRight: 6 }}
                        />
                        <div>
                            <Text strong style={{ fontSize: 20, letterSpacing: '-0.3px', color: 'rgba(15, 23, 42, 0.9)' }}>
                                Platform Health
                            </Text>
                            <div className="workbench-resource-facts" style={{ marginTop: 10 }}>
                                <span className="workbench-resource-fact" style={{ color: healthStatus.color }}>
                                    <span style={{ marginRight: 6, opacity: 0.8 }}>Status:</span>
                                    {String(healthStatus.status || 'unknown').toUpperCase()}
                                </span>
                                {health?.version && (
                                    <span className="workbench-resource-fact">
                                        <span style={{ marginRight: 6, opacity: 0.6 }}>Version:</span>
                                        v{health.version}
                                    </span>
                                )}
                                {typeof liveness?.status === 'string' && (
                                    <span className="workbench-resource-fact">
                                        <span style={{ marginRight: 6, opacity: 0.6 }}>Live:</span>
                                        {liveness.status.toUpperCase()}
                                    </span>
                                )}
                            </div>
                        </div>
                    </div>
                    <div className="dashboard-health__visual" style={{ color: healthStatus.color }}>
                        <DashboardIcon className="dashboard-health__visual-art" style={{ width: 64, height: 64, opacity: 0.9 }} />
                    </div>
                </div>
            </PageSurface>

            {/* Quick Stats */}
            <Row className="dashboard-page__stats-row" gutter={[16, 16]} style={{ marginBottom: 24 }}>
                {stats.map((stat) => (
                    <Col xs={24} sm={8} key={stat.title}>
                        <PageSurface className="dashboard-page__stat-surface" styles={{ body: { padding: '20px 24px' } }}>
                            <div className="dashboard-stat">
                                <div
                                    className="dashboard-stat__icon"
                                    style={{
                                        '--dashboard-stat-bg': stat.color,
                                        '--dashboard-stat-fg': stat.iconColor,
                                    } as CSSProperties}
                                >
                                    {stat.icon}
                                </div>
                                <Statistic
                                    className="dashboard-stat__metric"
                                    title={<span style={{ whiteSpace: 'nowrap', fontSize: 14, fontWeight: 500, color: 'rgba(15, 23, 42, 0.6)' }}>{stat.title}</span>}
                                    value={stat.value}
                                    suffix={stat.suffix}
                                    valueStyle={{ fontSize: 28, fontWeight: 700, color: 'rgba(15, 23, 42, 0.9)' }}
                                />
                            </div>
                        </PageSurface>
                    </Col>
                ))}
            </Row>

            {/* Pending Approvals Alert */}
            {(pendingTickets?.pagination?.total ?? 0) > 0 && (
                <Alert
                    className="dashboard-page__pending-alert"
                    type="warning"
                    showIcon
                    message={`${pendingTickets?.pagination?.total} pending request(s) are still in progress`}
                    style={{ marginBottom: 24, borderRadius: 8 }}
                />
            )}

            <div className="dashboard-page__setup-shell">
                <SetupGuideCard
                    variant="dashboard"
                    focusAction={setupAction}
                    snapshot={{
                        systemsTotal: systems?.pagination?.total ?? 0,
                        vmsTotal: vms?.pagination?.total ?? 0,
                    }}
                />
            </div>
        </div>
    );
}
