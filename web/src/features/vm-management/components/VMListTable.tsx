'use client';

import {
    Badge,
    Button,
    Dropdown,
    Space,
    Table,
    Tag,
    Tooltip,
    Typography,
} from 'antd';
import {
    CopyOutlined,
    DeleteOutlined,
    DesktopOutlined,
    EyeOutlined,
    MoreOutlined,
    PauseCircleOutlined,
    PlayCircleOutlined,
    RedoOutlined,
    SettingOutlined,
} from '@ant-design/icons';
import dayjs from 'dayjs';
import type { MenuProps } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import type { TFunction } from 'i18next';

import type { VM, VMList } from '../types';
import { VM_STATUS_MAP } from '../types';
import { PageSurface } from '@/components/layouts/PageSection';

const { Text: TypographyText } = Typography;

interface VMListTableProps {
    t: TFunction;
    vmData: VMList | undefined;
    isLoading: boolean;
    page: number;
    pageSize: number;
    onPageChange: (page: number, pageSize: number) => void;
    onStart: (vmId: string) => void;
    onStop: (vmId: string) => void;
    onRestart: (vmId: string) => void;
    onConsole: (vmId: string) => void;
    onDelete: (vmId: string, vmName: string, environment?: string) => void;
    onModify: (vmId: string, vmName: string) => void;
    onRequestSimilar: (vmId: string) => void;
    onDetail: (vmId: string) => void;
    selectedRowKeys: string[];
    onSelectionChange: (selectedKeys: string[]) => void;
}

export function VMListTable({
    t,
    vmData,
    isLoading,
    page,
    pageSize,
    onPageChange,
    onStart,
    onStop,
    onRestart,
    onConsole,
    onDelete,
    onModify,
    onRequestSimilar,
    onDetail,
    selectedRowKeys,
    onSelectionChange,
}: VMListTableProps) {
    const actionLabel = (actionKey: string, vmName: string) => `${t(actionKey)} ${vmName}`;

    const columns: ColumnsType<VM> = [
        {
            title: t('field.name'),
            dataIndex: 'name',
            key: 'name',
            render: (name: string, record) => (
                <Space>
                    <DesktopOutlined style={{ color: '#531dab' }} />
                    <TypographyText
                        strong
                        style={{ cursor: 'pointer', color: '#1677ff' }}
                        data-testid={`vm-action-detail-${record.id}`}
                        onClick={() => onDetail(record.id)}
                    >
                        {name}
                    </TypographyText>
                </Space>
            ),
        },
        {
            title: t('common:table.status'),
            dataIndex: 'status',
            key: 'status',
            width: 130,
            render: (status: VM['status']) => {
                const mapped = VM_STATUS_MAP[status] ?? VM_STATUS_MAP.UNKNOWN;
                return (
                    <Badge status={mapped.badge} text={<Tag color={mapped.color}>{t(`status.${status}`)}</Tag>} />
                );
            },
        },
        {
            title: t('field.namespace'),
            dataIndex: 'namespace',
            key: 'namespace',
            width: 150,
            render: (namespace: string) => <Tag>{namespace}</Tag>,
        },
        {
            title: t('field.cluster'),
            dataIndex: 'cluster_name',
            key: 'cluster_name',
            width: 150,
            render: (clusterName: string, record) => (
                <Space direction="vertical" size={0}>
                    <TypographyText>{clusterName || record.cluster_id || '—'}</TypographyText>
                    {clusterName && record.cluster_id && clusterName !== record.cluster_id && (
                        <TypographyText type="secondary" style={{ fontSize: 12 }}>
                            {record.cluster_id}
                        </TypographyText>
                    )}
                </Space>
            ),
        },
        {
            title: t('field.hostname'),
            dataIndex: 'hostname',
            key: 'hostname',
            width: 180,
            render: (hostname: string) => <TypographyText type="secondary">{hostname || '—'}</TypographyText>,
        },
        {
            title: t('common:table.created_at'),
            dataIndex: 'created_at',
            key: 'created_at',
            width: 160,
            render: (date: string) => (
                <TypographyText type="secondary">{date ? dayjs(date).format('YYYY-MM-DD HH:mm') : '—'}</TypographyText>
            ),
        },
        {
            title: t('common:table.actions'),
            key: 'actions',
            width: 200,
            render: (_, record) => {
                const isRunning = record.status === 'RUNNING';
                const isStopped = record.status === 'STOPPED';
                const canDelete = isStopped
                    || record.status === 'FAILED'
                    || record.status === 'NOT_FOUND'
                    || record.status === 'UNKNOWN';
                const moreItems: MenuProps['items'] = [
                    {
                        key: 'details',
                        icon: <EyeOutlined />,
                        label: t('action.view_details'),
                        onClick: () => onDetail(record.id),
                    },
                    {
                        key: 'modify',
                        icon: <SettingOutlined />,
                        label: t('action.request_modify'),
                        onClick: () => onModify(record.id, record.name),
                    },
                    {
                        key: 'request-similar',
                        icon: <CopyOutlined />,
                        label: t('action.request_similar'),
                        onClick: () => onRequestSimilar(record.id),
                    },
                ];

                return (
                    <Space size={4}>
                        <Tooltip title={t('action.start')}>
                            <Button
                                type="text"
                                size="small"
                                aria-label={actionLabel('action.start', record.name)}
                                data-testid={`vm-action-start-${record.id}`}
                                icon={<PlayCircleOutlined />}
                                disabled={!isStopped}
                                onClick={() => onStart(record.id)}
                                style={{ color: isStopped ? '#52c41a' : undefined }}
                            />
                        </Tooltip>
                        <Tooltip title={t('action.stop')}>
                            <Button
                                type="text"
                                size="small"
                                aria-label={actionLabel('action.stop', record.name)}
                                data-testid={`vm-action-stop-${record.id}`}
                                icon={<PauseCircleOutlined />}
                                disabled={!isRunning}
                                onClick={() => onStop(record.id)}
                                style={{ color: isRunning ? '#faad14' : undefined }}
                            />
                        </Tooltip>
                        <Tooltip title={t('action.restart')}>
                            <Button
                                type="text"
                                size="small"
                                aria-label={actionLabel('action.restart', record.name)}
                                data-testid={`vm-action-restart-${record.id}`}
                                icon={<RedoOutlined />}
                                disabled={!isRunning}
                                onClick={() => onRestart(record.id)}
                            />
                        </Tooltip>
                        <Tooltip title={t('action.console')}>
                            <Button
                                type="text"
                                size="small"
                                aria-label={actionLabel('action.console', record.name)}
                                data-testid={`vm-action-console-${record.id}`}
                                icon={<DesktopOutlined />}
                                disabled={!isRunning}
                                onClick={() => onConsole(record.id)}
                            />
                        </Tooltip>
                        <Tooltip title={t('action.delete')}>
                            <Button
                                type="text"
                                size="small"
                                danger
                                aria-label={actionLabel('action.delete', record.name)}
                                data-testid={`vm-action-delete-${record.id}`}
                                icon={<DeleteOutlined />}
                                disabled={!canDelete}
                                onClick={() => onDelete(record.id, record.name, record.environment)}
                            />
                        </Tooltip>
                        <Dropdown menu={{ items: moreItems }} trigger={['click']}>
                            <Button
                                type="text"
                                size="small"
                                aria-label={`${t('common:table.actions')} ${record.name}`}
                                data-testid={`vm-action-more-${record.id}`}
                                icon={<MoreOutlined />}
                            />
                        </Dropdown>
                    </Space>
                );
            },
        },
    ];

    return (
        <PageSurface flush={true}>
            <Table<VM>
                columns={columns}
                dataSource={vmData?.items ?? []}
                rowKey="id"
                loading={isLoading}
                rowSelection={{
                    selectedRowKeys,
                    onChange: (keys) => onSelectionChange(keys as string[]),
                    preserveSelectedRowKeys: true,
                }}
                pagination={{
                    current: page,
                    pageSize,
                    total: vmData?.pagination?.total ?? 0,
                    showTotal: (total) => t('common:table.total', { total }),
                    onChange: onPageChange,
                }}
                size="middle"
            />
        </PageSurface>
    );
}
