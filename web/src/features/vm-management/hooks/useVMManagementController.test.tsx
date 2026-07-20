import { act, renderHook } from "@testing-library/react";
import type { TFunction } from "i18next";
import { beforeEach, describe, expect, it, vi } from "vitest";

const {
  useApiGetMock,
  useApiMutationMock,
  useApiActionMock,
  postStartMutate,
  postStopMutate,
  postRestartMutate,
  createMutate,
  createModifyMutate,
  createBatchMutate,
  vmBatchMutate,
  vmBatchPowerMutate,
  retryBatchMutate,
  cancelBatchMutate,
  deleteMutate,
  apiGetMock,
  formState,
  useWatchMock,
  watchValues,
  vmItems,
  batchStatusState,
  authUserState,
  messageSuccessMock,
  messageErrorMock,
  messageWarningMock,
  messageInfoMock,
  batchStatusRefetchMock,
} = vi.hoisted(() => {
  const watchValues: Record<string, unknown> = {
    template_id: "tpl-1",
    instance_size_id: "size-1",
    namespace: "prod",
    reason: "scale up",
    service_id: "svc-1",
    batch_count: 1,
  };
  const vmItems: Array<{ id: string; name: string; status: string }> = [];
  const batchStatusState = { status: "PARTIAL_SUCCESS" };
  const authUserState = {
    user: {
      id: "u-alice",
      username: "alice",
      permissions: ["vm:create", "vm:delete", "vm:operate", "platform:admin"],
    },
  };

  return {
    useApiGetMock: vi.fn(),
    useApiMutationMock: vi.fn(),
    useApiActionMock: vi.fn(),
    postStartMutate: vi.fn(),
    postStopMutate: vi.fn(),
    postRestartMutate: vi.fn(),
    createMutate: vi.fn(),
    createModifyMutate: vi.fn(),
    createBatchMutate: vi.fn(),
    vmBatchMutate: vi.fn(),
    vmBatchPowerMutate: vi.fn(),
    retryBatchMutate: vi.fn(),
    cancelBatchMutate: vi.fn(),
    deleteMutate: vi.fn(),
    apiGetMock: vi.fn(),
    formState: {
      validateFields: vi.fn(),
      resetFields: vi.fn(),
      setFieldValue: vi.fn(),
      setFieldsValue: vi.fn(),
      getFieldsValue: vi.fn(),
    },
    useWatchMock: vi.fn(
      (field: string, options?: { form?: unknown; preserve?: boolean }) => {
        void options;
        return watchValues[field];
      },
    ),
    watchValues,
    vmItems,
    batchStatusState,
    authUserState,
    messageSuccessMock: vi.fn(),
    messageErrorMock: vi.fn(),
    messageWarningMock: vi.fn(),
    messageInfoMock: vi.fn(),
    batchStatusRefetchMock: vi.fn(),
  };
});

vi.mock("antd", () => ({
  App: {
    useApp: () => ({
      message: {
        success: messageSuccessMock,
        error: messageErrorMock,
        warning: messageWarningMock,
        info: messageInfoMock,
      },
    }),
  },
  Form: {
    useForm: vi.fn(() => [formState]),
    useWatch: (
      field: string,
      options?: { form?: unknown; preserve?: boolean },
    ) => useWatchMock(field, options),
  },
}));

vi.mock("@/hooks/useApiQuery", () => ({
  useApiGet: (...args: unknown[]) => useApiGetMock(...args),
  useApiMutation: (...args: unknown[]) => useApiMutationMock(...args),
  useApiAction: (...args: unknown[]) => useApiActionMock(...args),
}));

vi.mock("@/lib/api/client", () => ({
  api: {
    GET: (...args: unknown[]) => apiGetMock(...args),
  },
}));

vi.mock("@/stores/auth", () => ({
  useAuthStore: (
    selector: (state: {
      user: { id: string; username: string; permissions: string[] };
    }) => unknown,
  ) => selector(authUserState),
}));

import { useVMManagementController } from "./useVMManagementController";
import { buildVMRequestDraftStorageKey } from "../draftStorage";

describe("useVMManagementController", () => {
  const t = ((key: string) => key) as unknown as TFunction;

  const getMutationOptions = (path: string, occurrence = 0) =>
    useApiMutationMock.mock.calls.filter((call) =>
      String(call[0]).includes(`"${path}"`),
    )[occurrence]?.[1] as
      | {
          onSuccess?: (
            data: Record<string, unknown>,
            variables?: Record<string, unknown>,
          ) => void;
          onError?: (
            error: {
              code?: string;
              status?: number;
              retry_after_seconds?: number;
              params?: Record<string, unknown>;
            },
            variables?: Record<string, unknown>,
          ) => void;
        }
      | undefined;

  const getActionOptions = (path: string) =>
    useApiActionMock.mock.calls.find((call) =>
      String(call[0]).includes(`"${path}"`),
    )?.[1] as
      | {
          onError?: (error: {
            code: string;
            status?: number;
            params?: Record<string, unknown>;
          }) => void;
        }
      | undefined;

  beforeEach(() => {
    vi.clearAllMocks();
    window.localStorage.clear();
    window.sessionStorage.clear();
    useWatchMock.mockClear();
    watchValues.template_id = "tpl-1";
    watchValues.instance_size_id = "size-1";
    watchValues.namespace = "prod";
    watchValues.reason = "scale up";
    watchValues.service_id = "svc-1";
    watchValues.batch_count = 1;
    vmItems.length = 0;
    batchStatusState.status = "PARTIAL_SUCCESS";
    authUserState.user.id = "u-alice";
    authUserState.user.username = "alice";
    authUserState.user.permissions = [
      "vm:create",
      "vm:delete",
      "vm:operate",
      "platform:admin",
    ];
    formState.getFieldsValue.mockReturnValue({
      service_id: "svc-1",
      template_id: "tpl-1",
      instance_size_id: "size-1",
      namespace: "prod",
      reason: "scale up",
    });
    formState.validateFields.mockResolvedValue(undefined);

    useApiGetMock.mockImplementation((queryKey: unknown) => {
      if (!Array.isArray(queryKey)) {
        return { data: undefined, isLoading: false };
      }

      if (queryKey[0] === "vms" && queryKey[1] === 1 && queryKey[2] === 20) {
        return { data: { items: vmItems }, isLoading: false, refetch: vi.fn() };
      }

      if (queryKey[0] === "vms" && queryKey[1] === "filter-options") {
        return { data: { items: [] }, isLoading: false };
      }

      if (queryKey[0] === "systems" && queryKey[1] === "vm-wizard") {
        return {
          data: { items: [{ id: "sys-1", name: "System A" }] },
          isLoading: false,
        };
      }

      if (queryKey[0] === "services" && queryKey[2] === "vm-wizard") {
        return {
          data: { items: [{ id: "svc-1", name: "Service A" }] },
          isLoading: false,
        };
      }

      if (
        queryKey[0] === "vm-request-context" &&
        queryKey[1] === "placement-hint"
      ) {
        return {
          data: {
            placement_hint: {
              status: "AVAILABLE",
              compatible_cluster_count: 1,
              evaluated_cluster_count: 2,
            },
          },
          isLoading: false,
        };
      }

      if (queryKey[0] === "vm-request-context") {
        return {
          data: {
            namespaces: ["prod"],
            templates: [{ id: "tpl-1", name: "Ubuntu Template" }],
            instance_sizes: [
              { id: "size-1", name: "small", cpu_cores: 2, memory_gi: 4 },
            ],
          },
          isLoading: false,
        };
      }

      if (queryKey[0] === "templates" && queryKey[1] === "vm-wizard-fallback") {
        return { data: { items: [] }, isLoading: false };
      }

      if (
        queryKey[0] === "instance-sizes" &&
        queryKey[1] === "vm-wizard-fallback"
      ) {
        return { data: { items: [] }, isLoading: false };
      }

      if (queryKey[0] === "vm-batch") {
        return {
          data: {
            batch_id: "batch-live-1",
            operation: "CREATE",
            status: batchStatusState.status,
            child_count: 3,
            success_count: 1,
            failed_count: 1,
            pending_count: 1,
            created_by: "owner-1",
            created_at: "2026-02-15T00:00:00Z",
            updated_at: "2026-02-15T00:00:00Z",
            children: [
              {
                ticket_id: "ticket-failed-1",
                event_id: "ev-1",
                status: "FAILED",
                resource_name: "vm-a",
              },
              {
                ticket_id: "ticket-pending-1",
                event_id: "ev-2",
                status: "PENDING",
                resource_name: "vm-b",
                attempt_count: 0,
              },
              {
                ticket_id: "ticket-success-1",
                event_id: "ev-3",
                status: "SUCCESS",
                resource_name: "vm-c",
                attempt_count: 1,
              },
            ],
          },
          isLoading: false,
          refetch: batchStatusRefetchMock,
        };
      }

      if (queryKey[0] === "vm-modify-context") {
        return {
          data: {
            vm_id: "vm-1",
            vm_name: "vm-one",
            namespace: "prod",
            current_cpu_cores: 2,
            current_memory_gi: 4,
            current_disk_gb: 20,
            cpu_supported: true,
            memory_supported: true,
            disk_supported: true,
          },
          isLoading: false,
        };
      }

      return { data: undefined, isLoading: false };
    });

    useApiMutationMock.mockImplementation(
      (mutationFn: unknown, options: unknown) => {
        const source = String(mutationFn);
        if (source.includes('"/vms/request"')) {
          return { mutate: createMutate, isPending: false };
        }
        if (source.includes('"/vms/{vm_id}/modify-request"')) {
          return { mutate: createModifyMutate, isPending: false };
        }
        if (source.includes('"/vms/batch/power"')) {
          return { mutate: vmBatchPowerMutate, isPending: false };
        }
        if (source.includes('"/vms/batch/{batch_id}/retry"')) {
          return { mutate: retryBatchMutate, isPending: false };
        }
        if (source.includes('"/vms/batch/{batch_id}/cancel"')) {
          return { mutate: cancelBatchMutate, isPending: false };
        }
        if (source.includes('api.DELETE("/vms/{vm_id}"')) {
          return { mutate: deleteMutate, isPending: false };
        }
        if (source.includes('"/vms/batch"')) {
          const isWizardBatch = String(
            (options as { onSuccess?: unknown } | undefined)?.onSuccess,
          ).includes("clearSavedDraft");
          return {
            mutate: isWizardBatch ? createBatchMutate : vmBatchMutate,
            isPending: false,
          };
        }
        throw new Error(`unexpected mutation in controller test: ${source}`);
      },
    );

    let actionCall = 0;
    useApiActionMock.mockImplementation(() => {
      actionCall += 1;
      if (actionCall % 3 === 1)
        return { mutate: postStartMutate, isPending: false };
      if (actionCall % 3 === 2)
        return { mutate: postStopMutate, isPending: false };
      return { mutate: postRestartMutate, isPending: false };
    });

    apiGetMock.mockImplementation((path: string) => {
      if (path === "/vms/{vm_id}/modify-context") {
        return Promise.resolve({
          data: {
            vm_id: "vm-1",
            vm_name: "vm-one",
            namespace: "prod",
            current_cpu_cores: 2,
            current_memory_gi: 4,
            current_disk_gb: 20,
            cpu_supported: true,
            memory_supported: true,
            disk_supported: true,
          },
          error: undefined,
          response: new Response(),
        });
      }
      if (path === "/vms/{vm_id}/request-prefill") {
        return Promise.resolve({
          data: {
            system_id: "sys-1",
            service_id: "svc-prefill",
            template_id: "tpl-prefill",
            instance_size_id: "size-prefill",
            namespace: "prefill-ns",
            reason: "reuse vm request",
            batch_count: 2,
          },
          error: undefined,
          response: new Response(),
        });
      }
      return Promise.resolve({
        data: undefined,
        error: undefined,
        response: new Response(),
      });
    });
  });

  it("advances wizard steps after validating required fields and submits request payload", async () => {
    const { result } = renderHook(() => useVMManagementController({ t }));

    act(() => {
      result.current.openWizard();
      result.current.onSystemChange("sys-1");
    });
    expect(formState.resetFields).toHaveBeenCalledWith(["service_id"]);

    await act(async () => {
      await result.current.goToNextWizardStep();
    });
    expect(formState.validateFields).toHaveBeenCalledWith(["service_id"]);

    await act(async () => {
      result.current.submitWizard();
    });
    expect(createMutate).toHaveBeenCalledWith({
      service_id: "svc-1",
      template_id: "tpl-1",
      instance_size_id: "size-1",
      namespace: "prod",
      reason: "scale up",
    });
  });

  it("submits create payload with requested target resources when they differ from the selected size", async () => {
    formState.getFieldsValue.mockReturnValue({
      service_id: "svc-1",
      template_id: "tpl-1",
      instance_size_id: "size-1",
      namespace: "prod",
      reason: "scale up",
      target_cpu_cores: 3,
      target_memory_gi: 6,
      target_disk_gb: 80,
    });

    const { result } = renderHook(() => useVMManagementController({ t }));

    await act(async () => {
      await result.current.submitWizard();
    });

    expect(createMutate).toHaveBeenCalledWith({
      service_id: "svc-1",
      template_id: "tpl-1",
      instance_size_id: "size-1",
      namespace: "prod",
      reason: "scale up",
      target_cpu_cores: 3,
      target_memory_gi: 6,
      target_disk_gb: 80,
    });
  });

  it("prefills system and service when wizard is opened from a scoped entry point", () => {
    const { result } = renderHook(() => useVMManagementController({ t }));

    act(() => {
      result.current.openWizard({ systemId: "sys-1", serviceId: "svc-2" });
    });

    expect(formState.resetFields).toHaveBeenCalled();
    expect(formState.setFieldsValue).toHaveBeenCalledWith({
      batch_count: 1,
      service_id: "svc-2",
      template_id: undefined,
      instance_size_id: undefined,
      namespace: undefined,
      reason: undefined,
      target_cpu_cores: undefined,
      target_memory_gi: undefined,
      target_disk_gb: undefined,
    });
  });

  it("opens the request modal in full-form mode with prefilled request values", () => {
    const { result } = renderHook(() => useVMManagementController({ t }));

    act(() => {
      result.current.openWizard({
        requestMode: "full",
        systemId: "sys-1",
        serviceId: "svc-2",
        templateId: "tpl-2",
        instanceSizeId: "size-2",
        namespace: "team-prod",
        reason: "reuse request",
        batchCount: 3,
      });
    });

    expect(result.current.requestMode).toBe("full");
    expect(formState.setFieldsValue).toHaveBeenCalledWith({
      batch_count: 3,
      service_id: "svc-2",
      template_id: "tpl-2",
      instance_size_id: "size-2",
      namespace: "team-prod",
      reason: "reuse request",
      target_cpu_cores: undefined,
      target_memory_gi: undefined,
      target_disk_gb: undefined,
    });
  });

  it("opens a full-form request using reusable parameters from an existing vm", async () => {
    const { result } = renderHook(() => useVMManagementController({ t }));

    await act(async () => {
      await result.current.openSimilarRequest("vm-1");
    });

    expect(apiGetMock).toHaveBeenCalledWith("/vms/{vm_id}/request-prefill", {
      params: { path: { vm_id: "vm-1" } },
    });
    expect(result.current.requestMode).toBe("full");
    expect(formState.resetFields).toHaveBeenCalled();
    expect(formState.setFieldsValue).toHaveBeenCalledWith({
      batch_count: 2,
      service_id: "svc-prefill",
      template_id: "tpl-prefill",
      instance_size_id: "size-prefill",
      namespace: "prefill-ns",
      reason: "reuse vm request",
      target_cpu_cores: undefined,
      target_memory_gi: undefined,
      target_disk_gb: undefined,
    });
  });

  it("warns when a vm does not expose reusable request context", async () => {
    apiGetMock.mockResolvedValueOnce({
      data: undefined,
      error: {
        code: "VM_REQUEST_PREFILL_UNAVAILABLE",
        message: "no reusable request context",
      },
      response: new Response(),
    });
    const { result } = renderHook(() => useVMManagementController({ t }));

    await act(async () => {
      await result.current.openSimilarRequest("vm-404");
    });

    expect(messageWarningMock).toHaveBeenCalledWith(
      "request_similar.unavailable",
    );
  });

  it("autosaves a meaningful vm request draft while the wizard is open", async () => {
    vi.useFakeTimers();
    watchValues.service_id = "svc-2";
    const { result } = renderHook(() => useVMManagementController({ t }));

    act(() => {
      result.current.openWizard({ systemId: "sys-1", serviceId: "svc-2" });
    });

    await act(async () => {
      await Promise.resolve();
      vi.advanceTimersByTime(500);
      await Promise.resolve();
    });

    expect(
      window.localStorage.getItem(buildVMRequestDraftStorageKey("u-alice")),
    ).toContain('"systemId":"sys-1"');
    expect(
      window.localStorage.getItem(buildVMRequestDraftStorageKey("u-alice")),
    ).toContain('"serviceId":"svc-2"');

    watchValues.service_id = "svc-1";
    vi.useRealTimers();
  });

  it("restores a saved draft back into the wizard form", () => {
    window.localStorage.setItem(
      buildVMRequestDraftStorageKey("u-alice"),
      JSON.stringify({
        version: 1,
        systemId: "sys-1",
        serviceId: "svc-restore",
        templateId: "tpl-restore",
        instanceSizeId: "size-restore",
        namespace: "restore-ns",
        reason: "restore reason",
        batchCount: 3,
        wizardStep: 2,
        requestMode: "full",
        updatedAt: "2026-03-16T12:00:00Z",
      }),
    );
    const { result } = renderHook(() => useVMManagementController({ t }));

    act(() => {
      result.current.resumeDraft();
    });

    expect(result.current.requestMode).toBe("full");
    expect(formState.resetFields).toHaveBeenCalled();
    expect(formState.setFieldsValue).toHaveBeenCalledWith({
      service_id: "svc-restore",
      template_id: "tpl-restore",
      instance_size_id: "size-restore",
      namespace: "restore-ns",
      reason: "restore reason",
      target_cpu_cores: undefined,
      target_memory_gi: undefined,
      target_disk_gb: undefined,
      batch_count: 3,
    });
  });

  it("validates all required fields before submitting in full-form mode", async () => {
    const { result } = renderHook(() => useVMManagementController({ t }));

    act(() => {
      result.current.openWizard({ requestMode: "full" });
    });

    await act(async () => {
      await result.current.submitWizard();
    });

    expect(formState.validateFields).toHaveBeenCalledWith([
      "service_id",
      "template_id",
      "instance_size_id",
      "namespace",
      "reason",
      "batch_count",
    ]);
    expect(createMutate).toHaveBeenCalledWith({
      service_id: "svc-1",
      template_id: "tpl-1",
      instance_size_id: "size-1",
      namespace: "prod",
      reason: "scale up",
    });
  });

  it("discards a saved draft from local storage", () => {
    window.localStorage.setItem(
      buildVMRequestDraftStorageKey("u-alice"),
      JSON.stringify({
        version: 1,
        serviceId: "svc-restore",
        reason: "restore reason",
        batchCount: 1,
        wizardStep: 1,
        updatedAt: "2026-03-16T12:00:00Z",
      }),
    );
    const { result } = renderHook(() => useVMManagementController({ t }));

    act(() => {
      result.current.discardDraft();
    });

    expect(
      window.localStorage.getItem(buildVMRequestDraftStorageKey("u-alice")),
    ).toBeNull();
  });

  it("requests placement hint when namespace, template, and size are selected", async () => {
    const { result } = renderHook(() => useVMManagementController({ t }));

    act(() => {
      result.current.openWizard({ requestMode: "full" });
    });

    expect(result.current.placementHint?.status).toBe("AVAILABLE");

    const hintCall = [...useApiGetMock.mock.calls]
      .reverse()
      .find(
        (call) =>
          Array.isArray(call[0]) &&
          call[0][0] === "vm-request-context" &&
          call[0][1] === "placement-hint",
      );
    const hintQueryFn = hintCall?.[1];
    expect(hintQueryFn).toBeTypeOf("function");

    await act(async () => {
      await hintQueryFn();
    });

    expect(apiGetMock).toHaveBeenCalledWith("/vms/request-context", {
      params: {
        query: {
          namespace: "prod",
          template_id: "tpl-1",
          instance_size_id: "size-1",
        },
      },
    });
  });

  it("dispatches vm power and delete actions with vm identity in test env", async () => {
    const { result } = renderHook(() => useVMManagementController({ t }));

    await act(async () => {
      result.current.startVM("vm-1");
      result.current.stopVM("vm-1");
      result.current.restartVM("vm-1");
      result.current.deleteVM("vm-2", "vm-two", "test");
    });

    expect(postStartMutate).toHaveBeenCalledWith("vm-1");
    expect(postStopMutate).toHaveBeenCalledWith("vm-1");
    expect(postRestartMutate).toHaveBeenCalledWith("vm-1");
    expect(result.current.deleteOpen).toBe(true);
    expect(result.current.deletingVM).toEqual({
      id: "vm-2",
      name: "vm-two",
      environment: "test",
    });
    expect(result.current.deleteConfirmName).toBe("");
    expect(deleteMutate).not.toHaveBeenCalled();

    act(() => {
      result.current.submitDelete();
    });

    expect(deleteMutate).toHaveBeenCalledWith({
      vmId: "vm-2",
      vmName: "vm-two",
    });
  });

  it("requires confirm name before deleting in non-test env", () => {
    const { result } = renderHook(() => useVMManagementController({ t }));

    act(() => {
      result.current.deleteVM("vm-2", "vm-two", "prod");
    });

    expect(result.current.deleteOpen).toBe(true);
    expect(result.current.deletingVM).toEqual({
      id: "vm-2",
      name: "vm-two",
      environment: "prod",
    });
    expect(deleteMutate).not.toHaveBeenCalled();

    act(() => {
      result.current.submitDelete();
    });

    expect(messageWarningMock).toHaveBeenCalledWith(
      "action.delete_type_name_hint",
    );
    expect(deleteMutate).not.toHaveBeenCalled();

    act(() => {
      result.current.setDeleteConfirmName("vm-two");
    });
    act(() => {
      result.current.submitDelete();
    });

    expect(deleteMutate).toHaveBeenCalledWith({
      vmId: "vm-2",
      vmName: "vm-two",
    });
  });

  it("submits create as batch when batch_count > 1", async () => {
    watchValues.batch_count = 3;
    formState.getFieldsValue.mockReturnValue({
      service_id: "svc-1",
      template_id: "tpl-1",
      instance_size_id: "size-1",
      namespace: "prod",
      reason: "scale up",
      batch_count: 3,
    });

    const { result } = renderHook(() => useVMManagementController({ t }));

    await act(async () => {
      result.current.submitWizard();
    });

    expect(createBatchMutate).toHaveBeenCalledWith({
      body: {
        operation: "CREATE",
        request_id: expect.any(String),
        reason: "scale up",
        items: [
          {
            service_id: "svc-1",
            template_id: "tpl-1",
            instance_size_id: "size-1",
            namespace: "prod",
            reason: "scale up",
          },
          {
            service_id: "svc-1",
            template_id: "tpl-1",
            instance_size_id: "size-1",
            namespace: "prod",
            reason: "scale up",
          },
          {
            service_id: "svc-1",
            template_id: "tpl-1",
            instance_size_id: "size-1",
            namespace: "prod",
            reason: "scale up",
          },
        ],
      },
      intent: expect.objectContaining({
        actorKey: "u-alice",
        operationKey: "CREATE",
        requestId: expect.any(String),
        fingerprint: expect.any(String),
      }),
      submissionSequence: expect.any(Number),
    });
    watchValues.batch_count = 1;
  });

  it("normalizes batch create text before fingerprinting whitespace-equivalent retries", async () => {
    watchValues.batch_count = 2;
    formState.getFieldsValue
      .mockReturnValueOnce({
        service_id: " svc-1 ",
        template_id: " tpl-1 ",
        instance_size_id: " size-1 ",
        namespace: " prod ",
        reason: " scale up ",
        batch_count: 2,
      })
      .mockReturnValueOnce({
        service_id: "svc-1",
        template_id: "tpl-1",
        instance_size_id: "size-1",
        namespace: "prod",
        reason: "scale up",
        batch_count: 2,
      });
    const { result } = renderHook(() => useVMManagementController({ t }));

    await act(async () => result.current.submitWizard());
    await act(async () => result.current.submitWizard());
    const [first, retry] = createBatchMutate.mock.calls.map(
      (call) =>
        call[0] as {
          body: {
            request_id: string;
            reason: string;
            items: Array<{
              service_id: string;
              template_id: string;
              instance_size_id: string;
              namespace: string;
              reason: string;
            }>;
          };
        },
    );

    expect(first?.body).toEqual(
      expect.objectContaining({
        reason: "scale up",
        items: expect.arrayContaining([
          expect.objectContaining({
            service_id: "svc-1",
            template_id: "tpl-1",
            instance_size_id: "size-1",
            namespace: "prod",
            reason: "scale up",
          }),
        ]),
      }),
    );
    expect(retry?.body.request_id).toBe(first?.body.request_id);
    watchValues.batch_count = 1;
  });

  it("includes requested target resources in batch create items", async () => {
    watchValues.batch_count = 2;
    formState.getFieldsValue.mockReturnValue({
      service_id: "svc-1",
      template_id: "tpl-1",
      instance_size_id: "size-1",
      namespace: "prod",
      reason: "scale up",
      batch_count: 2,
      target_cpu_cores: 3,
      target_memory_gi: 6,
      target_disk_gb: 80,
    });

    const { result } = renderHook(() => useVMManagementController({ t }));

    await act(async () => {
      await result.current.submitWizard();
    });

    expect(createBatchMutate).toHaveBeenCalledWith({
      body: {
        operation: "CREATE",
        request_id: expect.any(String),
        reason: "scale up",
        items: [
          {
            service_id: "svc-1",
            template_id: "tpl-1",
            instance_size_id: "size-1",
            namespace: "prod",
            reason: "scale up",
            target_cpu_cores: 3,
            target_memory_gi: 6,
            target_disk_gb: 80,
          },
          {
            service_id: "svc-1",
            template_id: "tpl-1",
            instance_size_id: "size-1",
            namespace: "prod",
            reason: "scale up",
            target_cpu_cores: 3,
            target_memory_gi: 6,
            target_disk_gb: 80,
          },
        ],
      },
      intent: expect.objectContaining({
        actorKey: "u-alice",
        operationKey: "CREATE",
        requestId: expect.any(String),
        fingerprint: expect.any(String),
      }),
      submissionSequence: expect.any(Number),
    });
    watchValues.batch_count = 1;
  });

  it("watches preserved wizard fields so confirm step summaries stay populated", () => {
    renderHook(() => useVMManagementController({ t }));

    expect(useWatchMock).toHaveBeenCalledWith(
      "template_id",
      expect.objectContaining({ form: formState, preserve: true }),
    );
    expect(useWatchMock).toHaveBeenCalledWith(
      "instance_size_id",
      expect.objectContaining({ form: formState, preserve: true }),
    );
    expect(useWatchMock).toHaveBeenCalledWith(
      "service_id",
      expect.objectContaining({ form: formState, preserve: true }),
    );
    expect(useWatchMock).toHaveBeenCalledWith(
      "batch_count",
      expect.objectContaining({ form: formState, preserve: true }),
    );
    expect(useWatchMock).toHaveBeenCalledWith(
      "target_cpu_cores",
      expect.objectContaining({ form: formState, preserve: true }),
    );
    expect(useWatchMock).toHaveBeenCalledWith(
      "target_memory_gi",
      expect.objectContaining({ form: formState, preserve: true }),
    );
    expect(useWatchMock).toHaveBeenCalledWith(
      "target_disk_gb",
      expect.objectContaining({ form: formState, preserve: true }),
    );
  });

  it("submits batch power/delete with selected VM ids", () => {
    const { result } = renderHook(() => useVMManagementController({ t }));

    act(() => {
      result.current.setSelectedVMIDs(["vm-1", "vm-2"]);
    });
    act(() => {
      result.current.submitBatchPowerSelected("START");
      result.current.submitBatchDeleteSelected();
    });

    expect(vmBatchPowerMutate).toHaveBeenCalledWith({
      body: {
        operation: "START",
        request_id: expect.any(String),
        reason: "batch.power_reason",
        items: [
          { vm_id: "vm-1", reason: "batch.power_reason" },
          { vm_id: "vm-2", reason: "batch.power_reason" },
        ],
      },
      intent: expect.objectContaining({
        operationKey: "POWER:START",
        requestId: expect.any(String),
      }),
      submissionSequence: expect.any(Number),
    });
    expect(vmBatchMutate).toHaveBeenCalledWith({
      body: {
        operation: "DELETE",
        request_id: expect.any(String),
        reason: "batch.delete_reason",
        items: [
          { vm_id: "vm-1", reason: "batch.delete_reason" },
          { vm_id: "vm-2", reason: "batch.delete_reason" },
        ],
      },
      intent: expect.objectContaining({
        operationKey: "DELETE",
        requestId: expect.any(String),
      }),
      submissionSequence: expect.any(Number),
    });
  });

  it("reuses request_id after a lost response and rotates it only after success or a new intent", () => {
    const { result } = renderHook(() => useVMManagementController({ t }));
    const powerBatchOptions = getMutationOptions("/vms/batch/power");

    act(() => result.current.setSelectedVMIDs(["vm-2", "vm-1"]));
    act(() => result.current.submitBatchPowerSelected("RESTART"));
    const firstSubmission = vmBatchPowerMutate.mock.calls[0]?.[0] as {
      body: { request_id: string };
      intent: Record<string, unknown>;
    };

    act(() => {
      powerBatchOptions?.onError?.({
        code: "HTTP_ERROR",
        status: 503,
      });
      result.current.setSelectedVMIDs(["vm-1", "vm-2"]);
    });
    act(() => result.current.submitBatchPowerSelected("RESTART"));
    const retrySubmission = vmBatchPowerMutate.mock.calls[1]?.[0] as {
      body: { request_id: string };
      intent: Record<string, unknown>;
    };
    expect(retrySubmission.body.request_id).toBe(
      firstSubmission.body.request_id,
    );

    act(() => {
      powerBatchOptions?.onSuccess?.(
        {
          batch_id: "batch-restart",
          status: "IN_PROGRESS",
          status_url: "/api/v1/vms/batch/batch-restart",
          retry_after_seconds: 2,
        },
        retrySubmission,
      );
    });
    act(() => result.current.submitBatchPowerSelected("RESTART"));
    const afterSuccessSubmission = vmBatchPowerMutate.mock.calls[2]?.[0] as {
      body: { request_id: string };
    };
    expect(afterSuccessSubmission.body.request_id).not.toBe(
      firstSubmission.body.request_id,
    );

    act(() => result.current.setSelectedVMIDs(["vm-3"]));
    act(() => result.current.submitBatchPowerSelected("RESTART"));
    const changedIntentSubmission = vmBatchPowerMutate.mock.calls[3]?.[0] as {
      body: { request_id: string };
    };
    expect(changedIntentSubmission.body.request_id).not.toBe(
      afterSuccessSubmission.body.request_id,
    );
  });

  it("keeps a failed batch modify form open and resets it only after accepted success", async () => {
    formState.getFieldsValue.mockReturnValue({
      reason: "resize batch",
      target_memory_gi: 8,
    });
    const { result } = renderHook(() => useVMManagementController({ t }));
    const submitBatchOptions = getMutationOptions("/vms/batch", 1);

    act(() => result.current.setSelectedVMIDs(["vm-1", "vm-2"]));
    act(() => result.current.openBatchModifyModal());
    const resetsBeforeSubmit = formState.resetFields.mock.calls.length;
    await act(async () => result.current.submitModify());

    expect(result.current.modifyOpen).toBe(true);
    expect(formState.resetFields).toHaveBeenCalledTimes(resetsBeforeSubmit);
    const firstSubmission = vmBatchMutate.mock.calls[0]?.[0] as {
      body: { request_id: string; operation: string };
      intent: Record<string, unknown>;
    };
    expect(firstSubmission.body).toEqual(
      expect.objectContaining({
        operation: "MODIFY",
        request_id: expect.any(String),
      }),
    );

    act(() =>
      submitBatchOptions?.onError?.({ code: "HTTP_ERROR", status: 503 }),
    );
    expect(result.current.modifyOpen).toBe(true);
    expect(formState.resetFields).toHaveBeenCalledTimes(resetsBeforeSubmit);

    await act(async () => result.current.submitModify());
    const retrySubmission = vmBatchMutate.mock.calls[1]?.[0] as {
      body: { request_id: string };
      intent: Record<string, unknown>;
    };
    expect(retrySubmission.body.request_id).toBe(
      firstSubmission.body.request_id,
    );

    act(() => {
      submitBatchOptions?.onSuccess?.(
        {
          batch_id: "batch-modify",
          status: "PENDING_APPROVAL",
          status_url: "/api/v1/vms/batch/batch-modify",
          retry_after_seconds: 2,
        },
        retrySubmission,
      );
    });
    expect(result.current.modifyOpen).toBe(false);
    expect(formState.resetFields).toHaveBeenCalledTimes(resetsBeforeSubmit + 1);
  });

  it("keeps a pending modify request_id across close and reopen", async () => {
    formState.getFieldsValue.mockReturnValue({
      reason: "resize pending",
      target_memory_gi: 8,
    });
    const { result } = renderHook(() => useVMManagementController({ t }));

    act(() => result.current.setSelectedVMIDs(["vm-1", "vm-2"]));
    act(() => result.current.openBatchModifyModal());
    await act(async () => result.current.submitModify());
    const firstSubmission = vmBatchMutate.mock.calls[0]?.[0] as {
      body: { request_id: string };
    };

    act(() => result.current.closeModifyModal());
    const persistedAfterClose = window.sessionStorage.getItem(
      "shepherd-vm-batch-request-intents",
    );
    expect(persistedAfterClose).toContain(firstSubmission.body.request_id);

    act(() => result.current.openBatchModifyModal());
    await act(async () => result.current.submitModify());
    const reopenedSubmission = vmBatchMutate.mock.calls[1]?.[0] as {
      body: { request_id: string };
    };
    expect(reopenedSubmission.body.request_id).toBe(
      firstSubmission.body.request_id,
    );
  });

  it("normalizes batch modify text before fingerprinting whitespace-equivalent retries", async () => {
    formState.getFieldsValue
      .mockReturnValueOnce({
        reason: " resize batch ",
        target_memory_gi: 8,
      })
      .mockReturnValueOnce({
        reason: "resize batch",
        target_memory_gi: 8,
      });
    const { result } = renderHook(() => useVMManagementController({ t }));

    act(() => result.current.setSelectedVMIDs([" vm-2 ", " vm-1 "]));
    act(() => result.current.openBatchModifyModal());
    await act(async () => result.current.submitModify());
    act(() => result.current.setSelectedVMIDs(["vm-1", "vm-2"]));
    await act(async () => result.current.submitModify());
    const [first, retry] = vmBatchMutate.mock.calls.map(
      (call) =>
        call[0] as {
          body: {
            request_id: string;
            reason: string;
            items: Array<{ vm_id: string; reason: string }>;
          };
        },
    );

    expect(first?.body).toEqual(
      expect.objectContaining({
        reason: "resize batch",
        items: expect.arrayContaining([
          expect.objectContaining({ vm_id: "vm-1", reason: "resize batch" }),
          expect.objectContaining({ vm_id: "vm-2", reason: "resize batch" }),
        ]),
      }),
    );
    expect(retry?.body.request_id).toBe(first?.body.request_id);
  });

  it("reuses an unresolved power request_id after unmount and remount", () => {
    const firstView = renderHook(() => useVMManagementController({ t }));
    act(() => firstView.result.current.setSelectedVMIDs(["vm-2", "vm-1"]));
    act(() => firstView.result.current.submitBatchPowerSelected("RESTART"));
    const firstSubmission = vmBatchPowerMutate.mock.calls[0]?.[0] as {
      body: { request_id: string };
    };
    firstView.unmount();

    const secondView = renderHook(() => useVMManagementController({ t }));
    act(() => secondView.result.current.setSelectedVMIDs(["vm-1", "vm-2"]));
    act(() => secondView.result.current.submitBatchPowerSelected("RESTART"));
    const remountedSubmission = vmBatchPowerMutate.mock.calls[1]?.[0] as {
      body: { request_id: string };
    };

    expect(remountedSubmission.body.request_id).toBe(
      firstSubmission.body.request_id,
    );
  });

  it("retains independent A and B power intents for their own retries", () => {
    const { result } = renderHook(() => useVMManagementController({ t }));
    for (const vmID of ["vm-a", "vm-b", "vm-a", "vm-b"]) {
      act(() => result.current.setSelectedVMIDs([vmID]));
      act(() => result.current.submitBatchPowerSelected("START"));
    }
    const requestIDs = vmBatchPowerMutate.mock.calls.map(
      (call) => (call[0] as { body: { request_id: string } }).body.request_id,
    );

    expect(requestIDs[0]).toBe(requestIDs[2]);
    expect(requestIDs[1]).toBe(requestIDs[3]);
    expect(requestIDs[0]).not.toBe(requestIDs[1]);
  });

  it("reuses delete and power intents when only localized automatic reasons change", () => {
    let locale: "default" | "alternate" = "default";
    const localizedT = ((key: string) => {
      if (key === "batch.delete_reason") {
        return locale === "default" ? "Batch delete" : "Localized batch delete";
      }
      if (key === "batch.power_reason") {
        return locale === "default" ? "Batch power" : "Localized batch power";
      }
      return key;
    }) as unknown as TFunction;
    const { result } = renderHook(() =>
      useVMManagementController({ t: localizedT }),
    );

    act(() => result.current.setSelectedVMIDs([" vm-2 ", " vm-1 "]));
    act(() => {
      result.current.submitBatchPowerSelected("START");
      result.current.submitBatchDeleteSelected();
    });
    locale = "alternate";
    act(() => result.current.setSelectedVMIDs(["vm-1", "vm-2"]));
    act(() => {
      result.current.submitBatchPowerSelected("START");
      result.current.submitBatchDeleteSelected();
    });

    const [powerFirst, powerRetry] = vmBatchPowerMutate.mock.calls.map(
      (call) => call[0] as { body: { request_id: string; reason: string } },
    );
    const [deleteFirst, deleteRetry] = vmBatchMutate.mock.calls.map(
      (call) => call[0] as { body: { request_id: string; reason: string } },
    );
    expect(powerFirst?.body.reason).not.toBe(powerRetry?.body.reason);
    expect(powerRetry?.body.request_id).toBe(powerFirst?.body.request_id);
    expect(deleteFirst?.body.reason).not.toBe(deleteRetry?.body.reason);
    expect(deleteRetry?.body.request_id).toBe(deleteFirst?.body.request_id);
  });

  it("tracks out-of-order successes without letting an older success overwrite a newer one", async () => {
    const { result } = renderHook(() => useVMManagementController({ t }));
    const submitBatchOptions = getMutationOptions("/vms/batch", 1);
    act(() => result.current.setSelectedVMIDs(["vm-1"]));
    act(() => result.current.openBatchModifyModal());

    formState.getFieldsValue.mockReturnValue({
      reason: "intent-a",
      target_memory_gi: 8,
    });
    await act(async () => result.current.submitModify());
    const submissionA = vmBatchMutate.mock.calls[0]?.[0] as Record<
      string,
      unknown
    > & {
      body: { request_id: string };
    };

    formState.getFieldsValue.mockReturnValue({
      reason: "intent-b",
      target_memory_gi: 16,
    });
    await act(async () => result.current.submitModify());
    const submissionB = vmBatchMutate.mock.calls[1]?.[0] as Record<
      string,
      unknown
    > & {
      body: { request_id: string };
    };
    const resetsBeforeSuccess = formState.resetFields.mock.calls.length;

    act(() =>
      submitBatchOptions?.onSuccess?.(
        {
          batch_id: "batch-a",
          status: "PENDING_APPROVAL",
          status_url: "/api/v1/vms/batch/batch-a",
          retry_after_seconds: 2,
        },
        submissionA,
      ),
    );
    expect(result.current.modifyOpen).toBe(true);
    expect(result.current.activeBatchID).toBe("batch-a");
    expect(formState.resetFields).toHaveBeenCalledTimes(resetsBeforeSuccess);
    expect(
      window.sessionStorage.getItem("shepherd-vm-batch-request-intents"),
    ).not.toContain(submissionA.body.request_id);
    expect(
      window.sessionStorage.getItem("shepherd-vm-batch-request-intents"),
    ).toContain(submissionB.body.request_id);

    act(() =>
      submitBatchOptions?.onSuccess?.(
        {
          batch_id: "batch-b",
          status: "PENDING_APPROVAL",
          status_url: "/api/v1/vms/batch/batch-b",
          retry_after_seconds: 2,
        },
        submissionB,
      ),
    );
    expect(result.current.modifyOpen).toBe(false);
    expect(result.current.activeBatchID).toBe("batch-b");

    act(() => result.current.openBatchModifyModal());
    formState.getFieldsValue.mockReturnValue({
      reason: "intent-c",
      target_memory_gi: 32,
    });
    await act(async () => result.current.submitModify());
    const submissionC = vmBatchMutate.mock.calls[2]?.[0] as Record<
      string,
      unknown
    > & {
      body: { request_id: string };
    };
    const resetsBeforeStaleSuccess = formState.resetFields.mock.calls.length;

    act(() =>
      submitBatchOptions?.onSuccess?.(
        {
          batch_id: "batch-a-late",
          status: "PENDING_APPROVAL",
          status_url: "/api/v1/vms/batch/batch-a-late",
          retry_after_seconds: 2,
        },
        submissionA,
      ),
    );
    expect(result.current.modifyOpen).toBe(true);
    expect(result.current.activeBatchID).toBe("batch-b");
    expect(formState.resetFields).toHaveBeenCalledTimes(
      resetsBeforeStaleSuccess,
    );
    expect(
      window.sessionStorage.getItem("shepherd-vm-batch-request-intents"),
    ).toContain(submissionC.body.request_id);
  });

  it("keeps an earlier accepted batch tracked when a newer submission fails", () => {
    const { result } = renderHook(() => useVMManagementController({ t }));
    const powerBatchOptions = getMutationOptions("/vms/batch/power");

    act(() => result.current.setSelectedVMIDs(["vm-a"]));
    act(() => result.current.submitBatchPowerSelected("START"));
    const submissionA = vmBatchPowerMutate.mock.calls[0]?.[0] as Record<
      string,
      unknown
    >;

    act(() => result.current.setSelectedVMIDs(["vm-b"]));
    act(() => result.current.submitBatchPowerSelected("START"));
    const submissionB = vmBatchPowerMutate.mock.calls[1]?.[0] as Record<
      string,
      unknown
    >;

    act(() =>
      powerBatchOptions?.onSuccess?.(
        {
          batch_id: "batch-a",
          status: "IN_PROGRESS",
          status_url: "/api/v1/vms/batch/batch-a",
          retry_after_seconds: 2,
        },
        submissionA,
      ),
    );
    expect(result.current.activeBatchID).toBe("batch-a");

    act(() =>
      powerBatchOptions?.onError?.(
        {
          code: "HTTP_ERROR",
          status: 503,
        },
        submissionB,
      ),
    );
    expect(result.current.activeBatchID).toBe("batch-a");
  });

  it("uses status_url for active batch tracking when batch submit succeeds", async () => {
    formState.getFieldsValue.mockReturnValue({
      service_id: "svc-1",
      template_id: "tpl-1",
      instance_size_id: "size-1",
      namespace: "prod",
      reason: "scale up",
      batch_count: 2,
    });
    const { result } = renderHook(() => useVMManagementController({ t }));

    const createBatchOptions = getMutationOptions("/vms/batch") as {
      onSuccess?: (
        data: {
          batch_id: string;
          status: string;
          status_url: string;
          retry_after_seconds: number;
        },
        variables: Record<string, unknown>,
      ) => void;
    };

    await act(async () => result.current.submitWizard());
    const submission = createBatchMutate.mock.calls[0]?.[0] as Record<
      string,
      unknown
    >;

    act(() => {
      createBatchOptions.onSuccess?.(
        {
          batch_id: "fallback-id",
          status: "PENDING_APPROVAL",
          status_url: "/api/v1/vms/batch/batch-from-status-url",
          retry_after_seconds: 3,
        },
        submission,
      );
    });

    expect(result.current.activeBatchID).toBe("batch-from-status-url");
    expect(result.current.activeBatchStatusURL).toBe(
      "/api/v1/vms/batch/batch-from-status-url",
    );
    expect(result.current.activeBatchKind).toBe("request");
    expect(messageSuccessMock).toHaveBeenCalledWith("batch.request_submitted");
  });

  it("marks power batches as job tracking and keeps batch workspace routing semantics", () => {
    const { result } = renderHook(() => useVMManagementController({ t }));

    const powerBatchOptions = getMutationOptions("/vms/batch/power") as {
      onSuccess?: (
        data: {
          batch_id: string;
          status: string;
          status_url: string;
          retry_after_seconds: number;
        },
        variables: Record<string, unknown>,
      ) => void;
    };

    act(() => result.current.setSelectedVMIDs(["vm-1"]));
    act(() => result.current.submitBatchPowerSelected("START"));
    const submission = vmBatchPowerMutate.mock.calls[0]?.[0] as Record<
      string,
      unknown
    >;

    act(() => {
      powerBatchOptions.onSuccess?.(
        {
          batch_id: "power-batch-1",
          status: "IN_PROGRESS",
          status_url: "/api/v1/vms/batch/power-batch-1",
          retry_after_seconds: 0,
        },
        submission,
      );
    });

    expect(result.current.activeBatchKind).toBe("job");
    expect(messageSuccessMock).toHaveBeenCalledWith("batch.job_submitted");
  });

  it("uses only eligible FAILED children and records authoritative retry/cancel results", () => {
    batchStatusState.status = "IN_PROGRESS";
    const { result } = renderHook(() => useVMManagementController({ t }));
    const powerBatchOptions = getMutationOptions("/vms/batch/power") as {
      onSuccess?: (
        data: {
          batch_id: string;
          status: string;
          status_url: string;
          retry_after_seconds: number;
        },
        variables: Record<string, unknown>,
      ) => void;
    };
    const retryOptions = getMutationOptions("/vms/batch/{batch_id}/retry") as {
      onSuccess?: (
        data: {
          batch_id: string;
          status: string;
          affected_count: number;
          affected_ticket_ids?: string[];
        },
        variables?: Record<string, unknown>,
      ) => void;
    };
    const cancelOptions = getMutationOptions(
      "/vms/batch/{batch_id}/cancel",
    ) as {
      onSuccess?: (
        data: {
          batch_id: string;
          status: string;
          affected_count: number;
          affected_ticket_ids?: string[];
        },
        variables?: Record<string, unknown>,
      ) => void;
    };

    act(() => result.current.setSelectedVMIDs(["vm-1"]));
    act(() => result.current.submitBatchPowerSelected("START"));
    const submission = vmBatchPowerMutate.mock.calls[0]?.[0] as Record<
      string,
      unknown
    >;
    act(() => {
      powerBatchOptions.onSuccess?.(
        {
          batch_id: "batch-live-1",
          status: "IN_PROGRESS",
          status_url: "/api/v1/vms/batch/batch-live-1",
          retry_after_seconds: 0,
        },
        submission,
      );
    });
    act(() => result.current.retryBatch());
    expect(retryBatchMutate).toHaveBeenCalledWith({
      actorKey: "u-alice",
      batchID: "batch-live-1",
      targetTicketIDs: ["ticket-failed-1"],
    });
    const retrySubmission = retryBatchMutate.mock.calls[0]?.[0] as Record<
      string,
      unknown
    >;

    act(() => {
      retryOptions.onSuccess?.(
        {
          batch_id: "batch-live-1",
          status: "IN_PROGRESS",
          affected_count: 1,
          affected_ticket_ids: ["ticket-failed-1"],
        },
        retrySubmission,
      );
    });
    expect(result.current.lastBatchActionFeedback).toEqual({
      action: "retry",
      affectedCount: 1,
      affectedTicketIDs: ["ticket-failed-1"],
    });

    act(() => result.current.cancelBatch());
    expect(cancelBatchMutate).toHaveBeenCalledWith({
      actorKey: "u-alice",
      batchID: "batch-live-1",
      targetTicketIDs: ["ticket-pending-1"],
    });
    const cancelSubmission = cancelBatchMutate.mock.calls[0]?.[0] as Record<
      string,
      unknown
    >;
    act(() => {
      cancelOptions.onSuccess?.(
        {
          batch_id: "batch-live-1",
          status: "CANCELLED",
          affected_count: 1,
          affected_ticket_ids: ["ticket-pending-1"],
        },
        cancelSubmission,
      );
    });
    expect(result.current.lastBatchActionFeedback).toEqual({
      action: "cancel",
      affectedCount: 1,
      affectedTicketIDs: ["ticket-pending-1"],
    });
  });

  it("applies Retry-After cooldown to retry and prevents repeated mutation", () => {
    batchStatusState.status = "IN_PROGRESS";
    const { result } = renderHook(() => useVMManagementController({ t }));
    const powerBatchOptions = getMutationOptions("/vms/batch/power") as {
      onSuccess?: (
        data: {
          batch_id: string;
          status: string;
          status_url: string;
          retry_after_seconds: number;
        },
        variables: Record<string, unknown>,
      ) => void;
    };
    const retryOptions = getMutationOptions("/vms/batch/{batch_id}/retry") as {
      onError?: (error: { code: string; retry_after_seconds?: number }) => void;
    };

    act(() => result.current.setSelectedVMIDs(["vm-1"]));
    act(() => result.current.submitBatchPowerSelected("START"));
    const submission = vmBatchPowerMutate.mock.calls[0]?.[0] as Record<
      string,
      unknown
    >;
    act(() => {
      powerBatchOptions.onSuccess?.(
        {
          batch_id: "batch-live-1",
          status: "IN_PROGRESS",
          status_url: "/api/v1/vms/batch/batch-live-1",
          retry_after_seconds: 0,
        },
        submission,
      );
    });
    act(() => {
      retryOptions.onError?.({
        code: "BATCH_RATE_LIMITED",
        retry_after_seconds: 7,
      });
    });
    expect(result.current.batchRetryAfterSeconds).toBeGreaterThan(0);

    act(() => result.current.retryBatch());
    expect(retryBatchMutate).not.toHaveBeenCalled();
    expect(messageWarningMock).toHaveBeenCalledWith("batch.rate_limited_wait");
  });

  it("surfaces ambiguous restart recovery metadata without a fence-clearing mutation", () => {
    const { result } = renderHook(() => useVMManagementController({ t }));
    const restartOptions = getActionOptions("/vms/{vm_id}/restart");
    expect(useApiMutationMock).toHaveBeenCalledTimes(8);

    act(() => {
      restartOptions?.onError?.({
        code: "POWER_OPERATION_IN_PROGRESS",
        status: 409,
        params: {
          operator_action_required: true,
          existing_event_id: "event-restart-1",
          reconciliation_path: "operator-runbook:ambiguous-vm-restart",
        },
      });
    });
    expect(result.current.restartReconciliationNotice).toEqual({
      eventId: "event-restart-1",
      reconciliationPath: "operator-runbook:ambiguous-vm-restart",
    });
    expect(
      useApiMutationMock.mock.calls.some((call) =>
        String(call[0]).includes("/admin/vm-power-events/"),
      ),
    ).toBe(false);
  });

  it("surfaces ambiguous restart recovery metadata from batch submit and retry errors", () => {
    const { result } = renderHook(() => useVMManagementController({ t }));
    const powerBatchOptions = getMutationOptions("/vms/batch/power");
    const retryOptions = getMutationOptions("/vms/batch/{batch_id}/retry");

    act(() => {
      powerBatchOptions?.onError?.({
        code: "POWER_OPERATION_IN_PROGRESS",
        status: 409,
        params: {
          operator_action_required: true,
          existing_event_id: "event-power-submit",
          reconciliation_path: "operator-runbook:ambiguous-vm-restart",
        },
      });
    });
    expect(result.current.restartReconciliationNotice?.eventId).toBe(
      "event-power-submit",
    );

    act(() => {
      retryOptions?.onError?.({
        code: "DUPLICATE_PENDING_REQUEST",
        status: 409,
        params: {
          operator_action_required: true,
          existing_event_id: "event-power-retry",
          reconciliation_path: "operator-runbook:ambiguous-vm-restart",
        },
      });
    });
    expect(result.current.restartReconciliationNotice).toEqual({
      eventId: "event-power-retry",
      reconciliationPath: "operator-runbook:ambiguous-vm-restart",
    });
    expect(
      useApiMutationMock.mock.calls.some((call) =>
        String(call[0]).includes("/admin/vm-power-events/"),
      ),
    ).toBe(false);
  });

  it("restores active batch tracking from session storage", () => {
    window.sessionStorage.setItem(
      "shepherd-active-batch",
      JSON.stringify({
        actor_id: "u-alice",
        batch_id: "batch-restored-1",
        status_url: "/api/v1/vms/batch/batch-restored-1",
        kind: "request",
      }),
    );

    const { result } = renderHook(() => useVMManagementController({ t }));

    expect(result.current.activeBatchID).toBe("batch-restored-1");
    expect(result.current.activeBatchStatusURL).toBe(
      "/api/v1/vms/batch/batch-restored-1",
    );
    expect(result.current.activeBatchKind).toBe("request");
  });

  it("waits for the authenticated actor before restoring owned batch state", () => {
    window.sessionStorage.setItem(
      "shepherd-active-batch",
      JSON.stringify({
        actor_id: "u-alice",
        batch_id: "batch-restored-after-auth",
        status_url: "/api/v1/vms/batch/batch-restored-after-auth",
        kind: "request",
      }),
    );
    authUserState.user.id = "";
    const view = renderHook(() => useVMManagementController({ t }));

    expect(view.result.current.activeBatchID).toBe("");
    expect(window.sessionStorage.getItem("shepherd-active-batch")).toContain(
      "batch-restored-after-auth",
    );

    authUserState.user.id = "u-alice";
    view.rerender();

    expect(view.result.current.activeBatchID).toBe("batch-restored-after-auth");
  });

  it("rejects active batch tracking owned by a previous actor", () => {
    window.sessionStorage.setItem(
      "shepherd-active-batch",
      JSON.stringify({
        actor_id: "u-alice",
        batch_id: "batch-alice",
        status_url: "/api/v1/vms/batch/batch-alice",
        kind: "job",
      }),
    );
    const view = renderHook(() => useVMManagementController({ t }));
    expect(view.result.current.activeBatchID).toBe("batch-alice");

    authUserState.user.id = "u-bob";
    authUserState.user.username = "bob";
    view.rerender();

    expect(view.result.current.activeBatchID).toBe("");
    expect(window.sessionStorage.getItem("shepherd-active-batch")).toBeNull();
  });

  it("enters cooldown on BATCH_RATE_LIMITED and blocks batch actions while countdown active", () => {
    watchValues.batch_count = 3;
    formState.getFieldsValue.mockReturnValue({
      service_id: "svc-1",
      template_id: "tpl-1",
      instance_size_id: "size-1",
      namespace: "prod",
      reason: "scale up",
      batch_count: 3,
    });

    const { result } = renderHook(() => useVMManagementController({ t }));

    const createBatchOptions = getMutationOptions("/vms/batch") as {
      onError?: (error: {
        code: string;
        params?: Record<string, unknown>;
      }) => void;
    };

    act(() => {
      createBatchOptions.onError?.({
        code: "BATCH_RATE_LIMITED",
        params: { retry_after_seconds: 5 },
      });
      result.current.setSelectedVMIDs(["vm-1", "vm-2"]);
    });

    expect(result.current.batchRateLimited).toBe(true);
    expect(result.current.batchRateLimitContactAdmin).toBe(false);
    expect(result.current.batchRetryAfterSeconds).toBeGreaterThan(0);

    act(() => {
      result.current.submitWizard();
      result.current.submitBatchPowerSelected("START");
      result.current.submitBatchDeleteSelected();
    });

    expect(createBatchMutate).not.toHaveBeenCalled();
    expect(vmBatchPowerMutate).not.toHaveBeenCalled();
    expect(vmBatchMutate).not.toHaveBeenCalled();
    expect(messageWarningMock).toHaveBeenCalledWith("batch.rate_limited_wait");

    watchValues.batch_count = 1;
  });

  it("shows administrator guidance when the server marks a batch limit as non-user-configurable", () => {
    const { result } = renderHook(() => useVMManagementController({ t }));
    const createBatchOptions = getMutationOptions("/vms/batch") as {
      onError?: (error: {
        code: string;
        params?: Record<string, unknown>;
      }) => void;
    };

    act(() => {
      createBatchOptions.onError?.({
        code: "BATCH_RATE_LIMITED",
        params: { retry_after_seconds: 5, contact_admin: true },
      });
    });

    expect(result.current.batchRateLimited).toBe(true);
    expect(result.current.batchRateLimitContactAdmin).toBe(true);
    expect(messageWarningMock).toHaveBeenCalledWith(
      "batch.rate_limited_contact_admin",
    );

    messageWarningMock.mockClear();
    act(() => result.current.submitBatchPowerSelected("START"));
    expect(messageWarningMock).toHaveBeenCalledWith(
      "batch.rate_limited_contact_admin",
    );
    expect(vmBatchPowerMutate).not.toHaveBeenCalled();
  });

  it("blocks batch delete early when selected VMs are still running", () => {
    vmItems.splice(
      0,
      vmItems.length,
      { id: "vm-1", name: "vm-one", status: "RUNNING" },
      { id: "vm-2", name: "vm-two", status: "STOPPED" },
    );

    const { result } = renderHook(() => useVMManagementController({ t }));

    act(() => {
      result.current.setSelectedVMIDs(["vm-1", "vm-2"]);
    });

    act(() => {
      result.current.submitBatchDeleteSelected();
    });

    expect(vmBatchMutate).not.toHaveBeenCalled();
    expect(messageWarningMock).toHaveBeenCalledWith(
      "batch.delete_requires_stopped",
    );
  });

  it("rejects modify submit when no target resource is provided", async () => {
    formState.getFieldsValue.mockReturnValue({
      reason: "scale up",
    });
    const { result } = renderHook(() => useVMManagementController({ t }));

    act(() => {
      result.current.openModifyModal("vm-1", "vm-one");
    });

    await act(async () => {
      await result.current.submitModify();
    });

    expect(createModifyMutate).not.toHaveBeenCalled();
    expect(messageWarningMock).toHaveBeenCalledWith("modify.target_required");
  });

  it("allows single-vm modify requests that reduce cpu or memory while still blocking disk shrink", async () => {
    formState.getFieldsValue.mockReturnValue({
      reason: "scale up",
      target_memory_gi: 4,
    });
    const { result } = renderHook(() => useVMManagementController({ t }));

    act(() => {
      result.current.openModifyModal("vm-1", "vm-one");
    });

    await act(async () => {
      await result.current.submitModify();
    });

    expect(createModifyMutate).toHaveBeenCalledWith({
      vmId: "vm-1",
      body: {
        reason: "scale up",
        target_cpu_cores: 0,
        target_memory_gi: 4,
        target_disk_gb: 0,
      },
    });
  });
});
