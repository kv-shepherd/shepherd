"use client";

import { App, Form } from "antd";
import type { TFunction } from "i18next";
import { useEffect, useMemo, useRef, useState } from "react";

import type { ApiErrorResponse } from "@/hooks/useApiQuery";
import { useApiAction, useApiGet, useApiMutation } from "@/hooks/useApiQuery";
import { api } from "@/lib/api/client";
import { translateApiError } from "@/lib/api/errorMessage";
import {
  canManageBatchOperation,
  createBatchActionFeedback,
  extractBatchRetryAfterSeconds,
  extractRestartReconciliationNotice,
  isBatchConflictError,
  isRetryableBatchChild,
  type BatchActionFeedback,
  type BatchActionKind,
  type RestartReconciliationNotice,
} from "@/features/vm-management/batchActions";
import {
  clearStoredBatchRequestIntent,
  isSameStoredBatchRequestIntent,
  resolveStoredBatchRequestIntent,
  type StoredBatchRequestIntent,
} from "@/features/vm-management/batchRequestIntentStorage";
import {
  filterCompatibleInstanceSizes,
  templateInstanceSizeCompatible,
} from "@/features/catalog/systemLabels";
import {
  clearStoredActiveBatchState,
  readStoredActiveBatchState,
  saveStoredActiveBatchState,
  type ActiveBatchKind,
} from "@/lib/storage/activeBatchTracking";
import { useAuthStore } from "@/stores/auth";

import type {
  DeleteVMResponse,
  InstanceSize,
  InstanceSizeList,
  ServiceList,
  SystemList,
  Template,
  TemplateList,
  VMBatchActionResponse,
  VMBatchPowerAction,
  VMBatchPowerRequest,
  VMBatchStatusResponse,
  VMBatchSubmitRequest,
  VMBatchSubmitResponse,
  TicketResponse,
  VM,
  VMCreateRequest,
  VMFilterOptions,
  VMPlacementHint,
  VMRequestDraft,
  VMList,
  VMModifyContext,
  VMModifyRequest,
  VMRequestContext,
  VMRequestLaunchPrefill,
  VMRequestMode,
  VMRequestPrefill,
} from "../types";
import {
  clearVMRequestDraft,
  hasMeaningfulVMRequestDraft,
  loadVMRequestDraft,
  resolveVMRequestDraftOwner,
  saveVMRequestDraft,
  VM_REQUEST_DRAFT_CHANGED_EVENT,
} from "../draftStorage";

interface UseVMManagementControllerArgs {
  t: TFunction;
}

interface VMListFilters {
  search: string;
  namespace: string;
  status: "" | VM["status"];
  clusterId: string;
  systemId: string;
  serviceId: string;
  osName: string;
  ipAddress: string;
}

type VMCreateFormValues = VMCreateRequest & { batch_count?: number };
type VMModifyFormValues = VMModifyRequest;
type VMBatchSubmitMutationInput = {
  body: VMBatchSubmitRequest;
  intent: StoredBatchRequestIntent;
  submissionSequence: number;
};
type VMBatchPowerMutationInput = {
  body: VMBatchPowerRequest;
  intent: StoredBatchRequestIntent;
  submissionSequence: number;
};
type VMBatchActionMutationInput = {
  actorKey: string;
  batchID: string;
  targetTicketIDs: string[];
};

const TERMINAL_BATCH_STATUSES = new Set([
  "COMPLETED",
  "PARTIAL_SUCCESS",
  "FAILED",
  "CANCELLED",
]);
const REQUEST_TRACKED_BATCH_OPERATIONS = new Set([
  "CREATE",
  "MODIFY",
  "DELETE",
]);
const VM_DELETE_ALLOWED_STATUSES = new Set([
  "STOPPED",
  "FAILED",
  "NOT_FOUND",
  "UNKNOWN",
]);

const VM_REQUEST_DRAFT_SAVE_DEBOUNCE_MS = 400;

const parseBatchIDFromStatusURL = (
  statusURL: string,
  fallback: string,
): string => {
  const trimmed = statusURL.trim();
  if (trimmed === "") {
    return fallback;
  }
  const segments = trimmed.split("/").filter(Boolean);
  const candidate = segments.at(-1);
  return candidate && candidate.trim() !== "" ? candidate : fallback;
};

const normalizeRetryAfterSeconds = (value: unknown): number => {
  const n = Number(value);
  if (!Number.isFinite(n)) {
    return 0;
  }
  return Math.max(0, Math.ceil(n));
};

const normalizeActiveBatchKind = (value: unknown): ActiveBatchKind | "" =>
  value === "request" || value === "job" ? value : "";

const inferActiveBatchKindFromOperation = (
  operation: string | undefined,
): ActiveBatchKind | "" => {
  if (typeof operation !== "string" || operation.trim() === "") {
    return "";
  }
  return REQUEST_TRACKED_BATCH_OPERATIONS.has(operation) ? "request" : "job";
};

const normalizeDraftString = (value: unknown): string | undefined => {
  if (typeof value !== "string") {
    return undefined;
  }
  const trimmed = value.trim();
  return trimmed === "" ? undefined : trimmed;
};

const normalizeDraftBatchCount = (value: unknown): number => {
  const n = Number(value);
  if (!Number.isFinite(n) || n < 1) {
    return 1;
  }
  return Math.max(1, Math.floor(n));
};

const normalizeDraftWizardStep = (value: unknown): number => {
  const n = Number(value);
  if (!Number.isFinite(n) || n < 0) {
    return 0;
  }
  return Math.max(0, Math.min(4, Math.floor(n)));
};

const normalizeDraftRequestMode = (value: unknown): VMRequestMode =>
  value === "full" ? "full" : "guided";

const normalizeOptionalTargetNumber = (value: unknown): number | undefined => {
  const n = Number(value);
  if (!Number.isFinite(n) || n <= 0) {
    return undefined;
  }
  return n;
};

const normalizeCreateTargetOverride = (
  value: unknown,
  defaultValue: number | undefined,
): number | undefined => {
  const normalized = normalizeOptionalTargetNumber(value);
  if (normalized === undefined) {
    return undefined;
  }
  if (
    typeof defaultValue === "number" &&
    Number.isFinite(defaultValue) &&
    Number(normalized) === Number(defaultValue)
  ) {
    return undefined;
  }
  return normalized;
};

export function useVMManagementController({
  t,
}: UseVMManagementControllerArgs) {
  const { message: messageApi } = App.useApp();
  const messageContextHolder = null;
  const user = useAuthStore((state) => state.user);
  const currentActorKey = user?.id?.trim() ?? "";
  const [initialActiveBatchState] = useState(() =>
    readStoredActiveBatchState(currentActorKey),
  );
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);
  const [filters, setFilters] = useState<VMListFilters>({
    search: "",
    namespace: "",
    status: "",
    clusterId: "",
    systemId: "",
    serviceId: "",
    osName: "",
    ipAddress: "",
  });
  const [wizardOpen, setWizardOpen] = useState(false);
  const [wizardStep, setWizardStep] = useState(0);
  const [requestMode, setRequestMode] = useState<VMRequestMode>("guided");
  const [selectedSystemId, setSelectedSystemId] = useState("");
  const [savedDraft, setSavedDraft] = useState<VMRequestDraft | null>(null);
  const [selectedVMIDs, setSelectedVMIDs] = useState<string[]>([]);
  const [modifyOpen, setModifyOpen] = useState(false);
  const [modifyScope, setModifyScope] = useState<"single" | "batch">("single");
  const [modifyTargetVM, setModifyTargetVM] = useState<{
    id: string;
    name: string;
  } | null>(null);
  const [activeBatchActorKey, setActiveBatchActorKey] =
    useState(currentActorKey);
  const [activeBatchID, setActiveBatchID] = useState(
    initialActiveBatchState.batch_id,
  );
  const [activeBatchStatusURL, setActiveBatchStatusURL] = useState(
    initialActiveBatchState.status_url,
  );
  const [activeBatchKind, setActiveBatchKind] = useState<ActiveBatchKind | "">(
    normalizeActiveBatchKind(initialActiveBatchState.kind),
  );
  const [batchAutoPolling, setBatchAutoPolling] = useState(true);
  const [batchPollingIntervalMs, setBatchPollingIntervalMs] = useState(2000);
  const [batchRateLimit, setBatchRateLimit] = useState({
    untilMs: 0,
    contactAdmin: false,
  });
  const [nowMs, setNowMs] = useState(() => Date.now());
  const [lastBatchActionFeedback, setLastBatchActionFeedback] =
    useState<BatchActionFeedback | null>(null);
  const [restartReconciliationNotice, setRestartReconciliationNotice] =
    useState<RestartReconciliationNotice | null>(null);
  const nextBatchSubmissionSequenceRef = useRef(0);
  const trackedBatchSubmissionSequenceRef = useRef(0);
  const currentCreateIntentRef = useRef<StoredBatchRequestIntent | null>(null);
  const currentModifyIntentRef = useRef<StoredBatchRequestIntent | null>(null);
  const [form] = Form.useForm<VMCreateFormValues>();
  const [modifyForm] = Form.useForm<VMModifyFormValues>();
  const watchOptions = useMemo(
    () => ({ form, preserve: true as const }),
    [form],
  );
  const modifyWatchOptions = useMemo(
    () => ({ form: modifyForm, preserve: true as const }),
    [modifyForm],
  );
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [deletingVM, setDeletingVM] = useState<{
    id: string;
    name: string;
    environment?: string;
  } | null>(null);
  const [deleteConfirmName, setDeleteConfirmName] = useState("");

  const activeBatchOwnedByCurrentActor =
    currentActorKey !== "" && activeBatchActorKey === currentActorKey;
  const effectiveActiveBatchID = activeBatchOwnedByCurrentActor
    ? activeBatchID
    : "";
  const effectiveActiveBatchStatusURL = activeBatchOwnedByCurrentActor
    ? activeBatchStatusURL
    : "";
  const effectiveActiveBatchKind = activeBatchOwnedByCurrentActor
    ? activeBatchKind
    : "";

  const selectedTemplateId = Form.useWatch("template_id", watchOptions);
  const selectedSizeId = Form.useWatch("instance_size_id", watchOptions);
  const namespaceValue = Form.useWatch("namespace", watchOptions);
  const reasonValue = Form.useWatch("reason", watchOptions);
  const serviceIdValue = Form.useWatch("service_id", watchOptions);
  const batchCountValue = Form.useWatch("batch_count", watchOptions) ?? 1;
  const createTargetCPUValue = Form.useWatch("target_cpu_cores", watchOptions);
  const createTargetMemoryValue = Form.useWatch(
    "target_memory_gi",
    watchOptions,
  );
  const createTargetDiskValue = Form.useWatch("target_disk_gb", watchOptions);
  const modifyTargetCPUValue = Form.useWatch(
    "target_cpu_cores",
    modifyWatchOptions,
  );
  const modifyTargetMemoryValue = Form.useWatch(
    "target_memory_gi",
    modifyWatchOptions,
  );
  const modifyTargetDiskValue = Form.useWatch(
    "target_disk_gb",
    modifyWatchOptions,
  );
  const draftOwner = resolveVMRequestDraftOwner(user);

  const vmListQuery = useApiGet<VMList>(
    [
      "vms",
      page,
      pageSize,
      filters.search,
      filters.namespace,
      filters.status,
      filters.clusterId,
      filters.systemId,
      filters.serviceId,
      filters.osName,
      filters.ipAddress,
    ],
    () =>
      api.GET("/vms", {
        params: {
          query: {
            page,
            per_page: pageSize,
            ...(filters.search ? { search: filters.search } : {}),
            ...(filters.namespace ? { namespace: filters.namespace } : {}),
            ...(filters.status ? { status: filters.status } : {}),
            ...(filters.clusterId ? { cluster_id: filters.clusterId } : {}),
            ...(filters.systemId ? { system_id: filters.systemId } : {}),
            ...(filters.serviceId ? { service_id: filters.serviceId } : {}),
            ...(filters.osName ? { os_name: filters.osName } : {}),
            ...(filters.ipAddress ? { ip_address: filters.ipAddress } : {}),
          },
        },
      }),
  );

  const vmFilterOptionsQuery = useApiGet<VMFilterOptions>(
    ["vms", "filter-options"],
    () => api.GET("/vms/filter-options"),
  );

  const systemsQuery = useApiGet<SystemList>(
    ["systems", "vm-wizard"],
    () => api.GET("/systems", { params: { query: { per_page: 100 } } }),
    { enabled: wizardOpen },
  );

  const servicesQuery = useApiGet<ServiceList>(
    ["services", selectedSystemId, "vm-wizard"],
    () =>
      api.GET("/systems/{system_id}/services", {
        params: {
          path: { system_id: selectedSystemId },
          query: { per_page: 100 },
        },
      }),
    { enabled: wizardOpen && Boolean(selectedSystemId) },
  );

  const requestContextQuery = useApiGet<VMRequestContext>(
    ["vm-request-context"],
    () => api.GET("/vms/request-context"),
    { enabled: wizardOpen },
  );
  const trimmedNamespaceValue =
    typeof namespaceValue === "string" ? namespaceValue.trim() : "";

  // Backward-compatible fallback for environments where request-context is unavailable.
  const templatesFallbackQuery = useApiGet<TemplateList>(
    ["templates", "vm-wizard-fallback"],
    () => api.GET("/templates"),
    { enabled: wizardOpen && requestContextQuery.isError },
  );

  const instanceSizesFallbackQuery = useApiGet<InstanceSizeList>(
    ["instance-sizes", "vm-wizard-fallback"],
    () => api.GET("/instance-sizes"),
    { enabled: wizardOpen && requestContextQuery.isError },
  );

  const batchStatusQuery = useApiGet<VMBatchStatusResponse>(
    [
      "vm-batch",
      currentActorKey,
      effectiveActiveBatchID,
      effectiveActiveBatchStatusURL,
    ],
    () =>
      api.GET("/vms/batch/{batch_id}", {
        params: { path: { batch_id: effectiveActiveBatchID } },
      }),
    {
      enabled: Boolean(effectiveActiveBatchID),
      retry: 3,
      retryDelay: (attempt) => Math.min(1000 * 2 ** attempt, 30000),
      refetchInterval: (query) => {
        if (!batchAutoPolling) {
          return false;
        }
        const status = (query.state.data as VMBatchStatusResponse | undefined)
          ?.status;
        if (status && TERMINAL_BATCH_STATUSES.has(status)) {
          return false;
        }
        return batchPollingIntervalMs;
      },
    },
  );
  const modifyContextQuery = useApiGet<VMModifyContext>(
    ["vm-modify-context", modifyTargetVM?.id],
    () =>
      api.GET("/vms/{vm_id}/modify-context", {
        params: { path: { vm_id: modifyTargetVM?.id ?? "" } },
      }),
    {
      enabled:
        modifyOpen && modifyScope === "single" && Boolean(modifyTargetVM?.id),
    },
  );

  const selectedTemplate = useMemo(() => {
    const templates =
      requestContextQuery.data?.templates ??
      templatesFallbackQuery.data?.items ??
      [];
    return templates.find((item: Template) => item.id === selectedTemplateId);
  }, [
    selectedTemplateId,
    requestContextQuery.data,
    templatesFallbackQuery.data,
  ]);

  const selectedSize = useMemo(() => {
    const sizes =
      requestContextQuery.data?.instance_sizes ??
      instanceSizesFallbackQuery.data?.items ??
      [];
    return sizes.find((item: InstanceSize) => item.id === selectedSizeId);
  }, [
    selectedSizeId,
    requestContextQuery.data,
    instanceSizesFallbackQuery.data,
  ]);
  const selectedSystem = useMemo(
    () =>
      systemsQuery.data?.items?.find((item) => item.id === selectedSystemId),
    [selectedSystemId, systemsQuery.data],
  );
  const selectedService = useMemo(
    () => servicesQuery.data?.items?.find((item) => item.id === serviceIdValue),
    [serviceIdValue, servicesQuery.data],
  );

  const templatesData = useMemo<TemplateList | undefined>(() => {
    if (requestContextQuery.data) {
      return { items: requestContextQuery.data.templates ?? [] };
    }
    return templatesFallbackQuery.data;
  }, [requestContextQuery.data, templatesFallbackQuery.data]);

  const allSizesData = useMemo<InstanceSizeList | undefined>(() => {
    if (requestContextQuery.data) {
      return { items: requestContextQuery.data.instance_sizes ?? [] };
    }
    return instanceSizesFallbackQuery.data;
  }, [requestContextQuery.data, instanceSizesFallbackQuery.data]);

  const selectedSizeIsCompatible =
    !selectedTemplate ||
    !selectedSize ||
    templateInstanceSizeCompatible(
      selectedTemplate.system_labels,
      selectedSize.system_labels,
    );

  const sizesData = useMemo<InstanceSizeList | undefined>(() => {
    if (!allSizesData) {
      return undefined;
    }
    return {
      ...allSizesData,
      items: filterCompatibleInstanceSizes(
        allSizesData.items ?? [],
        selectedTemplate,
      ),
    };
  }, [allSizesData, selectedTemplate]);

  const placementHintQuery = useApiGet<VMRequestContext>(
    [
      "vm-request-context",
      "placement-hint",
      trimmedNamespaceValue,
      selectedTemplateId,
      selectedSizeId,
    ],
    () =>
      api.GET("/vms/request-context", {
        params: {
          query: {
            namespace: trimmedNamespaceValue,
            template_id: selectedTemplateId,
            instance_size_id: selectedSizeId,
          },
        },
      }),
    {
      enabled:
        wizardOpen &&
        !requestContextQuery.isError &&
        selectedSizeIsCompatible &&
        Boolean(trimmedNamespaceValue) &&
        Boolean(selectedTemplateId) &&
        Boolean(selectedSizeId),
    },
  );

  useEffect(() => {
    if (selectedTemplate && selectedSize && !selectedSizeIsCompatible) {
      form.setFieldValue("instance_size_id", undefined);
    }
  }, [form, selectedSize, selectedSizeIsCompatible, selectedTemplate]);

  useEffect(() => {
    if (activeBatchActorKey === currentActorKey) {
      return;
    }
    const stored = readStoredActiveBatchState(currentActorKey);
    setActiveBatchActorKey(currentActorKey);
    setActiveBatchID(stored.batch_id);
    setActiveBatchStatusURL(stored.status_url);
    setActiveBatchKind(normalizeActiveBatchKind(stored.kind));
    setBatchAutoPolling(true);
    setLastBatchActionFeedback(null);
    setRestartReconciliationNotice(null);
    setBatchRateLimit({ untilMs: 0, contactAdmin: false });
    setNowMs(Date.now());
    currentCreateIntentRef.current = null;
    currentModifyIntentRef.current = null;
    trackedBatchSubmissionSequenceRef.current = 0;
  }, [activeBatchActorKey, currentActorKey]);

  const resolvedActiveBatchKind = useMemo<ActiveBatchKind | "">(
    () =>
      effectiveActiveBatchKind ||
      inferActiveBatchKindFromOperation(batchStatusQuery.data?.operation),
    [effectiveActiveBatchKind, batchStatusQuery.data?.operation],
  );

  useEffect(() => {
    if (!activeBatchOwnedByCurrentActor) {
      return;
    }
    if (effectiveActiveBatchID.trim() === "") {
      clearStoredActiveBatchState();
      return;
    }
    saveStoredActiveBatchState({
      actor_id: currentActorKey,
      batch_id: effectiveActiveBatchID,
      status_url: effectiveActiveBatchStatusURL,
      kind: resolvedActiveBatchKind,
    });
  }, [
    activeBatchOwnedByCurrentActor,
    currentActorKey,
    effectiveActiveBatchID,
    effectiveActiveBatchStatusURL,
    resolvedActiveBatchKind,
  ]);

  useEffect(() => {
    setSavedDraft(loadVMRequestDraft(draftOwner));
  }, [draftOwner]);

  useEffect(() => {
    if (typeof window === "undefined" || draftOwner === "") {
      return;
    }

    const refreshDraft = () => {
      setSavedDraft(loadVMRequestDraft(draftOwner));
    };

    const onStorage = (event: StorageEvent) => {
      if (event.storageArea !== window.localStorage) {
        return;
      }
      refreshDraft();
    };

    window.addEventListener(VM_REQUEST_DRAFT_CHANGED_EVENT, refreshDraft);
    window.addEventListener("storage", onStorage);
    return () => {
      window.removeEventListener(VM_REQUEST_DRAFT_CHANGED_EVENT, refreshDraft);
      window.removeEventListener("storage", onStorage);
    };
  }, [draftOwner]);

  const draftSnapshot = useMemo<VMRequestDraft | null>(() => {
    if (!wizardOpen || draftOwner === "") {
      return null;
    }

    const draft: VMRequestDraft = {
      version: 1,
      systemId: normalizeDraftString(selectedSystemId),
      systemLabel: normalizeDraftString(selectedSystem?.name),
      serviceId: normalizeDraftString(serviceIdValue),
      serviceLabel: normalizeDraftString(selectedService?.name),
      templateId: normalizeDraftString(selectedTemplateId),
      templateLabel: normalizeDraftString(
        selectedTemplate?.display_name ?? selectedTemplate?.name,
      ),
      instanceSizeId: normalizeDraftString(selectedSizeId),
      instanceSizeLabel: normalizeDraftString(
        selectedSize?.display_name ?? selectedSize?.name,
      ),
      namespace: normalizeDraftString(namespaceValue),
      reason: normalizeDraftString(reasonValue),
      targetCpuCores: normalizeOptionalTargetNumber(createTargetCPUValue),
      targetMemoryGi: normalizeOptionalTargetNumber(createTargetMemoryValue),
      targetDiskGb: normalizeOptionalTargetNumber(createTargetDiskValue),
      batchCount: normalizeDraftBatchCount(batchCountValue),
      wizardStep: normalizeDraftWizardStep(wizardStep),
      requestMode,
      updatedAt: new Date().toISOString(),
    };

    return hasMeaningfulVMRequestDraft(draft) ? draft : null;
  }, [
    batchCountValue,
    createTargetCPUValue,
    createTargetDiskValue,
    createTargetMemoryValue,
    draftOwner,
    namespaceValue,
    reasonValue,
    selectedService?.name,
    selectedSize?.display_name,
    selectedSize?.name,
    selectedSizeId,
    selectedSystem?.name,
    selectedSystemId,
    selectedTemplate?.display_name,
    selectedTemplate?.name,
    selectedTemplateId,
    serviceIdValue,
    requestMode,
    wizardOpen,
    wizardStep,
  ]);

  useEffect(() => {
    if (!draftSnapshot || draftOwner === "") {
      return;
    }

    const timer = window.setTimeout(() => {
      saveVMRequestDraft(draftOwner, draftSnapshot);
      setSavedDraft(draftSnapshot);
    }, VM_REQUEST_DRAFT_SAVE_DEBOUNCE_MS);

    return () => {
      window.clearTimeout(timer);
    };
  }, [draftOwner, draftSnapshot]);

  useEffect(() => {
    if (batchRateLimit.untilMs <= Date.now()) {
      return;
    }
    const timer = window.setInterval(() => {
      const now = Date.now();
      setNowMs(now);
      setBatchRateLimit((current) =>
        now >= current.untilMs ? { untilMs: 0, contactAdmin: false } : current,
      );
    }, 1000);
    return () => window.clearInterval(timer);
  }, [batchRateLimit.untilMs]);

  const batchRetryAfterSeconds = Math.max(
    0,
    Math.ceil((batchRateLimit.untilMs - nowMs) / 1000),
  );
  const batchRateLimited = batchRetryAfterSeconds > 0;
  const batchRateLimitContactAdmin = batchRateLimit.contactAdmin;
  const warnBatchRateLimited = () => {
    messageApi.warning(
      t(
        batchRateLimitContactAdmin
          ? "batch.rate_limited_contact_admin"
          : "batch.rate_limited_wait",
        { seconds: batchRetryAfterSeconds },
      ),
    );
  };
  const selectedVMRecords = useMemo<VM[]>(
    () =>
      (vmListQuery.data?.items ?? []).filter((vm) =>
        selectedVMIDs.includes(vm.id),
      ),
    [selectedVMIDs, vmListQuery.data?.items],
  );

  const setBatchRateLimitCooldown = (seconds: number, contactAdmin = false) => {
    const normalized = normalizeRetryAfterSeconds(seconds);
    if (normalized <= 0) {
      return false;
    }
    const now = Date.now();
    setNowMs(now);
    const requestedUntilMs = now + normalized * 1000;
    setBatchRateLimit((current) => {
      const currentIsActive = current.untilMs > now;
      return {
        untilMs: Math.max(current.untilMs, requestedUntilMs),
        contactAdmin: (currentIsActive && current.contactAdmin) || contactAdmin,
      };
    });
    messageApi.warning(
      t(
        contactAdmin
          ? "batch.rate_limited_contact_admin"
          : "batch.rate_limited_wait",
        { seconds: normalized },
      ),
    );
    return true;
  };

  const trackBatchSubmission = (
    resp: VMBatchSubmitResponse,
    kind: ActiveBatchKind,
    submission: VMBatchSubmitMutationInput | VMBatchPowerMutationInput,
  ): boolean => {
    if (
      submission.intent.actorKey !== currentActorKey ||
      submission.submissionSequence <= trackedBatchSubmissionSequenceRef.current
    ) {
      return false;
    }
    trackedBatchSubmissionSequenceRef.current = submission.submissionSequence;
    const trackedBatchID = parseBatchIDFromStatusURL(
      resp.status_url,
      resp.batch_id,
    );
    setActiveBatchActorKey(currentActorKey);
    setActiveBatchID(trackedBatchID);
    setActiveBatchStatusURL(resp.status_url);
    setActiveBatchKind(kind);
    setBatchAutoPolling(true);
    setLastBatchActionFeedback(null);
    const intervalSeconds = normalizeRetryAfterSeconds(
      resp.retry_after_seconds,
    );
    setBatchPollingIntervalMs(
      intervalSeconds > 0 ? intervalSeconds * 1000 : 2000,
    );
    setNowMs(Date.now());
    return true;
  };

  const pickBatchActionTargets = (action: BatchActionKind): string[] => {
    const children = batchStatusQuery.data?.children ?? [];
    if (action === "retry") {
      return children
        .filter(isRetryableBatchChild)
        .map((child) => child.ticket_id);
    }
    return children
      .filter((child) => child.status === "PENDING")
      .map((child) => child.ticket_id);
  };

  const recordBatchActionFeedback = (
    action: BatchActionKind,
    response: VMBatchActionResponse,
    targetTicketIDs: readonly string[],
  ) => {
    const feedback = createBatchActionFeedback(
      action,
      response,
      targetTicketIDs,
    );
    setLastBatchActionFeedback(feedback);
    messageApi.success(
      t(action === "retry" ? "batch.retry_feedback" : "batch.cancel_feedback", {
        count: feedback.affectedCount,
        ids:
          feedback.affectedTicketIDs.join(", ") || t("batch.affected_ids_none"),
      }),
    );
  };

  const onBatchMutationRateLimit = (err: ApiErrorResponse): boolean => {
    if (err.code !== "BATCH_RATE_LIMITED") {
      return false;
    }
    return setBatchRateLimitCooldown(
      extractBatchRetryAfterSeconds(err),
      err.params?.contact_admin === true,
    );
  };

  const getStableBatchRequestIntent = (
    operationKey: string,
    payload: unknown,
  ) =>
    resolveStoredBatchRequestIntent({
      actorKey: currentActorKey,
      operationKey,
      payload,
    });

  const nextBatchSubmissionSequence = () => {
    nextBatchSubmissionSequenceRef.current += 1;
    return nextBatchSubmissionSequenceRef.current;
  };

  const batchSubmissionBelongsToCurrentActor = (
    submission:
      VMBatchSubmitMutationInput | VMBatchPowerMutationInput | undefined,
  ) => !submission || submission.intent.actorKey === currentActorKey;

  const captureRestartReconciliationNotice = (err: ApiErrorResponse) => {
    const notice = extractRestartReconciliationNotice(err);
    if (notice) {
      setRestartReconciliationNotice(notice);
    }
  };

  const applyDraftToWizard = (draft: VMRequestDraft) => {
    currentCreateIntentRef.current = null;
    setWizardOpen(true);
    setWizardStep(normalizeDraftWizardStep(draft.wizardStep));
    setRequestMode(normalizeDraftRequestMode(draft.requestMode));
    setSelectedSystemId(draft.systemId ?? "");
    form.resetFields();
    form.setFieldsValue({
      service_id: draft.serviceId,
      template_id: draft.templateId,
      instance_size_id: draft.instanceSizeId,
      namespace: draft.namespace,
      reason: draft.reason,
      target_cpu_cores: normalizeOptionalTargetNumber(draft.targetCpuCores),
      target_memory_gi: normalizeOptionalTargetNumber(draft.targetMemoryGi),
      target_disk_gb: normalizeOptionalTargetNumber(draft.targetDiskGb),
      batch_count: normalizeDraftBatchCount(draft.batchCount),
    });
  };

  const applyPrefillToWizard = (prefill?: VMRequestLaunchPrefill) => {
    currentCreateIntentRef.current = null;
    setWizardOpen(true);
    setWizardStep(0);
    setRequestMode(prefill?.requestMode ?? "guided");
    setSelectedSystemId(prefill?.systemId ?? "");
    form.resetFields();
    form.setFieldsValue({
      batch_count: normalizeDraftBatchCount(prefill?.batchCount),
      service_id: prefill?.serviceId,
      template_id: prefill?.templateId,
      instance_size_id: prefill?.instanceSizeId,
      namespace: prefill?.namespace,
      reason: prefill?.reason,
      target_cpu_cores: normalizeOptionalTargetNumber(prefill?.targetCpuCores),
      target_memory_gi: normalizeOptionalTargetNumber(prefill?.targetMemoryGi),
      target_disk_gb: normalizeOptionalTargetNumber(prefill?.targetDiskGb),
    });
  };

  const clearSavedDraft = () => {
    if (draftOwner === "") {
      return;
    }
    clearVMRequestDraft(draftOwner);
    setSavedDraft(null);
  };

  const createVMRequest = useApiMutation<VMCreateRequest, TicketResponse>(
    (req) => api.POST("/vms/request", { body: req }),
    {
      invalidateKeys: [["vms"], ["tickets"], ["builtin-approval-tasks"]],
      onSuccess: () => {
        clearSavedDraft();
        messageApi.success(t("request_submitted"));
        setWizardOpen(false);
        setWizardStep(0);
        setRequestMode("guided");
        setSelectedSystemId("");
        form.resetFields();
      },
      onError: (err) => messageApi.error(translateApiError(t, err)),
    },
  );

  const createVMModifyRequest = useApiMutation<
    { vmId: string; body: VMModifyRequest },
    TicketResponse
  >(
    ({ vmId, body }) =>
      api.POST("/vms/{vm_id}/modify-request", {
        params: { path: { vm_id: vmId } },
        body,
      }),
    {
      invalidateKeys: [["tickets"], ["builtin-approval-tasks"], ["vms"]],
      onSuccess: () => {
        messageApi.success(t("modify.request_submitted"));
        setModifyOpen(false);
        setModifyTargetVM(null);
        modifyForm.resetFields();
      },
      onError: (err) => messageApi.error(translateApiError(t, err)),
    },
  );

  const submitCreateBatch = useApiMutation<
    VMBatchSubmitMutationInput,
    VMBatchSubmitResponse
  >(({ body }) => api.POST("/vms/batch", { body }), {
    invalidateKeys: [
      ["vms"],
      ["tickets"],
      ["my-tickets"],
      ["builtin-approval-tasks"],
    ],
    onSuccess: (resp, submission) => {
      clearStoredBatchRequestIntent(submission.intent);
      if (!batchSubmissionBelongsToCurrentActor(submission)) {
        return;
      }
      trackBatchSubmission(resp, "request", submission);
      if (
        isSameStoredBatchRequestIntent(
          currentCreateIntentRef.current,
          submission.intent,
        )
      ) {
        currentCreateIntentRef.current = null;
        clearSavedDraft();
        setWizardOpen(false);
        setWizardStep(0);
        setRequestMode("guided");
        setSelectedSystemId("");
        form.resetFields();
      }
      messageApi.success(t("batch.request_submitted"));
    },
    onError: (err, submission) => {
      if (!batchSubmissionBelongsToCurrentActor(submission)) {
        return;
      }
      if (onBatchMutationRateLimit(err)) {
        return;
      }
      messageApi.error(translateApiError(t, err));
    },
  });

  const submitVMBatch = useApiMutation<
    VMBatchSubmitMutationInput,
    VMBatchSubmitResponse
  >(({ body }) => api.POST("/vms/batch", { body }), {
    invalidateKeys: [
      ["vms"],
      ["tickets"],
      ["my-tickets"],
      ["builtin-approval-tasks"],
    ],
    onSuccess: (resp, submission) => {
      clearStoredBatchRequestIntent(submission.intent);
      if (!batchSubmissionBelongsToCurrentActor(submission)) {
        return;
      }
      trackBatchSubmission(resp, "request", submission);
      if (
        submission.body.operation === "MODIFY" &&
        isSameStoredBatchRequestIntent(
          currentModifyIntentRef.current,
          submission.intent,
        )
      ) {
        currentModifyIntentRef.current = null;
        setModifyOpen(false);
        setModifyTargetVM(null);
        modifyForm.resetFields();
      }
      messageApi.success(t("batch.request_submitted"));
    },
    onError: (err, submission) => {
      if (!batchSubmissionBelongsToCurrentActor(submission)) {
        return;
      }
      captureRestartReconciliationNotice(err);
      if (onBatchMutationRateLimit(err)) {
        return;
      }
      messageApi.error(translateApiError(t, err));
    },
  });

  const submitVMBatchPower = useApiMutation<
    VMBatchPowerMutationInput,
    VMBatchSubmitResponse
  >(({ body }) => api.POST("/vms/batch/power", { body }), {
    invalidateKeys: [["vms"]],
    onSuccess: (resp, submission) => {
      clearStoredBatchRequestIntent(submission.intent);
      if (!batchSubmissionBelongsToCurrentActor(submission)) {
        return;
      }
      trackBatchSubmission(resp, "job", submission);
      messageApi.success(t("batch.job_submitted"));
    },
    onError: (err, submission) => {
      if (!batchSubmissionBelongsToCurrentActor(submission)) {
        return;
      }
      captureRestartReconciliationNotice(err);
      if (onBatchMutationRateLimit(err)) {
        return;
      }
      messageApi.error(translateApiError(t, err));
    },
  });

  const retryBatchMutation = useApiMutation<
    VMBatchActionMutationInput,
    VMBatchActionResponse
  >(
    (submission) =>
      api.POST("/vms/batch/{batch_id}/retry", {
        params: { path: { batch_id: submission.batchID } },
      }),
    {
      invalidateKeys: [
        [
          "vm-batch",
          currentActorKey,
          effectiveActiveBatchID,
          effectiveActiveBatchStatusURL,
        ],
        ["vm-batch-list"],
        ["vms"],
      ],
      onSuccess: (response, submission) => {
        if (submission && submission.actorKey !== currentActorKey) {
          return;
        }
        setBatchAutoPolling(true);
        recordBatchActionFeedback(
          "retry",
          response,
          submission?.targetTicketIDs ?? [],
        );
        void batchStatusQuery.refetch();
      },
      onError: (err, submission) => {
        if (submission && submission.actorKey !== currentActorKey) {
          return;
        }
        captureRestartReconciliationNotice(err);
        if (onBatchMutationRateLimit(err)) {
          return;
        }
        if (isBatchConflictError(err)) {
          void batchStatusQuery.refetch();
        }
        messageApi.error(translateApiError(t, err));
      },
    },
  );

  const cancelBatchMutation = useApiMutation<
    VMBatchActionMutationInput,
    VMBatchActionResponse
  >(
    (submission) =>
      api.POST("/vms/batch/{batch_id}/cancel", {
        params: { path: { batch_id: submission.batchID } },
      }),
    {
      invalidateKeys: [
        [
          "vm-batch",
          currentActorKey,
          effectiveActiveBatchID,
          effectiveActiveBatchStatusURL,
        ],
        ["vm-batch-list"],
        ["vms"],
      ],
      onSuccess: (response, submission) => {
        if (submission && submission.actorKey !== currentActorKey) {
          return;
        }
        setBatchAutoPolling(true);
        recordBatchActionFeedback(
          "cancel",
          response,
          submission?.targetTicketIDs ?? [],
        );
        void batchStatusQuery.refetch();
      },
      onError: (err, submission) => {
        if (submission && submission.actorKey !== currentActorKey) {
          return;
        }
        if (onBatchMutationRateLimit(err)) {
          return;
        }
        if (isBatchConflictError(err)) {
          void batchStatusQuery.refetch();
        }
        messageApi.error(translateApiError(t, err));
      },
    },
  );

  const startVM = useApiAction<string>(
    (vmId) =>
      api.POST("/vms/{vm_id}/start", { params: { path: { vm_id: vmId } } }),
    {
      invalidateKeys: [["vms"]],
      onSuccess: () => messageApi.success(t("common:message.success")),
      onError: (err) => messageApi.error(translateApiError(t, err)),
    },
  );

  const stopVM = useApiAction<string>(
    (vmId) =>
      api.POST("/vms/{vm_id}/stop", { params: { path: { vm_id: vmId } } }),
    {
      invalidateKeys: [["vms"]],
      onSuccess: () => messageApi.success(t("common:message.success")),
      onError: (err) => messageApi.error(translateApiError(t, err)),
    },
  );

  const restartVM = useApiAction<string>(
    (vmId) =>
      api.POST("/vms/{vm_id}/restart", { params: { path: { vm_id: vmId } } }),
    {
      invalidateKeys: [["vms"]],
      onSuccess: () => messageApi.success(t("common:message.success")),
      onError: (err) => {
        const notice = extractRestartReconciliationNotice(err);
        if (notice) {
          setRestartReconciliationNotice(notice);
        }
        messageApi.error(translateApiError(t, err));
      },
    },
  );

  const deleteVM = useApiMutation<
    { vmId: string; vmName: string },
    DeleteVMResponse
  >(
    ({ vmId, vmName }) =>
      api.DELETE("/vms/{vm_id}", {
        params: {
          path: { vm_id: vmId },
          query: { confirm: true, confirm_name: vmName },
        },
      }),
    {
      invalidateKeys: [
        ["vms"],
        ["tickets"],
        ["my-tickets"],
        ["builtin-approval-tasks"],
      ],
      onSuccess: () => {
        messageApi.success(t("delete_request_submitted"));
        setDeleteOpen(false);
        setTimeout(() => setDeletingVM(null), 300);
      },
      onError: (err) => messageApi.error(translateApiError(t, err)),
    },
  );

  const openDeleteModal = (
    vmId: string,
    vmName: string,
    environment?: string,
  ) => {
    setDeletingVM({ id: vmId, name: vmName, environment });
    setDeleteConfirmName("");
    setDeleteOpen(true);
  };

  const closeDeleteModal = () => {
    setDeleteOpen(false);
    setTimeout(() => setDeletingVM(null), 300);
  };

  const submitDelete = () => {
    if (!deletingVM) return;
    if (
      deletingVM.environment !== "test" &&
      deleteConfirmName !== deletingVM.name
    ) {
      messageApi.warning(t("action.delete_type_name_hint"));
      return;
    }
    deleteVM.mutate({ vmId: deletingVM.id, vmName: deletingVM.name });
  };

  const openModifyModal = (vmId: string, vmName: string) => {
    currentModifyIntentRef.current = null;
    setModifyScope("single");
    setModifyTargetVM({ id: vmId, name: vmName });
    modifyForm.resetFields();
    setModifyOpen(true);
  };

  const openBatchModifyModal = () => {
    if (selectedVMIDs.length === 0) {
      messageApi.warning(t("batch.no_selection"));
      return;
    }
    currentModifyIntentRef.current = null;
    modifyForm.resetFields();
    setModifyTargetVM(null);
    setModifyScope("batch");
    setModifyOpen(true);
  };

  const closeModifyModal = () => {
    currentModifyIntentRef.current = null;
    setModifyOpen(false);
    setModifyTargetVM(null);
    modifyForm.resetFields();
  };

  const modifyHasRequestedTarget =
    normalizeOptionalTargetNumber(modifyTargetCPUValue) !== undefined ||
    normalizeOptionalTargetNumber(modifyTargetMemoryValue) !== undefined ||
    normalizeOptionalTargetNumber(modifyTargetDiskValue) !== undefined;

  const modifySubmitDisabled =
    !modifyHasRequestedTarget ||
    (modifyScope === "single" &&
      (modifyContextQuery.isLoading || !modifyContextQuery.data));

  const submitModify = async () => {
    try {
      await modifyForm.validateFields();
    } catch {
      return;
    }

    const values = modifyForm.getFieldsValue(true);
    const normalizedReason = values.reason.trim();
    const targetCPU = normalizeOptionalTargetNumber(values.target_cpu_cores);
    const targetMemory = normalizeOptionalTargetNumber(values.target_memory_gi);
    const targetDisk = normalizeOptionalTargetNumber(values.target_disk_gb);
    if (
      targetCPU === undefined &&
      targetMemory === undefined &&
      targetDisk === undefined
    ) {
      messageApi.warning(t("modify.target_required"));
      return;
    }

    const targetVM = modifyScope === "single" ? modifyTargetVM : null;
    if (modifyScope === "single") {
      const modifyContext = modifyContextQuery.data;
      if (!targetVM || !modifyContext) {
        messageApi.warning(t("modify.context_unavailable"));
        return;
      }
      if (
        targetDisk !== undefined &&
        targetDisk <= Number(modifyContext.current_disk_gb ?? 0)
      ) {
        messageApi.warning(
          t("modify.target_disk_expand_only", {
            current: modifyContext.current_disk_gb,
          }),
        );
        return;
      }
    }

    const body: VMModifyRequest = {
      reason: normalizedReason,
      target_cpu_cores: Number(targetCPU ?? 0),
      target_memory_gi: Number(targetMemory ?? 0),
      target_disk_gb: Number(targetDisk ?? 0),
    };

    if (modifyScope === "single") {
      createVMModifyRequest.mutate({ vmId: targetVM!.id, body });
      return;
    }

    if (batchRateLimited) {
      warnBatchRateLimited();
      return;
    }
    const payload: Omit<VMBatchSubmitRequest, "request_id"> = {
      operation: "MODIFY",
      reason: normalizedReason,
      items: selectedVMIDs.map((vmId) => ({
        vm_id: vmId.trim(),
        reason: normalizedReason,
        target_cpu_cores: Number(targetCPU ?? 0),
        target_memory_gi: Number(targetMemory ?? 0),
        target_disk_gb: Number(targetDisk ?? 0),
      })),
    };
    const intent = getStableBatchRequestIntent("MODIFY", payload);
    currentModifyIntentRef.current = intent;
    submitVMBatch.mutate({
      body: { ...payload, request_id: intent.requestId },
      intent,
      submissionSequence: nextBatchSubmissionSequence(),
    });
  };

  const wizardSteps = [
    { title: t("wizard.step.service") },
    { title: t("wizard.step.template") },
    { title: t("wizard.step.size") },
    { title: t("wizard.step.config") },
    { title: t("wizard.step.confirm") },
  ];

  const openWizard = (prefill?: VMRequestLaunchPrefill) => {
    applyPrefillToWizard(prefill);
  };

  const resumeDraft = () => {
    const draft = loadVMRequestDraft(draftOwner);
    if (!draft) {
      messageApi.info(t("draft.missing"));
      setSavedDraft(null);
      return;
    }
    setSavedDraft(draft);
    applyDraftToWizard(draft);
  };

  const closeWizard = () => {
    currentCreateIntentRef.current = null;
    setWizardOpen(false);
    setWizardStep(0);
    setRequestMode("guided");
    setSelectedSystemId("");
    form.resetFields();
  };

  const onSystemChange = (systemId: string) => {
    setSelectedSystemId(systemId);
    form.resetFields(["service_id"]);
  };

  const goToNextWizardStep = async () => {
    const fieldsByStep: Array<Array<keyof VMCreateFormValues>> = [
      ["service_id"],
      ["template_id"],
      ["instance_size_id"],
      ["namespace", "reason", "batch_count"],
      [],
    ];

    const fields = fieldsByStep[wizardStep] ?? [];
    if (fields.length === 0) {
      setWizardStep((step) => step + 1);
      return;
    }

    try {
      await form.validateFields(fields);
      setWizardStep((step) => step + 1);
    } catch {
      // Ant Form shows validation errors in place.
    }
  };

  const submitWizard = async () => {
    if (requestMode === "full") {
      try {
        await form.validateFields([
          "service_id",
          "template_id",
          "instance_size_id",
          "namespace",
          "reason",
          "batch_count",
        ]);
      } catch {
        return;
      }
    }

    // Include unmounted previous-step fields; default getFieldsValue() only
    // returns currently mounted fields and would drop wizard data on step 5.
    const values = form.getFieldsValue(true);
    const targetCPU = normalizeCreateTargetOverride(
      values.target_cpu_cores,
      selectedSize?.cpu_cores,
    );
    const targetMemory = normalizeCreateTargetOverride(
      values.target_memory_gi,
      selectedSize?.memory_gi,
    );
    const targetDisk = normalizeCreateTargetOverride(
      values.target_disk_gb,
      selectedSize?.disk_gb,
    );
    const singlePayload: VMCreateRequest = {
      service_id: values.service_id.trim(),
      template_id: values.template_id.trim(),
      instance_size_id: values.instance_size_id.trim(),
      namespace: values.namespace.trim(),
      reason: values.reason.trim(),
      ...(targetCPU !== undefined ? { target_cpu_cores: targetCPU } : {}),
      ...(targetMemory !== undefined ? { target_memory_gi: targetMemory } : {}),
      ...(targetDisk !== undefined ? { target_disk_gb: targetDisk } : {}),
    };
    const batchCount = Number(values.batch_count ?? 1);

    if (!Number.isFinite(batchCount) || batchCount <= 1) {
      createVMRequest.mutate(singlePayload);
      return;
    }
    if (batchRateLimited) {
      warnBatchRateLimited();
      return;
    }

    const batchPayload: Omit<VMBatchSubmitRequest, "request_id"> = {
      operation: "CREATE",
      reason: singlePayload.reason,
      items: Array.from({ length: batchCount }, () => ({
        service_id: singlePayload.service_id,
        template_id: singlePayload.template_id,
        instance_size_id: singlePayload.instance_size_id,
        namespace: singlePayload.namespace,
        reason: singlePayload.reason,
        ...(targetCPU !== undefined ? { target_cpu_cores: targetCPU } : {}),
        ...(targetMemory !== undefined
          ? { target_memory_gi: targetMemory }
          : {}),
        ...(targetDisk !== undefined ? { target_disk_gb: targetDisk } : {}),
      })),
    };
    const intent = getStableBatchRequestIntent("CREATE", batchPayload);
    currentCreateIntentRef.current = intent;
    submitCreateBatch.mutate({
      body: { ...batchPayload, request_id: intent.requestId },
      intent,
      submissionSequence: nextBatchSubmissionSequence(),
    });
  };

  const submitBatchDeleteSelected = () => {
    if (batchRateLimited) {
      warnBatchRateLimited();
      return;
    }
    if (selectedVMIDs.length === 0) {
      messageApi.warning(t("batch.no_selection"));
      return;
    }
    const invalidDeleteTargets = selectedVMRecords.filter(
      (vm) => !VM_DELETE_ALLOWED_STATUSES.has(vm.status),
    );
    if (invalidDeleteTargets.length > 0) {
      messageApi.warning(
        t("batch.delete_requires_stopped", {
          count: invalidDeleteTargets.length,
          names: invalidDeleteTargets
            .slice(0, 3)
            .map((vm) => vm.name)
            .join("、"),
          allowed_states: Array.from(VM_DELETE_ALLOWED_STATUSES).join(", "),
        }),
      );
      return;
    }
    const deleteReason = t("batch.delete_reason").trim();
    const batchVMIDs = selectedVMIDs.map((vmID) => vmID.trim());
    const payload: Omit<VMBatchSubmitRequest, "request_id"> = {
      operation: "DELETE",
      reason: deleteReason,
      items: batchVMIDs.map((vmID) => ({
        vm_id: vmID,
        reason: deleteReason,
      })),
    };
    const intent = getStableBatchRequestIntent("DELETE", {
      operation: payload.operation,
      vm_ids: batchVMIDs,
    });
    submitVMBatch.mutate({
      body: { ...payload, request_id: intent.requestId },
      intent,
      submissionSequence: nextBatchSubmissionSequence(),
    });
  };

  const submitBatchPowerSelected = (operation: VMBatchPowerAction) => {
    if (batchRateLimited) {
      warnBatchRateLimited();
      return;
    }
    if (selectedVMIDs.length === 0) {
      messageApi.warning(t("batch.no_selection"));
      return;
    }
    const powerReason = t("batch.power_reason", { operation }).trim();
    const batchVMIDs = selectedVMIDs.map((vmID) => vmID.trim());
    const payload: Omit<VMBatchPowerRequest, "request_id"> = {
      operation,
      reason: powerReason,
      items: batchVMIDs.map((vmID) => ({
        vm_id: vmID,
        reason: powerReason,
      })),
    };
    const intent = getStableBatchRequestIntent(`POWER:${operation}`, {
      operation,
      vm_ids: batchVMIDs,
    });
    submitVMBatchPower.mutate({
      body: { ...payload, request_id: intent.requestId },
      intent,
      submissionSequence: nextBatchSubmissionSequence(),
    });
  };

  const openSimilarRequest = async (vmID: string) => {
    const { data, error } = await api.GET("/vms/{vm_id}/request-prefill", {
      params: { path: { vm_id: vmID } },
    });

    if (error || !data) {
      if (error?.code === "VM_REQUEST_PREFILL_UNAVAILABLE") {
        messageApi.warning(t("request_similar.unavailable"));
        return;
      }
      messageApi.error(translateApiError(t, error));
      return;
    }

    const prefill = data as VMRequestPrefill;
    applyPrefillToWizard({
      systemId: prefill.system_id,
      serviceId: prefill.service_id,
      templateId: prefill.template_id,
      instanceSizeId: prefill.instance_size_id,
      namespace: prefill.namespace,
      reason: prefill.reason,
      batchCount: prefill.batch_count,
      requestMode: "full",
    });
  };

  const changeFilters = (nextFilters: Partial<VMListFilters>) => {
    setPage(1);
    setFilters((current) => ({
      ...current,
      ...nextFilters,
    }));
  };

  const resetFilters = () => {
    setPage(1);
    setFilters({
      search: "",
      namespace: "",
      status: "",
      clusterId: "",
      systemId: "",
      serviceId: "",
      osName: "",
      ipAddress: "",
    });
  };

  const activeBatchOperation = batchStatusQuery.data?.operation;
  const batchActionAllowed = canManageBatchOperation(
    user,
    activeBatchOperation,
  );
  const canRetryActiveBatch =
    batchActionAllowed &&
    (batchStatusQuery.data?.status === "IN_PROGRESS" ||
      batchStatusQuery.data?.status === "FAILED" ||
      batchStatusQuery.data?.status === "PARTIAL_SUCCESS") &&
    (batchStatusQuery.data?.children ?? []).some(isRetryableBatchChild);
  const canCancelActiveBatch =
    batchActionAllowed &&
    (batchStatusQuery.data?.status === "PENDING_APPROVAL" ||
      batchStatusQuery.data?.status === "IN_PROGRESS") &&
    (batchStatusQuery.data?.children ?? []).some(
      (child) => child.status === "PENDING",
    );

  return {
    messageContextHolder,
    page,
    pageSize,
    setPage,
    setPageSize,
    wizardOpen,
    wizardStep,
    setWizardStep,
    requestMode,
    setRequestMode,
    filters,
    changeFilters,
    resetFilters,
    form,
    selectedSystemId,
    selectedTemplate,
    selectedSize,
    placementHint: placementHintQuery.data?.placement_hint as
      VMPlacementHint | undefined,
    placementHintLoading: placementHintQuery.isLoading,
    namespaceValue,
    reasonValue,
    serviceIdValue,
    batchCountValue,
    targetCpuValue: normalizeOptionalTargetNumber(createTargetCPUValue),
    targetMemoryValue: normalizeOptionalTargetNumber(createTargetMemoryValue),
    targetDiskValue: normalizeOptionalTargetNumber(createTargetDiskValue),
    wizardSteps,
    vmData: vmListQuery.data,
    isLoading: vmListQuery.isLoading,
    refetch: vmListQuery.refetch,
    vmFilterOptions: vmFilterOptionsQuery.data,
    vmFilterOptionsLoading: vmFilterOptionsQuery.isLoading,
    systemsData: systemsQuery.data,
    servicesData: servicesQuery.data,
    templatesData,
    sizesData,
    namespaceOptions: requestContextQuery.data?.namespaces ?? [],
    createVMRequest,
    createVMModifyRequest,
    savedDraft,
    openWizard,
    openSimilarRequest,
    resumeDraft,
    closeWizard,
    discardDraft: clearSavedDraft,
    onSystemChange,
    goToNextWizardStep,
    submitWizard,
    selectedVMIDs,
    setSelectedVMIDs,
    modifyOpen,
    modifyScope,
    modifyForm,
    modifyTargetVM,
    modifyContext: modifyContextQuery.data,
    modifyContextLoading: modifyContextQuery.isLoading,
    openModifyModal,
    openBatchModifyModal,
    closeModifyModal,
    submitModify,
    modifySubmitDisabled,
    activeBatchID: effectiveActiveBatchID,
    activeBatchKind: resolvedActiveBatchKind,
    activeBatchStatusURL: effectiveActiveBatchStatusURL,
    batchStatus: batchStatusQuery.data,
    batchLoading: batchStatusQuery.isLoading,
    batchRateLimited,
    batchRateLimitContactAdmin,
    batchRetryAfterSeconds,
    lastBatchActionFeedback,
    canRetryActiveBatch,
    canCancelActiveBatch,
    refreshBatch: () => {
      if (!effectiveActiveBatchID) {
        return;
      }
      setBatchAutoPolling(true);
      void batchStatusQuery.refetch();
    },
    clearBatchTracking: () => {
      setActiveBatchID("");
      setActiveBatchStatusURL("");
      setActiveBatchKind("");
      setBatchAutoPolling(false);
      setLastBatchActionFeedback(null);
      clearStoredActiveBatchState();
    },
    retryBatch: () => {
      if (!effectiveActiveBatchID || !canRetryActiveBatch) {
        return;
      }
      if (batchRateLimited) {
        warnBatchRateLimited();
        return;
      }
      const targets = pickBatchActionTargets("retry");
      if (targets.length === 0) {
        messageApi.warning(t("batch.no_retryable_children"));
        return;
      }
      retryBatchMutation.mutate({
        actorKey: currentActorKey,
        batchID: effectiveActiveBatchID,
        targetTicketIDs: targets,
      });
    },
    cancelBatch: () => {
      if (!effectiveActiveBatchID || !canCancelActiveBatch) {
        return;
      }
      if (batchRateLimited) {
        warnBatchRateLimited();
        return;
      }
      const targets = pickBatchActionTargets("cancel");
      if (targets.length === 0) {
        messageApi.warning(t("batch.no_cancellable_children"));
        return;
      }
      cancelBatchMutation.mutate({
        actorKey: currentActorKey,
        batchID: effectiveActiveBatchID,
        targetTicketIDs: targets,
      });
    },
    submitBatchDeleteSelected,
    submitBatchPowerSelected,
    batchSubmitPending:
      submitVMBatch.isPending ||
      submitVMBatchPower.isPending ||
      submitCreateBatch.isPending,
    modifySubmitPending:
      createVMModifyRequest.isPending || submitVMBatch.isPending,
    batchActionPending:
      retryBatchMutation.isPending || cancelBatchMutation.isPending,
    startVM: (vmId: string) => startVM.mutate(vmId),
    stopVM: (vmId: string) => stopVM.mutate(vmId),
    restartVM: (vmId: string) => restartVM.mutate(vmId),
    restartReconciliationNotice,
    dismissRestartReconciliation: () => setRestartReconciliationNotice(null),
    deleteVM: openDeleteModal,
    openDeleteModal,
    deleteOpen,
    deletingVM,
    deleteConfirmName,
    setDeleteConfirmName,
    closeDeleteModal,
    submitDelete,
    deletePending: deleteVM.isPending,
  };
}
