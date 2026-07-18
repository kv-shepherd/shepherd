import {
    createCanonicalBatchIntentFingerprint,
    createOpaqueBatchRequestId,
} from './batchActions';

const BATCH_REQUEST_INTENTS_STORAGE_KEY = 'shepherd-vm-batch-request-intents';
const BATCH_REQUEST_INTENTS_VERSION = 1;
const BATCH_REQUEST_INTENT_TTL_MS = 24 * 60 * 60 * 1_000;
const BATCH_REQUEST_INTENT_CAPACITY = 32;

export interface StoredBatchRequestIntent {
    actorKey: string;
    operationKey: string;
    fingerprint: string;
    requestId: string;
    createdAtMs: number;
    expiresAtMs: number;
}

interface StoredBatchRequestIntentEnvelope {
    version: typeof BATCH_REQUEST_INTENTS_VERSION;
    intents: StoredBatchRequestIntent[];
}

interface ResolveStoredBatchRequestIntentArgs {
    actorKey: string;
    operationKey: string;
    payload: unknown;
    nowMs?: number;
    createRequestId?: () => string;
}

let volatileIntents: StoredBatchRequestIntent[] = [];
let sessionStorageUnavailable = false;

const normalizeKey = (value: string): string => value.trim();

const intentStorageIdentity = (
    intent: Pick<StoredBatchRequestIntent, 'actorKey' | 'operationKey' | 'fingerprint'>,
): string => JSON.stringify([
    intent.actorKey,
    intent.operationKey,
    intent.fingerprint,
]);

const isStoredBatchRequestIntent = (value: unknown): value is StoredBatchRequestIntent => {
    if (!value || typeof value !== 'object') {
        return false;
    }
    const candidate = value as Partial<StoredBatchRequestIntent>;
    return typeof candidate.actorKey === 'string'
        && candidate.actorKey.trim() !== ''
        && typeof candidate.operationKey === 'string'
        && candidate.operationKey.trim() !== ''
        && typeof candidate.fingerprint === 'string'
        && candidate.fingerprint !== ''
        && typeof candidate.requestId === 'string'
        && candidate.requestId.trim() !== ''
        && typeof candidate.createdAtMs === 'number'
        && Number.isFinite(candidate.createdAtMs)
        && typeof candidate.expiresAtMs === 'number'
        && Number.isFinite(candidate.expiresAtMs)
        && candidate.expiresAtMs > candidate.createdAtMs;
};

const compactStoredIntents = (
    candidates: readonly unknown[],
    nowMs: number,
): StoredBatchRequestIntent[] => {
    const byIdentity = new Map<string, StoredBatchRequestIntent>();
    for (const candidate of candidates) {
        if (!isStoredBatchRequestIntent(candidate) || candidate.expiresAtMs <= nowMs) {
            continue;
        }
        const normalized: StoredBatchRequestIntent = {
            ...candidate,
            actorKey: normalizeKey(candidate.actorKey),
            operationKey: normalizeKey(candidate.operationKey),
            requestId: candidate.requestId.trim(),
        };
        const identity = intentStorageIdentity(normalized);
        const existing = byIdentity.get(identity);
        if (!existing || normalized.createdAtMs > existing.createdAtMs) {
            byIdentity.set(identity, normalized);
        }
    }
    return Array.from(byIdentity.values())
        .sort((left, right) => left.createdAtMs - right.createdAtMs)
        .slice(-BATCH_REQUEST_INTENT_CAPACITY);
};

const readVolatileIntents = (nowMs: number): StoredBatchRequestIntent[] => {
    volatileIntents = compactStoredIntents(volatileIntents, nowMs);
    return volatileIntents;
};

const writeStoredIntents = (intents: StoredBatchRequestIntent[]): void => {
    if (typeof window === 'undefined') {
        return;
    }
    volatileIntents = intents;
    if (sessionStorageUnavailable) {
        return;
    }
    try {
        if (intents.length === 0) {
            window.sessionStorage.removeItem(BATCH_REQUEST_INTENTS_STORAGE_KEY);
            return;
        }
        const envelope: StoredBatchRequestIntentEnvelope = {
            version: BATCH_REQUEST_INTENTS_VERSION,
            intents,
        };
        window.sessionStorage.setItem(
            BATCH_REQUEST_INTENTS_STORAGE_KEY,
            JSON.stringify(envelope),
        );
    } catch {
        // Storage can be unavailable or full. Keep exact unresolved intents in
        // module memory so retries in this page lifetime remain idempotent.
        sessionStorageUnavailable = true;
    }
};

const readStoredIntents = (nowMs: number): StoredBatchRequestIntent[] => {
    if (typeof window === 'undefined') {
        return [];
    }
    if (sessionStorageUnavailable) {
        return readVolatileIntents(nowMs);
    }
    let raw: string | null = null;
    try {
        raw = window.sessionStorage.getItem(BATCH_REQUEST_INTENTS_STORAGE_KEY);
    } catch {
        sessionStorageUnavailable = true;
        return readVolatileIntents(nowMs);
    }
    if (!raw) {
        volatileIntents = [];
        return [];
    }

    try {
        const parsed = JSON.parse(raw) as Partial<StoredBatchRequestIntentEnvelope>;
        if (
            parsed.version !== BATCH_REQUEST_INTENTS_VERSION
            || !Array.isArray(parsed.intents)
        ) {
            writeStoredIntents([]);
            return [];
        }

        const intents = compactStoredIntents(parsed.intents, nowMs);
        if (JSON.stringify(parsed.intents) !== JSON.stringify(intents)) {
            writeStoredIntents(intents);
        } else {
            volatileIntents = intents;
        }
        return intents;
    } catch {
        writeStoredIntents([]);
        return [];
    }
};

export const resolveStoredBatchRequestIntent = ({
    actorKey,
    operationKey,
    payload,
    nowMs = Date.now(),
    createRequestId = createOpaqueBatchRequestId,
}: ResolveStoredBatchRequestIntentArgs): StoredBatchRequestIntent => {
    const normalizedActorKey = normalizeKey(actorKey);
    const normalizedOperationKey = normalizeKey(operationKey);
    if (normalizedActorKey === '' || normalizedOperationKey === '') {
        throw new Error('authenticated actor and batch operation are required');
    }
    const fingerprint = createCanonicalBatchIntentFingerprint(payload);
    const intents = readStoredIntents(nowMs);
    const existing = intents.find((intent) =>
        intent.actorKey === normalizedActorKey
        && intent.operationKey === normalizedOperationKey
        && intent.fingerprint === fingerprint);
    if (existing) {
        return existing;
    }

    const created: StoredBatchRequestIntent = {
        actorKey: normalizedActorKey,
        operationKey: normalizedOperationKey,
        fingerprint,
        requestId: createRequestId(),
        createdAtMs: nowMs,
        expiresAtMs: nowMs + BATCH_REQUEST_INTENT_TTL_MS,
    };
    writeStoredIntents(
        [...intents, created]
            .sort((left, right) => left.createdAtMs - right.createdAtMs)
            .slice(-BATCH_REQUEST_INTENT_CAPACITY),
    );
    return created;
};

export const clearStoredBatchRequestIntent = (
    accepted: StoredBatchRequestIntent,
    nowMs = Date.now(),
): boolean => {
    const intents = readStoredIntents(nowMs);
    const acceptedIdentity = intentStorageIdentity(accepted);
    let cleared = false;
    const remaining = intents.filter((intent) => {
        const matches = intentStorageIdentity(intent) === acceptedIdentity
            && intent.requestId === accepted.requestId;
        cleared ||= matches;
        return !matches;
    });
    if (cleared) {
        writeStoredIntents(remaining);
    }
    return cleared;
};

export const isSameStoredBatchRequestIntent = (
    left: StoredBatchRequestIntent | null | undefined,
    right: StoredBatchRequestIntent | null | undefined,
): boolean => Boolean(left && right
    && intentStorageIdentity(left) === intentStorageIdentity(right)
    && left.requestId === right.requestId);
