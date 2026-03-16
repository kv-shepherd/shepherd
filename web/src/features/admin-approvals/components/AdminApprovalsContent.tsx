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
  CheckCircleOutlined,
  CloseCircleOutlined,
  DeleteOutlined,
  ExclamationCircleOutlined,
  ReloadOutlined,
} from "@ant-design/icons";
import { useTranslation } from "react-i18next";

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
  type ApprovalTicket,
  type Cluster,
} from "../types";

const { Title, Text } = Typography;
type PayloadRecord = Record<string, unknown>;
type RootVolumeResolution = NonNullable<
  NonNullable<Cluster["compatibility"]>["root_volume_resolution"]
>;
type RootVolumeModeOption = NonNullable<RootVolumeResolution["mode_options"]>[number];

/** Safely convert an unknown ticket_payload field to a displayable string. */
function toStr(value: unknown): string {
  if (value === null || value === undefined) return "—";
  if (typeof value === "string") return value || "—";
  return String(value);
}

function formatTicketID(id: string): string {
  if (id.length <= 14) {
    return id;
  }
  return `${id.slice(0, 8)}…${id.slice(-4)}`;
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

function payloadString(value: unknown): string | undefined {
  return typeof value === "string" && value.trim() !== ""
    ? value.trim()
    : undefined;
}

function payloadNumber(value: unknown): number | undefined {
  return typeof value === "number" && Number.isFinite(value)
    ? value
    : undefined;
}

function payloadBool(value: unknown): boolean | undefined {
  return typeof value === "boolean" ? value : undefined;
}

function templateLabel(payload: PayloadRecord | undefined): string {
  return toStr(
    payload?.template_label ??
      payload?.template_display_name ??
      payload?.template_name ??
      payload?.template_id,
  );
}

function instanceSizeLabel(payload: PayloadRecord | undefined): string {
  return toStr(
    payload?.instance_size_label ??
      payload?.instance_size_display_name ??
      payload?.instance_size_name ??
      payload?.instance_size_id,
  );
}

function instanceSizeDiskGB(
  payload: PayloadRecord | undefined,
): number | undefined {
  return payloadNumber(payload?.instance_size_disk_gb);
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

function batchSummary(
  payload: PayloadRecord | undefined,
): PayloadRecord | undefined {
  return asPayloadRecord(payload?.batch_summary);
}

function distinctPayloadStrings(
  items: PayloadRecord[],
  selector: (item: PayloadRecord) => string,
): string[] {
  const values = new Set<string>();
  for (const item of items) {
    const value = selector(item).trim();
    if (value !== "" && value !== "—") {
      values.add(value);
    }
  }
  return Array.from(values);
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
  const { t } = useTranslation(["approval", "common"]);
  const approvals = useAdminApprovalsController({ t });
  const selectedClusterOptionLabel = clusterDisplayLabel(approvals.selectedCluster);

  const columns: ColumnsType<ApprovalTicket> = [
    {
      title: t("ticket_id"),
      dataIndex: "id",
      key: "id",
      width: 120,
      render: (id: string) => (
        <Space>
          <AuditOutlined style={{ color: "#d4380d" }} />
          <Text copyable={{ text: id }} style={{ fontSize: 12 }}>
            {formatTicketID(id)}
          </Text>
        </Space>
      ),
    },
    {
      title: t("operation_type"),
      dataIndex: "operation_type",
      key: "operation_type",
      width: 110,
      render: (opType: ApprovalTicket["operation_type"], record) => {
        const config =
          OP_TYPE_CONFIG[opType ?? "CREATE"] ?? OP_TYPE_CONFIG.CREATE;
        const Icon = config.icon;
        const itemCount = batchPayloadItems(
          record.ticket_payload as PayloadRecord | undefined,
        ).length;
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
        if (record.operation_type === "DELETE" && record.target_vm_name) {
          return (
            <Space>
              <DeleteOutlined style={{ color: "#cf1322" }} />
              <Text strong style={{ color: "#cf1322" }}>
                {record.target_vm_name}
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
        if (!placement?.selected_cluster_id) {
          return <Text type="secondary">—</Text>;
        }
        const displayName =
          placement.selected_cluster_name || placement.selected_cluster_id;
        return (
          <Space direction="vertical" size={0}>
            <Text strong>{displayName}</Text>
            {placement.advisory_code && (
              <Text type="warning" style={{ fontSize: 12 }}>
                {placement.advisory_code}
              </Text>
            )}
            {placement.selected_cluster_name &&
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
      render: (status: ApprovalTicket["status"]) => (
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
          <Space>
            <Button
              type="primary"
              size="small"
              icon={<CheckCircleOutlined />}
              data-testid={`approval-action-approve-${record.id}`}
              onClick={() => approvals.openApproveModal(record)}
            >
              {t("common:button.approve")}
            </Button>
            <Button
              danger
              size="small"
              icon={<CloseCircleOutlined />}
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
                size="small"
                icon={<ExclamationCircleOutlined />}
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
      <div
        style={{
          display: "flex",
          justifyContent: "space-between",
          alignItems: "center",
          marginBottom: 24,
        }}
      >
        <div>
          <Title level={4} style={{ margin: 0 }}>
            {t("title")}
          </Title>
          <Text type="secondary">{t("subtitle")}</Text>
        </div>
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
      </div>
      <Card style={{ borderRadius: 12, marginBottom: 16 }}>
        <Space wrap size={12}>
          <Select
            value={approvals.operationFilter}
            onChange={(value) =>
              approvals.changeOperationFilter(
                value as "ALL" | ApprovalTicket["operation_type"],
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
      </Card>

      <Card style={{ borderRadius: 12 }} styles={{ body: { padding: 0 } }}>
        {/* ADR-0015 §11: Priority tier highlighting styles */}
        <style>{`
                    .approval-row-urgent td { background-color: rgba(255, 77, 79, 0.06) !important; }
                    .approval-row-warning td { background-color: rgba(250, 173, 20, 0.06) !important; }
                `}</style>
        <Table<ApprovalTicket>
          columns={columns}
          dataSource={approvals.data?.items ?? []}
          rowKey="id"
          loading={approvals.isLoading}
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
      </Card>

      <Modal
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
        forceRender
        data-testid="approve-modal"
      >
        <Form
          form={approvals.approveForm}
          layout="vertical"
          name="approve-form"
        >
          {approvals.approveModal?.operation_type === "CREATE"
            ? (() => {
                const payload = asPayloadRecord(
                  approvals.approveModal?.ticket_payload,
                );
                const provisioning = approvals.approveModal?.provisioning;
                const batchItems = batchPayloadItems(payload);
                const summary = batchSummary(payload);
                const defaultDiskGB = instanceSizeDiskGB(payload);
                const dedicatedCPU = instanceSizeDedicatedCPU(payload);
                const namespaceValues = distinctPayloadStrings(
                  batchItems,
                  (item) => payloadString(item.namespace) ?? "",
                );
                const templateValues = distinctPayloadStrings(
                  batchItems,
                  (item) => templateLabel(item),
                );
                const instanceSizeValues = distinctPayloadStrings(
                  batchItems,
                  (item) => instanceSizeLabel(item),
                );
                const batchTableData = batchItems.map((item, index) => ({
                  ...item,
                  __rowKey: `batch-item-${index}`,
                }));
                const commonNamespace =
                  payloadString(payload?.namespace) ??
                  (namespaceValues.length === 1
                    ? namespaceValues[0]
                    : undefined);
                const commonTemplate =
                  batchItems.length > 0
                    ? templateValues.length === 1
                      ? templateValues[0]
                      : t("approve_modal.mixed_value", {
                          defaultValue: "Mixed",
                        })
                    : templateLabel(payload);
                const commonInstanceSize =
                  batchItems.length > 0
                    ? instanceSizeValues.length === 1
                      ? instanceSizeValues[0]
                      : t("approve_modal.mixed_value", {
                          defaultValue: "Mixed",
                        })
                    : instanceSizeLabel(payload);
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
                    {payload && batchItems.length === 0 && (
                      <Descriptions
                        bordered
                        size="small"
                        column={1}
                        style={{ marginBottom: 16 }}
                        title={t(
                          "approve_modal.request_details",
                          "Request Details",
                        )}
                      >
                        <Descriptions.Item
                          label={t("approve_modal.namespace", "Namespace")}
                        >
                          {toStr(payload.namespace)}
                        </Descriptions.Item>
                        <Descriptions.Item
                          label={t("approve_modal.template", "Template")}
                        >
                          {templateLabel(payload)}
                        </Descriptions.Item>
                        <Descriptions.Item
                          label={t(
                            "approve_modal.instance_size",
                            "Instance Size",
                          )}
                        >
                          {instanceSizeLabel(payload)}
                        </Descriptions.Item>
                        <Descriptions.Item
                          label={t(
                            "approve_modal.dedicated_cpu",
                            "Dedicated CPU",
                          )}
                        >
                          {dedicatedCPU ? (
                            <Tag color="blue">{t("common:yes", "Yes")}</Tag>
                          ) : (
                            <Text type="secondary">{t("common:no", "No")}</Text>
                          )}
                        </Descriptions.Item>
                        <Descriptions.Item
                          label={t(
                            "approve_modal.default_disk_gb",
                            "Default Disk",
                          )}
                        >
                          {typeof defaultDiskGB === "number"
                            ? `${defaultDiskGB} GB`
                            : "—"}
                        </Descriptions.Item>
                      </Descriptions>
                    )}
                    {payload && batchItems.length > 0 && (
                      <>
                        <Descriptions
                          bordered
                          size="small"
                          column={1}
                          style={{ marginBottom: 12 }}
                          title={t(
                            "approve_modal.batch_scope_title",
                            "Batch Request",
                          )}
                        >
                          <Descriptions.Item
                            label={t(
                              "approve_modal.batch_item_count",
                              "Requested VMs",
                            )}
                          >
                            {payloadNumber(payload.batch_item_count) ??
                              batchItems.length}
                          </Descriptions.Item>
                          <Descriptions.Item
                            label={t("approve_modal.namespace", "Namespace")}
                          >
                            {commonNamespace ??
                              t("approve_modal.mixed_value", {
                                defaultValue: "Mixed",
                              })}
                          </Descriptions.Item>
                          <Descriptions.Item
                            label={t("approve_modal.template", "Template")}
                          >
                            {commonTemplate}
                          </Descriptions.Item>
                          <Descriptions.Item
                            label={t(
                              "approve_modal.instance_size",
                              "Instance Size",
                            )}
                          >
                            {commonInstanceSize}
                          </Descriptions.Item>
                          {summary && (
                            <Descriptions.Item
                              label={t(
                                "approve_modal.batch_status",
                                "Batch Status",
                              )}
                            >
                              <Space wrap>
                                <Tag color="blue">{toStr(summary.status)}</Tag>
                                <Text type="secondary">
                                  {t("approve_modal.batch_status_counts", {
                                    defaultValue:
                                      "{{success}} success / {{failed}} failed / {{pending}} pending",
                                    success:
                                      payloadNumber(summary.success_count) ?? 0,
                                    failed:
                                      payloadNumber(summary.failed_count) ?? 0,
                                    pending:
                                      payloadNumber(summary.pending_count) ?? 0,
                                  })}
                                </Text>
                              </Space>
                            </Descriptions.Item>
                          )}
                        </Descriptions>
                        <Alert
                          type={
                            approvals.approveCreateContext.hasMixedSelection
                              ? "warning"
                              : "info"
                          }
                          showIcon
                          style={{ marginBottom: 12 }}
                          message={
                            approvals.approveCreateContext.hasMixedSelection
                              ? t("approve_modal.batch_scope_mixed_title", {
                                  defaultValue: "Mixed batch request",
                                })
                              : t("approve_modal.batch_scope_description", {
                                  defaultValue:
                                    "One approval decision will apply to every VM in this batch.",
                                })
                          }
                          description={
                            approvals.approveCreateContext.hasMixedSelection
                              ? t(
                                  "approve_modal.batch_scope_mixed_description",
                                  {
                                    defaultValue:
                                      "This batch mixes namespaces, templates, or instance sizes. Cluster compatibility hints are broad, but the approval action still applies to the full parent ticket.",
                                  },
                                )
                              : t(
                                  "approve_modal.batch_scope_simple_description",
                                  {
                                    defaultValue:
                                      "Review the common request details below and verify the item list before approving.",
                                  },
                                )
                          }
                        />
                        <Table<PayloadRecord & { __rowKey: string }>
                          size="small"
                          pagination={false}
                          rowKey="__rowKey"
                          dataSource={batchTableData}
                          style={{ marginBottom: 16 }}
                          scroll={{ x: 720 }}
                          columns={[
                            {
                              title: t("approve_modal.batch_item_index", "#"),
                              key: "index",
                              width: 70,
                              render: (_, __, index) => index + 1,
                            },
                            {
                              title: t("approve_modal.namespace", "Namespace"),
                              key: "namespace",
                              render: (_, record) => toStr(record.namespace),
                            },
                            {
                              title: t("approve_modal.template", "Template"),
                              key: "template",
                              render: (_, record) => templateLabel(record),
                            },
                            {
                              title: t(
                                "approve_modal.instance_size",
                                "Instance Size",
                              ),
                              key: "instance_size",
                              render: (_, record) => instanceSizeLabel(record),
                            },
                            {
                              title: t(
                                "approve_modal.dedicated_cpu",
                                "Dedicated CPU",
                              ),
                              key: "dedicated_cpu",
                              width: 130,
                              render: (_, record) =>
                                instanceSizeDedicatedCPU(record) ? (
                                  <Tag color="blue">
                                    {t("common:yes", "Yes")}
                                  </Tag>
                                ) : (
                                  <Text type="secondary">
                                    {t("common:no", "No")}
                                  </Text>
                                ),
                            },
                            {
                              title: t(
                                "approve_modal.default_disk_gb",
                                "Default Disk",
                              ),
                              key: "disk",
                              width: 120,
                              render: (_, record) => {
                                const diskGB = instanceSizeDiskGB(record);
                                return typeof diskGB === "number"
                                  ? `${diskGB} GB`
                                  : "—";
                              },
                            },
                          ]}
                        />
                      </>
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
          {approvals.approveModal?.operation_type === "DELETE" && (
            <div style={{ marginBottom: 16 }}>
              <Descriptions
                bordered
                size="small"
                column={1}
                style={{ marginBottom: 12 }}
              >
                <Descriptions.Item label={t("approve_modal.delete_target_vm")}>
                  <Text strong style={{ color: "#cf1322" }}>
                    {approvals.approveModal.target_vm_name || "—"}
                  </Text>
                </Descriptions.Item>
                <Descriptions.Item label={t("requester")}>
                  {approvals.approveModal.requester}
                </Descriptions.Item>
                {approvals.approveModal.reason && (
                  <Descriptions.Item label={t("reason")}>
                    {approvals.approveModal.reason}
                  </Descriptions.Item>
                )}
              </Descriptions>
              <div
                style={{
                  padding: "12px 16px",
                  background: "#fff2e8",
                  border: "1px solid #ffbb96",
                  borderRadius: 8,
                  display: "flex",
                  alignItems: "flex-start",
                  gap: 8,
                }}
              >
                <ExclamationCircleOutlined
                  style={{ color: "#d4380d", marginTop: 2 }}
                />
                <Text type="warning">{t("approve_modal.delete_warning")}</Text>
              </div>
            </div>
          )}
          <Form.Item name="comment" label={t("approve_modal.comment")}>
            <Input.TextArea rows={3} />
          </Form.Item>
        </Form>
      </Modal>

      <Modal
        title={t("reject_modal.title")}
        open={Boolean(approvals.rejectModal)}
        onOk={() => {
          void approvals.submitReject();
        }}
        onCancel={approvals.closeRejectModal}
        confirmLoading={approvals.rejectPending}
        forceRender
        data-testid="reject-modal"
      >
        <Form form={approvals.rejectForm} layout="vertical" name="reject-form">
          <Form.Item
            name="reason"
            label={t("reject_modal.reason")}
            rules={[
              { required: true, message: "Rejection reason is required" },
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
