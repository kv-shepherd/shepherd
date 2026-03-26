import type { components } from '@/types/api.gen';

export type VM = components['schemas']['VM'];
export type VMList = components['schemas']['VMList'];
export type VMCreateRequest = components['schemas']['VMCreateRequest'];
export type VMRequestContext = components['schemas']['VMRequestContext'];
export type VMRequestPrefill = components['schemas']['VMRequestPrefill'];
export type VMModifyContext = components['schemas']['VMModifyContext'];
export type VMModifyRequest = components['schemas']['VMModifyRequest'];
export type VMPlacementHint = components['schemas']['VMPlacementHint'];
export type SystemList = components['schemas']['SystemList'];
export type ServiceList = components['schemas']['ServiceList'];
export type TemplateList = components['schemas']['TemplateList'];
export type InstanceSizeList = components['schemas']['InstanceSizeList'];
export type Template = components['schemas']['Template'];
export type InstanceSize = components['schemas']['InstanceSize'];
export type TicketResponse = components['schemas']['TicketResponse'];
export type DeleteVMResponse = components['schemas']['DeleteVMResponse'];
export type VMConsoleRequestResponse = components['schemas']['VMConsoleRequestResponse'];
export type VMVNCSessionResponse = components['schemas']['VMVNCSessionResponse'];
export type VMBatchSubmitRequest = components['schemas']['VMBatchSubmitRequest'];
export type VMBatchPowerRequest = components['schemas']['VMBatchPowerRequest'];
export type VMBatchSubmitResponse = components['schemas']['VMBatchSubmitResponse'];
export type VMBatchStatusResponse = components['schemas']['VMBatchStatusResponse'];
export type VMBatchActionResponse = components['schemas']['VMBatchActionResponse'];
export type VMBatchPowerAction = components['schemas']['VMBatchPowerAction'];
export type VMRequestMode = 'guided' | 'full';

export interface VMRequestLaunchPrefill {
    systemId?: string;
    serviceId?: string;
    templateId?: string;
    instanceSizeId?: string;
    namespace?: string;
    reason?: string;
    batchCount?: number;
    requestMode?: VMRequestMode;
}

export interface VMRequestDraft {
    version: 1;
    systemId?: string;
    systemLabel?: string;
    serviceId?: string;
    serviceLabel?: string;
    templateId?: string;
    templateLabel?: string;
    instanceSizeId?: string;
    instanceSizeLabel?: string;
    namespace?: string;
    reason?: string;
    batchCount: number;
    wizardStep: number;
    requestMode?: VMRequestMode;
    updatedAt: string;
}

export const VM_STATUS_MAP: Record<
    string,
    { color: string; badge: 'success' | 'processing' | 'error' | 'warning' | 'default' }
> = {
    CREATING: { color: 'cyan', badge: 'processing' },
    STARTING: { color: 'cyan', badge: 'processing' },
    RUNNING: { color: 'green', badge: 'success' },
    STOPPING: { color: 'orange', badge: 'warning' },
    STOPPED: { color: 'default', badge: 'default' },
    DELETING: { color: 'orange', badge: 'warning' },
    FAILED: { color: 'red', badge: 'error' },
    PENDING: { color: 'gold', badge: 'warning' },
    MIGRATING: { color: 'blue', badge: 'processing' },
    PAUSED: { color: 'purple', badge: 'warning' },
    UNKNOWN: { color: 'default', badge: 'default' },
    NOT_FOUND: { color: 'red', badge: 'error' },
};

/**
 * Formats memory in GiB for display.
 * Values are already in Gi from the API (post int→float64 migration).
 */
export const formatMemory = (memoryGi: number): string => {
    if (!Number.isFinite(memoryGi) || memoryGi <= 0) {
        return '0 Gi';
    }
    return `${Number.isInteger(memoryGi) ? memoryGi : memoryGi.toFixed(1)} Gi`;
};
