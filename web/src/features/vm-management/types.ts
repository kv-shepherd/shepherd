import type { components } from '@/types/api.gen';

export type VM = components['schemas']['VM'];
export type VMList = components['schemas']['VMList'];
export type VMCreateRequest = components['schemas']['VMCreateRequest'];
export type VMRequestContext = components['schemas']['VMRequestContext'];
export type SystemList = components['schemas']['SystemList'];
export type ServiceList = components['schemas']['ServiceList'];
export type TemplateList = components['schemas']['TemplateList'];
export type InstanceSizeList = components['schemas']['InstanceSizeList'];
export type Template = components['schemas']['Template'];
export type InstanceSize = components['schemas']['InstanceSize'];
export type ApprovalTicketResponse = components['schemas']['ApprovalTicketResponse'];
export type DeleteVMResponse = components['schemas']['DeleteVMResponse'];
export type VMConsoleRequestResponse = components['schemas']['VMConsoleRequestResponse'];
export type VMConsoleStatusResponse = components['schemas']['VMConsoleStatusResponse'];
export type VMVNCSessionResponse = components['schemas']['VMVNCSessionResponse'];
export type VMBatchSubmitRequest = components['schemas']['VMBatchSubmitRequest'];
export type VMBatchPowerRequest = components['schemas']['VMBatchPowerRequest'];
export type VMBatchSubmitResponse = components['schemas']['VMBatchSubmitResponse'];
export type VMBatchStatusResponse = components['schemas']['VMBatchStatusResponse'];
export type VMBatchActionResponse = components['schemas']['VMBatchActionResponse'];
export type VMBatchPowerAction = components['schemas']['VMBatchPowerAction'];

export const VM_STATUS_MAP: Record<
    string,
    { color: string; badge: 'success' | 'processing' | 'error' | 'warning' | 'default' }
> = {
    CREATING: { color: 'cyan', badge: 'processing' },
    RUNNING: { color: 'green', badge: 'success' },
    STOPPING: { color: 'orange', badge: 'warning' },
    STOPPED: { color: 'default', badge: 'default' },
    DELETING: { color: 'orange', badge: 'warning' },
    FAILED: { color: 'red', badge: 'error' },
    PENDING: { color: 'gold', badge: 'warning' },
    MIGRATING: { color: 'blue', badge: 'processing' },
    PAUSED: { color: 'purple', badge: 'warning' },
    UNKNOWN: { color: 'default', badge: 'default' },
};

export const formatMemory = (memoryMb: number): string => {
    if (!Number.isFinite(memoryMb) || memoryMb <= 0) {
        return '0 MB';
    }
    if (memoryMb % 1024 === 0) {
        return `${memoryMb / 1024} Gi`;
    }
    return `${memoryMb} MB`;
};
