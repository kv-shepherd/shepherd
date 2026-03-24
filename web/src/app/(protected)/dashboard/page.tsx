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
    HealthOverviewGlyph,
    RequestsOverviewGlyph,
    SystemsOverviewGlyph,
    VirtualMachinesOverviewGlyph,
} from '@/components/illustrations/DashboardIllustrations';
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
            icon: <SystemsOverviewGlyph className="dashboard-stat__icon-art" />,
            color: '#E6F4FF',
            iconColor: '#1D5BFF',
        },
        {
            title: t('nav.vms'),
            value: vms?.pagination?.total ?? 0,
            icon: <VirtualMachinesOverviewGlyph className="dashboard-stat__icon-art" />,
            color: '#F6ECFF',
            iconColor: '#6D4DE3',
        },
        {
            title: t('nav.my_requests'),
            value: pendingTickets?.pagination?.total ?? 0,
            icon: <RequestsOverviewGlyph className="dashboard-stat__icon-art" />,
            color: '#FFF1E8',
            iconColor: '#D66A1F',
            suffix: t('approval:status.PENDING'),
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
        <div>
            <PageHeader
                title={t('nav.dashboard')}
                subtitle={t('dashboard.subtitle')}
            />

            {/* Health Status Banner */}
            <PageSurface style={{ marginBottom: 24 }} styles={{ body: { padding: '16px 24px' } }}>
                <div className="dashboard-health">
                    <div className="dashboard-health__meta">
                        <Badge
                            status={healthStatus.status === 'ok' ? 'success' : healthStatus.status === 'degraded' ? 'warning' : 'error'}
                        />
                        <div>
                            <Text strong>Platform Health</Text>
                            <br />
                            <Text type="secondary" style={{ fontSize: 12 }}>
                                Status: {String(healthStatus.status || 'unknown').toUpperCase()}
                                {health?.version && ` · v${health.version}`}
                                {typeof liveness?.status === 'string' && ` · Live: ${liveness.status.toUpperCase()}`}
                            </Text>
                        </div>
                    </div>
                    <div className="dashboard-health__visual" style={{ color: healthStatus.color }}>
                        <HealthOverviewGlyph className="dashboard-health__visual-art" />
                    </div>
                </div>
            </PageSurface>

            {/* Quick Stats */}
            <Row gutter={[16, 16]} style={{ marginBottom: 24 }}>
                {stats.map((stat) => (
                    <Col xs={24} sm={8} key={stat.title}>
                        <PageSurface styles={{ body: { padding: '20px 24px' } }}>
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
                                    title={stat.title}
                                    value={stat.value}
                                    suffix={stat.suffix}
                                />
                            </div>
                        </PageSurface>
                    </Col>
                ))}
            </Row>

            {/* Pending Approvals Alert */}
            {(pendingTickets?.pagination?.total ?? 0) > 0 && (
                <Alert
                    type="warning"
                    showIcon
                    message={`${pendingTickets?.pagination?.total} pending request(s) are still in progress`}
                    style={{ marginBottom: 24, borderRadius: 8 }}
                />
            )}

            <SetupGuideCard
                variant="dashboard"
                focusAction={setupAction}
                snapshot={{
                    systemsTotal: systems?.pagination?.total ?? 0,
                    vmsTotal: vms?.pagination?.total ?? 0,
                }}
            />
        </div>
    );
}
