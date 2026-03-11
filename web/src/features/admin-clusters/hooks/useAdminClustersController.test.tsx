import { act, renderHook } from "@testing-library/react";
import type { TFunction } from "i18next";
import { beforeEach, describe, expect, it, vi } from "vitest";

const {
  useApiGetMock,
  useApiMutationMock,
  useFormMock,
  messageSuccessMock,
  messageErrorMock,
  apiGetMock,
  apiPutMock,
  createFormState,
  envFormState,
  policyFormState,
} = vi.hoisted(() => ({
  useApiGetMock: vi.fn(),
  useApiMutationMock: vi.fn(),
  useFormMock: vi.fn(),
  messageSuccessMock: vi.fn(),
  messageErrorMock: vi.fn(),
  apiGetMock: vi.fn(),
  apiPutMock: vi.fn(),
  createFormState: {
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

vi.mock("@/hooks/useApiQuery", () => ({
  useApiGet: (...args: unknown[]) => useApiGetMock(...args),
  useApiMutation: (...args: unknown[]) => useApiMutationMock(...args),
}));

vi.mock("@/lib/api/client", () => ({
  api: {
    GET: (...args: unknown[]) => apiGetMock(...args),
    PUT: (...args: unknown[]) => apiPutMock(...args),
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
      if (formCall === 2) return [envFormState];
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
});
