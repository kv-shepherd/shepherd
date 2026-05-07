import type { components } from '@/types/api.gen';

export type VMProvisioningStatus = components['schemas']['ProvisioningStatus'];

type VMProvisioningSource = {
    status?: string;
    provisioning?: VMProvisioningStatus;
};

const ACTIVE_VM_STATUSES = new Set(['CREATING', 'STARTING', 'PENDING']);
const TERMINAL_PROVISIONING_PHASES = new Set(['failed', 'ready', 'succeeded', 'success', 'complete', 'completed']);

export function parseProvisioningPercent(progress?: string): number | undefined {
    if (!progress) {
        return undefined;
    }
    const match = progress.trim().match(/^(\d+(?:\.\d+)?)%$/);
    if (!match) {
        return undefined;
    }
    const value = Number(match[1]);
    if (!Number.isFinite(value)) {
        return undefined;
    }
    return Math.min(100, Math.max(0, Math.round(value)));
}

export function hasVisibleProvisioningStatus(provisioning?: VMProvisioningStatus): boolean {
    if (!provisioning) {
        return false;
    }
    return [
        provisioning.root_data_volume_name,
        provisioning.claim_name,
        provisioning.phase,
        provisioning.progress,
        provisioning.pvc_phase,
        provisioning.clone_type,
        provisioning.clone_phase,
        provisioning.clone_fallback_reason,
        provisioning.failure_message,
    ].some((value) => typeof value === 'string' && value.trim() !== '')
        || typeof provisioning.restart_count === 'number'
        || Boolean(provisioning.conditions?.length)
        || Boolean(provisioning.recent_events?.length);
}

export function hasProvisioningFailure(provisioning?: VMProvisioningStatus): boolean {
    if (!provisioning) {
        return false;
    }
    const phase = provisioning.phase?.trim().toLowerCase();
    return phase === 'failed';
}

export function isVMProvisioningPollingActive(vm?: VMProvisioningSource): boolean {
    if (!vm) {
        return false;
    }
    if (hasProvisioningFailure(vm.provisioning)) {
        return false;
    }
    if (ACTIVE_VM_STATUSES.has(vm.status ?? '')) {
        return true;
    }
    const phase = vm.provisioning?.phase?.trim().toLowerCase();
    if (!phase) {
        return false;
    }
    return !TERMINAL_PROVISIONING_PHASES.has(phase);
}
