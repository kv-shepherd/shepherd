import { beforeEach, describe, expect, it, vi } from 'vitest';

import {
    clearStoredBatchRequestIntent,
    resolveStoredBatchRequestIntent,
} from './batchRequestIntentStorage';

const STORAGE_KEY = 'shepherd-vm-batch-request-intents';

const resolveIntent = (
    payload: unknown,
    nowMs: number,
    createRequestId: () => string,
) => resolveStoredBatchRequestIntent({
    actorKey: ' user-1 ',
    operationKey: ' POWER:RESTART ',
    payload,
    nowMs,
    createRequestId,
});

describe('batch request intent storage', () => {
    beforeEach(() => {
        vi.restoreAllMocks();
        window.sessionStorage.clear();
    });

    it('persists separate canonical intents and clears only the exact accepted identity', () => {
        const createRequestId = vi.fn()
            .mockReturnValueOnce('request-a')
            .mockReturnValueOnce('request-b');
        const intentA = resolveIntent({ vm_ids: ['vm-2', 'vm-1'] }, 1_000, createRequestId);
        const intentB = resolveIntent({ vm_ids: ['vm-3'] }, 2_000, createRequestId);

        expect(resolveIntent({ vm_ids: ['vm-1', 'vm-2'] }, 3_000, createRequestId))
            .toEqual(intentA);
        expect(resolveIntent({ vm_ids: ['vm-3'] }, 3_000, createRequestId))
            .toEqual(intentB);
        expect(clearStoredBatchRequestIntent({ ...intentA, requestId: 'wrong' }, 3_000))
            .toBe(false);
        expect(clearStoredBatchRequestIntent(intentA, 3_000)).toBe(true);
        expect(resolveIntent({ vm_ids: ['vm-3'] }, 3_000, createRequestId))
            .toEqual(intentB);
    });

    it('isolates the same operation and fingerprint by authenticated actor', () => {
        const createRequestId = vi.fn()
            .mockReturnValueOnce('request-user-1')
            .mockReturnValueOnce('request-user-2');
        const first = resolveStoredBatchRequestIntent({
            actorKey: 'user-1',
            operationKey: 'DELETE',
            payload: { vm_ids: ['vm-1'] },
            nowMs: 1_000,
            createRequestId,
        });
        const second = resolveStoredBatchRequestIntent({
            actorKey: 'user-2',
            operationKey: 'DELETE',
            payload: { vm_ids: ['vm-1'] },
            nowMs: 1_000,
            createRequestId,
        });

        expect(first.requestId).toBe('request-user-1');
        expect(second.requestId).toBe('request-user-2');
    });

    it.each([
        { actorKey: ' ', operationKey: 'DELETE' },
        { actorKey: 'user-1', operationKey: '\t' },
    ])('rejects incomplete storage identities before creating a request ID', (identity) => {
        const createRequestId = vi.fn().mockReturnValue('must-not-be-created');

        expect(() => resolveStoredBatchRequestIntent({
            ...identity,
            payload: { vm_ids: ['vm-1'] },
            nowMs: 1_000,
            createRequestId,
        })).toThrow('authenticated actor and batch operation are required');
        expect(createRequestId).not.toHaveBeenCalled();
        expect(window.sessionStorage.getItem(STORAGE_KEY)).toBeNull();
    });

    it('recovers from corrupt and incompatible storage without throwing', () => {
        window.sessionStorage.setItem(STORAGE_KEY, '{bad json');
        const fromCorrupt = resolveIntent({ vm_ids: ['vm-1'] }, 1_000, () => 'request-1');
        expect(fromCorrupt.requestId).toBe('request-1');

        window.sessionStorage.setItem(STORAGE_KEY, JSON.stringify({ version: 99, intents: [] }));
        const fromUnknownVersion = resolveIntent(
            { vm_ids: ['vm-2'] },
            2_000,
            () => 'request-2',
        );
        expect(fromUnknownVersion.requestId).toBe('request-2');
        expect(JSON.parse(window.sessionStorage.getItem(STORAGE_KEY) ?? '{}'))
            .toMatchObject({ version: 1 });
    });

    it('rotates expired entries and prunes them from the persisted envelope', () => {
        const createRequestId = vi.fn()
            .mockReturnValueOnce('request-old')
            .mockReturnValueOnce('request-new');
        const first = resolveIntent({ vm_ids: ['vm-1'] }, 1_000, createRequestId);
        const afterExpiry = resolveIntent(
            { vm_ids: ['vm-1'] },
            first.expiresAtMs + 1,
            createRequestId,
        );

        expect(afterExpiry.requestId).toBe('request-new');
        const stored = JSON.parse(window.sessionStorage.getItem(STORAGE_KEY) ?? '{}') as {
            intents?: Array<{ requestId: string }>;
        };
        expect(stored.intents?.map((intent) => intent.requestId)).toEqual(['request-new']);
    });

    it('bounds unresolved intent storage and evicts the oldest entry first', () => {
        for (let index = 0; index < 33; index += 1) {
            resolveIntent(
                { vm_ids: [`vm-${index}`] },
                1_000 + index,
                () => `request-${index}`,
            );
        }
        const stored = JSON.parse(window.sessionStorage.getItem(STORAGE_KEY) ?? '{}') as {
            version?: number;
            intents?: Array<{ requestId: string }>;
        };

        expect(stored.version).toBe(1);
        expect(stored.intents).toHaveLength(32);
        expect(stored.intents?.some((intent) => intent.requestId === 'request-0')).toBe(false);
        expect(stored.intents?.some((intent) => intent.requestId === 'request-32')).toBe(true);
    });

    it('reuses the exact in-memory intent when sessionStorage writes throw', async () => {
        vi.resetModules();
        const isolatedStorage = await import('./batchRequestIntentStorage');
        const createRequestId = vi.fn().mockReturnValue('request-write-fallback');
        vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
            throw new DOMException('quota exceeded', 'QuotaExceededError');
        });

        const args = {
            actorKey: 'user-write-fallback',
            operationKey: 'DELETE',
            payload: { vm_ids: ['vm-1'] },
            nowMs: 1_000,
            createRequestId,
        };
        const first = isolatedStorage.resolveStoredBatchRequestIntent(args);
        const retry = isolatedStorage.resolveStoredBatchRequestIntent({
            ...args,
            nowMs: 2_000,
        });

        expect(retry).toEqual(first);
        expect(createRequestId).toHaveBeenCalledTimes(1);
    });

    it('reuses the exact in-memory intent when sessionStorage reads throw', async () => {
        vi.resetModules();
        const isolatedStorage = await import('./batchRequestIntentStorage');
        const createRequestId = vi.fn().mockReturnValue('request-read-fallback');
        vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
            throw new DOMException('storage blocked', 'SecurityError');
        });

        const args = {
            actorKey: 'user-read-fallback',
            operationKey: 'POWER:START',
            payload: { vm_ids: ['vm-2'] },
            nowMs: 1_000,
            createRequestId,
        };
        const first = isolatedStorage.resolveStoredBatchRequestIntent(args);
        const retry = isolatedStorage.resolveStoredBatchRequestIntent({
            ...args,
            nowMs: 2_000,
        });

        expect(retry).toEqual(first);
        expect(createRequestId).toHaveBeenCalledTimes(1);
    });
});
