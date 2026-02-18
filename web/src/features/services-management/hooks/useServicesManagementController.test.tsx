import { act, renderHook } from '@testing-library/react';
import type { TFunction } from 'i18next';
import { beforeEach, describe, expect, it, vi } from 'vitest';

const {
  useApiGetMock,
  useApiMutationMock,
  useFormMock,
  formState,
  editFormState,
  messageSuccessMock,
  messageErrorMock,
  apiGetMock,
} = vi.hoisted(() => ({
  useApiGetMock: vi.fn(),
  useApiMutationMock: vi.fn(),
  useFormMock: vi.fn(),
  formState: {
    validateFields: vi.fn(),
    resetFields: vi.fn(),
    setFieldValue: vi.fn(),
    setFieldsValue: vi.fn(),
  },
  editFormState: {
    validateFields: vi.fn(),
    resetFields: vi.fn(),
    setFieldValue: vi.fn(),
    setFieldsValue: vi.fn(),
  },
  messageSuccessMock: vi.fn(),
  messageErrorMock: vi.fn(),
  apiGetMock: vi.fn(),
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
}));

vi.mock('@/lib/api/client', () => ({
  api: {
    GET: (...args: unknown[]) => apiGetMock(...args),
  },
}));

import { useServicesManagementController } from './useServicesManagementController';

describe('useServicesManagementController', () => {
  const t = ((key: string) => key) as unknown as TFunction;

  beforeEach(() => {
    vi.clearAllMocks();
    let formCall = 0;
    useFormMock.mockImplementation(() => {
      formCall += 1;
      return formCall % 2 === 1 ? [formState] : [editFormState];
    });
    formState.validateFields.mockResolvedValue({
      system_id: 'sys-1',
      name: 'svc-a',
      description: 'service a',
    });
    editFormState.validateFields.mockResolvedValue({
      description: 'updated description',
    });
    let getCall = 0;
    useApiGetMock.mockImplementation(() => {
      getCall += 1;
      if (getCall % 2 === 1) {
        return {
          data: { items: [{ id: 'sys-1', name: 'System A' }] },
          isLoading: false,
        };
      }
      return {
        data: { items: [{ id: 'svc-1', system_id: 'sys-1', name: 'Service A' }] },
        isLoading: false,
        refetch: vi.fn(),
      };
    });
    apiGetMock.mockResolvedValue({
      data: { id: 'svc-1', system_id: 'sys-1', name: 'Service A', description: 'old' },
      error: undefined,
      response: new Response(),
    });
  });

  it('submits create request with split system/body payload', async () => {
    const createMutate = vi.fn();
    const deleteMutate = vi.fn();
    const updateMutate = vi.fn();

    let mutationCall = 0;
    useApiMutationMock.mockImplementation(() => {
      mutationCall += 1;
      if (mutationCall%3 === 1) return { mutate: createMutate, isPending: false };
      if (mutationCall%3 === 2) return { mutate: deleteMutate, isPending: false };
      return { mutate: updateMutate, isPending: false };
    });

    const { result } = renderHook(() => useServicesManagementController({ t }));

    await act(async () => {
      await result.current.submitCreate();
    });

    expect(createMutate).toHaveBeenCalledWith({
      system_id: 'sys-1',
      body: {
        name: 'svc-a',
        description: 'service a',
      },
    });
  });

  it('submits update and delete operations for selected service', async () => {
    const createMutate = vi.fn();
    const deleteMutate = vi.fn();
    const updateMutate = vi.fn();

    let mutationCall = 0;
    useApiMutationMock.mockImplementation(() => {
      mutationCall += 1;
      if (mutationCall%3 === 1) return { mutate: createMutate, isPending: false };
      if (mutationCall%3 === 2) return { mutate: deleteMutate, isPending: false };
      return { mutate: updateMutate, isPending: false };
    });

    const { result } = renderHook(() => useServicesManagementController({ t }));

    await act(async () => {
      result.current.openEditModal({
        id: 'svc-1',
        system_id: 'sys-1',
        name: 'Service A',
        description: 'old',
      } as never);
      await Promise.resolve();
      result.current.submitDelete('sys-1', 'svc-1');
    });
    expect(deleteMutate).toHaveBeenCalledWith({ systemId: 'sys-1', serviceId: 'svc-1' });

    await act(async () => {
      await result.current.submitEdit();
    });
    expect(updateMutate).toHaveBeenCalledWith({
      systemId: 'sys-1',
      serviceId: 'svc-1',
      body: { description: 'updated description' },
    });
  });
});
