'use client';

/**
 * Audit Log Viewer — admin page.
 *
 * OpenAPI: GET /audit-logs (listAuditLogs)
 * ADR-0019: Audit log with data masking (redaction compliance)
 * ADR-0021: Uses typed api client — token injection handled by middleware.
 */
import { useState } from 'react';
import {
    Table,
    Button,
    Space,
    Typography,
    Tag,
    Input,
    Select,
    Card,
    Row,
    Col,
    Badge,
} from 'antd';
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

const { Title, Text } = Typography;

type AuditLog = components['schemas']['AuditLog'];
type AuditLogList = components['schemas']['AuditLogList'];

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

const RESOURCE_TYPE_OPTIONS = [
    { label: 'All', value: '' },
    { label: 'VM', value: 'vm' },
    { label: 'System', value: 'system' },
    { label: 'Service', value: 'service' },
    { label: 'Approval', value: 'approval' },
    { label: 'Cluster', value: 'cluster' },
    { label: 'User', value: 'user' },
];

export default function AuditLogPage() {
    const { t } = useTranslation(['admin', 'common']);
    const [page, setPage] = useState(1);
    const [pageSize, setPageSize] = useState(20);
    const [filters, setFilters] = useState({
        action: '',
        actor: '',
        resource_type: '',
        resource_id: '',
    });

    // Fetch audit logs via typed api client (ADR-0021 contract-first).
    // JWT token is automatically attached by the api client middleware.
    const { data, isLoading, refetch } = useApiGet<AuditLogList>(
        ['audit-logs', page, pageSize, filters],
        () =>
            api.GET('/audit-logs', {
                params: {
                    query: {
                        page,
                        per_page: pageSize,
                        ...(filters.action ? { action: filters.action } : {}),
                        ...(filters.actor ? { actor: filters.actor } : {}),
                        ...(filters.resource_type ? { resource_type: filters.resource_type } : {}),
                        ...(filters.resource_id ? { resource_id: filters.resource_id } : {}),
                    },
                },
            })
    );

    const columns: ColumnsType<AuditLog> = [
        {
            title: t('audit.action'),
            dataIndex: 'action',
            key: 'action',
            width: 130,
            render: (action: string) => (
                <Tag color={ACTION_COLORS[action?.toUpperCase()] ?? 'default'}>
                    {action?.toUpperCase()}
                </Tag>
            ),
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
            render: (type: string) => <Badge status="processing" text={type} />,
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
            title: t('audit.details'),
            dataIndex: 'details',
            key: 'details',
            ellipsis: true,
            render: (details: Record<string, unknown>) => (
                <Text type="secondary" style={{ fontSize: 12 }}>
                    {details ? JSON.stringify(details).slice(0, 100) : '—'}
                </Text>
            ),
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

            {/* Filters */}
            <Card style={{ marginBottom: 16, borderRadius: 12 }}>
                <Row gutter={16}>
                    <Col xs={24} sm={6}>
                        <Select
                            style={{ width: '100%' }}
                            placeholder={t('audit.filter.resource_type')}
                            value={filters.resource_type || undefined}
                            onChange={(val) => setFilters((f) => ({ ...f, resource_type: val || '' }))}
                            options={RESOURCE_TYPE_OPTIONS}
                            allowClear
                        />
                    </Col>
                    <Col xs={24} sm={6}>
                        <Input
                            placeholder={t('audit.filter.action')}
                            value={filters.action}
                            onChange={(e) => setFilters((f) => ({ ...f, action: e.target.value }))}
                            prefix={<SearchOutlined />}
                            allowClear
                        />
                    </Col>
                    <Col xs={24} sm={6}>
                        <Input
                            placeholder={t('audit.filter.actor')}
                            value={filters.actor}
                            onChange={(e) => setFilters((f) => ({ ...f, actor: e.target.value }))}
                            prefix={<SearchOutlined />}
                            allowClear
                        />
                    </Col>
                    <Col xs={24} sm={6}>
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
                />
            </Card>
        </div>
    );
}
