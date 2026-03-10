'use client';

import { Form, message } from 'antd';
import type { TFunction } from 'i18next';
import { useState } from 'react';

import { useApiAction, useApiGet, useApiMutation } from '@/hooks/useApiQuery';
import { api } from '@/lib/api/client';

import type {
    ApprovalDecisionRequest,
    ApprovalStatus,
    ApprovalTicket,
    ApprovalTicketList,
    ClusterList,
    RejectDecisionRequest,
} from '../types';

interface UseAdminApprovalsControllerArgs {
    t: TFunction;
}

export function useAdminApprovalsController({ t }: UseAdminApprovalsControllerArgs) {
    const [messageApi, messageContextHolder] = message.useMessage();
    const [statusFilter, setStatusFilter] = useState<'ALL' | ApprovalStatus>('PENDING');
    const [operationFilter, setOperationFilter] = useState<'ALL' | ApprovalTicket['operation_type']>('ALL');
    const [selectedClusterFilter, setSelectedClusterFilter] = useState('');
    const [placementAdvisoryFilter, setPlacementAdvisoryFilter] = useState('');
    const [placementSnapshotFilter, setPlacementSnapshotFilter] = useState<'ALL' | 'present' | 'missing'>('ALL');
    const [page, setPage] = useState(1);
    const [pageSize, setPageSize] = useState(20);
    const [approveModal, setApproveModal] = useState<ApprovalTicket | null>(null);
    const [rejectModal, setRejectModal] = useState<ApprovalTicket | null>(null);
    const [approveForm] = Form.useForm<ApprovalDecisionRequest>();
    const [rejectForm] = Form.useForm<RejectDecisionRequest>();
    const watchedStorageClass = Form.useWatch('selected_storage_class', approveForm);
    const watchedEnableOverride = Form.useWatch('enable_override', approveForm);
    const watchedCPURequest = Form.useWatch('cpu_request', approveForm);
    const watchedCPULimit = Form.useWatch('cpu_limit', approveForm);
    const watchedMemoryRequestGi = Form.useWatch('memory_request_gi', approveForm);
    const watchedMemoryLimitGi = Form.useWatch('memory_limit_gi', approveForm);
    const trimmedSelectedClusterFilter = selectedClusterFilter.trim();
    const trimmedPlacementAdvisoryFilter = placementAdvisoryFilter.trim();

    const approvalListQuery = useApiGet<ApprovalTicketList>(
        ['approvals', statusFilter, operationFilter, trimmedSelectedClusterFilter, trimmedPlacementAdvisoryFilter, placementSnapshotFilter, page, pageSize],
        () => api.GET('/approvals', {
            params: {
                query: {
                    ...(statusFilter !== 'ALL' ? { status: statusFilter } : {}),
                    ...(operationFilter !== 'ALL' ? { operation_type: operationFilter } : {}),
                    ...(trimmedSelectedClusterFilter ? { selected_cluster_id: trimmedSelectedClusterFilter } : {}),
                    ...(trimmedPlacementAdvisoryFilter ? { placement_advisory_code: trimmedPlacementAdvisoryFilter } : {}),
                    ...(placementSnapshotFilter !== 'ALL' ? { placement_snapshot: placementSnapshotFilter } : {}),
                    page,
                    per_page: pageSize,
                },
            },
        })
    );

    const isCreateTicket = approveModal?.operation_type === 'CREATE';
    const approvePayload = approveModal?.ticket_payload as Record<string, unknown> | undefined;
    const compatibilityQuery = isCreateTicket && approveModal ? {
        include_incompatible: true,
        namespace: readPayloadString(approvePayload, 'namespace'),
        template_id: readPayloadString(approvePayload, 'template_id'),
        instance_size_id: readPayloadString(approvePayload, 'instance_size_id'),
        ...(typeof watchedStorageClass === 'string' && watchedStorageClass.trim()
            ? { selected_storage_class: watchedStorageClass.trim() }
            : {}),
        ...(watchedEnableOverride ? {
            ...(typeof watchedCPURequest === 'number' ? { cpu_request: watchedCPURequest } : {}),
            ...(typeof watchedCPULimit === 'number' ? { cpu_limit: watchedCPULimit } : {}),
            ...(typeof watchedMemoryRequestGi === 'number' ? { memory_request_gi: watchedMemoryRequestGi } : {}),
            ...(typeof watchedMemoryLimitGi === 'number' ? { memory_limit_gi: watchedMemoryLimitGi } : {}),
        } : {}),
    } : undefined;
    const clusterListQuery = useApiGet<ClusterList>(
        ['admin-clusters', 'approval-select', compatibilityQuery],
        () => api.GET('/admin/clusters', compatibilityQuery ? { params: { query: compatibilityQuery } } : undefined),
        { enabled: Boolean(approveModal) && isCreateTicket }
    );

    const approveMutation = useApiMutation<
        { ticketId: string; body: ApprovalDecisionRequest },
        unknown
    >(
        ({ ticketId, body }) => api.POST('/approvals/{ticket_id}/approve', {
            params: { path: { ticket_id: ticketId } },
            body,
        }),
        {
            invalidateKeys: [['approvals'], ['vms']],
            onSuccess: () => {
                messageApi.success(t('common:message.success'));
                closeApproveModal();
            },
            onError: (err) => messageApi.error(err.message || t('common:message.error')),
        }
    );

    const rejectMutation = useApiMutation<
        { ticketId: string; body: RejectDecisionRequest },
        unknown
    >(
        ({ ticketId, body }) => api.POST('/approvals/{ticket_id}/reject', {
            params: { path: { ticket_id: ticketId } },
            body,
        }),
        {
            invalidateKeys: [['approvals']],
            onSuccess: () => {
                messageApi.success(t('common:message.success'));
                closeRejectModal();
            },
            onError: (err) => messageApi.error(err.message || t('common:message.error')),
        }
    );

    const cancelMutation = useApiAction<string>(
        (ticketId) => api.POST('/approvals/{ticket_id}/cancel', {
            params: { path: { ticket_id: ticketId } },
        }),
        {
            invalidateKeys: [['approvals']],
            onSuccess: () => messageApi.success(t('common:message.success')),
            onError: (err) => messageApi.error(err.message || t('common:message.error')),
        }
    );

    const changeStatusFilter = (value: 'ALL' | ApprovalStatus) => {
        setStatusFilter(value);
        setPage(1);
    };

    const changeOperationFilter = (value: 'ALL' | ApprovalTicket['operation_type']) => {
        setOperationFilter(value);
        setPage(1);
    };

    const changeSelectedClusterFilter = (value: string) => {
        setSelectedClusterFilter(value);
        setPage(1);
    };

    const changePlacementAdvisoryFilter = (value: string) => {
        setPlacementAdvisoryFilter(value);
        setPage(1);
    };

    const changePlacementSnapshotFilter = (value: 'ALL' | 'present' | 'missing') => {
        setPlacementSnapshotFilter(value);
        setPage(1);
    };

    const openApproveModal = (ticket: ApprovalTicket) => {
        setApproveModal(ticket);
    };

    const closeApproveModal = () => {
        setApproveModal(null);
        approveForm.resetFields();
    };

    const openRejectModal = (ticket: ApprovalTicket) => {
        setRejectModal(ticket);
    };

    const closeRejectModal = () => {
        setRejectModal(null);
        rejectForm.resetFields();
    };

    const submitApprove = async () => {
        if (!approveModal) {
            return;
        }
        const values = await approveForm.validateFields();
        approveMutation.mutate({ ticketId: approveModal.id, body: values });
    };

    const submitReject = async () => {
        if (!rejectModal) {
            return;
        }
        const values = await rejectForm.validateFields();
        rejectMutation.mutate({ ticketId: rejectModal.id, body: values });
    };

    const submitCancel = (ticketId: string) => {
        cancelMutation.mutate(ticketId);
    };

    return {
        messageContextHolder,
        statusFilter,
        changeStatusFilter,
        operationFilter,
        changeOperationFilter,
        selectedClusterFilter,
        changeSelectedClusterFilter,
        placementAdvisoryFilter,
        changePlacementAdvisoryFilter,
        placementSnapshotFilter,
        changePlacementSnapshotFilter,
        page,
        pageSize,
        setPage,
        setPageSize,
        data: approvalListQuery.data,
        isLoading: approvalListQuery.isLoading,
        refetch: approvalListQuery.refetch,
        approveModal,
        rejectModal,
        approveForm,
        rejectForm,
        clustersData: clusterListQuery.data,
        openApproveModal,
        closeApproveModal,
        openRejectModal,
        closeRejectModal,
        submitApprove,
        submitReject,
        submitCancel,
        approvePending: approveMutation.isPending,
        rejectPending: rejectMutation.isPending,
        cancelPending: cancelMutation.isPending,
    };
}

function readPayloadString(payload: Record<string, unknown> | undefined, key: string): string | undefined {
    if (!payload) {
        return undefined;
    }
    const value = payload[key];
    if (typeof value !== 'string') {
        return undefined;
    }
    const trimmed = value.trim();
    return trimmed || undefined;
}
