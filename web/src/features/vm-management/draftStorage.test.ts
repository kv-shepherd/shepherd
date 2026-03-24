import { describe, expect, it, beforeEach } from 'vitest';

import {
    buildVMRequestDraftStorageKey,
    clearVMRequestDraft,
    hasMeaningfulVMRequestDraft,
    loadVMRequestDraft,
    resolveVMRequestDraftOwner,
    saveVMRequestDraft,
} from './draftStorage';

describe('draftStorage', () => {
    beforeEach(() => {
        window.localStorage.clear();
    });

    it('resolves a stable owner from user identity', () => {
        expect(resolveVMRequestDraftOwner({ id: 'u-1', username: 'alice' })).toBe('u-1');
        expect(resolveVMRequestDraftOwner({ username: 'alice' })).toBe('alice');
        expect(resolveVMRequestDraftOwner(null)).toBe('');
    });

    it('persists and restores a vm request draft', () => {
        saveVMRequestDraft('u-1', {
            version: 1,
            systemId: 'sys-1',
            systemLabel: 'System A',
            serviceId: 'svc-1',
            serviceLabel: 'Service A',
            templateId: 'tpl-1',
            templateLabel: 'Ubuntu 22.04',
            instanceSizeId: 'size-1',
            instanceSizeLabel: 'M4 Large',
            namespace: 'team-prod',
            reason: 'Need a VM',
            batchCount: 2,
            wizardStep: 3,
            requestMode: 'full',
            updatedAt: '2026-03-16T12:00:00Z',
        });

        expect(loadVMRequestDraft('u-1')).toEqual({
            version: 1,
            systemId: 'sys-1',
            systemLabel: 'System A',
            serviceId: 'svc-1',
            serviceLabel: 'Service A',
            templateId: 'tpl-1',
            templateLabel: 'Ubuntu 22.04',
            instanceSizeId: 'size-1',
            instanceSizeLabel: 'M4 Large',
            namespace: 'team-prod',
            reason: 'Need a VM',
            batchCount: 2,
            wizardStep: 3,
            requestMode: 'full',
            updatedAt: '2026-03-16T12:00:00Z',
        });
    });

    it('clears a persisted draft', () => {
        window.localStorage.setItem(
            buildVMRequestDraftStorageKey('u-1'),
            JSON.stringify({
                serviceId: 'svc-1',
                batchCount: 1,
                wizardStep: 1,
                updatedAt: '2026-03-16T12:00:00Z',
            })
        );

        clearVMRequestDraft('u-1');

        expect(loadVMRequestDraft('u-1')).toBeNull();
    });

    it('drops empty or malformed drafts instead of reviving garbage state', () => {
        window.localStorage.setItem(buildVMRequestDraftStorageKey('u-1'), '{bad json');
        expect(loadVMRequestDraft('u-1')).toBeNull();

        window.localStorage.setItem(
            buildVMRequestDraftStorageKey('u-1'),
            JSON.stringify({ batchCount: 1, wizardStep: 0, updatedAt: '2026-03-16T12:00:00Z' })
        );
        expect(loadVMRequestDraft('u-1')).toBeNull();
    });

    it('only treats non-empty request content as meaningful draft state', () => {
        expect(hasMeaningfulVMRequestDraft({ batchCount: 1 })).toBe(false);
        expect(hasMeaningfulVMRequestDraft({ reason: 'Need a VM' })).toBe(true);
        expect(hasMeaningfulVMRequestDraft({ batchCount: 3 })).toBe(true);
    });
});
