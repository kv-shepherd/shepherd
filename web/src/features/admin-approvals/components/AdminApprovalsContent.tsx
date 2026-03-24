"use client";

import {
  Alert,
  Badge,
  Button,
  Card,
  Descriptions,
  Form,
  Input,
  Modal,
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
import {
  AuditOutlined,
  DeleteOutlined,
  ExclamationCircleOutlined,
  ReloadOutlined,
} from "@ant-design/icons";
import { useRouter } from "next/navigation";
import { useTranslation } from "react-i18next";

import { ActionEmptyState } from "@/components/feedback/ActionEmptyState";
import { SummaryMetricCard } from "@/components/feedback/SummaryMetricCard";
import {
  QueueReviewGlyph,
  RequestsOverviewGlyph,
  ServiceWorkspaceGlyph,
  VirtualMachinesOverviewGlyph,
} from "@/components/illustrations/DashboardIllustrations";
import { PageHeader, PageSurface } from "@/components/layouts/PageSection";
import { SetupGuideCard } from "@/features/setup-guide/components/SetupGuideCard";
import { useSetupGuide } from "@/features/setup-guide/hooks/useSetupGuide";
import { translateApiError } from "@/lib/api/errorMessage";
import {
  approvalPrimaryAlert,
  approvalSummaryMeta,
  approvalSummaryTitle,
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

function payloadBool(value: unknown): boolean | undefined {
  return typeof value === "boolean" ? value : undefined;
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
  const setupGuide = useSetupGuide();
  const modalBodyStyles = {
    maxHeight: "calc(100vh - 220px)",
    overflowY: "auto" as const,
    overflowX: "hidden" as const,
    paddingRight: 8,
  };
  const modalViewportStyle = { top: 16 } as const;
  const approveModalWidth = "min(1040px, calc(100vw - 16px))";
  const rejectModalWidth = "min(720px, calc(100vw - 16px))";
  const approvalSectionGridStyles = {
    display: "grid",
    gap: 16,
    gridTemplateColumns: "repeat(auto-fit, minmax(240px, 1fr))",
    marginBottom: 16,
  } as const;
  const selectedClusterOptionLabel = clusterDisplayLabel(approvals.selectedCluster);
  const pageItems = approvals.data?.items ?? [];
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
    approvals.operationFilter === "ALL" &&
    approvals.selectedClusterFilter.trim() === "" &&
    approvals.placementAdvisoryFilter.trim() === "" &&
    approvals.placementSnapshotFilter === "ALL";
  const isQueueEmpty = pageItems.length === 0 && !approvals.isLoading;
  const shouldShowSetupEmpty = isQueueEmpty && isDefaultQueueState;
  const approveAlert = approvals.approveModal
    ? approvalPrimaryAlert(approvals.approveModal, t)
    : null;
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

  const columns: ColumnsType<ApprovalTask> = [
    {
      title: t("request_summary"),
      key: "request_summary",
      width: 280,
      render: (_, record) => {
        const summaryMeta = approvalSummaryMeta(record, t);
        return (
          <Space direction="vertical" size={0}>
            <Space size={8}>
              <AuditOutlined style={{ color: "#d4380d" }} />
              <Text strong>{approvalSummaryTitle(record, t)}</Text>
            </Space>
            {summaryMeta.length > 0 && (
              <Text type="secondary" style={{ fontSize: 12 }}>
                {summaryMeta.join(" · ")}
              </Text>
            )}
            <Text copyable={{ text: record.id }} type="secondary" style={{ fontSize: 12 }}>
              {t("ticket_id")}: {formatApprovalRecordID(record.id)}
            </Text>
          </Space>
        );
      },
    },
    {
      title: t("operation_type"),
      dataIndex: "operation_type",
      key: "operation_type",
      width: 110,
      render: (opType: ApprovalTask["operation_type"], record) => {
        const config =
          OP_TYPE_CONFIG[opType ?? "CREATE"] ?? OP_TYPE_CONFIG.CREATE;
        const Icon = config.icon;
        const itemCount = record.summary?.batch_count ||
          batchPayloadItems(record.ticket_payload as PayloadRecord | undefined).length;
        return (
          <Space size={[0, 4]} wrap>
            <Tag color={config.color} icon={<Icon />}>
              {t(`op_type.${opType ?? "CREATE"}`)}
            </Tag>
            {itemCount > 0 && (
              <Tag color="gold">
                {t("batch.child_count", {
                  defaultValue: "{{count}} items",
                  count: itemCount,
                })}
              </Tag>
            )}
          </Space>
        );
      },
    },
    {
      title: t("target_vm"),
      key: "target_vm",
      width: 160,
      render: (_, record) => {
        const targetVM =
          record.summary?.vm_name ||
          record.target_vm_name ||
          record.summary?.vm_id;
        if (targetVM) {
          return (
            <Space>
              <DeleteOutlined style={{ color: "#cf1322" }} />
              <Text strong style={{ color: "#cf1322" }}>
                {targetVM}
              </Text>
            </Space>
          );
        }
        return <Text type="secondary">—</Text>;
      },
    },
    {
      title: t("selected_cluster", "Selected Cluster"),
      key: "selected_cluster",
      width: 180,
      render: (_, record) => {
        const placement = record.placement_evaluation;
        const clusterDisplay =
          placement?.selected_cluster_name ||
          placement?.selected_cluster_id ||
          record.summary?.cluster_name ||
          record.summary?.cluster_id;
        if (!clusterDisplay) {
          return <Text type="secondary">—</Text>;
        }
        return (
          <Space direction="vertical" size={0}>
            <Text strong>{clusterDisplay}</Text>
            {placement?.advisory_code && (
              <Text type="warning" style={{ fontSize: 12 }}>
                {placement.advisory_code}
              </Text>
            )}
            {placement?.selected_cluster_name &&
              placement.selected_cluster_name !==
                placement.selected_cluster_id && (
                <Text type="secondary" style={{ fontSize: 12 }}>
                  {placement.selected_cluster_id}
                </Text>
              )}
          </Space>
        );
      },
    },
    {
      title: t("approve_modal.provisioning.title", "Provisioning Status"),
      key: "provisioning",
      width: 190,
      render: (_, record) => {
        if (record.operation_type !== "CREATE" || !record.provisioning) {
          return <Text type="secondary">—</Text>;
        }
        const provisioning = record.provisioning;
        return (
          <Space
            direction="vertical"
            size={2}
            data-testid={`approval-provisioning-summary-${record.id}`}
          >
            <Tag color={getProvisioningPhaseTagColor(provisioning.phase)}>
              {provisioning.phase || "—"}
            </Tag>
            {provisioning.clone_type === "copy" && (
              <Tag color={getCloneTypeTagColor(provisioning.clone_type)}>
                {t(
                  "approve_modal.provisioning.clone_type_copy",
                  "Host-assisted copy",
                )}
              </Tag>
            )}
            {provisioning.failure_message ? (
              <Text type="danger" style={{ fontSize: 12 }}>
                {provisioning.failure_message}
              </Text>
            ) : provisioning.progress ? (
              <Text type="secondary" style={{ fontSize: 12 }}>
                {provisioning.progress}
              </Text>
            ) : null}
          </Space>
        );
      },
    },
    {
      title: t("common:table.status"),
      dataIndex: "status",
      key: "status",
      width: 120,
      render: (status: ApprovalTask["status"]) => (
        <Badge
          status={STATUS_BADGES[status] ?? "default"}
          text={
            <Tag color={STATUS_COLORS[status]}>{t(`status.${status}`)}</Tag>
          }
        />
      ),
    },
    {
      title: t("requester"),
      dataIndex: "requester",
      key: "requester",
      width: 140,
    },
    {
      title: t("reason"),
      dataIndex: "reason",
      key: "reason",
      ellipsis: true,
      render: (reason: string) => <Text type="secondary">{reason || "—"}</Text>,
    },
    {
      title: t("approver"),
      dataIndex: "approver",
      key: "approver",
      width: 140,
      render: (approver: string) => (
        <Text type="secondary">{approver || "—"}</Text>
      ),
    },
    {
      title: t("common:table.created_at"),
      dataIndex: "created_at",
      key: "created_at",
      width: 160,
      render: (date: string) => (
        <Text type="secondary">
          <LocalDateTimeText value={date} />
        </Text>
      ),
    },
    {
      title: t("common:table.actions"),
      key: "actions",
      width: 160,
      render: (_, record) => {
        if (record.status !== "PENDING") {
          return <Text type="secondary">—</Text>;
        }
        return (
          <Space size={4} wrap>
            <Button
              type="primary"
              size="small"
              icon={<AuditOutlined />}
              data-testid={`approval-action-approve-${record.id}`}
              onClick={() => approvals.openApproveModal(record)}
            >
              {t("action.review")}
            </Button>
            <Button
              type="link"
              size="small"
              danger
              data-testid={`approval-action-reject-${record.id}`}
              onClick={() => approvals.openRejectModal(record)}
            >
              {t("common:button.reject")}
            </Button>
            <Popconfirm
              title={t("cancel_confirm")}
              onConfirm={() => approvals.submitCancel(record.id)}
              okText={t("common:button.confirm")}
              cancelText={t("common:button.cancel")}
            >
              <Button
                type="link"
                size="small"
                data-testid={`approval-action-cancel-${record.id}`}
                loading={approvals.cancelPending}
              >
                {t("cancel")}
              </Button>
            </Popconfirm>
          </Space>
        );
      },
    },
  ];

  return (
    <div data-testid="admin-approvals-page">
      {approvals.messageContextHolder}
      <PageHeader
        title={t("common:nav.approval_tasks")}
        subtitle={t("subtitle")}
        actions={(
          <Space>
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
      />
      <div className="summary-card-grid">
        <SummaryMetricCard
          title={t("summary.pending_title")}
          value={pendingOnPage}
          description={t("summary.pending_description")}
          visual={<QueueReviewGlyph className="summary-metric-card__art" />}
          accentColor="#D97706"
          surfaceColor="#FFF4E5"
        />
        <SummaryMetricCard
          title={t("summary.urgent_title")}
          value={<span style={{ color: urgentOnPage > 0 ? "#d4380d" : undefined }}>{urgentOnPage}</span>}
          description={t("summary.urgent_description")}
          visual={<RequestsOverviewGlyph className="summary-metric-card__art" />}
          accentColor="#CF1322"
          surfaceColor="#FFF1F0"
        />
        <SummaryMetricCard
          title={t("summary.create_title")}
          value={createOnPage}
          description={t("summary.create_description")}
          visual={<ServiceWorkspaceGlyph className="summary-metric-card__art" />}
          accentColor="#1D5BFF"
          surfaceColor="#E6F4FF"
        />
        <SummaryMetricCard
          title={t("summary.provisioning_title")}
          value={provisioningVisible}
          description={t("summary.provisioning_description")}
          visual={<VirtualMachinesOverviewGlyph className="summary-metric-card__art" />}
          accentColor="#6D4DE3"
          surfaceColor="#F5EDFF"
        />
      </div>
      <PageSurface style={{ marginBottom: 16 }}>
        <Space direction="vertical" size={12} style={{ width: "100%" }}>
          {approvals.listError && (
            <Alert
              type="error"
              showIcon
              message={t("list.load_failed_title")}
              description={translateApiError(t, approvals.listError)}
            />
          )}
          <Space direction="vertical" size={2}>
            <Text strong>{t("triage.title")}</Text>
            <Text type="secondary">{t("triage.description")}</Text>
          </Space>
          <Space wrap size={12}>
          <Select
            value={approvals.operationFilter}
            onChange={(value) =>
              approvals.changeOperationFilter(
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
            value={approvals.placementSnapshotFilter}
            onChange={(value) =>
              approvals.changePlacementSnapshotFilter(
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
          <Input
            allowClear
            value={approvals.selectedClusterFilter}
            onChange={(event) =>
              approvals.changeSelectedClusterFilter(event.target.value)
            }
            placeholder={t("filter.selected_cluster", "Filter by cluster ID")}
            style={{ width: 240 }}
          />
          <Input
            allowClear
            value={approvals.placementAdvisoryFilter}
            onChange={(event) =>
              approvals.changePlacementAdvisoryFilter(event.target.value)
            }
            placeholder={t(
              "filter.placement_advisory",
              "Filter by placement advisory",
            )}
            style={{ width: 260 }}
          />
          </Space>
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
              <Button onClick={approvals.resetFilters}>
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
            scroll={{ x: 1280 }}
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

      <Modal
        title={
          approvals.approveModal?.operation_type === "DELETE"
            ? t("approve_modal.delete_title")
            : t("approve_modal.title")
        }
        width={approveModalWidth}
        style={modalViewportStyle}
        open={Boolean(approvals.approveModal)}
        onOk={() => {
          void approvals.submitApprove();
        }}
        onCancel={approvals.closeApproveModal}
        confirmLoading={approvals.approvePending}
        forceRender={true}
        wrapClassName="admin-approvals-modal"
        styles={{ body: modalBodyStyles }}
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
          {(approveOverviewItems.length > 0 ||
            approveScopeItems.length > 0 ||
            approveChangeItems.length > 0) && (
            <div
              className="admin-approvals-modal__sections"
              style={approvalSectionGridStyles}
            >
              {approveOverviewItems.length > 0 && (
                <Card size="small" title={t("summary.overview_title")}>
                  <Descriptions
                    bordered
                    size="small"
                    column={1}
                    items={approveOverviewItems}
                  />
                </Card>
              )}
              {approveScopeItems.length > 0 && (
                <Card size="small" title={t("summary.scope_title")}>
                  <Descriptions
                    bordered
                    size="small"
                    column={1}
                    items={approveScopeItems}
                  />
                </Card>
              )}
              {approveChangeItems.length > 0 && (
                <Card size="small" title={t("summary.change_title")}>
                  <Descriptions
                    bordered
                    size="small"
                    column={1}
                    items={approveChangeItems}
                  />
                </Card>
              )}
            </div>
          )}
          {approveBatchItems.length > 0 && (
            <Card
              size="small"
              style={{ marginBottom: 16 }}
              title={t("summary.affected_items_title")}
            >
              <Table
                size="small"
                pagination={false}
                rowKey="key"
                dataSource={approveBatchItems}
                scroll={{ x: 920, y: 280 }}
                columns={[
                  {
                    title: t("summary.item"),
                    dataIndex: "title",
                    key: "title",
                  },
                  {
                    title: t("summary.scope"),
                    dataIndex: "scope",
                    key: "scope",
                    render: (value: string | undefined) => value || "—",
                  },
                  {
                    title: t("summary.cluster"),
                    dataIndex: "cluster",
                    key: "cluster",
                    render: (value: string | undefined) => value || "—",
                  },
                  {
                    title: t("summary.request_vm_status"),
                    dataIndex: "requestStatus",
                    key: "requestStatus",
                    render: (value: string | undefined) => value || "—",
                  },
                  {
                    title: t("summary.latest_vm_status"),
                    dataIndex: "latestStatus",
                    key: "latestStatus",
                    render: (value: string | undefined, record) => (
                      <Space direction="vertical" size={0}>
                        <span>{value || "—"}</span>
                        {record.statusChanged && (
                          <Text type="warning" style={{ fontSize: 12 }}>
                            {t("summary.status_changed")}
                          </Text>
                        )}
                      </Space>
                    ),
                  },
                  {
                    title: t("summary.current_resources"),
                    dataIndex: "currentShape",
                    key: "currentShape",
                    render: (value: string | undefined) => value || "—",
                  },
                  {
                    title: t("summary.target_resources"),
                    dataIndex: "targetShape",
                    key: "targetShape",
                    render: (value: string | undefined) => value || "—",
                  },
                  {
                    title: t("summary.power_action"),
                    dataIndex: "action",
                    key: "action",
                    render: (value: string | undefined) => value || "—",
                  },
                ]}
              />
            </Card>
          )}
          {approvals.approveModal?.operation_type === "CREATE"
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
      </Modal>

      <Modal
        title={t("reject_modal.title")}
        width={rejectModalWidth}
        style={modalViewportStyle}
        open={Boolean(approvals.rejectModal)}
        onOk={() => {
          void approvals.submitReject();
        }}
        onCancel={approvals.closeRejectModal}
        confirmLoading={approvals.rejectPending}
        forceRender={true}
        wrapClassName="admin-approvals-modal"
        styles={{ body: modalBodyStyles }}
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
      </Modal>
    </div>
  );
}
