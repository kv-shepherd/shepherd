'use client';

export type ActiveBatchKind = 'request' | 'job';

interface StoredActiveBatchState {
    batch_id: string;
    status_url: string;
    kind: ActiveBatchKind | '';
}

export const ACTIVE_BATCH_STORAGE_KEY = 'shepherd-active-batch';
export const ACTIVE_BATCH_CHANGED_EVENT = 'shepherd:active-batch-changed';

function trimString(value: unknown): string {
    return typeof value === 'string' ? value.trim() : '';
}

function trimKind(value: unknown): ActiveBatchKind | '' {
    return value === 'request' || value === 'job' ? value : '';
}

function notifyActiveBatchChange() {
    if (typeof window === 'undefined') {
        return;
    }
    window.dispatchEvent(new Event(ACTIVE_BATCH_CHANGED_EVENT));
}

export function readStoredActiveBatchState(): StoredActiveBatchState {
    if (typeof window === 'undefined') {
        return { batch_id: '', status_url: '', kind: '' };
    }

    const raw = window.sessionStorage.getItem(ACTIVE_BATCH_STORAGE_KEY);
    if (!raw) {
        return { batch_id: '', status_url: '', kind: '' };
    }

    try {
        const parsed = JSON.parse(raw) as Partial<StoredActiveBatchState>;
        return {
            batch_id: trimString(parsed.batch_id),
            status_url: trimString(parsed.status_url),
            kind: trimKind(parsed.kind),
        };
    } catch {
        window.sessionStorage.removeItem(ACTIVE_BATCH_STORAGE_KEY);
        return { batch_id: '', status_url: '', kind: '' };
    }
}

export function saveStoredActiveBatchState(state: StoredActiveBatchState) {
    if (typeof window === 'undefined') {
        return;
    }

    const batchID = trimString(state.batch_id);
    const statusURL = trimString(state.status_url);
    const kind = trimKind(state.kind);
    if (batchID === '') {
        window.sessionStorage.removeItem(ACTIVE_BATCH_STORAGE_KEY);
        notifyActiveBatchChange();
        return;
    }

    window.sessionStorage.setItem(
        ACTIVE_BATCH_STORAGE_KEY,
        JSON.stringify({
            batch_id: batchID,
            status_url: statusURL,
            kind,
        }),
    );
    notifyActiveBatchChange();
}

export function clearStoredActiveBatchState() {
    if (typeof window === 'undefined') {
        return;
    }
    window.sessionStorage.removeItem(ACTIVE_BATCH_STORAGE_KEY);
    notifyActiveBatchChange();
}
