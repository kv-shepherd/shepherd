import { act, renderHook } from '@testing-library/react';
import type { TFunction } from 'i18next';
import { beforeEach, describe, expect, it, vi } from 'vitest';

const {
  useApiGetMock,
  useApiMutationMock,
  useApiActionMock,
  useFormMock,
  createFormState,
  editFormState,
  messageSuccessMock,
  messageErrorMock,
  apiGetMock,
} = vi.hoisted(() => ({
  useApiGetMock: vi.fn(),
  useApiMutationMock: vi.fn(),
  useApiActionMock: vi.fn(),
  useFormMock: vi.fn(),
  createFormState: {
    validateFields: vi.fn(),
    resetFields: vi.fn(),
    setFieldsValue: vi.fn(),
    setFieldValue: vi.fn(),
    getFieldsValue: vi.fn(),
  },
  editFormState: {
    validateFields: vi.fn(),
    resetFields: vi.fn(),
    setFieldsValue: vi.fn(),
    setFieldValue: vi.fn(),
    getFieldsValue: vi.fn(),
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
  useApiAction: (...args: unknown[]) => useApiActionMock(...args),
}));

vi.mock('@/lib/api/client', () => ({
  api: {
    GET: (...args: unknown[]) => apiGetMock(...args),
  },
}));

import { useSystemsManagementController } from './useSystemsManagementController';

describe('useSystemsManagementController', () => {
  const t = ((key: string) => key) as unknown as TFunction;

  beforeEach(() => {
    vi.clearAllMocks();
    let formCall = 0;
    useFormMock.mockImplementation(() => {
      formCall += 1;
      return formCall % 2 === 1 ? [createFormState] : [editFormState];
    });
    createFormState.validateFields.mockResolvedValue({
      name: 'sys-a',
      description: 'desc-a',
    });
    editFormState.validateFields.mockResolvedValue({
      description: 'updated',
    });
    useApiGetMock.mockReturnValue({
      data: { items: [{ id: 'sys-1', name: 'System A', description: '' }] },
      isLoading: false,
      refetch: vi.fn(),
    });
    apiGetMock.mockResolvedValue({
      data: { id: 'sys-1', name: 'System A', description: 'old' },
      error: undefined,
      response: new Response(),
    });
  });

  it('submits create and delete operations with expected payload', async () => {
    const createMutate = vi.fn();
    const updateMutate = vi.fn();
    const deleteMutate = vi.fn();

    let mutationCall = 0;
    useApiMutationMock.mockImplementation(() => {
      mutationCall += 1;
      return mutationCall % 2 === 1
        ? { mutate: createMutate, isPending: false }
        : { mutate: updateMutate, isPending: false };
    });
    useApiActionMock.mockReturnValue({ mutate: deleteMutate, isPending: false });

    const { result } = renderHook(() => useSystemsManagementController({ t }));

    await act(async () => {
      await result.current.submitCreate();
    });
    expect(createFormState.validateFields).toHaveBeenCalled();
    expect(createMutate).toHaveBeenCalledWith({ name: 'sys-a', description: 'desc-a' });

    await act(async () => {
      result.current.openDeleteModal({ id: 'sys-1', name: 'System A', description: '' } as never);
      await Promise.resolve();
      result.current.setDeleteConfirmName('System A');
    });
    act(() => {
      result.current.submitDelete();
    });
    expect(deleteMutate).toHaveBeenCalledWith({ id: 'sys-1', confirmName: 'System A' });
  });

  it('submits description-only edit with selected system id', async () => {
    const createMutate = vi.fn();
    const updateMutate = vi.fn();
    const deleteMutate = vi.fn();

    let mutationCall = 0;
    useApiMutationMock.mockImplementation(() => {
      mutationCall += 1;
      return mutationCall % 2 === 1
        ? { mutate: createMutate, isPending: false }
        : { mutate: updateMutate, isPending: false };
    });
    useApiActionMock.mockReturnValue({ mutate: deleteMutate, isPending: false });

    const { result } = renderHook(() => useSystemsManagementController({ t }));

    await act(async () => {
      result.current.openEditModal({ id: 'sys-1', name: 'System A', description: 'old' } as never);
      await Promise.resolve();
    });
    expect(editFormState.setFieldsValue).toHaveBeenCalledWith({ description: 'old' });

    await act(async () => {
      await result.current.submitEdit();
    });
    expect(updateMutate).toHaveBeenCalledWith({
      id: 'sys-1',
      body: { description: 'updated' },
    });
  });
});
