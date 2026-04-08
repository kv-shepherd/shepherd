import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, renderHook } from '@testing-library/react';
import type { PropsWithChildren } from 'react';
import type { TFunction } from 'i18next';
import { beforeEach, describe, expect, it, vi } from 'vitest';

const {
  useApiGetMock,
  useApiMutationMock,
  useApiActionMock,
  useFormMock,
  messageSuccessMock,
  messageErrorMock,
  messageWarningMock,
  apiGetMock,
  apiPostMock,
  roleCreateFormState,
  roleEditFormState,
  bindingFormState,
} = vi.hoisted(() => ({
  useApiGetMock: vi.fn(),
  useApiMutationMock: vi.fn(),
  useApiActionMock: vi.fn(),
  useFormMock: vi.fn(),
  messageSuccessMock: vi.fn(),
  messageErrorMock: vi.fn(),
  messageWarningMock: vi.fn(),
  apiGetMock: vi.fn(),
  apiPostMock: vi.fn(),
  roleCreateFormState: {
    validateFields: vi.fn(),
    resetFields: vi.fn(),
    setFieldsValue: vi.fn(),
  },
  roleEditFormState: {
    validateFields: vi.fn(),
    resetFields: vi.fn(),
    setFieldsValue: vi.fn(),
  },
  bindingFormState: {
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
        warning: messageWarningMock,
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

vi.mock('@/lib/api/client', () => ({
  api: {
    GET: (...args: unknown[]) => apiGetMock(...args),
    POST: (...args: unknown[]) => apiPostMock(...args),
  },
}));

import { useAdminRbacController } from './useAdminRbacController';

describe('useAdminRbacController', () => {
  const t = ((key: string) => key) as unknown as TFunction;
  const createWrapper = () => {
    const queryClient = new QueryClient({
      defaultOptions: {
        queries: { retry: false },
        mutations: { retry: false },
      },
    });
    function QueryClientWrapper({ children }: PropsWithChildren) {
      return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>;
    }
    QueryClientWrapper.displayName = 'QueryClientWrapper';
    return QueryClientWrapper;
  };

  beforeEach(() => {
    vi.clearAllMocks();

    let formCall = 0;
    useFormMock.mockImplementation(() => {
      formCall += 1;
      if (formCall === 1) return [roleCreateFormState];
      if (formCall === 2) return [roleEditFormState];
      return [bindingFormState];
    });

    roleCreateFormState.validateFields.mockResolvedValue({
      name: 'ops_auditor',
      permissions: ['builtin_approval:view'],
      enabled: true,
    });
    bindingFormState.validateFields.mockResolvedValue({
      role_id: 'role-1',
      scope_type: 'global',
      allowed_environments: ['test'],
    });

    let queryCall = 0;
    useApiGetMock.mockImplementation(() => {
      queryCall += 1;
      if (queryCall === 1) {
        return {
          data: { items: [{ id: 'role-1', name: 'admin', permissions: ['platform:admin'], built_in: true, enabled: true }] },
          isLoading: false,
          refetch: vi.fn(),
        };
      }
      if (queryCall === 2) {
        return {
          data: { items: [{ key: 'builtin_approval:view', description: 'View built-in approval tasks' }] },
          isLoading: false,
          refetch: vi.fn(),
        };
      }
      if (queryCall === 3) {
        return {
          data: {
            items: [{ id: 'u-1', username: 'user1', enabled: true, created_at: new Date().toISOString() }],
          },
          isLoading: false,
          isFetching: false,
          refetch: vi.fn(),
        };
      }
      return {
        data: { items: [] },
        isLoading: false,
        isFetching: false,
        refetch: vi.fn(),
      };
    });
    apiPostMock.mockResolvedValue({
      data: {},
      error: undefined,
      response: new Response(null, { status: 200 }),
    });
  });

  it('submits role creation and user role binding payloads', async () => {
    const createRoleMutate = vi.fn();
    const mutationResults = [
      { mutate: createRoleMutate, isPending: false },
      { mutate: vi.fn(), isPending: false },
    ];
    let mutationCall = 0;
    useApiMutationMock.mockImplementation(() => {
      const result = mutationResults[mutationCall % mutationResults.length];
      mutationCall += 1;
      return result;
    });

    const actionResults = [
      { mutate: vi.fn(), isPending: false },
      { mutate: vi.fn(), isPending: false },
    ];
    let actionCall = 0;
    useApiActionMock.mockImplementation(() => {
      const result = actionResults[actionCall % actionResults.length];
      actionCall += 1;
      return result;
    });

    const { result } = renderHook(() => useAdminRbacController({ t }), { wrapper: createWrapper() });

    await act(async () => {
      result.current.openCreateRoleModal();
      await result.current.submitCreateRole();
    });
    expect(createRoleMutate).toHaveBeenCalledWith({
      name: 'ops_auditor',
      permissions: ['builtin_approval:view'],
      enabled: true,
    });

    act(() => {
      result.current.openAddBindingModal([
        {
          id: 'u-1',
          username: 'user1',
          enabled: true,
          created_at: new Date().toISOString(),
        },
      ]);
    });

    await act(async () => {
      await result.current.submitAddBinding();
    });

    expect(apiPostMock).toHaveBeenCalledWith('/admin/users/{user_id}/role-bindings', {
      params: { path: { user_id: 'u-1' } },
      body: {
        role_id: 'role-1',
        scope_type: 'global',
        scope_id: undefined,
        allowed_environments: ['test'],
      },
    });
  });

  it('hydrates role edit form fields after opening edit modal', async () => {
    useApiMutationMock
      .mockReturnValueOnce({ mutate: vi.fn(), isPending: false })
      .mockReturnValueOnce({ mutate: vi.fn(), isPending: false })
      .mockReturnValueOnce({ mutate: vi.fn(), isPending: false });
    useApiActionMock
      .mockReturnValueOnce({ mutate: vi.fn(), isPending: false })
      .mockReturnValueOnce({ mutate: vi.fn(), isPending: false });

    const { result } = renderHook(() => useAdminRbacController({ t }), { wrapper: createWrapper() });

    act(() => {
      result.current.openEditRoleModal({
        id: 'role-2',
        name: 'ops-editor',
        display_name: 'Ops Editor',
        description: 'edit ops resources',
        permissions: ['builtin_approval:view'],
        built_in: false,
        enabled: true,
      });
    });

    expect(roleEditFormState.resetFields).toHaveBeenCalled();
    expect(roleEditFormState.setFieldsValue).toHaveBeenCalledWith({
      display_name: 'Ops Editor',
      description: 'edit ops resources',
      permissions: ['builtin_approval:view'],
      enabled: true,
    });
  });

  it('queries the searchable admin users endpoint for the RBAC user picker', async () => {
    useApiMutationMock.mockImplementation(() => ({ mutate: vi.fn(), isPending: false }));
    useApiActionMock.mockImplementation(() => ({ mutate: vi.fn(), isPending: false }));
    apiGetMock.mockResolvedValue({
      data: { items: [], pagination: { total: 0, page: 1, per_page: 50, total_pages: 0 } },
      error: undefined,
      response: new Response(),
    });

    const { result } = renderHook(() => useAdminRbacController({ t }), { wrapper: createWrapper() });

    act(() => {
      result.current.setUserSearch('alice');
    });

    const usersCall = [...useApiGetMock.mock.calls]
      .reverse()
      .find((call) => Array.isArray(call[0]) && call[0][0] === 'admin-rbac-users');

    expect(usersCall?.[0]).toEqual(['admin-rbac-users', 'alice']);

    const fetcher = usersCall?.[1] as (() => Promise<unknown>) | undefined;
    await fetcher?.();

    expect(apiGetMock).toHaveBeenCalledWith('/admin/users', {
      params: {
        query: {
          page: 1,
          per_page: 50,
          search: 'alice',
        },
      },
    });
  });
});
