'use client';

import { useState } from 'react';
import {
    Table,
    Button,
    Typography,
    Tag,
    Input,
    Select,
    Card,
    Row,
    Col,
    Badge,
    Popover,
    Descriptions,
    Space,
} from 'antd';
import type { DescriptionsProps } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import {
    ReloadOutlined,
    SearchOutlined,
    FileTextOutlined,
} from '@ant-design/icons';
import { useTranslation } from 'react-i18next';
import dayjs from 'dayjs';

import { useApiGet } from '@/hooks/useApiQuery';
import { api } from '@/lib/api/client';
import type { components } from '@/types/api.gen';

import { buildAuditLogQuery, type AuditLogFilters } from '../query';

const { Title, Text } = Typography;

type AuditLog = components['schemas']['AuditLog'];
type AuditLogList = components['schemas']['AuditLogList'];

type PlacementEvaluationSummary = {
    selectedClusterName?: string;
    selectedClusterId?: string;
    eligible?: boolean;
    reasonCode?: string;
    advisoryCode?: string;
};

const ACTION_COLORS: Record<string, string> = {
    CREATE: 'green',
    UPDATE: 'blue',
    DELETE: 'red',
    APPROVE: 'cyan',
    REJECT: 'orange',
    START: 'green',
    STOP: 'gold',
    RESTART: 'purple',
    LOGIN: 'geekblue',
};

function normalizeActionKey(action?: string): string {
    return (action ?? '').trim().toLowerCase().replace(/[.\s-]+/g, '_');
}

function actionSuffix(action?: string): string {
    const normalized = normalizeActionKey(action);
    const tokens = normalized.split('_').filter(Boolean);
    return tokens.at(-1) ?? normalized;
}

function asRecord(value: unknown): Record<string, unknown> | undefined {
    if (!value || typeof value !== 'object' || Array.isArray(value)) {
        return undefined;
    }
    return value as Record<string, unknown>;
}

function readStringField(record: Record<string, unknown> | undefined, key: string): string | undefined {
    if (!record) {
        return undefined;
    }
    const value = record[key];
    if (typeof value !== 'string') {
        return undefined;
    }
    const trimmed = value.trim();
    return trimmed || undefined;
}

function readBooleanField(record: Record<string, unknown> | undefined, key: string): boolean | undefined {
    if (!record) {
        return undefined;
    }
    const value = record[key];
    return typeof value === 'boolean' ? value : undefined;
}

function readApprovalDecision(details: Record<string, unknown> | undefined): string | undefined {
    return readStringField(details, 'decision');
}

function readPlacementEvaluation(details: Record<string, unknown> | undefined): PlacementEvaluationSummary | undefined {
    const placement = asRecord(details?.placement_evaluation);
    if (!placement) {
        return undefined;
    }
    return {
        selectedClusterName: readStringField(placement, 'selected_cluster_name'),
        selectedClusterId: readStringField(placement, 'selected_cluster_id'),
        eligible: readBooleanField(placement, 'eligible'),
        reasonCode: readStringField(placement, 'reason_code'),
        advisoryCode: readStringField(placement, 'advisory_code'),
    };
}

function placementStatusTagColor(summary: PlacementEvaluationSummary): string {
    if (summary.eligible === true) {
        return 'success';
    }
    if (summary.eligible === false) {
        return 'error';
    }
    return 'default';
}

export function AdminAuditContent() {
    const { t } = useTranslation(['admin', 'common']);
    const [page, setPage] = useState(1);
    const [pageSize, setPageSize] = useState(20);
    const [filters, setFilters] = useState<AuditLogFilters>({
        action: '',
        approval_decision: '',
        actor: '',
        placement_advisory_code: '',
        placement_reason_code: '',
        resource_type: '',
        resource_id: '',
    });
    const resourceTypeOptions = [
        { label: t('audit.resource_option.all'), value: '' },
        { label: t('audit.resource_option.vm'), value: 'vm' },
        { label: t('audit.resource_option.system'), value: 'system' },
        { label: t('audit.resource_option.service'), value: 'service' },
        { label: t('audit.resource_option.approval_ticket'), value: 'approval_ticket' },
        { label: t('audit.resource_option.cluster'), value: 'cluster' },
        { label: t('audit.resource_option.user'), value: 'user' },
        { label: t('audit.resource_option.namespace'), value: 'namespace' },
        { label: t('audit.resource_option.template'), value: 'template' },
        { label: t('audit.resource_option.instance_size'), value: 'instance_size' },
        { label: t('audit.resource_option.role'), value: 'role' },
        { label: t('audit.resource_option.auth_provider'), value: 'auth_provider' },
    ];
    const approvalDecisionOptions = [
        { label: t('audit.decision_option.all'), value: '' },
        { label: t('audit.decision_option.approved'), value: 'approved' },
        { label: t('audit.decision_option.rejected'), value: 'rejected' },
        { label: t('audit.decision_option.validation_failed'), value: 'validation_failed' },
        { label: t('audit.decision_option.power_approved'), value: 'power_approved' },
        { label: t('audit.decision_option.delete_approved'), value: 'delete_approved' },
        { label: t('audit.decision_option.vnc_access_approved'), value: 'vnc_access_approved' },
        { label: t('audit.decision_option.batch_approved'), value: 'batch_approved' },
        { label: t('audit.decision_option.batch_rejected'), value: 'batch_rejected' },
        { label: t('audit.decision_option.cancelled'), value: 'cancelled' },
        { label: t('audit.decision_option.batch_cancelled'), value: 'batch_cancelled' },
    ];

    const { data, isLoading, refetch } = useApiGet<AuditLogList>(
        ['audit-logs', page, pageSize, filters],
        () =>
            api.GET('/audit-logs', {
                params: {
                    query: buildAuditLogQuery(page, pageSize, filters),
                },
            })
    );

    const columns: ColumnsType<AuditLog> = [
        {
            title: t('audit.action'),
            dataIndex: 'action',
            key: 'action',
            width: 130,
            render: (action: string) => {
                const normalized = normalizeActionKey(action);
                const normalizedLabel = t(`audit.action_code.${normalized}`, { defaultValue: '' });
                const suffix = actionSuffix(action);
                const fallbackLabel = t(`audit.action_code.${suffix}`, { defaultValue: action?.toUpperCase() ?? '—' });
                const label = normalizedLabel || fallbackLabel;
                return (
                    <Tag color={ACTION_COLORS[suffix.toUpperCase()] ?? ACTION_COLORS[action?.toUpperCase()] ?? 'default'}>
                        {label}
                    </Tag>
                );
            },
        },
        {
            title: t('audit.decision', { defaultValue: 'Decision' }),
            dataIndex: 'details',
            key: 'decision',
            width: 160,
            render: (details: Record<string, unknown>) => {
                const decision = readApprovalDecision(details);
                if (!decision) {
                    return <Text type="secondary">—</Text>;
                }
                return <Tag color="blue">{decision}</Tag>;
            },
        },
        {
            title: t('audit.actor'),
            dataIndex: 'actor',
            key: 'actor',
            width: 150,
        },
        {
            title: t('audit.resource_type'),
            dataIndex: 'resource_type',
            key: 'resource_type',
            width: 130,
            render: (type: string) => (
                <Badge
                    status="processing"
                    text={t(`audit.resource_option.${(type ?? '').toLowerCase()}`, { defaultValue: type })}
                />
            ),
        },
        {
            title: t('audit.resource_id'),
            dataIndex: 'resource_id',
            key: 'resource_id',
            width: 150,
            render: (id: string) => (
                <Text copyable style={{ fontSize: 12 }}>{id?.slice(0, 8) ?? '—'}</Text>
            ),
        },
        {
            title: t('audit.placement', { defaultValue: 'Placement' }),
            dataIndex: 'details',
            key: 'placement',
            width: 280,
            render: (details: Record<string, unknown>) => {
                const summary = readPlacementEvaluation(details);
                if (!summary) {
                    return <Text type="secondary">—</Text>;
                }
                return (
                    <Space direction="vertical" size={2}>
                        <Space wrap size={4}>
                            <Tag color={placementStatusTagColor(summary)}>
                                {summary.eligible === true
                                    ? t('audit.placement.eligible', { defaultValue: 'Eligible' })
                                    : summary.eligible === false
                                        ? t('audit.placement.denied', { defaultValue: 'Denied' })
                                        : '—'}
                            </Tag>
                            {summary.reasonCode && (
                                <Tag color="red">{summary.reasonCode}</Tag>
                            )}
                            {summary.advisoryCode && (
                                <Tag color="orange">{summary.advisoryCode}</Tag>
                            )}
                        </Space>
                        {(summary.selectedClusterName || summary.selectedClusterId) && (
                            <Text type="secondary" style={{ fontSize: 12 }}>
                                {t('audit.placement.cluster', { defaultValue: 'Cluster' })}: {summary.selectedClusterName ?? summary.selectedClusterId}
                            </Text>
                        )}
                    </Space>
                );
            },
        },
        {
            title: t('audit.details'),
            dataIndex: 'details',
            key: 'details',
            ellipsis: true,
            render: (details: Record<string, unknown>) => {
                if (!details || Object.keys(details).length === 0) return <Text type="secondary">—</Text>;
                const decision = readApprovalDecision(details);
                const placement = readPlacementEvaluation(details);
                const descriptionItems: NonNullable<DescriptionsProps['items']> = [];
                if (decision) {
                    descriptionItems.push({
                        key: 'decision',
                        label: t('audit.decision', { defaultValue: 'Decision' }),
                        children: <Tag color="blue">{decision}</Tag>,
                    });
                }
                if (placement?.selectedClusterName || placement?.selectedClusterId) {
                    descriptionItems.push({
                        key: 'cluster',
                        label: t('audit.placement.cluster', { defaultValue: 'Cluster' }),
                        children: placement?.selectedClusterName ?? placement?.selectedClusterId,
                    });
                }
                if (placement?.eligible !== undefined) {
                    descriptionItems.push({
                        key: 'eligible',
                        label: t('audit.placement', { defaultValue: 'Placement' }),
                        children: (
                            <Tag color={placementStatusTagColor(placement)}>
                                {placement.eligible
                                    ? t('audit.placement.eligible', { defaultValue: 'Eligible' })
                                    : t('audit.placement.denied', { defaultValue: 'Denied' })}
                            </Tag>
                        ),
                    });
                }
                if (placement?.reasonCode) {
                    descriptionItems.push({
                        key: 'reason',
                        label: t('audit.placement.reason', { defaultValue: 'Reason Code' }),
                        children: <Tag color="red">{placement.reasonCode}</Tag>,
                    });
                }
                if (placement?.advisoryCode) {
                    descriptionItems.push({
                        key: 'advisory',
                        label: t('audit.placement.advisory', { defaultValue: 'Advisory Code' }),
                        children: <Tag color="orange">{placement.advisoryCode}</Tag>,
                    });
                }
                return (
                    <Popover
                        content={
                            <div style={{ maxWidth: 420 }}>
                                {descriptionItems.length > 0 && (
                                    <Descriptions
                                        size="small"
                                        column={1}
                                        items={descriptionItems}
                                        style={{ marginBottom: 12 }}
                                    />
                                )}
                                <Text type="secondary" style={{ fontSize: 12 }}>
                                    {t('audit.details.raw_json', { defaultValue: 'Raw JSON' })}
                                </Text>
                                <pre style={{ maxWidth: 400, maxHeight: 300, overflow: 'auto', fontSize: 12, marginTop: 8 }}>
                                    {JSON.stringify(details, null, 2)}
                                </pre>
                            </div>
                        }
                        title={t('audit.details')}
                        trigger="click"
                    >
                        <Text code style={{ cursor: 'pointer', fontSize: 12 }}>
                            {'{...}'}
                        </Text>
                    </Popover>
                );
            },
        },
        {
            title: t('audit.timestamp'),
            dataIndex: 'created_at',
            key: 'created_at',
            width: 170,
            render: (date: string) => (
                <Text type="secondary">{dayjs(date).format('YYYY-MM-DD HH:mm:ss')}</Text>
            ),
        },
    ];

    return (
        <div>
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 24 }}>
                <div>
                    <Title level={4} style={{ margin: 0 }}>
                        <FileTextOutlined style={{ marginRight: 8, color: '#1677ff' }} />
                        {t('audit.title')}
                    </Title>
                    <Text type="secondary">{t('audit.subtitle')}</Text>
                </div>
                <Button icon={<ReloadOutlined />} onClick={() => refetch()}>
                    {t('common:button.refresh')}
                </Button>
            </div>

            <Card style={{ marginBottom: 16, borderRadius: 12 }}>
                <Row gutter={16}>
                    <Col xs={24} sm={12} lg={6}>
                        <Select
                            style={{ width: '100%' }}
                            placeholder={t('audit.filter.resource_type')}
                            value={filters.resource_type || undefined}
                            onChange={(val) => setFilters((f) => ({ ...f, resource_type: val || '' }))}
                            options={resourceTypeOptions}
                            allowClear
                        />
                    </Col>
                    <Col xs={24} sm={12} lg={6}>
                        <Input
                            placeholder={t('audit.filter.action')}
                            value={filters.action}
                            onChange={(e) => setFilters((f) => ({ ...f, action: e.target.value }))}
                            prefix={<SearchOutlined />}
                            allowClear
                        />
                    </Col>
                    <Col xs={24} sm={12} lg={6}>
                        <Select
                            style={{ width: '100%' }}
                            placeholder={t('audit.filter.approval_decision')}
                            value={filters.approval_decision || undefined}
                            onChange={(val) => setFilters((f) => ({ ...f, approval_decision: val || '' }))}
                            options={approvalDecisionOptions}
                            allowClear
                        />
                    </Col>
                    <Col xs={24} sm={12} lg={6}>
                        <Input
                            placeholder={t('audit.filter.actor')}
                            value={filters.actor}
                            onChange={(e) => setFilters((f) => ({ ...f, actor: e.target.value }))}
                            prefix={<SearchOutlined />}
                            allowClear
                        />
                    </Col>
                    <Col xs={24} sm={12} lg={6}>
                        <Input
                            placeholder={t('audit.filter.placement_advisory_code')}
                            value={filters.placement_advisory_code}
                            onChange={(e) => setFilters((f) => ({ ...f, placement_advisory_code: e.target.value }))}
                            prefix={<SearchOutlined />}
                            allowClear
                        />
                    </Col>
                    <Col xs={24} sm={12} lg={6}>
                        <Input
                            placeholder={t('audit.filter.placement_reason_code')}
                            value={filters.placement_reason_code}
                            onChange={(e) => setFilters((f) => ({ ...f, placement_reason_code: e.target.value }))}
                            prefix={<SearchOutlined />}
                            allowClear
                        />
                    </Col>
                    <Col xs={24} sm={12} lg={6}>
                        <Button
                            type="primary"
                            icon={<SearchOutlined />}
                            onClick={() => { setPage(1); refetch(); }}
                            style={{ width: '100%' }}
                        >
                            {t('common:button.search')}
                        </Button>
                    </Col>
                </Row>
            </Card>

            <Card style={{ borderRadius: 12 }} styles={{ body: { padding: 0 } }}>
                <Table<AuditLog>
                    columns={columns}
                    dataSource={data?.items ?? []}
                    rowKey="id"
                    loading={isLoading}
                    pagination={{
                        current: page,
                        pageSize,
                        total: data?.pagination?.total ?? 0,
                        showTotal: (total) => t('common:table.total', { total }),
                        onChange: (p, ps) => { setPage(p); setPageSize(ps); },
                    }}
                    size="middle"
                    scroll={{ x: 'max-content' }}
                />
            </Card>
        </div>
    );
}
