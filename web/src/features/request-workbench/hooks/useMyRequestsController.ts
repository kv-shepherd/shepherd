'use client';

import { useEffect, useState } from 'react';
import type { TFunction } from 'i18next';

import { useApiGet } from '@/lib/api/useApiGet';
import { api } from '@/lib/api/client';
import { translateApiError } from '@/lib/api/errorMessage';
import { useMessage } from '@/lib/hooks/useMessage';
import { useApiAction } from '@/hooks/useApiQuery';
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
    HistoryStatusFilter,
    RequestTicketOperationType,
    RequestWorkbenchView,
} from '../types';

interface UseMyRequestsControllerArgs {
    t: TFunction;
}

export function useMyRequestsController({ t }: UseMyRequestsControllerArgs) {
    const user = useAuthStore((state) => state.user);
    const [view, setView] = useState<RequestWorkbenchView>('in_progress');
    const [historyStatus, setHistoryStatus] = useState<HistoryStatusFilter>('ALL');
    const [page, setPage] = useState(1);
    const [pageSize, setPageSize] = useState(20);
    const [search, setSearch] = useState('');
    const [operationType, setOperationType] = useState<RequestTicketOperationType | ''>('');
    const [savedVmDraft, setSavedVmDraft] = useState<VMRequestDraft | null>(null);
    const { messageApi, messageContextHolder } = useMessage();
    const draftOwner = resolveVMRequestDraftOwner(user);

    const requestQueryEnabled = view === 'in_progress' || view === 'history';
    const requestStatus =
        view === 'history' && historyStatus !== 'ALL' ? historyStatus : '';
    const requestStatusGroup = view === 'in_progress' ? 'ACTIVE' : 'TERMINAL';

    const { data, isLoading, refetch } = useApiGet<TicketList>(
        ['my-tickets', view, requestStatusGroup, requestStatus, page, pageSize, search, operationType],
        () =>
            api.GET('/tickets', {
                params: {
                    query: {
                        page,
                        per_page: pageSize,
                        mine: true,
                        ...(requestStatus ? { status: requestStatus as never } : {}),
                        status_group: requestStatusGroup as never,
                        search: search || undefined,
                        operation_type: operationType || undefined,
                    },
                },
            }) as Promise<{ data?: TicketList; error?: unknown; response?: Response }>,
        { enabled: requestQueryEnabled }
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

    const changeView = (nextView: RequestWorkbenchView) => {
        setView(nextView);
        setPage(1);
    };

    const changeHistoryStatus = (nextStatus: HistoryStatusFilter) => {
        setHistoryStatus(nextStatus);
        setPage(1);
    };

    const applySearch = (nextSearch: string) => {
        setSearch(nextSearch.trim());
        setPage(1);
    };

    const applyOperationType = (nextOperationType: RequestTicketOperationType | '') => {
        setOperationType(nextOperationType);
        setPage(1);
    };

    const clearListFilters = () => {
        setSearch('');
        setOperationType('');
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

    return {
        data,
        isLoading,
        refetch,
        view,
        page,
        pageSize,
        historyStatus,
        search,
        operationType,
        savedVmDraft,
        cancelMutation,
        messageContextHolder,
        setPage,
        setPageSize,
        changeView,
        changeHistoryStatus,
        applySearch,
        applyOperationType,
        clearListFilters,
        discardSavedVmDraft,
        prepareHistoryReuse,
    };
}
