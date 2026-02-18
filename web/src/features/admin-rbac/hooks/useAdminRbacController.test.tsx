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
  messageWarningMock,
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
  Form: {
    useForm: (...args: unknown[]) => useFormMock(...args),
  },
  message: {
    useMessage: () => [
      {
        success: messageSuccessMock,
        error: messageErrorMock,
        warning: messageWarningMock,
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

import { useAdminRbacController } from './useAdminRbacController';

describe('useAdminRbacController', () => {
  const t = ((key: string) => key) as unknown as TFunction;

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
      permissions: ['approval:view'],
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
          data: { items: [{ key: 'approval:view', description: 'View approval tickets' }] },
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
          refetch: vi.fn(),
        };
      }
      return {
        data: { items: [] },
        isLoading: false,
        refetch: vi.fn(),
      };
    });
  });

  it('submits role creation and user role binding payloads', async () => {
    const createRoleMutate = vi.fn();
    const createBindingMutate = vi.fn();

    const mutationResults = [
      { mutate: createRoleMutate, isPending: false },
      { mutate: vi.fn(), isPending: false },
      { mutate: createBindingMutate, isPending: false },
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

    const { result } = renderHook(() => useAdminRbacController({ t }));

    await act(async () => {
      result.current.openCreateRoleModal();
      await result.current.submitCreateRole();
    });
    expect(createRoleMutate).toHaveBeenCalledWith({
      name: 'ops_auditor',
      permissions: ['approval:view'],
      enabled: true,
    });

    act(() => {
      result.current.setSelectedUserId('u-1');
    });

    await act(async () => {
      result.current.openAddBindingModal();
      await result.current.submitAddBinding();
    });

    expect(createBindingMutate).toHaveBeenCalledWith({
      role_id: 'role-1',
      scope_type: 'global',
      scope_id: undefined,
      allowed_environments: ['test'],
    });
  });
});
