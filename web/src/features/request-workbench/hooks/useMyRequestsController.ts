'use client';

import { useEffect, useState } from 'react';
import type { TFunction } from 'i18next';

import { useApiGet } from '@/lib/api/useApiGet';
import { useApiMutation } from '@/lib/api/useApiMutation';
import { api } from '@/lib/api/client';
import { translateApiError } from '@/lib/api/errorMessage';
import { useMessage } from '@/lib/hooks/useMessage';
import { useApiAction } from '@/hooks/useApiQuery';
import {
    ACTIVE_BATCH_CHANGED_EVENT,
    clearStoredActiveBatchState,
    readStoredActiveBatchState,
} from '@/lib/storage/activeBatchTracking';
import { useAuthStore } from '@/stores/auth';
import {
    clearVMRequestDraft,
    loadVMRequestDraft,
    resolveVMRequestDraftOwner,
    saveVMRequestDraft,
    VM_REQUEST_DRAFT_CHANGED_EVENT,
} from '@/features/vm-management/draftStorage';
import type { VMRequestDraft } from '@/features/vm-management/types';
import type {
    Ticket,
    TicketList,
    BatchActionResponse,
    BatchStatusResponse,
    HistoryStatusFilter,
    RequestWorkbenchView,
} from '../types';

interface UseMyRequestsControllerArgs {
    t: TFunction;
}

const ACTIVE_BATCH_POLL_INTERVAL_MS = 5000;
const RETRYABLE_BATCH_STATUSES = new Set(['FAILED', 'PARTIAL_SUCCESS']);
const CANCELLABLE_BATCH_STATUSES = new Set(['PENDING_APPROVAL', 'IN_PROGRESS']);

export function useMyRequestsController({ t }: UseMyRequestsControllerArgs) {
    const user = useAuthStore((state) => state.user);
    const [view, setView] = useState<RequestWorkbenchView>('in_progress');
    const [historyStatus, setHistoryStatus] = useState<HistoryStatusFilter>('SUCCESS');
    const [page, setPage] = useState(1);
    const [pageSize, setPageSize] = useState(20);
    const [savedVmDraft, setSavedVmDraft] = useState<VMRequestDraft | null>(null);
    const [activeBatchID, setActiveBatchID] = useState(
        () => readStoredActiveBatchState().batch_id
    );
    const [activeBatchStatusURL, setActiveBatchStatusURL] = useState(
        () => readStoredActiveBatchState().status_url
    );
    const { messageApi, messageContextHolder } = useMessage();
    const draftOwner = resolveVMRequestDraftOwner(user);

    const requestQueryEnabled = view === 'in_progress' || view === 'history';
    const requestStatus = view === 'in_progress' ? 'PENDING' : historyStatus;
    const batchQueryEnabled = view === 'batch_jobs' && activeBatchID !== '';

    const { data, isLoading, refetch } = useApiGet<TicketList>(
        ['my-tickets', view, requestStatus, page, pageSize],
        () =>
            api.GET('/tickets', {
                params: {
                    query: {
                        page,
                        per_page: pageSize,
                        mine: true,
                        status: requestStatus as never,
                    },
                },
            }) as Promise<{ data?: TicketList; error?: unknown; response?: Response }>,
        { enabled: requestQueryEnabled }
    );

    const {
        data: batchStatus,
        isLoading: batchLoading,
        refetch: refetchBatch,
    } = useApiGet<BatchStatusResponse>(
        ['my-requests', 'batch', activeBatchID, activeBatchStatusURL],
        () =>
            api.GET('/vms/batch/{batch_id}', {
                params: {
                    path: { batch_id: activeBatchID },
                },
            }) as Promise<{ data?: BatchStatusResponse; error?: unknown; response?: Response }>,
        {
            enabled: batchQueryEnabled,
            refetchInterval: batchQueryEnabled ? ACTIVE_BATCH_POLL_INTERVAL_MS : undefined,
        }
    );

    const cancelMutation = useApiAction<string>(
        (id: string) =>
            api.POST('/tickets/{ticket_id}/cancel', { params: { path: { ticket_id: id } } }),
        {
            invalidateKeys: [['my-tickets'], ['tickets']],
            onSuccess: () => {
                void messageApi.success(t('cancel_success'));
                void refetch();
            },
            onError: (error) => {
                void messageApi.error(translateApiError(t, error));
            },
        }
    );

    const retryBatchMutation = useApiMutation<BatchActionResponse, string>(
        (batchID: string) =>
            api.POST('/vms/batch/{batch_id}/retry', {
                params: { path: { batch_id: batchID } },
            }),
        {
            onSuccess: () => {
                void messageApi.success(t('workbench.batch_jobs.retry_submitted'));
                void refetchBatch();
            },
            onError: (error) => {
                void messageApi.error(translateApiError(t, error));
            },
        }
    );

    const cancelBatchMutation = useApiMutation<BatchActionResponse, string>(
        (batchID: string) =>
            api.POST('/vms/batch/{batch_id}/cancel', {
                params: { path: { batch_id: batchID } },
            }),
        {
            onSuccess: () => {
                void messageApi.success(t('workbench.batch_jobs.cancel_submitted'));
                void refetchBatch();
            },
            onError: (error) => {
                void messageApi.error(translateApiError(t, error));
            },
        }
    );

    const changeView = (nextView: RequestWorkbenchView) => {
        setView(nextView);
        setPage(1);
    };

    const changeHistoryStatus = (nextStatus: HistoryStatusFilter) => {
        setHistoryStatus(nextStatus);
        setPage(1);
    };

    const discardSavedVmDraft = () => {
        if (draftOwner === '') {
            return;
        }
        clearVMRequestDraft(draftOwner);
        setSavedVmDraft(null);
        void messageApi.success(t('workbench.drafts.discarded'));
    };

    const prepareHistoryReuse = (ticket: Ticket): boolean => {
        if (draftOwner === '' || !ticket.request_prefill) {
            return false;
        }

        const draft: VMRequestDraft = {
            version: 1,
            systemId: ticket.request_prefill.system_id,
            serviceId: ticket.request_prefill.service_id,
            templateId: ticket.request_prefill.template_id,
            instanceSizeId: ticket.request_prefill.instance_size_id,
            namespace: ticket.request_prefill.namespace,
            reason: ticket.request_prefill.reason,
            batchCount: Math.max(1, ticket.request_prefill.batch_count),
            wizardStep: 0,
            requestMode: 'full',
            updatedAt: new Date().toISOString(),
        };

        saveVMRequestDraft(draftOwner, draft);
        setSavedVmDraft(loadVMRequestDraft(draftOwner));
        return true;
    };

    useEffect(() => {
        setSavedVmDraft(loadVMRequestDraft(draftOwner));
    }, [draftOwner]);

    useEffect(() => {
        if (typeof window === 'undefined' || draftOwner === '') {
            return;
        }

        const refreshDraft = () => {
            setSavedVmDraft(loadVMRequestDraft(draftOwner));
        };

        const onStorage = (event: StorageEvent) => {
            if (event.storageArea !== window.localStorage) {
                return;
            }
            refreshDraft();
        };

        window.addEventListener(VM_REQUEST_DRAFT_CHANGED_EVENT, refreshDraft);
        window.addEventListener('storage', onStorage);
        return () => {
            window.removeEventListener(VM_REQUEST_DRAFT_CHANGED_EVENT, refreshDraft);
            window.removeEventListener('storage', onStorage);
        };
    }, [draftOwner]);

    useEffect(() => {
        if (typeof window === 'undefined') {
            return;
        }

        const refreshActiveBatch = () => {
            const stored = readStoredActiveBatchState();
            setActiveBatchID(stored.batch_id);
            setActiveBatchStatusURL(stored.status_url);
        };

        const onStorage = (event: StorageEvent) => {
            if (event.storageArea !== window.sessionStorage) {
                return;
            }
            refreshActiveBatch();
        };

        window.addEventListener(ACTIVE_BATCH_CHANGED_EVENT, refreshActiveBatch);
        window.addEventListener('storage', onStorage);
        return () => {
            window.removeEventListener(ACTIVE_BATCH_CHANGED_EVENT, refreshActiveBatch);
            window.removeEventListener('storage', onStorage);
        };
    }, []);

    const clearActiveBatchTracking = () => {
        clearStoredActiveBatchState();
        setActiveBatchID('');
        setActiveBatchStatusURL('');
    };

    return {
        data,
        isLoading,
        refetch,
        view,
        page,
        pageSize,
        historyStatus,
        savedVmDraft,
        cancelMutation,
        activeBatchID,
        batchStatus,
        batchLoading,
        batchCanRetry: RETRYABLE_BATCH_STATUSES.has(batchStatus?.status ?? ''),
        batchCanCancel: CANCELLABLE_BATCH_STATUSES.has(batchStatus?.status ?? ''),
        batchActionPending:
            retryBatchMutation.isPending || cancelBatchMutation.isPending,
        messageContextHolder,
        setPage,
        setPageSize,
        changeView,
        changeHistoryStatus,
        discardSavedVmDraft,
        prepareHistoryReuse,
        refreshBatch: () => {
            void refetchBatch();
        },
        clearBatchTracking: clearActiveBatchTracking,
        retryBatch: () => {
            if (activeBatchID !== '') {
                retryBatchMutation.mutate(activeBatchID);
            }
        },
        cancelBatch: () => {
            if (activeBatchID !== '') {
                cancelBatchMutation.mutate(activeBatchID);
            }
        },
    };
}
