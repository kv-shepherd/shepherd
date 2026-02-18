import { act, renderHook } from '@testing-library/react';
import type { TFunction } from 'i18next';
import { beforeEach, describe, expect, it, vi } from 'vitest';

const {
  useApiGetMock,
  useApiMutationMock,
  useApiActionMock,
  formState,
  addMemberMutate,
  removeMemberMutate,
  updateRoleMutate,
  messageSuccessMock,
  messageErrorMock,
} = vi.hoisted(() => ({
  useApiGetMock: vi.fn(),
  useApiMutationMock: vi.fn(),
  useApiActionMock: vi.fn(),
  formState: {
    validateFields: vi.fn(),
    resetFields: vi.fn(),
  },
  addMemberMutate: vi.fn(),
  removeMemberMutate: vi.fn(),
  updateRoleMutate: vi.fn(),
  messageSuccessMock: vi.fn(),
  messageErrorMock: vi.fn(),
}));

vi.mock('antd', () => ({
  App: {
    useApp: () => ({
      message: {
        success: messageSuccessMock,
        error: messageErrorMock,
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

import { useSystemMembersController } from './useSystemMembersController';

describe('useSystemMembersController', () => {
  const t = ((key: string) => key) as unknown as TFunction;

  beforeEach(() => {
    vi.clearAllMocks();
    formState.validateFields.mockResolvedValue({
      user_id: 'user-1',
      role: 'member',
    });
    useApiGetMock.mockReturnValue({
      data: {
        items: [{ user_id: 'user-1', role: 'member' }],
      },
      isLoading: false,
      refetch: vi.fn(),
    });
    let mutationCall = 0;
    useApiMutationMock.mockImplementation(() => {
      mutationCall += 1;
      return mutationCall % 2 === 1
        ? { mutate: addMemberMutate, isPending: false }
        : { mutate: updateRoleMutate, isPending: false };
    });
    useApiActionMock.mockReturnValue({ mutate: removeMemberMutate, isPending: false });
  });

  it('submits add-member payload and closes modal state', async () => {
    const { result } = renderHook(() => useSystemMembersController({ t, systemId: 'sys-1' }));

    act(() => {
      result.current.openAddMemberModal();
    });
    expect(result.current.addMemberOpen).toBe(true);

    await act(async () => {
      await result.current.submitAddMember();
    });
    expect(addMemberMutate).toHaveBeenCalledWith({ user_id: 'user-1', role: 'member' });
  });

  it('dispatches remove/update role operations with user identity', () => {
    const { result } = renderHook(() => useSystemMembersController({ t, systemId: 'sys-1' }));

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
