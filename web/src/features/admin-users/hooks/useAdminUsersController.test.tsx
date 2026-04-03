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
  createUserFormState,
  editUserFormState,
} = vi.hoisted(() => ({
  useApiGetMock: vi.fn(),
  useApiMutationMock: vi.fn(),
  useApiActionMock: vi.fn(),
  useFormMock: vi.fn(),
  messageSuccessMock: vi.fn(),
  messageErrorMock: vi.fn(),
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
  App: {
    useApp: () => ({
      message: {
        success: messageSuccessMock,
        error: messageErrorMock,
        warning: vi.fn(),
      },
    }),
  },
  Form: {
    useForm: (...args: unknown[]) => useFormMock(...args),
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
      const slot = formCall % 2;
      formCall += 1;
      if (slot === 0) return [createUserFormState];
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

    useApiGetMock.mockImplementation((queryKey?: unknown[]) => {
      const head = Array.isArray(queryKey) ? queryKey[0] : undefined;
      if (head === 'admin-users') {
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
            pagination: { total: 1, page: 1, per_page: 20 },
          },
          isLoading: false,
          refetch: vi.fn(),
        };
      }
      if (head === 'admin-roles-dropdown') {
        return {
          data: {
            items: [{ id: 'role-1', name: 'PlatformAdmin', display_name: 'Platform Admin', built_in: true }],
          },
          isLoading: false,
          refetch: vi.fn(),
        };
      }
      return {
        data: { items: [], pagination: { total: 0, page: 1, per_page: 20 } },
        isLoading: false,
        refetch: vi.fn(),
      };
    });

    useApiMutationMock.mockImplementation(() => ({ mutate: vi.fn(), isPending: false }));
    useApiActionMock.mockImplementation(() => ({ mutate: vi.fn(), isPending: false }));
  });

  it('submits create, edit, and delete user operations with expected payloads', async () => {
    const createUserMutate = vi.fn();
    const updateUserMutate = vi.fn();
    const deleteUserMutate = vi.fn();

    let mutationCall = 0;
    useApiMutationMock.mockImplementation(() => {
      const slot = mutationCall % 2;
      mutationCall += 1;
      return slot === 0
        ? { mutate: createUserMutate, isPending: false }
        : { mutate: updateUserMutate, isPending: false };
    });
    useApiActionMock.mockReturnValue({ mutate: deleteUserMutate, isPending: false });

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
    expect(createUserFormState.setFieldsValue).toHaveBeenCalledWith({
      enabled: true,
      force_password_change: true,
    });

    act(() => {
      result.current.openEditUserModal({
        id: 'u-1',
        username: 'user1',
        email: 'old@example.com',
        display_name: 'Existing User',
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
    expect(editUserFormState.setFieldsValue).toHaveBeenCalledWith({
      email: 'old@example.com',
      display_name: 'Existing User',
      enabled: true,
    });

    act(() => {
      result.current.deleteUser('u-2');
    });
    expect(deleteUserMutate).toHaveBeenCalledWith('u-2');
  });

  it('passes directory search text through the admin users query key', () => {
    useApiMutationMock
      .mockReturnValueOnce({ mutate: vi.fn(), isPending: false })
      .mockReturnValueOnce({ mutate: vi.fn(), isPending: false });
    useApiActionMock.mockReturnValue({ mutate: vi.fn(), isPending: false });

    const { result } = renderHook(() => useAdminUsersController({ t }));

    act(() => {
      result.current.setSearch('department:Engineering');
    });

    const usersCall = [...useApiGetMock.mock.calls]
      .reverse()
      .find((call) => Array.isArray(call[0]) && call[0][0] === 'admin-users');

    expect(usersCall?.[0]).toEqual(['admin-users', 1, 20, 'department:Engineering']);
  });

  it('exposes the role catalog query separately from the user directory', () => {
    useApiMutationMock
      .mockReturnValueOnce({ mutate: vi.fn(), isPending: false })
      .mockReturnValueOnce({ mutate: vi.fn(), isPending: false });
    useApiActionMock.mockReturnValue({ mutate: vi.fn(), isPending: false });

    const { result } = renderHook(() => useAdminUsersController({ t }));

    expect(result.current.roles?.items).toEqual([
      expect.objectContaining({
        id: 'role-1',
        name: 'PlatformAdmin',
      }),
    ]);
    expect(result.current.rolesLoading).toBe(false);
  });
});
