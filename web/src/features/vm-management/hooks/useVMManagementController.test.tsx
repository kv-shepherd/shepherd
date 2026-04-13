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
    selector: (state: { user: { id: string; username: string } }) => unknown,
  ) => selector({ user: { id: "u-alice", username: "alice" } }),
}));

import { useVMManagementController } from "./useVMManagementController";
import { buildVMRequestDraftStorageKey } from "../draftStorage";

describe("useVMManagementController", () => {
  const t = ((key: string) => key) as unknown as TFunction;

  const getMutationOptions = (slot: number) =>
    useApiMutationMock.mock.calls[slot]?.[1] as
      | {
          onSuccess?: (data: Record<string, unknown>) => void;
          onError?: (error: {
            code?: string;
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

      if (
        queryKey[0] === "vms" &&
        queryKey[1] === 1 &&
        queryKey[2] === 20
      ) {
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

      if (
        queryKey[0] === "templates" &&
        queryKey[1] === "vm-wizard-fallback"
      ) {
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
            status: "PARTIAL_SUCCESS",
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
                attempt_count: 2,
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

    let mutationCall = 0;
    useApiMutationMock.mockImplementation(() => {
      mutationCall += 1;
      const slot = ((mutationCall - 1) % 8) + 1;
      if (slot === 1) return { mutate: createMutate, isPending: false };
      if (slot === 2) return { mutate: createModifyMutate, isPending: false };
      if (slot === 3) return { mutate: createBatchMutate, isPending: false };
      if (slot === 4) return { mutate: vmBatchMutate, isPending: false };
      if (slot === 5) return { mutate: vmBatchPowerMutate, isPending: false };
      if (slot === 6) return { mutate: retryBatchMutate, isPending: false };
      if (slot === 7) return { mutate: cancelBatchMutate, isPending: false };
      return { mutate: deleteMutate, isPending: false };
    });

    let actionCall = 0;
    useApiActionMock.mockImplementation(() => {
      actionCall += 1;
      if (actionCall % 3 === 1)
        return { mutate: postStartMutate, isPending: false };
      if (actionCall % 3 === 2)
        return { mutate: postStopMutate, isPending: false };
      return { mutate: postRestartMutate, isPending: false };
    });

    apiGetMock.mockImplementation(
      (path: string) => {
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
      },
    );
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
      operation: "CREATE",
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
      operation: "START",
      reason: "batch.power_reason",
      items: [
        { vm_id: "vm-1", reason: "batch.power_reason" },
        { vm_id: "vm-2", reason: "batch.power_reason" },
      ],
    });
    expect(vmBatchMutate).toHaveBeenCalledWith({
      operation: "DELETE",
      reason: "batch.delete_reason",
      items: [
        { vm_id: "vm-1", reason: "batch.delete_reason" },
        { vm_id: "vm-2", reason: "batch.delete_reason" },
      ],
    });
  });

  it("uses status_url for active batch tracking when batch submit succeeds", () => {
    const { result } = renderHook(() => useVMManagementController({ t }));

    const createBatchOptions = getMutationOptions(2) as {
      onSuccess?: (data: {
        batch_id: string;
        status: string;
        status_url: string;
        retry_after_seconds: number;
      }) => void;
    };

    act(() => {
      createBatchOptions.onSuccess?.({
        batch_id: "fallback-id",
        status: "PENDING_APPROVAL",
        status_url: "/api/v1/vms/batch/batch-from-status-url",
        retry_after_seconds: 3,
      });
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

    const powerBatchOptions = getMutationOptions(4) as {
      onSuccess?: (data: {
        batch_id: string;
        status: string;
        status_url: string;
        retry_after_seconds: number;
      }) => void;
    };

    act(() => {
      powerBatchOptions.onSuccess?.({
        batch_id: "power-batch-1",
        status: "IN_PROGRESS",
        status_url: "/api/v1/vms/batch/power-batch-1",
        retry_after_seconds: 0,
      });
    });

    expect(result.current.activeBatchKind).toBe("job");
    expect(messageSuccessMock).toHaveBeenCalledWith("batch.job_submitted");
  });

  it("restores active batch tracking from session storage", () => {
    window.sessionStorage.setItem(
      "shepherd-active-batch",
      JSON.stringify({
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

    const createBatchOptions = getMutationOptions(2) as {
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
    expect(messageWarningMock).toHaveBeenCalledWith("batch.delete_requires_stopped");
  });

  it("records affected child ticket ids for retry/cancel feedback", () => {
    const { result } = renderHook(() => useVMManagementController({ t }));

    const createBatchOptions = getMutationOptions(2) as {
      onSuccess?: (data: {
        batch_id: string;
        status: string;
        status_url: string;
        retry_after_seconds: number;
      }) => void;
    };
    const retryOptions = getMutationOptions(5) as {
      onSuccess?: (data: {
        batch_id: string;
        status: string;
        affected_count: number;
        affected_ticket_ids?: string[];
      }) => void;
    };
    const cancelOptions = getMutationOptions(6) as {
      onSuccess?: (data: {
        batch_id: string;
        status: string;
        affected_count: number;
        affected_ticket_ids?: string[];
      }) => void;
    };

    act(() => {
      createBatchOptions.onSuccess?.({
        batch_id: "fallback-id",
        status: "PENDING_APPROVAL",
        status_url: "/api/v1/vms/batch/batch-live-1",
        retry_after_seconds: 2,
      });
    });

    act(() => {
      result.current.retryBatch();
      retryOptions.onSuccess?.({
        batch_id: "batch-live-1",
        status: "IN_PROGRESS",
        affected_count: 1,
        affected_ticket_ids: ["ticket-failed-1"],
      });
    });

    expect(retryBatchMutate).toHaveBeenCalledWith("batch-live-1");
    expect(result.current.lastBatchActionFeedback).toEqual({
      action: "retry",
      affectedCount: 1,
      affectedTicketIDs: ["ticket-failed-1"],
    });

    act(() => {
      result.current.cancelBatch();
      cancelOptions.onSuccess?.({
        batch_id: "batch-live-1",
        status: "CANCELLED",
        affected_count: 1,
        affected_ticket_ids: ["ticket-pending-1"],
      });
    });

    expect(cancelBatchMutate).toHaveBeenCalledWith("batch-live-1");
    expect(result.current.lastBatchActionFeedback).toEqual({
      action: "cancel",
      affectedCount: 1,
      affectedTicketIDs: ["ticket-pending-1"],
    });
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
