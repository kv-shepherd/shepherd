import { act, renderHook } from '@testing-library/react';
import type { TFunction } from 'i18next';
import { beforeEach, describe, expect, it, vi } from 'vitest';

const {
  useApiGetMock,
  useApiMutationMock,
  useApiActionMock,
  useFormMock,
  messageSuccessMock,
  messageErrorMock,
  addFormState,
  createUserFormState,
  editUserFormState,
} = vi.hoisted(() => ({
  useApiGetMock: vi.fn(),
  useApiMutationMock: vi.fn(),
  useApiActionMock: vi.fn(),
  useFormMock: vi.fn(),
  messageSuccessMock: vi.fn(),
  messageErrorMock: vi.fn(),
  addFormState: {
    validateFields: vi.fn(),
    resetFields: vi.fn(),
    setFieldsValue: vi.fn(),
  },
  createUserFormState: {
    validateFields: vi.fn(),
    resetFields: vi.fn(),
    setFieldsValue: vi.fn(),
  },
  editUserFormState: {
    validateFields: vi.fn(),
    resetFields: vi.fn(),
    setFieldsValue: vi.fn(),
  },
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
        warning: vi.fn(),
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

import { useAdminUsersController } from './useAdminUsersController';

describe('useAdminUsersController', () => {
  const t = ((key: string) => key) as unknown as TFunction;

  beforeEach(() => {
    vi.clearAllMocks();

    let formCall = 0;
    useFormMock.mockImplementation(() => {
      formCall += 1;
      if (formCall === 1) return [addFormState];
      if (formCall === 2) return [createUserFormState];
      return [editUserFormState];
    });

    createUserFormState.validateFields.mockResolvedValue({
      username: 'new-user',
      password: 'Passw0rd!',
      email: 'new@example.com',
      enabled: true,
      force_password_change: true,
    });
    editUserFormState.validateFields.mockResolvedValue({
      display_name: 'User One',
      email: 'user1@example.com',
      enabled: true,
      force_password_change: false,
    });

    let queryCall = 0;
    useApiGetMock.mockImplementation(() => {
      queryCall += 1;
      if (queryCall === 1) {
        return {
          data: {
            items: [
              {
                id: 'u-1',
                username: 'user1',
                enabled: true,
                created_at: new Date().toISOString(),
              },
            ],
            pagination: { total: 1 },
          },
          isLoading: false,
          refetch: vi.fn(),
        };
      }
      if (queryCall === 2) {
        return {
          data: { items: [{ id: 'sys-1', name: 'system-a' }] },
          isLoading: false,
          refetch: vi.fn(),
        };
      }
      if (queryCall === 3) {
        return {
          data: { items: [] },
          isLoading: false,
          refetch: vi.fn(),
        };
      }
      return {
        data: { items: [], generated_at: new Date().toISOString() },
        isLoading: false,
        refetch: vi.fn(),
      };
    });
  });

  it('submits create/edit/delete user operations with expected payload', async () => {
    const createUserMutate = vi.fn();
    const updateUserMutate = vi.fn();
    const deleteUserMutate = vi.fn();

    const mutationResults = [
      { mutate: createUserMutate, isPending: false },
      { mutate: updateUserMutate, isPending: false },
      { mutate: vi.fn(), isPending: false },
      { mutate: vi.fn(), isPending: false },
      { mutate: vi.fn(), isPending: false },
      { mutate: vi.fn(), isPending: false },
    ];
    let mutationCall = 0;
    useApiMutationMock.mockImplementation(() => {
      const result = mutationResults[mutationCall % mutationResults.length];
      mutationCall += 1;
      return result;
    });

    const actionResults = [
      { mutate: deleteUserMutate, isPending: false },
      { mutate: vi.fn(), isPending: false },
      { mutate: vi.fn(), isPending: false },
    ];
    let actionCall = 0;
    useApiActionMock.mockImplementation(() => {
      const result = actionResults[actionCall % actionResults.length];
      actionCall += 1;
      return result;
    });

    const { result } = renderHook(() => useAdminUsersController({ t }));

    await act(async () => {
      result.current.openCreateUserModal();
      await result.current.submitCreateUser();
    });
    expect(createUserMutate).toHaveBeenCalledWith({
      username: 'new-user',
      password: 'Passw0rd!',
      email: 'new@example.com',
      enabled: true,
      force_password_change: true,
    });

    act(() => {
      result.current.openEditUserModal({
        id: 'u-1',
        username: 'user1',
        enabled: true,
        created_at: new Date().toISOString(),
      } as never);
    });

    await act(async () => {
      await result.current.submitEditUser();
    });
    expect(updateUserMutate).toHaveBeenCalledWith({
      userId: 'u-1',
      body: {
        display_name: 'User One',
        email: 'user1@example.com',
        enabled: true,
        force_password_change: false,
      },
    });

    act(() => {
      result.current.deleteUser('u-2');
    });
    expect(deleteUserMutate).toHaveBeenCalledWith('u-2');
  });
});
