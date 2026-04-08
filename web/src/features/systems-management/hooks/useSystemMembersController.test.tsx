import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, renderHook } from '@testing-library/react';
import type { PropsWithChildren } from 'react';
import type { TFunction } from 'i18next';
import { beforeEach, describe, expect, it, vi } from 'vitest';

const {
  useApiGetMock,
  useApiMutationMock,
  useApiActionMock,
  formState,
  removeMemberMutate,
  updateRoleMutate,
  messageSuccessMock,
  messageErrorMock,
  messageWarningMock,
  apiPostMock,
} = vi.hoisted(() => ({
  useApiGetMock: vi.fn(),
  useApiMutationMock: vi.fn(),
  useApiActionMock: vi.fn(),
  formState: {
    validateFields: vi.fn(),
    resetFields: vi.fn(),
  },
  removeMemberMutate: vi.fn(),
  updateRoleMutate: vi.fn(),
  messageSuccessMock: vi.fn(),
  messageErrorMock: vi.fn(),
  messageWarningMock: vi.fn(),
  apiPostMock: vi.fn(),
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
    useForm: vi.fn(() => [formState]),
  },
}));

vi.mock('@/hooks/useApiQuery', () => ({
  useApiGet: (...args: unknown[]) => useApiGetMock(...args),
  useApiMutation: (...args: unknown[]) => useApiMutationMock(...args),
  useApiAction: (...args: unknown[]) => useApiActionMock(...args),
}));

vi.mock('@/lib/api/client', () => ({
  api: {
    POST: (...args: unknown[]) => apiPostMock(...args),
  },
}));

import { useSystemMembersController } from './useSystemMembersController';

describe('useSystemMembersController', () => {
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
    formState.validateFields.mockResolvedValue({
      user_id: 'user-1',
      role: 'member',
    });
    apiPostMock.mockResolvedValue({
      data: {},
      error: undefined,
      response: new Response(null, { status: 200 }),
    });
    useApiGetMock.mockReturnValue({
      data: {
        items: [{ user_id: 'user-1', role: 'member' }],
      },
      isLoading: false,
      refetch: vi.fn(),
    });
    useApiMutationMock.mockReturnValue({ mutate: updateRoleMutate, isPending: false });
    useApiActionMock.mockReturnValue({ mutate: removeMemberMutate, isPending: false });
  });

  it('submits add-member payload and closes modal state', async () => {
    const { result } = renderHook(() => useSystemMembersController({ t, systemId: 'sys-1' }), {
      wrapper: createWrapper(),
    });

    act(() => {
      result.current.openAddMemberModal();
      result.current.setSelectedCandidateUsers([
        'user-1',
      ], [
        {
          id: 'user-1',
          username: 'user1',
          enabled: true,
          created_at: new Date().toISOString(),
        },
      ]);
    });
    expect(result.current.addMemberOpen).toBe(true);

    await act(async () => {
      await result.current.submitAddMember();
    });
    expect(apiPostMock).toHaveBeenCalledWith('/systems/{system_id}/members', {
      params: { path: { system_id: 'sys-1' } },
      body: { user_id: 'user-1', role: 'member' },
    });
  });

  it('dispatches remove/update role operations with user identity', () => {
    const { result } = renderHook(() => useSystemMembersController({ t, systemId: 'sys-1' }), {
      wrapper: createWrapper(),
    });

    act(() => {
      result.current.removeMember('user-2');
      result.current.updateRole('user-3', 'admin');
    });

    expect(removeMemberMutate).toHaveBeenCalledWith({ userId: 'user-2' });
    expect(updateRoleMutate).toHaveBeenCalledWith({
      userId: 'user-3',
      body: { role: 'admin' },
    });
  });
});
