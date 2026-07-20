import { act, renderHook } from '@testing-library/react';
import type { TFunction } from 'i18next';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import type { ApiErrorResponse } from '@/hooks/useApiQuery';
import type {
    PendingAdoption,
    PendingAdoptionAdoptResponse,
    PendingAdoptionList,
} from '../types';

const {
    useApiGetMock,
    useApiMutationMock,
    messageSuccessMock,
    messageErrorMock,
    apiGetMock,
    apiPostMock,
    adoptMutateMock,
    rejectMutateMock,
    refetchMock,
    pendingState,
} = vi.hoisted(() => ({
    useApiGetMock: vi.fn(),
    useApiMutationMock: vi.fn(),
    messageSuccessMock: vi.fn(),
    messageErrorMock: vi.fn(),
    apiGetMock: vi.fn(),
    apiPostMock: vi.fn(),
    adoptMutateMock: vi.fn(),
    rejectMutateMock: vi.fn(),
    refetchMock: vi.fn(),
    pendingState: {
        adopt: false,
        reject: false,
    },
}));

vi.mock('antd', () => ({
    App: {
        useApp: () => ({
            message: {
                success: messageSuccessMock,
                error: messageErrorMock,
            },
        }),
    },
}));

vi.mock('@/hooks/useApiQuery', () => ({
    useApiGet: (...args: unknown[]) => useApiGetMock(...args),
    useApiMutation: (...args: unknown[]) => useApiMutationMock(...args),
}));

vi.mock('@/lib/api/client', () => ({
    api: {
        GET: (...args: unknown[]) => apiGetMock(...args),
        POST: (...args: unknown[]) => apiPostMock(...args),
    },
}));

import { useAdminAdoptionsController } from './useAdminAdoptionsController';

interface DecisionRequest {
    id: string;
    reason?: string;
}

interface MutationOptions {
    invalidateKeys?: readonly (readonly unknown[])[];
    onSuccess?: (data: unknown) => void;
    onError?: (error: ApiErrorResponse) => void;
}

interface MutationBinding {
    mutationFn: (request: DecisionRequest) => unknown;
    options: MutationOptions;
}

type TranslationOptions = string | {
    defaultValue?: string;
    [key: string]: unknown;
};

const pendingAdoption: PendingAdoption = {
    id: 'pending-1',
    cluster_id: 'cluster-a',
    namespace: 'team-a',
    resource_name: 'vm-live-a',
    resource_type: 'VirtualMachine',
    status: 'PENDING',
    discovered_by: 'system:vm-adoption-discovery',
    labels: { 'shepherd.io/service-id': 'service-a' },
    created_at: '2026-03-17T00:00:00Z',
    updated_at: '2026-03-17T00:00:00Z',
};

const listData: PendingAdoptionList = {
    items: [pendingAdoption],
    pagination: { page: 1, per_page: 20, total: 1, total_pages: 1 },
};

const translate = (key: string, options?: TranslationOptions) => {
    if (key === 'errors:ADOPTION_CONFLICT') {
        return 'Localized adoption conflict';
    }

    const fallback = typeof options === 'string' ? options : options?.defaultValue;
    let message = fallback ?? key;
    if (options && typeof options === 'object') {
        for (const [name, value] of Object.entries(options)) {
            if (name !== 'defaultValue') {
                message = message.replaceAll(`{{${name}}}`, String(value));
            }
        }
    }
    return message;
};

describe('useAdminAdoptionsController', () => {
    const tMock = vi.fn(translate);
    const t = tMock as unknown as TFunction;

    let mutationCallIndex = 0;
    let adoptBinding: MutationBinding | undefined;
    let rejectBinding: MutationBinding | undefined;
    let latestQueryKey: readonly unknown[] | undefined;
    let latestListFetcher: (() => unknown) | undefined;

    const resetCapturedState = () => {
        mutationCallIndex = 0;
        adoptBinding = undefined;
        rejectBinding = undefined;
        latestQueryKey = undefined;
        latestListFetcher = undefined;
        pendingState.adopt = false;
        pendingState.reject = false;
    };

    beforeEach(() => {
        vi.clearAllMocks();
        resetCapturedState();
        tMock.mockImplementation(translate);

        apiGetMock.mockResolvedValue({
            data: listData,
            response: new Response(null, { status: 200 }),
        });
        apiPostMock.mockResolvedValue({
            data: {},
            response: new Response(null, { status: 200 }),
        });

        useApiGetMock.mockImplementation((...args: unknown[]) => {
            latestQueryKey = args[0] as readonly unknown[];
            latestListFetcher = args[1] as () => unknown;
            return {
                data: listData,
                isLoading: false,
                refetch: refetchMock,
            };
        });

        useApiMutationMock.mockImplementation((...args: unknown[]) => {
            const binding: MutationBinding = {
                mutationFn: args[0] as MutationBinding['mutationFn'],
                options: args[1] as MutationOptions,
            };
            const isAdoptMutation = mutationCallIndex % 2 === 0;
            mutationCallIndex += 1;

            if (isAdoptMutation) {
                adoptBinding = binding;
                return {
                    mutate: adoptMutateMock,
                    isPending: pendingState.adopt,
                };
            }

            rejectBinding = binding;
            return {
                mutate: rejectMutateMock,
                isPending: pendingState.reject,
            };
        });
    });

    afterEach(() => {
        vi.clearAllMocks();
        resetCapturedState();
    });

    it('uses trimmed filters in both the query key and API request while resetting page', async () => {
        const { result } = renderHook(() => useAdminAdoptionsController({ t }));

        expect(result.current.data).toBe(listData);
        expect(result.current.isLoading).toBe(false);
        expect(result.current.refetch).toBe(refetchMock);
        expect(latestQueryKey).toEqual([
            'admin-pending-adoptions',
            1,
            'PENDING',
            '',
            '',
            '',
        ]);

        const changeFilterFromLaterPage = (changeFilter: () => void) => {
            act(() => result.current.setPage(7));
            expect(result.current.page).toBe(7);
            act(changeFilter);
            expect(result.current.page).toBe(1);
        };

        changeFilterFromLaterPage(() => result.current.changeSearch('  database vm  '));
        changeFilterFromLaterPage(() => result.current.changeStatusFilter('ADOPTED'));
        changeFilterFromLaterPage(() => result.current.changeClusterFilter('  cluster-a  '));
        changeFilterFromLaterPage(() => result.current.changeNamespaceFilter('  team-a  '));

        expect(latestQueryKey).toEqual([
            'admin-pending-adoptions',
            1,
            'ADOPTED',
            'database vm',
            'cluster-a',
            'team-a',
        ]);
        expect(latestListFetcher).toBeTypeOf('function');

        await latestListFetcher?.();

        expect(apiGetMock).toHaveBeenCalledWith('/admin/pending-adoptions', {
            params: {
                query: {
                    page: 1,
                    per_page: 20,
                    resource_type: 'VirtualMachine',
                    status: 'ADOPTED',
                    search: 'database vm',
                    cluster_id: 'cluster-a',
                    namespace: 'team-a',
                },
            },
        });
    });

    it('manages adopt and reject decisions and trims optional reasons at submission', () => {
        const { result } = renderHook(() => useAdminAdoptionsController({ t }));

        act(() => result.current.submitDecision());
        expect(adoptMutateMock).not.toHaveBeenCalled();
        expect(rejectMutateMock).not.toHaveBeenCalled();

        act(() => result.current.openDecision('adopt', pendingAdoption));
        expect(result.current.decision).toEqual({
            action: 'adopt',
            record: pendingAdoption,
            reason: '',
        });

        act(() => result.current.setDecisionReason('  recover inventory  '));
        expect(result.current.decision?.reason).toBe('  recover inventory  ');
        act(() => result.current.submitDecision());
        expect(adoptMutateMock).toHaveBeenCalledWith({
            id: 'pending-1',
            reason: 'recover inventory',
        });

        act(() => result.current.closeDecision());
        expect(result.current.decision).toBeNull();

        act(() => result.current.openDecision('reject', pendingAdoption));
        act(() => result.current.setDecisionReason('   '));
        act(() => result.current.submitDecision());
        expect(rejectMutateMock).toHaveBeenCalledWith({
            id: 'pending-1',
            reason: undefined,
        });
    });

    it('binds adopt and reject mutations to their exact endpoint, path, body, and invalidation key', async () => {
        renderHook(() => useAdminAdoptionsController({ t }));

        expect(adoptBinding).toBeDefined();
        expect(rejectBinding).toBeDefined();
        expect(adoptBinding?.options.invalidateKeys).toEqual([['admin-pending-adoptions']]);
        expect(rejectBinding?.options.invalidateKeys).toEqual([['admin-pending-adoptions']]);

        await adoptBinding?.mutationFn({ id: 'pending-1', reason: 'recovered' });
        expect(apiPostMock).toHaveBeenLastCalledWith(
            '/admin/pending-adoptions/{pending_adoption_id}/adopt',
            {
                params: { path: { pending_adoption_id: 'pending-1' } },
                body: { reason: 'recovered' },
            },
        );

        await rejectBinding?.mutationFn({ id: 'pending-2', reason: undefined });
        expect(apiPostMock).toHaveBeenLastCalledWith(
            '/admin/pending-adoptions/{pending_adoption_id}/reject',
            {
                params: { path: { pending_adoption_id: 'pending-2' } },
                body: {},
            },
        );
    });

    it('closes each decision and emits the resource-specific success message', () => {
        const { result } = renderHook(() => useAdminAdoptionsController({ t }));

        act(() => result.current.openDecision('adopt', pendingAdoption));
        const adoptResponse: PendingAdoptionAdoptResponse = {
            pending_adoption: { ...pendingAdoption, status: 'ADOPTED' },
            vm_id: 'vm-1',
            vm_name: 'vm-live-a',
        };
        act(() => adoptBinding?.options.onSuccess?.(adoptResponse));

        expect(result.current.decision).toBeNull();
        expect(messageSuccessMock).toHaveBeenLastCalledWith(
            'Adopted vm-live-a into VM inventory.',
        );

        act(() => result.current.openDecision('reject', pendingAdoption));
        act(() => rejectBinding?.options.onSuccess?.({
            ...pendingAdoption,
            resource_name: 'vm-rejected-a',
            status: 'REJECTED',
        }));

        expect(result.current.decision).toBeNull();
        expect(messageSuccessMock).toHaveBeenLastCalledWith(
            'Rejected adoption candidate vm-rejected-a.',
        );
        expect(messageSuccessMock).toHaveBeenCalledTimes(2);
    });

    it('translates mutation errors before displaying them', () => {
        renderHook(() => useAdminAdoptionsController({ t }));
        const error: ApiErrorResponse = {
            code: 'ADOPTION_CONFLICT',
            message: 'raw backend message',
        };

        act(() => {
            adoptBinding?.options.onError?.(error);
            rejectBinding?.options.onError?.(error);
        });

        expect(messageErrorMock).toHaveBeenNthCalledWith(1, 'Localized adoption conflict');
        expect(messageErrorMock).toHaveBeenNthCalledWith(2, 'Localized adoption conflict');
        expect(tMock).toHaveBeenCalledWith(
            'errors:ADOPTION_CONFLICT',
            expect.objectContaining({ defaultValue: '' }),
        );
    });

    it('aggregates pending state from either decision mutation', () => {
        pendingState.adopt = true;
        const { result, rerender } = renderHook(() => useAdminAdoptionsController({ t }));
        expect(result.current.decisionPending).toBe(true);

        pendingState.adopt = false;
        pendingState.reject = true;
        rerender();
        expect(result.current.decisionPending).toBe(true);

        pendingState.reject = false;
        rerender();
        expect(result.current.decisionPending).toBe(false);
    });
});
