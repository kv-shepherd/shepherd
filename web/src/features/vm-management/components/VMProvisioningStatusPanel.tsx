import { Alert, Descriptions, Progress, Space, Tag, Timeline, Typography } from 'antd';
import type { DescriptionsProps, TimelineProps } from 'antd';
import { useTranslation } from 'react-i18next';

import {
    hasProvisioningFailure,
    parseProvisioningPercent,
    type VMProvisioningStatus,
} from '@/features/vm-management/provisioning';

const { Text } = Typography;

function phaseTagColor(phase?: string): string {
    const normalized = phase?.trim().toLowerCase();
    if (!normalized) return 'default';
    if (normalized === 'failed') return 'red';
    if (['ready', 'succeeded', 'success', 'complete', 'completed'].includes(normalized)) return 'green';
    return 'blue';
}

function cloneTypeTagColor(cloneType?: string): string {
    if (!cloneType) return 'default';
    if (cloneType === 'copy') return 'orange';
    return 'geekblue';
}

function formatObservedAt(value?: string): string {
    if (!value) {
        return '';
    }
    return value.replace('T', ' ').replace('Z', ' UTC');
}

interface VMProvisioningStatusPanelProps {
    provisioning: VMProvisioningStatus;
    pollingActive?: boolean;
}

export function VMProvisioningStatusPanel({
    provisioning,
    pollingActive = false,
}: VMProvisioningStatusPanelProps) {
    const { t } = useTranslation(['vm', 'common']);
    const percent = parseProvisioningPercent(provisioning.progress);
    const failed = hasProvisioningFailure(provisioning);
    const progressStatus = failed ? 'exception' : percent === 100 ? 'success' : pollingActive ? 'active' : 'normal';
    const descriptionItems: DescriptionsProps['items'] = [
        {
            key: 'phase',
            label: t('provisioning.phase'),
            children: (
                <Tag color={phaseTagColor(provisioning.phase)} data-testid="vm-provisioning-phase">
                    {provisioning.phase || '—'}
                </Tag>
            ),
        },
        {
            key: 'progress',
            label: t('provisioning.progress'),
            children: provisioning.progress || '—',
        },
        {
            key: 'claim',
            label: t('provisioning.root_claim'),
            children: provisioning.claim_name || '—',
        },
        {
            key: 'pvcPhase',
            label: t('provisioning.pvc_phase'),
            children: provisioning.pvc_phase || '—',
        },
        {
            key: 'cloneType',
            label: t('provisioning.clone_type'),
            children: provisioning.clone_type ? (
                <Tag color={cloneTypeTagColor(provisioning.clone_type)}>
                    {provisioning.clone_type === 'copy'
                        ? t('provisioning.clone_type_copy')
                        : provisioning.clone_type}
                </Tag>
            ) : '—',
        },
        {
            key: 'restartCount',
            label: t('provisioning.restart_count'),
            children: provisioning.restart_count ?? '—',
        },
    ];

    const conditionItems: TimelineProps['items'] = (provisioning.conditions ?? [])
        .slice(0, 5)
        .map((condition) => ({
            color: condition.status === 'False' ? 'red' : 'blue',
            children: (
                <Space direction="vertical" size={0}>
                    <Text strong>{condition.type || condition.reason || '—'}</Text>
                    <Text type="secondary">
                        {[condition.status, condition.reason].filter(Boolean).join(' · ') || '—'}
                    </Text>
                    {condition.message ? <Text>{condition.message}</Text> : null}
                    {condition.last_transition_time ? (
                        <Text type="secondary">{formatObservedAt(condition.last_transition_time)}</Text>
                    ) : null}
                </Space>
            ),
        }));

    const eventItems: TimelineProps['items'] = (provisioning.recent_events ?? [])
        .slice(0, 5)
        .map((event) => ({
            color: event.type === 'Warning' ? 'red' : 'gray',
            children: (
                <Space direction="vertical" size={0}>
                    <Text strong>{event.reason || event.type || '—'}</Text>
                    {event.message ? <Text>{event.message}</Text> : null}
                    <Text type="secondary">
                        {[event.type, event.count ? `x${event.count}` : '', formatObservedAt(event.last_observed)]
                            .filter(Boolean)
                            .join(' · ') || '—'}
                    </Text>
                </Space>
            ),
        }));

    return (
        <Space
            direction="vertical"
            size={16}
            style={{ width: '100%' }}
            role={pollingActive ? 'status' : undefined}
            aria-live={pollingActive ? 'polite' : undefined}
            data-testid="vm-provisioning-panel"
        >
            <Alert
                type={failed ? 'error' : pollingActive ? 'info' : 'success'}
                showIcon
                message={failed ? t('provisioning.failed_title') : t('provisioning.current_title')}
                description={failed
                    ? provisioning.failure_message || t('provisioning.failed_without_message')
                    : t('provisioning.current_description', {
                        phase: provisioning.phase || t('provisioning.phase_unknown'),
                        progress: provisioning.progress || t('provisioning.progress_unknown'),
                    })}
            />
            <Progress
                percent={percent}
                status={progressStatus}
                showInfo={typeof percent === 'number'}
                data-testid="vm-provisioning-progress"
            />
            <Descriptions bordered size="small" column={2} items={descriptionItems} />
            {provisioning.clone_fallback_reason ? (
                <Alert
                    type="warning"
                    showIcon
                    message={t('provisioning.clone_fallback_reason')}
                    description={provisioning.clone_fallback_reason}
                />
            ) : null}
            {conditionItems.length > 0 ? (
                <Space direction="vertical" size={8} style={{ width: '100%' }}>
                    <Text strong>{t('provisioning.conditions')}</Text>
                    <Timeline items={conditionItems} />
                </Space>
            ) : null}
            {eventItems.length > 0 ? (
                <Space direction="vertical" size={8} style={{ width: '100%' }}>
                    <Text strong>{t('provisioning.recent_events')}</Text>
                    <Timeline items={eventItems} />
                </Space>
            ) : null}
        </Space>
    );
}
