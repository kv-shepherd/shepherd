'use client';

export type ActiveBatchKind = 'request' | 'job';

interface StoredActiveBatchState {
    actor_id: string;
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

function emptyStoredActiveBatchState(): StoredActiveBatchState {
    return { actor_id: '', batch_id: '', status_url: '', kind: '' };
}

function notifyActiveBatchChange() {
    if (typeof window === 'undefined') {
        return;
    }
    window.dispatchEvent(new Event(ACTIVE_BATCH_CHANGED_EVENT));
}

export function readStoredActiveBatchState(actorID: string): StoredActiveBatchState {
    const normalizedActorID = trimString(actorID);
    if (typeof window === 'undefined') {
        return emptyStoredActiveBatchState();
    }
    if (normalizedActorID === '') {
        // Authentication can hydrate after protected route modules render.
        // Leave a previously authenticated actor's state untouched until the
        // current actor is known and can be compared safely.
        return emptyStoredActiveBatchState();
    }

    const raw = window.sessionStorage.getItem(ACTIVE_BATCH_STORAGE_KEY);
    if (!raw) {
        return emptyStoredActiveBatchState();
    }

    try {
        const parsed = JSON.parse(raw) as Partial<StoredActiveBatchState>;
        const storedActorID = trimString(parsed.actor_id);
        if (storedActorID === '' || storedActorID !== normalizedActorID) {
            window.sessionStorage.removeItem(ACTIVE_BATCH_STORAGE_KEY);
            return emptyStoredActiveBatchState();
        }
        return {
            actor_id: storedActorID,
            batch_id: trimString(parsed.batch_id),
            status_url: trimString(parsed.status_url),
            kind: trimKind(parsed.kind),
        };
    } catch {
        window.sessionStorage.removeItem(ACTIVE_BATCH_STORAGE_KEY);
        return emptyStoredActiveBatchState();
    }
}

export function saveStoredActiveBatchState(state: StoredActiveBatchState) {
    if (typeof window === 'undefined') {
        return;
    }

    const batchID = trimString(state.batch_id);
    const actorID = trimString(state.actor_id);
    const statusURL = trimString(state.status_url);
    const kind = trimKind(state.kind);
    if (actorID === '' || batchID === '') {
        window.sessionStorage.removeItem(ACTIVE_BATCH_STORAGE_KEY);
        notifyActiveBatchChange();
        return;
    }

    window.sessionStorage.setItem(
        ACTIVE_BATCH_STORAGE_KEY,
        JSON.stringify({
            actor_id: actorID,
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
