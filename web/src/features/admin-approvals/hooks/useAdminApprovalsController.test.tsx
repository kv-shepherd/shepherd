import { act, renderHook } from '@testing-library/react';
import type { TFunction } from 'i18next';
import { beforeEach, describe, expect, it, vi } from 'vitest';

const {
  useApiGetMock,
  useApiMutationMock,
  useApiActionMock,
  useFormMock,
  useWatchMock,
  apiGetMock,
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
  useWatchMock: vi.fn(),
  apiGetMock: vi.fn(),
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
    useWatch: (...args: unknown[]) => useWatchMock(...args),
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

vi.mock('@/lib/api/client', () => ({
  api: {
    GET: (...args: unknown[]) => apiGetMock(...args),
  },
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
    useWatchMock.mockImplementation((name: string) => {
      switch (name) {
        case 'selected_storage_class':
          return 'rook-ceph';
        case 'enable_override':
          return true;
        case 'cpu_request':
          return 1;
        case 'cpu_limit':
          return 2;
        case 'memory_request_gi':
          return 2;
        case 'memory_limit_gi':
          return 4;
        default:
          return undefined;
      }
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
    apiGetMock.mockResolvedValue({});
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

  it('requests approvals using operation, cluster, and placement filters', async () => {
    const { result } = renderHook(() => useAdminApprovalsController({ t }));

    act(() => {
      result.current.changeOperationFilter('POWER');
      result.current.changeSelectedClusterFilter('cluster-a');
      result.current.changePlacementAdvisoryFilter('PVC_CLONE_HOST_ASSISTED_FALLBACK_LIKELY');
      result.current.changePlacementSnapshotFilter('present');
    });

    const approvalsCall = [...useApiGetMock.mock.calls].reverse().find(
      (call) => Array.isArray(call[0]) && call[0][0] === 'approvals'
    );
    const approvalsQueryFn = approvalsCall?.[1];
    expect(approvalsQueryFn).toBeTypeOf('function');

    await act(async () => {
      await approvalsQueryFn();
    });

    expect(apiGetMock).toHaveBeenCalledWith('/approvals', {
      params: {
        query: {
          status: 'PENDING',
          operation_type: 'POWER',
          selected_cluster_id: 'cluster-a',
          placement_advisory_code: 'PVC_CLONE_HOST_ASSISTED_FALLBACK_LIKELY',
          placement_snapshot: 'present',
          page: 1,
          per_page: 20,
        },
      },
    });
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

  it('requests compatible clusters using ticket payload and form overrides', async () => {
    const { result } = renderHook(() => useAdminApprovalsController({ t }));

    act(() => {
      result.current.openApproveModal({
        id: 'ticket-1',
        operation_type: 'CREATE',
        status: 'PENDING',
        requester: 'alice',
        ticket_payload: {
          namespace: 'prod-a',
          template_id: 'tpl-1',
          instance_size_id: 'sz-1',
        },
      } as never);
    });

    const clusterCall = [...useApiGetMock.mock.calls].reverse().find(
      (call) => Array.isArray(call[0]) && call[0][0] === 'admin-clusters'
    );
    const clusterQueryFn = clusterCall?.[1];
    expect(clusterQueryFn).toBeTypeOf('function');

    await act(async () => {
      await clusterQueryFn();
    });

    expect(apiGetMock).toHaveBeenCalledWith('/admin/clusters', {
      params: {
        query: {
          include_incompatible: true,
          namespace: 'prod-a',
          template_id: 'tpl-1',
          instance_size_id: 'sz-1',
          selected_storage_class: 'rook-ceph',
          cpu_request: 1,
          cpu_limit: 2,
          memory_request_gi: 2,
          memory_limit_gi: 4,
        },
      },
    });
  });
});
