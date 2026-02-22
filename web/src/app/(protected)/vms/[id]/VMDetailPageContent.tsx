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
 *   vm-action-console-{id}
 */
import { Badge, Button, Card, Descriptions, Space, Tag, Typography } from 'antd';
import {
    ArrowLeftOutlined,
    DesktopOutlined,
    PauseCircleOutlined,
    PlayCircleOutlined,
    RedoOutlined,
} from '@ant-design/icons';
import { useParams, useRouter } from 'next/navigation';
import { useTranslation } from 'react-i18next';
import dayjs from 'dayjs';

import { useApiGet } from '@/lib/api/useApiGet';
import { useApiMutation } from '@/lib/api/useApiMutation';
import { api } from '@/lib/api/client';
import { useMessage } from '@/lib/hooks/useMessage';
import { VM_STATUS_MAP } from '@/features/vm-management/types';

const { Title, Text } = Typography;

export default function VMDetailPage() {
    const { t } = useTranslation(['vm', 'common']);
    const params = useParams<{ id: string }>();
    const router = useRouter();
    const vmId = params.id;
    const { messageApi, messageContextHolder } = useMessage();

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

    // vm data is typed from the generated spec
    const vmData = vm;
    const status = vmData?.status as string | undefined;
    const mapped = VM_STATUS_MAP[status ?? 'UNKNOWN'] ?? VM_STATUS_MAP.UNKNOWN;
    const isRunning = status === 'RUNNING';
    const isStopped = status === 'STOPPED';

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
                </Space>
            </Card>
        </div>
    );
}
