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
  apiGetMock,
  addFormState,
  createUserFormState,
  editUserFormState,
  roleBindingCreateFormState,
  roleBindingsRefetchMock,
} = vi.hoisted(() => ({
  useApiGetMock: vi.fn(),
  useApiMutationMock: vi.fn(),
  useApiActionMock: vi.fn(),
  useFormMock: vi.fn(),
  messageSuccessMock: vi.fn(),
  messageErrorMock: vi.fn(),
  apiGetMock: vi.fn(),
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
  roleBindingCreateFormState: {
    validateFields: vi.fn(),
    resetFields: vi.fn(),
    setFieldsValue: vi.fn(),
  },
  roleBindingsRefetchMock: vi.fn(),
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

vi.mock('@/lib/api/client', () => ({
  api: {
    GET: (...args: unknown[]) => apiGetMock(...args),
  },
}));

import { useAdminUsersController } from './useAdminUsersController';

describe('useAdminUsersController', () => {
  const t = ((key: string) => key) as unknown as TFunction;

  beforeEach(() => {
    vi.clearAllMocks();

    let formCall = 0;
    useFormMock.mockImplementation(() => {
      const slot = formCall % 4;
      formCall += 1;
      if (slot === 0) return [addFormState];
      if (slot === 1) return [createUserFormState];
      if (slot === 2) return [editUserFormState];
      return [roleBindingCreateFormState];
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
            pagination: { total: 1 },
          },
          isLoading: false,
          refetch: vi.fn(),
        };
      }
      if (head === 'member-systems') {
        return {
          data: { items: [{ id: 'sys-1', name: 'system-a' }] },
          isLoading: false,
          refetch: vi.fn(),
        };
      }
      if (head === 'system-members') {
        return {
          data: { items: [] },
          isLoading: false,
          refetch: vi.fn(),
        };
      }
      if (head === 'user-role-bindings') {
        return {
          data: { items: [], pagination: { total: 0 } },
          isLoading: false,
          refetch: roleBindingsRefetchMock,
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
        data: { items: [], generated_at: new Date().toISOString() },
        isLoading: false,
        isFetching: false,
        refetch: vi.fn(),
      };
    });
  });

  it('submits create/edit/delete user operations with expected payload', async () => {
    const createUserMutate = vi.fn();
    const updateUserMutate = vi.fn();
    const deleteUserMutate = vi.fn();

    const mutationResults = [
      { mutate: createUserMutate, mutateAsync: vi.fn(), isPending: false },
      { mutate: updateUserMutate, mutateAsync: vi.fn(), isPending: false },
      { mutate: vi.fn(), mutateAsync: vi.fn(), isPending: false },
      { mutate: vi.fn(), mutateAsync: vi.fn(), isPending: false },
      { mutate: vi.fn(), mutateAsync: vi.fn(), isPending: false },
      { mutate: vi.fn(), mutateAsync: vi.fn(), isPending: false },
      { mutate: vi.fn(), mutateAsync: vi.fn(), isPending: false },
    ];
    let mutationCall = 0;
    useApiMutationMock.mockImplementation(() => {
      const result = mutationResults[mutationCall % 7];
      mutationCall += 1;
      return result;
    });

    const actionResults = [
      { mutate: deleteUserMutate, isPending: false },
      { mutate: vi.fn(), isPending: false },
      { mutate: vi.fn(), isPending: false },
      { mutate: vi.fn(), isPending: false },
    ];
    let actionCall = 0;
    useApiActionMock.mockImplementation(() => {
      const result = actionResults[actionCall % 4];
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

  it('queries system-scoped member candidates instead of reusing the paginated user directory', async () => {
    useApiMutationMock.mockImplementation(() => ({ mutate: vi.fn(), mutateAsync: vi.fn(), isPending: false }));
    useApiActionMock.mockImplementation(() => ({ mutate: vi.fn(), isPending: false }));
    apiGetMock.mockResolvedValue({
      data: { items: [], pagination: { total: 0, page: 1, per_page: 50, total_pages: 0 } },
      error: undefined,
      response: new Response(),
    });

    const { result } = renderHook(() => useAdminUsersController({ t }));

    act(() => {
      result.current.setSelectedSystemId('sys-1');
    });
    act(() => {
      result.current.openAddModal();
    });
    act(() => {
      result.current.setMemberCandidateSearch('alice');
    });

    const candidateCall = [...useApiGetMock.mock.calls]
      .reverse()
      .find((call) => Array.isArray(call[0]) && call[0][0] === 'system-member-candidates');

    expect(candidateCall?.[0]).toEqual(['system-member-candidates', 'sys-1', 'alice']);
    expect(candidateCall?.[2]).toEqual({ enabled: true });

    const fetcher = candidateCall?.[1] as (() => Promise<unknown>) | undefined;
    await fetcher?.();

    expect(apiGetMock).toHaveBeenCalledWith('/systems/{system_id}/member-candidates', {
      params: {
        path: { system_id: 'sys-1' },
        query: {
          page: 1,
          per_page: 50,
          search: 'alice',
        },
      },
    });
  });

  it('passes directory search text through the admin users query', async () => {
    useApiMutationMock.mockImplementation(() => ({ mutate: vi.fn(), mutateAsync: vi.fn(), isPending: false }));
    useApiActionMock.mockImplementation(() => ({ mutate: vi.fn(), isPending: false }));
    apiGetMock.mockResolvedValue({
      data: { items: [], pagination: { total: 0, page: 1, per_page: 20, total_pages: 0 } },
      error: undefined,
      response: new Response(),
    });

    const { result } = renderHook(() => useAdminUsersController({ t }));

    act(() => {
      result.current.setSearch('department:Engineering');
    });

    const usersCall = [...useApiGetMock.mock.calls]
      .reverse()
      .find((call) => Array.isArray(call[0]) && call[0][0] === 'admin-users');

    expect(usersCall?.[0]).toEqual(['admin-users', 1, 20, 'department:Engineering']);

    const fetcher = usersCall?.[1] as (() => Promise<unknown>) | undefined;
    await fetcher?.();

    expect(apiGetMock).toHaveBeenCalledWith('/admin/users', {
      params: {
        query: {
          page: 1,
          per_page: 20,
          search: 'department:Engineering',
        },
      },
    });
  });

  it('closes and refreshes role bindings after creating a binding', async () => {
    const createRoleBindingMutateAsync = vi.fn().mockResolvedValue({
      id: 'binding-1',
      role_id: 'role-1',
    });

    const mutationResults = [
      { mutate: vi.fn(), mutateAsync: vi.fn(), isPending: false },
      { mutate: vi.fn(), mutateAsync: vi.fn(), isPending: false },
      { mutate: vi.fn(), mutateAsync: vi.fn(), isPending: false },
      { mutate: vi.fn(), mutateAsync: vi.fn(), isPending: false },
      { mutate: vi.fn(), mutateAsync: vi.fn(), isPending: false },
      { mutate: vi.fn(), mutateAsync: vi.fn(), isPending: false },
      { mutate: vi.fn(), mutateAsync: createRoleBindingMutateAsync, isPending: false },
    ];
    let mutationCall = 0;
    useApiMutationMock.mockImplementation(() => {
      const result = mutationResults[mutationCall % 7];
      mutationCall += 1;
      return result;
    });
    useApiActionMock.mockImplementation(() => ({ mutate: vi.fn(), isPending: false }));
    roleBindingsRefetchMock.mockResolvedValue(undefined);
    roleBindingCreateFormState.validateFields.mockResolvedValue({
      role_id: 'role-1',
      scope_type: 'global',
    });

    const { result } = renderHook(() => useAdminUsersController({ t }));

    act(() => {
      result.current.openRoleBindingsModal({
        id: 'u-1',
        username: 'user1',
        display_name: 'User One',
      });
      result.current.openRoleBindingCreateModal();
    });

    await act(async () => {
      await result.current.submitCreateRoleBinding();
    });

    expect(createRoleBindingMutateAsync).toHaveBeenCalledWith({
      role_id: 'role-1',
      scope_type: 'global',
    });
    expect(roleBindingsRefetchMock).toHaveBeenCalled();
    expect(roleBindingCreateFormState.resetFields).toHaveBeenCalled();
  });
});
