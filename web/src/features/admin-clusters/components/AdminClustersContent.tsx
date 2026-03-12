"use client";

import {
  Alert,
  Badge,
  Button,
  Card,
  Divider,
  Form,
  Input,
  Modal,
  Select,
  Space,
  Switch,
  Table,
  Tag,
  Typography,
} from "antd";
import type { ColumnsType } from "antd/es/table";
import {
  ClusterOutlined,
  PlusOutlined,
  ReloadOutlined,
} from "@ant-design/icons";
import dayjs from "dayjs";
import type { TFunction } from "i18next";
import { useTranslation } from "react-i18next";

import {
  HUGEPAGES_PRESET_OPTIONS,
  isValidHugepagesPageSizeValue,
  normalizeHugepagesPageSizeList,
} from "@/lib/hugepages";
import { useAdminClustersController } from "../hooks/useAdminClustersController";
import { CLUSTER_STATUS_MAP, type Cluster } from "../types";

const { Title, Text } = Typography;

export function AdminClustersContent() {
  const { t } = useTranslation(["admin", "common"]);
  const clusters = useAdminClustersController({ t });

  const columns: ColumnsType<Cluster> = [
    {
      title: t("common:table.name"),
      dataIndex: "display_name",
      key: "name",
      render: (displayName: string, record: Cluster) => (
        <Space>
          <ClusterOutlined style={{ color: "#1677ff" }} />
          <div>
            <Text strong>{displayName ?? record.name}</Text>
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
            text={<Tag color={config.color}>{status}</Tag>}
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
          {enabled ? "Yes" : "No"}
        </Tag>
      ),
    },
    {
      title: t("clusters.api_server"),
      dataIndex: "api_server_url",
      key: "api_server_url",
      ellipsis: true,
      render: (url: string) => (
        <Text type="secondary" copyable>
          {url}
        </Text>
      ),
    },
    {
      title: t("clusters.enabled_features"),
      dataIndex: "enabled_features",
      key: "enabled_features",
      render: (features: Cluster["enabled_features"]) => {
        if (!features || features.length === 0) {
          return <Text type="secondary">—</Text>;
        }
        const MAX_VISIBLE = 3;
        const visible = features.slice(0, MAX_VISIBLE);
        const overflow = features.length - MAX_VISIBLE;
        return (
          <Space size={[0, 4]} wrap>
            {visible.map((f) => (
              <Tag key={f} color="geekblue" style={{ marginBottom: 2 }}>
                {f}
              </Tag>
            ))}
            {overflow > 0 && <Tag color="default">+{overflow}</Tag>}
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
      render: (date: string) => (
        <Text type="secondary">
          {date ? dayjs(date).format("YYYY-MM-DD HH:mm") : "—"}
        </Text>
      ),
    },
    {
      title: t("common:table.actions", "Actions"),
      key: "actions",
      width: 160,
      render: (_, record: Cluster) => (
        <Button
          size="small"
          data-testid={`cluster-action-edit-policy-${record.id}`}
          onClick={() => {
            void clusters.openPolicyModal(record);
          }}
        >
          {t("clusters.edit_policy", "Edit Policy")}
        </Button>
      ),
    },
  ];

  return (
    <div data-testid="admin-clusters-page">
      {clusters.messageContextHolder}
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
            {t("clusters.title")}
          </Title>
          <Text type="secondary">{t("clusters.subtitle")}</Text>
        </div>
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
      </div>

      <Card style={{ borderRadius: 12 }} styles={{ body: { padding: 0 } }}>
        <Table<Cluster>
          columns={columns}
          dataSource={clusters.data?.items ?? []}
          rowKey="id"
          loading={clusters.isLoading}
          pagination={{
            total: clusters.data?.pagination?.total ?? 0,
            pageSize: 20,
            showTotal: (total) => t("common:table.total", { total }),
          }}
          scroll={{ x: 1560 }}
          size="middle"
        />
      </Card>

      <Modal
        title={t("clusters.add")}
        open={clusters.createOpen}
        onOk={() => {
          void clusters.submitCreate();
        }}
        onCancel={clusters.closeCreateModal}
        confirmLoading={clusters.createPending}
        forceRender
        data-testid="cluster-create-modal"
      >
        <Form form={clusters.form} layout="vertical" name="create-cluster">
          <Form.Item
            name="name"
            label={t("common:table.name")}
            rules={[{ required: true, message: "Cluster name is required" }]}
          >
            <Input placeholder="e.g. cluster-prod-01" />
          </Form.Item>
          <Form.Item name="display_name" label="Display Name">
            <Input placeholder="e.g. Production Cluster" />
          </Form.Item>
          <Form.Item
            name="environment"
            label={t("clusters.environment")}
            initialValue="test"
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
            name="kubeconfig"
            label="Kubeconfig (Base64)"
            rules={[{ required: true, message: "Kubeconfig is required" }]}
            extra="Base64-encoded kubeconfig (stored encrypted, ADR-0012)"
          >
            <Input.TextArea
              rows={6}
              placeholder="Paste base64-encoded kubeconfig content..."
            />
          </Form.Item>
        </Form>
      </Modal>
      <Modal
        title={t("clusters.set_environment")}
        open={clusters.envModalOpen}
        onOk={() => {
          void clusters.submitEnvUpdate();
        }}
        onCancel={clusters.closeEnvModal}
        confirmLoading={clusters.updateEnvironmentPending}
        destroyOnHidden={true}
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
        destroyOnHidden={true}
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
