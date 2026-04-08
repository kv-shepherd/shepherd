"use client";

import {
  Alert,
  Badge,
  Button,
  Divider,
  Form,
  Input,
  Modal,
  Popconfirm,
  Select,
  Space,
  Switch,
  Table,
  Tag,
  Tooltip,
  Typography,
} from "antd";
import type { ColumnsType } from "antd/es/table";
import {
  ClusterOutlined,
  PlusOutlined,
  ReloadOutlined,
} from "@ant-design/icons";
import type { TFunction } from "i18next";
import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";

import { ActionEmptyState } from "@/components/feedback/ActionEmptyState";
import { SummaryMetricCard } from "@/components/feedback/SummaryMetricCard";
import {
  HealthOverviewGlyph,
  NotificationInboxGlyph,
  QueueReviewGlyph,
  SystemsOverviewGlyph,
} from "@/components/illustrations/DashboardIllustrations";
import { PageHeader, PageSurface } from "@/components/layouts/PageSection";
import { LocalDateTimeText } from "@/components/ui/LocalDateTimeText";
import { PageSearchToolbar, filterOptionByLabel } from "@/components/ui/PageSearchToolbar";
import {
  HUGEPAGES_PRESET_OPTIONS,
  isValidHugepagesPageSizeValue,
  normalizeHugepagesPageSizeList,
} from "@/lib/hugepages";
import { extractKubeconfigServer } from "../kubeconfig";
import { useAdminClustersController } from "../hooks/useAdminClustersController";
import { CLUSTER_STATUS_MAP, type Cluster } from "../types";

const { Text } = Typography;

function clusterMatchesSearch(record: Cluster, query: string) {
  const normalized = query.trim().toLowerCase();
  if (!normalized) {
    return true;
  }
  return [
    record.display_name,
    record.name,
    record.api_server_url,
    record.kubevirt_version,
    record.environment,
    record.status,
    ...(record.enabled_features ?? []),
    ...(record.storage_classes ?? []),
  ]
    .filter(Boolean)
    .some((value) => String(value).toLowerCase().includes(normalized));
}

export function AdminClustersContent() {
  const { t } = useTranslation(["admin", "common"]);
  const clusters = useAdminClustersController({ t });
  const [quickSearch, setQuickSearch] = useState("");
  const [quickSearchDraft, setQuickSearchDraft] = useState("");
  const [filtersOpen, setFiltersOpen] = useState(false);
  const [environmentFilter, setEnvironmentFilter] = useState<"" | "test" | "prod">("");
  const [statusFilter, setStatusFilter] = useState<"" | Cluster["status"]>("");
  const [enabledFilter, setEnabledFilter] = useState<"" | "enabled" | "disabled">("");
  const [environmentFilterDraft, setEnvironmentFilterDraft] = useState<"" | "test" | "prod">("");
  const [statusFilterDraft, setStatusFilterDraft] = useState<"" | Cluster["status"]>("");
  const [enabledFilterDraft, setEnabledFilterDraft] = useState<"" | "enabled" | "disabled">("");
  const clusterItems = useMemo(
    () =>
      (clusters.data?.items ?? []).filter((cluster) => {
        if (!clusterMatchesSearch(cluster, quickSearch)) {
          return false;
        }
        if (environmentFilter && cluster.environment !== environmentFilter) {
          return false;
        }
        if (statusFilter && cluster.status !== statusFilter) {
          return false;
        }
        if (enabledFilter) {
          const expected = enabledFilter === "enabled";
          if (cluster.enabled !== expected) {
            return false;
          }
        }
        return true;
      }),
    [clusters.data?.items, enabledFilter, environmentFilter, quickSearch, statusFilter],
  );
  const clusterSummary = {
    total: clusterItems.length,
    healthy: clusterItems.filter((cluster) => cluster.status === "HEALTHY").length,
    prod: clusterItems.filter((cluster) => cluster.environment === "prod").length,
    disabled: clusterItems.filter((cluster) => !cluster.enabled).length,
  };

  const columns: ColumnsType<Cluster> = [
    {
      title: t("clusters.table.cluster", "Cluster"),
      dataIndex: "display_name",
      key: "name",
      width: 260,
      minWidth: 240,
      render: (displayName: string, record: Cluster) => (
        <Space size={8} align="start">
          <ClusterOutlined style={{ color: "#1677ff" }} />
          <div style={{ minWidth: 0 }}>
            <Text strong ellipsis={{ tooltip: displayName ?? record.name }}>
              {displayName ?? record.name}
            </Text>
            <br />
            <Text type="secondary" style={{ fontSize: 12 }}>
              {record.name}
            </Text>
          </div>
        </Space>
      ),
    },
    {
      title: t("common:table.status"),
      dataIndex: "status",
      key: "status",
      width: 140,
      render: (status: Cluster["status"]) => {
        const config = CLUSTER_STATUS_MAP[status] ?? CLUSTER_STATUS_MAP.UNKNOWN;
        return (
          <Badge
            status={config.badge}
            text={(
              <Tag color={config.color}>
                {t(`clusters.status.${status.toLowerCase()}`, status)}
              </Tag>
            )}
          />
        );
      },
    },
    {
      title: t("clusters.kubevirt_version"),
      dataIndex: "kubevirt_version",
      key: "kubevirt_version",
      width: 130,
      render: (version: string | undefined) =>
        version ? <Tag color="blue">KV {version}</Tag> : "—",
    },
    {
      title: t("clusters.environment"),
      dataIndex: "environment",
      key: "environment",
      width: 160,
      render: (env: "test" | "prod" | undefined, record: Cluster) => (
        <Button
          size="small"
          data-testid={`cluster-action-set-environment-${record.id}`}
          onClick={() => clusters.openEnvModal(record.id, env ?? "test")}
        >
          {env === "prod" ? t("clusters.env_prod") : t("clusters.env_test")}
        </Button>
      ),
    },
    {
      title: t("clusters.enabled"),
      dataIndex: "enabled",
      key: "enabled",
      width: 90,
      render: (enabled: boolean) => (
        <Tag color={enabled ? "green" : "default"}>
          {enabled
            ? t("clusters.enabled_yes", "Enabled")
            : t("clusters.enabled_no", "Disabled")}
        </Tag>
      ),
    },
    {
      title: t("clusters.table.endpoint", "API endpoint"),
      dataIndex: "api_server_url",
      key: "api_server_url",
      width: 320,
      minWidth: 300,
      ellipsis: { showTitle: false },
      render: (url: string) => (
        <Text
          type="secondary"
          copyable={{ text: url }}
          ellipsis={{ tooltip: url }}
          style={{ maxWidth: 300 }}
        >
          {url}
        </Text>
      ),
    },
    {
      title: t("clusters.enabled_features"),
      dataIndex: "enabled_features",
      key: "enabled_features",
      width: 240,
      minWidth: 220,
      render: (features: Cluster["enabled_features"]) => {
        if (!features || features.length === 0) {
          return <Text type="secondary">—</Text>;
        }
        const MAX_VISIBLE = 4;
        const visible = features.slice(0, MAX_VISIBLE);
        const overflow = features.length - MAX_VISIBLE;
        return (
          <Space size={[0, 4]} wrap>
            {visible.map((f) => (
              <Tag key={f} color="geekblue" style={{ marginBottom: 2 }}>
                {f}
              </Tag>
            ))}
            {overflow > 0 && (
              <Tooltip title={features.join(", ")}>
                <Tag color="default">+{overflow}</Tag>
              </Tooltip>
            )}
          </Space>
        );
      },
    },
    {
      title: t("clusters.storage_classes", "Storage Classes"),
      dataIndex: "storage_classes",
      key: "storage_classes",
      width: 220,
      render: (storageClasses: Cluster["storage_classes"], record: Cluster) => {
        const items = storageClasses ?? [];
        if (items.length === 0) {
          if (record.status === "UNKNOWN") {
            return (
              <Text type="secondary">
                {t(
                  "clusters.storage_classes_pending",
                  "Waiting for health check",
                )}
              </Text>
            );
          }
          return (
            <Text type="secondary">
              {t(
                "clusters.storage_classes_empty",
                "No storage classes detected",
              )}
            </Text>
          );
        }
        const maxVisible = 2;
        const visible = items.slice(0, maxVisible);
        const overflow = items.length - maxVisible;
        return (
          <Space size={[0, 4]} wrap>
            {visible.map((item) => (
              <Tag
                key={item}
                color={
                  item === record.default_storage_class ? "green" : "default"
                }
              >
                {item}
                {item === record.default_storage_class
                  ? ` (${t("clusters.default_storage_class_short", "default")})`
                  : ""}
              </Tag>
            ))}
            {overflow > 0 ? <Tag color="default">+{overflow}</Tag> : null}
          </Space>
        );
      },
    },
    {
      title: t("clusters.policy_summary", "Policy Summary"),
      key: "policy_summary",
      width: 220,
      render: (_, record: Cluster) => {
        const summary = record.policy_summary;
        if (!summary) {
          return <Text type="secondary">—</Text>;
        }
        const mode = summary.mode ?? "MISSING";
        const detailTags = summarizePolicyDetails(summary, t);
        return (
          <Space direction="vertical" size={4}>
            <Tag color={policyModeColor(mode)}>
              {t(`clusters.policy_mode.${mode.toLowerCase()}`, mode)}
            </Tag>
            {detailTags.length > 0 ? (
              <Space size={[0, 4]} wrap>
                {detailTags.map((tag) => (
                  <Tag
                    key={tag.key}
                    color={tag.color}
                    style={{ marginBottom: 2 }}
                  >
                    {tag.label}
                  </Tag>
                ))}
              </Space>
            ) : (
              <Text type="secondary" style={{ fontSize: 12 }}>
                {mode === "OPEN"
                  ? t(
                      "clusters.policy_mode.open_hint",
                      "No explicit deny or allowlist guardrails",
                    )
                  : t(
                      "clusters.policy_mode.missing_hint",
                      "No ClusterPolicy configured",
                    )}
              </Text>
            )}
          </Space>
        );
      },
    },
    {
      title: t("common:table.created_at"),
      dataIndex: "created_at",
      key: "created_at",
      width: 160,
      render: (date: string) => <LocalDateTimeText value={date} />,
    },
    {
      title: t("common:table.actions", "Actions"),
      key: "actions",
      width: 240,
      align: "right",
      render: (_, record: Cluster) => (
        <Space size={0} split={<Divider type="vertical" />}>
          <Button
            type="link"
            size="small"
            data-testid={`cluster-action-edit-${record.id}`}
            onClick={() => {
              clusters.openEditModal(record);
            }}
          >
            {t("common:button.edit")}
          </Button>
          <Button
            type="link"
            size="small"
            data-testid={`cluster-action-edit-policy-${record.id}`}
            onClick={() => {
              void clusters.openPolicyModal(record);
            }}
          >
            {t("clusters.edit_policy", "Edit Policy")}
          </Button>
          <Popconfirm
            title={t("clusters.delete_confirm_title", "Delete cluster?")}
            description={t(
              "clusters.delete_confirm_description",
              "Only unused clusters can be deleted. Existing virtual machines must be removed first.",
            )}
            okText={t("common:button.delete")}
            cancelText={t("common:button.cancel")}
            onConfirm={() => {
              void clusters.deleteCluster(record.id);
            }}
          >
            <Button
              type="link"
              size="small"
              danger
              loading={
                clusters.deletePending &&
                clusters.deletingClusterId === record.id
              }
              data-testid={`cluster-action-delete-${record.id}`}
            >
              {t("common:button.delete")}
            </Button>
          </Popconfirm>
        </Space>
      ),
    },
  ];

  return (
    <div data-testid="admin-clusters-page" className="admin-clusters-page">
      {clusters.messageContextHolder}
      <PageHeader
        title={t("clusters.title")}
        subtitle={t("clusters.subtitle")}
        actions={(
          <Space>
          <Button
            icon={<ReloadOutlined />}
            data-testid="clusters-refresh-btn"
            onClick={() => clusters.refetch()}
          >
            {t("common:button.refresh")}
          </Button>
          <Button
            type="primary"
            icon={<PlusOutlined />}
            data-testid="cluster-create-button"
            onClick={clusters.openCreateModal}
          >
            {t("clusters.add")}
          </Button>
          </Space>
        )}
      />

      <div className="summary-card-grid">
        <SummaryMetricCard
          title={t("clusters.summary.total_title", "Registered clusters")}
          value={clusterSummary.total}
          description={t("clusters.summary.total_description", "All cluster connections currently registered with the platform.")}
          visual={<SystemsOverviewGlyph className="summary-metric-card__art" />}
          accentColor="#1D5BFF"
          surfaceColor="#E6F4FF"
        />
        <SummaryMetricCard
          title={t("clusters.summary.healthy_title", "Healthy")}
          value={clusterSummary.healthy}
          description={t("clusters.summary.healthy_description", "Clusters currently passing connectivity and KubeVirt health checks.")}
          visual={<HealthOverviewGlyph className="summary-metric-card__art" />}
          accentColor="#0F8F57"
          surfaceColor="#E8FFF2"
        />
        <SummaryMetricCard
          title={t("clusters.summary.prod_title", "Production")}
          value={clusterSummary.prod}
          description={t("clusters.summary.prod_description", "Clusters tagged for production placement and governance rules.")}
          visual={<QueueReviewGlyph className="summary-metric-card__art" />}
          accentColor="#D66A1F"
          surfaceColor="#FFF4E5"
        />
        <SummaryMetricCard
          title={t("clusters.summary.disabled_title", "Disabled")}
          value={clusterSummary.disabled}
          description={t("clusters.summary.disabled_description", "Registered clusters currently held out of placement decisions.")}
          visual={<NotificationInboxGlyph className="summary-metric-card__art" />}
          accentColor="#6D4DE3"
          surfaceColor="#F5EDFF"
        />
      </div>

      <PageSurface className="admin-clusters-page__workspace-surface" flush={true}>
        <PageSearchToolbar
          searchValue={quickSearch}
          searchDraftValue={quickSearchDraft}
          onSearchDraftChange={setQuickSearchDraft}
          onSearchChange={(value) => {
            setQuickSearchDraft(value);
            setQuickSearch(value);
          }}
          searchPlaceholder={t("clusters.search_placeholder", "Search clusters by name, endpoint, version, or feature")}
          searchHelp={t("clusters.search_help", "Press Enter or click Search. Quick search matches cluster names, display names, API endpoints, versions, and enabled features.")}
          advancedSearch={{
            open: filtersOpen,
            onToggle: () => setFiltersOpen((open) => !open),
            openLabel: t("common:search.advanced", { defaultValue: "Advanced search" }),
            closeLabel: t("common:search.hide_advanced", { defaultValue: "Hide advanced search" }),
            title: t("common:search.advanced", { defaultValue: "Advanced search" }),
            content: (
              <Space direction="vertical" size={12} style={{ width: "100%" }}>
                <Text type="secondary">
                  {t("clusters.advanced_search_help", {
                    defaultValue:
                      "Select exact cluster filters here. Options support keyword matching, but the applied filter remains an exact value.",
                  })}
                </Text>
                <Space wrap align="end">
                <Select
                  allowClear
                  showSearch
                  filterOption={filterOptionByLabel}
                  optionFilterProp="label"
                  style={{ width: 180 }}
                  value={environmentFilterDraft || undefined}
                  placeholder={t("clusters.environment")}
                  onChange={(value) => setEnvironmentFilterDraft((value as "test" | "prod" | undefined) ?? "")}
                  options={[
                    { value: "test", label: t("clusters.env_test") },
                    { value: "prod", label: t("clusters.env_prod") },
                  ]}
                />
                <Select
                  allowClear
                  showSearch
                  filterOption={filterOptionByLabel}
                  optionFilterProp="label"
                  style={{ width: 180 }}
                  value={statusFilterDraft || undefined}
                  placeholder={t("common:table.status")}
                  onChange={(value) => setStatusFilterDraft((value as Cluster["status"] | undefined) ?? "")}
                  options={Object.entries(CLUSTER_STATUS_MAP).map(([key]) => ({
                    value: key,
                    label: t(`clusters.status.${key.toLowerCase()}`, key),
                  }))}
                />
                <Select
                  allowClear
                  showSearch
                  filterOption={filterOptionByLabel}
                  optionFilterProp="label"
                  style={{ width: 180 }}
                  value={enabledFilterDraft || undefined}
                  placeholder={t("clusters.enabled")}
                  onChange={(value) =>
                    setEnabledFilterDraft((value as "enabled" | "disabled" | undefined) ?? "")
                  }
                  options={[
                    {
                      value: "enabled",
                      label: t("clusters.enabled_yes", "Enabled"),
                    },
                    {
                      value: "disabled",
                      label: t("clusters.enabled_no", "Disabled"),
                    },
                  ]}
                />
                <Button
                  type="primary"
                  data-testid="clusters-advanced-search-submit"
                  onClick={() => {
                    setQuickSearch(quickSearchDraft);
                    setEnvironmentFilter(environmentFilterDraft);
                    setStatusFilter(statusFilterDraft);
                    setEnabledFilter(enabledFilterDraft);
                  }}
                >
                  {t("common:button.search")}
                </Button>
                </Space>
              </Space>
            ),
          }}
          hasActiveFilters={Boolean(
            quickSearch.trim() || environmentFilter || statusFilter || enabledFilter,
          )}
          onClear={() => {
            setQuickSearch("");
            setQuickSearchDraft("");
            setEnvironmentFilter("");
            setEnvironmentFilterDraft("");
            setStatusFilter("");
            setStatusFilterDraft("");
            setEnabledFilter("");
            setEnabledFilterDraft("");
          }}
          clearLabel={t("common:button.clear_filters", { defaultValue: "Clear filters" })}
        />
        <Table<Cluster>
          style={{ marginTop: 16 }}
          columns={columns}
          dataSource={clusterItems}
          rowKey="id"
          loading={clusters.isLoading}
          tableLayout="auto"
          pagination={{
            total: clusterItems.length,
            pageSize: 20,
            showTotal: (total) => t("common:table.total", { total }),
          }}
          scroll={{ x: 1840 }}
          size="middle"
          locale={{
            emptyText: (
              <ActionEmptyState
                compact={true}
                title={t("clusters.empty", "No clusters registered")}
                description={t("clusters.empty_description", "Add the first cluster before routing approvals, placement decisions, or live VM operations to a KubeVirt target.")}
                visual={<HealthOverviewGlyph className="action-empty-state__art action-empty-state__art--compact" />}
              />
            ),
          }}
        />
      </PageSurface>

      {clusters.createOpen ? (
      <Modal
        title={t("clusters.add")}
        open={clusters.createOpen}
        onOk={() => {
          void clusters.submitCreate();
        }}
        onCancel={clusters.closeCreateModal}
        confirmLoading={clusters.createPending}
        data-testid="cluster-create-modal"
      >
        <Form
          form={clusters.form}
          layout="vertical"
          name="create-cluster"
          preserve={false}
          initialValues={{ environment: "test", enabled: true }}
        >
          <Form.Item
            name="name"
            label={t("common:table.name")}
            rules={[{ required: true, message: "Cluster name is required" }]}
          >
            <Input placeholder="e.g. cluster-prod-01" />
          </Form.Item>
          <Form.Item
            name="display_name"
            label={t("clusters.display_name", "Display Name")}
          >
            <Input placeholder="e.g. Production Cluster" />
          </Form.Item>
          <Form.Item
            name="environment"
            label={t("clusters.environment")}
            rules={[
              { required: true, message: t("clusters.environment_required") },
            ]}
          >
            <Select
              options={[
                { value: "test", label: t("clusters.env_test") },
                { value: "prod", label: t("clusters.env_prod") },
              ]}
            />
          </Form.Item>
          <Form.Item
            name="kubeconfig_text"
            label={t("clusters.kubeconfig", "Kubeconfig")}
            rules={[
              {
                required: true,
                message: t(
                  "clusters.kubeconfig_required",
                  "Kubeconfig is required",
                ),
              },
              {
                validator: async (_, value: string | undefined) => {
                  if (!value?.trim()) {
                    return;
                  }
                  try {
                    extractKubeconfigServer(value);
                  } catch (error) {
                    throw new Error(
                      error instanceof Error
                        ? error.message
                        : t(
                            "clusters.kubeconfig_invalid",
                            "Invalid kubeconfig YAML",
                          ),
                    );
                  }
                },
              },
            ]}
            extra={t(
              "clusters.kubeconfig_create_help",
              "Paste the kubeconfig YAML. The API transports it as base64 bytes, but base64 is not encryption.",
            )}
          >
            <Input.TextArea
              rows={6}
              placeholder={t(
                "clusters.kubeconfig_placeholder",
                "Paste kubeconfig YAML content...",
              )}
            />
          </Form.Item>
        </Form>
      </Modal>
      ) : null}
      {clusters.editOpen ? (
      <Modal
        title={t("clusters.edit_title", {
          cluster: clusters.editingClusterName || clusters.editingClusterId,
          defaultValue: "Edit Cluster: {{cluster}}",
        })}
        open={clusters.editOpen}
        onOk={() => {
          void clusters.submitEdit();
        }}
        onCancel={clusters.closeEditModal}
        confirmLoading={clusters.editPending}
        data-testid="cluster-edit-modal"
      >
        <Form
          form={clusters.editForm}
          layout="vertical"
          name="edit-cluster"
          preserve={false}
          initialValues={{ environment: "test", enabled: true }}
        >
          <Alert
            type="info"
            showIcon
            style={{ marginBottom: 16 }}
            message={t(
              "clusters.kubeconfig_update_title",
              "Replace kubeconfig only when credentials or target endpoint changed",
            )}
            description={t(
              "clusters.kubeconfig_update_help",
              "Leave kubeconfig empty to keep the existing credential. If you provide a new kubeconfig, health detection will run again.",
            )}
          />
          <Form.Item label={t("common:table.name")}>
            <Input value={clusters.editingCluster?.name ?? ""} disabled />
          </Form.Item>
          <Form.Item
            name="display_name"
            label={t("clusters.display_name", "Display Name")}
          >
            <Input placeholder="e.g. Production Cluster" />
          </Form.Item>
          <Form.Item
            name="environment"
            label={t("clusters.environment")}
            rules={[
              { required: true, message: t("clusters.environment_required") },
            ]}
          >
            <Select
              options={[
                { value: "test", label: t("clusters.env_test") },
                { value: "prod", label: t("clusters.env_prod") },
              ]}
            />
          </Form.Item>
          <Form.Item
            name="enabled"
            label={t("clusters.enabled")}
            valuePropName="checked"
          >
            <Switch />
          </Form.Item>
          <Form.Item
            name="kubeconfig_text"
            label={t("clusters.kubeconfig", "Kubeconfig")}
            rules={[
              {
                validator: async (_, value: string | undefined) => {
                  if (!value?.trim()) {
                    return;
                  }
                  try {
                    extractKubeconfigServer(value);
                  } catch (error) {
                    throw new Error(
                      error instanceof Error
                        ? error.message
                        : t(
                            "clusters.kubeconfig_invalid",
                            "Invalid kubeconfig YAML",
                          ),
                    );
                  }
                },
              },
            ]}
            extra={t(
              "clusters.kubeconfig_replace_optional",
              "Optional. Paste a new kubeconfig YAML only when you want to replace the stored credential.",
            )}
          >
            <Input.TextArea
              rows={6}
              placeholder={t(
                "clusters.kubeconfig_placeholder",
                "Paste kubeconfig YAML content...",
              )}
            />
          </Form.Item>
        </Form>
      </Modal>
      ) : null}
      {clusters.envModalOpen ? (
      <Modal
        title={t("clusters.set_environment")}
        open={clusters.envModalOpen}
        onOk={() => {
          void clusters.submitEnvUpdate();
        }}
        onCancel={clusters.closeEnvModal}
        confirmLoading={clusters.updateEnvironmentPending}
        data-testid="cluster-environment-modal"
      >
        <Form form={clusters.envForm} layout="vertical" preserve={false}>
          <Form.Item
            name="environment"
            label={t("clusters.environment")}
            rules={[{ required: true }]}
          >
            <Select
              options={[
                { value: "test", label: t("clusters.env_test") },
                { value: "prod", label: t("clusters.env_prod") },
              ]}
            />
          </Form.Item>
        </Form>
      </Modal>
      ) : null}
      {clusters.policyModalOpen ? (
      <Modal
        title={t("clusters.edit_policy_title", {
          cluster: clusters.selectedClusterName || clusters.selectedClusterId,
          defaultValue: "Edit Policy: {{cluster}}",
        })}
        open={clusters.policyModalOpen}
        onOk={() => {
          void clusters.submitPolicyUpdate();
        }}
        onCancel={clusters.closePolicyModal}
        confirmLoading={clusters.upsertPolicyPending}
        okButtonProps={{ disabled: clusters.policyLoading }}
        width={720}
        data-testid="cluster-policy-modal"
      >
        <Form form={clusters.policyForm} layout="vertical" preserve={false}>
          <Alert
            showIcon
            type={clusters.selectedClusterPolicyExists ? "info" : "warning"}
            style={{ marginBottom: 16 }}
            message={t(
              clusters.selectedClusterPolicyExists
                ? "clusters.policy.intent_title"
                : "clusters.policy.missing_title",
              clusters.selectedClusterPolicyExists
                ? "Cluster Policy adds cluster-side guardrails"
                : "This cluster does not have a saved ClusterPolicy yet",
            )}
            description={t(
              clusters.selectedClusterPolicyExists
                ? "clusters.policy.intent_description"
                : "clusters.policy.missing_description",
              clusters.selectedClusterPolicyExists
                ? "Leave switches enabled and allowlists empty to keep this cluster open. Templates describe workload needs; Cluster Policy narrows what this cluster is allowed to accept."
                : "Saving this form will create the first ClusterPolicy for this cluster. Enabled switches plus empty allowlists keep the cluster open; use allowlists only when you want stricter guardrails.",
            )}
          />
          <Divider orientation="left" plain>
            {t(
              "clusters.policy.capabilities_section",
              "Allowed workload capabilities",
            )}
          </Divider>
          <Form.Item
            name="allow_cpu_overcommit"
            label={t(
              "clusters.policy.allow_cpu_overcommit",
              "Allow CPU overcommit",
            )}
            valuePropName="checked"
          >
            <Switch disabled={clusters.policyLoading} />
          </Form.Item>
          <Form.Item
            name="allow_memory_overcommit"
            label={t(
              "clusters.policy.allow_memory_overcommit",
              "Allow memory overcommit",
            )}
            valuePropName="checked"
          >
            <Switch disabled={clusters.policyLoading} />
          </Form.Item>
          <Form.Item
            name="allow_dedicated_cpu"
            label={t(
              "clusters.policy.allow_dedicated_cpu",
              "Allow dedicated CPU",
            )}
            valuePropName="checked"
          >
            <Switch disabled={clusters.policyLoading} />
          </Form.Item>
          <Form.Item
            name="allow_gpu"
            label={t("clusters.policy.allow_gpu", "Allow GPU")}
            valuePropName="checked"
          >
            <Switch disabled={clusters.policyLoading} />
          </Form.Item>
          <Form.Item
            name="allow_sriov"
            label={t("clusters.policy.allow_sriov", "Allow SR-IOV")}
            valuePropName="checked"
          >
            <Switch disabled={clusters.policyLoading} />
          </Form.Item>
          <Form.Item
            name="allow_hugepages"
            label={t("clusters.policy.allow_hugepages", "Allow hugepages")}
            valuePropName="checked"
            extra={t(
              "clusters.policy.allow_hugepages_help",
              "Turn this off only when the cluster should reject every hugepages workload, even if the cluster technically supports it.",
            )}
          >
            <Switch disabled={clusters.policyLoading} />
          </Form.Item>
          <Form.Item
            name="allow_cdi_clone"
            label={t("clusters.policy.allow_cdi_clone", "Allow CDI clone")}
            valuePropName="checked"
            extra={t(
              "clusters.policy.allow_cdi_clone_help",
              "Templates declare the source PVC namespace. This policy optionally restricts which clone source namespaces this cluster may accept.",
            )}
          >
            <Switch disabled={clusters.policyLoading} />
          </Form.Item>
          <Divider orientation="left" plain>
            {t("clusters.policy.guardrails_section", "Optional allowlists")}
          </Divider>
          <Text type="secondary" style={{ display: "block", marginBottom: 12 }}>
            {t(
              "clusters.policy.guardrails_hint",
              "Use allowlists only when this cluster should be stricter than its detected capabilities.",
            )}
          </Text>
          <Form.Item
            noStyle
            shouldUpdate={(prev, cur) =>
              prev.allow_hugepages !== cur.allow_hugepages
            }
          >
            {({ getFieldValue }) =>
              getFieldValue("allow_hugepages") ? (
                <Form.Item
                  name="allowed_hugepages_sizes"
                  label={t(
                    "clusters.policy.allowed_hugepages_sizes",
                    "Allowed hugepages sizes",
                  )}
                  extra={t(
                    "clusters.policy.allowed_hugepages_sizes_help",
                    "Leave empty for no size restriction. Select 2Mi/1Gi, or input custom MB like 512.",
                  )}
                  getValueFromEvent={normalizeHugepagesPageSizeList}
                  rules={[
                    {
                      validator: (_, value: unknown) => {
                        if (
                          !Array.isArray(value) ||
                          value.every((item) =>
                            isValidHugepagesPageSizeValue(item),
                          )
                        ) {
                          return Promise.resolve();
                        }
                        return Promise.reject(
                          new Error(
                            t(
                              "clusters.policy.allowed_hugepages_sizes_invalid",
                              "Hugepages must be 2Mi/1Gi, or a custom MB value (for example 512).",
                            ),
                          ),
                        );
                      },
                    },
                  ]}
                >
                  <Select
                    mode="tags"
                    allowClear
                    maxTagCount="responsive"
                    tokenSeparators={[","]}
                    disabled={clusters.policyLoading}
                    options={HUGEPAGES_PRESET_OPTIONS.map((value) => ({
                      label: value,
                      value,
                    }))}
                    placeholder={t(
                      "clusters.policy.allowed_hugepages_sizes_placeholder",
                      "Select 2Mi/1Gi or input MB",
                    )}
                  />
                </Form.Item>
              ) : null
            }
          </Form.Item>
          <Form.Item
            noStyle
            shouldUpdate={(prev, cur) =>
              prev.allow_cdi_clone !== cur.allow_cdi_clone
            }
          >
            {({ getFieldValue }) =>
              getFieldValue("allow_cdi_clone") ? (
                <Form.Item
                  name="allowed_clone_source_namespaces"
                  label={t(
                    "clusters.policy.allowed_clone_source_namespaces",
                    "Allowed clone source namespaces",
                  )}
                  extra={
                    clusters.selectedClusterNamespaceOptions.length > 0
                      ? t(
                          "clusters.policy.allowed_clone_source_namespaces_detected_help",
                          "Suggested from registered namespaces in the same environment. Leave empty to allow all clone source namespaces.",
                        )
                      : t(
                          "clusters.policy.allowed_clone_source_namespaces_help",
                          "Leave empty to allow all clone source namespaces. You can still type a namespace manually.",
                        )
                  }
                >
                  <Select
                    mode="tags"
                    allowClear
                    maxTagCount="responsive"
                    tokenSeparators={[","]}
                    disabled={clusters.policyLoading}
                    loading={clusters.namespaceSuggestionsLoading}
                    options={clusters.selectedClusterNamespaceOptions.map(
                      (value) => ({
                        label: value,
                        value,
                      }),
                    )}
                    placeholder={t(
                      "clusters.policy.allowed_clone_source_namespaces_placeholder",
                      "e.g. golden-images",
                    )}
                  />
                </Form.Item>
              ) : null
            }
          </Form.Item>
          <Form.Item
            name="allowed_storage_classes"
            label={t(
              "clusters.policy.allowed_storage_classes",
              "Allowed storage classes",
            )}
            extra={
              clusters.selectedClusterStorageClasses.length > 0
                ? t(
                    "clusters.policy.allowed_storage_classes_detected_help",
                    "Suggested from auto-discovered cluster storage classes. Leave empty to allow all CDI-capable storage classes.",
                  )
                : t(
                    "clusters.policy.allowed_storage_classes_help",
                    "Leave empty to allow all CDI-capable storage classes. You can still type a storage class manually.",
                  )
            }
          >
            <Select
              mode="tags"
              allowClear
              maxTagCount="responsive"
              tokenSeparators={[","]}
              disabled={clusters.policyLoading}
              options={clusters.selectedClusterStorageClasses.map((value) => ({
                label: value,
                value,
              }))}
              placeholder={t(
                "clusters.policy.allowed_storage_classes_placeholder",
                "e.g. rook-ceph-block",
              )}
            />
          </Form.Item>
        </Form>
      </Modal>
      ) : null}
    </div>
  );
}

function policyModeColor(mode: string): string {
  switch (mode) {
    case "OPEN":
      return "green";
    case "GUARDED":
      return "orange";
    default:
      return "red";
  }
}

function summarizePolicyDetails(
  summary: Cluster["policy_summary"],
  t: TFunction,
): Array<{ key: string; label: string; color: string }> {
  if (!summary) {
    return [];
  }
  const items: Array<{ key: string; label: string; color: string }> = [];
  const deniedControls = summary.denied_controls ?? [];
  const scopedControls = summary.scoped_controls ?? [];

  for (const control of deniedControls.slice(0, 2)) {
    items.push({
      key: `deny-${control}`,
      label: t(`clusters.policy_control.${control}`, control),
      color: "red",
    });
  }
  for (const control of scopedControls.slice(0, 2)) {
    items.push({
      key: `scope-${control}`,
      label: scopedControlLabel(summary, control, t),
      color: "gold",
    });
  }
  return items;
}

function scopedControlLabel(
  summary: Cluster["policy_summary"],
  control: string,
  t: TFunction,
): string {
  switch (control) {
    case "storage_classes":
      return t("clusters.policy_scope.storage_classes", {
        count: summary?.allowed_storage_class_count ?? 0,
        defaultValue: "Storage classes ({{count}})",
      });
    case "clone_source_namespaces":
      return t("clusters.policy_scope.clone_source_namespaces", {
        count: summary?.allowed_clone_source_namespace_count ?? 0,
        defaultValue: "Clone namespaces ({{count}})",
      });
    case "hugepages_sizes":
      return t("clusters.policy_scope.hugepages_sizes", {
        count: summary?.allowed_hugepages_size_count ?? 0,
        defaultValue: "Hugepages sizes ({{count}})",
      });
    default:
      return t(`clusters.policy_control.${control}`, control);
  }
}
