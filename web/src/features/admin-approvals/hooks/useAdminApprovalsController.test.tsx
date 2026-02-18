import { act, renderHook } from '@testing-library/react';
import type { TFunction } from 'i18next';
import { beforeEach, describe, expect, it, vi } from 'vitest';

const {
  useApiGetMock,
  useApiMutationMock,
  useApiActionMock,
  useFormMock,
  approveFormState,
  rejectFormState,
  approveMutate,
  rejectMutate,
  cancelMutate,
  messageSuccessMock,
  messageErrorMock,
} = vi.hoisted(() => ({
  useApiGetMock: vi.fn(),
  useApiMutationMock: vi.fn(),
  useApiActionMock: vi.fn(),
  useFormMock: vi.fn(),
  approveFormState: {
    validateFields: vi.fn(),
    resetFields: vi.fn(),
  },
  rejectFormState: {
    validateFields: vi.fn(),
    resetFields: vi.fn(),
  },
  approveMutate: vi.fn(),
  rejectMutate: vi.fn(),
  cancelMutate: vi.fn(),
  messageSuccessMock: vi.fn(),
  messageErrorMock: vi.fn(),
}));

vi.mock('antd', () => ({
  Form: {
    useForm: (...args: unknown[]) => useFormMock(...args),
  },
  message: {
    useMessage: () => [
      {
        success: messageSuccessMock,
        error: messageErrorMock,
      },
      null,
    ],
  },
}));

vi.mock('@/hooks/useApiQuery', () => ({
  useApiGet: (...args: unknown[]) => useApiGetMock(...args),
  useApiMutation: (...args: unknown[]) => useApiMutationMock(...args),
  useApiAction: (...args: unknown[]) => useApiActionMock(...args),
}));

import { useAdminApprovalsController } from './useAdminApprovalsController';

describe('useAdminApprovalsController', () => {
  const t = ((key: string) => key) as unknown as TFunction;

  beforeEach(() => {
    vi.clearAllMocks();
    let formCall = 0;
    useFormMock.mockImplementation(() => {
      formCall += 1;
      return formCall % 2 === 1 ? [approveFormState] : [rejectFormState];
    });
    approveFormState.validateFields.mockResolvedValue({
      selected_cluster_id: 'cluster-a',
      selected_storage_class: 'rook-ceph',
      comment: 'approved',
    });
    rejectFormState.validateFields.mockResolvedValue({
      reason: 'policy violation',
    });
    let getCall = 0;
    useApiGetMock.mockImplementation(() => {
      getCall += 1;
      if (getCall % 2 === 1) {
        return {
          data: { items: [{ id: 'ticket-1', status: 'PENDING', operation_type: 'CREATE', requester: 'alice' }] },
          isLoading: false,
          refetch: vi.fn(),
        };
      }
      return {
        data: { items: [{ id: 'cluster-a', name: 'Cluster A', status: 'HEALTHY' }] },
        isLoading: false,
      };
    });
    let mutationCall = 0;
    useApiMutationMock.mockImplementation(() => {
      mutationCall += 1;
      return mutationCall % 2 === 1
        ? { mutate: approveMutate, isPending: false }
        : { mutate: rejectMutate, isPending: false };
    });
    useApiActionMock.mockReturnValue({ mutate: cancelMutate, isPending: false });
  });

  it('resets paging when switching status filter', () => {
    const { result } = renderHook(() => useAdminApprovalsController({ t }));

    act(() => {
      result.current.setPage(3);
      result.current.changeStatusFilter('ALL');
    });

    expect(result.current.statusFilter).toBe('ALL');
    expect(result.current.page).toBe(1);
  });

  it('submits approve/reject decisions with selected ticket ids', async () => {
    const { result } = renderHook(() => useAdminApprovalsController({ t }));
    const pendingTicket = {
      id: 'ticket-1',
      operation_type: 'CREATE',
      status: 'PENDING',
      requester: 'alice',
    };

    act(() => {
      result.current.openApproveModal(pendingTicket as never);
      result.current.openRejectModal(pendingTicket as never);
    });

    await act(async () => {
      await result.current.submitApprove();
    });
    expect(approveMutate).toHaveBeenCalledWith({
      ticketId: 'ticket-1',
      body: {
        selected_cluster_id: 'cluster-a',
        selected_storage_class: 'rook-ceph',
        comment: 'approved',
      },
    });

    await act(async () => {
      await result.current.submitReject();
    });
    expect(rejectMutate).toHaveBeenCalledWith({
      ticketId: 'ticket-1',
      body: { reason: 'policy violation' },
    });
  });
});
