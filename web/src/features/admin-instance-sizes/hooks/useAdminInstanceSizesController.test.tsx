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

import { useAdminInstanceSizesController } from './useAdminInstanceSizesController';

describe('useAdminInstanceSizesController', () => {
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
      data: { items: [] },
      isLoading: false,
      refetch: vi.fn(),
    });
  });

  /**
   * Since ADR-0023 Stage 1, spec fields are driven by DynamicSchemaForm.
   * The controller receives `spec_text` (JSON string from the form) and
   * parses it into `spec_overrides` for the API payload.
   *
   * spec_text is a raw JSON string produced by DynamicSchemaForm.
   * Valid object JSON → parsed into spec_overrides.
   * Invalid / non-object JSON → silently ignored (spec_overrides: undefined).
   *   This is intentional: DynamicSchemaForm already validates individual fields;
   *   the controller does not show a toast for malformed spec_text.
   */
  it('submits create payload with spec_overrides parsed from spec_text', async () => {
    const createMutate = vi.fn();
    const updateMutate = vi.fn();
    const deleteMutate = vi.fn();

    useApiMutationMock
      .mockReturnValueOnce({ mutate: createMutate, isPending: false })
      .mockReturnValueOnce({ mutate: updateMutate, isPending: false });
    useApiActionMock.mockReturnValue({ mutate: deleteMutate, isPending: false });

    // spec_text replaces spec_overrides_text; content is KubeVirt VirtualMachineSpec JSON.
    createFormState.validateFields.mockResolvedValue({
      name: 'm4.large',
      cpu_cores: 4,
      memory_gi: 8,
      enabled: true,
      spec_text: '{"spec":{"template":{"spec":{"domain":{"resources":{"limits":{"memory":"8Gi"}}}}}}}',
    });

    const { result } = renderHook(() => useAdminInstanceSizesController({ t }));

    await act(async () => {
      await result.current.submitCreate();
    });

    expect(createMutate).toHaveBeenCalledTimes(1);
    expect(createMutate).toHaveBeenCalledWith(expect.objectContaining({
      name: 'm4.large',
      cpu_cores: 4,
      memory_gi: 8,
      enabled: true,
      // requires_gpu / requires_hugepages removed from formToPayload (ADR-0023 Stage 1).
      // Those fields are now part of spec_overrides (KubeVirt spec), not top-level metadata.
      spec_overrides: {
        spec: {
          template: {
            spec: {
              domain: {
                resources: {
                  limits: {
                    memory: '8Gi',
                  },
                },
              },
            },
          },
        },
      },
    }));
  });

  /**
   * When spec_text is non-object JSON (array, null, primitive), spec_overrides is
   * silently set to undefined — no error toast is shown.  DynamicSchemaForm handles
   * field-level validation; the controller trusts its output.
   *
   * Contrast with old behaviour: spec_overrides_text triggered a user-facing
   * error toast for invalid JSON.  That UX belonged to the raw textarea escape-hatch
   * which has since been removed in favour of schema-driven rendering.
   */
  it('ignores non-object spec_text and still calls mutate with spec_overrides undefined', async () => {
    const createMutate = vi.fn();

    useApiMutationMock
      .mockReturnValueOnce({ mutate: createMutate, isPending: false })
      .mockReturnValueOnce({ mutate: vi.fn(), isPending: false });
    useApiActionMock.mockReturnValue({ mutate: vi.fn(), isPending: false });

    createFormState.validateFields.mockResolvedValue({
      name: 'm4.large',
      cpu_cores: 4,
      memory_gi: 8,
      // Array JSON is non-object — controller ignores it silently.
      spec_text: '[]',
    });

    const { result } = renderHook(() => useAdminInstanceSizesController({ t }));

    await act(async () => {
      await result.current.submitCreate();
    });

    // mutate IS called — invalid spec_text does not block submission.
    expect(createMutate).toHaveBeenCalledTimes(1);
    const payload = createMutate.mock.calls[0]?.[0] as Record<string, unknown>;
    expect(payload).toMatchObject({
      name: 'm4.large',
      cpu_cores: 4,
      memory_gi: 8,
    });
    expect(payload).not.toHaveProperty('spec_overrides');
    // No error toast — DynamicSchemaForm owns field validation, not the controller.
    expect(messageErrorMock).not.toHaveBeenCalled();
  });

  it('submits update payload with cpu_request=0 and memory_request_gi=0 when overcommit is disabled', async () => {
    const createMutate = vi.fn();
    const updateMutate = vi.fn();

    let mutationCall = 0;
    useApiMutationMock.mockImplementation(() => {
      mutationCall += 1;
      if (mutationCall % 2 === 1) {
        return { mutate: createMutate, isPending: false };
      }
      return { mutate: updateMutate, isPending: false };
    });
    useApiActionMock.mockReturnValue({ mutate: vi.fn(), isPending: false });

    const { result } = renderHook(() => useAdminInstanceSizesController({ t }));

    act(() => {
      result.current.openEditModal({
        id: 'size-1',
        name: 'm4.large',
        cpu_cores: 4,
        memory_gi: 8,
        disk_gb: 80,
        dedicated_cpu: false,
        enabled: true,
      });
    });

    editFormState.validateFields.mockResolvedValue({
      name: 'm4.large',
      cpu_cores: 4,
      memory_gi: 8,
      cpu_overcommit_enabled: false,
      memory_overcommit_enabled: false,
      spec_text: '{}',
      enabled: true,
    });

    await act(async () => {
      await result.current.submitEdit();
    });

    expect(updateMutate).toHaveBeenCalledTimes(1);
    expect(updateMutate).toHaveBeenCalledWith(expect.objectContaining({
      id: 'size-1',
      body: expect.objectContaining({
        cpu_request: 0,
        memory_request_gi: 0,
      }),
    }));
  });

  it('hydrates edit form fields after opening edit modal', async () => {
    useApiMutationMock
      .mockReturnValueOnce({ mutate: vi.fn(), isPending: false })
      .mockReturnValueOnce({ mutate: vi.fn(), isPending: false });
    useApiActionMock.mockReturnValue({ mutate: vi.fn(), isPending: false });

    const { result } = renderHook(() => useAdminInstanceSizesController({ t }));

    act(() => {
      result.current.openEditModal({
        id: 'size-2',
        name: 'm8.large',
        display_name: 'M8 Large',
        description: 'test size',
        cpu_cores: 8,
        memory_gi: 16,
        disk_gb: 100,
        dedicated_cpu: true,
        requires_sriov: true,
        enabled: true,
        spec_overrides: {},
      });
    });

    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 0));
    });

    expect(editFormState.resetFields).toHaveBeenCalled();
    expect(editFormState.setFieldsValue).toHaveBeenCalledWith(expect.objectContaining({
      name: 'm8.large',
      display_name: 'M8 Large',
      spec_text: '{}',
    }));
  });
});
