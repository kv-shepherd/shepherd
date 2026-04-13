import { act, renderHook } from '@testing-library/react';
import type { TFunction } from 'i18next';
import { beforeEach, describe, expect, it, vi } from 'vitest';

const {
    useApiGetMock,
    useApiActionMock,
    messageSuccessMock,
    messageErrorMock,
} = vi.hoisted(() => ({
    useApiGetMock: vi.fn(),
    useApiActionMock: vi.fn(),
    messageSuccessMock: vi.fn(),
    messageErrorMock: vi.fn(),
}));

vi.mock('@/lib/api/useApiGet', () => ({
    useApiGet: (...args: unknown[]) => useApiGetMock(...args),
}));

vi.mock('@/hooks/useApiQuery', () => ({
    useApiAction: (...args: unknown[]) => useApiActionMock(...args),
}));

vi.mock('@/lib/hooks/useMessage', () => ({
    useMessage: () => ({
        messageApi: {
            success: messageSuccessMock,
            error: messageErrorMock,
        },
        messageContextHolder: null,
    }),
}));

vi.mock('@/lib/api/client', () => ({
    api: {
        GET: vi.fn(),
        POST: vi.fn(),
    },
}));

vi.mock('@/stores/auth', () => ({
    useAuthStore: (selector: (state: { user: { id: string; username: string } }) => unknown) =>
        selector({ user: { id: 'u-alice', username: 'alice' } }),
}));

import { buildVMRequestDraftStorageKey } from '@/features/vm-management/draftStorage';
import { useMyRequestsController } from './useMyRequestsController';

describe('useMyRequestsController', () => {
    const t = ((key: string) => key) as unknown as TFunction;

    beforeEach(() => {
        vi.clearAllMocks();
        window.localStorage.clear();
        window.sessionStorage.clear();
        useApiGetMock.mockReturnValue({
            data: { items: [], pagination: { total: 0, page: 1, per_page: 20 } },
            isLoading: false,
            refetch: vi.fn(),
        });
        useApiActionMock.mockReturnValue({
            mutate: vi.fn(),
            isPending: false,
        });
    });

    it('stores a reusable history request as a full-form vm draft', () => {
        const { result } = renderHook(() => useMyRequestsController({ t }));

        let prepared = false;
        act(() => {
            prepared = result.current.prepareHistoryReuse({
                id: 'ticket-approved-1',
                operation_type: 'CREATE',
                status: 'APPROVED',
                requester: 'alice',
                created_at: '2026-03-16T00:00:00Z',
                request_prefill: {
                    system_id: 'sys-1',
                    service_id: 'svc-1',
                    template_id: 'tpl-1',
                    instance_size_id: 'size-1',
                    namespace: 'team-prod',
                    reason: 'Need a VM',
                    batch_count: 2,
                },
            });
        });

        expect(prepared).toBe(true);
        expect(result.current.savedVmDraft).toEqual(
            expect.objectContaining({
                systemId: 'sys-1',
                serviceId: 'svc-1',
                templateId: 'tpl-1',
                instanceSizeId: 'size-1',
                namespace: 'team-prod',
                reason: 'Need a VM',
                batchCount: 2,
                requestMode: 'full',
            })
        );
        expect(
            window.localStorage.getItem(buildVMRequestDraftStorageKey('u-alice'))
        ).toContain('"requestMode":"full"');
    });

    it('defaults history filter to ALL, loads active requests for in-progress, and scopes to the current user', async () => {
        const apiGet = await import('@/lib/api/client');

        const { result } = renderHook(() => useMyRequestsController({ t }));

        expect(result.current.historyStatus).toBe('ALL');
        expect(useApiGetMock).toHaveBeenCalled();

        const requestFetcher = useApiGetMock.mock.calls[0]?.[1] as (() => Promise<unknown>) | undefined;
        expect(requestFetcher).toBeTypeOf('function');

        await requestFetcher?.();

        expect(apiGet.api.GET).toHaveBeenCalledWith('/tickets', {
            params: {
                query: expect.objectContaining({
                    mine: true,
                    status_group: 'ACTIVE',
                }),
            },
        });
    });

});
