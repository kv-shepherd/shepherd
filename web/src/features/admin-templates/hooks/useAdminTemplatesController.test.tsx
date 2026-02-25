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

  /**
   * master-flow Step 3: Template create submits cloud_init as-is (YAML text, NOT JSON).
   * cloud_init is a first-class Template field edited directly by the admin.
   * No JSON parsing, no spec_text intermediary.
   */
  it('submits create payload with cloud_init YAML directly', async () => {
    const createMutate = vi.fn();
    const updateMutate = vi.fn();
    const deleteMutate = vi.fn();

    useApiMutationMock
      .mockReturnValueOnce({ mutate: createMutate, isPending: false })
      .mockReturnValueOnce({ mutate: updateMutate, isPending: false });
    useApiActionMock.mockReturnValue({ mutate: deleteMutate, isPending: false });

    const yamlCloudInit = '#cloud-config\nusers:\n  - name: admin\n    sudo: ALL=(ALL) NOPASSWD:ALL';
    createFormState.validateFields.mockResolvedValue({
      name: 'ubuntu-base',
      display_name: 'Ubuntu Base',
      enabled: true,
      source_type: 'image',
      image_url: 'docker.io/kubevirt/ubuntu:22.04',
      cloud_init: yamlCloudInit,
    });

    const { result } = renderHook(() => useAdminTemplatesController({ t }));

    await act(async () => {
      await result.current.submitCreate();
    });

    // cloud_init is passed verbatim — it is YAML text, not parsed JSON.
    // source_type='image' → pvc_name and pvc_namespace are cleared (undefined).
    expect(createMutate).toHaveBeenCalledWith({
      name: 'ubuntu-base',
      display_name: 'Ubuntu Base',
      enabled: true,
      source_type: 'image',
      image_url: 'docker.io/kubevirt/ubuntu:22.04',
      cloud_init: yamlCloudInit,
      pvc_name: undefined,
      pvc_namespace: undefined,
    });
  });

  it('clears image_url when source_type is pvc', async () => {
    const createMutate = vi.fn();

    useApiMutationMock
      .mockReturnValueOnce({ mutate: createMutate, isPending: false })
      .mockReturnValueOnce({ mutate: vi.fn(), isPending: false });
    useApiActionMock.mockReturnValue({ mutate: vi.fn(), isPending: false });

    createFormState.validateFields.mockResolvedValue({
      name: 'centos7-pvc',
      enabled: true,
      source_type: 'pvc',
      pvc_name: 'centos7-base-disk',
      pvc_namespace: 'default',
      image_url: 'docker.io/stale/image',  // stale value that should be cleared
    });

    const { result } = renderHook(() => useAdminTemplatesController({ t }));

    await act(async () => {
      await result.current.submitCreate();
    });

    // source_type='pvc' → image_url is cleared (undefined); pvc_namespace is preserved.
    expect(createMutate).toHaveBeenCalledWith(
      expect.objectContaining({ source_type: 'pvc', pvc_name: 'centos7-base-disk', pvc_namespace: 'default', image_url: undefined }),
    );
  });
});
