'use client';

/**
 * Dashboard page — system overview with real API data.
 *
 * Fetches health status and aggregated statistics from backend.
 * Uses TanStack Query for caching and automatic refetch.
 */
import { useMemo } from 'react';
import {
    Row,
    Col,
    Card,
    Statistic,
    Typography,
    Badge,
    Space,
    Spin,
    Alert,
} from 'antd';
import {
    CloudServerOutlined,
    AppstoreOutlined,
    DesktopOutlined,
    AuditOutlined,
    CheckCircleOutlined,
    WarningOutlined,
} from '@ant-design/icons';
import { useTranslation } from 'react-i18next';
import { useApiGet } from '@/hooks/useApiQuery';
import { api } from '@/lib/api/client';
import type { components } from '@/types/api.gen';

const { Title, Text } = Typography;

type SystemList = components['schemas']['SystemList'];
type VMList = components['schemas']['VMList'];
type ApprovalTicketList = components['schemas']['ApprovalTicketList'];
type Health = components['schemas']['Health'];

const HEALTH_STATUS_MAP: Record<string, { color: string; icon: React.ReactNode }> = {
    ok: { color: '#52c41a', icon: <CheckCircleOutlined /> },
    degraded: { color: '#faad14', icon: <WarningOutlined /> },
    error: { color: '#ff4d4f', icon: <WarningOutlined /> },
};

export default function DashboardPage() {
    const { t } = useTranslation('common');

    // Fetch health status
    const { data: health, isLoading: healthLoading } = useApiGet<Health>(
        ['health'],
        () => api.GET('/health/ready'),
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

    const { data: pendingApprovals, isLoading: approvalsLoading } = useApiGet<ApprovalTicketList>(
        ['approvals', 'dashboard'],
        () => api.GET('/approvals', { params: { query: { status: 'PENDING', per_page: 1 } } })
    );

    const isLoading = healthLoading || systemsLoading || vmsLoading || approvalsLoading;

    const healthStatus = useMemo(() => {
        if (!health) return { status: 'unknown', color: '#d9d9d9', icon: <WarningOutlined /> };
        const mapped = HEALTH_STATUS_MAP[health.status] ?? HEALTH_STATUS_MAP.error;
        return { status: health.status, ...mapped };
    }, [health]);

    const stats = useMemo(() => [
        {
            title: t('nav.systems'),
            value: systems?.pagination?.total ?? 0,
            icon: <AppstoreOutlined style={{ fontSize: 24, color: '#1677ff' }} />,
            color: '#e6f4ff',
        },
        {
            title: t('nav.vms'),
            value: vms?.pagination?.total ?? 0,
            icon: <DesktopOutlined style={{ fontSize: 24, color: '#531dab' }} />,
            color: '#f9f0ff',
        },
        {
            title: t('nav.approvals'),
            value: pendingApprovals?.pagination?.total ?? 0,
            icon: <AuditOutlined style={{ fontSize: 24, color: '#d4380d' }} />,
            color: '#fff2e8',
            suffix: 'pending',
        },
    ], [t, systems, vms, pendingApprovals]);

    if (isLoading) {
        return (
            <div style={{ display: 'flex', justifyContent: 'center', alignItems: 'center', minHeight: 400 }}>
                <Spin size="large" />
            </div>
        );
    }

    return (
        <div>
            <div style={{ marginBottom: 24 }}>
                <Title level={4} style={{ margin: 0 }}>{t('nav.dashboard')}</Title>
                <Text type="secondary">System overview and statistics</Text>
            </div>

            {/* Health Status Banner */}
            <Card
                style={{ marginBottom: 24, borderRadius: 12 }}
                styles={{ body: { padding: '16px 24px' } }}
            >
                <Space size="middle" align="center">
                    <Badge
                        status={healthStatus.status === 'ok' ? 'success' : healthStatus.status === 'degraded' ? 'warning' : 'error'}
                    />
                    <CloudServerOutlined style={{ fontSize: 20, color: healthStatus.color }} />
                    <div>
                        <Text strong>Platform Health</Text>
                        <br />
                        <Text type="secondary" style={{ fontSize: 12 }}>
                            Status: {healthStatus.status.toUpperCase()}
                            {health?.version && ` · v${health.version}`}
                        </Text>
                    </div>
                </Space>
            </Card>

            {/* Quick Stats */}
            <Row gutter={[16, 16]} style={{ marginBottom: 24 }}>
                {stats.map((stat) => (
                    <Col xs={24} sm={8} key={stat.title}>
                        <Card
                            style={{ borderRadius: 12 }}
                            styles={{ body: { padding: '20px 24px' } }}
                        >
                            <Space size="middle">
                                <div style={{
                                    width: 48,
                                    height: 48,
                                    borderRadius: 12,
                                    background: stat.color,
                                    display: 'flex',
                                    alignItems: 'center',
                                    justifyContent: 'center',
                                }}>
                                    {stat.icon}
                                </div>
                                <Statistic
                                    title={stat.title}
                                    value={stat.value}
                                    suffix={stat.suffix}
                                />
                            </Space>
                        </Card>
                    </Col>
                ))}
            </Row>

            {/* Pending Approvals Alert */}
            {(pendingApprovals?.pagination?.total ?? 0) > 0 && (
                <Alert
                    type="warning"
                    showIcon
                    message={`${pendingApprovals?.pagination?.total} pending approval(s) require attention`}
                    style={{ marginBottom: 24, borderRadius: 8 }}
                />
            )}
        </div>
    );
}
