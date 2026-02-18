import { act, renderHook } from '@testing-library/react';
import type { TFunction } from 'i18next';
import { beforeEach, describe, expect, it, vi } from 'vitest';

const {
  useApiGetMock,
  useApiMutationMock,
  useApiActionMock,
  postStartMutate,
  postStopMutate,
  postRestartMutate,
  requestConsoleMutate,
  createMutate,
  createBatchMutate,
  vmBatchMutate,
  vmBatchPowerMutate,
  retryBatchMutate,
  cancelBatchMutate,
  deleteMutate,
  apiGetMock,
  formState,
  watchValues,
  messageSuccessMock,
  messageErrorMock,
  messageWarningMock,
  messageInfoMock,
  batchStatusRefetchMock,
} = vi.hoisted(() => ({
  useApiGetMock: vi.fn(),
  useApiMutationMock: vi.fn(),
  useApiActionMock: vi.fn(),
  postStartMutate: vi.fn(),
  postStopMutate: vi.fn(),
  postRestartMutate: vi.fn(),
  requestConsoleMutate: vi.fn(),
  createMutate: vi.fn(),
  createBatchMutate: vi.fn(),
  vmBatchMutate: vi.fn(),
  vmBatchPowerMutate: vi.fn(),
  retryBatchMutate: vi.fn(),
  cancelBatchMutate: vi.fn(),
  deleteMutate: vi.fn(),
  apiGetMock: vi.fn(),
  formState: {
    validateFields: vi.fn(),
    resetFields: vi.fn(),
    setFieldValue: vi.fn(),
    getFieldsValue: vi.fn(),
  },
  watchValues: {
    template_id: 'tpl-1',
    instance_size_id: 'size-1',
    namespace: 'prod',
    reason: 'scale up',
    service_id: 'svc-1',
    batch_count: 1,
  } as Record<string, unknown>,
  messageSuccessMock: vi.fn(),
  messageErrorMock: vi.fn(),
  messageWarningMock: vi.fn(),
  messageInfoMock: vi.fn(),
  batchStatusRefetchMock: vi.fn(),
}));

vi.mock('antd', () => ({
  Form: {
    useForm: vi.fn(() => [formState]),
    useWatch: vi.fn((field: string) => watchValues[field]),
  },
  message: {
    useMessage: () => [
      {
        success: messageSuccessMock,
        error: messageErrorMock,
        warning: messageWarningMock,
        info: messageInfoMock,
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

import { useVMManagementController } from './useVMManagementController';

describe('useVMManagementController', () => {
  const t = ((key: string) => key) as unknown as TFunction;

  beforeEach(() => {
    vi.clearAllMocks();
    formState.getFieldsValue.mockReturnValue({
      service_id: 'svc-1',
      template_id: 'tpl-1',
      instance_size_id: 'size-1',
      namespace: 'prod',
      reason: 'scale up',
    });
    formState.validateFields.mockResolvedValue(undefined);

    let getCall = 0;
    useApiGetMock.mockImplementation(() => {
      getCall += 1;
      const slot = ((getCall - 1) % 7) + 1;
      if (slot === 1) return { data: { items: [] }, isLoading: false, refetch: vi.fn() };
      if (slot === 2) return { data: { items: [{ id: 'sys-1', name: 'System A' }] }, isLoading: false };
      if (slot === 3) return { data: { items: [{ id: 'svc-1', name: 'Service A' }] }, isLoading: false };
      if (slot === 4) {
        return {
          data: {
            namespaces: ['prod'],
            templates: [{ id: 'tpl-1', name: 'Ubuntu Template' }],
            instance_sizes: [{ id: 'size-1', name: 'small', cpu_cores: 2, memory_mb: 4096 }],
          },
          isLoading: false,
        };
      }
      if (slot === 5) return { data: { items: [] }, isLoading: false };
      if (slot === 6) return { data: { items: [] }, isLoading: false };
      return {
        data: {
          batch_id: 'batch-live-1',
          operation: 'CREATE',
          status: 'PARTIAL_SUCCESS',
          child_count: 3,
          success_count: 1,
          failed_count: 1,
          pending_count: 1,
          created_by: 'owner-1',
          created_at: '2026-02-15T00:00:00Z',
          updated_at: '2026-02-15T00:00:00Z',
          children: [
            { ticket_id: 'ticket-failed-1', event_id: 'ev-1', status: 'FAILED', resource_name: 'vm-a', attempt_count: 2 },
            { ticket_id: 'ticket-pending-1', event_id: 'ev-2', status: 'PENDING', resource_name: 'vm-b', attempt_count: 0 },
            { ticket_id: 'ticket-success-1', event_id: 'ev-3', status: 'SUCCESS', resource_name: 'vm-c', attempt_count: 1 },
          ],
        },
        isLoading: false,
        refetch: batchStatusRefetchMock,
      };
    });

    let mutationCall = 0;
    useApiMutationMock.mockImplementation(() => {
      mutationCall += 1;
      const slot = ((mutationCall - 1) % 8) + 1;
      if (slot === 1) return { mutate: createMutate, isPending: false };
      if (slot === 2) return { mutate: createBatchMutate, isPending: false };
      if (slot === 3) return { mutate: vmBatchMutate, isPending: false };
      if (slot === 4) return { mutate: vmBatchPowerMutate, isPending: false };
      if (slot === 5) return { mutate: retryBatchMutate, isPending: false };
      if (slot === 6) return { mutate: cancelBatchMutate, isPending: false };
      if (slot === 7) return { mutate: requestConsoleMutate, isPending: false };
      return { mutate: deleteMutate, isPending: false };
    });

    let actionCall = 0;
    useApiActionMock.mockImplementation(() => {
      actionCall += 1;
      if (actionCall % 3 === 1) return { mutate: postStartMutate, isPending: false };
      if (actionCall % 3 === 2) return { mutate: postStopMutate, isPending: false };
      return { mutate: postRestartMutate, isPending: false };
    });

    apiGetMock.mockImplementation((path: string, options?: { params?: { path?: { vm_id?: string } } }) => {
      if (path === '/vms/{vm_id}') {
        const vmID = options?.params?.path?.vm_id ?? 'vm-1';
        return Promise.resolve({
          data: { id: vmID, name: vmID === 'vm-2' ? 'vm-two' : 'vm-one', status: 'RUNNING' },
          error: undefined,
          response: new Response(),
        });
      }
      if (path === '/vms/{vm_id}/console/status') {
        return Promise.resolve({
          data: { status: 'PENDING_APPROVAL' },
          error: undefined,
          response: new Response(),
        });
      }
      if (path === '/vms/{vm_id}/vnc') {
        return Promise.resolve({
          data: { status: 'SESSION_READY', vm_id: 'vm-1', websocket_path: '/api/v1/vms/vm-1/vnc' },
          error: undefined,
          response: new Response(),
        });
      }
      return Promise.resolve({ data: undefined, error: undefined, response: new Response() });
    });
  });

  it('advances wizard steps after validating required fields and submits request payload', async () => {
    const { result } = renderHook(() => useVMManagementController({ t }));

    act(() => {
      result.current.openWizard();
      result.current.onSystemChange('sys-1');
    });
    expect(formState.setFieldValue).toHaveBeenCalledWith('service_id', undefined);

    await act(async () => {
      await result.current.goToNextWizardStep();
    });
    expect(formState.validateFields).toHaveBeenCalledWith(['service_id']);

    await act(async () => {
      result.current.submitWizard();
    });
    expect(createMutate).toHaveBeenCalledWith({
      service_id: 'svc-1',
      template_id: 'tpl-1',
      instance_size_id: 'size-1',
      namespace: 'prod',
      reason: 'scale up',
    });
  });

  it('dispatches vm power, console, and delete actions with vm identity', async () => {
    const { result } = renderHook(() => useVMManagementController({ t }));

    await act(async () => {
      result.current.startVM('vm-1');
      result.current.stopVM('vm-1');
      result.current.restartVM('vm-1');
      await result.current.requestConsole('vm-1');
      await result.current.deleteVM('vm-2', 'vm-two');
    });

    expect(postStartMutate).toHaveBeenCalledWith('vm-1');
    expect(postStopMutate).toHaveBeenCalledWith('vm-1');
    expect(postRestartMutate).toHaveBeenCalledWith('vm-1');
    expect(requestConsoleMutate).toHaveBeenCalledWith('vm-1', expect.any(Object));
    expect(deleteMutate).toHaveBeenCalledWith({ vmId: 'vm-2', vmName: 'vm-two' });
  });

  it('submits create as batch when batch_count > 1', async () => {
    watchValues.batch_count = 3;
    formState.getFieldsValue.mockReturnValue({
      service_id: 'svc-1',
      template_id: 'tpl-1',
      instance_size_id: 'size-1',
      namespace: 'prod',
      reason: 'scale up',
      batch_count: 3,
    });

    const { result } = renderHook(() => useVMManagementController({ t }));

    await act(async () => {
      result.current.submitWizard();
    });

    expect(createBatchMutate).toHaveBeenCalledWith({
      operation: 'CREATE',
      reason: 'scale up',
      items: [
        {
          service_id: 'svc-1',
          template_id: 'tpl-1',
          instance_size_id: 'size-1',
          namespace: 'prod',
          reason: 'scale up',
        },
        {
          service_id: 'svc-1',
          template_id: 'tpl-1',
          instance_size_id: 'size-1',
          namespace: 'prod',
          reason: 'scale up',
        },
        {
          service_id: 'svc-1',
          template_id: 'tpl-1',
          instance_size_id: 'size-1',
          namespace: 'prod',
          reason: 'scale up',
        },
      ],
    });
    watchValues.batch_count = 1;
  });

  it('submits batch power/delete with selected VM ids', () => {
    const { result } = renderHook(() => useVMManagementController({ t }));

    act(() => {
      result.current.setSelectedVMIDs(['vm-1', 'vm-2']);
    });
    act(() => {
      result.current.submitBatchPowerSelected('START');
      result.current.submitBatchDeleteSelected();
    });

    expect(vmBatchPowerMutate).toHaveBeenCalledWith({
      operation: 'START',
      reason: 'batch.power_reason',
      items: [
        { vm_id: 'vm-1', reason: 'batch.power_reason' },
        { vm_id: 'vm-2', reason: 'batch.power_reason' },
      ],
    });
    expect(vmBatchMutate).toHaveBeenCalledWith({
      operation: 'DELETE',
      reason: 'batch.delete_reason',
      items: [
        { vm_id: 'vm-1', reason: 'batch.delete_reason' },
        { vm_id: 'vm-2', reason: 'batch.delete_reason' },
      ],
    });
  });

  it('uses status_url for active batch tracking when batch submit succeeds', () => {
    const { result } = renderHook(() => useVMManagementController({ t }));

    const createBatchOptions = useApiMutationMock.mock.calls[1]?.[1] as {
      onSuccess?: (data: {
        batch_id: string;
        status: string;
        status_url: string;
        retry_after_seconds: number;
      }) => void;
    };

    act(() => {
      createBatchOptions.onSuccess?.({
        batch_id: 'fallback-id',
        status: 'PENDING_APPROVAL',
        status_url: '/api/v1/vms/batch/batch-from-status-url',
        retry_after_seconds: 3,
      });
    });

    expect(result.current.activeBatchID).toBe('batch-from-status-url');
    expect(result.current.activeBatchStatusURL).toBe('/api/v1/vms/batch/batch-from-status-url');
  });

  it('enters cooldown on BATCH_RATE_LIMITED and blocks batch actions while countdown active', () => {
    watchValues.batch_count = 3;
    formState.getFieldsValue.mockReturnValue({
      service_id: 'svc-1',
      template_id: 'tpl-1',
      instance_size_id: 'size-1',
      namespace: 'prod',
      reason: 'scale up',
      batch_count: 3,
    });

    const { result } = renderHook(() => useVMManagementController({ t }));

    const createBatchOptions = useApiMutationMock.mock.calls[1]?.[1] as {
      onError?: (error: { code: string; params?: Record<string, unknown> }) => void;
    };

    act(() => {
      createBatchOptions.onError?.({
        code: 'BATCH_RATE_LIMITED',
        params: { retry_after_seconds: 5 },
      });
      result.current.setSelectedVMIDs(['vm-1', 'vm-2']);
    });

    expect(result.current.batchRateLimited).toBe(true);
    expect(result.current.batchRetryAfterSeconds).toBeGreaterThan(0);

    act(() => {
      result.current.submitWizard();
      result.current.submitBatchPowerSelected('START');
      result.current.submitBatchDeleteSelected();
    });

    expect(createBatchMutate).not.toHaveBeenCalled();
    expect(vmBatchPowerMutate).not.toHaveBeenCalled();
    expect(vmBatchMutate).not.toHaveBeenCalled();
    expect(messageWarningMock).toHaveBeenCalledWith('batch.rate_limited_wait');

    watchValues.batch_count = 1;
  });

  it('records affected child ticket ids for retry/cancel feedback', () => {
    const { result } = renderHook(() => useVMManagementController({ t }));

    const createBatchOptions = useApiMutationMock.mock.calls[1]?.[1] as {
      onSuccess?: (data: {
        batch_id: string;
        status: string;
        status_url: string;
        retry_after_seconds: number;
      }) => void;
    };
    const retryOptions = useApiMutationMock.mock.calls[4]?.[1] as {
      onSuccess?: (data: {
        batch_id: string;
        status: string;
        affected_count: number;
        affected_ticket_ids?: string[];
      }) => void;
    };
    const cancelOptions = useApiMutationMock.mock.calls[5]?.[1] as {
      onSuccess?: (data: {
        batch_id: string;
        status: string;
        affected_count: number;
        affected_ticket_ids?: string[];
      }) => void;
    };

    act(() => {
      createBatchOptions.onSuccess?.({
        batch_id: 'fallback-id',
        status: 'PENDING_APPROVAL',
        status_url: '/api/v1/vms/batch/batch-live-1',
        retry_after_seconds: 2,
      });
    });

    act(() => {
      result.current.retryBatch();
      retryOptions.onSuccess?.({
        batch_id: 'batch-live-1',
        status: 'IN_PROGRESS',
        affected_count: 1,
        affected_ticket_ids: ['ticket-failed-1'],
      });
    });

    expect(retryBatchMutate).toHaveBeenCalledWith('batch-live-1');
    expect(result.current.lastBatchActionFeedback).toEqual({
      action: 'retry',
      affectedCount: 1,
      affectedTicketIDs: ['ticket-failed-1'],
    });

    act(() => {
      result.current.cancelBatch();
      cancelOptions.onSuccess?.({
        batch_id: 'batch-live-1',
        status: 'CANCELLED',
        affected_count: 1,
        affected_ticket_ids: ['ticket-pending-1'],
      });
    });

    expect(cancelBatchMutate).toHaveBeenCalledWith('batch-live-1');
    expect(result.current.lastBatchActionFeedback).toEqual({
      action: 'cancel',
      affectedCount: 1,
      affectedTicketIDs: ['ticket-pending-1'],
    });
  });
});
