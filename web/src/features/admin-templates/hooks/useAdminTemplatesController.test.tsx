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

import { useAdminTemplatesController } from './useAdminTemplatesController';

describe('useAdminTemplatesController', () => {
  const t = ((key: string) => key) as unknown as TFunction;

  beforeEach(() => {
    vi.clearAllMocks();

    let formCall = 0;
    useFormMock.mockImplementation(() => {
      formCall += 1;
      if (formCall === 1) return [createFormState];
      return [editFormState];
    });

    useApiGetMock.mockReturnValue({
      data: { items: [], pagination: { page: 1, per_page: 20, total: 0, total_pages: 0 } },
      isLoading: false,
      refetch: vi.fn(),
    });
  });

  it('submits create payload with parsed spec JSON', async () => {
    const createMutate = vi.fn();
    const updateMutate = vi.fn();
    const deleteMutate = vi.fn();

    useApiMutationMock
      .mockReturnValueOnce({ mutate: createMutate, isPending: false })
      .mockReturnValueOnce({ mutate: updateMutate, isPending: false });
    useApiActionMock.mockReturnValue({ mutate: deleteMutate, isPending: false });

    createFormState.validateFields.mockResolvedValue({
      name: 'ubuntu-base',
      display_name: 'Ubuntu Base',
      enabled: true,
      spec_text: '{"domain":{"cpu":{"cores":4}}}',
    });

    const { result } = renderHook(() => useAdminTemplatesController({ t }));

    await act(async () => {
      await result.current.submitCreate();
    });

    expect(createMutate).toHaveBeenCalledWith({
      name: 'ubuntu-base',
      display_name: 'Ubuntu Base',
      enabled: true,
      spec: { domain: { cpu: { cores: 4 } } },
    });
  });

  it('rejects invalid create spec JSON and does not mutate', async () => {
    const createMutate = vi.fn();

    useApiMutationMock
      .mockReturnValueOnce({ mutate: createMutate, isPending: false })
      .mockReturnValueOnce({ mutate: vi.fn(), isPending: false });
    useApiActionMock.mockReturnValue({ mutate: vi.fn(), isPending: false });

    createFormState.validateFields.mockResolvedValue({
      name: 'ubuntu-base',
      spec_text: '[]',
    });

    const { result } = renderHook(() => useAdminTemplatesController({ t }));

    await act(async () => {
      await result.current.submitCreate();
    });

    expect(createMutate).not.toHaveBeenCalled();
    expect(messageErrorMock).toHaveBeenCalledWith('templates.spec_invalid');
  });
});
