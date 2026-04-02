'use client';

import {
    Badge,
    Button,
    Dropdown,
    Popconfirm,
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
import { formatMemory, VM_STATUS_MAP } from '../types';
import { PageSurface } from '@/components/layouts/PageSection';
import { hasAnyConsoleCapability } from '@/features/vm-management/console';
import { formatVMOperatingSystem } from '@/features/vm-management/osInfo';

const { Text: TypographyText, Link: TypographyLink } = Typography;

const formatCPU = (cpuCores: number | undefined): string => {
    if (!Number.isFinite(cpuCores) || cpuCores === undefined || cpuCores <= 0) {
        return '—';
    }
    return `${Number.isInteger(cpuCores) ? cpuCores : cpuCores.toFixed(1)} vCPU`;
};

const formatDisk = (diskGb: number | undefined): string => {
    if (!Number.isFinite(diskGb) || diskGb === undefined || diskGb <= 0) {
        return '—';
    }
    return `${diskGb} Gi`;
};

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
    onConsole: (vm: VM) => void;
    onDelete: (vmId: string, vmName: string, environment?: string) => void;
    onModify: (vmId: string, vmName: string) => void;
    onRequestSimilar: (vmId: string) => void;
    onDetail: (vmId: string) => void;
    onOpenSystem: (systemId: string) => void;
    onOpenService: (systemId: string, serviceId: string) => void;
    contextSystemId?: string;
    contextServiceId?: string;
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
    onOpenSystem,
    onOpenService,
    contextSystemId,
    contextServiceId,
    selectedRowKeys,
    onSelectionChange,
}: VMListTableProps) {
    const actionLabel = (actionKey: string, vmName: string) => `${t(actionKey)} ${vmName}`;

    const columns: ColumnsType<VM> = [
        {
            title: t('field.name'),
            dataIndex: 'name',
            key: 'name',
            width: 400,
            render: (name: string, record) => {
                const osLabel = formatVMOperatingSystem(record);

                return (
                    <Space direction="vertical" size={4} className="workbench-table-stack">
                        <Space size={[8, 6]} wrap className="workbench-table-heading">
                            <DesktopOutlined style={{ color: '#531dab' }} />
                            <span className="vm-list-name-shell">
                                <TypographyLink
                                    className="selectable-inline-text"
                                    style={{ fontWeight: 600 }}
                                    data-testid={`vm-action-detail-${record.id}`}
                                    onClick={() => onDetail(record.id)}
                                >
                                    {name}
                                </TypographyLink>
                                <TypographyText
                                    copyable={{ text: name, tooltips: false, icon: <CopyOutlined /> }}
                                    className="vm-list-name-copy"
                                    data-testid={`vm-name-copy-${record.id}`}
                                >
                                    {' '}
                                </TypographyText>
                            </span>
                            {record.ip_address ? (
                                <TypographyText
                                    copyable={{ text: record.ip_address }}
                                    className="vm-list-inline-chip selectable-inline-text"
                                >
                                    <span className="vm-list-inline-chip__label">{t('field.ip_address')}</span>
                                    <span className="vm-list-inline-chip__value">{record.ip_address}</span>
                                </TypographyText>
                            ) : null}
                        </Space>
                        {record.hostname && record.hostname !== name ? (
                            <TypographyText type="secondary" className="workbench-table-note selectable-inline-text">
                                {t('field.hostname')}: {record.hostname}
                            </TypographyText>
                        ) : null}
                        {osLabel ? (
                            <TypographyText type="secondary" className="workbench-table-note selectable-inline-text">
                                {osLabel}
                            </TypographyText>
                        ) : null}
                    </Space>
                );
            },
        },
        {
            title: t('common:table.status'),
            dataIndex: 'status',
            key: 'status',
            width: 130,
            render: (status: VM['status'], record) => {
                const mapped = VM_STATUS_MAP[status] ?? VM_STATUS_MAP.UNKNOWN;
                return (
                    <Space direction="vertical" size={4} className="workbench-table-stack">
                        <Badge status={mapped.badge} text={<Tag color={mapped.color}>{t(`status.${status}`)}</Tag>} />
                        <TypographyText type="secondary" className="workbench-table-note">
                            {record.created_at ? dayjs(record.created_at).format('YYYY-MM-DD HH:mm') : '—'}
                        </TypographyText>
                    </Space>
                );
            },
        },
        {
            title: t('field.scope'),
            key: 'scope',
            width: 240,
            render: (_, record) => {
                const inCurrentServiceContext =
                    Boolean(contextSystemId)
                    && Boolean(contextServiceId)
                    && record.system_id === contextSystemId
                    && record.service_id === contextServiceId;

                return (
                    <Space direction="vertical" size={4} className="workbench-table-stack">
                        <div className="vm-list-labeled-lines">
                            <div className="vm-list-labeled-line">
                                <TypographyText type="secondary" className="vm-list-labeled-line__label">
                                    {t('field.system', { defaultValue: 'System' })}
                                </TypographyText>
                                {record.system_id && record.system_name ? (
                                    <TypographyLink
                                        className="selectable-inline-text vm-list-labeled-line__value"
                                        onClick={() => onOpenSystem(record.system_id!)}
                                    >
                                        {record.system_name}
                                    </TypographyLink>
                                ) : (
                                    <TypographyText className="vm-list-labeled-line__value selectable-inline-text">
                                        {record.system_name || '—'}
                                    </TypographyText>
                                )}
                            </div>
                            <div className="vm-list-labeled-line">
                                <TypographyText type="secondary" className="vm-list-labeled-line__label">
                                    {t('field.service')}
                                </TypographyText>
                                {record.system_id && record.service_id && record.service_name ? (
                                    <TypographyLink
                                        className="selectable-inline-text vm-list-labeled-line__value"
                                        onClick={() => onOpenService(record.system_id!, record.service_id!)}
                                    >
                                        {record.service_name}
                                    </TypographyLink>
                                ) : (
                                    <TypographyText type="secondary" className="vm-list-labeled-line__value selectable-inline-text">
                                        {record.service_name || '—'}
                                    </TypographyText>
                                )}
                            </div>
                        </div>
                        {inCurrentServiceContext ? <Tag color="green">{t('context.row_badge')}</Tag> : null}
                    </Space>
                );
            },
        },
        {
            title: t('field.resources'),
            key: 'resources',
            width: 280,
            render: (_, record) => {
                const resourceSummary = [
                    formatCPU(record.cpu_cores),
                    formatMemory(record.memory_gi ?? 0),
                    formatDisk(record.disk_gb),
                ]
                    .filter((value) => value && value !== '—')
                    .join(' · ');
                const placementSummary = [record.namespace, record.cluster_name, record.environment]
                    .filter(Boolean)
                    .join(' · ');

                return (
                    <Space direction="vertical" size={4} className="workbench-table-stack">
                        <TypographyText className="workbench-table-description selectable-inline-text">
                            {resourceSummary || '—'}
                        </TypographyText>
                        {placementSummary ? (
                            <div className="vm-list-inline-facts">
                                {record.namespace ? (
                                    <span className="vm-list-inline-fact">
                                        <TypographyText type="secondary" className="vm-list-inline-fact__label">
                                            {t('field.namespace')}
                                        </TypographyText>
                                        <TypographyText className="vm-list-inline-fact__value selectable-inline-text">
                                            {record.namespace}
                                        </TypographyText>
                                    </span>
                                ) : null}
                                {record.cluster_name ? (
                                    <span className="vm-list-inline-fact">
                                        <TypographyText type="secondary" className="vm-list-inline-fact__label">
                                            {t('field.cluster')}
                                        </TypographyText>
                                        <TypographyText className="vm-list-inline-fact__value selectable-inline-text">
                                            {record.cluster_name}
                                        </TypographyText>
                                    </span>
                                ) : null}
                                {record.environment ? (
                                    <span className="vm-list-inline-fact">
                                        <TypographyText type="secondary" className="vm-list-inline-fact__label">
                                            {t('field.environment')}
                                        </TypographyText>
                                        <TypographyText className="vm-list-inline-fact__value selectable-inline-text">
                                            {record.environment}
                                        </TypographyText>
                                    </span>
                                ) : null}
                            </div>
                        ) : null}
                    </Space>
                );
            },
        },
        {
            title: t('common:table.actions'),
            key: 'actions',
            width: 200,
            render: (_, record) => {
                const isRunning = record.status === 'RUNNING';
                const isStoppable = record.status === 'RUNNING' || record.status === 'STARTING';
                const isStopped = record.status === 'STOPPED';
                const canDelete = isStopped
                    || record.status === 'FAILED'
                    || record.status === 'NOT_FOUND'
                    || record.status === 'UNKNOWN';
                const consoleAvailable = hasAnyConsoleCapability(record);
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
                    <Space size={4} className="copy-friendly-actions">
                        <Tooltip title={t('action.start')}>
                            <Popconfirm
                                title={t('action.start_confirm')}
                                okText={t('common:button.confirm')}
                                cancelText={t('common:button.cancel')}
                                disabled={!isStopped}
                                onConfirm={() => onStart(record.id)}
                            >
                                <Button
                                    type="text"
                                    size="small"
                                    aria-label={actionLabel('action.start', record.name)}
                                    data-testid={`vm-action-start-${record.id}`}
                                    icon={<PlayCircleOutlined />}
                                    disabled={!isStopped}
                                    style={{ color: isStopped ? '#52c41a' : undefined }}
                                />
                            </Popconfirm>
                        </Tooltip>
                        <Tooltip title={t('action.stop')}>
                            <Popconfirm
                                title={t('action.stop_confirm')}
                                okText={t('common:button.confirm')}
                                cancelText={t('common:button.cancel')}
                                disabled={!isStoppable}
                                onConfirm={() => onStop(record.id)}
                            >
                                <Button
                                    type="text"
                                    size="small"
                                    aria-label={actionLabel('action.stop', record.name)}
                                    data-testid={`vm-action-stop-${record.id}`}
                                    icon={<PauseCircleOutlined />}
                                    disabled={!isStoppable}
                                    style={{ color: isStoppable ? '#faad14' : undefined }}
                                />
                            </Popconfirm>
                        </Tooltip>
                        <Tooltip title={t('action.restart')}>
                            <Popconfirm
                                title={t('action.restart_confirm')}
                                okText={t('common:button.confirm')}
                                cancelText={t('common:button.cancel')}
                                disabled={!isRunning}
                                onConfirm={() => onRestart(record.id)}
                            >
                                <Button
                                    type="text"
                                    size="small"
                                    aria-label={actionLabel('action.restart', record.name)}
                                    data-testid={`vm-action-restart-${record.id}`}
                                    icon={<RedoOutlined />}
                                    disabled={!isRunning}
                                />
                            </Popconfirm>
                        </Tooltip>
                        <Tooltip title={t('action.console')}>
                            <Button
                                type="text"
                                size="small"
                                aria-label={actionLabel('action.console', record.name)}
                                data-testid={`vm-action-console-${record.id}`}
                                icon={<DesktopOutlined />}
                                disabled={!isRunning || !consoleAvailable}
                                onClick={() => onConsole(record)}
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
                scroll={{ x: 'max-content' }}
            />
        </PageSurface>
    );
}
