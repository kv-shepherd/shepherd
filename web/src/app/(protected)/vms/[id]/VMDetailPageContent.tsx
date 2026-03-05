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
 */
import { Badge, Button, Card, Descriptions, Input, Modal, Space, Tag, Typography } from 'antd';
import {
    ArrowLeftOutlined,
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
import { useMessage } from '@/lib/hooks/useMessage';
import { VM_STATUS_MAP } from '@/features/vm-management/types';

const { Title, Text, Paragraph } = Typography;

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
            onSuccess: (resp) => {
                const ticketID = resp?.ticket_id ?? '—';
                void messageApi.success(t('delete_request_submitted', { ticket_id: ticketID }));
                setDeleteOpen(false);
                setDeleteConfirmName('');
                router.push('/vms');
            },
            onError: (err) => {
                void messageApi.error(err.message || t('common:message.error'));
            },
        }
    );

    // vm data is typed from the generated spec
    const vmData = vm;
    const status = vmData?.status as string | undefined;
    const mapped = VM_STATUS_MAP[status ?? 'UNKNOWN'] ?? VM_STATUS_MAP.UNKNOWN;
    const isRunning = status === 'RUNNING';
    const isStopped = status === 'STOPPED';
    const canDelete = isStopped || status === 'FAILED';
    const requiresNameConfirm = vmData?.environment !== 'test';
    const deleteConfirmMatched = !requiresNameConfirm || deleteConfirmName === (vmData?.name ?? '');

    return (
        <div data-testid="vm-detail-page">
            {messageContextHolder}
            <div style={{ marginBottom: 24 }}>
                <Space>
                    <Button
                        icon={<ArrowLeftOutlined />}
                        type="text"
                        onClick={() => router.push('/vms')}
                    >
                        {t('common:button.back')}
                    </Button>
                </Space>
                <Title level={4} style={{ margin: '8px 0 0' }}>
                    <DesktopOutlined style={{ marginRight: 8, color: '#531dab' }} />
                    {vmData?.name ?? vmId}
                </Title>
            </div>

            <Card style={{ borderRadius: 12, marginBottom: 16 }} loading={isLoading}>
                <Descriptions bordered column={2}>
                    <Descriptions.Item label={t('field.name')}>
                        <Text strong>{vmData?.name}</Text>
                    </Descriptions.Item>
                    <Descriptions.Item label={t('common:table.status')}>
                        <Badge
                            status={mapped.badge}
                            text={<Tag color={mapped.color}>{status}</Tag>}
                        />
                    </Descriptions.Item>
                    <Descriptions.Item label={t('field.namespace')}>
                        <Tag>{vmData?.namespace}</Tag>
                    </Descriptions.Item>
                    <Descriptions.Item label={t('field.hostname')}>
                        <Text type="secondary">{vmData?.hostname || '—'}</Text>
                    </Descriptions.Item>
                    <Descriptions.Item label={t('common:table.created_at')}>
                        <Text type="secondary">
                            {vmData?.created_at ? dayjs(vmData.created_at).format('YYYY-MM-DD HH:mm:ss') : '—'}
                        </Text>
                    </Descriptions.Item>
                </Descriptions>
            </Card>

            <Card style={{ borderRadius: 12 }}>
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
                        icon={<DesktopOutlined />}
                        data-testid={`vm-console-status-${vmId}`}
                        onClick={() => void refetch()}
                        loading={isLoading}
                    >
                        {t('action.refresh_status', { defaultValue: 'Refresh Status' })}
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
            </Card>
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
                destroyOnHidden={true}
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
