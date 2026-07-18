import { beforeEach, describe, expect, it, vi } from 'vitest';

import {
    ACTIVE_BATCH_CHANGED_EVENT,
    ACTIVE_BATCH_STORAGE_KEY,
    clearStoredActiveBatchState,
    readStoredActiveBatchState,
    saveStoredActiveBatchState,
} from './activeBatchTracking';

describe('activeBatchTracking', () => {
    beforeEach(() => {
        window.sessionStorage.clear();
    });

    it('persists and restores the active batch state', () => {
        saveStoredActiveBatchState({
            actor_id: 'actor-1',
            batch_id: 'batch-1',
            status_url: '/vms/batch/batch-1',
            kind: 'request',
        });

        expect(readStoredActiveBatchState('actor-1')).toEqual({
            actor_id: 'actor-1',
            batch_id: 'batch-1',
            status_url: '/vms/batch/batch-1',
            kind: 'request',
        });
    });

    it('rejects legacy state without an actor owner', () => {
        window.sessionStorage.setItem(
            ACTIVE_BATCH_STORAGE_KEY,
            JSON.stringify({
                batch_id: 'batch-legacy',
                status_url: '/vms/batch/batch-legacy',
            }),
        );

        expect(readStoredActiveBatchState('actor-1')).toEqual({
            actor_id: '',
            batch_id: '',
            status_url: '',
            kind: '',
        });
        expect(window.sessionStorage.getItem(ACTIVE_BATCH_STORAGE_KEY)).toBeNull();
    });

    it('rejects and removes state owned by another actor', () => {
        saveStoredActiveBatchState({
            actor_id: 'actor-1',
            batch_id: 'batch-1',
            status_url: '/vms/batch/batch-1',
            kind: 'job',
        });

        expect(readStoredActiveBatchState('actor-2')).toEqual({
            actor_id: '',
            batch_id: '',
            status_url: '',
            kind: '',
        });
        expect(window.sessionStorage.getItem(ACTIVE_BATCH_STORAGE_KEY)).toBeNull();
    });

    it('leaves owned state untouched while the authenticated actor is unknown', () => {
        saveStoredActiveBatchState({
            actor_id: 'actor-1',
            batch_id: 'batch-1',
            status_url: '/vms/batch/batch-1',
            kind: 'request',
        });
        const raw = window.sessionStorage.getItem(ACTIVE_BATCH_STORAGE_KEY);

        expect(readStoredActiveBatchState('')).toEqual({
            actor_id: '',
            batch_id: '',
            status_url: '',
            kind: '',
        });
        expect(window.sessionStorage.getItem(ACTIVE_BATCH_STORAGE_KEY)).toBe(raw);
    });

    it('clears empty or invalid state instead of reviving garbage', () => {
        window.sessionStorage.setItem(ACTIVE_BATCH_STORAGE_KEY, '{bad json');
        expect(readStoredActiveBatchState('actor-1')).toEqual({
            actor_id: '',
            batch_id: '',
            status_url: '',
            kind: '',
        });

        saveStoredActiveBatchState({
            actor_id: 'actor-1',
            batch_id: '',
            status_url: '/vms/batch/batch-1',
            kind: 'job',
        });
        expect(window.sessionStorage.getItem(ACTIVE_BATCH_STORAGE_KEY)).toBeNull();
    });

    it('dispatches a change event when the active batch changes', () => {
        const listener = vi.fn();
        window.addEventListener(ACTIVE_BATCH_CHANGED_EVENT, listener);

        saveStoredActiveBatchState({
            actor_id: 'actor-1',
            batch_id: 'batch-1',
            status_url: '/vms/batch/batch-1',
            kind: 'job',
        });
        clearStoredActiveBatchState();

        expect(listener).toHaveBeenCalledTimes(2);
        window.removeEventListener(ACTIVE_BATCH_CHANGED_EVENT, listener);
    });
});
