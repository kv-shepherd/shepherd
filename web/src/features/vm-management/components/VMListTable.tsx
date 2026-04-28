'use client';

import {
    Badge,
    Button,
    Collapse,
    Dropdown,
    Pagination,
    Popconfirm,
    Segmented,
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
    TagsOutlined,
} from '@ant-design/icons';
import dayjs from 'dayjs';
import type { MenuProps, TableProps } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import type { TFunction } from 'i18next';
import type { Key, ReactNode } from 'react';
import { useMemo, useState } from 'react';

import type { VM, VMList } from '../types';
import { formatMemory, VM_STATUS_MAP } from '../types';
import { PageSurface } from '@/components/layouts/PageSection';
import { hasAnyConsoleCapability } from '@/features/vm-management/console';
import { formatVMOperatingSystem } from '@/features/vm-management/osInfo';

const CpuResourceIcon = () => (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" width="1em" height="1em">
        <rect x="4" y="4" width="16" height="16" rx="2" ry="2" />
        <rect x="9" y="9" width="6" height="6" />
        <line x1="9" y1="1" x2="9" y2="4" />
        <line x1="15" y1="1" x2="15" y2="4" />
        <line x1="9" y1="20" x2="9" y2="23" />
        <line x1="15" y1="20" x2="15" y2="23" />
        <line x1="20" y1="9" x2="23" y2="9" />
        <line x1="20" y1="14" x2="23" y2="14" />
        <line x1="1" y1="9" x2="4" y2="9" />
        <line x1="1" y1="14" x2="4" y2="14" />
    </svg>
);

const MemoryResourceIcon = () => (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" width="1em" height="1em">
        <line x1="4" y1="8" x2="20" y2="8" />
        <line x1="4" y1="16" x2="20" y2="16" />
        <line x1="8" y1="8" x2="8" y2="16" />
        <line x1="12" y1="8" x2="12" y2="16" />
        <line x1="16" y1="8" x2="16" y2="16" />
        <path d="M4 4h16v16H4z" />
    </svg>
);

const DiskResourceIcon = () => (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" width="1em" height="1em">
        <line x1="22" y1="12" x2="2" y2="12" />
        <path d="M5.45 5.11 2 12v6a2 2 0 0 0 2 2h16a2 2 0 0 0 2-2v-6l-3.45-6.89A2 2 0 0 0 16.76 4H7.24a2 2 0 0 0-1.79 1.11z" />
        <line x1="6" y1="16" x2="6.01" y2="16" />
        <line x1="10" y1="16" x2="14" y2="16" />
    </svg>
);

const { Text: TypographyText, Link: TypographyLink } = Typography;
type VMListViewMode = 'grouped' | 'table';

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

function renderTooltipFact(
    title: string,
    icon: ReactNode,
    value: string | undefined,
    className = 'vm-list-inline-fact',
) {
    if (!value || value.trim() === '') {
        return null;
    }
    return (
        <Tooltip title={title} trigger={['hover', 'focus']} destroyOnHidden={true}>
            <span className={className}>
                <span className={`${className}__icon`} aria-hidden="true">
                    {icon}
                </span>
                <span className={`${className}__value selectable-inline-text`}>
                    {value}
                </span>
            </span>
        </Tooltip>
    );
}

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
    const [viewMode, setViewMode] = useState<VMListViewMode>('grouped');
    const actionLabel = (actionKey: string, vmName: string) => `${t(actionKey)} ${vmName}`;

    const columns: ColumnsType<VM> = [
        {
            title: t('field.name'),
            dataIndex: 'name',
            key: 'name',
            width: 220,
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
                            {record.hostname && record.hostname !== name ? (
                                <span className="vm-list-inline-chip vm-list-inline-chip--identity selectable-inline-text">
                                    <span className="vm-list-inline-chip__label">{t('field.hostname')}</span>
                                    <span className="vm-list-inline-chip__value">{record.hostname}</span>
                                </span>
                            ) : null}
                            {record.ip_address ? (
                                <TypographyText
                                    copyable={{ text: record.ip_address }}
                                    className="vm-list-inline-chip vm-list-inline-chip--network selectable-inline-text"
                                >
                                    <span className="vm-list-inline-chip__label">{t('field.ip_address')}</span>
                                    <span className="vm-list-inline-chip__value">{record.ip_address}</span>
                                </TypographyText>
                            ) : null}
                        </Space>
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
            width: 104,
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
            width: 152,
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
                        <div className="vm-list-scope-facts">
                            {record.environment ? (
                                renderTooltipFact(
                                    t('field.environment'),
                                    <TagsOutlined />,
                                    record.environment,
                                )
                            ) : null}
                            {inCurrentServiceContext ? <Tag color="green">{t('context.row_badge')}</Tag> : null}
                        </div>
                    </Space>
                );
            },
        },
        {
            title: t('field.placement', { defaultValue: 'Runtime location' }),
            key: 'placement',
            width: 160,
            render: (_, record) => (
                <Space direction="vertical" size={4} className="workbench-table-stack">
                    <div className="vm-list-labeled-lines">
                        <div className="vm-list-labeled-line">
                            <TypographyText type="secondary" className="vm-list-labeled-line__label">
                                {t('field.namespace')}
                            </TypographyText>
                            <TypographyText className="vm-list-labeled-line__value selectable-inline-text">
                                {record.namespace || '—'}
                            </TypographyText>
                        </div>
                        <div className="vm-list-labeled-line">
                            <TypographyText type="secondary" className="vm-list-labeled-line__label">
                                {t('field.cluster')}
                            </TypographyText>
                            <TypographyText className="vm-list-labeled-line__value selectable-inline-text">
                                {record.cluster_name || '—'}
                            </TypographyText>
                        </div>
                    </div>
                </Space>
            ),
        },
        {
            title: t('field.resources'),
            key: 'resources',
            width: 132,
            render: (_, record) => {
                const cpuValue = formatCPU(record.cpu_cores);
                const memoryValue = formatMemory(record.memory_gi ?? 0);
                const diskValue = formatDisk(record.disk_gb);

                return (
                    <Space direction="vertical" size={4} className="workbench-table-stack">
                        <div className="vm-list-resource-chips">
                            {renderTooltipFact(
                                t('field.cpu', { defaultValue: 'CPU' }),
                                <CpuResourceIcon />,
                                cpuValue !== '—' ? cpuValue : undefined,
                                'vm-list-resource-chip',
                            )}
                            {renderTooltipFact(
                                t('field.memory', { defaultValue: 'Memory' }),
                                <MemoryResourceIcon />,
                                memoryValue !== '0 Mi' && memoryValue !== '—' ? memoryValue : undefined,
                                'vm-list-resource-chip',
                            )}
                            {renderTooltipFact(
                                t('field.disk', { defaultValue: 'Disk' }),
                                <DiskResourceIcon />,
                                diskValue !== '—' ? diskValue : undefined,
                                'vm-list-resource-chip',
                            )}
                            {!record.cpu_cores && !record.memory_gi && !record.disk_gb ? (
                                <TypographyText className="workbench-table-description selectable-inline-text">
                                    —
                                </TypographyText>
                            ) : null}
                        </div>
                    </Space>
                );
            },
        },
        {
            title: t('common:table.actions'),
            key: 'actions',
            width: 176,
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

    const vmItems = useMemo(() => vmData?.items ?? [], [vmData?.items]);
    const vmGroups = useMemo(() => {
        const systems = new Map<string, {
            key: string;
            systemName: string;
            items: VM[];
            services: Map<string, { key: string; serviceName: string; items: VM[] }>;
        }>();
        vmItems.forEach((vm) => {
            const systemKey = vm.system_id || vm.system_name || 'unknown-system';
            const system = systems.get(systemKey) ?? {
                key: systemKey,
                systemName: vm.system_name || t('group.unknown_system'),
                items: [],
                services: new Map<string, { key: string; serviceName: string; items: VM[] }>(),
            };
            system.items.push(vm);
            const serviceKey = vm.service_id || vm.service_name || `${systemKey}:unknown-service`;
            const service = system.services.get(serviceKey) ?? {
                key: serviceKey,
                serviceName: vm.service_name || t('group.unknown_service'),
                items: [],
            };
            service.items.push(vm);
            system.services.set(serviceKey, service);
            systems.set(systemKey, system);
        });
        return Array.from(systems.values())
            .sort((a, b) => a.systemName.localeCompare(b.systemName))
            .map((system) => ({
                ...system,
                services: Array.from(system.services.values()).sort((a, b) => a.serviceName.localeCompare(b.serviceName)),
            }));
    }, [t, vmItems]);

    const rowSelection: TableProps<VM>['rowSelection'] = {
        selectedRowKeys,
        onChange: (keys: Key[]) => onSelectionChange(keys as string[]),
        preserveSelectedRowKeys: true,
    };

    const pagination: TableProps<VM>['pagination'] = {
        current: page,
        pageSize,
        total: vmData?.pagination?.total ?? 0,
        showTotal: (total: number) => t('common:table.total', { total }),
        onChange: onPageChange,
    };

    return (
        <PageSurface className="vm-page-surface vm-page-surface--table" flush={true}>
            <div className="vm-list-view-toolbar">
                <Segmented<VMListViewMode>
                    value={viewMode}
                    onChange={setViewMode}
                    options={[
                        { value: 'grouped', label: t('group.view_grouped') },
                        { value: 'table', label: t('group.view_table') },
                    ]}
                />
            </div>
            {viewMode === 'grouped' && vmGroups.length > 0 ? (
                <div className="vm-grouped-list">
                    <Collapse
                        defaultActiveKey={vmGroups.slice(0, 1).map((system) => system.key)}
                        items={vmGroups.map((system) => ({
                            key: system.key,
                            label: (
                                <div className="vm-grouped-list__system-label">
                                    <TypographyText strong>{system.systemName}</TypographyText>
                                    <Tag color="blue">{t('group.vm_count', { count: system.items.length })}</Tag>
                                </div>
                            ),
                            children: (
                                <Collapse
                                    ghost
                                    className="vm-grouped-list__service-collapse"
                                    defaultActiveKey={system.services.slice(0, 1).map((service) => service.key)}
                                    items={system.services.map((service) => ({
                                        key: service.key,
                                        label: (
                                            <div className="vm-grouped-list__service-label">
                                                <TypographyText>{service.serviceName}</TypographyText>
                                                <Tag>{t('group.vm_count', { count: service.items.length })}</Tag>
                                            </div>
                                        ),
                                        children: (
                                            <Table<VM>
                                                columns={columns}
                                                dataSource={service.items}
                                                rowKey="id"
                                                loading={isLoading}
                                                rowSelection={rowSelection}
                                                pagination={false}
                                                size="middle"
                                                scroll={{ x: 980 }}
                                            />
                                        ),
                                    }))}
                                />
                            ),
                        }))}
                    />
                    <Pagination
                        className="vm-grouped-list__pagination"
                        current={page}
                        pageSize={pageSize}
                        total={vmData?.pagination?.total ?? 0}
                        showTotal={(total) => t('common:table.total', { total })}
                        onChange={onPageChange}
                    />
                </div>
            ) : (
                <Table<VM>
                    columns={columns}
                    dataSource={vmItems}
                    rowKey="id"
                    loading={isLoading}
                    rowSelection={rowSelection}
                    pagination={pagination}
                    size="middle"
                    scroll={{ x: 980 }}
                />
            )}
        </PageSurface>
    );
}
