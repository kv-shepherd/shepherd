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
            batch_id: 'batch-1',
            status_url: '/vms/batch/batch-1',
            kind: 'request',
        });

        expect(readStoredActiveBatchState()).toEqual({
            batch_id: 'batch-1',
            status_url: '/vms/batch/batch-1',
            kind: 'request',
        });
    });

    it('treats missing batch kind as legacy state and restores it safely', () => {
        window.sessionStorage.setItem(
            ACTIVE_BATCH_STORAGE_KEY,
            JSON.stringify({
                batch_id: 'batch-legacy',
                status_url: '/vms/batch/batch-legacy',
            }),
        );

        expect(readStoredActiveBatchState()).toEqual({
            batch_id: 'batch-legacy',
            status_url: '/vms/batch/batch-legacy',
            kind: '',
        });
    });

    it('clears empty or invalid state instead of reviving garbage', () => {
        window.sessionStorage.setItem(ACTIVE_BATCH_STORAGE_KEY, '{bad json');
        expect(readStoredActiveBatchState()).toEqual({
            batch_id: '',
            status_url: '',
            kind: '',
        });

        saveStoredActiveBatchState({
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
            batch_id: 'batch-1',
            status_url: '/vms/batch/batch-1',
            kind: 'job',
        });
        clearStoredActiveBatchState();

        expect(listener).toHaveBeenCalledTimes(2);
        window.removeEventListener(ACTIVE_BATCH_CHANGED_EVENT, listener);
    });
});
