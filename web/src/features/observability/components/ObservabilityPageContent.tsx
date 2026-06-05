'use client';

import { useMemo, useState, type ReactNode } from 'react';
import {
    ApiOutlined,
    AuditOutlined,
    CheckCircleOutlined,
    ClockCircleOutlined,
    DatabaseOutlined,
    LineChartOutlined,
    NodeIndexOutlined,
    ReloadOutlined,
    WarningOutlined,
} from '@ant-design/icons';
import {
    Alert,
    Badge,
    Button,
    Empty,
    Segmented,
    Space,
    Table,
    Tabs,
    Tag,
    Tooltip,
    Typography,
} from 'antd';
import type { ColumnsType } from 'antd/es/table';
import { useTranslation } from 'react-i18next';

import { PageHeader } from '@/components/layouts/PageSection';
import { LocalDateTimeText } from '@/components/ui/LocalDateTimeText';
import { useApiGet } from '@/hooks/useApiQuery';
import { api } from '@/lib/api/client';
import { isHostedDemoHost } from '@/lib/auth/demoEnvironment';
import { hasPermission } from '@/lib/auth/permissions';
import { translateI18nMessage } from '@/lib/i18n/messages';
import { useAuthStore } from '@/stores/auth';
import type { components } from '@/types/api.gen';

const { Text, Title } = Typography;

const TRACE_WINDOWS = [15, 60, 360] as const;

type TraceSummary = components['schemas']['AdminObservabilityTraceSummary'];
type TraceEndpoint = components['schemas']['AdminObservabilityTraceEndpoint'];
type TraceSample = components['schemas']['AdminObservabilityTraceSample'];
type SpanGroup = components['schemas']['AdminObservabilitySpanGroup'];
type AuditSignalSummary = components['schemas']['AdminObservabilityAuditSignalSummary'];
type AuditLogList = components['schemas']['AuditLogList'];
type AuditLog = components['schemas']['AuditLog'];
type TranslationFn = (key: string, options?: Record<string, unknown>) => string;
type Severity = 'good' | 'warning' | 'danger';

export function ObservabilityPageContent() {
    const { t } = useTranslation('common');
    const [traceWindowMinutes, setTraceWindowMinutes] = useState<number>(60);
    const currentUser = useAuthStore((state) => state.user);
    const canReadAuditLogs = hasPermission(currentUser, 'audit:read');
    const isDemoMode = useMemo(
        () => typeof window !== 'undefined' && isHostedDemoHost(window.location.hostname),
        [],
    );

    const traceQuery = useApiGet<TraceSummary>(
        ['admin-observability-traces', traceWindowMinutes],
        () => api.GET('/admin/observability/traces', {
            params: {
                query: {
                    lookback_minutes: traceWindowMinutes,
                    limit: 100,
                },
            },
        }),
        {
            staleTime: 30_000,
            refetchInterval: 60_000,
            retry: false,
            enabled: !isDemoMode,
        },
    );
    const auditSignalQuery = useApiGet<AuditSignalSummary>(
        ['admin-observability-audit-signals'],
        () => api.GET('/admin/observability/audit-signals', {}),
        {
            staleTime: 30_000,
            refetchInterval: 60_000,
            retry: false,
            enabled: !isDemoMode,
        },
    );
    const auditFeedQuery = useApiGet<AuditLogList>(
        ['admin-observability-audit-feed'],
        () => api.GET('/audit-logs', {
            params: {
                query: {
                    page: 1,
                    per_page: 8,
                    category: 'approvals',
                },
            },
        }),
        {
            staleTime: 30_000,
            refetchInterval: 60_000,
            retry: false,
            enabled: !isDemoMode && canReadAuditLogs,
        },
    );

    const traceData = useMemo<TraceSummary | undefined>(() => {
        if (!isDemoMode) {
            return traceQuery.data;
        }
        return {
            ...MOCK_TRACE_SUMMARY,
            window_seconds: traceWindowMinutes * 60,
        };
    }, [isDemoMode, traceQuery.data, traceWindowMinutes]);
    const auditSignalData = isDemoMode ? MOCK_AUDIT_SIGNAL_SUMMARY : auditSignalQuery.data;
    const auditLogs = isDemoMode ? MOCK_AUDIT_LOGS : canReadAuditLogs ? auditFeedQuery.data?.items ?? [] : [];
    const traceLoading = !isDemoMode && (traceQuery.isLoading || traceQuery.isFetching);
    const auditLoading = !isDemoMode && (auditSignalQuery.isLoading || auditSignalQuery.isFetching);
    const auditFeedLoading = !isDemoMode && canReadAuditLogs && (auditFeedQuery.isLoading || auditFeedQuery.isFetching);
    const traceHasError = !isDemoMode && Boolean(traceQuery.error);
    const auditHasError = !isDemoMode && Boolean(auditSignalQuery.error);
    const auditFeedHasError = !isDemoMode && canReadAuditLogs && Boolean(auditFeedQuery.error);

    const refreshAll = () => {
        if (!isDemoMode) {
            void traceQuery.refetch();
            void auditSignalQuery.refetch();
            if (canReadAuditLogs) {
                void auditFeedQuery.refetch();
            }
        }
    };

    return (
        <div className="observability-page">
            <PageHeader
                title={t('observability.title')}
                subtitle={t('observability.subtitle')}
                actions={(
                    <Tooltip title={t('observability.refresh')}>
                        <Button
                            className="app-shell-icon-action"
                            aria-label={t('observability.refresh')}
                            icon={<ReloadOutlined />}
                            onClick={refreshAll}
                            />
                        </Tooltip>
                    )}
                />
            <Tabs
                className="observability-tabs"
                defaultActiveKey="traces"
                items={[
                    {
                        key: 'traces',
                        label: (
                            <span className="observability-tab-label">
                                <ApiOutlined />
                                {t('observability.tabs.traces')}
                            </span>
                        ),
                        children: (
                            <TraceLayer
                                data={traceData}
                                loading={traceLoading}
                                hasError={traceHasError}
                                windowMinutes={traceWindowMinutes}
                                onWindowChange={setTraceWindowMinutes}
                            />
                        ),
                    },
                    {
                        key: 'audit',
                        label: (
                            <span className="observability-tab-label">
                                <AuditOutlined />
                                {t('observability.tabs.audit')}
                            </span>
                        ),
                        children: (
                            <AuditLayer
                                data={auditSignalData}
                                logs={auditLogs}
                                loading={auditLoading}
                                feedLoading={auditFeedLoading}
                                hasError={auditHasError}
                                feedHasError={auditFeedHasError}
                            />
                        ),
                    },
                    {
                        key: 'metrics',
                        label: (
                            <span className="observability-tab-label">
                                <LineChartOutlined />
                                {t('observability.tabs.metrics')}
                            </span>
                        ),
                        children: (
                            <MetricsLayer
                                traceData={traceData}
                                auditData={auditSignalData}
                                traceLoading={traceLoading}
                                auditLoading={auditLoading}
                                traceHasError={traceHasError}
                                auditHasError={auditHasError}
                            />
                        ),
                    },
                ]}
            />
        </div>
    );
}

const MOCK_TRACE_SUMMARY: TraceSummary = {
    generated_at: '2026-06-04T08:00:00Z',
    source: 'tempo',
    status: 'ok',
    window_seconds: 3600,
    endpoints: [
        {
            route: 'GET /api/v1/admin/observability/traces',
            request_count: 842,
            error_count: 0,
            error_rate: 0,
            p95_ms: 146.8,
            avg_ms: 62.4,
            max_ms: 311.7,
            slowest_trace_id: '8b8e2f57b4a2c18f96c4d6ef2f27a120',
        },
        {
            route: 'POST /api/v1/approval-tickets/:id/decisions',
            request_count: 214,
            error_count: 3,
            error_rate: 0.014,
            p95_ms: 688.1,
            avg_ms: 184.2,
            max_ms: 1470.3,
            slowest_trace_id: 'f234c0b9ad4f83f1a3cc4377f8d9e2aa',
        },
        {
            route: 'GET /api/v1/audit-logs',
            request_count: 1276,
            error_count: 0,
            error_rate: 0,
            p95_ms: 92.3,
            avg_ms: 38.7,
            max_ms: 244.6,
            slowest_trace_id: '1d0f1ec2f81f7f04b7cb14f7f07aeb2c',
        },
        {
            route: 'POST /api/v1/vms',
            request_count: 156,
            error_count: 5,
            error_rate: 0.0321,
            p95_ms: 1124.6,
            avg_ms: 421.5,
            max_ms: 2380.4,
            slowest_trace_id: '4edb94d42307ac19bd4bcd5c3f7a4e81',
        },
    ],
    slow_traces: [
        {
            trace_id: '4edb94d42307ac19bd4bcd5c3f7a4e81',
            root_name: 'POST /api/v1/vms',
            route: 'POST /api/v1/vms',
            duration_ms: 2380.4,
            status_code: 202,
            error: false,
            started_at: '2026-06-04T07:58:18Z',
        },
        {
            trace_id: 'f234c0b9ad4f83f1a3cc4377f8d9e2aa',
            root_name: 'POST /api/v1/approval-tickets/:id/decisions',
            route: 'POST /api/v1/approval-tickets/:id/decisions',
            duration_ms: 1470.3,
            status_code: 500,
            error: true,
            started_at: '2026-06-04T07:52:41Z',
        },
    ],
    dependencies: [
        {
            category: 'kubevirt',
            name: 'kubevirt.virtualmachine.create',
            span_count: 156,
            error_count: 2,
            p95_ms: 952.2,
            max_ms: 2140.1,
        },
        {
            category: 'database',
            name: 'postgres.approval_tickets.update',
            span_count: 428,
            error_count: 0,
            p95_ms: 181.5,
            max_ms: 430.2,
        },
        {
            category: 'provider',
            name: 'provider.namespace.resolve',
            span_count: 312,
            error_count: 0,
            p95_ms: 96.4,
            max_ms: 211.8,
        },
        {
            category: 'business',
            name: 'approval.decision.apply',
            span_count: 214,
            error_count: 3,
            p95_ms: 512.7,
            max_ms: 1390.6,
        },
    ],
};

const MOCK_AUDIT_SIGNAL_SUMMARY: AuditSignalSummary = {
    generated_at: '2026-06-04T08:00:00Z',
    status: 'ok',
    window_seconds: 3600,
    approval_tickets: [
        { status: 'PENDING', operation_type: 'CREATE', count: 6 },
        { status: 'EXECUTING', operation_type: 'POWER', count: 2 },
        { status: 'FAILED', operation_type: 'DELETE', count: 1 },
    ],
    approval_pending_ages: [
        { operation_type: 'CREATE', age_seconds: 7320 },
        { operation_type: 'POWER', age_seconds: 1840 },
    ],
    batch_approval_tickets: [
        { status: 'PENDING_APPROVAL', batch_type: 'BATCH_CREATE', count: 2 },
        { status: 'FAILED', batch_type: 'BATCH_POWER', count: 1 },
    ],
    batch_approval_pending_ages: [
        { batch_type: 'BATCH_CREATE', age_seconds: 5400 },
    ],
    batch_approval_failed_children: [
        { batch_type: 'BATCH_POWER', count: 3 },
    ],
    approval_audit_actions: [
        { action: 'approval.approved', count: 42 },
        { action: 'approval.validation_failed', count: 4 },
    ],
    approval_failure_audit_actions: [
        { action: 'approval.validation_failed', count: 4 },
        { action: 'approval.batch_rejected', count: 1 },
    ],
};

const MOCK_AUDIT_LOGS: AuditLog[] = [
    {
        id: 'audit-demo-001',
        action: 'approval.validation_failed',
        resource_type: 'approval_ticket',
        resource_id: 'ticket-demo-001',
        actor: 'ops.lead',
        actor_summary: {
            display_name: 'Operations Lead',
            secondary: 'ops.lead@example.com',
        },
        resource_summary: {
            display_name: 'checkout-api-01',
            secondary: 'payments / checkout',
            tertiary: 'prod-east',
        },
        ticket_summary: {
            system_id: 'system-payments',
            system_name: 'payments',
            service_id: 'service-checkout',
            service_name: 'checkout',
            namespace: 'prod-east',
            vm_name: 'checkout-api-01',
            requester_username: 'developer.one',
            requester_display_name: 'Developer One',
        },
        message_i18n: {
            key: 'audit.message.approval_validation_failed',
            params: {},
        },
        details: {},
        created_at: '2026-06-04T07:55:00Z',
    },
    {
        id: 'audit-demo-002',
        action: 'vm_batch_power_submit',
        resource_type: 'batch_approval_ticket',
        resource_id: 'batch-demo-042',
        actor: 'developer.two',
        actor_summary: {
            display_name: 'Developer Two',
            secondary: 'developer.two@example.com',
        },
        resource_summary: {
            display_name: 'Nightly power batch',
            secondary: 'payments / checkout',
            tertiary: '12 virtual machines',
        },
        message_i18n: {
            key: 'audit.message.vm_batch_power_submit',
            params: {},
        },
        details: {},
        created_at: '2026-06-04T07:48:00Z',
    },
    {
        id: 'audit-demo-003',
        action: 'approval.approved',
        resource_type: 'approval_ticket',
        resource_id: 'ticket-demo-003',
        actor: 'release.manager',
        actor_summary: {
            display_name: 'Release Manager',
            secondary: 'release.manager@example.com',
        },
        approval_decision: 'approved',
        resource_summary: {
            display_name: 'catalog-worker-02',
            secondary: 'platform / catalog',
            tertiary: 'staging',
        },
        ticket_summary: {
            system_id: 'system-platform',
            system_name: 'platform',
            service_id: 'service-catalog',
            service_name: 'catalog',
            namespace: 'staging',
            vm_name: 'catalog-worker-02',
            requester_username: 'developer.three',
            requester_display_name: 'Developer Three',
        },
        message_i18n: {
            key: 'audit.message.approval_approved',
            params: {},
        },
        details: {},
        created_at: '2026-06-04T07:42:00Z',
    },
];

function TraceLayer({
    data,
    hasError,
    loading,
    onWindowChange,
    windowMinutes,
}: {
    data?: TraceSummary;
    hasError: boolean;
    loading: boolean;
    windowMinutes: number;
    onWindowChange: (value: number) => void;
}) {
    const { t } = useTranslation('common');
    const totals = useMemo(() => summarizeTraceTotals(data), [data]);
    const endpoints = useMemo(
        () => [...(data?.endpoints ?? [])].sort(compareEndpointRisk),
        [data?.endpoints],
    );
    const dependencies = useMemo(
        () => [...(data?.dependencies ?? [])].sort(compareSpanGroupRisk),
        [data?.dependencies],
    );
    const slowTraces = useMemo(
        () => [...(data?.slow_traces ?? [])].sort((a, b) => b.duration_ms - a.duration_ms),
        [data?.slow_traces],
    );
    const endpointColumns = useTraceEndpointColumns();
    const dependencyColumns = useDependencyColumns();
    const slowTraceColumns = useSlowTraceColumns();

    return (
        <div className="observability-layer">
            {hasError ? (
                <Alert
                    type="warning"
                    showIcon
                    message={t('observability.traces.unavailable')}
                />
            ) : null}
            <div className="observability-layer-toolbar">
                <Segmented<number>
                    size="small"
                    value={windowMinutes}
                    options={TRACE_WINDOWS.map((minutes) => ({
                        label: t(`observability.window.${minutes}`),
                        value: minutes,
                    }))}
                    onChange={onWindowChange}
                />
                <TraceSnapshotMeta data={data} />
            </div>
            <MetricStrip
                items={[
                    {
                        label: t('observability.traces.total_requests'),
                        value: formatCount(totals.requests),
                    },
                    {
                        label: t('observability.traces.errors'),
                        value: formatCount(totals.errors),
                        tone: totals.errors > 0 ? 'danger' : 'normal',
                    },
                    {
                        label: t('observability.traces.p95'),
                        value: formatMs(totals.p95Ms),
                    },
                    {
                        label: t('observability.traces.slow_samples'),
                        value: formatCount(data?.slow_traces.length ?? 0),
                    },
                ]}
            />
            <TraceInsightGrid
                dependencies={dependencies.slice(0, 4)}
                slowTrace={slowTraces[0]}
            />
            <section className="observability-section">
                <SectionHeader
                    title={t('observability.traces.endpoint_title')}
                    count={endpoints.length}
                />
                <Table<TraceEndpoint>
                    rowKey={(record) => record.route}
                    columns={endpointColumns}
                    dataSource={endpoints}
                    loading={loading}
                    pagination={false}
                    size="small"
                    scroll={{ x: 1040 }}
                    locale={{ emptyText: <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={t('observability.empty')} /> }}
                />
            </section>
            <section className="observability-section">
                <SectionHeader
                    title={t('observability.traces.dependencies_title')}
                    count={dependencies.length}
                />
                <Table<SpanGroup>
                    rowKey={(record) => `${record.category}:${record.name}`}
                    columns={dependencyColumns}
                    dataSource={dependencies}
                    loading={loading}
                    pagination={false}
                    size="small"
                    scroll={{ x: 960 }}
                    locale={{ emptyText: <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={t('observability.empty')} /> }}
                />
            </section>
            <section className="observability-section">
                <SectionHeader
                    title={t('observability.traces.slow_title')}
                    count={slowTraces.length}
                />
                <Table<TraceSample>
                    rowKey={(record) => record.trace_id}
                    columns={slowTraceColumns}
                    dataSource={slowTraces}
                    loading={loading}
                    pagination={false}
                    size="small"
                    scroll={{ x: 760 }}
                    locale={{ emptyText: <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={t('observability.empty')} /> }}
                />
            </section>
        </div>
    );
}

function AuditLayer({
    data,
    feedHasError,
    feedLoading,
    hasError,
    loading,
    logs,
}: {
    data?: AuditSignalSummary;
    logs: AuditLog[];
    loading: boolean;
    feedLoading: boolean;
    hasError: boolean;
    feedHasError: boolean;
}) {
    const { t } = useTranslation('common');
    const totals = useMemo(() => summarizeAuditSignals(data), [data]);
    const pendingApprovalRows = useMemo(
        () => (data?.approval_tickets ?? []).filter((item) => isPendingStatus(item.status)),
        [data?.approval_tickets],
    );
    const failedApprovalRows = useMemo(
        () => (data?.approval_tickets ?? []).filter((item) => isFailureStatus(item.status)),
        [data?.approval_tickets],
    );
    const failedBatchRows = data?.batch_approval_failed_children ?? [];
    const failureActionRows = data?.approval_failure_audit_actions ?? [];
    const auditActionColumns = useAuditActionColumns();
    const approvalTicketColumns = useApprovalTicketColumns();
    const batchTicketColumns = useBatchTicketColumns();
    const logColumns = useAuditLogColumns();

    return (
        <div className="observability-layer">
            <div className="observability-layer-toolbar">
                <AuditSnapshotMeta data={data} />
            </div>
            {hasError ? (
                <Alert
                    type="warning"
                    showIcon
                    message={t('observability.audit.unavailable')}
                />
            ) : null}
            <MetricStrip
                items={[
                    {
                        label: t('observability.audit.pending_approvals'),
                        value: formatCount(totals.pendingApprovals),
                        tone: totals.pendingApprovals > 0 ? 'warning' : 'normal',
                    },
                    {
                        label: t('observability.audit.oldest_pending'),
                        value: formatDuration(totals.oldestPendingSeconds),
                        tone: totals.oldestPendingSeconds >= 3600 ? 'warning' : 'normal',
                    },
                    {
                        label: t('observability.audit.failed_children'),
                        value: formatCount(totals.failedChildren),
                        tone: totals.failedChildren > 0 ? 'danger' : 'normal',
                    },
                    {
                        label: t('observability.audit.failure_actions'),
                        value: formatCount(totals.failureActions),
                        tone: totals.failureActions > 0 ? 'danger' : 'normal',
                    },
                ]}
            />
            <AuditSignalBoard
                pendingApprovalRows={pendingApprovalRows}
                failedApprovalRows={failedApprovalRows}
                failedBatchRows={failedBatchRows}
                failureActionRows={failureActionRows}
            />
            <section className="observability-section observability-section-grid">
                <div>
                    <SectionHeader
                        title={t('observability.audit.approval_counts')}
                        count={data?.approval_tickets.length ?? 0}
                    />
                    <Table<AuditSignalSummary['approval_tickets'][number]>
                        rowKey={(record) => `${record.status}:${record.operation_type}`}
                        columns={approvalTicketColumns}
                        dataSource={data?.approval_tickets ?? []}
                        loading={loading}
                        pagination={false}
                        size="small"
                        locale={{ emptyText: <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={t('observability.empty')} /> }}
                    />
                </div>
                <div>
                    <SectionHeader
                        title={t('observability.audit.batch_counts')}
                        count={data?.batch_approval_tickets.length ?? 0}
                    />
                    <Table<AuditSignalSummary['batch_approval_tickets'][number]>
                        rowKey={(record) => `${record.status}:${record.batch_type}`}
                        columns={batchTicketColumns}
                        dataSource={data?.batch_approval_tickets ?? []}
                        loading={loading}
                        pagination={false}
                        size="small"
                        locale={{ emptyText: <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={t('observability.empty')} /> }}
                    />
                </div>
            </section>
            <section className="observability-section observability-section-grid">
                <div>
                    <SectionHeader
                        title={t('observability.audit.failure_actions_title')}
                        count={data?.approval_failure_audit_actions.length ?? 0}
                    />
                    <Table<AuditSignalSummary['approval_failure_audit_actions'][number]>
                        rowKey={(record) => record.action}
                        columns={auditActionColumns}
                        dataSource={data?.approval_failure_audit_actions ?? []}
                        loading={loading}
                        pagination={false}
                        size="small"
                        locale={{ emptyText: <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={t('observability.empty')} /> }}
                    />
                </div>
                <div>
                    <SectionHeader
                        title={t('observability.audit.recent_logs')}
                        count={logs.length}
                    />
                    {feedHasError ? (
                        <Alert
                            type="warning"
                            showIcon
                            message={t('observability.audit.feed_unavailable')}
                            className="observability-inline-alert"
                        />
                    ) : null}
                    <Table<AuditLog>
                        rowKey={(record) => record.id}
                        columns={logColumns}
                        dataSource={logs}
                        loading={feedLoading}
                        pagination={false}
                        size="small"
                        scroll={{ x: 760 }}
                        locale={{ emptyText: <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={t('observability.empty')} /> }}
                    />
                </div>
            </section>
        </div>
    );
}

function MetricsLayer({
    auditData,
    auditHasError,
    auditLoading,
    traceData,
    traceHasError,
    traceLoading,
}: {
    traceData?: TraceSummary;
    auditData?: AuditSignalSummary;
    traceLoading: boolean;
    auditLoading: boolean;
    traceHasError: boolean;
    auditHasError: boolean;
}) {
    const { t } = useTranslation('common');
    const traceTotals = useMemo(() => summarizeTraceTotals(traceData), [traceData]);
    const endpointHealth = useMemo(() => summarizeEndpointHealth(traceData), [traceData]);
    const dependencyTotals = useMemo(() => summarizeDependencyTotals(traceData), [traceData]);
    const auditTotals = useMemo(() => summarizeAuditSignals(auditData), [auditData]);
    const loading = traceLoading || auditLoading;

    return (
        <div className="observability-layer">
            {traceHasError || auditHasError ? (
                <Alert
                    type="warning"
                    showIcon
                    message={t('observability.metrics.unavailable')}
                />
            ) : null}
            <div className="observability-layer-toolbar">
                <div className="observability-context-strip">
                    <ContextItem
                        label={t('observability.metrics.trace_updated')}
                        value={traceData?.generated_at ? <LocalDateTimeText value={traceData.generated_at} /> : t('observability.no_snapshot')}
                    />
                    <ContextItem
                        label={t('observability.metrics.audit_updated')}
                        value={auditData?.generated_at ? <LocalDateTimeText value={auditData.generated_at} /> : t('observability.no_snapshot')}
                    />
                </div>
            </div>
            <MetricStrip
                items={[
                    {
                        label: t('observability.metrics.api_requests'),
                        value: formatCount(traceTotals.requests),
                    },
                    {
                        label: t('observability.metrics.api_error_rate'),
                        value: formatRate(endpointHealth.errorRate),
                        tone: endpointHealth.errorRate > 0 ? 'danger' : 'normal',
                    },
                    {
                        label: t('observability.metrics.api_p95'),
                        value: formatMs(traceTotals.p95Ms),
                        tone: metricToneFromSeverity(latencySeverity(traceTotals.p95Ms)),
                    },
                    {
                        label: t('observability.metrics.approval_backlog'),
                        value: formatCount(auditTotals.pendingApprovals),
                        tone: auditTotals.pendingApprovals > 0 ? 'warning' : 'normal',
                    },
                    {
                        label: t('observability.metrics.failure_signals'),
                        value: formatCount(auditTotals.failedChildren + auditTotals.failureActions + traceTotals.errors),
                        tone: auditTotals.failedChildren + auditTotals.failureActions + traceTotals.errors > 0 ? 'danger' : 'normal',
                    },
                ]}
            />
            <div className="observability-insight-grid observability-insight-grid--ops">
                <InsightPanel
                    title={t('observability.metrics.api_health_title')}
                    count={endpointHealth.total}
                    icon={<CheckCircleOutlined />}
                >
                    <OpsSignalList
                        loading={loading}
                        emptyText={t('observability.empty')}
                        items={[
                            {
                                key: 'healthy-endpoints',
                                title: t('observability.metrics.healthy_endpoints'),
                                meta: t('observability.table.health'),
                                value: formatCount(endpointHealth.healthy),
                                tone: endpointHealth.risky > 0 ? 'warning' : 'good',
                            },
                            {
                                key: 'risky-endpoints',
                                title: t('observability.metrics.risky_endpoints'),
                                meta: t('observability.table.errors'),
                                value: formatCount(endpointHealth.risky),
                                tone: endpointHealth.risky > 0 ? 'danger' : 'good',
                            },
                            {
                                key: 'error-rate',
                                title: t('observability.metrics.api_error_rate'),
                                meta: t('observability.table.error_rate'),
                                value: formatRate(endpointHealth.errorRate),
                                tone: endpointHealth.errorRate > 0 ? 'danger' : 'good',
                            },
                        ]}
                    />
                </InsightPanel>
                <InsightPanel
                    title={t('observability.metrics.dependency_title')}
                    count={dependencyTotals.count}
                    icon={<DatabaseOutlined />}
                >
                    <OpsSignalList
                        loading={loading}
                        emptyText={t('observability.empty')}
                        items={[
                            {
                                key: 'dependency-spans',
                                title: t('observability.metrics.dependency_spans'),
                                meta: t('observability.table.spans'),
                                value: formatCount(dependencyTotals.spans),
                                tone: 'good',
                            },
                            {
                                key: 'dependency-p95',
                                title: t('observability.metrics.dependency_p95'),
                                meta: t('observability.table.p95'),
                                value: formatMs(dependencyTotals.p95Ms),
                                tone: latencySeverity(dependencyTotals.p95Ms),
                            },
                            {
                                key: 'dependency-errors',
                                title: t('observability.metrics.dependency_errors'),
                                meta: t('observability.table.errors'),
                                value: formatCount(dependencyTotals.errors),
                                tone: dependencyTotals.errors > 0 ? 'danger' : 'good',
                            },
                        ]}
                    />
                </InsightPanel>
                <InsightPanel
                    title={t('observability.metrics.business_title')}
                    count={auditTotals.pendingApprovals + auditTotals.failedChildren + auditTotals.failureActions}
                    icon={<NodeIndexOutlined />}
                >
                    <OpsSignalList
                        loading={loading}
                        emptyText={t('observability.empty')}
                        items={[
                            {
                                key: 'approval-backlog',
                                title: t('observability.metrics.pending_approvals'),
                                meta: t('observability.audit.pending_queue_title'),
                                value: formatCount(auditTotals.pendingApprovals),
                                tone: auditTotals.pendingApprovals > 0 ? 'warning' : 'good',
                            },
                            {
                                key: 'failed-children',
                                title: t('observability.metrics.failed_children'),
                                meta: t('observability.audit.batch_failure_title'),
                                value: formatCount(auditTotals.failedChildren),
                                tone: auditTotals.failedChildren > 0 ? 'danger' : 'good',
                            },
                            {
                                key: 'failure-actions',
                                title: t('observability.metrics.failure_actions'),
                                meta: t('observability.audit.recent_failure_title'),
                                value: formatCount(auditTotals.failureActions),
                                tone: auditTotals.failureActions > 0 ? 'danger' : 'good',
                            },
                        ]}
                    />
                </InsightPanel>
            </div>
        </div>
    );
}

function TraceSnapshotMeta({ data }: { data?: TraceSummary }) {
    const { t } = useTranslation('common');
    return (
        <div className="observability-context-strip">
            <ContextItem
                label={t('observability.traces.source')}
                value={data?.source ? t(`observability.traces.source_${data.source}`, { defaultValue: data.source }) : t('observability.no_snapshot')}
            />
            <ContextItem
                label={t('observability.traces.window')}
                value={data ? formatDuration(data.window_seconds) : t('observability.no_snapshot')}
            />
            <ContextItem
                label={t('observability.traces.generated_at')}
                value={data?.generated_at ? <LocalDateTimeText value={data.generated_at} /> : t('observability.no_snapshot')}
            />
        </div>
    );
}

function AuditSnapshotMeta({ data }: { data?: AuditSignalSummary }) {
    const { t } = useTranslation('common');
    return (
        <div className="observability-context-strip">
            <ContextItem
                label={t('observability.audit.window')}
                value={data ? formatDuration(data.window_seconds) : t('observability.no_snapshot')}
            />
            <ContextItem
                label={t('observability.audit.generated_at')}
                value={data?.generated_at ? <LocalDateTimeText value={data.generated_at} /> : t('observability.no_snapshot')}
            />
        </div>
    );
}

function ContextItem({
    label,
    value,
}: {
    label: string;
    value: ReactNode;
}) {
    return (
        <span className="observability-context-item">
            <Text type="secondary">{label}</Text>
            <strong>{value}</strong>
        </span>
    );
}

function TraceInsightGrid({
    dependencies,
    slowTrace,
}: {
    dependencies: SpanGroup[];
    slowTrace?: TraceSample;
}) {
    const { t } = useTranslation('common');
    return (
        <div className="observability-insight-grid observability-insight-grid--trace">
            <InsightPanel
                title={t('observability.traces.dependency_hotspots')}
                count={dependencies.length}
                icon={<DatabaseOutlined />}
            >
                {dependencies.length > 0 ? (
                    <div className="observability-insight-list">
                        {dependencies.map((dependency) => (
                            <DependencyInsightItem key={`${dependency.category}:${dependency.name}`} dependency={dependency} />
                        ))}
                    </div>
                ) : (
                    <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={t('observability.traces.no_dependency')} />
                )}
            </InsightPanel>
            <InsightPanel
                title={t('observability.traces.slowest_call')}
                count={slowTrace ? 1 : 0}
                icon={<ClockCircleOutlined />}
            >
                {slowTrace ? (
                    <div className="observability-slow-call">
                        <EndpointRoute value={slowTrace.route} />
                        <div className="observability-slow-call__meta">
                            <LatencyPill value={slowTrace.duration_ms} />
                            <StatusCodeTag statusCode={slowTrace.status_code} error={slowTrace.error} />
                        </div>
                        <TraceId value={slowTrace.trace_id} />
                    </div>
                ) : (
                    <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={t('observability.empty')} />
                )}
            </InsightPanel>
        </div>
    );
}

function AuditSignalBoard({
    failedApprovalRows,
    failedBatchRows,
    failureActionRows,
    pendingApprovalRows,
}: {
    pendingApprovalRows: AuditSignalSummary['approval_tickets'];
    failedApprovalRows: AuditSignalSummary['approval_tickets'];
    failedBatchRows: AuditSignalSummary['batch_approval_failed_children'];
    failureActionRows: AuditSignalSummary['approval_failure_audit_actions'];
}) {
    const { t } = useTranslation(['common', 'admin', 'approval', 'vm']);
    return (
        <div className="observability-insight-grid observability-insight-grid--audit">
            <InsightPanel
                title={t('observability.audit.pending_queue_title')}
                count={pendingApprovalRows.length}
                icon={<ClockCircleOutlined />}
            >
                <AuditCountList
                    emptyText={t('observability.audit.no_pending_items')}
                    items={pendingApprovalRows.map((item) => ({
                        key: `${item.status}:${item.operation_type}`,
                        title: translateOperationType(t, item.operation_type),
                        meta: translateApprovalStatus(t, item.status),
                        count: item.count,
                        tone: 'warning',
                    }))}
                />
            </InsightPanel>
            <InsightPanel
                title={t('observability.audit.failed_queue_title')}
                count={failedApprovalRows.length}
                icon={<WarningOutlined />}
            >
                <AuditCountList
                    emptyText={t('observability.audit.no_failed_items')}
                    items={failedApprovalRows.map((item) => ({
                        key: `${item.status}:${item.operation_type}`,
                        title: translateOperationType(t, item.operation_type),
                        meta: translateApprovalStatus(t, item.status),
                        count: item.count,
                        tone: 'danger',
                    }))}
                />
            </InsightPanel>
            <InsightPanel
                title={t('observability.audit.batch_failure_title')}
                count={failedBatchRows.length}
                icon={<NodeIndexOutlined />}
            >
                <AuditCountList
                    emptyText={t('observability.audit.no_failed_children')}
                    items={failedBatchRows.map((item) => ({
                        key: item.batch_type,
                        title: translateBatchType(t, item.batch_type),
                        meta: t('observability.audit.failed_children'),
                        count: item.count,
                        tone: 'danger',
                    }))}
                />
            </InsightPanel>
            <InsightPanel
                title={t('observability.audit.recent_failure_title')}
                count={failureActionRows.length}
                icon={<AuditOutlined />}
            >
                <AuditCountList
                    emptyText={t('observability.audit.no_failure_actions')}
                    items={failureActionRows.map((item) => ({
                        key: item.action,
                        title: translateAuditAction(t, item.action),
                        meta: normalizeActionKey(item.action),
                        count: item.count,
                        tone: 'danger',
                    }))}
                />
            </InsightPanel>
        </div>
    );
}

function InsightPanel({
    children,
    count,
    icon,
    title,
}: {
    children: ReactNode;
    count: number;
    icon: ReactNode;
    title: string;
}) {
    return (
        <section className="observability-insight-panel">
            <div className="observability-insight-panel__header">
                <span className="observability-insight-panel__title">
                    {icon}
                    <Text strong>{title}</Text>
                </span>
                <Badge count={count} showZero color="#64748b" />
            </div>
            {children}
        </section>
    );
}

function DependencyInsightItem({ dependency }: { dependency: SpanGroup }) {
    const { t } = useTranslation('common');
    const max = Math.max(dependency.max_ms, dependency.p95_ms, 1);
    return (
        <div className={`observability-insight-item observability-insight-item--${spanGroupSeverity(dependency)}`}>
            <div className="observability-insight-main">
                <Space size={6} wrap className="observability-dependency-heading">
                    <CategoryTag value={dependency.category} />
                    <Text className="observability-dependency-name">{dependency.name}</Text>
                </Space>
                <LatencyPill value={dependency.p95_ms} />
            </div>
            <div className="observability-dependency-stats">
                <StatPair label={t('observability.table.p95')} value={formatMs(dependency.p95_ms)} />
                <StatPair label={t('observability.table.max')} value={formatMs(dependency.max_ms)} />
                <StatPair label={t('observability.table.spans')} value={formatCount(dependency.span_count)} />
                <StatPair label={t('observability.table.errors')} value={formatCount(dependency.error_count)} tone={dependency.error_count > 0 ? 'danger' : 'normal'} />
            </div>
            <LatencyBar value={dependency.p95_ms} max={max} />
        </div>
    );
}

function StatPair({
    label,
    tone = 'normal',
    value,
}: {
    label: string;
    value: string;
    tone?: 'normal' | 'danger';
}) {
    return (
        <span className={`observability-stat-pair observability-stat-pair--${tone}`}>
            <Text type="secondary">{label}</Text>
            <strong>{value}</strong>
        </span>
    );
}

function AuditCountList({
    emptyText,
    items,
}: {
    emptyText: string;
    items: Array<{ key: string; title: string; meta: string; count: number; tone: Severity }>;
}) {
    if (items.length === 0) {
        return <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={emptyText} />;
    }
    return (
        <div className="observability-insight-list">
            {items.map((item) => (
                <div key={item.key} className={`observability-audit-count observability-audit-count--${item.tone}`}>
                    <span>
                        <Text strong>{item.title}</Text>
                        <Text type="secondary">{item.meta}</Text>
                    </span>
                    <strong>{formatCount(item.count)}</strong>
                </div>
            ))}
        </div>
    );
}

function OpsSignalList({
    emptyText,
    items,
    loading,
}: {
    emptyText: string;
    items: Array<{ key: string; title: string; meta: string; value: string; tone: Severity }>;
    loading: boolean;
}) {
    if (loading) {
        return (
            <div className="observability-insight-list">
                {items.map((item) => (
                    <div key={item.key} className="observability-audit-count">
                        <span>
                            <Text strong>{item.title}</Text>
                            <Text type="secondary">{item.meta}</Text>
                        </span>
                        <Text type="secondary">{formatCount(0)}</Text>
                    </div>
                ))}
            </div>
        );
    }
    if (items.length === 0) {
        return <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={emptyText} />;
    }
    return (
        <div className="observability-insight-list">
            {items.map((item) => (
                <div key={item.key} className={`observability-audit-count observability-audit-count--${item.tone}`}>
                    <span>
                        <Text strong>{item.title}</Text>
                        <Text type="secondary">{item.meta}</Text>
                    </span>
                    <strong>{item.value}</strong>
                </div>
            ))}
        </div>
    );
}

function MetricStrip({
    items,
}: {
    items: Array<{ label: string; value: string; tone?: 'normal' | 'warning' | 'danger' }>;
}) {
    return (
        <div className="observability-metric-strip">
            {items.map((item) => (
                <div
                    key={item.label}
                    className={`observability-metric observability-metric--${item.tone ?? 'normal'}`}
                >
                    <Text type="secondary">{item.label}</Text>
                    <strong>{item.value}</strong>
                </div>
            ))}
        </div>
    );
}

function SectionHeader({ count, title }: { count: number; title: string }) {
    return (
        <div className="observability-section-header">
            <Title level={5}>{title}</Title>
            <Badge count={count} showZero color="#64748b" />
        </div>
    );
}

function useTraceEndpointColumns(): ColumnsType<TraceEndpoint> {
    const { t } = useTranslation('common');
    return useMemo(() => [
        {
            title: t('observability.table.health'),
            key: 'health',
            width: 130,
            render: (_, record) => <EndpointHealth endpoint={record} />,
        },
        {
            title: t('observability.table.route'),
            dataIndex: 'route',
            key: 'route',
            width: 320,
            render: (value: string) => <EndpointRoute value={value} />,
        },
        {
            title: t('observability.table.requests'),
            dataIndex: 'request_count',
            key: 'request_count',
            width: 110,
            align: 'right',
            render: (value: number) => formatCount(value),
        },
        {
            title: t('observability.table.errors'),
            dataIndex: 'error_count',
            key: 'error_count',
            width: 100,
            align: 'right',
            render: (value: number) => <ErrorCount value={value} />,
        },
        {
            title: t('observability.table.error_rate'),
            dataIndex: 'error_rate',
            key: 'error_rate',
            width: 110,
            align: 'right',
            render: (value: number) => formatRate(value),
        },
        {
            title: t('observability.table.p95'),
            dataIndex: 'p95_ms',
            key: 'p95_ms',
            width: 110,
            align: 'right',
            render: (value: number) => <LatencyPill value={value} />,
        },
        {
            title: t('observability.table.avg'),
            dataIndex: 'avg_ms',
            key: 'avg_ms',
            width: 110,
            align: 'right',
            render: (value: number) => formatMs(value),
        },
        {
            title: t('observability.table.max'),
            dataIndex: 'max_ms',
            key: 'max_ms',
            width: 110,
            align: 'right',
            render: (value: number) => formatMs(value),
        },
        {
            title: t('observability.table.slowest_trace'),
            dataIndex: 'slowest_trace_id',
            key: 'slowest_trace_id',
            width: 220,
            render: (value: string) => <TraceId value={value} />,
        },
    ], [t]);
}

function useDependencyColumns(): ColumnsType<SpanGroup> {
    const { t } = useTranslation('common');
    return useMemo(() => [
        {
            title: t('observability.table.category'),
            dataIndex: 'category',
            key: 'category',
            width: 120,
            render: (value: SpanGroup['category']) => <CategoryTag value={value} />,
        },
        {
            title: t('observability.table.name'),
            dataIndex: 'name',
            key: 'name',
            width: 520,
            render: (value: string) => <Text className="observability-dependency-name">{value}</Text>,
        },
        {
            title: t('observability.table.spans'),
            dataIndex: 'span_count',
            key: 'span_count',
            width: 100,
            align: 'right',
            render: (value: number) => formatCount(value),
        },
        {
            title: t('observability.table.errors'),
            dataIndex: 'error_count',
            key: 'error_count',
            width: 100,
            align: 'right',
            render: (value: number) => <ErrorCount value={value} />,
        },
        {
            title: t('observability.table.p95'),
            dataIndex: 'p95_ms',
            key: 'p95_ms',
            width: 110,
            align: 'right',
            render: (value: number) => <LatencyPill value={value} />,
        },
        {
            title: t('observability.table.max'),
            dataIndex: 'max_ms',
            key: 'max_ms',
            width: 110,
            align: 'right',
            render: (value: number) => formatMs(value),
        },
    ], [t]);
}

function useSlowTraceColumns(): ColumnsType<TraceSample> {
    const { t } = useTranslation('common');
    return useMemo(() => [
        {
            title: t('observability.table.trace_id'),
            dataIndex: 'trace_id',
            key: 'trace_id',
            width: 220,
            render: (value: string) => <TraceId value={value} />,
        },
        {
            title: t('observability.table.route'),
            dataIndex: 'route',
            key: 'route',
            width: 240,
            render: (value: string) => <EndpointRoute value={value} />,
        },
        {
            title: t('observability.table.duration'),
            dataIndex: 'duration_ms',
            key: 'duration_ms',
            width: 110,
            align: 'right',
            render: (value: number) => <LatencyPill value={value} />,
        },
        {
            title: t('observability.table.status'),
            dataIndex: 'status_code',
            key: 'status_code',
            width: 100,
            render: (value: number, record) => <StatusCodeTag statusCode={value} error={record.error} />,
        },
        {
            title: t('observability.table.started_at'),
            dataIndex: 'started_at',
            key: 'started_at',
            width: 150,
            render: (value: string) => <LocalDateTimeText value={value} />,
        },
    ], [t]);
}

function useAuditActionColumns(): ColumnsType<AuditSignalSummary['approval_failure_audit_actions'][number]> {
    const { t } = useTranslation(['common', 'admin']);
    return useMemo(() => [
        {
            title: t('observability.table.action'),
            dataIndex: 'action',
            key: 'action',
            render: (value: string) => <TranslatedAuditAction value={value} />,
        },
        {
            title: t('observability.table.count'),
            dataIndex: 'count',
            key: 'count',
            width: 110,
            align: 'right',
            render: (value: number) => formatCount(value),
        },
    ], [t]);
}

function useApprovalTicketColumns(): ColumnsType<AuditSignalSummary['approval_tickets'][number]> {
    const { t } = useTranslation(['common', 'approval']);
    return useMemo(() => [
        {
            title: t('observability.table.status'),
            dataIndex: 'status',
            key: 'status',
            render: (value: string) => <ApprovalStatusTag value={value} />,
        },
        {
            title: t('observability.table.operation_type'),
            dataIndex: 'operation_type',
            key: 'operation_type',
            render: (value: string) => <TranslatedOperation value={value} />,
        },
        {
            title: t('observability.table.count'),
            dataIndex: 'count',
            key: 'count',
            align: 'right',
            render: (value: number) => formatCount(value),
        },
    ], [t]);
}

function useBatchTicketColumns(): ColumnsType<AuditSignalSummary['batch_approval_tickets'][number]> {
    const { t } = useTranslation(['common', 'approval', 'vm']);
    return useMemo(() => [
        {
            title: t('observability.table.status'),
            dataIndex: 'status',
            key: 'status',
            render: (value: string) => <BatchStatusTag value={value} />,
        },
        {
            title: t('observability.table.batch_type'),
            dataIndex: 'batch_type',
            key: 'batch_type',
            render: (value: string) => <TranslatedBatchType value={value} />,
        },
        {
            title: t('observability.table.count'),
            dataIndex: 'count',
            key: 'count',
            align: 'right',
            render: (value: number) => formatCount(value),
        },
    ], [t]);
}

function useAuditLogColumns(): ColumnsType<AuditLog> {
    const { t } = useTranslation(['common', 'admin']);
    return useMemo(() => [
        {
            title: t('observability.table.event'),
            key: 'event',
            width: 260,
            render: (_, record) => <AuditEventSummary record={record} />,
        },
        {
            title: t('observability.table.resource'),
            key: 'resource',
            width: 220,
            render: (_, record) => <AuditResourceCell record={record} />,
        },
        {
            title: t('observability.table.actor'),
            key: 'actor',
            width: 150,
            render: (_, record) => <AuditActorCell record={record} />,
        },
        {
            title: t('observability.table.created_at'),
            dataIndex: 'created_at',
            key: 'created_at',
            width: 150,
            render: (value: string) => <LocalDateTimeText value={value} />,
        },
    ], [t]);
}

function EndpointRoute({ value }: { value?: string }) {
    const route = splitRoute(value);
    if (!route.path) {
        return <Text type="secondary">-</Text>;
    }
    return (
        <span className="observability-route">
            {route.method ? <Tag color={methodColor(route.method)}>{route.method}</Tag> : null}
            <Text className="observability-route-path">{route.path}</Text>
        </span>
    );
}

function EndpointHealth({ endpoint }: { endpoint: TraceEndpoint }) {
    const { t } = useTranslation('common');
    const severity = endpointSeverity(endpoint);
    const colorBySeverity: Record<Severity, string> = {
        good: 'green',
        warning: 'gold',
        danger: 'red',
    };
    const iconBySeverity: Record<Severity, ReactNode> = {
        good: <CheckCircleOutlined />,
        warning: <ClockCircleOutlined />,
        danger: <WarningOutlined />,
    };
    return (
        <Tag icon={iconBySeverity[severity]} color={colorBySeverity[severity]}>
            {t(`observability.health.${severity}`)}
        </Tag>
    );
}

function LatencyPill({ value }: { value: number }) {
    const severity = latencySeverity(value);
    const colorBySeverity: Record<Severity, string> = {
        good: 'green',
        warning: 'gold',
        danger: 'red',
    };
    return <Tag color={colorBySeverity[severity]}>{formatMs(value)}</Tag>;
}

function LatencyBar({ max, value }: { value: number; max: number }) {
    const width = Math.max(4, Math.min(100, (value / Math.max(max, 1)) * 100));
    return (
        <span className="observability-latency-bar" aria-hidden="true">
            <span style={{ width: `${width}%` }} />
        </span>
    );
}

function StatusCodeTag({
    error,
    statusCode,
}: {
    statusCode: number;
    error?: boolean;
}) {
    const { t } = useTranslation('common');
    const color = error || statusCode >= 500 ? 'red' : statusCode >= 400 ? 'gold' : 'green';
    return (
        <Tag color={color}>
            {statusCode > 0 ? statusCode : t('observability.unknown')}
        </Tag>
    );
}

function ApprovalStatusTag({ value }: { value: string }) {
    const { t } = useTranslation(['approval', 'common']);
    return (
        <Tag color={approvalStatusColor(value)}>
            {translateApprovalStatus(t, value)}
        </Tag>
    );
}

function BatchStatusTag({ value }: { value: string }) {
    const { t } = useTranslation(['common', 'approval', 'vm']);
    return (
        <Tag color={approvalStatusColor(value)}>
            {translateBatchStatus(t, value)}
        </Tag>
    );
}

function TranslatedOperation({ value }: { value: string }) {
    const { t } = useTranslation(['approval', 'common']);
    return <Text>{translateOperationType(t, value)}</Text>;
}

function TranslatedBatchType({ value }: { value: string }) {
    const { t } = useTranslation(['common', 'approval']);
    return <Text>{translateBatchType(t, value)}</Text>;
}

function TranslatedAuditAction({ value }: { value: string }) {
    const { t } = useTranslation(['admin', 'common']);
    const label = translateAuditAction(t, value);
    return (
        <Space direction="vertical" size={0}>
            <Text>{label}</Text>
            <Text type="secondary" className="observability-secondary-code">
                {normalizeActionKey(value)}
            </Text>
        </Space>
    );
}

function AuditEventSummary({ record }: { record: AuditLog }) {
    const { t } = useTranslation(['admin', 'common']);
    const adminT = (key: string, options?: Record<string, unknown>) => t(key, { ...options, ns: 'admin' });
    const fallback = translateAuditAction(t, record.action);
    const title = translateI18nMessage(adminT, record.message_i18n, '') || fallback;
    return (
        <Space direction="vertical" size={0} className="observability-audit-event">
            <Text strong>{title}</Text>
            <Space size={6} wrap>
                <Tag>{translateAuditAction(t, record.action)}</Tag>
                {record.approval_decision ? (
                    <Tag color={approvalDecisionColor(record.approval_decision)}>
                        {translateAuditDecision(t, record.approval_decision)}
                    </Tag>
                ) : null}
            </Space>
        </Space>
    );
}

function AuditResourceCell({ record }: { record: AuditLog }) {
    const { t } = useTranslation(['admin', 'common']);
    const resource = auditResourceDisplay(record, t);
    return (
        <Space direction="vertical" size={0} className="observability-readable-cell">
            <Text strong>{resource.primary}</Text>
            {resource.secondary ? <Text type="secondary">{resource.secondary}</Text> : null}
            {resource.tertiary ? <Text type="secondary">{resource.tertiary}</Text> : null}
        </Space>
    );
}

function AuditActorCell({ record }: { record: AuditLog }) {
    const actor = auditActorDisplay(record);
    return (
        <Space direction="vertical" size={0} className="observability-readable-cell">
            <Text>{actor.primary}</Text>
            {actor.secondary ? <Text type="secondary">{actor.secondary}</Text> : null}
        </Space>
    );
}

function ErrorCount({ value }: { value: number }) {
    if (value <= 0) {
        return <Text>{formatCount(value)}</Text>;
    }
    return <Text type="danger">{formatCount(value)}</Text>;
}

function CategoryTag({ value }: { value: SpanGroup['category'] }) {
    const { t } = useTranslation('common');
    const colorByCategory: Record<string, string> = {
        business: 'blue',
        database: 'geekblue',
        kubevirt: 'cyan',
        provider: 'purple',
        worker: 'gold',
        internal: 'default',
    };
    return (
        <Tag color={colorByCategory[value] ?? 'default'}>
            {t(`observability.category.${value}`)}
        </Tag>
    );
}

function TraceId({ value }: { value?: string }) {
    if (!value) {
        return <Text type="secondary">-</Text>;
    }
    return (
        <Text code copyable className="observability-trace-id">
            {value}
        </Text>
    );
}

function summarizeTraceTotals(data?: TraceSummary) {
    const endpoints = data?.endpoints ?? [];
    return {
        requests: endpoints.reduce((sum, item) => sum + item.request_count, 0),
        errors: endpoints.reduce((sum, item) => sum + item.error_count, 0),
        p95Ms: Math.max(0, ...endpoints.map((item) => item.p95_ms)),
    };
}

function summarizeEndpointHealth(data?: TraceSummary) {
    const endpoints = data?.endpoints ?? [];
    const healthy = endpoints.filter((item) => endpointSeverity(item) === 'good').length;
    const requests = endpoints.reduce((sum, item) => sum + item.request_count, 0);
    const errors = endpoints.reduce((sum, item) => sum + item.error_count, 0);
    return {
        total: endpoints.length,
        healthy,
        risky: endpoints.length - healthy,
        errorRate: requests > 0 ? errors / requests : 0,
    };
}

function summarizeDependencyTotals(data?: TraceSummary) {
    const dependencies = data?.dependencies ?? [];
    return {
        count: dependencies.length,
        spans: dependencies.reduce((sum, item) => sum + item.span_count, 0),
        errors: dependencies.reduce((sum, item) => sum + item.error_count, 0),
        p95Ms: Math.max(0, ...dependencies.map((item) => item.p95_ms)),
    };
}

function compareEndpointRisk(a: TraceEndpoint, b: TraceEndpoint) {
    const riskA = a.error_count * 100_000 + a.error_rate * 10_000 + a.p95_ms + a.max_ms * 0.1;
    const riskB = b.error_count * 100_000 + b.error_rate * 10_000 + b.p95_ms + b.max_ms * 0.1;
    return riskB - riskA;
}

function compareSpanGroupRisk(a: SpanGroup, b: SpanGroup) {
    const riskA = a.error_count * 100_000 + a.p95_ms + a.max_ms * 0.1;
    const riskB = b.error_count * 100_000 + b.p95_ms + b.max_ms * 0.1;
    return riskB - riskA;
}

function endpointSeverity(endpoint: TraceEndpoint): Severity {
    if (endpoint.error_count > 0 || endpoint.error_rate >= 0.05 || endpoint.p95_ms >= 1000) {
        return 'danger';
    }
    if (endpoint.error_rate > 0 || endpoint.p95_ms >= 300) {
        return 'warning';
    }
    return 'good';
}

function spanGroupSeverity(span: SpanGroup): Severity {
    if (span.error_count > 0 || span.p95_ms >= 1000) {
        return 'danger';
    }
    if (span.p95_ms >= 300) {
        return 'warning';
    }
    return 'good';
}

function latencySeverity(value: number): Severity {
    if (value >= 1000) {
        return 'danger';
    }
    if (value >= 300) {
        return 'warning';
    }
    return 'good';
}

function metricToneFromSeverity(severity: Severity): 'normal' | 'warning' | 'danger' {
    return severity === 'good' ? 'normal' : severity;
}

function summarizeAuditSignals(data?: AuditSignalSummary) {
    const approvalTickets = data?.approval_tickets ?? [];
    const pendingApprovals = approvalTickets
        .filter((item) => isPendingStatus(item.status))
        .reduce((sum, item) => sum + item.count, 0);
    const oldestApprovalAge = Math.max(0, ...(data?.approval_pending_ages ?? []).map((item) => item.age_seconds));
    const oldestBatchAge = Math.max(0, ...(data?.batch_approval_pending_ages ?? []).map((item) => item.age_seconds));
    const failedChildren = (data?.batch_approval_failed_children ?? []).reduce((sum, item) => sum + item.count, 0);
    const failureActions = (data?.approval_failure_audit_actions ?? []).reduce((sum, item) => sum + item.count, 0);
    return {
        pendingApprovals,
        oldestPendingSeconds: Math.max(oldestApprovalAge, oldestBatchAge),
        failedChildren,
        failureActions,
    };
}

function isPendingStatus(value?: string) {
    const normalized = normalizeStatusKey(value);
    return normalized === 'PENDING' || normalized === 'PENDING_APPROVAL' || normalized === 'EXECUTING';
}

function isFailureStatus(value?: string) {
    const normalized = normalizeStatusKey(value);
    return normalized === 'FAILED' || normalized === 'ERROR' || normalized === 'VALIDATION_FAILED';
}

function normalizeStatusKey(value?: string) {
    return (value ?? '').trim().toUpperCase().replace(/[.\s-]+/g, '_');
}

function normalizeActionKey(action?: string): string {
    return (action ?? '').trim().toLowerCase().replace(/[.\s-]+/g, '_');
}

function actionSuffix(action?: string): string {
    const normalized = normalizeActionKey(action);
    const tokens = normalized.split('_').filter(Boolean);
    return tokens.at(-1) ?? normalized;
}

function prettifyToken(value?: string): string {
    if (!value) {
        return '-';
    }
    return value
        .trim()
        .replace(/[._-]+/g, ' ')
        .replace(/\s+/g, ' ')
        .split(' ')
        .filter(Boolean)
        .map((token) => token.charAt(0).toUpperCase() + token.slice(1).toLowerCase())
        .join(' ');
}

function translateAuditAction(t: TranslationFn, action?: string): string {
    const normalized = normalizeActionKey(action);
    const normalizedLabel = t(`audit.action_code.${normalized}`, { ns: 'admin', defaultValue: '' });
    if (normalizedLabel) {
        return normalizedLabel;
    }
    const suffix = actionSuffix(action);
    const suffixLabel = t(`audit.action_code.${suffix}`, { ns: 'admin', defaultValue: '' });
    return suffixLabel || prettifyToken(action);
}

function translateAuditResourceType(t: TranslationFn, resourceType?: string): string {
    const normalized = (resourceType ?? '').trim().toLowerCase();
    return t(`audit.resource_option.${normalized}`, {
        ns: 'admin',
        defaultValue: prettifyToken(normalized),
    });
}

function auditResourceDisplay(record: AuditLog, t: TranslationFn) {
    const ticketPrimary = ticketResourcePrimary(record.ticket_summary);
    const summaryPrimary = record.resource_summary?.display_name?.trim();
    const primary = ticketPrimary || summaryPrimary || translateAuditResourceType(t, record.resource_type);
    const summaryParts = [
        record.resource_summary?.secondary?.trim(),
        record.resource_summary?.tertiary?.trim(),
    ].filter(Boolean);
    const ticketMeta = ticketResourceMeta(record.ticket_summary);
    const fallbackID = record.resource_id?.trim();
    return {
        primary,
        secondary: ticketMeta || summaryParts[0] || fallbackID || '',
        tertiary: summaryParts[1] && summaryParts[1] !== ticketMeta ? summaryParts[1] : '',
    };
}

function auditActorDisplay(record: AuditLog) {
    const summaryPrimary = record.actor_summary?.display_name?.trim();
    const summarySecondary = record.actor_summary?.secondary?.trim();
    const actor = record.actor?.trim() ?? '';
    return {
        primary: summaryPrimary || actor || '-',
        secondary: summarySecondary && summarySecondary !== summaryPrimary ? summarySecondary : '',
    };
}

function ticketResourcePrimary(summary?: AuditLog['ticket_summary']) {
    if (!summary) {
        return '';
    }
    return firstNonEmpty(
        summary.vm_name,
        joinNonEmpty(' / ', summary.system_name, summary.service_name),
        summary.service_name,
        summary.system_name,
        summary.namespace,
    );
}

function ticketResourceMeta(summary?: AuditLog['ticket_summary']) {
    if (!summary) {
        return '';
    }
    const scope = joinNonEmpty(' / ', summary.namespace, summary.cluster_name);
    const requester = firstNonEmpty(summary.requester_display_name, summary.requester_username);
    return firstNonEmpty(scope, requester);
}

function firstNonEmpty(...values: Array<string | undefined>) {
    return values.map((value) => value?.trim()).find(Boolean) ?? '';
}

function joinNonEmpty(separator: string, ...values: Array<string | undefined>) {
    return values
        .map((value) => value?.trim())
        .filter((value): value is string => Boolean(value))
        .join(separator);
}

function translateAuditDecision(t: TranslationFn, decision?: string): string {
    if (!decision) {
        return '';
    }
    return t(`audit.decision_option.${decision}`, {
        ns: 'admin',
        defaultValue: prettifyToken(decision),
    });
}

function translateApprovalStatus(t: TranslationFn, status?: string): string {
    const normalized = normalizeStatusKey(status);
    const label = t(`status.${normalized}`, { ns: 'approval', defaultValue: '' });
    return label || t(`observability.audit.status.${normalized}`, { ns: 'common', defaultValue: prettifyToken(status) });
}

function translateBatchStatus(t: TranslationFn, status?: string): string {
    const normalized = normalizeStatusKey(status);
    const observabilityLabel = t(`observability.audit.batch_status.${normalized}`, { ns: 'common', defaultValue: '' });
    if (observabilityLabel) {
        return observabilityLabel;
    }
    const approvalLabel = t(`status.${normalized}`, { ns: 'approval', defaultValue: '' });
    if (approvalLabel) {
        return approvalLabel;
    }
    return t(`batch.status_value.${normalized}`, { ns: 'vm', defaultValue: prettifyToken(status) });
}

function translateOperationType(t: TranslationFn, operationType?: string): string {
    const normalized = normalizeStatusKey(operationType);
    return t(`op_type.${normalized}`, {
        ns: 'approval',
        defaultValue: t(`observability.audit.operation_type.${normalized}`, {
            ns: 'common',
            defaultValue: prettifyToken(operationType),
        }),
    });
}

function translateBatchType(t: TranslationFn, batchType?: string): string {
    const normalized = normalizeStatusKey(batchType);
    const withoutPrefix = normalized.startsWith('BATCH_') ? normalized.slice('BATCH_'.length) : normalized;
    const approvalLabel = t(`op_type.${withoutPrefix}`, { ns: 'approval', defaultValue: '' });
    if (approvalLabel) {
        return approvalLabel;
    }
    return t(`observability.audit.batch_type.${normalized}`, {
        ns: 'common',
        defaultValue: prettifyToken(batchType),
    });
}

function approvalStatusColor(status?: string) {
    const normalized = normalizeStatusKey(status);
    if (normalized.includes('FAILED') || normalized.includes('REJECTED')) {
        return 'red';
    }
    if (normalized.includes('PENDING') || normalized === 'EXECUTING') {
        return 'gold';
    }
    if (normalized.includes('APPROVED') || normalized.includes('SUCCESS') || normalized.includes('COMPLETED')) {
        return 'green';
    }
    if (normalized.includes('CANCELLED')) {
        return 'default';
    }
    return 'blue';
}

function approvalDecisionColor(decision?: string) {
    const normalized = normalizeActionKey(decision);
    if (normalized.includes('rejected') || normalized.includes('failed')) {
        return 'red';
    }
    if (normalized.includes('cancelled')) {
        return 'default';
    }
    return 'green';
}

function splitRoute(value?: string) {
    const trimmed = (value ?? '').trim();
    const match = trimmed.match(/^([A-Z]+)\s+(.+)$/);
    if (!match) {
        return { method: '', path: trimmed };
    }
    return { method: match[1], path: match[2] };
}

function methodColor(method: string) {
    switch (method.toUpperCase()) {
    case 'GET':
        return 'blue';
    case 'POST':
        return 'green';
    case 'PUT':
    case 'PATCH':
        return 'gold';
    case 'DELETE':
        return 'red';
    default:
        return 'default';
    }
}

function formatCount(value: number) {
    return new Intl.NumberFormat(undefined, { maximumFractionDigits: 0 }).format(value);
}

function formatRate(value: number) {
    return new Intl.NumberFormat(undefined, {
        style: 'percent',
        maximumFractionDigits: 2,
    }).format(value);
}

function formatMs(value: number) {
    if (!Number.isFinite(value) || value <= 0) {
        return '0 ms';
    }
    if (value >= 1000) {
        return `${(value / 1000).toFixed(2)} s`;
    }
    return `${value.toFixed(value >= 10 ? 0 : 1)} ms`;
}

function formatDuration(seconds: number) {
    if (!Number.isFinite(seconds) || seconds <= 0) {
        return '0m';
    }
    if (seconds >= 86400) {
        return `${Math.floor(seconds / 86400)}d`;
    }
    if (seconds >= 3600) {
        return `${Math.floor(seconds / 3600)}h ${Math.floor((seconds % 3600) / 60)}m`;
    }
    return `${Math.floor(seconds / 60)}m`;
}
