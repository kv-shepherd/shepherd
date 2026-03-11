"use client";

import { Form, message } from "antd";
import type { TFunction } from "i18next";
import { useEffect, useMemo, useState } from "react";

import { useApiAction, useApiGet, useApiMutation } from "@/hooks/useApiQuery";
import { api } from "@/lib/api/client";
import type { components } from "@/types/api.gen";

import type {
  ApprovalDecisionRequest,
  ApprovalStatus,
  ApprovalTicket,
  Cluster,
  ApprovalTicketList,
  ClusterList,
  RejectDecisionRequest,
} from "../types";

interface UseAdminApprovalsControllerArgs {
  t: TFunction;
}

interface ApprovalCreateContext {
  namespace?: string;
  templateId?: string;
  instanceSizeId?: string;
  batchItemCount?: number;
  hasMixedSelection: boolean;
}

type ClusterPolicy = components["schemas"]["ClusterPolicy"];

export function useAdminApprovalsController({
  t,
}: UseAdminApprovalsControllerArgs) {
  const [messageApi, messageContextHolder] = message.useMessage();
  const [statusFilter, setStatusFilter] = useState<"ALL" | ApprovalStatus>(
    "PENDING",
  );
  const [operationFilter, setOperationFilter] = useState<
    "ALL" | ApprovalTicket["operation_type"]
  >("ALL");
  const [selectedClusterFilter, setSelectedClusterFilter] = useState("");
  const [placementAdvisoryFilter, setPlacementAdvisoryFilter] = useState("");
  const [placementSnapshotFilter, setPlacementSnapshotFilter] = useState<
    "ALL" | "present" | "missing"
  >("ALL");
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);
  const [approveModal, setApproveModal] = useState<ApprovalTicket | null>(null);
  const [rejectModal, setRejectModal] = useState<ApprovalTicket | null>(null);
  const [approveForm] = Form.useForm<ApprovalDecisionRequest>();
  const [rejectForm] = Form.useForm<RejectDecisionRequest>();
  const watchedSelectedClusterId = Form.useWatch(
    "selected_cluster_id",
    approveForm,
  );
  const watchedStorageClass = Form.useWatch(
    "selected_storage_class",
    approveForm,
  );
  const watchedEnableOverride = Form.useWatch("enable_override", approveForm);
  const watchedCPURequest = Form.useWatch("cpu_request", approveForm);
  const watchedCPULimit = Form.useWatch("cpu_limit", approveForm);
  const watchedMemoryRequestGi = Form.useWatch(
    "memory_request_gi",
    approveForm,
  );
  const watchedMemoryLimitGi = Form.useWatch("memory_limit_gi", approveForm);
  const trimmedSelectedClusterFilter = selectedClusterFilter.trim();
  const trimmedPlacementAdvisoryFilter = placementAdvisoryFilter.trim();

  const approvalListQuery = useApiGet<ApprovalTicketList>(
    [
      "approvals",
      statusFilter,
      operationFilter,
      trimmedSelectedClusterFilter,
      trimmedPlacementAdvisoryFilter,
      placementSnapshotFilter,
      page,
      pageSize,
    ],
    () =>
      api.GET("/approvals", {
        params: {
          query: {
            ...(statusFilter !== "ALL" ? { status: statusFilter } : {}),
            ...(operationFilter !== "ALL"
              ? { operation_type: operationFilter }
              : {}),
            ...(trimmedSelectedClusterFilter
              ? { selected_cluster_id: trimmedSelectedClusterFilter }
              : {}),
            ...(trimmedPlacementAdvisoryFilter
              ? { placement_advisory_code: trimmedPlacementAdvisoryFilter }
              : {}),
            ...(placementSnapshotFilter !== "ALL"
              ? { placement_snapshot: placementSnapshotFilter }
              : {}),
            page,
            per_page: pageSize,
          },
        },
      }),
  );

  const isCreateTicket = approveModal?.operation_type === "CREATE";
  const approvePayload = approveModal?.ticket_payload as
    | Record<string, unknown>
    | undefined;
  const approveCreateContext = extractApprovalCreateContext(approvePayload);
  const compatibilityQuery =
    isCreateTicket && approveModal
      ? {
          include_incompatible: true,
          ...(approveCreateContext.namespace
            ? { namespace: approveCreateContext.namespace }
            : {}),
          ...(approveCreateContext.templateId
            ? { template_id: approveCreateContext.templateId }
            : {}),
          ...(approveCreateContext.instanceSizeId
            ? { instance_size_id: approveCreateContext.instanceSizeId }
            : {}),
          ...(typeof watchedStorageClass === "string" &&
          watchedStorageClass.trim()
            ? { selected_storage_class: watchedStorageClass.trim() }
            : {}),
          ...(watchedEnableOverride
            ? {
                ...(typeof watchedCPURequest === "number"
                  ? { cpu_request: watchedCPURequest }
                  : {}),
                ...(typeof watchedCPULimit === "number"
                  ? { cpu_limit: watchedCPULimit }
                  : {}),
                ...(typeof watchedMemoryRequestGi === "number"
                  ? { memory_request_gi: watchedMemoryRequestGi }
                  : {}),
                ...(typeof watchedMemoryLimitGi === "number"
                  ? { memory_limit_gi: watchedMemoryLimitGi }
                  : {}),
              }
            : {}),
        }
      : undefined;
  const clusterListQuery = useApiGet<ClusterList>(
    ["admin-clusters", "approval-select", compatibilityQuery],
    () =>
      api.GET(
        "/admin/clusters",
        compatibilityQuery
          ? { params: { query: compatibilityQuery } }
          : undefined,
    ),
    { enabled: Boolean(approveModal) && isCreateTicket },
  );
  const selectedClusterId = normalizeOptionalString(watchedSelectedClusterId);
  const selectedCluster = useMemo(
    () =>
      (clusterListQuery.data?.items ?? []).find(
        (cluster) => cluster.id === selectedClusterId,
      ),
    [clusterListQuery.data?.items, selectedClusterId],
  );
  const selectedClusterPolicyQuery = useApiGet<ClusterPolicy>(
    ["admin-cluster-policy", "approval-select", selectedClusterId],
    () =>
      api.GET("/admin/clusters/{cluster_id}/policy", {
        params: { path: { cluster_id: selectedClusterId as string } },
      }),
    {
      enabled: Boolean(approveModal) && isCreateTicket && selectedClusterId !== "",
      retry: false,
    },
  );
  const selectedClusterStorageClassOptions = useMemo(
    () =>
      buildApprovalStorageClassOptions(
        selectedCluster,
        selectedClusterPolicyQuery.data,
      ),
    [selectedCluster, selectedClusterPolicyQuery.data],
  );

  useEffect(() => {
    if (!approveModal || !isCreateTicket || !selectedClusterId) {
      return;
    }
    if (selectedClusterStorageClassOptions.length === 0) {
      return;
    }

    const currentValue = normalizeOptionalString(
      approveForm.getFieldValue("selected_storage_class"),
    );
    if (
      currentValue &&
      selectedClusterStorageClassOptions.some(
        (option) =>
          normalizeStorageClassName(option) ===
          normalizeStorageClassName(currentValue),
      )
    ) {
      return;
    }

    const preferredStorageClass = selectPreferredStorageClass(
      selectedCluster,
      selectedClusterStorageClassOptions,
    );
    if (preferredStorageClass) {
      approveForm.setFieldValue("selected_storage_class", preferredStorageClass);
    }
  }, [
    approveForm,
    approveModal,
    isCreateTicket,
    selectedCluster,
    selectedClusterId,
    selectedClusterStorageClassOptions,
  ]);

  const approveMutation = useApiMutation<
    { ticketId: string; body: ApprovalDecisionRequest },
    unknown
  >(
    ({ ticketId, body }) =>
      api.POST("/approvals/{ticket_id}/approve", {
        params: { path: { ticket_id: ticketId } },
        body,
      }),
    {
      invalidateKeys: [["approvals"], ["vms"]],
      onSuccess: () => {
        messageApi.success(t("common:message.success"));
        closeApproveModal();
      },
      onError: (err) =>
        messageApi.error(err.message || t("common:message.error")),
    },
  );

  const rejectMutation = useApiMutation<
    { ticketId: string; body: RejectDecisionRequest },
    unknown
  >(
    ({ ticketId, body }) =>
      api.POST("/approvals/{ticket_id}/reject", {
        params: { path: { ticket_id: ticketId } },
        body,
      }),
    {
      invalidateKeys: [["approvals"]],
      onSuccess: () => {
        messageApi.success(t("common:message.success"));
        closeRejectModal();
      },
      onError: (err) =>
        messageApi.error(err.message || t("common:message.error")),
    },
  );

  const cancelMutation = useApiAction<string>(
    (ticketId) =>
      api.POST("/approvals/{ticket_id}/cancel", {
        params: { path: { ticket_id: ticketId } },
      }),
    {
      invalidateKeys: [["approvals"]],
      onSuccess: () => messageApi.success(t("common:message.success")),
      onError: (err) =>
        messageApi.error(err.message || t("common:message.error")),
    },
  );

  const changeStatusFilter = (value: "ALL" | ApprovalStatus) => {
    setStatusFilter(value);
    setPage(1);
  };

  const changeOperationFilter = (
    value: "ALL" | ApprovalTicket["operation_type"],
  ) => {
    setOperationFilter(value);
    setPage(1);
  };

  const changeSelectedClusterFilter = (value: string) => {
    setSelectedClusterFilter(value);
    setPage(1);
  };

  const changePlacementAdvisoryFilter = (value: string) => {
    setPlacementAdvisoryFilter(value);
    setPage(1);
  };

  const changePlacementSnapshotFilter = (
    value: "ALL" | "present" | "missing",
  ) => {
    setPlacementSnapshotFilter(value);
    setPage(1);
  };

  const openApproveModal = (ticket: ApprovalTicket) => {
    setApproveModal(ticket);
  };

  const closeApproveModal = () => {
    setApproveModal(null);
    approveForm.resetFields();
  };

  const openRejectModal = (ticket: ApprovalTicket) => {
    setRejectModal(ticket);
  };

  const closeRejectModal = () => {
    setRejectModal(null);
    rejectForm.resetFields();
  };

  const submitApprove = async () => {
    if (!approveModal) {
      return;
    }
    const values = await approveForm.validateFields();
    approveMutation.mutate({
      ticketId: approveModal.id,
      body: normalizeApprovalDecisionValues(values),
    });
  };

  const submitReject = async () => {
    if (!rejectModal) {
      return;
    }
    const values = await rejectForm.validateFields();
    rejectMutation.mutate({ ticketId: rejectModal.id, body: values });
  };

  const submitCancel = (ticketId: string) => {
    cancelMutation.mutate(ticketId);
  };

  return {
    messageContextHolder,
    statusFilter,
    changeStatusFilter,
    operationFilter,
    changeOperationFilter,
    selectedClusterFilter,
    changeSelectedClusterFilter,
    placementAdvisoryFilter,
    changePlacementAdvisoryFilter,
    placementSnapshotFilter,
    changePlacementSnapshotFilter,
    page,
    pageSize,
    setPage,
    setPageSize,
    data: approvalListQuery.data,
    isLoading: approvalListQuery.isLoading,
    refetch: approvalListQuery.refetch,
    approveModal,
    rejectModal,
    approveForm,
    rejectForm,
    clustersData: clusterListQuery.data,
    selectedClusterId,
    selectedClusterPolicy: selectedClusterPolicyQuery.data,
    selectedClusterPolicyLoading: selectedClusterPolicyQuery.isLoading,
    selectedClusterStorageClassOptions,
    approveCreateContext,
    openApproveModal,
    closeApproveModal,
    openRejectModal,
    closeRejectModal,
    submitApprove,
    submitReject,
    submitCancel,
    approvePending: approveMutation.isPending,
    rejectPending: rejectMutation.isPending,
    cancelPending: cancelMutation.isPending,
  };
}

function readPayloadString(
  payload: Record<string, unknown> | undefined,
  key: string,
): string | undefined {
  if (!payload) {
    return undefined;
  }
  const value = payload[key];
  if (typeof value !== "string") {
    return undefined;
  }
  const trimmed = value.trim();
  return trimmed || undefined;
}

function extractApprovalCreateContext(
  payload: Record<string, unknown> | undefined,
): ApprovalCreateContext {
  if (!payload) {
    return { hasMixedSelection: false };
  }

  const directNamespace = readPayloadString(payload, "namespace");
  const directTemplateId = readPayloadString(payload, "template_id");
  const directInstanceSizeId = readPayloadString(payload, "instance_size_id");
  if (directNamespace || directTemplateId || directInstanceSizeId) {
    return {
      namespace: directNamespace,
      templateId: directTemplateId,
      instanceSizeId: directInstanceSizeId,
      hasMixedSelection: false,
    };
  }

  const items = Array.isArray(payload.items)
    ? payload.items.filter(
        (item): item is Record<string, unknown> =>
          typeof item === "object" && item !== null,
      )
    : [];
  if (items.length === 0) {
    return { hasMixedSelection: false };
  }

  const namespaces = collectDistinctPayloadStrings(items, "namespace");
  const templateIds = collectDistinctPayloadStrings(items, "template_id");
  const instanceSizeIds = collectDistinctPayloadStrings(
    items,
    "instance_size_id",
  );

  return {
    namespace: namespaces.length === 1 ? namespaces[0] : undefined,
    templateId: templateIds.length === 1 ? templateIds[0] : undefined,
    instanceSizeId:
      instanceSizeIds.length === 1 ? instanceSizeIds[0] : undefined,
    batchItemCount: items.length,
    hasMixedSelection:
      namespaces.length > 1 ||
      templateIds.length > 1 ||
      instanceSizeIds.length > 1,
  };
}

function collectDistinctPayloadStrings(
  items: Record<string, unknown>[],
  key: string,
): string[] {
  const values = new Set<string>();
  for (const item of items) {
    const value = readPayloadString(item, key);
    if (value) {
      values.add(value);
    }
  }
  return Array.from(values);
}

function normalizeApprovalDecisionValues(
  values: ApprovalDecisionRequest,
): ApprovalDecisionRequest {
  if (!values.enable_override) {
    return {
      selected_cluster_id: values.selected_cluster_id,
      selected_storage_class: values.selected_storage_class,
      enable_override: false,
      comment: values.comment,
    };
  }

  return values;
}

function normalizeOptionalString(value: unknown): string {
  return typeof value === "string" ? value.trim() : "";
}

function normalizeStorageClassName(value: string): string {
  return value.trim().toLowerCase();
}

function buildApprovalStorageClassOptions(
  cluster: Cluster | undefined,
  policy: ClusterPolicy | undefined,
): string[] {
  if (!cluster) {
    return [];
  }

  const detected = dedupeStorageClasses([
    cluster.default_storage_class,
    ...(cluster.storage_classes ?? []),
  ]);
  const allowed = dedupeStorageClasses(policy?.allowed_storage_classes ?? []);

  if (allowed.length === 0) {
    return detected;
  }

  const detectedSet = new Set(detected.map(normalizeStorageClassName));
  const intersection = allowed.filter((value) =>
    detectedSet.has(normalizeStorageClassName(value)),
  );
  if (intersection.length > 0) {
    return intersection;
  }
  return allowed;
}

function dedupeStorageClasses(values: Array<string | undefined>): string[] {
  const byNormalizedValue = new Map<string, string>();
  for (const rawValue of values) {
    const trimmed = typeof rawValue === "string" ? rawValue.trim() : "";
    if (trimmed === "") {
      continue;
    }
    const normalized = normalizeStorageClassName(trimmed);
    if (!byNormalizedValue.has(normalized)) {
      byNormalizedValue.set(normalized, trimmed);
    }
  }
  return Array.from(byNormalizedValue.values());
}

function selectPreferredStorageClass(
  cluster: Cluster | undefined,
  options: string[],
): string | undefined {
  if (!cluster || options.length === 0) {
    return options[0];
  }
  const defaultStorageClass = normalizeOptionalString(cluster.default_storage_class);
  if (defaultStorageClass !== "") {
    const preferred = options.find(
      (option) =>
        normalizeStorageClassName(option) ===
        normalizeStorageClassName(defaultStorageClass),
    );
    if (preferred) {
      return preferred;
    }
  }
  return options[0];
}
