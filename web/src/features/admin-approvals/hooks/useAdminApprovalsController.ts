"use client";

import { App, Form } from "antd";
import type { TFunction } from "i18next";
import { useEffect, useMemo, useRef, useState } from "react";

import { useApiAction, useApiGet, useApiMutation } from "@/hooks/useApiQuery";
import { api } from "@/lib/api/client";
import { translateApiError } from "@/lib/api/errorMessage";
import type { components } from "@/types/api.gen";

import type {
  ApprovalDecisionRequest,
  ApprovalStatus,
  ApprovalTask,
  Cluster,
  ApprovalTaskList,
  ClusterList,
  RejectDecisionRequest,
} from "../types";
import {
  loadRememberedApprovalClusterPlacement,
  saveRememberedApprovalClusterPlacement,
} from "../approvalPlacementMemory";

interface UseAdminApprovalsControllerArgs {
  t: TFunction;
}

interface ApprovalDecisionFormValues extends ApprovalDecisionRequest {
  selected_root_volume_mode_key?: string;
}

interface ApprovalCreateContext {
  namespace?: string;
  templateId?: string;
  instanceSizeId?: string;
  batchItemCount?: number;
  hasMixedSelection: boolean;
}

type ClusterPolicy = components["schemas"]["ClusterPolicy"];
const APPROVAL_ERROR_MESSAGE_DURATION_SECONDS = 10;

export function useAdminApprovalsController({
  t,
}: UseAdminApprovalsControllerArgs) {
  const { message: messageApi } = App.useApp();
  const messageContextHolder = null;
  const [statusFilter, setStatusFilter] = useState<"ALL" | ApprovalStatus>(
    "ALL",
  );
  const [searchFilter, setSearchFilter] = useState("");
  const [operationFilter, setOperationFilter] = useState<
    "ALL" | ApprovalTask["operation_type"]
  >("ALL");
  const [selectedClusterFilter, setSelectedClusterFilter] = useState("");
  const [placementAdvisoryFilter, setPlacementAdvisoryFilter] = useState("");
  const [placementSnapshotFilter, setPlacementSnapshotFilter] = useState<
    "ALL" | "present" | "missing"
  >("ALL");
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);
  const [approveModal, setApproveModal] = useState<ApprovalTask | null>(null);
  const [rejectModal, setRejectModal] = useState<ApprovalTask | null>(null);
  const rememberedStorageAutofillRef = useRef("");
  const rememberedRootModeAutofillRef = useRef("");
  const [approveForm] = Form.useForm<ApprovalDecisionFormValues>();
  const [rejectForm] = Form.useForm<RejectDecisionRequest>();
  const watchedSelectedClusterId = Form.useWatch(
    "selected_cluster_id",
    approveForm,
  );
  const watchedSelectedRootVolumeModeKey = Form.useWatch(
    "selected_root_volume_mode_key",
    approveForm,
  );
  const watchedSelectedDVAccessModes = Form.useWatch(
    "selected_dv_access_modes",
    approveForm,
  );
  const watchedSelectedDVVolumeMode = Form.useWatch(
    "selected_dv_volume_mode",
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
  const trimmedSearchFilter = searchFilter.trim();
  const trimmedSelectedClusterFilter = selectedClusterFilter.trim();
  const trimmedPlacementAdvisoryFilter = placementAdvisoryFilter.trim();

  const approvalListQuery = useApiGet<ApprovalTaskList>(
    [
      "builtin-approval-tasks",
      statusFilter,
      trimmedSearchFilter,
      operationFilter,
      trimmedSelectedClusterFilter,
      trimmedPlacementAdvisoryFilter,
      placementSnapshotFilter,
      page,
      pageSize,
    ],
    () =>
      api.GET("/builtin-approval/tasks", {
        params: {
          query: {
            ...(statusFilter !== "ALL" ? { status: statusFilter } : {}),
            ...(trimmedSearchFilter ? { search: trimmedSearchFilter } : {}),
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

  const filterClusterListQuery = useApiGet<ClusterList>(
    ["admin-clusters", "approval-filters"],
    () =>
      api.GET("/admin/clusters", {
        params: {
          query: {
            page: 1,
            per_page: 200,
          },
        },
      }),
    {
      retry: false,
    },
  );
  const sortedApprovalData = useMemo(
    () => sortApprovalTaskList(approvalListQuery.data),
    [approvalListQuery.data],
  );

  const isCreateTicket = approveModal?.operation_type === "CREATE";
  const approvePayload = approveModal?.ticket_payload as
    | Record<string, unknown>
    | undefined;
  const approveCreateContext = extractApprovalCreateContext(approvePayload);
  const baseCompatibilityQuery =
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
  const baseClusterListQuery = useApiGet<ClusterList>(
    ["admin-clusters", "approval-select", "base", baseCompatibilityQuery],
    () =>
      api.GET(
        "/admin/clusters",
        baseCompatibilityQuery
          ? { params: { query: baseCompatibilityQuery } }
          : undefined,
      ),
    {
      enabled: Boolean(approveModal) && isCreateTicket,
      retry: false,
    },
  );
  const selectedClusterId = normalizeOptionalString(watchedSelectedClusterId);
  const baseSelectedCluster = useMemo(
    () =>
      (baseClusterListQuery.data?.items ?? []).find(
        (cluster) => cluster.id === selectedClusterId,
      ),
    [baseClusterListQuery.data?.items, selectedClusterId],
  );
  const selectedClusterPolicyQuery = useApiGet<ClusterPolicy>(
    ["admin-cluster-policy", "approval-select", selectedClusterId],
    () =>
      api.GET("/admin/clusters/{cluster_id}/policy", {
        params: { path: { cluster_id: selectedClusterId as string } },
      }),
    {
      enabled:
        Boolean(approveModal) && isCreateTicket && selectedClusterId !== "",
      retry: false,
    },
  );
  const selectedClusterStorageClassOptions = useMemo(
    () =>
      buildApprovalStorageClassOptions(
        baseSelectedCluster,
        selectedClusterPolicyQuery.data,
      ),
    [baseSelectedCluster, selectedClusterPolicyQuery.data],
  );
  const explicitSelectedStorageClass = useMemo(() => {
    const currentValue = normalizeOptionalString(watchedStorageClass);
    if (!currentValue) {
      return "";
    }

    const matchedOption = selectedClusterStorageClassOptions.find(
      (option) =>
        normalizeStorageClassName(option) ===
        normalizeStorageClassName(currentValue),
    );
    return matchedOption ?? "";
  }, [selectedClusterStorageClassOptions, watchedStorageClass]);
  const effectiveSelectedStorageClass =
    explicitSelectedStorageClass ||
    (selectedClusterStorageClassOptions.length === 1
      ? selectedClusterStorageClassOptions[0]
      : "");
  const resolvedCompatibilityQuery =
    isCreateTicket &&
    approveModal &&
    selectedClusterId !== "" &&
    effectiveSelectedStorageClass !== ""
      ? {
          ...baseCompatibilityQuery,
          selected_storage_class: effectiveSelectedStorageClass,
        }
      : undefined;
  const resolvedClusterListQuery = useApiGet<ClusterList>(
    [
      "admin-clusters",
      "approval-select",
      "resolved",
      resolvedCompatibilityQuery,
    ],
    () =>
      api.GET(
        "/admin/clusters",
        resolvedCompatibilityQuery
          ? { params: { query: resolvedCompatibilityQuery } }
          : undefined,
      ),
    {
      enabled: resolvedCompatibilityQuery !== undefined,
      retry: false,
    },
  );
  const resolvedSelectedCluster = useMemo(
    () =>
      (resolvedClusterListQuery.data?.items ?? []).find(
        (cluster) => cluster.id === selectedClusterId,
      ),
    [resolvedClusterListQuery.data?.items, selectedClusterId],
  );
  const selectionSourceCluster = useMemo(
    () =>
      mergeClusterCompatibility(baseSelectedCluster, resolvedSelectedCluster),
    [baseSelectedCluster, resolvedSelectedCluster],
  );
  const selectionSourceRootVolumeResolution =
    selectionSourceCluster?.compatibility?.root_volume_resolution;
  const rootVolumeModeOptions = useMemo(
    () => selectionSourceRootVolumeResolution?.mode_options ?? [],
    [selectionSourceRootVolumeResolution?.mode_options],
  );
  const requiresManualRootVolumeModeInput =
    selectionSourceRootVolumeResolution?.state === "profile_incomplete";
  const explicitSelectedRootVolumeModeKey = normalizeOptionalString(
    watchedSelectedRootVolumeModeKey,
  );
  const explicitSelectedRootVolumeMode = useMemo(() => {
    if (!explicitSelectedRootVolumeModeKey) {
      return undefined;
    }
    return rootVolumeModeOptions.find(
      (option) =>
        rootVolumeModeOptionKey(option) === explicitSelectedRootVolumeModeKey,
    );
  }, [explicitSelectedRootVolumeModeKey, rootVolumeModeOptions]);
  const manualSelectedRootVolumeMode = useMemo(() => {
    if (!requiresManualRootVolumeModeInput) {
      return undefined;
    }

    const accessModes = normalizeStringArray(watchedSelectedDVAccessModes);
    const volumeMode = normalizeVolumeMode(watchedSelectedDVVolumeMode);
    if (accessModes.length === 0 || !volumeMode) {
      return undefined;
    }
    return {
      access_modes: accessModes,
      volume_mode: volumeMode,
    };
  }, [
    requiresManualRootVolumeModeInput,
    watchedSelectedDVAccessModes,
    watchedSelectedDVVolumeMode,
  ]);
  const resolvedRootVolumeMode = useMemo(() => {
    const accessModes = normalizeStringArray(
      selectionSourceRootVolumeResolution?.effective_access_modes,
    );
    const volumeMode = normalizeVolumeMode(
      selectionSourceRootVolumeResolution?.effective_volume_mode,
    );
    if (accessModes.length === 0 || !volumeMode) {
      return undefined;
    }
    return {
      access_modes: accessModes,
      volume_mode: volumeMode,
    };
  }, [
    selectionSourceRootVolumeResolution?.effective_access_modes,
    selectionSourceRootVolumeResolution?.effective_volume_mode,
  ]);
  const implicitResolvedRootVolumeMode =
    selectionSourceRootVolumeResolution?.intent_mode === "explicit" ||
    rootVolumeModeOptions.length === 0
      ? resolvedRootVolumeMode
      : undefined;
  const effectiveSelectedRootVolumeMode =
    explicitSelectedRootVolumeMode ||
    manualSelectedRootVolumeMode ||
    (rootVolumeModeOptions.length === 1
      ? rootVolumeModeOptions[0]
      : implicitResolvedRootVolumeMode);
  const effectiveSelectedRootVolumeModeKey = rootVolumeModeOptionKey(
    effectiveSelectedRootVolumeMode,
  );
  const effectiveSelectedDVAccessModes = normalizeStringArray(
    effectiveSelectedRootVolumeMode?.access_modes,
  );
  const effectiveSelectedDVVolumeMode = normalizeVolumeMode(
    effectiveSelectedRootVolumeMode?.volume_mode,
  );
  const canSelectRootVolumeMode = rootVolumeModeOptions.length > 1;
  const hasCompleteExplicitRootVolumeModeSelection =
    effectiveSelectedDVAccessModes.length > 0 &&
    effectiveSelectedDVVolumeMode !== undefined;
  const validationCompatibilityQuery =
    resolvedCompatibilityQuery && hasCompleteExplicitRootVolumeModeSelection
      ? {
          ...resolvedCompatibilityQuery,
          ...(effectiveSelectedDVAccessModes.length > 0
            ? { selected_dv_access_modes: effectiveSelectedDVAccessModes }
            : {}),
          ...(effectiveSelectedDVVolumeMode
            ? { selected_dv_volume_mode: effectiveSelectedDVVolumeMode }
            : {}),
        }
      : undefined;
  const validatedClusterListQuery = useApiGet<ClusterList>(
    [
      "admin-clusters",
      "approval-select",
      "validated",
      validationCompatibilityQuery,
    ],
    () =>
      api.GET(
        "/admin/clusters",
        validationCompatibilityQuery
          ? { params: { query: validationCompatibilityQuery } }
          : undefined,
      ),
    {
      enabled: validationCompatibilityQuery !== undefined,
      retry: false,
    },
  );
  const validatedSelectedCluster = useMemo(
    () =>
      (validatedClusterListQuery.data?.items ?? []).find(
        (cluster) => cluster.id === selectedClusterId,
      ),
    [validatedClusterListQuery.data?.items, selectedClusterId],
  );
  const clustersData = baseClusterListQuery.data;
  const selectedCluster = useMemo(
    () =>
      mergeClusterCompatibility(
        selectionSourceCluster,
        validatedSelectedCluster,
      ),
    [selectionSourceCluster, validatedSelectedCluster],
  );
  const selectedRootVolumeResolution =
    selectedCluster?.compatibility?.root_volume_resolution;

  useEffect(() => {
    if (!approveModal || !isCreateTicket || selectedClusterId === "") {
      return;
    }
    if (!baseSelectedCluster || selectedClusterPolicyQuery.isLoading) {
      return;
    }

    const autofillKey = `${approveModal.id}:${selectedClusterId}`;
    if (rememberedStorageAutofillRef.current === autofillKey) {
      return;
    }

    const currentStorageClass = normalizeOptionalString(
      approveForm.getFieldValue("selected_storage_class"),
    );
    if (currentStorageClass !== "") {
      rememberedStorageAutofillRef.current = autofillKey;
      return;
    }

    const remembered = loadRememberedApprovalClusterPlacement(selectedClusterId);
    const rememberedStorageClass = normalizeOptionalString(
      remembered?.selectedStorageClass,
    );
    if (!rememberedStorageClass) {
      rememberedStorageAutofillRef.current = autofillKey;
      return;
    }

    const matchedStorageClass = selectedClusterStorageClassOptions.find(
      (option) =>
        normalizeStorageClassName(option) ===
        normalizeStorageClassName(rememberedStorageClass),
    );
    if (!matchedStorageClass) {
      rememberedStorageAutofillRef.current = autofillKey;
      return;
    }

    approveForm.setFieldsValue({
      selected_storage_class: matchedStorageClass,
    });
    rememberedStorageAutofillRef.current = autofillKey;
  }, [
    approveForm,
    approveModal,
    baseSelectedCluster,
    isCreateTicket,
    selectedClusterId,
    selectedClusterPolicyQuery.isLoading,
    selectedClusterStorageClassOptions,
  ]);

  useEffect(() => {
    if (
      !approveModal ||
      !isCreateTicket ||
      selectedClusterId === "" ||
      effectiveSelectedStorageClass === ""
    ) {
      return;
    }
    if (!selectionSourceRootVolumeResolution && resolvedClusterListQuery.isLoading) {
      return;
    }

    const autofillKey = `${approveModal.id}:${selectedClusterId}:${normalizeStorageClassName(
      effectiveSelectedStorageClass,
    )}`;
    if (rememberedRootModeAutofillRef.current === autofillKey) {
      return;
    }

    const currentRootModeKey = normalizeOptionalString(
      approveForm.getFieldValue("selected_root_volume_mode_key"),
    );
    const currentAccessModes = normalizeStringArray(
      approveForm.getFieldValue("selected_dv_access_modes"),
    );
    const currentVolumeMode = normalizeVolumeMode(
      approveForm.getFieldValue("selected_dv_volume_mode"),
    );
    if (
      currentRootModeKey !== "" ||
      currentAccessModes.length > 0 ||
      currentVolumeMode !== undefined
    ) {
      rememberedRootModeAutofillRef.current = autofillKey;
      return;
    }

    const remembered = loadRememberedApprovalClusterPlacement(selectedClusterId);
    if (
      normalizeStorageClassName(remembered?.selectedStorageClass ?? "") !==
      normalizeStorageClassName(effectiveSelectedStorageClass)
    ) {
      rememberedRootModeAutofillRef.current = autofillKey;
      return;
    }

    const rememberedRootVolumeModeKey = normalizeOptionalString(
      remembered?.selectedRootVolumeModeKey,
    );
    if (canSelectRootVolumeMode && rememberedRootVolumeModeKey) {
      const matchedRootMode = rootVolumeModeOptions.find(
        (option) =>
          rootVolumeModeOptionKey(option) === rememberedRootVolumeModeKey,
      );
      if (matchedRootMode) {
        approveForm.setFieldsValue({
          selected_root_volume_mode_key: rememberedRootVolumeModeKey,
          selected_dv_access_modes: matchedRootMode.access_modes,
          selected_dv_volume_mode: matchedRootMode.volume_mode,
        });
      }
      rememberedRootModeAutofillRef.current = autofillKey;
      return;
    }

    if (!requiresManualRootVolumeModeInput) {
      rememberedRootModeAutofillRef.current = autofillKey;
      return;
    }

    const rememberedAccessModes = normalizeStringArray(
      remembered?.selectedDVAccessModes,
    );
    const rememberedVolumeMode = normalizeVolumeMode(
      remembered?.selectedDVVolumeMode,
    );
    if (rememberedAccessModes.length > 0 && rememberedVolumeMode) {
      approveForm.setFieldsValue({
        selected_dv_access_modes: rememberedAccessModes,
        selected_dv_volume_mode: rememberedVolumeMode,
      });
    }
    rememberedRootModeAutofillRef.current = autofillKey;
  }, [
    approveForm,
    approveModal,
    canSelectRootVolumeMode,
    effectiveSelectedStorageClass,
    isCreateTicket,
    requiresManualRootVolumeModeInput,
    resolvedClusterListQuery.isLoading,
    rootVolumeModeOptions,
    selectedClusterId,
    selectionSourceRootVolumeResolution,
  ]);

  useEffect(() => {
    if (
      !approveModal ||
      !isCreateTicket ||
      selectedClusterId === "" ||
      effectiveSelectedStorageClass === ""
    ) {
      return;
    }

    saveRememberedApprovalClusterPlacement({
      clusterId: selectedClusterId,
      selectedStorageClass: effectiveSelectedStorageClass,
      selectedRootVolumeModeKey:
        canSelectRootVolumeMode && effectiveSelectedRootVolumeModeKey !== ""
          ? effectiveSelectedRootVolumeModeKey
          : undefined,
      selectedDVAccessModes:
        requiresManualRootVolumeModeInput && effectiveSelectedDVAccessModes.length > 0
          ? effectiveSelectedDVAccessModes
          : undefined,
      selectedDVVolumeMode:
        requiresManualRootVolumeModeInput
          ? effectiveSelectedDVVolumeMode
          : undefined,
    });
  }, [
    approveModal,
    canSelectRootVolumeMode,
    effectiveSelectedDVAccessModes,
    effectiveSelectedDVVolumeMode,
    effectiveSelectedRootVolumeModeKey,
    effectiveSelectedStorageClass,
    isCreateTicket,
    requiresManualRootVolumeModeInput,
    selectedClusterId,
  ]);

  const showApprovalError = (
    error?:
      | Error
      | { code?: string; message?: string; params?: Record<string, unknown> },
  ) => {
    messageApi.error({
      content: translateApiError(t, error),
      duration: APPROVAL_ERROR_MESSAGE_DURATION_SECONDS,
    });
  };

  const approveMutation = useApiMutation<
    { ticketId: string; body: ApprovalDecisionRequest },
    unknown
  >(
    ({ ticketId, body }) =>
      api.POST("/builtin-approval/tasks/{ticket_id}/approve", {
        params: { path: { ticket_id: ticketId } },
        body,
      }),
    {
      invalidateKeys: [["builtin-approval-tasks"], ["tickets"], ["vms"]],
      onSuccess: () => {
        messageApi.info(t("approve_submitted"));
        closeApproveModal();
      },
      onError: (err) => showApprovalError(err),
    },
  );

  const rejectMutation = useApiMutation<
    { ticketId: string; body: RejectDecisionRequest },
    unknown
  >(
    ({ ticketId, body }) =>
      api.POST("/builtin-approval/tasks/{ticket_id}/reject", {
        params: { path: { ticket_id: ticketId } },
        body,
      }),
    {
      invalidateKeys: [["builtin-approval-tasks"], ["tickets"]],
      onSuccess: () => {
        messageApi.success(t("reject_submitted"));
        closeRejectModal();
      },
      onError: (err) => showApprovalError(err),
    },
  );

  const cancelMutation = useApiAction<string>(
    (ticketId) =>
      api.POST("/tickets/{ticket_id}/cancel", {
        params: { path: { ticket_id: ticketId } },
      }),
    {
      invalidateKeys: [["builtin-approval-tasks"], ["tickets"]],
      onSuccess: () => messageApi.success(t("common:message.success")),
      onError: (err) => showApprovalError(err),
    },
  );

  const retryBatchMutation = useApiMutation<
    string,
    components["schemas"]["VMBatchActionResponse"]
  >(
    (batchId) =>
      api.POST("/vms/batch/{batch_id}/retry", {
        params: { path: { batch_id: batchId } },
      }),
    {
      onSuccess: (response) => {
        messageApi.success(
          t("vm:batch.retry_reactivated", {
            count: response?.affected_count ?? 0,
          }),
        );
        void approvalListQuery.refetch();
      },
      onError: (err) => showApprovalError(err),
    },
  );

  const changeStatusFilter = (value: "ALL" | ApprovalStatus) => {
    setStatusFilter(value);
    setPage(1);
  };

  const changeSearchFilter = (value: string) => {
    setSearchFilter(value);
    setPage(1);
  };

  const changeOperationFilter = (
    value: "ALL" | ApprovalTask["operation_type"],
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

  const resetFilters = () => {
    setStatusFilter("ALL");
    setSearchFilter("");
    setOperationFilter("ALL");
    setSelectedClusterFilter("");
    setPlacementAdvisoryFilter("");
    setPlacementSnapshotFilter("ALL");
    setPage(1);
  };

  const openApproveModal = (ticket: ApprovalTask) => {
    rememberedStorageAutofillRef.current = "";
    rememberedRootModeAutofillRef.current = "";
    setApproveModal(ticket);
    approveForm.resetFields();
    if (ticket.operation_type === "MODIFY") {
      const payload = asPayloadRecord(ticket.ticket_payload);
      const currentCPURequest = payloadNumber(payload?.current_cpu_request);
      const currentMemoryRequestGi = payloadNumber(
        payload?.current_memory_request_gi,
      );
      const targetCPULimit = payloadNumber(payload?.target_cpu_cores);
      const targetMemoryLimitGi = payloadNumber(payload?.target_memory_gi);
      const requestReviewRequired =
        (typeof currentCPURequest === "number" &&
          typeof targetCPULimit === "number" &&
          currentCPURequest > targetCPULimit) ||
        (typeof currentMemoryRequestGi === "number" &&
          typeof targetMemoryLimitGi === "number" &&
          currentMemoryRequestGi > targetMemoryLimitGi);
      approveForm.setFieldsValue({
        enable_override: requestReviewRequired,
        cpu_request: currentCPURequest,
        memory_request_gi: currentMemoryRequestGi,
      });
    }
  };

  const closeApproveModal = () => {
    rememberedStorageAutofillRef.current = "";
    rememberedRootModeAutofillRef.current = "";
    setApproveModal(null);
    approveForm.resetFields();
  };

  const openRejectModal = (ticket: ApprovalTask) => {
    setRejectModal(ticket);
  };

  const closeRejectModal = () => {
    setRejectModal(null);
    rejectForm.resetFields();
  };

  const handleSelectedClusterChange = () => {
    rememberedStorageAutofillRef.current = "";
    rememberedRootModeAutofillRef.current = "";
    const currentStorageClass = normalizeOptionalString(
      approveForm.getFieldValue("selected_storage_class"),
    );
    const currentModeKey = normalizeOptionalString(
      approveForm.getFieldValue("selected_root_volume_mode_key"),
    );
    const currentAccessModes = normalizeStringArray(
      approveForm.getFieldValue("selected_dv_access_modes"),
    );
    const currentVolumeMode = normalizeVolumeMode(
      approveForm.getFieldValue("selected_dv_volume_mode"),
    );

    if (
      currentStorageClass === "" &&
      currentModeKey === "" &&
      currentAccessModes.length === 0 &&
      currentVolumeMode === undefined
    ) {
      return;
    }

    approveForm.setFieldsValue({
      selected_storage_class: undefined,
      selected_root_volume_mode_key: undefined,
      selected_dv_access_modes: undefined,
      selected_dv_volume_mode: undefined,
    });
  };

  const handleSelectedStorageClassChange = () => {
    rememberedRootModeAutofillRef.current = "";
    const currentModeKey = normalizeOptionalString(
      approveForm.getFieldValue("selected_root_volume_mode_key"),
    );
    const currentAccessModes = normalizeStringArray(
      approveForm.getFieldValue("selected_dv_access_modes"),
    );
    const currentVolumeMode = normalizeVolumeMode(
      approveForm.getFieldValue("selected_dv_volume_mode"),
    );

    if (
      currentModeKey === "" &&
      currentAccessModes.length === 0 &&
      currentVolumeMode === undefined
    ) {
      return;
    }

    approveForm.setFieldsValue({
      selected_root_volume_mode_key: undefined,
      selected_dv_access_modes: undefined,
      selected_dv_volume_mode: undefined,
    });
  };

  const handleSelectedRootVolumeModeChange = (value: unknown) => {
    const nextModeKey = normalizeOptionalString(value);
    const matchedOption = rootVolumeModeOptions.find(
      (option) => rootVolumeModeOptionKey(option) === nextModeKey,
    );

    approveForm.setFields([
      {
        name: ["selected_dv_access_modes"],
        value: matchedOption?.access_modes,
      },
      {
        name: ["selected_dv_volume_mode"],
        value: matchedOption?.volume_mode,
      },
    ]);

    return nextModeKey || undefined;
  };

  const submitApprove = async () => {
    if (!approveModal) {
      return;
    }
    const values = await approveForm.validateFields();
    approveMutation.mutate({
      ticketId: approveModal.id,
      body: normalizeApprovalDecisionValues(values, {
        selected_storage_class: effectiveSelectedStorageClass || undefined,
        selected_dv_access_modes:
          effectiveSelectedDVAccessModes.length > 0
            ? effectiveSelectedDVAccessModes
            : undefined,
        selected_dv_volume_mode: effectiveSelectedDVVolumeMode,
      }),
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

  const submitBatchRetry = (batchId: string) => {
    retryBatchMutation.mutate(batchId);
  };

  return {
    messageContextHolder,
    statusFilter,
    changeStatusFilter,
    searchFilter,
    changeSearchFilter,
    operationFilter,
    changeOperationFilter,
    selectedClusterFilter,
    changeSelectedClusterFilter,
    placementAdvisoryFilter,
    changePlacementAdvisoryFilter,
    placementSnapshotFilter,
    changePlacementSnapshotFilter,
    resetFilters,
    page,
    pageSize,
    setPage,
    setPageSize,
    data: sortedApprovalData,
    isLoading: approvalListQuery.isLoading,
    listError: approvalListQuery.error,
    refetch: approvalListQuery.refetch,
    approveModal,
    rejectModal,
    approveForm,
    rejectForm,
    clustersData,
    filterClusters: filterClusterListQuery.data?.items ?? [],
    filterClustersLoading: filterClusterListQuery.isLoading,
    clusterQueryError:
      validatedClusterListQuery.error ??
      resolvedClusterListQuery.error ??
      baseClusterListQuery.error,
    clusterQueryLoading:
      baseClusterListQuery.isLoading ||
      resolvedClusterListQuery.isLoading ||
      validatedClusterListQuery.isLoading,
    selectedClusterId,
    selectedClusterPolicy: selectedClusterPolicyQuery.data,
    selectedClusterPolicyLoading: selectedClusterPolicyQuery.isLoading,
    selectedClusterStorageClassOptions,
    effectiveSelectedStorageClass,
    selectedCluster,
    selectedRootVolumeResolution,
    rootVolumeModeOptions,
    canSelectRootVolumeMode,
    requiresManualRootVolumeModeInput,
    effectiveSelectedRootVolumeMode,
    effectiveSelectedRootVolumeModeKey,
    approveCreateContext,
    handleSelectedClusterChange,
    handleSelectedStorageClassChange,
    handleSelectedRootVolumeModeChange,
    openApproveModal,
    closeApproveModal,
    openRejectModal,
    closeRejectModal,
    submitApprove,
    submitReject,
    submitCancel,
    submitBatchRetry,
    approvePending: approveMutation.isPending,
    rejectPending: rejectMutation.isPending,
    cancelPending: cancelMutation.isPending,
    retryBatchPending: retryBatchMutation.isPending,
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
  values: ApprovalDecisionFormValues,
  defaults: {
    selected_storage_class?: string;
    selected_dv_access_modes?: string[];
    selected_dv_volume_mode?: "Block" | "Filesystem";
  } = {},
): ApprovalDecisionRequest {
  const base: ApprovalDecisionRequest = {
    selected_cluster_id: values.selected_cluster_id,
    selected_storage_class:
      normalizeOptionalString(defaults.selected_storage_class) ||
      normalizeOptionalString(values.selected_storage_class) ||
      undefined,
    selected_dv_access_modes:
      values.selected_dv_access_modes &&
      values.selected_dv_access_modes.length > 0
        ? values.selected_dv_access_modes
        : defaults.selected_dv_access_modes,
    selected_dv_volume_mode:
      values.selected_dv_volume_mode ?? defaults.selected_dv_volume_mode,
    enable_override: values.enable_override,
    cpu_request: values.cpu_request,
    cpu_limit: values.cpu_limit,
    memory_request_gi: values.memory_request_gi,
    memory_limit_gi: values.memory_limit_gi,
    disk_gb: values.disk_gb,
    comment: values.comment,
  };

  if (!values.enable_override) {
    return Object.fromEntries(
      Object.entries({
        selected_cluster_id: base.selected_cluster_id,
        selected_storage_class: base.selected_storage_class,
        selected_dv_access_modes: base.selected_dv_access_modes,
        selected_dv_volume_mode: base.selected_dv_volume_mode,
        enable_override: false,
        comment: base.comment,
      }).filter(([, value]) => value !== undefined),
    ) as ApprovalDecisionRequest;
  }

  return Object.fromEntries(
    Object.entries(base).filter(([, value]) => value !== undefined),
  ) as ApprovalDecisionRequest;
}

function normalizeOptionalString(value: unknown): string {
  return typeof value === "string" ? value.trim() : "";
}

function asPayloadRecord(value: unknown): Record<string, unknown> | undefined {
  return typeof value === "object" && value !== null
    ? (value as Record<string, unknown>)
    : undefined;
}

function payloadNumber(value: unknown): number | undefined {
  return typeof value === "number" && Number.isFinite(value)
    ? value
    : undefined;
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

function mergeClusterCompatibility(
  base: Cluster | undefined,
  resolved: Cluster | undefined,
): Cluster | undefined {
  if (!base) {
    return resolved;
  }
  if (!resolved) {
    return base;
  }

  return {
    ...base,
    ...resolved,
    compatibility: mergeClusterCompatibilityDetails(
      base.compatibility,
      resolved.compatibility,
    ),
  };
}

function mergeClusterCompatibilityDetails(
  base: Cluster["compatibility"],
  resolved: Cluster["compatibility"],
): Cluster["compatibility"] {
  if (!base) {
    return resolved;
  }
  if (!resolved) {
    return base;
  }

  return {
    ...base,
    ...resolved,
    root_volume_resolution: mergeRootVolumeResolution(
      base.root_volume_resolution,
      resolved.root_volume_resolution,
    ),
  };
}

function mergeRootVolumeResolution(
  base: NonNullable<Cluster["compatibility"]>["root_volume_resolution"],
  resolved: NonNullable<Cluster["compatibility"]>["root_volume_resolution"],
): NonNullable<Cluster["compatibility"]>["root_volume_resolution"] {
  if (!base) {
    return resolved;
  }
  if (!resolved) {
    return base;
  }

  return {
    ...base,
    ...resolved,
    requested_access_modes:
      resolved.requested_access_modes &&
      resolved.requested_access_modes.length > 0
        ? resolved.requested_access_modes
        : base.requested_access_modes,
    effective_access_modes:
      resolved.effective_access_modes &&
      resolved.effective_access_modes.length > 0
        ? resolved.effective_access_modes
        : base.effective_access_modes,
    mode_options:
      resolved.mode_options && resolved.mode_options.length > 0
        ? resolved.mode_options
        : base.mode_options,
  };
}

function sortApprovalTaskList(
  data: ApprovalTaskList | undefined,
): ApprovalTaskList | undefined {
  if (!data?.items?.length) {
    return data;
  }

  return {
    ...data,
    items: [...data.items].sort(compareApprovalTasks),
  };
}

function compareApprovalTasks(left: ApprovalTask, right: ApprovalTask): number {
  const pendingDiff =
    approvalStatusSortRank(left.status) - approvalStatusSortRank(right.status);
  if (pendingDiff !== 0) {
    return pendingDiff;
  }

  const createdAtDiff =
    approvalCreatedAtSortValue(right.created_at) -
    approvalCreatedAtSortValue(left.created_at);
  if (createdAtDiff !== 0) {
    return createdAtDiff;
  }

  return (left.id ?? "").localeCompare(right.id ?? "");
}

function approvalStatusSortRank(
  status: ApprovalTask["status"] | undefined,
): number {
  return status === "PENDING" ? 0 : 1;
}

function approvalCreatedAtSortValue(createdAt: string | undefined): number {
  if (!createdAt) {
    return Number.NEGATIVE_INFINITY;
  }
  const parsed = Date.parse(createdAt);
  return Number.isNaN(parsed) ? Number.NEGATIVE_INFINITY : parsed;
}

function rootVolumeModeOptionKey(
  option:
    | {
        access_modes?: string[];
        volume_mode?: string;
      }
    | undefined,
): string {
  if (!option) {
    return "";
  }
  const accessModes = Array.isArray(option.access_modes)
    ? option.access_modes
        .map((value) => value.trim())
        .filter(Boolean)
        .sort()
    : [];
  const volumeMode =
    typeof option.volume_mode === "string" ? option.volume_mode.trim() : "";
  if (!volumeMode || accessModes.length === 0) {
    return "";
  }
  return `${volumeMode}|${accessModes.join(",")}`;
}

function normalizeVolumeMode(
  value: unknown,
): "Block" | "Filesystem" | undefined {
  if (typeof value !== "string") {
    return undefined;
  }
  const trimmed = value.trim();
  if (trimmed === "Block" || trimmed === "Filesystem") {
    return trimmed;
  }
  return undefined;
}

function normalizeStringArray(value: unknown): string[] {
  if (!Array.isArray(value)) {
    return [];
  }
  return value
    .filter((item): item is string => typeof item === "string")
    .map((item) => item.trim())
    .filter(Boolean)
    .sort();
}
