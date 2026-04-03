import { act, renderHook } from "@testing-library/react";
import type { TFunction } from "i18next";
import { beforeEach, describe, expect, it, vi } from "vitest";

const {
  useApiGetMock,
  useApiMutationMock,
  useApiActionMock,
  useFormMock,
  messageSuccessMock,
  messageErrorMock,
  apiGetMock,
  apiPostMock,
  apiPutMock,
  apiPatchMock,
  apiDeleteMock,
  createFormState,
  editFormState,
  envFormState,
  policyFormState,
} = vi.hoisted(() => ({
  useApiGetMock: vi.fn(),
  useApiMutationMock: vi.fn(),
  useApiActionMock: vi.fn(),
  useFormMock: vi.fn(),
  messageSuccessMock: vi.fn(),
  messageErrorMock: vi.fn(),
  apiGetMock: vi.fn(),
  apiPostMock: vi.fn(),
  apiPutMock: vi.fn(),
  apiPatchMock: vi.fn(),
  apiDeleteMock: vi.fn(),
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
  envFormState: {
    validateFields: vi.fn(),
    resetFields: vi.fn(),
    setFieldsValue: vi.fn(),
  },
  policyFormState: {
    validateFields: vi.fn(),
    resetFields: vi.fn(),
    setFieldsValue: vi.fn(),
  },
}));

vi.mock("antd", () => ({
  App: {
    useApp: () => ({
      message: {
        success: messageSuccessMock,
        error: messageErrorMock,
      },
    }),
  },
  Form: {
    useForm: (...args: unknown[]) => useFormMock(...args),
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
    POST: (...args: unknown[]) => apiPostMock(...args),
    PUT: (...args: unknown[]) => apiPutMock(...args),
    PATCH: (...args: unknown[]) => apiPatchMock(...args),
    DELETE: (...args: unknown[]) => apiDeleteMock(...args),
  },
}));

import { useAdminClustersController } from "./useAdminClustersController";

describe("useAdminClustersController", () => {
  const t = ((key: string) => key) as unknown as TFunction;

  beforeEach(() => {
    vi.clearAllMocks();

    let formCall = 0;
    useFormMock.mockImplementation(() => {
      formCall += 1;
      if (formCall === 1) return [createFormState];
      if (formCall === 2) return [editFormState];
      if (formCall === 3) return [envFormState];
      return [policyFormState];
    });

    useApiGetMock.mockReturnValue({
      data: {
        items: [],
        pagination: { page: 1, per_page: 20, total: 0, total_pages: 0 },
      },
      isLoading: false,
      refetch: vi.fn(),
    });

    let mutationCall = 0;
    useApiMutationMock.mockImplementation(
      (mutationFn: (req: unknown) => Promise<unknown>) => {
        mutationCall += 1;
        return {
          mutate: vi.fn((req: unknown) => mutationFn(req)),
          mutateAsync: vi.fn((req: unknown) => mutationFn(req)),
          isPending: false,
          key: mutationCall,
        };
      },
    );

    useApiActionMock.mockImplementation(
      (actionFn: (req: unknown) => Promise<unknown>) => ({
        mutate: vi.fn((req: unknown) => actionFn(req)),
        mutateAsync: vi.fn((req: unknown) => actionFn(req)),
        isPending: false,
      }),
    );
  });

  it("loads cluster policy via GET when opening the policy modal", async () => {
    apiGetMock.mockResolvedValue({
      data: {
        id: "cp-1",
        cluster_id: "cl-1",
        allow_cpu_overcommit: false,
        allow_memory_overcommit: true,
        allow_dedicated_cpu: true,
        allow_gpu: false,
        allow_sriov: true,
        allow_hugepages: false,
        allowed_hugepages_sizes: ["2Mi"],
        allow_cdi_clone: true,
        allowed_clone_source_namespaces: ["golden-images"],
        allowed_storage_classes: ["rook-ceph-block"],
        created_by: "admin",
        created_at: "2026-03-10T00:00:00Z",
        updated_at: "2026-03-10T00:00:00Z",
      },
      response: { status: 200 },
    });

    const { result } = renderHook(() => useAdminClustersController({ t }));

    await act(async () => {
      await result.current.openPolicyModal({
        id: "cl-1",
        name: "cluster-a",
        display_name: "Cluster A",
      } as never);
    });

    expect(apiGetMock).toHaveBeenCalledWith(
      "/admin/clusters/{cluster_id}/policy",
      {
        params: { path: { cluster_id: "cl-1" } },
      },
    );
    expect(result.current.selectedClusterPolicyExists).toBe(true);
    expect(policyFormState.setFieldsValue).toHaveBeenCalledWith(
      expect.objectContaining({
        allow_cpu_overcommit: false,
        allow_gpu: false,
        allowed_storage_classes: ["rook-ceph-block"],
      }),
    );
  });

  it("uses default policy values when the cluster policy row does not exist yet", async () => {
    apiGetMock.mockResolvedValue({
      error: { code: "CLUSTER_POLICY_NOT_FOUND", message: "not found" },
      response: { status: 404 },
    });

    const { result } = renderHook(() => useAdminClustersController({ t }));

    await act(async () => {
      await result.current.openPolicyModal({
        id: "cl-404",
        name: "cluster-b",
      } as never);
    });

    expect(result.current.selectedClusterPolicyExists).toBe(false);
    expect(policyFormState.setFieldsValue).toHaveBeenCalledWith({
      allow_cpu_overcommit: true,
      allow_memory_overcommit: true,
      allow_dedicated_cpu: false,
      allow_gpu: false,
      allow_sriov: false,
      allow_hugepages: false,
      allowed_hugepages_sizes: [],
      allow_cdi_clone: true,
      allowed_clone_source_namespaces: [],
      allowed_storage_classes: [],
    });
    expect(messageErrorMock).not.toHaveBeenCalled();
  });

  it("submits policy updates via PUT using the selected cluster id", async () => {
    apiGetMock.mockResolvedValue({
      error: { code: "CLUSTER_POLICY_NOT_FOUND", message: "not found" },
      response: { status: 404 },
    });
    apiPutMock.mockResolvedValue({
      data: { id: "cp-1" },
      response: { ok: true, status: 200 },
    });
    policyFormState.validateFields.mockResolvedValue({
      allow_cpu_overcommit: false,
      allow_memory_overcommit: true,
      allow_dedicated_cpu: true,
      allow_gpu: false,
      allow_sriov: true,
      allow_hugepages: true,
      allowed_hugepages_sizes: ["2Mi"],
      allow_cdi_clone: true,
      allowed_clone_source_namespaces: ["golden-images"],
      allowed_storage_classes: ["rook-ceph-block"],
    });

    const { result } = renderHook(() => useAdminClustersController({ t }));

    await act(async () => {
      await result.current.openPolicyModal({
        id: "cl-1",
        name: "cluster-a",
      } as never);
      await result.current.submitPolicyUpdate();
    });

    expect(apiPutMock).toHaveBeenCalledWith(
      "/admin/clusters/{cluster_id}/policy",
      {
        params: { path: { cluster_id: "cl-1" } },
        body: {
          allow_cpu_overcommit: false,
          allow_memory_overcommit: true,
          allow_dedicated_cpu: true,
          allow_gpu: false,
          allow_sriov: true,
          allow_hugepages: true,
          allowed_hugepages_sizes: ["2Mi"],
          allow_cdi_clone: true,
          allowed_clone_source_namespaces: ["golden-images"],
          allowed_storage_classes: ["rook-ceph-block"],
        },
      },
    );
  });

  it("clears scoped hugepages and clone namespace lists when the capability is disabled", async () => {
    apiGetMock.mockResolvedValue({
      error: { code: "CLUSTER_POLICY_NOT_FOUND", message: "not found" },
      response: { status: 404 },
    });
    apiPutMock.mockResolvedValue({
      data: { id: "cp-1" },
      response: { ok: true, status: 200 },
    });
    policyFormState.validateFields.mockResolvedValue({
      allow_cpu_overcommit: true,
      allow_memory_overcommit: true,
      allow_dedicated_cpu: true,
      allow_gpu: true,
      allow_sriov: true,
      allow_hugepages: false,
      allowed_hugepages_sizes: ["2Mi"],
      allow_cdi_clone: false,
      allowed_clone_source_namespaces: ["golden-images"],
      allowed_storage_classes: ["rook-ceph-block"],
    });

    const { result } = renderHook(() => useAdminClustersController({ t }));

    await act(async () => {
      await result.current.openPolicyModal({
        id: "cl-2",
        name: "cluster-b",
      } as never);
      await result.current.submitPolicyUpdate();
    });

    expect(apiPutMock).toHaveBeenCalledWith(
      "/admin/clusters/{cluster_id}/policy",
      {
        params: { path: { cluster_id: "cl-2" } },
        body: {
          allow_cpu_overcommit: true,
          allow_memory_overcommit: true,
          allow_dedicated_cpu: true,
          allow_gpu: true,
          allow_sriov: true,
          allow_hugepages: false,
          allowed_hugepages_sizes: [],
          allow_cdi_clone: false,
          allowed_clone_source_namespaces: [],
          allowed_storage_classes: ["rook-ceph-block"],
        },
      },
    );
  });

  it("submits cluster updates via PATCH and base64-encodes kubeconfig text", async () => {
    apiPatchMock.mockResolvedValue({
      data: { id: "cl-1" },
      response: { ok: true, status: 200 },
    });
    editFormState.validateFields.mockResolvedValue({
      name: "cluster-a",
      display_name: "Cluster A",
      environment: "prod",
      enabled: true,
      kubeconfig_text:
        "apiVersion: v1\nkind: Config\nclusters:\n- name: c1\n  cluster:\n    server: https://cluster.example.com\n",
    });

    const { result } = renderHook(() => useAdminClustersController({ t }));

    await act(async () => {
      result.current.openEditModal({
        id: "cl-1",
        name: "cluster-a",
        display_name: "Cluster A",
        environment: "test",
        enabled: true,
      } as never);
      await result.current.submitEdit();
    });

    expect(apiPatchMock).toHaveBeenCalledWith(
      "/admin/clusters/{cluster_id}",
      {
        params: { path: { cluster_id: "cl-1" } },
        body: expect.objectContaining({
          display_name: "Cluster A",
          environment: "prod",
          enabled: true,
          kubeconfig:
            "YXBpVmVyc2lvbjogdjEKa2luZDogQ29uZmlnCmNsdXN0ZXJzOgotIG5hbWU6IGMxCiAgY2x1c3RlcjoKICAgIHNlcnZlcjogaHR0cHM6Ly9jbHVzdGVyLmV4YW1wbGUuY29t",
        }),
      },
    );
  });

  it("hydrates the edit form from the selected cluster after opening the modal", async () => {
    const { result } = renderHook(() => useAdminClustersController({ t }));

    await act(async () => {
      result.current.openEditModal({
        id: "cl-2",
        name: "cluster-b",
        display_name: "Cluster B",
        environment: "prod",
        enabled: false,
      } as never);
    });

    expect(result.current.editingCluster?.name).toBe("cluster-b");
    expect(result.current.editingClusterName).toBe("Cluster B");
    expect(result.current.editOpen).toBe(true);
  });

  it("deletes clusters via DELETE action", async () => {
    apiDeleteMock.mockResolvedValue({
      response: { ok: true, status: 204 },
    });

    const { result } = renderHook(() => useAdminClustersController({ t }));

    await act(async () => {
      await result.current.deleteCluster("cl-9");
    });

    expect(apiDeleteMock).toHaveBeenCalledWith(
      "/admin/clusters/{cluster_id}",
      {
        params: { path: { cluster_id: "cl-9" } },
      },
    );
  });
});
