'use client';

/**
 * /vms/[id] — VM detail page.
 * master-flow.md §5: VM Detail — single VM status and console access.
 *
 * API contracts:
 *   GET  /vms/{vm_id}                → VM
 *   POST /vms/{vm_id}/power          → VM (start/stop/restart)
 *   GET  /vms/{vm_id}/console/status → VMConsoleStatusResponse
 *
 * E2E data-testid requirements:
 *   vm-detail-page
 *   vm-console-status-{id}
 *   vm-action-start-{id}
 *   vm-action-stop-{id}
 *   vm-action-delete-{id}
 *   vm-action-console-{id}
 *   vm-action-request-similar-{id}
 */
import { Badge, Button, Descriptions, Input, Modal, Space, Tag, Typography } from 'antd';
import type { DescriptionsProps } from 'antd';
import {
    ArrowLeftOutlined,
    CopyOutlined,
    DeleteOutlined,
    DesktopOutlined,
    ExclamationCircleOutlined,
    PauseCircleOutlined,
    PlayCircleOutlined,
    RedoOutlined,
} from '@ant-design/icons';
import { useParams, useRouter } from 'next/navigation';
import { useTranslation } from 'react-i18next';
import { useState } from 'react';
import dayjs from 'dayjs';

import { useApiGet } from '@/lib/api/useApiGet';
import { useApiMutation } from '@/lib/api/useApiMutation';
import { api } from '@/lib/api/client';
import { translateApiError } from '@/lib/api/errorMessage';
import { useMessage } from '@/lib/hooks/useMessage';
import { VM_STATUS_MAP } from '@/features/vm-management/types';
import { PageHeader, PageSurface } from '@/components/layouts/PageSection';

const { Text, Paragraph } = Typography;

export default function VMDetailPage() {
    const { t } = useTranslation(['vm', 'common']);
    const params = useParams<{ id: string }>();
    const router = useRouter();
    const vmId = params.id;
    const { messageApi, messageContextHolder } = useMessage();
    const [deleteOpen, setDeleteOpen] = useState(false);
    const [deleteConfirmName, setDeleteConfirmName] = useState('');

    const { data: vm, isLoading, refetch } = useApiGet(
        ['vm-detail', vmId],
        () => api.GET('/vms/{vm_id}', { params: { path: { vm_id: vmId } } })
    );

    const powerMutation = useApiMutation(
        ({ action }: { action: 'start' | 'stop' | 'restart' }) =>
            api.POST('/vms/{vm_id}/power', {
                params: { path: { vm_id: vmId } },
                body: { action },
            }),
        {
            onSuccess: () => {
                void messageApi.success(t('message.action_submitted'));
                void refetch();
            },
        }
    );

    const deleteMutation = useApiMutation(
        () =>
            api.DELETE('/vms/{vm_id}', {
                params: {
                    path: { vm_id: vmId },
                    query: { confirm: true, confirm_name: vm?.name ?? '' },
                },
            }),
        {
            onSuccess: () => {
                void messageApi.success(t('delete_request_submitted'));
                setDeleteOpen(false);
                setDeleteConfirmName('');
                router.push('/vms');
            },
            onError: (err) => {
                void messageApi.error(translateApiError(t, err));
            },
        }
    );

    // vm data is typed from the generated spec
    const vmData = vm;
    const status = vmData?.status as string | undefined;
    const mapped = VM_STATUS_MAP[status ?? 'UNKNOWN'] ?? VM_STATUS_MAP.UNKNOWN;
    const isRunning = status === 'RUNNING';
    const isStopped = status === 'STOPPED';
    const canDelete = isStopped || status === 'FAILED' || status === 'NOT_FOUND';
    const requiresNameConfirm = vmData?.environment !== 'test';
    const deleteConfirmMatched = !requiresNameConfirm || deleteConfirmName === (vmData?.name ?? '');
    const detailItems: DescriptionsProps['items'] = [
        {
            key: 'name',
            label: t('field.name'),
            children: <Text strong>{vmData?.name}</Text>,
        },
        {
            key: 'status',
            label: t('common:table.status'),
            children: (
                <Badge
                    status={mapped.badge}
                    text={<Tag color={mapped.color}>{t(`status.${status ?? 'UNKNOWN'}`)}</Tag>}
                />
            ),
        },
        {
            key: 'namespace',
            label: t('field.namespace'),
            children: <Tag>{vmData?.namespace}</Tag>,
        },
        {
            key: 'hostname',
            label: t('field.hostname'),
            children: <Text type="secondary">{vmData?.hostname || '—'}</Text>,
        },
        {
            key: 'createdAt',
            label: t('common:table.created_at'),
            children: (
                <Text type="secondary">
                    {vmData?.created_at ? dayjs(vmData.created_at).format('YYYY-MM-DD HH:mm:ss') : '—'}
                </Text>
            ),
        },
    ];

    return (
        <div data-testid="vm-detail-page">
            {messageContextHolder}
            <PageHeader
                title={(
                    <Space size="small">
                        <DesktopOutlined style={{ color: '#531dab' }} />
                        <span>{vmData?.name ?? vmId}</span>
                    </Space>
                )}
                subtitle={t('detail.subtitle')}
                actions={(
                    <Button
                        icon={<ArrowLeftOutlined />}
                        type="text"
                        onClick={() => router.push('/vms')}
                    >
                        {t('common:button.back')}
                    </Button>
                )}
            />

            <PageSurface loading={isLoading}>
                <Descriptions bordered column={2} items={detailItems} />
            </PageSurface>

            <PageSurface title={t('common:table.actions')}>
                <Space wrap>
                    <Button
                        type="primary"
                        icon={<PlayCircleOutlined />}
                        data-testid={`vm-action-start-${vmId}`}
                        disabled={!isStopped}
                        loading={powerMutation.isPending}
                        onClick={() => powerMutation.mutate({ action: 'start' })}
                    >
                        {t('action.start')}
                    </Button>
                    <Button
                        icon={<PauseCircleOutlined />}
                        data-testid={`vm-action-stop-${vmId}`}
                        disabled={!isRunning}
                        loading={powerMutation.isPending}
                        onClick={() => powerMutation.mutate({ action: 'stop' })}
                    >
                        {t('action.stop')}
                    </Button>
                    <Button
                        icon={<RedoOutlined />}
                        data-testid={`vm-action-restart-${vmId}`}
                        disabled={!isRunning}
                        loading={powerMutation.isPending}
                        onClick={() => powerMutation.mutate({ action: 'restart' })}
                    >
                        {t('action.restart')}
                    </Button>
                    <Button
                        icon={<DesktopOutlined />}
                        data-testid={`vm-action-console-${vmId}`}
                        disabled={!isRunning}
                        onClick={() => {
                            window.open(`/api/v1/vms/${vmId}/console`, '_blank');
                        }}
                    >
                        {t('action.console')}
                    </Button>
                    <Button
                        icon={<CopyOutlined />}
                        data-testid={`vm-action-request-similar-${vmId}`}
                        onClick={() => router.push(`/vms?request=create&source_vm_id=${vmId}`)}
                    >
                        {t('action.request_similar')}
                    </Button>
                    <Button
                        icon={<DesktopOutlined />}
                        data-testid={`vm-console-status-${vmId}`}
                        onClick={() => void refetch()}
                        loading={isLoading}
                    >
                        {t('action.refresh_status')}
                    </Button>
                    <Button
                        danger
                        icon={<DeleteOutlined />}
                        data-testid={`vm-action-delete-${vmId}`}
                        disabled={!canDelete || !vmData?.name}
                        loading={deleteMutation.isPending}
                        onClick={() => {
                            setDeleteConfirmName('');
                            setDeleteOpen(true);
                        }}
                    >
                        {t('action.delete')}
                    </Button>
                </Space>
            </PageSurface>
            <Modal
                title={(
                    <Space>
                        <ExclamationCircleOutlined style={{ color: '#ff4d4f' }} />
                        {t('action.delete_confirm')}
                    </Space>
                )}
                open={deleteOpen}
                onOk={() => {
                    if (!vmData?.name) {
                        return;
                    }
                    if (requiresNameConfirm && deleteConfirmName !== vmData.name) {
                        void messageApi.warning(t('action.delete_type_name_hint'));
                        return;
                    }
                    deleteMutation.mutate();
                }}
                onCancel={() => {
                    setDeleteOpen(false);
                    setDeleteConfirmName('');
                }}
                confirmLoading={deleteMutation.isPending}
                okButtonProps={{ danger: true, disabled: !deleteConfirmMatched }}
                okText={t('common:button.delete')}
                forceRender={true}
                data-testid="vm-delete-modal"
            >
                <Paragraph>
                    {t('action.delete_confirm_name', { name: vmData?.name ?? vmId })}
                </Paragraph>
                {requiresNameConfirm && (
                    <>
                        <Paragraph type="secondary">
                            {t('action.delete_type_name_hint')}
                        </Paragraph>
                        <Input
                            value={deleteConfirmName}
                            onChange={(e) => setDeleteConfirmName(e.target.value)}
                            placeholder={vmData?.name ?? vmId}
                            status={deleteConfirmName && deleteConfirmName !== vmData?.name ? 'error' : undefined}
                        />
                    </>
                )}
            </Modal>
        </div>
    );
}
