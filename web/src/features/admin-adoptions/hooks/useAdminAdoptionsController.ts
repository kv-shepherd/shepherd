'use client';

import { App } from 'antd';
import type { TFunction } from 'i18next';
import { useState } from 'react';

import { useApiGet, useApiMutation } from '@/hooks/useApiQuery';
import { api } from '@/lib/api/client';
import { translateApiError } from '@/lib/api/errorMessage';

import {
    PENDING_ADOPTION_PAGE_SIZE,
    PENDING_ADOPTION_RESOURCE_TYPE,
    type PendingAdoption,
    type PendingAdoptionAdoptResponse,
    type PendingAdoptionList,
    type PendingAdoptionStatus,
} from '../types';

type PendingAdoptionDecisionAction = 'adopt' | 'reject';

interface PendingAdoptionDecision {
    action: PendingAdoptionDecisionAction;
    record: PendingAdoption;
    reason: string;
}

interface PendingAdoptionDecisionRequest {
    id: string;
    reason?: string;
}

interface UseAdminAdoptionsControllerArgs {
    t: TFunction;
}

const decisionBody = (reason: string | undefined) => ({
    ...(reason ? { reason } : {}),
});

export function useAdminAdoptionsController({
    t,
}: UseAdminAdoptionsControllerArgs) {
    const { message: messageApi } = App.useApp();
    const [page, setPage] = useState(1);
    const [search, setSearch] = useState('');
    const [statusFilter, setStatusFilter] = useState<PendingAdoptionStatus | ''>('PENDING');
    const [clusterFilter, setClusterFilter] = useState('');
    const [namespaceFilter, setNamespaceFilter] = useState('');
    const [decision, setDecision] = useState<PendingAdoptionDecision | null>(null);

    const trimmedSearch = search.trim();
    const trimmedCluster = clusterFilter.trim();
    const trimmedNamespace = namespaceFilter.trim();

    const adoptionListQuery = useApiGet<PendingAdoptionList>(
        [
            'admin-pending-adoptions',
            page,
            statusFilter,
            trimmedSearch,
            trimmedCluster,
            trimmedNamespace,
        ],
        () => api.GET('/admin/pending-adoptions', {
            params: {
                query: {
                    page,
                    per_page: PENDING_ADOPTION_PAGE_SIZE,
                    resource_type: PENDING_ADOPTION_RESOURCE_TYPE,
                    ...(statusFilter ? { status: statusFilter } : {}),
                    ...(trimmedSearch ? { search: trimmedSearch } : {}),
                    ...(trimmedCluster ? { cluster_id: trimmedCluster } : {}),
                    ...(trimmedNamespace ? { namespace: trimmedNamespace } : {}),
                },
            },
        }),
    );

    const adoptMutation = useApiMutation<
        PendingAdoptionDecisionRequest,
        PendingAdoptionAdoptResponse
    >(
        ({ id, reason }) => api.POST('/admin/pending-adoptions/{pending_adoption_id}/adopt', {
            params: { path: { pending_adoption_id: id } },
            body: decisionBody(reason),
        }),
        {
            invalidateKeys: [['admin-pending-adoptions']],
            onSuccess: (result) => {
                messageApi.success(
                    t('adoptions.message.adopted', {
                        defaultValue: 'Adopted {{name}} into VM inventory.',
                        name: result.vm_name,
                    }),
                );
                setDecision(null);
            },
            onError: (err) => messageApi.error(translateApiError(t, err)),
        },
    );

    const rejectMutation = useApiMutation<PendingAdoptionDecisionRequest, PendingAdoption>(
        ({ id, reason }) => api.POST('/admin/pending-adoptions/{pending_adoption_id}/reject', {
            params: { path: { pending_adoption_id: id } },
            body: decisionBody(reason),
        }),
        {
            invalidateKeys: [['admin-pending-adoptions']],
            onSuccess: (record) => {
                messageApi.success(
                    t('adoptions.message.rejected', {
                        defaultValue: 'Rejected adoption candidate {{name}}.',
                        name: record.resource_name,
                    }),
                );
                setDecision(null);
            },
            onError: (err) => messageApi.error(translateApiError(t, err)),
        },
    );

    const changeSearch = (value: string) => {
        setSearch(value);
        setPage(1);
    };

    const changeStatusFilter = (value: PendingAdoptionStatus | '' | undefined) => {
        setStatusFilter(value ?? '');
        setPage(1);
    };

    const changeClusterFilter = (value: string) => {
        setClusterFilter(value);
        setPage(1);
    };

    const changeNamespaceFilter = (value: string) => {
        setNamespaceFilter(value);
        setPage(1);
    };

    const clearFilters = () => {
        setSearch('');
        setStatusFilter('PENDING');
        setClusterFilter('');
        setNamespaceFilter('');
        setPage(1);
    };

    const openDecision = (
        action: PendingAdoptionDecisionAction,
        record: PendingAdoption,
    ) => {
        setDecision({ action, record, reason: '' });
    };

    const closeDecision = () => {
        setDecision(null);
    };

    const setDecisionReason = (reason: string) => {
        setDecision((current) => current ? { ...current, reason } : current);
    };

    const submitDecision = () => {
        if (!decision) {
            return;
        }
        const request = {
            id: decision.record.id,
            reason: decision.reason.trim() || undefined,
        };
        if (decision.action === 'adopt') {
            adoptMutation.mutate(request);
            return;
        }
        rejectMutation.mutate(request);
    };

    return {
        data: adoptionListQuery.data,
        isLoading: adoptionListQuery.isLoading,
        refetch: adoptionListQuery.refetch,
        page,
        setPage,
        search,
        statusFilter,
        clusterFilter,
        namespaceFilter,
        changeSearch,
        changeStatusFilter,
        changeClusterFilter,
        changeNamespaceFilter,
        clearFilters,
        decision,
        openDecision,
        closeDecision,
        setDecisionReason,
        submitDecision,
        decisionPending: adoptMutation.isPending || rejectMutation.isPending,
    };
}
