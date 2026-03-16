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
      source_type: 'cdi_image_import',
      image_url: 'docker.io/kubevirt/ubuntu:22.04',
      cloud_init: yamlCloudInit,
    });

    const { result } = renderHook(() => useAdminTemplatesController({ t }));

    await act(async () => {
      await result.current.submitCreate();
    });

    // cloud_init is passed verbatim — it is YAML text, not parsed JSON.
    // non-clone source types clear PVC fields.
    expect(createMutate).toHaveBeenCalledWith({
      name: 'ubuntu-base',
      display_name: 'Ubuntu Base',
      enabled: true,
      source_type: 'cdi_image_import',
      image_url: 'docker.io/kubevirt/ubuntu:22.04',
      cloud_init: yamlCloudInit,
      pvc_name: undefined,
      pvc_namespace: undefined,
    });
  });

  it('clears image_url when source_type is cdi_pvc_clone', async () => {
    const createMutate = vi.fn();

    useApiMutationMock
      .mockReturnValueOnce({ mutate: createMutate, isPending: false })
      .mockReturnValueOnce({ mutate: vi.fn(), isPending: false });
    useApiActionMock.mockReturnValue({ mutate: vi.fn(), isPending: false });

    createFormState.validateFields.mockResolvedValue({
      name: 'centos7-pvc',
      enabled: true,
      source_type: 'cdi_pvc_clone',
      pvc_name: 'centos7-base-disk',
      pvc_namespace: 'default',
      image_url: 'docker.io/stale/image',  // stale value that should be cleared
    });

    const { result } = renderHook(() => useAdminTemplatesController({ t }));

    await act(async () => {
      await result.current.submitCreate();
    });

    // clone source types clear image_url and preserve source PVC coordinates.
    expect(createMutate).toHaveBeenCalledWith(
      expect.objectContaining({ source_type: 'cdi_pvc_clone', pvc_name: 'centos7-base-disk', pvc_namespace: 'default', image_url: undefined }),
    );
  });

  it('keeps experimental sources hidden by default in create flow until explicitly enabled', () => {
    let mutationCall = 0;
    useApiMutationMock.mockImplementation(() => {
      mutationCall += 1;
      return { mutate: vi.fn(), isPending: false, key: mutationCall };
    });
    useApiActionMock.mockReturnValue({ mutate: vi.fn(), isPending: false });

    const { result } = renderHook(() => useAdminTemplatesController({ t }));

    expect(result.current.createExperimentalSourcesEnabled).toBe(false);

    act(() => {
      result.current.openCreateModal();
    });

    expect(result.current.createExperimentalSourcesEnabled).toBe(false);

    act(() => {
      result.current.enableCreateExperimentalSources();
    });

    expect(result.current.createExperimentalSourcesEnabled).toBe(true);
  });

  it('auto-enables experimental sources when editing an existing containerdisk template', () => {
    let mutationCall = 0;
    useApiMutationMock.mockImplementation(() => {
      mutationCall += 1;
      return { mutate: vi.fn(), isPending: false, key: mutationCall };
    });
    useApiActionMock.mockReturnValue({ mutate: vi.fn(), isPending: false });

    const { result } = renderHook(() => useAdminTemplatesController({ t }));

    act(() => {
      result.current.openEditModal({
        id: 'tpl-1',
        name: 'fedora-ephemeral',
        source_type: 'containerdisk',
        image_url: 'docker://quay.io/containerdisks/fedora:40',
        catalog_scope: 'test',
        enabled: true,
      } as never);
    });

    expect(result.current.editExperimentalSourcesEnabled).toBe(true);
  });

  it('hydrates edit form after the modal opens for image-import templates', () => {
    let mutationCall = 0;
    useApiMutationMock.mockImplementation(() => {
      mutationCall += 1;
      return { mutate: vi.fn(), isPending: false, key: mutationCall };
    });
    useApiActionMock.mockReturnValue({ mutate: vi.fn(), isPending: false });

    const { result } = renderHook(() => useAdminTemplatesController({ t }));

    act(() => {
      result.current.openEditModal({
        id: 'tpl-1',
        name: 'ubuntu-base',
        display_name: 'Ubuntu Base',
        description: 'Canonical live template',
        catalog_scope: 'all',
        os_family: 'linux',
        os_version: '22.04',
        enabled: true,
        source_type: 'cdi_image_import',
        image_url: 'docker://quay.io/containerdisks/ubuntu:22.04',
        pvc_name: '',
        pvc_namespace: '',
        cloud_init: '#cloud-config\nusers:\n  - default',
      } as never);
    });

    expect(editFormState.resetFields).toHaveBeenCalled();
    expect(editFormState.setFieldsValue).toHaveBeenCalledTimes(1);
    expect(editFormState.setFieldsValue).toHaveBeenCalledWith({
      display_name: 'Ubuntu Base',
      description: 'Canonical live template',
      catalog_scope: 'all',
      os_family: 'linux',
      os_version: '22.04',
      enabled: true,
      source_type: 'cdi_image_import',
      image_url: 'docker://quay.io/containerdisks/ubuntu:22.04',
      pvc_name: undefined,
      pvc_namespace: undefined,
      cloud_init: '#cloud-config\nusers:\n  - default',
    });
  });

  it('hydrates edit form after the modal opens for pvc-clone templates', () => {
    let mutationCall = 0;
    useApiMutationMock.mockImplementation(() => {
      mutationCall += 1;
      return { mutate: vi.fn(), isPending: false, key: mutationCall };
    });
    useApiActionMock.mockReturnValue({ mutate: vi.fn(), isPending: false });

    const { result } = renderHook(() => useAdminTemplatesController({ t }));

    act(() => {
      result.current.openEditModal({
        id: 'tpl-2',
        name: 'ubuntu-clone',
        display_name: 'Ubuntu Clone',
        description: 'PVC clone template',
        catalog_scope: 'all',
        os_family: 'linux',
        os_version: '22.04',
        enabled: true,
        source_type: 'cdi_pvc_clone',
        pvc_name: 'ubuntu-golden',
        pvc_namespace: 'golden-images',
        cloud_init: '#cloud-config\nusers:\n  - default',
      } as never);
    });

    expect(editFormState.resetFields).toHaveBeenCalled();
    expect(editFormState.setFieldsValue).toHaveBeenCalledTimes(1);
    expect(editFormState.setFieldsValue).toHaveBeenCalledWith({
      display_name: 'Ubuntu Clone',
      description: 'PVC clone template',
      catalog_scope: 'all',
      os_family: 'linux',
      os_version: '22.04',
      enabled: true,
      source_type: 'cdi_pvc_clone',
      image_url: undefined,
      pvc_name: 'ubuntu-golden',
      pvc_namespace: 'golden-images',
      cloud_init: '#cloud-config\nusers:\n  - default',
    });
  });
});
