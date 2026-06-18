import { act, renderHook, waitFor } from "@testing-library/react";
import type { TFunction } from "i18next";
import { beforeEach, describe, expect, it, vi } from "vitest";

const {
  useApiGetMock,
  useApiMutationMock,
  useApiActionMock,
  useFormMock,
  useWatchMock,
  apiGetMock,
  applyApiFieldErrorsMock,
  approveFormState,
  rejectFormState,
  watchedValues,
  approveMutate,
  rejectMutate,
  retryBatchMutate,
  cancelMutate,
  messageSuccessMock,
  messageInfoMock,
  messageErrorMock,
} = vi.hoisted(() => ({
  useApiGetMock: vi.fn(),
  useApiMutationMock: vi.fn(),
  useApiActionMock: vi.fn(),
  useFormMock: vi.fn(),
  useWatchMock: vi.fn(),
  apiGetMock: vi.fn(),
  applyApiFieldErrorsMock: vi.fn(),
  approveFormState: {
    validateFields: vi.fn(),
    resetFields: vi.fn(),
    getFieldValue: vi.fn(),
    setFieldValue: vi.fn(),
    setFieldsValue: vi.fn(),
    setFields: vi.fn(),
  },
  rejectFormState: {
    validateFields: vi.fn(),
    resetFields: vi.fn(),
  },
  watchedValues: {
    selected_cluster_id: "cluster-a",
    selected_storage_class: "rook-ceph",
    selected_root_volume_mode_key: undefined,
    selected_dv_access_modes: undefined,
    selected_dv_volume_mode: undefined,
    enable_override: true,
    cpu_request: 1,
    cpu_limit: 2,
    memory_request_gi: 2,
    memory_limit_gi: 4,
  } as Record<string, unknown>,
  approveMutate: vi.fn(),
  rejectMutate: vi.fn(),
  retryBatchMutate: vi.fn(),
  cancelMutate: vi.fn(),
  messageSuccessMock: vi.fn(),
  messageInfoMock: vi.fn(),
  messageErrorMock: vi.fn(),
}));

vi.mock("antd", () => ({
  App: {
    useApp: () => ({
      message: {
        success: messageSuccessMock,
        info: messageInfoMock,
        error: messageErrorMock,
      },
    }),
  },
  Form: {
    useForm: (...args: unknown[]) => useFormMock(...args),
    useWatch: (...args: unknown[]) => useWatchMock(...args),
  },
}));

vi.mock("@/hooks/useApiQuery", () => ({
  useApiGet: (...args: unknown[]) => useApiGetMock(...args),
  useApiMutation: (...args: unknown[]) => useApiMutationMock(...args),
  useApiAction: (...args: unknown[]) => useApiActionMock(...args),
}));

vi.mock("@/hooks/applyApiFieldErrors", () => ({
  applyApiFieldErrors: (...args: unknown[]) => applyApiFieldErrorsMock(...args),
}));

vi.mock("@/lib/api/client", () => ({
  api: {
    GET: (...args: unknown[]) => apiGetMock(...args),
  },
}));

import { useAdminApprovalsController } from "./useAdminApprovalsController";
import { saveRememberedApprovalClusterPlacement } from "../approvalPlacementMemory";

function findAdminClustersQueryFn(stage: "base" | "resolved" | "validated") {
  const call = [...useApiGetMock.mock.calls]
    .reverse()
    .find(
      (entry) =>
        Array.isArray(entry[0]) &&
        entry[0][0] === "admin-clusters" &&
        entry[0][2] === stage,
    );
  return call?.[1];
}

describe("useAdminApprovalsController", () => {
  const t = ((key: string) => key) as unknown as TFunction;

  beforeEach(() => {
    vi.clearAllMocks();
    window.localStorage.clear();
    applyApiFieldErrorsMock.mockReturnValue(false);
    let formCall = 0;
    useFormMock.mockImplementation(() => {
      formCall += 1;
      return formCall % 2 === 1 ? [approveFormState] : [rejectFormState];
    });
    Object.assign(watchedValues, {
      selected_cluster_id: "cluster-a",
      selected_storage_class: "rook-ceph",
      selected_root_volume_mode_key: undefined,
      selected_dv_access_modes: undefined,
      selected_dv_volume_mode: undefined,
      enable_override: true,
      cpu_request: 1,
      cpu_limit: 2,
      memory_request_gi: 2,
      memory_limit_gi: 4,
    });
    approveFormState.getFieldValue.mockImplementation(
      (name: string) => watchedValues[name],
    );
    approveFormState.setFieldValue.mockImplementation(
      (name: string, value: unknown) => {
        watchedValues[name] = value;
      },
    );
    approveFormState.resetFields.mockImplementation(() => {
      watchedValues.selected_storage_class = "rook-ceph";
      watchedValues.selected_cluster_id = "cluster-a";
    });
    approveFormState.setFieldsValue.mockImplementation(
      (patch: Record<string, unknown>) => {
        Object.assign(watchedValues, patch);
      },
    );
    approveFormState.setFields.mockImplementation(
      (
        fields: Array<{
          name: string | string[];
          value?: unknown;
          errors?: string[];
        }>,
      ) => {
        for (const field of fields) {
          const name = Array.isArray(field.name)
            ? field.name[field.name.length - 1]
            : field.name;
          if (typeof name === "string") {
            watchedValues[name] = field.value;
          }
        }
      },
    );
    useWatchMock.mockImplementation((name: string) => {
      return watchedValues[name];
    });
    approveFormState.validateFields.mockResolvedValue({
      selected_cluster_id: "cluster-a",
      selected_storage_class: "rook-ceph",
      enable_override: true,
      comment: "approved",
    });
    rejectFormState.validateFields.mockResolvedValue({
      reason: "policy violation",
    });
    useApiGetMock.mockImplementation((queryKey: unknown[]) => {
      const key = Array.isArray(queryKey) ? queryKey[0] : undefined;
      if (key === "builtin-approval-tasks") {
        return {
          data: {
            items: [
              {
                id: "ticket-1",
                status: "PENDING",
                operation_type: "CREATE",
                requester: "alice",
              },
            ],
          },
          isLoading: false,
          refetch: vi.fn(),
        };
      }
      if (key === "admin-clusters") {
        return {
          data: {
            items: [
              {
                id: "cluster-a",
                name: "Cluster A",
                status: "HEALTHY",
                enabled: true,
                storage_classes: ["rook-ceph", "gold-sc"],
                default_storage_class: "rook-ceph",
                compatibility: {
                  eligible: false,
                  reason_code: "CLUSTER_POLICY_STORAGE_CLASS_REQUIRED",
                  reason_message:
                    "cluster policy requires an explicit allowed storage class",
                },
              },
            ],
          },
          isLoading: false,
          error: undefined,
        };
      }
      if (key === "admin-cluster-policy") {
        return {
          data: {
            id: "policy-1",
            cluster_id: "cluster-a",
            allow_cdi_clone: true,
            allowed_storage_classes: ["rook-ceph"],
          },
          isLoading: false,
        };
      }
      return {
        data: undefined,
        isLoading: false,
        error: undefined,
      };
    });
    let mutationCall = 0;
    useApiMutationMock.mockImplementation(() => {
      mutationCall += 1;
      const hookIndex = (mutationCall - 1) % 3;
      if (hookIndex === 0) {
        return { mutate: approveMutate, isPending: false };
      }
      if (hookIndex === 1) {
        return { mutate: rejectMutate, isPending: false };
      }
      return { mutate: retryBatchMutate, isPending: false };
    });
    useApiActionMock.mockReturnValue({
      mutate: cancelMutate,
      isPending: false,
    });
    apiGetMock.mockResolvedValue({});
  });

  it("resets paging when switching status filter", () => {
    const { result } = renderHook(() => useAdminApprovalsController({ t }));

    act(() => {
      result.current.setPage(3);
      result.current.changeStatusFilter("ALL");
    });

    expect(result.current.statusFilter).toBe("ALL");
    expect(result.current.page).toBe(1);
  });

  it("requests approvals using operation, cluster, and placement filters", async () => {
    const { result } = renderHook(() => useAdminApprovalsController({ t }));

    act(() => {
      result.current.changeOperationFilter("POWER");
      result.current.changeSelectedClusterFilter("cluster-a");
      result.current.changePlacementAdvisoryFilter(
        "PVC_CLONE_HOST_ASSISTED_FALLBACK_LIKELY",
      );
      result.current.changePlacementSnapshotFilter("present");
    });

    const approvalsCall = [...useApiGetMock.mock.calls]
      .reverse()
      .find(
        (call) =>
          Array.isArray(call[0]) && call[0][0] === "builtin-approval-tasks",
      );
    const approvalsQueryFn = approvalsCall?.[1];
    expect(approvalsQueryFn).toBeTypeOf("function");

    await act(async () => {
      await approvalsQueryFn();
    });

    expect(apiGetMock).toHaveBeenCalledWith("/builtin-approval/tasks", {
      params: {
        query: {
          operation_type: "POWER",
          selected_cluster_id: "cluster-a",
          placement_advisory_code: "PVC_CLONE_HOST_ASSISTED_FALLBACK_LIKELY",
          placement_snapshot: "present",
          page: 1,
          per_page: 20,
        },
      },
    });
  });

  it("disables retries and exposes cluster compatibility query errors", () => {
    const clusterQueryError = {
      code: "OVERCOMMIT_INVALID",
      message: "cpu request must not exceed cpu cores",
      status: 400,
    };
    useApiGetMock.mockImplementation((queryKey: unknown[]) => {
      const key = Array.isArray(queryKey) ? queryKey[0] : undefined;
      if (key === "builtin-approval-tasks") {
        return {
          data: { items: [] },
          isLoading: false,
          refetch: vi.fn(),
          error: undefined,
        };
      }
      if (key === "admin-clusters") {
        return {
          data: undefined,
          isLoading: false,
          error: clusterQueryError,
        };
      }
      return {
        data: undefined,
        isLoading: false,
        error: undefined,
      };
    });

    const { result } = renderHook(() => useAdminApprovalsController({ t }));

    expect(result.current.clusterQueryError).toEqual(clusterQueryError);
    const clusterCall = useApiGetMock.mock.calls.find(
      (call) => Array.isArray(call[0]) && call[0][0] === "admin-clusters",
    );
    expect(clusterCall?.[2]).toMatchObject({ retry: false });
  });

  it("submits approve/reject decisions with selected ticket ids", async () => {
    const { result } = renderHook(() => useAdminApprovalsController({ t }));
    const pendingTicket = {
      id: "ticket-1",
      operation_type: "CREATE",
      status: "PENDING",
      requester: "alice",
    };

    act(() => {
      result.current.openApproveModal(pendingTicket as never);
      result.current.openRejectModal(pendingTicket as never);
    });

    await act(async () => {
      await result.current.submitApprove();
    });
    expect(approveMutate).toHaveBeenCalledWith({
      ticketId: "ticket-1",
      body: {
        selected_cluster_id: "cluster-a",
        selected_storage_class: "rook-ceph",
        enable_override: true,
        comment: "approved",
      },
    });

    await act(async () => {
      await result.current.submitReject();
    });
    expect(rejectMutate).toHaveBeenCalledWith({
      ticketId: "ticket-1",
      body: { reason: "policy violation" },
    });
  });

  it("blocks create approvals when the target cluster is still missing", async () => {
    watchedValues.selected_cluster_id = undefined;
    approveFormState.validateFields.mockResolvedValue({
      selected_cluster_id: undefined,
      selected_storage_class: "rook-ceph",
      enable_override: true,
      comment: "approved",
    });

    const { result } = renderHook(() => useAdminApprovalsController({ t }));

    act(() => {
      result.current.openApproveModal({
        id: "ticket-1",
        operation_type: "CREATE",
        status: "PENDING",
        requester: "alice",
      } as never);
    });

    await act(async () => {
      await result.current.submitApprove();
    });

    expect(approveMutate).not.toHaveBeenCalled();
    expect(approveFormState.setFields).toHaveBeenCalledWith([
      {
        name: ["selected_cluster_id"],
        errors: ["approve_modal.cluster_required"],
      },
    ]);
  });

  it("prefills failed create batch review from persisted approval inputs", () => {
    const { result } = renderHook(() => useAdminApprovalsController({ t }));

    act(() => {
      result.current.openBatchRetryReviewModal({
        id: "ticket-batch-failed",
        operation_type: "CREATE",
        status: "FAILED",
        requester: "alice",
        selected_cluster_id: "cluster-review",
        selected_storage_class: "gold-sc",
        placement_evaluation: {
          requested_dv_access_modes: ["ReadWriteOnce"],
          requested_dv_volume_mode: "Filesystem",
          override: {
            enabled: true,
            cpu_request: 2,
            memory_request_gi: 4,
            disk_gb: 120,
          },
        },
      } as never);
    });

    expect(approveFormState.setFieldsValue).toHaveBeenCalledWith(
      expect.objectContaining({
        selected_cluster_id: "cluster-review",
        selected_storage_class: "gold-sc",
        selected_dv_access_modes: ["ReadWriteOnce"],
        selected_dv_volume_mode: "Filesystem",
        enable_override: true,
        cpu_request: 2,
        memory_request_gi: 4,
        disk_gb: 120,
      }),
    );
  });

  it("submits failed create batch retries through the batch retry mutation", async () => {
    const { result } = renderHook(() => useAdminApprovalsController({ t }));

    act(() => {
      result.current.openBatchRetryReviewModal({
        id: "ticket-batch-failed",
        operation_type: "CREATE",
        status: "FAILED",
        requester: "alice",
      } as never);
    });

    await act(async () => {
      await result.current.submitApprove();
    });

    expect(retryBatchMutate).toHaveBeenCalledWith({
      batchId: "ticket-batch-failed",
      body: {
        selected_cluster_id: "cluster-a",
        selected_storage_class: "rook-ceph",
        enable_override: true,
        comment: "approved",
      },
    });
    expect(approveMutate).not.toHaveBeenCalled();
  });

  it("requests compatible clusters using ticket payload and form overrides", async () => {
    const { result } = renderHook(() => useAdminApprovalsController({ t }));

    act(() => {
      result.current.openApproveModal({
        id: "ticket-1",
        operation_type: "CREATE",
        status: "PENDING",
        requester: "alice",
        ticket_payload: {
          namespace: "prod-a",
          template_id: "tpl-1",
          instance_size_id: "sz-1",
        },
      } as never);
    });

    const clusterQueryFn = findAdminClustersQueryFn("base");
    expect(clusterQueryFn).toBeTypeOf("function");

    await act(async () => {
      await clusterQueryFn();
    });

    expect(apiGetMock).toHaveBeenCalledWith("/admin/clusters", {
      params: {
        query: {
          include_incompatible: true,
          namespace: "prod-a",
          template_id: "tpl-1",
          instance_size_id: "sz-1",
          cpu_request: 1,
          cpu_limit: 2,
          memory_request_gi: 2,
          memory_limit_gi: 4,
        },
      },
    });
  });

  it("includes explicitly selected root volume mode in compatibility queries", async () => {
    watchedValues.selected_root_volume_mode_key = "Block|ReadWriteMany";
    watchedValues.selected_dv_access_modes = ["ReadWriteMany"];
    watchedValues.selected_dv_volume_mode = "Block";
    useApiGetMock.mockImplementation((queryKey: unknown[]) => {
      const key = Array.isArray(queryKey) ? queryKey[0] : undefined;
      if (key === "builtin-approval-tasks") {
        return {
          data: { items: [] },
          isLoading: false,
          refetch: vi.fn(),
        };
      }
      if (key === "admin-clusters") {
        return {
          data: {
            items: [
              {
                id: "cluster-a",
                name: "Cluster A",
                status: "HEALTHY",
                enabled: true,
                storage_classes: ["rook-ceph"],
                default_storage_class: "rook-ceph",
                compatibility: {
                  eligible: false,
                  root_volume_resolution: {
                    state: "mode_required",
                    message:
                      "storage class supports multiple root volume modes",
                    mode_options: [
                      {
                        access_modes: ["ReadWriteMany"],
                        volume_mode: "Block",
                      },
                    ],
                  },
                },
              },
            ],
          },
          isLoading: false,
        };
      }
      if (key === "admin-cluster-policy") {
        return {
          data: {
            id: "policy-1",
            cluster_id: "cluster-a",
            allow_cdi_clone: true,
            allowed_storage_classes: ["rook-ceph"],
          },
          isLoading: false,
        };
      }
      return {
        data: undefined,
        isLoading: false,
      };
    });

    const { result } = renderHook(() => useAdminApprovalsController({ t }));

    act(() => {
      result.current.openApproveModal({
        id: "ticket-1",
        operation_type: "CREATE",
        status: "PENDING",
        requester: "alice",
        ticket_payload: {
          namespace: "prod-a",
          template_id: "tpl-1",
          instance_size_id: "sz-1",
        },
      } as never);
    });

    const clusterQueryFn = findAdminClustersQueryFn("validated");
    expect(clusterQueryFn).toBeTypeOf("function");

    await act(async () => {
      await clusterQueryFn();
    });

    expect(apiGetMock).toHaveBeenCalledWith("/admin/clusters", {
      params: {
        query: {
          include_incompatible: true,
          namespace: "prod-a",
          template_id: "tpl-1",
          instance_size_id: "sz-1",
          selected_storage_class: "rook-ceph",
          selected_dv_access_modes: ["ReadWriteMany"],
          selected_dv_volume_mode: "Block",
          cpu_request: 1,
          cpu_limit: 2,
          memory_request_gi: 2,
          memory_limit_gi: 4,
        },
      },
    });
  });

  it("includes manually entered root volume mode in compatibility queries when StorageProfile is incomplete", async () => {
    watchedValues.selected_root_volume_mode_key = undefined;
    watchedValues.selected_storage_class = "block-sc";
    watchedValues.selected_dv_access_modes = ["ReadWriteOnce"];
    watchedValues.selected_dv_volume_mode = "Filesystem";
    useApiGetMock.mockImplementation((queryKey: unknown[]) => {
      const key = Array.isArray(queryKey) ? queryKey[0] : undefined;
      if (key === "builtin-approval-tasks") {
        return {
          data: { items: [] },
          isLoading: false,
          refetch: vi.fn(),
        };
      }
      if (key === "admin-clusters") {
        return {
          data: {
            items: [
              {
                id: "cluster-a",
                name: "Cluster A",
                status: "HEALTHY",
                enabled: true,
                storage_classes: ["block-sc"],
                default_storage_class: "block-sc",
                compatibility: {
                  eligible: false,
                  root_volume_resolution: {
                    state: "profile_incomplete",
                    message:
                      'storage class "block-sc" does not expose claimPropertySets in StorageProfile',
                  },
                },
              },
            ],
          },
          isLoading: false,
        };
      }
      if (key === "admin-cluster-policy") {
        return {
          data: {
            id: "policy-1",
            cluster_id: "cluster-a",
            allow_cdi_clone: true,
            allowed_storage_classes: ["block-sc"],
          },
          isLoading: false,
        };
      }
      return {
        data: undefined,
        isLoading: false,
      };
    });

    const { result } = renderHook(() => useAdminApprovalsController({ t }));

    act(() => {
      result.current.openApproveModal({
        id: "ticket-1",
        operation_type: "CREATE",
        status: "PENDING",
        requester: "alice",
        ticket_payload: {
          namespace: "prod-a",
          template_id: "tpl-1",
          instance_size_id: "sz-1",
        },
      } as never);
    });

    const clusterQueryFn = findAdminClustersQueryFn("validated");
    expect(clusterQueryFn).toBeTypeOf("function");

    await act(async () => {
      await clusterQueryFn();
    });

    expect(apiGetMock).toHaveBeenCalledWith("/admin/clusters", {
      params: {
        query: {
          include_incompatible: true,
          namespace: "prod-a",
          template_id: "tpl-1",
          instance_size_id: "sz-1",
          selected_storage_class: "block-sc",
          selected_dv_access_modes: ["ReadWriteOnce"],
          selected_dv_volume_mode: "Filesystem",
          cpu_request: 1,
          cpu_limit: 2,
          memory_request_gi: 2,
          memory_limit_gi: 4,
        },
      },
    });
  });

  it("auto-fills the remembered storage class when it is still available", async () => {
    watchedValues.selected_storage_class = undefined;
    approveFormState.resetFields.mockImplementation(() => {
      watchedValues.selected_cluster_id = "cluster-a";
      watchedValues.selected_storage_class = undefined;
      watchedValues.selected_root_volume_mode_key = undefined;
      watchedValues.selected_dv_access_modes = undefined;
      watchedValues.selected_dv_volume_mode = undefined;
    });

    saveRememberedApprovalClusterPlacement({
      clusterId: "cluster-a",
      selectedStorageClass: "gold-sc",
    });

    useApiGetMock.mockImplementation((queryKey: unknown[]) => {
      const key = Array.isArray(queryKey) ? queryKey[0] : undefined;
      if (key === "builtin-approval-tasks") {
        return {
          data: { items: [] },
          isLoading: false,
          refetch: vi.fn(),
        };
      }
      if (key === "admin-clusters") {
        return {
          data: {
            items: [
              {
                id: "cluster-a",
                name: "Cluster A",
                status: "HEALTHY",
                enabled: true,
                storage_classes: ["rook-ceph", "gold-sc"],
                default_storage_class: "rook-ceph",
                compatibility: {
                  eligible: true,
                },
              },
            ],
          },
          isLoading: false,
          error: undefined,
        };
      }
      if (key === "admin-cluster-policy") {
        return {
          data: {
            id: "policy-1",
            cluster_id: "cluster-a",
            allow_cdi_clone: true,
            allowed_storage_classes: ["rook-ceph", "gold-sc"],
          },
          isLoading: false,
        };
      }
      return {
        data: undefined,
        isLoading: false,
        error: undefined,
      };
    });

    const { result } = renderHook(() => useAdminApprovalsController({ t }));

    act(() => {
      result.current.openApproveModal({
        id: "ticket-1",
        operation_type: "CREATE",
        status: "PENDING",
        requester: "alice",
        ticket_payload: {
          namespace: "prod-a",
          template_id: "tpl-1",
          instance_size_id: "sz-1",
        },
      } as never);
    });

    await waitFor(() => {
      expect(approveFormState.setFieldsValue).toHaveBeenCalledWith({
        selected_storage_class: "gold-sc",
      });
    });
    expect(watchedValues.selected_storage_class).toBe("gold-sc");
  });

  it("skips remembered storage classes that are no longer available", async () => {
    watchedValues.selected_storage_class = undefined;
    approveFormState.resetFields.mockImplementation(() => {
      watchedValues.selected_cluster_id = "cluster-a";
      watchedValues.selected_storage_class = undefined;
      watchedValues.selected_root_volume_mode_key = undefined;
      watchedValues.selected_dv_access_modes = undefined;
      watchedValues.selected_dv_volume_mode = undefined;
    });

    saveRememberedApprovalClusterPlacement({
      clusterId: "cluster-a",
      selectedStorageClass: "gold-sc",
    });

    const { result } = renderHook(() => useAdminApprovalsController({ t }));

    act(() => {
      result.current.openApproveModal({
        id: "ticket-1",
        operation_type: "CREATE",
        status: "PENDING",
        requester: "alice",
        ticket_payload: {
          namespace: "prod-a",
          template_id: "tpl-1",
          instance_size_id: "sz-1",
        },
      } as never);
    });

    await act(async () => {});

    expect(watchedValues.selected_storage_class).toBeUndefined();
    expect(approveFormState.setFieldsValue).not.toHaveBeenCalledWith({
      selected_storage_class: "gold-sc",
    });
  });

  it("auto-fills remembered manual root volume mode for the same cluster and storage class", async () => {
    watchedValues.selected_storage_class = undefined;
    watchedValues.selected_root_volume_mode_key = undefined;
    watchedValues.selected_dv_access_modes = undefined;
    watchedValues.selected_dv_volume_mode = undefined;
    approveFormState.resetFields.mockImplementation(() => {
      watchedValues.selected_cluster_id = "cluster-a";
      watchedValues.selected_storage_class = undefined;
      watchedValues.selected_root_volume_mode_key = undefined;
      watchedValues.selected_dv_access_modes = undefined;
      watchedValues.selected_dv_volume_mode = undefined;
    });

    saveRememberedApprovalClusterPlacement({
      clusterId: "cluster-a",
      selectedStorageClass: "block-sc",
      selectedDVAccessModes: ["ReadWriteOnce"],
      selectedDVVolumeMode: "Block",
    });

    useApiGetMock.mockImplementation((queryKey: unknown[]) => {
      const key = Array.isArray(queryKey) ? queryKey[0] : undefined;
      if (key === "builtin-approval-tasks") {
        return {
          data: { items: [] },
          isLoading: false,
          refetch: vi.fn(),
        };
      }
      if (key === "admin-clusters") {
        return {
          data: {
            items: [
              {
                id: "cluster-a",
                name: "Cluster A",
                status: "HEALTHY",
                enabled: true,
                storage_classes: ["block-sc"],
                default_storage_class: "block-sc",
                compatibility: {
                  eligible: false,
                  root_volume_resolution: {
                    state: "profile_incomplete",
                    message:
                      'storage class "block-sc" does not expose claimPropertySets in StorageProfile',
                  },
                },
              },
            ],
          },
          isLoading: false,
          error: undefined,
        };
      }
      if (key === "admin-cluster-policy") {
        return {
          data: {
            id: "policy-1",
            cluster_id: "cluster-a",
            allow_cdi_clone: true,
            allowed_storage_classes: ["block-sc"],
          },
          isLoading: false,
        };
      }
      return {
        data: undefined,
        isLoading: false,
        error: undefined,
      };
    });

    const { result } = renderHook(() => useAdminApprovalsController({ t }));

    act(() => {
      result.current.openApproveModal({
        id: "ticket-1",
        operation_type: "CREATE",
        status: "PENDING",
        requester: "alice",
        ticket_payload: {
          namespace: "prod-a",
          template_id: "tpl-1",
          instance_size_id: "sz-1",
        },
      } as never);
    });

    await waitFor(() => {
      expect(watchedValues.selected_storage_class).toBe("block-sc");
      expect(watchedValues.selected_dv_access_modes).toEqual(["ReadWriteOnce"]);
      expect(watchedValues.selected_dv_volume_mode).toBe("Block");
    });
  });

  it("keeps explicitly selected root volume mode fields once compatibility resolves", () => {
    watchedValues.selected_root_volume_mode_key = "Block|ReadWriteMany";
    watchedValues.selected_dv_access_modes = ["ReadWriteMany"];
    watchedValues.selected_dv_volume_mode = "Block";

    useApiGetMock.mockImplementation((queryKey: unknown[]) => {
      const key = Array.isArray(queryKey) ? queryKey[0] : undefined;
      if (key === "builtin-approval-tasks") {
        return {
          data: { items: [] },
          isLoading: false,
          refetch: vi.fn(),
        };
      }
      if (key === "admin-clusters") {
        return {
          data: {
            items: [
              {
                id: "cluster-a",
                name: "Cluster A",
                status: "HEALTHY",
                enabled: true,
                storage_classes: ["rook-ceph"],
                default_storage_class: "rook-ceph",
                compatibility: {
                  eligible: true,
                  root_volume_resolution: {
                    state: "resolved",
                    effective_access_modes: ["ReadWriteMany"],
                    effective_volume_mode: "Block",
                  },
                },
              },
            ],
          },
          isLoading: false,
        };
      }
      if (key === "admin-cluster-policy") {
        return {
          data: {
            id: "policy-1",
            cluster_id: "cluster-a",
            allow_cdi_clone: true,
            allowed_storage_classes: ["rook-ceph"],
          },
          isLoading: false,
        };
      }
      return {
        data: undefined,
        isLoading: false,
      };
    });

    const { result } = renderHook(() => useAdminApprovalsController({ t }));

    act(() => {
      result.current.openApproveModal({
        id: "ticket-1",
        operation_type: "CREATE",
        status: "PENDING",
        requester: "alice",
        ticket_payload: {
          namespace: "prod-a",
          template_id: "tpl-1",
          instance_size_id: "sz-1",
        },
      } as never);
    });

    expect(watchedValues.selected_root_volume_mode_key).toBe(
      "Block|ReadWriteMany",
    );
    expect(watchedValues.selected_dv_access_modes).toEqual(["ReadWriteMany"]);
    expect(watchedValues.selected_dv_volume_mode).toBe("Block");
  });

  it("keeps a newly selected root volume mode while resolved compatibility is refreshing", () => {
    watchedValues.selected_root_volume_mode_key = "Block|ReadWriteOnce";
    watchedValues.selected_dv_access_modes = ["ReadWriteOnce"];
    watchedValues.selected_dv_volume_mode = "Block";

    useApiGetMock.mockImplementation((queryKey: unknown[]) => {
      const key = Array.isArray(queryKey) ? queryKey[0] : undefined;
      if (key === "builtin-approval-tasks") {
        return {
          data: { items: [] },
          isLoading: false,
          refetch: vi.fn(),
        };
      }
      if (key === "admin-clusters") {
        return {
          data: {
            items: [
              {
                id: "cluster-a",
                name: "Cluster A",
                status: "HEALTHY",
                enabled: true,
                storage_classes: ["rook-ceph"],
                default_storage_class: "rook-ceph",
                compatibility: {
                  eligible: true,
                  root_volume_resolution: {
                    intent_mode: "auto",
                    state: "resolved",
                    effective_access_modes: ["ReadWriteMany"],
                    effective_volume_mode: "Block",
                    mode_options: [
                      {
                        access_modes: ["ReadWriteMany"],
                        volume_mode: "Block",
                      },
                      {
                        access_modes: ["ReadWriteOnce"],
                        volume_mode: "Block",
                      },
                    ],
                  },
                },
              },
            ],
          },
          isLoading: false,
        };
      }
      if (key === "admin-cluster-policy") {
        return {
          data: {
            id: "policy-1",
            cluster_id: "cluster-a",
            allow_cdi_clone: true,
            allowed_storage_classes: ["rook-ceph"],
          },
          isLoading: false,
        };
      }
      return {
        data: undefined,
        isLoading: false,
      };
    });

    renderHook(() => useAdminApprovalsController({ t }));

    expect(approveFormState.setFieldsValue).not.toHaveBeenCalledWith({
      selected_root_volume_mode_key: undefined,
      selected_dv_access_modes: undefined,
      selected_dv_volume_mode: undefined,
    });
    expect(watchedValues.selected_root_volume_mode_key).toBe(
      "Block|ReadWriteOnce",
    );
    expect(watchedValues.selected_dv_access_modes).toEqual(["ReadWriteOnce"]);
    expect(watchedValues.selected_dv_volume_mode).toBe("Block");
  });

  it("does not auto-select a root volume mode when auto intent resolves to multiple candidates", () => {
    watchedValues.selected_root_volume_mode_key = undefined;
    watchedValues.selected_dv_access_modes = undefined;
    watchedValues.selected_dv_volume_mode = undefined;

    useApiGetMock.mockImplementation((queryKey: unknown[]) => {
      if (!Array.isArray(queryKey)) {
        return { data: undefined, isLoading: false, error: undefined };
      }
      const [key, , stage] = queryKey;
      if (key === "builtin-approval-tasks") {
        return {
          data: { items: [] },
          isLoading: false,
          refetch: vi.fn(),
        };
      }
      if (key === "admin-clusters" && stage === "base") {
        return {
          data: {
            items: [
              {
                id: "cluster-a",
                name: "Cluster A",
                status: "HEALTHY",
                enabled: true,
                storage_classes: ["rook-ceph"],
                default_storage_class: "rook-ceph",
                compatibility: {
                  eligible: false,
                  root_volume_resolution: {
                    intent_mode: "auto",
                    state: "mode_required",
                    mode_options: [
                      {
                        access_modes: ["ReadWriteMany"],
                        volume_mode: "Block",
                      },
                      {
                        access_modes: ["ReadWriteOnce"],
                        volume_mode: "Block",
                      },
                    ],
                  },
                },
              },
            ],
          },
          isLoading: false,
          error: undefined,
        };
      }
      if (key === "admin-clusters" && stage === "resolved") {
        return {
          data: {
            items: [
              {
                id: "cluster-a",
                name: "Cluster A",
                status: "HEALTHY",
                enabled: true,
                storage_classes: ["rook-ceph"],
                default_storage_class: "rook-ceph",
                compatibility: {
                  eligible: false,
                  root_volume_resolution: {
                    intent_mode: "auto",
                    state: "mode_required",
                    effective_storage_class: "rook-ceph",
                    effective_access_modes: ["ReadWriteMany"],
                    effective_volume_mode: "Block",
                    mode_options: [
                      {
                        access_modes: ["ReadWriteMany"],
                        volume_mode: "Block",
                      },
                      {
                        access_modes: ["ReadWriteOnce"],
                        volume_mode: "Block",
                      },
                    ],
                  },
                },
              },
            ],
          },
          isLoading: false,
          error: undefined,
        };
      }
      if (key === "admin-cluster-policy") {
        return {
          data: {
            id: "policy-1",
            cluster_id: "cluster-a",
            allow_cdi_clone: true,
            allowed_storage_classes: ["rook-ceph"],
          },
          isLoading: false,
        };
      }
      return {
        data: undefined,
        isLoading: false,
        error: undefined,
      };
    });

    const { result } = renderHook(() => useAdminApprovalsController({ t }));

    expect(result.current.effectiveSelectedRootVolumeModeKey).toBe("");
    const validatedCall = useApiGetMock.mock.calls.find(
      (call) =>
        Array.isArray(call[0]) &&
        call[0][0] === "admin-clusters" &&
        call[0][2] === "validated",
    );
    expect(validatedCall?.[2]).toMatchObject({ enabled: false });
  });

  it("preserves base root volume mode options when resolved compatibility omits them", () => {
    watchedValues.selected_storage_class = "rook-ceph";
    watchedValues.selected_dv_access_modes = ["ReadWriteOnce"];
    watchedValues.selected_dv_volume_mode = "Block";

    useApiGetMock.mockImplementation((queryKey: unknown[]) => {
      if (!Array.isArray(queryKey)) {
        return { data: undefined, isLoading: false, error: undefined };
      }
      const [key, , stage] = queryKey;
      if (key === "builtin-approval-tasks") {
        return {
          data: { items: [] },
          isLoading: false,
          refetch: vi.fn(),
        };
      }
      if (key === "admin-clusters" && stage === "base") {
        return {
          data: {
            items: [
              {
                id: "cluster-a",
                name: "Cluster A",
                status: "HEALTHY",
                enabled: true,
                storage_classes: ["rook-ceph"],
                default_storage_class: "rook-ceph",
                compatibility: {
                  eligible: false,
                  root_volume_resolution: {
                    intent_mode: "auto",
                    state: "mode_required",
                    mode_options: [
                      {
                        access_modes: ["ReadWriteMany"],
                        volume_mode: "Block",
                      },
                      {
                        access_modes: ["ReadWriteOnce"],
                        volume_mode: "Block",
                      },
                    ],
                  },
                },
              },
            ],
          },
          isLoading: false,
          error: undefined,
        };
      }
      if (key === "admin-clusters" && stage === "resolved") {
        return {
          data: {
            items: [
              {
                id: "cluster-a",
                name: "Cluster A",
                status: "HEALTHY",
                enabled: true,
                storage_classes: ["rook-ceph"],
                default_storage_class: "rook-ceph",
                compatibility: {
                  eligible: true,
                  root_volume_resolution: {
                    intent_mode: "auto",
                    state: "resolved",
                    effective_storage_class: "rook-ceph",
                    effective_access_modes: ["ReadWriteOnce"],
                    effective_volume_mode: "Block",
                  },
                },
              },
            ],
          },
          isLoading: false,
          error: undefined,
        };
      }
      if (key === "admin-cluster-policy") {
        return {
          data: {
            id: "policy-1",
            cluster_id: "cluster-a",
            allow_cdi_clone: true,
            allowed_storage_classes: ["rook-ceph"],
          },
          isLoading: false,
        };
      }
      return {
        data: undefined,
        isLoading: false,
        error: undefined,
      };
    });

    const { result } = renderHook(() => useAdminApprovalsController({ t }));

    expect(result.current.selectedRootVolumeResolution?.state).toBe("resolved");
    expect(
      result.current.selectedRootVolumeResolution?.mode_options,
    ).toHaveLength(2);
  });

  it("does not rewrite root volume fields when resolution is already clear", () => {
    watchedValues.selected_root_volume_mode_key = undefined;
    watchedValues.selected_dv_access_modes = undefined;
    watchedValues.selected_dv_volume_mode = undefined;

    useApiGetMock.mockImplementation((queryKey: unknown[]) => {
      const key = Array.isArray(queryKey) ? queryKey[0] : undefined;
      if (key === "builtin-approval-tasks") {
        return {
          data: { items: [] },
          isLoading: false,
          refetch: vi.fn(),
        };
      }
      if (key === "admin-clusters") {
        return {
          data: {
            items: [
              {
                id: "cluster-a",
                name: "Cluster A",
                status: "HEALTHY",
                enabled: true,
                storage_classes: ["rook-ceph"],
                default_storage_class: "rook-ceph",
                compatibility: {
                  eligible: true,
                  root_volume_resolution: {
                    state: "resolved",
                    effective_access_modes: ["ReadWriteMany"],
                    effective_volume_mode: "Block",
                  },
                },
              },
            ],
          },
          isLoading: false,
        };
      }
      if (key === "admin-cluster-policy") {
        return {
          data: {
            id: "policy-1",
            cluster_id: "cluster-a",
            allow_cdi_clone: true,
            allowed_storage_classes: ["rook-ceph"],
          },
          isLoading: false,
        };
      }
      return {
        data: undefined,
        isLoading: false,
      };
    });

    renderHook(() => useAdminApprovalsController({ t }));

    expect(approveFormState.setFieldsValue).not.toHaveBeenCalledWith({
      selected_root_volume_mode_key: undefined,
      selected_dv_access_modes: undefined,
      selected_dv_volume_mode: undefined,
    });
  });

  it("auto-selects an allowed storage class for the chosen cluster", async () => {
    watchedValues.selected_storage_class = "";

    const { result } = renderHook(() => useAdminApprovalsController({ t }));

    act(() => {
      result.current.openApproveModal({
        id: "ticket-1",
        operation_type: "CREATE",
        status: "PENDING",
        requester: "alice",
        ticket_payload: {
          namespace: "prod-a",
          template_id: "tpl-1",
          instance_size_id: "sz-1",
        },
      } as never);
    });

    expect(result.current.selectedClusterStorageClassOptions).toEqual([
      "rook-ceph",
    ]);
    expect(result.current.effectiveSelectedStorageClass).toBe("rook-ceph");
    expect(approveFormState.setFieldValue).not.toHaveBeenCalledWith(
      "selected_storage_class",
      expect.any(String),
    );

    const resolvedClusterQueryFn = findAdminClustersQueryFn("resolved");
    expect(resolvedClusterQueryFn).toBeTypeOf("function");

    await act(async () => {
      await resolvedClusterQueryFn?.();
    });

    expect(apiGetMock).toHaveBeenCalledWith("/admin/clusters", {
      params: {
        query: {
          include_incompatible: true,
          namespace: "prod-a",
          template_id: "tpl-1",
          instance_size_id: "sz-1",
          selected_storage_class: "rook-ceph",
          cpu_request: 1,
          cpu_limit: 2,
          memory_request_gi: 2,
          memory_limit_gi: 4,
        },
      },
    });
  });

  it("prefills modify approval request review and enables override when current requests exceed target limits", () => {
    const { result } = renderHook(() => useAdminApprovalsController({ t }));

    act(() => {
      result.current.openApproveModal({
        id: "ticket-modify-1",
        operation_type: "MODIFY",
        status: "PENDING",
        requester: "alice",
        ticket_payload: {
          current_cpu_request: 8,
          current_memory_request_gi: 8,
          target_cpu_cores: 4,
          target_memory_gi: 4,
        },
      } as never);
    });

    expect(approveFormState.setFieldsValue).toHaveBeenCalledWith({
      enable_override: true,
      cpu_request: 4,
      memory_request_gi: 4,
    });
  });

  it("prefills modify memory request to target limit when hugepages are enabled", () => {
    const { result } = renderHook(() => useAdminApprovalsController({ t }));

    act(() => {
      result.current.openApproveModal({
        id: "ticket-modify-hugepages",
        operation_type: "MODIFY",
        status: "PENDING",
        requester: "alice",
        ticket_payload: {
          current_memory_gi: 8,
          current_memory_request_gi: 4,
          target_memory_gi: 16,
          hugepages_page_size: "2Mi",
        },
      } as never);
    });

    expect(approveFormState.setFieldsValue).toHaveBeenCalledWith({
      enable_override: true,
      cpu_request: undefined,
      memory_request_gi: 16,
    });
  });

  it("submits the derived storage class when the cluster has exactly one eligible option", async () => {
    watchedValues.selected_storage_class = "";
    approveFormState.validateFields.mockResolvedValue({
      selected_cluster_id: "cluster-a",
      selected_storage_class: undefined,
      enable_override: true,
      comment: "approved",
    });

    const { result } = renderHook(() => useAdminApprovalsController({ t }));

    act(() => {
      result.current.openApproveModal({
        id: "ticket-1",
        operation_type: "CREATE",
        status: "PENDING",
        requester: "alice",
        ticket_payload: {
          namespace: "prod-a",
          template_id: "tpl-1",
          instance_size_id: "sz-1",
        },
      } as never);
    });

    await act(async () => {
      await result.current.submitApprove();
    });

    expect(approveMutate).toHaveBeenCalledWith({
      ticketId: "ticket-1",
      body: {
        selected_cluster_id: "cluster-a",
        selected_storage_class: "rook-ceph",
        enable_override: true,
        comment: "approved",
      },
    });
  });

  it("submits the uniquely resolved root volume mode when approval does not require manual selection", async () => {
    watchedValues.selected_storage_class = "";
    watchedValues.selected_root_volume_mode_key = undefined;
    watchedValues.selected_dv_access_modes = undefined;
    watchedValues.selected_dv_volume_mode = undefined;
    approveFormState.validateFields.mockResolvedValue({
      selected_cluster_id: "cluster-a",
      selected_storage_class: undefined,
      selected_root_volume_mode_key: undefined,
      selected_dv_access_modes: undefined,
      selected_dv_volume_mode: undefined,
      enable_override: true,
      comment: "approved",
    });

    useApiGetMock.mockImplementation((queryKey: unknown[]) => {
      const key = Array.isArray(queryKey) ? queryKey[0] : undefined;
      if (key === "builtin-approval-tasks") {
        return {
          data: { items: [] },
          isLoading: false,
          refetch: vi.fn(),
          error: undefined,
        };
      }
      if (key === "admin-clusters") {
        return {
          data: {
            items: [
              {
                id: "cluster-a",
                name: "Cluster A",
                status: "HEALTHY",
                enabled: true,
                storage_classes: ["rook-ceph"],
                default_storage_class: "rook-ceph",
                compatibility: {
                  eligible: true,
                  root_volume_resolution: {
                    intent_mode: "auto",
                    state: "resolved",
                    effective_storage_class: "rook-ceph",
                    effective_access_modes: ["ReadWriteMany"],
                    effective_volume_mode: "Block",
                  },
                },
              },
            ],
          },
          isLoading: false,
          error: undefined,
        };
      }
      if (key === "admin-cluster-policy") {
        return {
          data: {
            id: "policy-1",
            cluster_id: "cluster-a",
            allow_cdi_clone: true,
            allowed_storage_classes: ["rook-ceph"],
          },
          isLoading: false,
        };
      }
      return {
        data: undefined,
        isLoading: false,
        error: undefined,
      };
    });

    const { result } = renderHook(() => useAdminApprovalsController({ t }));

    act(() => {
      result.current.openApproveModal({
        id: "ticket-1",
        operation_type: "CREATE",
        status: "PENDING",
        requester: "alice",
        ticket_payload: {
          namespace: "prod-a",
          template_id: "tpl-1",
          instance_size_id: "sz-1",
        },
      } as never);
    });

    await act(async () => {
      await result.current.submitApprove();
    });

    expect(approveMutate).toHaveBeenCalledWith({
      ticketId: "ticket-1",
      body: {
        selected_cluster_id: "cluster-a",
        selected_storage_class: "rook-ceph",
        selected_dv_access_modes: ["ReadWriteMany"],
        selected_dv_volume_mode: "Block",
        enable_override: true,
        comment: "approved",
      },
    });
  });

  it("does not auto-select a storage class when multiple eligible options exist", () => {
    watchedValues.selected_storage_class = "";
    approveFormState.resetFields.mockImplementation(() => {
      watchedValues.selected_cluster_id = "cluster-a";
      watchedValues.selected_storage_class = "";
    });
    useApiGetMock.mockImplementation((queryKey: unknown[]) => {
      const key = Array.isArray(queryKey) ? queryKey[0] : undefined;
      if (key === "builtin-approval-tasks") {
        return {
          data: { items: [] },
          isLoading: false,
          refetch: vi.fn(),
        };
      }
      if (key === "admin-clusters") {
        return {
          data: {
            items: [
              {
                id: "cluster-a",
                name: "Cluster A",
                status: "HEALTHY",
                enabled: true,
                storage_classes: ["rook-ceph", "gold-sc"],
                default_storage_class: "rook-ceph",
                compatibility: {
                  eligible: false,
                  reason_code: "CLUSTER_POLICY_STORAGE_CLASS_REQUIRED",
                  reason_message:
                    "cluster policy requires an explicit allowed storage class",
                },
              },
            ],
          },
          isLoading: false,
        };
      }
      if (key === "admin-cluster-policy") {
        return {
          data: {
            id: "policy-1",
            cluster_id: "cluster-a",
            allow_cdi_clone: true,
            allowed_storage_classes: ["rook-ceph", "gold-sc"],
          },
          isLoading: false,
        };
      }
      return {
        data: undefined,
        isLoading: false,
      };
    });

    const { result } = renderHook(() => useAdminApprovalsController({ t }));

    act(() => {
      result.current.openApproveModal({
        id: "ticket-1",
        operation_type: "CREATE",
        status: "PENDING",
        requester: "alice",
        ticket_payload: {
          namespace: "prod-a",
          template_id: "tpl-1",
          instance_size_id: "sz-1",
        },
      } as never);
    });

    expect(result.current.selectedClusterStorageClassOptions).toEqual([
      "rook-ceph",
      "gold-sc",
    ]);
    expect(result.current.effectiveSelectedStorageClass).toBe("");
    expect(approveFormState.setFieldValue).not.toHaveBeenCalledWith(
      "selected_storage_class",
      expect.any(String),
    );
  });

  it("derives create context from batch parent payload items", async () => {
    const { result } = renderHook(() => useAdminApprovalsController({ t }));

    act(() => {
      result.current.openApproveModal({
        id: "ticket-batch-1",
        operation_type: "CREATE",
        status: "PENDING",
        requester: "alice",
        ticket_payload: {
          items: [
            {
              namespace: "prod-a",
              template_id: "tpl-1",
              instance_size_id: "sz-1",
            },
            {
              namespace: "prod-a",
              template_id: "tpl-1",
              instance_size_id: "sz-1",
            },
          ],
        },
      } as never);
    });

    expect(result.current.approveCreateContext).toEqual({
      namespace: "prod-a",
      templateId: "tpl-1",
      instanceSizeId: "sz-1",
      batchItemCount: 2,
      hasMixedSelection: false,
    });

    const clusterQueryFn = findAdminClustersQueryFn("base");
    expect(clusterQueryFn).toBeTypeOf("function");

    await act(async () => {
      await clusterQueryFn();
    });

    expect(apiGetMock).toHaveBeenCalledWith("/admin/clusters", {
      params: {
        query: {
          include_incompatible: true,
          namespace: "prod-a",
          template_id: "tpl-1",
          instance_size_id: "sz-1",
          cpu_request: 1,
          cpu_limit: 2,
          memory_request_gi: 2,
          memory_limit_gi: 4,
        },
      },
    });
  });

  it("drops override fields but keeps comment when override is disabled", async () => {
    useWatchMock.mockImplementation((name: string) => {
      switch (name) {
        case "selected_storage_class":
          return "rook-ceph";
        case "enable_override":
          return false;
        default:
          return undefined;
      }
    });
    approveFormState.validateFields.mockResolvedValue({
      selected_cluster_id: "cluster-a",
      selected_storage_class: "rook-ceph",
      enable_override: false,
      cpu_request: 1,
      cpu_limit: 2,
      memory_request_gi: 2,
      memory_limit_gi: 4,
      disk_gb: 120,
      comment: "approved",
    });

    const { result } = renderHook(() => useAdminApprovalsController({ t }));

    act(() => {
      result.current.openApproveModal({
        id: "ticket-1",
        operation_type: "CREATE",
        status: "PENDING",
        requester: "alice",
      } as never);
    });

    await act(async () => {
      await result.current.submitApprove();
    });

    expect(approveMutate).toHaveBeenCalledWith({
      ticketId: "ticket-1",
      body: {
        selected_cluster_id: "cluster-a",
        selected_storage_class: "rook-ceph",
        enable_override: false,
        comment: "approved",
      },
    });
  });

  it("shows approval errors with longer retention for easier recording", () => {
    renderHook(() => useAdminApprovalsController({ t }));

    const approveMutationOptions = useApiMutationMock.mock.calls[0]?.[1] as
      | { onError?: (err: { message?: string }) => void }
      | undefined;
    expect(approveMutationOptions?.onError).toBeTypeOf("function");

    approveMutationOptions?.onError?.({
      message: "vm spec rejected by cluster",
    });

    expect(messageErrorMock).toHaveBeenCalledWith({
      content: "vm spec rejected by cluster",
      duration: 10,
    });
  });

  it("applies backend field errors to the approve form before showing a toast", () => {
    applyApiFieldErrorsMock.mockReturnValue(true);
    renderHook(() => useAdminApprovalsController({ t }));

    const approveMutationOptions = useApiMutationMock.mock.calls[0]?.[1] as
      | {
          onError?: (err: {
            field_errors?: Array<{
              field: string;
              code: string;
              message?: string;
            }>;
          }) => void;
        }
      | undefined;

    approveMutationOptions?.onError?.({
      field_errors: [
        {
          field: "selected_cluster_id",
          code: "REQUIRED",
          message: "selected cluster is required for create approval",
        },
      ],
    });

    expect(applyApiFieldErrorsMock).toHaveBeenCalledWith(approveFormState, {
      field_errors: [
        {
          field: "selected_cluster_id",
          code: "REQUIRED",
          message: "selected cluster is required for create approval",
        },
      ],
    });
    expect(messageErrorMock).not.toHaveBeenCalled();
  });

  it("applies backend field errors to retry review before showing a toast", () => {
    applyApiFieldErrorsMock.mockReturnValue(true);
    const { result } = renderHook(() => useAdminApprovalsController({ t }));

    act(() => {
      result.current.openBatchRetryReviewModal({
        id: "ticket-batch-failed",
        operation_type: "CREATE",
        status: "FAILED",
        requester: "alice",
      } as never);
    });

    const retryMutationOptions = useApiMutationMock.mock.calls[2]?.[1] as
      | {
          onError?: (err: {
            field_errors?: Array<{
              field: string;
              code: string;
              message?: string;
            }>;
          }) => void;
        }
      | undefined;

    retryMutationOptions?.onError?.({
      field_errors: [
        {
          field: "selected_cluster_id",
          code: "REQUIRED",
          message: "selected cluster is required for create approval",
        },
      ],
    });

    expect(applyApiFieldErrorsMock).toHaveBeenCalledWith(approveFormState, {
      field_errors: [
        {
          field: "selected_cluster_id",
          code: "REQUIRED",
          message: "selected cluster is required for create approval",
        },
      ],
    });
    expect(messageErrorMock).not.toHaveBeenCalled();
  });

  it("sorts approval tasks with pending items first, then newest items first within each status group", () => {
    useApiGetMock.mockImplementation((queryKey: unknown[]) => {
      const key = Array.isArray(queryKey) ? queryKey[0] : undefined;
      if (key === "builtin-approval-tasks") {
        return {
          data: {
            items: [
              {
                id: "ticket-approved-new",
                status: "APPROVED",
                operation_type: "CREATE",
                created_at: "2026-04-15T08:00:00Z",
              },
              {
                id: "ticket-pending-old",
                status: "PENDING",
                operation_type: "CREATE",
                created_at: "2026-04-14T08:00:00Z",
              },
              {
                id: "ticket-approved-old",
                status: "APPROVED",
                operation_type: "CREATE",
                created_at: "2026-04-13T08:00:00Z",
              },
              {
                id: "ticket-pending-new",
                status: "PENDING",
                operation_type: "CREATE",
                created_at: "2026-04-15T09:00:00Z",
              },
            ],
            pagination: { page: 1, per_page: 20, total: 4, total_pages: 1 },
          },
          isLoading: false,
          refetch: vi.fn(),
        };
      }
      if (key === "admin-clusters") {
        return {
          data: { items: [] },
          isLoading: false,
          error: undefined,
        };
      }
      if (key === "admin-cluster-policy") {
        return {
          data: undefined,
          isLoading: false,
        };
      }
      return {
        data: undefined,
        isLoading: false,
        error: undefined,
      };
    });

    const { result } = renderHook(() => useAdminApprovalsController({ t }));

    expect(result.current.data?.items?.map((item) => item.id)).toEqual([
      "ticket-pending-new",
      "ticket-pending-old",
      "ticket-approved-new",
      "ticket-approved-old",
    ]);
  });
});
