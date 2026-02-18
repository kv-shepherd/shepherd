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
    const [page, setPage] = useState(1);
    const [pageSize, setPageSize] = useState(20);
    const [approveModal, setApproveModal] = useState<ApprovalTicket | null>(null);
    const [rejectModal, setRejectModal] = useState<ApprovalTicket | null>(null);
    const [approveForm] = Form.useForm<ApprovalDecisionRequest>();
    const [rejectForm] = Form.useForm<RejectDecisionRequest>();

    const approvalListQuery = useApiGet<ApprovalTicketList>(
        ['approvals', statusFilter, page, pageSize],
        () => api.GET('/approvals', {
            params: {
                query: statusFilter === 'ALL'
                    ? { page, per_page: pageSize }
                    : { status: statusFilter, page, per_page: pageSize },
            },
        })
    );

    const isCreateTicket = approveModal?.operation_type !== 'DELETE';
    const clusterListQuery = useApiGet<ClusterList>(
        ['admin-clusters', 'approval-select'],
        () => api.GET('/admin/clusters'),
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
