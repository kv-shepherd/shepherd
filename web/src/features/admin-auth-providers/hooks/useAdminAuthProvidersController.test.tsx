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
  createFormState,
  editFormState,
} = vi.hoisted(() => ({
  useApiGetMock: vi.fn(),
  useApiMutationMock: vi.fn(),
  useApiActionMock: vi.fn(),
  useFormMock: vi.fn(),
  messageSuccessMock: vi.fn(),
  messageErrorMock: vi.fn(),
  createFormState: {
    validateFields: vi.fn(),
    resetFields: vi.fn(),
    setFieldsValue: vi.fn(),
  },
  editFormState: {
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

import { useAdminAuthProvidersController } from './useAdminAuthProvidersController';

describe('useAdminAuthProvidersController', () => {
  const t = ((key: string) => key) as unknown as TFunction;

  beforeEach(() => {
    vi.clearAllMocks();

    let formCall = 0;
    useFormMock.mockImplementation(() => {
      formCall += 1;
      return formCall === 1 ? [createFormState] : [editFormState];
    });

    createFormState.validateFields.mockResolvedValue({
      name: 'corp-oidc',
      auth_type: 'oidc',
      enabled: true,
      sort_order: 10,
      config_text: '{"issuer":"https://idp.example.com"}',
    });

    useApiGetMock.mockReturnValue({
      data: { items: [] },
      isLoading: false,
      refetch: vi.fn(),
    });
  });

  it('submits create payload with parsed JSON config', async () => {
    const createMutate = vi.fn();

    let mutationCall = 0;
    useApiMutationMock.mockImplementation(() => {
      mutationCall += 1;
      if (mutationCall === 1) return { mutate: createMutate, isPending: false };
      return { mutate: vi.fn(), isPending: false };
    });

    useApiActionMock.mockReturnValue({ mutate: vi.fn(), isPending: false });

    const { result } = renderHook(() => useAdminAuthProvidersController({ t }));

    await act(async () => {
      result.current.openCreateModal();
      await result.current.submitCreate();
    });

    expect(createMutate).toHaveBeenCalledWith({
      name: 'corp-oidc',
      auth_type: 'oidc',
      enabled: true,
      sort_order: 10,
      config: { issuer: 'https://idp.example.com' },
    });
  });

  it('blocks create mutation when config JSON is invalid', async () => {
    const createMutate = vi.fn();

    let mutationCall = 0;
    useApiMutationMock.mockImplementation(() => {
      mutationCall += 1;
      if (mutationCall === 1) return { mutate: createMutate, isPending: false };
      return { mutate: vi.fn(), isPending: false };
    });

    useApiActionMock.mockReturnValue({ mutate: vi.fn(), isPending: false });

    createFormState.validateFields.mockResolvedValueOnce({
      name: 'bad-json',
      auth_type: 'oidc',
      config_text: '{not-json}',
    });

    const { result } = renderHook(() => useAdminAuthProvidersController({ t }));

    await act(async () => {
      result.current.openCreateModal();
      await result.current.submitCreate();
    });

    expect(createMutate).not.toHaveBeenCalled();
    expect(messageErrorMock).toHaveBeenCalled();
  });

  it('uses backend-discovered provider types when opening create modal', async () => {
    useApiGetMock
      .mockImplementationOnce(() => ({
        data: { items: [] },
        isLoading: false,
        refetch: vi.fn(),
      }))
      .mockImplementationOnce(() => ({
        data: { items: [{ type: 'custom-sso', display_name: 'Custom SSO', built_in: false }] },
        isLoading: false,
        refetch: vi.fn(),
      }))
      .mockImplementation(() => ({
        data: { items: [] },
        isLoading: false,
        refetch: vi.fn(),
      }));

    useApiMutationMock.mockReturnValue({ mutate: vi.fn(), isPending: false });
    useApiActionMock.mockReturnValue({ mutate: vi.fn(), isPending: false });

    const { result } = renderHook(() => useAdminAuthProvidersController({ t }));

    await act(async () => {
      result.current.openCreateModal();
    });

    expect(createFormState.setFieldsValue).toHaveBeenCalledWith(
      expect.objectContaining({ auth_type: 'custom-sso' })
    );
  });
});
