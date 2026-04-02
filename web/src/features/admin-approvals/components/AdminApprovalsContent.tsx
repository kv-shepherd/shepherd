"use client";

import { useMemo, useState, type CSSProperties } from "react";
import {
  Alert,
  Badge,
  Button,
  Card,
  Descriptions,
  Form,
  Input,
  Popover,
  Popconfirm,
  Select,
  Segmented,
  Space,
  Switch,
  Table,
  Tag,
  Typography,
} from "antd";
import type { ColumnsType } from "antd/es/table";
import type { TFunction } from "i18next";
import {
  AuditOutlined,
  ExclamationCircleOutlined,
  MoreOutlined,
  ReloadOutlined,
} from "@ant-design/icons";
import { useRouter } from "next/navigation";
import { useTranslation } from "react-i18next";

import { ActionEmptyState } from "@/components/feedback/ActionEmptyState";
import {
  QueueReviewGlyph,
  RequestsOverviewGlyph,
  ServiceWorkspaceGlyph,
  VirtualMachinesOverviewGlyph,
} from "@/components/illustrations/DashboardIllustrations";
import { PageHeader, PageSurface } from "@/components/layouts/PageSection";
import { PageSearchToolbar } from "@/components/ui/PageSearchToolbar";
import { WorkbenchDetailModal } from "@/components/workbench/WorkbenchDetailModal";
import { SetupGuideCard } from "@/features/setup-guide/components/SetupGuideCard";
import { useSetupGuide } from "@/features/setup-guide/hooks/useSetupGuide";
import { translateApiError } from "@/lib/api/errorMessage";
import {
  approvalPrimaryAlert,
  approvalApproverSummary,
  formatApprovalResourceShape,
  approvalSummaryTitle,
  approvalRequesterSummary,
  buildApprovalBatchDisplayItems,
  buildApprovalChangeItems,
  buildApprovalOverviewItems,
  buildApprovalScopeItems,
  formatApprovalRecordID,
} from "@/features/approval-shared/summary";
import { useAdminApprovalsController } from "../hooks/useAdminApprovalsController";
import {
  ApprovalProvisioningCard,
  getCloneTypeTagColor,
  getProvisioningPhaseTagColor,
} from "./ApprovalProvisioningCard";
import { UnitInputNumber } from "@/components/form/UnitInputNumber";
import { LocalDateTimeText } from "@/components/ui/LocalDateTimeText";
import {
  getPriorityTier,
  OPERATION_FILTER_OPTIONS,
  OP_TYPE_CONFIG,
  STATUS_BADGES,
  STATUS_COLORS,
  STATUS_FILTER_OPTIONS,
  type ApprovalStatus,
  type ApprovalTask,
  type Cluster,
} from "../types";

const { Text } = Typography;
type PayloadRecord = Record<string, unknown>;
type RootVolumeResolution = NonNullable<
  NonNullable<Cluster["compatibility"]>["root_volume_resolution"]
>;
type RootVolumeModeOption = NonNullable<RootVolumeResolution["mode_options"]>[number];

interface SearchSelectOption {
  label: string;
  value: string;
}

interface CompactField {
  label: string;
  value?: string;
}

function renderSectionCard(title: string, fields: CompactField[]) {
  if (fields.every((field) => !field.value)) {
    return null;
  }
  return (
    <div className="workbench-table-section">
      <Text type="secondary" className="workbench-table-section__label">
        {title}
      </Text>
      {renderCompactFieldGrid(fields)}
    </div>
  );
}

function approvalContextFields(
  record: ApprovalTask,
  t: TFunction,
): CompactField[] {
  const summary = record.summary;
  const placement = record.placement_evaluation;
  const clusterDisplay = firstVisibleValue(
    placement?.selected_cluster_name,
    summary?.cluster_name,
    summary?.cluster_id,
  );

  return [
    {
      label: t("summary.system", { ns: "approval" }),
      value: firstVisibleValue(summary?.system_name, summary?.system_id),
    },
    {
      label: t("summary.service", { ns: "approval" }),
      value: firstVisibleValue(summary?.service_name, summary?.service_id),
    },
    {
      label: t("summary.namespace", { ns: "approval" }),
      value: firstVisibleValue(summary?.namespace),
    },
    {
      label: t("summary.cluster", { ns: "approval" }),
      value: clusterDisplay,
    },
  ];
}

function approvalRequestFields(
  record: ApprovalTask,
  t: TFunction,
): CompactField[] {
  const summary = record.summary;
  const targetVM = firstVisibleValue(
    summary?.vm_name,
    record.target_vm_name,
    summary?.vm_id,
  );
  const requestedShape = formatApprovalResourceShape(
    summary?.target_cpu_cores,
    summary?.target_memory_gi,
    summary?.target_disk_gb,
    t,
  );
  const currentShape = formatApprovalResourceShape(
    summary?.current_cpu_cores,
    summary?.current_memory_gi,
    summary?.current_disk_gb,
    t,
  );
  const changeSummary =
    currentShape && requestedShape && currentShape !== requestedShape
      ? `${currentShape} → ${requestedShape}`
      : requestedShape || currentShape;

  if (record.operation_type === "CREATE") {
    return [
      {
        label: t("summary.template", { ns: "approval" }),
        value: firstVisibleValue(summary?.template_name, summary?.template_id),
      },
      {
        label: t("summary.instance_size", { ns: "approval" }),
        value: firstVisibleValue(summary?.instance_size_name, summary?.instance_size_id),
      },
      {
        label: t("summary.target_resources", { ns: "approval" }),
        value: requestedShape,
      },
    ];
  }

  if (record.operation_type === "MODIFY") {
    return [
      {
        label: t("summary.virtual_machine", { ns: "approval" }),
        value: targetVM,
      },
      {
        label: t("summary.target_resources", { ns: "approval" }),
        value: changeSummary,
      },
    ];
  }

  if (record.operation_type === "POWER") {
    return [
      {
        label: t("summary.virtual_machine", { ns: "approval" }),
        value: targetVM,
      },
      {
        label: t("summary.power_action", { ns: "approval" }),
        value: firstVisibleValue(summary?.power_action),
      },
    ];
  }

  return [
    {
      label: t("summary.virtual_machine", { ns: "approval" }),
      value: targetVM,
    },
  ];
}

function asPayloadRecord(value: unknown): PayloadRecord | undefined {
  return typeof value === "object" && value !== null
    ? (value as PayloadRecord)
    : undefined;
}

function asPayloadRecords(value: unknown): PayloadRecord[] {
  if (!Array.isArray(value)) {
    return [];
  }
  return value
    .map(asPayloadRecord)
    .filter((item): item is PayloadRecord => Boolean(item));
}

function firstVisibleValue(
  ...values: Array<string | undefined | null>
): string | undefined {
  for (const value of values) {
    if (typeof value === "string" && value.trim() !== "") {
      return value.trim();
    }
  }
  return undefined;
}

function renderCompactFieldGrid(fields: CompactField[]) {
  const visibleFields = fields.filter((field) => field.value);
  if (visibleFields.length === 0) {
    return null;
  }
  return (
    <div className="workbench-compact-grid">
      {visibleFields.map((field) => (
        <div key={field.label} className="workbench-compact-grid__item">
          <Text type="secondary" className="workbench-compact-grid__label">
            {field.label}
          </Text>
          <Text strong className="workbench-compact-grid__value">
            {field.value}
          </Text>
        </div>
      ))}
    </div>
  );
}

function payloadBool(value: unknown): boolean | undefined {
  return typeof value === "boolean" ? value : undefined;
}

function payloadNumber(value: unknown): number | undefined {
  return typeof value === "number" && Number.isFinite(value) ? value : undefined;
}

function instanceSizeDiskGB(
  payload: PayloadRecord | undefined,
): number | undefined {
  return typeof payload?.instance_size_disk_gb === "number" &&
    Number.isFinite(payload.instance_size_disk_gb)
    ? payload.instance_size_disk_gb
    : undefined;
}

function instanceSizeDedicatedCPU(payload: PayloadRecord | undefined): boolean {
  return (
    payloadBool(payload?.instance_size_dedicated_cpu) ??
    payloadBool(payload?.dedicated_cpu) ??
    false
  );
}

function batchPayloadItems(
  payload: PayloadRecord | undefined,
): PayloadRecord[] {
  return asPayloadRecords(payload?.items);
}

function requiresStorageClassSelection(cluster: Cluster): boolean {
  return (
    cluster.compatibility?.reason_code === "CLUSTER_POLICY_STORAGE_CLASS_REQUIRED" ||
    cluster.compatibility?.root_volume_resolution?.state === "storage_class_required"
  );
}

function requiresRootVolumeModeSelection(cluster: Cluster): boolean {
  return cluster.compatibility?.root_volume_resolution?.state === "mode_required";
}

function clusterDisplayLabel(cluster: Cluster | undefined): string {
  if (!cluster) {
    return "—";
  }
  return cluster.display_name || cluster.name || cluster.id || "—";
}

function approvalClusterOptionLabel(cluster: Cluster): string {
  const primary = cluster.display_name?.trim() || cluster.name?.trim() || cluster.id;
  const secondary =
    cluster.display_name?.trim() &&
    cluster.name?.trim() &&
    cluster.display_name.trim() !== cluster.name.trim()
      ? cluster.name.trim()
      : "";
  return secondary ? `${primary} · ${secondary}` : primary;
}

function approvalPlacementAdvisoryLabel(
  code: string,
  t: (key: string, defaultValue?: string) => string,
): string {
  switch (code) {
    case "PVC_CLONE_HOST_ASSISTED_FALLBACK_LIKELY":
      return `${t(
        "filter.placement_advisory_host_assisted_clone",
        "Host-assisted clone fallback likely",
      )} · ${code}`;
    default:
      return code;
  }
}

function rootVolumeModeOptionKey(option: RootVolumeModeOption | undefined): string {
  if (!option?.volume_mode || !option.access_modes?.length) {
    return "";
  }
  return `${option.volume_mode}|${[...option.access_modes].sort().join(",")}`;
}

function rootVolumeModeOptionLabel(
  option: RootVolumeModeOption | undefined,
): string {
  if (!option?.volume_mode) {
    return "—";
  }
  const accessModes = option.access_modes?.length
    ? option.access_modes.join(" / ")
    : "—";
  return `${option.volume_mode} + ${accessModes}`;
}

function rootVolumeModeRecommendationRank(
  option: RootVolumeModeOption | undefined,
): number {
  if (!option?.volume_mode || !option.access_modes?.length) {
    return Number.MAX_SAFE_INTEGER;
  }
  const volumeMode = option.volume_mode.trim();
  const accessModes = [...option.access_modes].map((item) => item.trim()).sort();
  const key = `${volumeMode}|${accessModes.join(",")}`;
  switch (key) {
    case "Block|ReadWriteMany":
      return 0;
    case "Block|ReadWriteOnce":
      return 1;
    case "Filesystem|ReadWriteOnce":
      return 2;
    default:
      return 10;
  }
}

function recommendedRootVolumeModeOption(
  options: RootVolumeModeOption[],
): RootVolumeModeOption | undefined {
  if (options.length === 0) {
    return undefined;
  }
  return [...options].sort((left, right) => {
    const rankDiff =
      rootVolumeModeRecommendationRank(left) -
      rootVolumeModeRecommendationRank(right);
    if (rankDiff !== 0) {
      return rankDiff;
    }
    return rootVolumeModeOptionKey(left).localeCompare(rootVolumeModeOptionKey(right));
  })[0];
}

function renderRootVolumeResolutionMessage(
  resolution: RootVolumeResolution | undefined,
): string {
  if (!resolution) {
    return "—";
  }
  if (resolution.message) {
    return resolution.message;
  }
  if (resolution.state === "resolved" && resolution.effective_volume_mode) {
    return `${resolution.effective_storage_class ?? "—"} · ${rootVolumeModeOptionLabel({
      volume_mode: resolution.effective_volume_mode,
      access_modes: resolution.effective_access_modes,
    })}`;
  }
  return "—";
}

export function AdminApprovalsContent() {
  const router = useRouter();
  const { t } = useTranslation(["approval", "common", "vm"]);
  const approvals = useAdminApprovalsController({ t });
  const [quickSearchDraft, setQuickSearchDraft] = useState(
    () => approvals.searchFilter,
  );
  const [operationFilterDraft, setOperationFilterDraft] = useState(
    () => approvals.operationFilter,
  );
  const [selectedClusterFilterDraft, setSelectedClusterFilterDraft] = useState(
    () => approvals.selectedClusterFilter,
  );
  const [placementAdvisoryFilterDraft, setPlacementAdvisoryFilterDraft] = useState(
    () => approvals.placementAdvisoryFilter,
  );
  const [placementSnapshotFilterDraft, setPlacementSnapshotFilterDraft] = useState(
    () => approvals.placementSnapshotFilter,
  );
  const [openActionMenuId, setOpenActionMenuId] = useState<string | null>(null);
  const [advancedSearchOpen, setAdvancedSearchOpen] = useState(
    () =>
      approvals.operationFilter !== "ALL" ||
      approvals.selectedClusterFilter.trim() !== "" ||
      approvals.placementAdvisoryFilter.trim() !== "" ||
      approvals.placementSnapshotFilter !== "ALL",
  );
  const setupGuide = useSetupGuide();
  const pageItems = useMemo(() => approvals.data?.items ?? [], [approvals.data?.items]);
  const selectedClusterOptionLabel = clusterDisplayLabel(approvals.selectedCluster);
  const clusterFilterOptions = useMemo(() => {
    const groups = new Map<string, SearchSelectOption[]>();
    for (const cluster of approvals.filterClusters) {
      const label = approvalClusterOptionLabel(cluster);
      const groupKey = cluster.environment || "";
      const existing = groups.get(groupKey) ?? [];
      existing.push({ label, value: cluster.id });
      groups.set(groupKey, existing);
    }

    const orderedGroups = ["prod", "test", ""];
    const groupOptions = orderedGroups
      .filter((groupKey) => (groups.get(groupKey) ?? []).length > 0)
      .map((groupKey) => ({
        label:
          groupKey === "prod"
            ? t("common:environment.prod", { defaultValue: "Production" })
            : groupKey === "test"
              ? t("common:environment.test", { defaultValue: "Test" })
              : t("common:label.other", { defaultValue: "Other" }),
        options: (groups.get(groupKey) ?? [])
          .slice()
          .sort((left, right) => left.label.localeCompare(right.label)),
      }));

    const remainingGroups = Array.from(groups.entries())
      .filter(([groupKey]) => !orderedGroups.includes(groupKey))
      .sort(([left], [right]) => left.localeCompare(right))
      .map(([groupKey, options]) => ({
        label: groupKey,
        options: options
          .slice()
          .sort((left, right) => left.label.localeCompare(right.label)),
      }));

    return [...groupOptions, ...remainingGroups];
  }, [approvals.filterClusters, t]);
  const placementAdvisoryOptions = useMemo(() => {
    const values = new Set<string>();
    for (const item of pageItems) {
      const advisoryCode = item.placement_evaluation?.advisory_code?.trim();
      if (advisoryCode) {
        values.add(advisoryCode);
      }
    }
    if (placementAdvisoryFilterDraft.trim()) {
      values.add(placementAdvisoryFilterDraft.trim());
    }
    return Array.from(values)
      .sort((left, right) => left.localeCompare(right))
      .map((code) => ({
        label: approvalPlacementAdvisoryLabel(code, (key, defaultValue) =>
          t(key, { defaultValue }),
        ),
        value: code,
      }));
  }, [pageItems, placementAdvisoryFilterDraft, t]);
  const pendingOnPage = pageItems.filter((ticket) => ticket.status === "PENDING").length;
  const urgentOnPage = pageItems.filter(
    (ticket) =>
      ticket.status === "PENDING" && getPriorityTier(ticket.created_at) === "urgent",
  ).length;
  const createOnPage = pageItems.filter(
    (ticket) => ticket.operation_type === "CREATE",
  ).length;
  const provisioningVisible = pageItems.filter((ticket) => Boolean(ticket.provisioning)).length;
  const isDefaultQueueState =
    approvals.statusFilter === "PENDING" &&
    approvals.searchFilter.trim() === "" &&
    approvals.operationFilter === "ALL" &&
    approvals.selectedClusterFilter.trim() === "" &&
    approvals.placementAdvisoryFilter.trim() === "" &&
    approvals.placementSnapshotFilter === "ALL";
  const hasActiveFilters =
    approvals.statusFilter !== "PENDING" ||
    approvals.searchFilter.trim() !== "" ||
    approvals.operationFilter !== "ALL" ||
    approvals.selectedClusterFilter.trim() !== "" ||
    approvals.placementAdvisoryFilter.trim() !== "" ||
    approvals.placementSnapshotFilter !== "ALL";
  const isQueueEmpty = pageItems.length === 0 && !approvals.isLoading;
  const shouldShowSetupEmpty = isQueueEmpty && isDefaultQueueState;
  const approveAlert = approvals.approveModal
    ? approvalPrimaryAlert(approvals.approveModal, t)
    : null;
  const approveSummaryTitle = approvals.approveModal
    ? approvalSummaryTitle(approvals.approveModal, t)
    : "—";
  const approveOverviewItems = approvals.approveModal
    ? buildApprovalOverviewItems(approvals.approveModal, t)
    : [];
  const approveScopeItems = approvals.approveModal
    ? buildApprovalScopeItems(approvals.approveModal, t)
    : [];
  const approveChangeItems = approvals.approveModal
    ? buildApprovalChangeItems(approvals.approveModal, t)
    : [];
  const approveBatchItems = approvals.approveModal
    ? buildApprovalBatchDisplayItems(approvals.approveModal, t)
    : [];
  const applyQueueFilters = () => {
    approvals.changeOperationFilter(operationFilterDraft);
    approvals.changeSelectedClusterFilter(selectedClusterFilterDraft);
    approvals.changePlacementAdvisoryFilter(placementAdvisoryFilterDraft);
    approvals.changePlacementSnapshotFilter(placementSnapshotFilterDraft);
  };
  const resetQueueFilters = () => {
    setQuickSearchDraft("");
    setOperationFilterDraft("ALL");
    setSelectedClusterFilterDraft("");
    setPlacementAdvisoryFilterDraft("");
    setPlacementSnapshotFilterDraft("ALL");
    setAdvancedSearchOpen(false);
    approvals.resetFilters();
  };
  const approveModalPayload = asPayloadRecord(approvals.approveModal?.ticket_payload);
  const modifyRequiresRestart =
    payloadBool(approveModalPayload?.requires_restart) ?? false;
  const modifyCurrentCPURequest = payloadNumber(
    approveModalPayload?.current_cpu_request,
  );
  const modifyCurrentMemoryRequestGi = payloadNumber(
    approveModalPayload?.current_memory_request_gi,
  );
  const modifyTargetCPULimit =
    approvals.approveModal?.summary?.target_cpu_cores ??
    payloadNumber(approveModalPayload?.target_cpu_cores);
  const modifyTargetMemoryLimitGi =
    approvals.approveModal?.summary?.target_memory_gi ??
    payloadNumber(approveModalPayload?.target_memory_gi);
  const modifyCPURequestNeedsReview =
    typeof modifyCurrentCPURequest === "number" &&
    typeof modifyTargetCPULimit === "number" &&
    modifyCurrentCPURequest > modifyTargetCPULimit;
  const modifyMemoryRequestNeedsReview =
    typeof modifyCurrentMemoryRequestGi === "number" &&
    typeof modifyTargetMemoryLimitGi === "number" &&
    modifyCurrentMemoryRequestGi > modifyTargetMemoryLimitGi;

  const columns: ColumnsType<ApprovalTask> = [
    {
      title: t("request_summary"),
      key: "request_summary",
      width: 340,
      render: (_, record) => {
        const operationLabel = record.operation_type
          ? t(`op_type.${record.operation_type}`)
          : undefined;
        const requestReason = record.reason?.trim();
        const failureReason =
          record.provisioning?.failure_message?.trim() ||
          record.reject_reason?.trim() ||
          undefined;
        const showRequestReason =
          Boolean(requestReason) && requestReason !== approvalSummaryTitle(record, t);
        return (
          <Space direction="vertical" size={4} className="workbench-table-stack">
            <Space size={8} className="workbench-table-heading">
              <AuditOutlined style={{ color: "#d4380d" }} />
              <Text strong className="workbench-table-title">
                {approvalSummaryTitle(record, t)}
              </Text>
              {operationLabel ? <Tag color="purple">{operationLabel}</Tag> : null}
            </Space>
            {showRequestReason ? (
              <div className="workbench-inline-meta">
                <Text type="secondary" className="workbench-inline-meta__label">
                  {t("reason")}
                </Text>
                <Text className="workbench-inline-meta__value">{requestReason}</Text>
              </div>
            ) : null}
            <Text copyable={{ text: record.id }} type="secondary" className="workbench-ticket-meta">
              {t("ticket_id")}: {formatApprovalRecordID(record.id)}
            </Text>
            {record.status === "FAILED" &&
            failureReason &&
            failureReason !== requestReason ? (
              <Text type="danger" className="workbench-table-note">
                {failureReason}
              </Text>
            ) : null}
          </Space>
        );
      },
    },
    {
      title: t("table.target_context"),
      key: "target_context",
      width: 460,
      render: (_, record) => {
        const placement = record.placement_evaluation;
        const summary = record.summary;
        const itemCount = summary?.batch_count ||
          batchPayloadItems(record.ticket_payload as PayloadRecord | undefined).length;
        const provisioning = record.provisioning;
        const opType = record.operation_type;
        const config = OP_TYPE_CONFIG[opType ?? "CREATE"] ?? OP_TYPE_CONFIG.CREATE;
        const Icon = config.icon;
        const contextFields = approvalContextFields(record, t);
        const requestSummaryFields = approvalRequestFields(record, t);
        return (
          <Space direction="vertical" size={4} className="workbench-table-stack">
            <Space wrap size={[6, 6]} className="workbench-table-tag-row">
              <Tag color={config.color} icon={<Icon />}>
                {t(`op_type.${opType ?? "CREATE"}`)}
              </Tag>
              {itemCount > 0 ? (
                <Tag color="gold">
                  {t("batch.child_count", {
                    defaultValue: "{{count}} items",
                    count: itemCount,
                  })}
                </Tag>
                ) : null}
            </Space>
            <div className="workbench-table-section-grid">
              {renderSectionCard(t("workbench.table.scope_label"), contextFields)}
              {renderSectionCard(t("workbench.table.request_label"), requestSummaryFields)}
            </div>
            {placement?.advisory_code ? (
              <Tag color="warning">
                {approvalPlacementAdvisoryLabel(
                  placement.advisory_code,
                  (key, defaultValue) => t(key, { defaultValue }),
                )}
              </Tag>
            ) : null}
            {record.operation_type === "CREATE" && provisioning ? (
              <Space
                direction="vertical"
                size={2}
                data-testid={`approval-provisioning-summary-${record.id}`}
              >
                <Space wrap size={[6, 6]}>
                  <Tag color={getProvisioningPhaseTagColor(provisioning.phase)}>
                    {provisioning.phase || "—"}
                  </Tag>
                  {provisioning.clone_type === "copy" ? (
                    <Tag color={getCloneTypeTagColor(provisioning.clone_type)}>
                      {t(
                        "approve_modal.provisioning.clone_type_copy",
                        "Host-assisted copy",
                      )}
                    </Tag>
                  ) : null}
                </Space>
                {provisioning.failure_message ? (
                  <Text type="danger" className="workbench-table-note">
                    {provisioning.failure_message}
                  </Text>
                ) : provisioning.progress ? (
                  <Text type="secondary" className="workbench-table-note">
                    {provisioning.progress}
                  </Text>
                ) : null}
              </Space>
            ) : null}
          </Space>
        );
      },
    },
    {
      title: t("table.queue_state"),
      key: "queue_state",
      width: 220,
      render: (_, record) => {
        const requester = approvalRequesterSummary(record);
        const approver = approvalApproverSummary(record);
        return (
          <Space direction="vertical" size={4} className="workbench-table-stack">
            <Badge
              status={STATUS_BADGES[record.status] ?? "default"}
              text={
                <Tag color={STATUS_COLORS[record.status]}>{t(`status.${record.status}`)}</Tag>
              }
            />
            <div className="workbench-table-section workbench-table-section--compact">
              <Text type="secondary" className="workbench-table-section__label">
                {t("workbench.table.queue_label")}
              </Text>
              <div className="workbench-actor-line">
                <Text type="secondary" className="workbench-table-note">
                  {t("requester")}
                </Text>
                <Text>{requester?.primary || "—"}</Text>
                {requester?.secondary ? (
                  <Text type="secondary" className="workbench-table-note">
                    {requester.secondary}
                  </Text>
                ) : null}
              </div>
              <div className="workbench-actor-line">
                <Text type="secondary" className="workbench-table-note">
                  {t("approver")}
                </Text>
                <Text>{approver?.primary || "—"}</Text>
                {approver?.secondary ? (
                  <Text type="secondary" className="workbench-table-note">
                    {approver.secondary}
                  </Text>
                ) : null}
              </div>
            </div>
            <div className="workbench-table-meta-stack">
              <Text type="secondary" className="workbench-table-note">
                {t("common:table.created_at")}: <LocalDateTimeText value={record.created_at} />
              </Text>
            </div>
          </Space>
        );
      },
    },
    {
      title: t("common:table.actions"),
      key: "actions",
      width: 180,
      render: (_, record) => {
        if (record.status !== "PENDING") {
          return <Text type="secondary">—</Text>;
        }
        const moreContent = (
          <div className="workbench-row-menu">
            <Button
              type="text"
              danger
              block
              data-testid={`approval-action-reject-${record.id}`}
              onClick={() => {
                setOpenActionMenuId(null);
                approvals.openRejectModal(record);
              }}
            >
              {t("common:button.reject")}
            </Button>
            <Popconfirm
              title={t("cancel_confirm")}
              onConfirm={() => {
                setOpenActionMenuId(null);
                approvals.submitCancel(record.id);
              }}
              okText={t("common:button.confirm")}
              cancelText={t("common:button.cancel")}
            >
              <Button
                type="text"
                danger
                block
                data-testid={`approval-action-cancel-${record.id}`}
                loading={approvals.cancelPending}
              >
                {t("cancel")}
              </Button>
            </Popconfirm>
          </div>
        );
        return (
          <Space size={8} wrap className="workbench-row-actions">
            <Button
              type="primary"
              size="small"
              icon={<AuditOutlined />}
              data-testid={`approval-action-approve-${record.id}`}
              onClick={() => approvals.openApproveModal(record)}
            >
              {t("action.review")}
            </Button>
            <Popover
              trigger="click"
              placement="bottomRight"
              open={openActionMenuId === record.id}
              onOpenChange={(open) => setOpenActionMenuId(open ? record.id : null)}
              content={moreContent}
            >
              <Button
                size="small"
                data-testid={`approval-action-more-${record.id}`}
                aria-label={`${t("common:table.actions")} ${record.id}`}
                icon={<MoreOutlined />}
              />
            </Popover>
          </Space>
        );
      },
    },
  ];

  const renderQueueOverviewStrip = () => (
    <div className="workbench-overview-strip">
      <div
        className={[
          "workbench-overview-card",
          approvals.statusFilter === "PENDING" ? "workbench-overview-card--active" : "",
        ].filter(Boolean).join(" ")}
        style={{ "--workbench-overview-accent": "#D97706" } as CSSProperties}
      >
        <div className="workbench-overview-card__header">
          <span className="workbench-overview-card__title">{t("summary.pending_title")}</span>
          <QueueReviewGlyph className="workbench-overview-card__art" />
        </div>
        <div className="workbench-overview-card__value">{pendingOnPage}</div>
        <div className="workbench-overview-card__meta">{t("summary.pending_description")}</div>
      </div>

      <div
        className="workbench-overview-card"
        style={{ "--workbench-overview-accent": "#CF1322" } as CSSProperties}
      >
        <div className="workbench-overview-card__header">
          <span className="workbench-overview-card__title">{t("summary.urgent_title")}</span>
          <RequestsOverviewGlyph className="workbench-overview-card__art" />
        </div>
        <div className="workbench-overview-card__value">
          <span style={{ color: urgentOnPage > 0 ? "#cf1322" : undefined }}>{urgentOnPage}</span>
        </div>
        <div className="workbench-overview-card__meta">{t("summary.urgent_description")}</div>
      </div>

      <div
        className="workbench-overview-card"
        style={{ "--workbench-overview-accent": "#1D5BFF" } as CSSProperties}
      >
        <div className="workbench-overview-card__header">
          <span className="workbench-overview-card__title">{t("summary.create_title")}</span>
          <ServiceWorkspaceGlyph className="workbench-overview-card__art" />
        </div>
        <div className="workbench-overview-card__value">{createOnPage}</div>
        <div className="workbench-overview-card__meta">{t("summary.create_description")}</div>
      </div>

      <div
        className="workbench-overview-card"
        style={{ "--workbench-overview-accent": "#6D4DE3" } as CSSProperties}
      >
        <div className="workbench-overview-card__header">
          <span className="workbench-overview-card__title">{t("summary.provisioning_title")}</span>
          <VirtualMachinesOverviewGlyph className="workbench-overview-card__art" />
        </div>
        <div className="workbench-overview-card__value">{provisioningVisible}</div>
        <div className="workbench-overview-card__meta">{t("summary.provisioning_description")}</div>
      </div>
    </div>
  );

  const renderQueueBanner = () => (
    <div className="workbench-queue-banner">
      <div className="workbench-queue-banner__header">
        <Space wrap size={8}>
          <Text strong>{t("triage.title")}</Text>
          <Tag>{t("common:table.total", { total: pageItems.length })}</Tag>
          <Tag color={approvals.statusFilter === "PENDING" ? "orange" : "blue"}>
            {approvals.statusFilter === "ALL"
              ? t("filter_all")
              : t(`status.${approvals.statusFilter}`)}
          </Tag>
        </Space>
      </div>
      <Text type="secondary">{t("triage.description")}</Text>
    </div>
  );

  return (
    <div data-testid="admin-approvals-page">
      {approvals.messageContextHolder}
      <PageHeader
        title={t("common:nav.approval_tasks")}
        subtitle={t("subtitle")}
      />
      {renderQueueOverviewStrip()}
      <PageSurface
        style={{ marginBottom: 16 }}
        title={t("triage.title")}
        extra={(
          <Space wrap>
            <Segmented
              data-testid="approvals-status-filter"
              value={approvals.statusFilter}
              onChange={(value) =>
                approvals.changeStatusFilter(value as "ALL" | ApprovalStatus)
              }
              options={STATUS_FILTER_OPTIONS.map((option) => ({
                label: t(option.i18nKey),
                value: option.key,
              }))}
            />
            <Button
              icon={<ReloadOutlined />}
              data-testid="approvals-refresh-btn"
              onClick={() => approvals.refetch()}
            >
              {t("common:button.refresh")}
            </Button>
          </Space>
        )}
      >
        <Space direction="vertical" size={12} style={{ width: "100%" }}>
          {renderQueueBanner()}
          {approvals.listError && (
            <Alert
              type="error"
              showIcon
              message={t("list.load_failed_title")}
              description={translateApiError(t, approvals.listError)}
            />
          )}
          <PageSearchToolbar
            searchValue={approvals.searchFilter}
            searchDraftValue={quickSearchDraft}
            onSearchDraftChange={setQuickSearchDraft}
            onSearchChange={(value) => {
              approvals.changeSearchFilter(value);
              setQuickSearchDraft(value);
            }}
            searchPlaceholder={t("filter.quick_search_placeholder", {
              defaultValue: "Search ticket ID, requester, or selected cluster",
            })}
            searchHelp={t("filter.quick_search_help", {
              defaultValue:
                "Quick search matches ticket ID, requester, and selected cluster name. Pasted cluster IDs still work. Use advanced search for approval-specific queue filters.",
            })}
            searchTestId="approvals-search-input"
            hasActiveFilters={hasActiveFilters}
            onClear={resetQueueFilters}
            clearLabel={t("common:button.reset")}
            clearTestId="approvals-clear-filters"
            advancedSearch={{
              open: advancedSearchOpen,
              onToggle: () => setAdvancedSearchOpen((current) => !current),
              openLabel: t("common:search.advanced"),
              closeLabel: t("common:search.hide_advanced"),
              title: t("triage.title"),
              toggleTestId: "approvals-advanced-search-toggle",
              content: (
                <Space direction="vertical" size={12} style={{ width: "100%" }}>
                  <Text type="secondary">{t("triage.description")}</Text>
                  <Space wrap size={12}>
                    <Select
                      showSearch
                      optionFilterProp="label"
                      value={operationFilterDraft}
                      onChange={(value) =>
                        setOperationFilterDraft(
                          value as "ALL" | ApprovalTask["operation_type"],
                        )
                      }
                      options={OPERATION_FILTER_OPTIONS.map((option) => ({
                        label: t(option.i18nKey),
                        value: option.key,
                      }))}
                      style={{ minWidth: 180 }}
                      placeholder={t("filter.operation_label", "Operation")}
                    />
                    <Select
                      showSearch
                      optionFilterProp="label"
                      value={placementSnapshotFilterDraft}
                      onChange={(value) =>
                        setPlacementSnapshotFilterDraft(
                          value as "ALL" | "present" | "missing",
                        )
                      }
                      options={[
                        {
                          label: t("filter.placement_all", "All placement states"),
                          value: "ALL",
                        },
                        {
                          label: t("filter.placement_present", "Placement captured"),
                          value: "present",
                        },
                        {
                          label: t("filter.placement_missing", "Placement missing"),
                          value: "missing",
                        },
                      ]}
                      style={{ minWidth: 200 }}
                      placeholder={t("filter.placement_label", "Placement snapshot")}
                    />
                    <Select
                      showSearch
                      allowClear
                      optionFilterProp="label"
                      value={selectedClusterFilterDraft}
                      onChange={(value) =>
                        setSelectedClusterFilterDraft(value ?? "")
                      }
                      options={clusterFilterOptions}
                      placeholder={t(
                        "filter.selected_cluster",
                        "Filter by cluster name",
                      )}
                      style={{ minWidth: 260 }}
                    />
                    <Select
                      showSearch
                      allowClear
                      optionFilterProp="label"
                      value={placementAdvisoryFilterDraft}
                      onChange={(value) =>
                        setPlacementAdvisoryFilterDraft(value ?? "")
                      }
                      options={placementAdvisoryOptions}
                      placeholder={t(
                        "filter.placement_advisory",
                        "Filter by placement advisory",
                      )}
                      style={{ minWidth: 300 }}
                    />
                  </Space>
                  <Space style={{ width: "100%", justifyContent: "flex-end" }}>
                    <Button type="primary" onClick={applyQueueFilters}>
                      {t("common:button.search")}
                    </Button>
                  </Space>
                </Space>
              ),
            }}
          />
        </Space>
      </PageSurface>

      <PageSurface flush={true}>
        {/* ADR-0015 §11: Priority tier highlighting styles */}
        <style>{`
                    .approval-row-urgent td { background-color: rgba(255, 77, 79, 0.06) !important; }
                    .approval-row-warning td { background-color: rgba(250, 173, 20, 0.06) !important; }
                `}</style>
        {shouldShowSetupEmpty ? (
          !setupGuide.vmRequestReady ? (
            <div style={{ padding: 24 }}>
              <SetupGuideCard variant="vm" surface={false} />
            </div>
          ) : (
            <div style={{ padding: 32 }}>
              <ActionEmptyState
                title={t("empty.pending_title")}
                description={t("empty.pending_description")}
                visual={<QueueReviewGlyph className="action-empty-state__art" />}
                actions={(
                <Button type="primary" onClick={() => router.push("/vms?request=create")}>
                  {t("empty.open_vm_request")}
                </Button>
                )}
              />
            </div>
          )
        ) : isQueueEmpty ? (
          <div style={{ padding: 32 }}>
            <ActionEmptyState
              title={t("empty.filtered_title")}
              description={t("empty.filtered_description")}
              visual={<RequestsOverviewGlyph className="action-empty-state__art" />}
              actions={(
              <Button onClick={resetQueueFilters}>
                {t("common:button.reset")}
              </Button>
              )}
            />
          </div>
        ) : (
          <Table<ApprovalTask>
            columns={columns}
            dataSource={approvals.data?.items ?? []}
            rowKey="id"
            loading={approvals.isLoading}
            scroll={{ x: 1080 }}
            rowClassName={(record) => {
              if (record.status !== "PENDING") {
                return "";
              }
              const tier = getPriorityTier(record.created_at);
              if (tier === "urgent") {
                return "approval-row-urgent";
              }
              if (tier === "warning") {
                return "approval-row-warning";
              }
              return "";
            }}
            pagination={{
              current: approvals.page,
              pageSize: approvals.pageSize,
              total: approvals.data?.pagination?.total ?? 0,
              showTotal: (total) => t("common:table.total", { total }),
              onChange: (page, pageSize) => {
                approvals.setPage(page);
                approvals.setPageSize(pageSize);
              },
            }}
            size="middle"
          />
        )}
      </PageSurface>

      {approvals.approveModal ? (
      <WorkbenchDetailModal
        title={
          approvals.approveModal?.operation_type === "DELETE"
            ? t("approve_modal.delete_title")
            : t("approve_modal.title")
        }
        open={Boolean(approvals.approveModal)}
        onOk={() => {
          void approvals.submitApprove();
        }}
        onCancel={approvals.closeApproveModal}
        confirmLoading={approvals.approvePending}
        contentMinWidth={960}
        data-testid="approve-modal"
      >
        <Form
          form={approvals.approveForm}
          layout="vertical"
          name="approve-form"
          preserve={false}
        >
          {approveAlert && (
            <Alert
              type={approveAlert.type}
              showIcon
              style={{ marginBottom: 16 }}
              message={approveAlert.message}
              description={approveAlert.description}
            />
          )}
          <Card size="small" className="workbench-detail-hero" style={{ marginBottom: 16 }}>
            <Space direction="vertical" size={6} style={{ width: "100%" }}>
              <Space wrap size={8}>
                <Text strong style={{ fontSize: 16 }}>{approveSummaryTitle}</Text>
                <Tag color={STATUS_COLORS[approvals.approveModal.status]}>
                  {t(`status.${approvals.approveModal.status}`)}
                </Tag>
                {approvals.approveModal.operation_type ? (
                  <Tag color="purple">{t(`op_type.${approvals.approveModal.operation_type}`)}</Tag>
                ) : null}
              </Space>
              <Space wrap size={10} className="workbench-detail-hero__meta">
                <Text copyable={{ text: approvals.approveModal.id }} type="secondary" className="workbench-ticket-meta">
                {t("ticket_id")}: {formatApprovalRecordID(approvals.approveModal.id)}
                </Text>
                <Text type="secondary">
                  {t("common:table.created_at")}: <LocalDateTimeText value={approvals.approveModal.created_at} />
                </Text>
              </Space>
              <div className="workbench-detail-hero__grid">
                {renderSectionCard(
                  t("workbench.table.scope_label"),
                  approvalContextFields(approvals.approveModal, t),
                )}
                {renderSectionCard(
                  t("workbench.table.request_label"),
                  approvalRequestFields(approvals.approveModal, t),
                )}
              </div>
            </Space>
          </Card>
          {(approveOverviewItems.length > 0 ||
            approveScopeItems.length > 0 ||
            approveChangeItems.length > 0) && (
            <div
              className="workbench-detail-modal__grid"
            >
              {approveScopeItems.length > 0 && (
                <Card
                  size="small"
                  className="workbench-detail-section-card workbench-detail-section-card--primary"
                  title={t("summary.scope_title")}
                >
                  <Descriptions
                    bordered
                    size="small"
                    column={1}
                    items={approveScopeItems}
                  />
                </Card>
              )}
              {approveChangeItems.length > 0 && (
                <Card
                  size="small"
                  className="workbench-detail-section-card workbench-detail-section-card--secondary"
                  title={t("summary.change_title")}
                >
                  <Descriptions
                    bordered
                    size="small"
                    column={1}
                    items={approveChangeItems}
                  />
                </Card>
              )}
              {approveOverviewItems.length > 0 && (
                <Card
                  size="small"
                  className="workbench-detail-section-card workbench-detail-section-card--tertiary"
                  title={t("summary.workflow_title")}
                >
                  <Descriptions
                    bordered
                    size="small"
                    column={1}
                    items={approveOverviewItems}
                  />
                </Card>
              )}
            </div>
          )}
          {approveBatchItems.length > 0 && (
            <Card
              size="small"
              className="workbench-detail-section-card workbench-detail-section-card--wide"
              style={{ marginBottom: 16 }}
              title={t("summary.affected_items_title")}
            >
              <div className="workbench-detail-modal__table-scroll">
                <Table
                  size="small"
                  pagination={false}
                  rowKey="key"
                  dataSource={approveBatchItems}
                  scroll={{ x: 760, y: 280 }}
                  columns={[
                    {
                      title: t("summary.item"),
                      dataIndex: "title",
                      key: "title",
                      width: 240,
                      render: (value: string | undefined) => (
                        <Space direction="vertical" size={4} className="workbench-table-stack">
                          <Text strong className="workbench-table-title">
                            {value || "—"}
                          </Text>
                        </Space>
                      ),
                    },
                    {
                      title: t("summary.scope"),
                      key: "scope",
                      width: 280,
                      render: (_, record) => (
                        <Space direction="vertical" size={4} className="workbench-batch-cell">
                          <div className="workbench-batch-cell__row">
                            <Text type="secondary" className="workbench-batch-cell__label">
                              {t("summary.scope")}
                            </Text>
                            <Text>{record.scope || "—"}</Text>
                          </div>
                          <div className="workbench-batch-cell__row">
                            <Text type="secondary" className="workbench-batch-cell__label">
                              {t("summary.cluster")}
                            </Text>
                            <Text>{record.cluster || "—"}</Text>
                          </div>
                          <div className="workbench-batch-cell__row">
                            <Text type="secondary" className="workbench-batch-cell__label">
                              {t("summary.request_vm_status")}
                            </Text>
                            <Text>{record.requestStatus || "—"}</Text>
                          </div>
                          <div className="workbench-batch-cell__row">
                            <Text type="secondary" className="workbench-batch-cell__label">
                              {t("summary.latest_vm_status")}
                            </Text>
                            <Space direction="vertical" size={0}>
                              <Text>{record.latestStatus || "—"}</Text>
                            </Space>
                          </div>
                          {record.statusChanged && (
                            <Text type="warning" className="workbench-table-note">
                              {t("summary.status_changed")}
                            </Text>
                          )}
                        </Space>
                      ),
                    },
                    {
                      title: t("summary.target_resources"),
                      key: "target_resources",
                      width: 280,
                      render: (_, record) => (
                        <Space direction="vertical" size={4} className="workbench-batch-cell">
                          <div className="workbench-batch-cell__row">
                            <Text type="secondary" className="workbench-batch-cell__label">
                              {t("summary.current_resources")}
                            </Text>
                            <Text>{record.currentShape || "—"}</Text>
                          </div>
                          <div className="workbench-batch-cell__row">
                            <Text type="secondary" className="workbench-batch-cell__label">
                              {t("summary.target_resources")}
                            </Text>
                            <Text>{record.targetShape || "—"}</Text>
                          </div>
                          <div className="workbench-batch-cell__row">
                            <Text type="secondary" className="workbench-batch-cell__label">
                              {t("summary.power_action")}
                            </Text>
                            <Text>{record.action || "—"}</Text>
                          </div>
                        </Space>
                      ),
                    },
                  ]}
                />
              </div>
            </Card>
          )}
          {approvals.approveModal?.operation_type === "MODIFY"
            ? (
                <>
                  {modifyRequiresRestart && (
                    <Alert
                      type="warning"
                      showIcon
                      style={{ marginBottom: 16 }}
                      message={t(
                        "approve_modal.modify_restart_required_title",
                      )}
                      description={t(
                        "approve_modal.modify_restart_required_description",
                      )}
                    />
                  )}
                  <Alert
                    type={
                      modifyCPURequestNeedsReview ||
                      modifyMemoryRequestNeedsReview
                        ? "warning"
                        : "info"
                    }
                    showIcon
                    style={{ marginBottom: 16 }}
                    message={t("approve_modal.modify_request_review_title")}
                    description={t(
                      modifyCPURequestNeedsReview ||
                        modifyMemoryRequestNeedsReview
                        ? "approve_modal.modify_request_review_required_description"
                        : "approve_modal.modify_request_review_description",
                      {
                        cpu_request:
                          typeof modifyCurrentCPURequest === "number"
                            ? modifyCurrentCPURequest
                            : "—",
                        memory_request_gi:
                          typeof modifyCurrentMemoryRequestGi === "number"
                            ? modifyCurrentMemoryRequestGi
                            : "—",
                      },
                    )}
                  />
                  <Form.Item
                    name="enable_override"
                    valuePropName="checked"
                    label={t("approve_modal.modify_request_override")}
                    extra={t("approve_modal.modify_request_override_help")}
                  >
                    <Switch />
                  </Form.Item>
                  <Card
                    size="small"
                    style={{ marginBottom: 16, background: "#fafafa" }}
                    title={t("approve_modal.modify_request_snapshot_title")}
                  >
                    <Space direction="vertical" style={{ width: "100%" }}>
                      <Space style={{ width: "100%" }}>
                        <Form.Item
                          label={t("approve_modal.cpu_request_current")}
                          style={{ marginBottom: 0, flex: 1 }}
                        >
                          <Input
                            readOnly
                            value={
                              typeof modifyCurrentCPURequest === "number"
                                ? `${modifyCurrentCPURequest} ${t(
                                    "approve_modal.cores",
                                  )}`
                                : "—"
                            }
                          />
                        </Form.Item>
                        <Form.Item
                          label={t("approve_modal.cpu_limit_review")}
                          style={{ marginBottom: 0, flex: 1 }}
                        >
                          <Input
                            readOnly
                            value={
                              typeof modifyTargetCPULimit === "number"
                                ? `${modifyTargetCPULimit} ${t(
                                    "approve_modal.cores",
                                  )}`
                                : "—"
                            }
                          />
                        </Form.Item>
                      </Space>
                      <Space style={{ width: "100%" }}>
                        <Form.Item
                          label={t("approve_modal.memory_request_current")}
                          style={{ marginBottom: 0, flex: 1 }}
                        >
                          <Input
                            readOnly
                            value={
                              typeof modifyCurrentMemoryRequestGi === "number"
                                ? `${modifyCurrentMemoryRequestGi} Gi`
                                : "—"
                            }
                          />
                        </Form.Item>
                        <Form.Item
                          label={t("approve_modal.memory_limit_review")}
                          style={{ marginBottom: 0, flex: 1 }}
                        >
                          <Input
                            readOnly
                            value={
                              typeof modifyTargetMemoryLimitGi === "number"
                                ? `${modifyTargetMemoryLimitGi} Gi`
                                : "—"
                            }
                          />
                        </Form.Item>
                      </Space>
                    </Space>
                  </Card>
                  <Form.Item
                    noStyle
                    shouldUpdate={(prev, cur) =>
                      prev.enable_override !== cur.enable_override ||
                      prev.cpu_request !== cur.cpu_request ||
                      prev.memory_request_gi !== cur.memory_request_gi
                    }
                  >
                    {({ getFieldValue }) =>
                      getFieldValue("enable_override") ? (
                        <Card
                          size="small"
                          style={{ marginBottom: 16, background: "#fafafa" }}
                        >
                          <Space direction="vertical" style={{ width: "100%" }}>
                            <Space style={{ width: "100%" }}>
                              <Form.Item
                                name="cpu_request"
                                label={t("approve_modal.cpu_request")}
                                style={{ marginBottom: 0, flex: 1 }}
                                rules={[
                                  () => ({
                                    validator(_, value) {
                                      if (
                                        typeof value === "number" &&
                                        typeof modifyTargetCPULimit === "number" &&
                                        value > modifyTargetCPULimit
                                      ) {
                                        return Promise.reject(
                                          new Error(
                                            t(
                                              "approve_modal.modify_cpu_request_exceeds_limit",
                                            ),
                                          ),
                                        );
                                      }
                                      return Promise.resolve();
                                    },
                                  }),
                                ]}
                              >
                                <UnitInputNumber
                                  min={0.5}
                                  step={0.5}
                                  precision={1}
                                  unit={t("approve_modal.cores")}
                                />
                              </Form.Item>
                              <Form.Item
                                label={t("approve_modal.cpu_limit_review")}
                                style={{ marginBottom: 0, flex: 1 }}
                              >
                                <Input
                                  readOnly
                                  value={
                                    typeof modifyTargetCPULimit === "number"
                                      ? `${modifyTargetCPULimit} ${t(
                                          "approve_modal.cores",
                                        )}`
                                      : "—"
                                  }
                                />
                              </Form.Item>
                            </Space>
                            <Space style={{ width: "100%" }}>
                              <Form.Item
                                name="memory_request_gi"
                                label={t("approve_modal.memory_request")}
                                style={{ marginBottom: 0, flex: 1 }}
                                rules={[
                                  () => ({
                                    validator(_, value) {
                                      if (
                                        typeof value === "number" &&
                                        typeof modifyTargetMemoryLimitGi ===
                                          "number" &&
                                        value > modifyTargetMemoryLimitGi
                                      ) {
                                        return Promise.reject(
                                          new Error(
                                            t(
                                              "approve_modal.modify_memory_request_exceeds_limit",
                                            ),
                                          ),
                                        );
                                      }
                                      return Promise.resolve();
                                    },
                                  }),
                                ]}
                              >
                                <UnitInputNumber
                                  min={0.5}
                                  step={0.5}
                                  precision={1}
                                  unit="Gi"
                                />
                              </Form.Item>
                              <Form.Item
                                label={t("approve_modal.memory_limit_review")}
                                style={{ marginBottom: 0, flex: 1 }}
                              >
                                <Input
                                  readOnly
                                  value={
                                    typeof modifyTargetMemoryLimitGi === "number"
                                      ? `${modifyTargetMemoryLimitGi} Gi`
                                      : "—"
                                  }
                                />
                              </Form.Item>
                            </Space>
                          </Space>
                        </Card>
                      ) : null
                    }
                  </Form.Item>
                </>
              )
            : approvals.approveModal?.operation_type === "CREATE"
            ? (() => {
                const payload = asPayloadRecord(
                  approvals.approveModal?.ticket_payload,
                );
                const provisioning = approvals.approveModal?.provisioning;
                const defaultDiskGB = instanceSizeDiskGB(payload);
                const dedicatedCPU = instanceSizeDedicatedCPU(payload);
                const rootVolumeResolution =
                  approvals.selectedRootVolumeResolution;
                const rootVolumeModeOptions = approvals.rootVolumeModeOptions;
                const canSelectRootVolumeMode =
                  approvals.canSelectRootVolumeMode;
                const selectedRootVolumeMode =
                  approvals.effectiveSelectedRootVolumeMode;
                const recommendedRootVolumeMode =
                  recommendedRootVolumeModeOption(rootVolumeModeOptions);
                const recommendedRootVolumeModeKey = rootVolumeModeOptionKey(
                  recommendedRootVolumeMode,
                );
                return (
                  <>
                    {approvals.approveCreateContext.hasMixedSelection && (
                      <Alert
                        type="warning"
                        showIcon
                        style={{ marginBottom: 16 }}
                        message={t("approve_modal.batch_scope_mixed_title", {
                          defaultValue: "Mixed batch request",
                        })}
                        description={t(
                          "approve_modal.batch_scope_mixed_description",
                          {
                            defaultValue:
                              "This approval applies to every requested VM. Review the affected items table above for per-item differences before selecting placement inputs below.",
                          },
                        )}
                      />
                    )}
                    {provisioning && (
                      <ApprovalProvisioningCard provisioning={provisioning} />
                    )}
                    {approvals.clusterQueryError && (
                      <Alert
                        type="error"
                        showIcon
                        style={{ marginBottom: 16 }}
                        message={t(
                          "approve_modal.cluster_query_error_title",
                          "Cluster compatibility check failed",
                        )}
                        description={
                          approvals.clusterQueryError.message ||
                          t(
                            "approve_modal.cluster_query_error_description",
                            "The current request context is invalid, so the platform cannot evaluate cluster compatibility yet.",
                          )
                        }
                      />
                    )}
                    <Form.Item
                      name="selected_cluster_id"
                      label={t("approve_modal.cluster")}
                      extra={t("approve_modal.cluster_hint")}
                      getValueFromEvent={(value) => {
                        approvals.handleSelectedClusterChange();
                        return value;
                      }}
                    >
                      <Select
                        placeholder={t("approve_modal.cluster")}
                        options={approvals.clustersData?.items?.map(
                          (cluster: Cluster) => {
                            const compatible =
                              cluster.compatibility?.eligible !== false;
                            const needsStorageClassSelection =
                              requiresStorageClassSelection(cluster);
                            const needsRootVolumeModeSelection =
                              requiresRootVolumeModeSelection(cluster);
                            const needsApprovalInput =
                              needsStorageClassSelection ||
                              needsRootVolumeModeSelection;
                            const disabled =
                              cluster.enabled === false ||
                              (!compatible &&
                                !needsStorageClassSelection &&
                                !needsRootVolumeModeSelection);
                            return {
                              label: (
                                <div>
                                  <Space wrap>
                                    <Text strong>
                                      {cluster.display_name || cluster.name}
                                    </Text>
                                    {cluster.kubevirt_version && (
                                      <Tag color="blue">
                                        KV {cluster.kubevirt_version}
                                      </Tag>
                                    )}
                                    {!compatible && !needsApprovalInput && (
                                      <Tag color="red">
                                        {t(
                                          "approve_modal.cluster_incompatible",
                                          "Incompatible",
                                        )}
                                      </Tag>
                                    )}
                                    {needsStorageClassSelection && (
                                      <Tag color="gold">
                                        {t(
                                          "approve_modal.cluster_requires_storage_class",
                                          "Select storage class",
                                        )}
                                      </Tag>
                                    )}
                                    {needsRootVolumeModeSelection && (
                                      <Tag color="gold">
                                        {t(
                                          "approve_modal.cluster_requires_root_volume_mode",
                                          "Select root volume mode",
                                        )}
                                      </Tag>
                                    )}
                                    {compatible &&
                                      cluster.compatibility?.advisory_code && (
                                        <Tag color="orange">
                                          {t(
                                            "approve_modal.cluster_advisory",
                                            "Clone fallback likely",
                                          )}
                                        </Tag>
                                      )}
                                  </Space>
                                  {compatible &&
                                    cluster.compatibility?.advisory_message && (
                                      <div style={{ marginTop: 4 }}>
                                        <Text
                                          type="warning"
                                          style={{ fontSize: 12 }}
                                        >
                                          {
                                            cluster.compatibility
                                              .advisory_message
                                          }
                                        </Text>
                                      </div>
                                    )}
                                  {!compatible &&
                                    cluster.compatibility?.reason_message && (
                                      <div style={{ marginTop: 4 }}>
                                        <Text
                                          type={
                                            needsStorageClassSelection ||
                                            needsRootVolumeModeSelection
                                              ? "warning"
                                              : "secondary"
                                          }
                                          style={{ fontSize: 12 }}
                                        >
                                          {cluster.compatibility.reason_message}
                                        </Text>
                                      </div>
                                    )}
                                </div>
                              ),
                              value: cluster.id,
                              title: clusterDisplayLabel(cluster),
                              disabled,
                            };
                          },
                        )}
                        labelRender={() => selectedClusterOptionLabel}
                        optionFilterProp="title"
                      />
                    </Form.Item>
                    <Form.Item
                      name="selected_storage_class"
                      label={t("approve_modal.storage_class")}
                      getValueProps={() => ({
                        value:
                          approvals.effectiveSelectedStorageClass || undefined,
                      })}
                      getValueFromEvent={(value) => {
                        approvals.handleSelectedStorageClassChange();
                        return typeof value === "string" && value.trim() !== ""
                          ? value.trim()
                          : undefined;
                      }}
                      rules={[
                        () => ({
                          validator() {
                            if (
                              approvals.selectedCluster &&
                              requiresStorageClassSelection(
                                approvals.selectedCluster,
                              ) &&
                              !approvals.effectiveSelectedStorageClass
                            ) {
                              return Promise.reject(
                                new Error(
                                  t(
                                    "approve_modal.storage_class_required",
                                    "Select a storage class before approving this request.",
                                  ),
                                ),
                              );
                            }
                            return Promise.resolve();
                          },
                        }),
                      ]}
                      extra={
                        approvals.selectedClusterId
                          ? approvals.selectedClusterStorageClassOptions.length > 0
                            ? approvals.selectedClusterStorageClassOptions.length === 1
                              ? t(
                                  "approve_modal.storage_class_auto_detected_single",
                                  "Exactly one eligible storage class was detected for this cluster and is auto-selected.",
                                )
                              : t(
                                  "approve_modal.storage_class_auto_detected_multiple",
                                  "Multiple eligible storage classes were detected. Choose one before approving.",
                                )
                            : t(
                                "approve_modal.storage_class_unavailable",
                                "No storage class was detected for this cluster yet.",
                              )
                          : t(
                              "approve_modal.storage_class_select_cluster_first",
                              "Select a cluster first.",
                            )
                      }
                    >
                      <Select
                        placeholder={t("approve_modal.storage_class")}
                        options={approvals.selectedClusterStorageClassOptions.map(
                          (value) => ({
                            label: value,
                            value,
                          }),
                        )}
                        loading={approvals.selectedClusterPolicyLoading}
                        disabled={!approvals.selectedClusterId}
                        allowClear={
                          approvals.selectedClusterStorageClassOptions.length > 1
                        }
                        showSearch
                        optionFilterProp="label"
                      />
                    </Form.Item>
                    {rootVolumeResolution &&
                      rootVolumeResolution.state !== "not_applicable" && (
                        <Alert
                          type={
                            selectedRootVolumeMode ||
                            rootVolumeResolution.state === "resolved"
                              ? "success"
                              : rootVolumeResolution.state === "profile_incomplete" ||
                                  rootVolumeResolution.state === "unsupported"
                                ? "error"
                                : "warning"
                          }
                          showIcon
                          style={{ marginBottom: 16 }}
                          message={t(
                            `approve_modal.root_volume_resolution.${rootVolumeResolution.state}.title`,
                            {
                              defaultValue:
                                selectedRootVolumeMode
                                  ? "Root volume mode selected"
                                  : rootVolumeResolution.state === "resolved"
                                  ? "Root volume mode resolved"
                                  : rootVolumeResolution.state === "mode_required"
                                    ? "Select root volume mode"
                                    : rootVolumeResolution.state === "storage_class_required"
                                      ? "Select storage class"
                                      : "Root volume resolution blocked",
                            },
                          )}
                          description={
                            selectedRootVolumeMode
                              ? rootVolumeModeOptionLabel(selectedRootVolumeMode)
                              : renderRootVolumeResolutionMessage(
                                  rootVolumeResolution,
                                )
                          }
                        />
                      )}
                    {canSelectRootVolumeMode && (
                      <Form.Item
                        name="selected_root_volume_mode_key"
                        label={t(
                          "approve_modal.root_volume_mode",
                          "Root Volume Mode",
                        )}
                        getValueProps={() => ({
                          value:
                            approvals.effectiveSelectedRootVolumeModeKey ||
                            undefined,
                        })}
                        getValueFromEvent={(value) =>
                          approvals.handleSelectedRootVolumeModeChange(value)
                        }
                        extra={
                          rootVolumeResolution?.intent_mode === "auto"
                            ? t(
                                "approve_modal.root_volume_mode_help",
                                "This specification still uses Auto. The selected cluster exposes multiple StorageProfile combinations, so approval must choose one explicit root volume mode.",
                              )
                            : t(
                                "approve_modal.root_volume_mode_editable_help",
                                "This cluster exposes multiple supported root volume modes. You can adjust the selection before approving.",
                              )
                        }
                        rules={[
                          {
                            required: true,
                            message: t(
                              "approve_modal.root_volume_mode_required",
                              "Select a root volume mode before approving.",
                            ),
                          },
                        ]}
                      >
                        <Select
                          placeholder={t(
                            "approve_modal.root_volume_mode_placeholder",
                            "Select root volume mode",
                          )}
                          options={rootVolumeModeOptions.map((option) => ({
                            label:
                              rootVolumeModeOptionKey(option) ===
                              recommendedRootVolumeModeKey
                                ? `${rootVolumeModeOptionLabel(option)} ${t(
                                    "approve_modal.root_volume_mode_recommended_suffix",
                                    "(Recommended)",
                                  )}`
                                : rootVolumeModeOptionLabel(option),
                            value: rootVolumeModeOptionKey(option),
                          }))}
                        />
                      </Form.Item>
                    )}
                    {canSelectRootVolumeMode && recommendedRootVolumeMode && (
                      <Alert
                        type="info"
                        showIcon
                        style={{ marginTop: -8, marginBottom: 16 }}
                        message={t(
                          "approve_modal.root_volume_mode_recommendation_title",
                          "Recommended root volume mode",
                        )}
                        description={t(
                          "approve_modal.root_volume_mode_recommendation_description",
                          {
                            defaultValue:
                              "{{mode}} is recommended here because it preserves shared-block semantics and is the safest default for migration-friendly VM workloads.",
                            mode: rootVolumeModeOptionLabel(
                              recommendedRootVolumeMode,
                            ),
                          },
                        )}
                      />
                    )}
                    <Form.Item name="selected_dv_access_modes" hidden>
                      <Select mode="multiple" options={[]} />
                    </Form.Item>
                    <Form.Item name="selected_dv_volume_mode" hidden>
                      <Input />
                    </Form.Item>
                    <Form.Item
                      name="enable_override"
                      valuePropName="checked"
                      label={t("approve_modal.enable_override")}
                    >
                      <Switch />
                    </Form.Item>
                    <Form.Item
                      noStyle
                      shouldUpdate={(prev, cur) =>
                        prev.enable_override !== cur.enable_override
                      }
                    >
                      {({ getFieldValue }) =>
                        getFieldValue("enable_override") ? (
                          <Card
                            size="small"
                            style={{ marginBottom: 16, background: "#fafafa" }}
                          >
                            <Space
                              direction="vertical"
                              style={{ width: "100%" }}
                            >
                              <Form.Item
                                name="disk_gb"
                                label={t("approve_modal.disk_gb")}
                                extra={
                                  typeof defaultDiskGB === "number"
                                    ? `${t("approve_modal.default_disk_hint", "Leave empty to use instance size default")}: ${defaultDiskGB} GB`
                                    : undefined
                                }
                                style={{ marginBottom: 0 }}
                              >
                                <UnitInputNumber min={1} max={500} step={1} precision={0} unit="GB" />
                              </Form.Item>
                              <Space style={{ width: "100%" }}>
                                <Form.Item
                                  name="cpu_request"
                                  label={t("approve_modal.cpu_request")}
                                  style={{ marginBottom: 0, flex: 1 }}
                                  dependencies={["cpu_limit"]}
                                  rules={[
                                    ({ getFieldValue }) => ({
                                      validator(_, value) {
                                        const lim = getFieldValue("cpu_limit");
                                        if (
                                          dedicatedCPU &&
                                          value &&
                                          lim &&
                                          value !== lim
                                        ) {
                                          return Promise.reject(
                                            new Error(
                                              t(
                                                "approve_modal.dedicated_cpu_no_overcommit",
                                                "Dedicated CPU requires request == limit",
                                              ),
                                            ),
                                          );
                                        }
                                        return Promise.resolve();
                                      },
                                    }),
                                  ]}
                                >
                                  <UnitInputNumber
                                    min={0.5}
                                    step={0.5}
                                    precision={1}
                                    unit={t("approve_modal.cores")}
                                  />
                                </Form.Item>
                                <Form.Item
                                  name="cpu_limit"
                                  label={t("approve_modal.cpu_limit")}
                                  style={{ marginBottom: 0, flex: 1 }}
                                  dependencies={["cpu_request"]}
                                  rules={[
                                    ({ getFieldValue }) => ({
                                      validator(_, value) {
                                        const req =
                                          getFieldValue("cpu_request");
                                        if (
                                          dedicatedCPU &&
                                          req &&
                                          value &&
                                          req !== value
                                        ) {
                                          return Promise.reject(
                                            new Error(
                                              t(
                                                "approve_modal.dedicated_cpu_no_overcommit",
                                                "Dedicated CPU requires request == limit",
                                              ),
                                            ),
                                          );
                                        }
                                        return Promise.resolve();
                                      },
                                    }),
                                  ]}
                                >
                                  <UnitInputNumber
                                    min={0.5}
                                    step={0.5}
                                    precision={1}
                                    unit={t("approve_modal.cores")}
                                  />
                                </Form.Item>
                              </Space>
                              <Space style={{ width: "100%" }}>
                                <Form.Item
                                  name="memory_request_gi"
                                  label={t("approve_modal.memory_request")}
                                  style={{ marginBottom: 0, flex: 1 }}
                                >
                                  <UnitInputNumber
                                    min={0.5}
                                    step={0.5}
                                    precision={1}
                                    unit="Gi"
                                  />
                                </Form.Item>
                                <Form.Item
                                  name="memory_limit_gi"
                                  label={t("approve_modal.memory_limit")}
                                  style={{ marginBottom: 0, flex: 1 }}
                                >
                                  <UnitInputNumber
                                    min={0.5}
                                    step={0.5}
                                    precision={1}
                                    unit="Gi"
                                  />
                                </Form.Item>
                              </Space>
                            </Space>
                            <Form.Item
                              noStyle
                              shouldUpdate={(prev, cur) =>
                                prev.cpu_request !== cur.cpu_request ||
                                prev.cpu_limit !== cur.cpu_limit ||
                                prev.memory_request_gi !==
                                  cur.memory_request_gi ||
                                prev.memory_limit_gi !== cur.memory_limit_gi
                              }
                            >
                              {({ getFieldValue: gfv }) => {
                                const cpuReq = gfv("cpu_request");
                                const cpuLim = gfv("cpu_limit");
                                const memReq = gfv("memory_request_gi");
                                const memLim = gfv("memory_limit_gi");
                                const isOvercommit =
                                  (cpuReq && cpuLim && cpuReq !== cpuLim) ||
                                  (memReq && memLim && memReq !== memLim);
                                if (!isOvercommit) return null;
                                return (
                                  <div
                                    style={{
                                      padding: "8px 12px",
                                      marginTop: 8,
                                      background: "#fffbe6",
                                      border: "1px solid #ffe58f",
                                      borderRadius: 6,
                                    }}
                                  >
                                    <Space>
                                      <ExclamationCircleOutlined
                                        style={{ color: "#faad14" }}
                                      />
                                      <Text type="warning">
                                        {t("approve_modal.overcommit_warning")}
                                      </Text>
                                    </Space>
                                  </div>
                                );
                              }}
                            </Form.Item>
                          </Card>
                        ) : null
                      }
                    </Form.Item>
                  </>
                );
              })()
            : null}
          <Form.Item name="comment" label={t("approve_modal.comment")}>
            <Input.TextArea rows={3} />
          </Form.Item>
        </Form>
      </WorkbenchDetailModal>
      ) : null}

      {approvals.rejectModal ? (
      <WorkbenchDetailModal
        title={t("reject_modal.title")}
        open={Boolean(approvals.rejectModal)}
        onOk={() => {
          void approvals.submitReject();
        }}
        onCancel={approvals.closeRejectModal}
        confirmLoading={approvals.rejectPending}
        width="min(720px, calc(100vw - 16px))"
        contentMinWidth={560}
        data-testid="reject-modal"
      >
        <Form
          form={approvals.rejectForm}
          layout="vertical"
          name="reject-form"
          preserve={false}
        >
          <Form.Item
            name="reason"
            label={t("reject_modal.reason")}
            rules={[
              { required: true, message: t("reject_modal.reason_required") },
            ]}
          >
            <Input.TextArea
              rows={4}
              placeholder={t("reject_modal.reason_placeholder")}
            />
          </Form.Item>
        </Form>
      </WorkbenchDetailModal>
      ) : null}
    </div>
  );
}
