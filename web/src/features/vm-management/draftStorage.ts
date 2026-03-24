'use client';

import type { VMRequestDraft } from './types';

const VM_REQUEST_DRAFT_STORAGE_PREFIX = 'shepherd-vm-request-draft';
export const VM_REQUEST_DRAFT_CHANGED_EVENT = 'shepherd:vm-request-draft-changed';

function trimString(value: unknown): string | undefined {
    if (typeof value !== 'string') {
        return undefined;
    }
    const trimmed = value.trim();
    return trimmed === '' ? undefined : trimmed;
}

function normalizePositiveInteger(value: unknown, fallback: number): number {
    const n = Number(value);
    if (!Number.isFinite(n) || n < 1) {
        return fallback;
    }
    return Math.max(1, Math.floor(n));
}

function normalizeWizardStep(value: unknown): number {
    const n = Number(value);
    if (!Number.isFinite(n) || n < 0) {
        return 0;
    }
    return Math.max(0, Math.min(4, Math.floor(n)));
}

function normalizeRequestMode(value: unknown): VMRequestDraft['requestMode'] {
    return value === 'full' ? 'full' : 'guided';
}

export function resolveVMRequestDraftOwner(user?: {
    id?: string | null;
    username?: string | null;
} | null): string {
    const id = trimString(user?.id);
    if (id) {
        return id;
    }
    return trimString(user?.username) ?? '';
}

export function buildVMRequestDraftStorageKey(owner: string): string {
    return `${VM_REQUEST_DRAFT_STORAGE_PREFIX}:${owner}`;
}

export function hasMeaningfulVMRequestDraft(draft: Partial<VMRequestDraft>): boolean {
    return Boolean(
        trimString(draft.systemId) ||
        trimString(draft.serviceId) ||
        trimString(draft.templateId) ||
        trimString(draft.instanceSizeId) ||
        trimString(draft.namespace) ||
        trimString(draft.reason) ||
        normalizePositiveInteger(draft.batchCount, 1) > 1
    );
}

function notifyDraftChange() {
    if (typeof window === 'undefined') {
        return;
    }
    window.dispatchEvent(new Event(VM_REQUEST_DRAFT_CHANGED_EVENT));
}

export function loadVMRequestDraft(owner: string): VMRequestDraft | null {
    if (typeof window === 'undefined' || owner.trim() === '') {
        return null;
    }

    const raw = window.localStorage.getItem(buildVMRequestDraftStorageKey(owner));
    if (!raw) {
        return null;
    }

    try {
        const parsed = JSON.parse(raw) as Partial<VMRequestDraft>;
        const draft: VMRequestDraft = {
            version: 1,
            systemId: trimString(parsed.systemId),
            systemLabel: trimString(parsed.systemLabel),
            serviceId: trimString(parsed.serviceId),
            serviceLabel: trimString(parsed.serviceLabel),
            templateId: trimString(parsed.templateId),
            templateLabel: trimString(parsed.templateLabel),
            instanceSizeId: trimString(parsed.instanceSizeId),
            instanceSizeLabel: trimString(parsed.instanceSizeLabel),
            namespace: trimString(parsed.namespace),
            reason: trimString(parsed.reason),
            batchCount: normalizePositiveInteger(parsed.batchCount, 1),
            wizardStep: normalizeWizardStep(parsed.wizardStep),
            requestMode: normalizeRequestMode(parsed.requestMode),
            updatedAt: trimString(parsed.updatedAt) ?? new Date(0).toISOString(),
        };

        if (!hasMeaningfulVMRequestDraft(draft)) {
            window.localStorage.removeItem(buildVMRequestDraftStorageKey(owner));
            return null;
        }

        return draft;
    } catch {
        window.localStorage.removeItem(buildVMRequestDraftStorageKey(owner));
        return null;
    }
}

export function saveVMRequestDraft(owner: string, draft: VMRequestDraft) {
    if (typeof window === 'undefined' || owner.trim() === '' || !hasMeaningfulVMRequestDraft(draft)) {
        return;
    }

    window.localStorage.setItem(
        buildVMRequestDraftStorageKey(owner),
        JSON.stringify({
            ...draft,
            version: 1,
            batchCount: normalizePositiveInteger(draft.batchCount, 1),
            wizardStep: normalizeWizardStep(draft.wizardStep),
            requestMode: normalizeRequestMode(draft.requestMode),
            updatedAt: trimString(draft.updatedAt) ?? new Date().toISOString(),
        })
    );
    notifyDraftChange();
}

export function clearVMRequestDraft(owner: string) {
    if (typeof window === 'undefined' || owner.trim() === '') {
        return;
    }
    window.localStorage.removeItem(buildVMRequestDraftStorageKey(owner));
    notifyDraftChange();
}
