"use client";

/**
 * /vms/[id] — VM detail page.
 * master-flow.md §5: VM Detail — single VM status and console access.
 *
 * API contracts:
 *   GET  /vms/{vm_id}                → VM
 *   POST /vms/{vm_id}/power          → VM (start/stop/restart)
 *   GET  /vms/{vm_id}/console/status → VMConsoleStatusResponse
 *
 * E2E data-testid requirements:
 *   vm-detail-page
 *   vm-console-status-{id}
 *   vm-action-start-{id}
 *   vm-action-stop-{id}
 *   vm-action-delete-{id}
 *   vm-action-console-{id}
 *   vm-action-request-similar-{id}
 */
import {
  Alert,
  Badge,
  Button,
  Descriptions,
  Input,
  Modal,
  Popconfirm,
  Space,
  Tag,
  Typography,
} from "antd";
import type { DescriptionsProps } from "antd";
import {
  ArrowLeftOutlined,
  FileTextOutlined,
  CopyOutlined,
  DeleteOutlined,
  DesktopOutlined,
  ExclamationCircleOutlined,
  PauseCircleOutlined,
  PlayCircleOutlined,
  RedoOutlined,
} from "@ant-design/icons";
import { useParams, useRouter, useSearchParams } from "next/navigation";
import { useTranslation } from "react-i18next";
import { useEffect, useRef, useState } from "react";
import dayjs from "dayjs";

import { ConsoleModeModal } from "@/features/vm-management/components/ConsoleModeModal";
import { VMConsoleSessionPanel } from "@/features/vm-management/components/VMConsoleSessionPanel";
import { useApiMutation } from "@/hooks/useApiQuery";
import { useApiGet } from "@/lib/api/useApiGet";
import { api } from "@/lib/api/client";
import { translateApiError } from "@/lib/api/errorMessage";
import { isCodespacesDemoHost } from "@/lib/auth/demoEnvironment";
import { hasPermission, PLATFORM_ADMIN_PERMISSION } from "@/lib/auth/permissions";
import { useMessage } from "@/lib/hooks/useMessage";
import {
  hasAnyConsoleCapability,
  readStoredPreferredConsoleType,
  resolveDefaultConsoleType,
  resolveApprovedConsoleTarget,
  saveStoredPreferredConsoleType,
  type ResolvedConsoleTarget,
  type VMConsoleType,
} from "@/features/vm-management/console";
import {
  buildVMRemoteAccessCommand,
  describeVMRemoteAccess,
  formatVMOperatingSystem,
  resolveVMRemoteAccessMode,
} from "@/features/vm-management/osInfo";
import type { components } from "@/types/api.gen";
import type {
  DeleteVMResponse,
  VM,
  VMConsoleRequestResponse,
} from "@/features/vm-management/types";
import { formatMemory, VM_STATUS_MAP } from "@/features/vm-management/types";
import { PageHeader, PageSurface } from "@/components/layouts/PageSection";
import { useAuthStore } from "@/stores/auth";

const { Text, Paragraph } = Typography;

const formatCPU = (cpuCores: number | undefined): string => {
  if (!Number.isFinite(cpuCores) || cpuCores === undefined || cpuCores <= 0) {
    return "—";
  }
  return `${Number.isInteger(cpuCores) ? cpuCores : cpuCores.toFixed(1)} vCPU`;
};

const formatDisk = (diskGb: number | undefined): string => {
  if (!Number.isFinite(diskGb) || diskGb === undefined || diskGb <= 0) {
    return "—";
  }
  return `${diskGb} Gi`;
};

export default function VMDetailPage() {
  const { t } = useTranslation(["vm", "common"]);
  const params = useParams<{ id: string }>();
  const router = useRouter();
  const searchParams = useSearchParams();
  const vmId = params.id;
  const user = useAuthStore((state) => state.user);
  const { messageApi, messageContextHolder } = useMessage();
  const isCodespacesDemo =
    typeof window !== "undefined" && isCodespacesDemoHost(window.location.hostname);
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [deleteConfirmName, setDeleteConfirmName] = useState("");
  const [consolePending, setConsolePending] = useState(false);
  const [consoleChooserOpen, setConsoleChooserOpen] = useState(false);
  const [selectedConsoleType, setSelectedConsoleType] =
    useState<VMConsoleType>("SERIAL");
  const [activeConsoleTarget, setActiveConsoleTarget] =
    useState<ResolvedConsoleTarget | null>(null);
  const [manifestViewerOpen, setManifestViewerOpen] = useState(false);
  const consoleSectionRef = useRef<HTMLDivElement | null>(null);

  const {
    data: vm,
    isLoading,
    refetch,
  } = useApiGet<VM>(["vm-detail", vmId], () =>
    api.GET("/vms/{vm_id}", { params: { path: { vm_id: vmId } } }),
  );

  const powerMutation = useApiMutation(
    ({ action }: { action: "start" | "stop" | "restart" }) =>
      api.POST("/vms/{vm_id}/power", {
        params: { path: { vm_id: vmId } },
        body: { action },
      }),
    {
      onSuccess: () => {
        void messageApi.success(t("message.action_submitted"));
        void refetch();
      },
    },
  );

  const deleteMutation = useApiMutation<DeleteVMResponse>(
    () =>
      api.DELETE("/vms/{vm_id}", {
        params: {
          path: { vm_id: vmId },
          query: { confirm: true, confirm_name: vm?.name ?? "" },
        },
      }),
    {
      onSuccess: () => {
        void messageApi.success(t("delete_request_submitted"));
        setDeleteOpen(false);
        setDeleteConfirmName("");
        router.push("/vms");
      },
      onError: (err) => {
        void messageApi.error(translateApiError(t, err));
      },
    },
  );

  const requestConsole = async (preferredConsoleType?: VMConsoleType) => {
    setConsolePending(true);
    try {
      const { data, response, error } = await api.POST(
        "/vms/{vm_id}/console/request",
        {
          params: { path: { vm_id: vmId } },
          body: preferredConsoleType
            ? { preferred_console_type: preferredConsoleType }
            : undefined,
        },
      );
      const payload = data as VMConsoleRequestResponse | undefined;
      if (error) {
        void messageApi.error(translateApiError(t, error));
        return false;
      }
      if (response.status === 202 || payload?.status === "PENDING_APPROVAL") {
        void messageApi.info(t("console.pending_approval"));
        return true;
      }
      const target = resolveApprovedConsoleTarget(payload);
      if (!target) {
        void messageApi.error(t("console.unavailable"));
        return false;
      }
      setActiveConsoleTarget(target);
      return true;
    } finally {
      setConsolePending(false);
    }
  };

  // vm data is typed from the generated spec
  const vmData = vm;
  const status = vmData?.status as string | undefined;
  const mapped = VM_STATUS_MAP[status ?? "UNKNOWN"] ?? VM_STATUS_MAP.UNKNOWN;
  const isRunning = status === "RUNNING";
  const isStoppable = status === "RUNNING" || status === "STARTING";
  const isStopped = status === "STOPPED";
  const canDelete = isStopped || status === "FAILED" || status === "NOT_FOUND";
  const requiresNameConfirm = vmData?.environment !== "test";
  const deleteConfirmMatched =
    !requiresNameConfirm || deleteConfirmName === (vmData?.name ?? "");
  const hostnameVisible = vmData?.hostname && vmData.hostname !== vmData.name;
  const scopeValue =
    [vmData?.system_name, vmData?.service_name].filter(Boolean).join(" / ") ||
    "—";
  const consoleCapabilities = vmData?.console_capabilities;
  const consoleAvailable = hasAnyConsoleCapability(vmData);
  const canViewManifest = hasPermission(user, PLATFORM_ADMIN_PERMISSION);
  const operatingSystemLabel = formatVMOperatingSystem(vmData);
  const remoteAccessMode = resolveVMRemoteAccessMode(vmData);
  const remoteAccessCommand = buildVMRemoteAccessCommand(vmData);
  const remoteAccessDescription = describeVMRemoteAccess(t, vmData);
  const {
    data: consoleStatus,
    refetch: refetchConsoleStatus,
  } = useApiGet<components["schemas"]["VMConsoleStatusResponse"]>(
    ["vm-console-status", vmId],
    () =>
      api.GET("/vms/{vm_id}/console/status", {
        params: { path: { vm_id: vmId } },
      }),
    {
      enabled: Boolean(vmId && isRunning && consoleAvailable),
      staleTime: 15_000,
      refetchInterval: activeConsoleTarget ? undefined : 15_000,
    },
  );
  const manifestQuery = useApiGet<components["schemas"]["VMManifestResponse"]>(
    ["vm-manifest", vmId],
    () =>
      api.GET("/vms/{vm_id}/manifest", {
        params: { path: { vm_id: vmId } },
      }),
    {
      enabled: canViewManifest && manifestViewerOpen && Boolean(vmId),
    },
  );
  const manifestYaml = manifestQuery.data?.yaml ?? "";
  const openConsoleChooser = () => {
    setSelectedConsoleType(
      resolveDefaultConsoleType(
        consoleCapabilities,
        readStoredPreferredConsoleType(),
      ),
    );
    setConsoleChooserOpen(true);
  };

  const closeConsoleChooser = () => {
    if (consolePending) {
      return;
    }
    setConsoleChooserOpen(false);
  };

  const confirmConsoleChooser = async () => {
    saveStoredPreferredConsoleType(selectedConsoleType);
    const success = await requestConsole(selectedConsoleType);
    if (success) {
      setConsoleChooserOpen(false);
      window.setTimeout(() => {
        if (typeof consoleSectionRef.current?.scrollIntoView === "function") {
          consoleSectionRef.current.scrollIntoView({
            behavior: "smooth",
            block: "start",
          });
        }
      }, 0);
    }
  };

  const reconnectConsoleSession = async (
    preferredConsoleType: VMConsoleType,
  ): Promise<boolean> => {
    const success = await requestConsole(preferredConsoleType);
    if (success) {
      saveStoredPreferredConsoleType(preferredConsoleType);
    }
    return success;
  };

  useEffect(() => {
    if (searchParams.get("focus") !== "console") {
      return;
    }
    const timer = window.setTimeout(() => {
      if (typeof consoleSectionRef.current?.scrollIntoView === "function") {
        consoleSectionRef.current.scrollIntoView({
          behavior: "smooth",
          block: "start",
        });
      }
    }, 0);
    return () => window.clearTimeout(timer);
  }, [searchParams]);
  const copyManifestYaml = async () => {
    if (!manifestYaml || typeof navigator === "undefined" || !navigator.clipboard) {
      return;
    }
    try {
      await navigator.clipboard.writeText(manifestYaml);
      void messageApi.success(t("manifest.copy_success"));
    } catch (error) {
      void messageApi.error(translateApiError(t, error as Error));
    }
  };
  const detailItems: DescriptionsProps["items"] = [
    {
      key: "name",
      label: t("field.name"),
      children: vmData?.name ? (
        <Text
          strong={true}
          copyable={{ text: vmData.name }}
          className="selectable-inline-text"
        >
          {vmData.name}
        </Text>
      ) : (
        "—"
      ),
    },
    {
      key: "status",
      label: t("common:table.status"),
      children: (
        <Badge
          status={mapped.badge}
          text={
            <Tag color={mapped.color}>{t(`status.${status ?? "UNKNOWN"}`)}</Tag>
          }
        />
      ),
    },
    {
      key: "namespace",
      label: t("field.namespace"),
      children: <Tag>{vmData?.namespace}</Tag>,
    },
    hostnameVisible
      ? {
          key: "hostname",
          label: t("field.hostname"),
          children: vmData?.hostname ? (
            <Text
              type="secondary"
              copyable={{ text: vmData.hostname }}
              className="selectable-inline-text"
            >
              {vmData.hostname}
            </Text>
          ) : (
            "—"
          ),
        }
      : null,
    {
      key: "ipAddress",
      label: t("field.ip_address"),
      children: vmData?.ip_address ? (
        <Text
          copyable={{ text: vmData.ip_address }}
          className="selectable-inline-text"
        >
          {vmData.ip_address}
        </Text>
      ) : (
        "—"
      ),
    },
    {
      key: "hostIp",
      label: t("field.host_ip"),
      children: vmData?.host_ip ? (
        <Text className="selectable-inline-text">{vmData.host_ip}</Text>
      ) : (
        "—"
      ),
    },
    {
      key: "nodeName",
      label: t("field.node_name"),
      children: vmData?.node_name ? (
        <Text
          copyable={{ text: vmData.node_name }}
          className="selectable-inline-text"
        >
          {vmData.node_name}
        </Text>
      ) : (
        "—"
      ),
    },
    {
      key: "scope",
      label: t("field.scope"),
      children:
        vmData?.system_id && vmData?.system_name ? (
          <Space size={0} direction="vertical">
            <Typography.Link
              className="selectable-inline-text"
              onClick={() =>
                router.push(`/systems?detail_system_id=${vmData.system_id}`)
              }
            >
              {vmData.system_name}
            </Typography.Link>
            {vmData.service_id && vmData.service_name ? (
              <Typography.Link
                className="selectable-inline-text"
                onClick={() =>
                  router.push(
                    `/services?system_id=${vmData.system_id}&detail_service_id=${vmData.service_id}`,
                  )
                }
              >
                {vmData.service_name}
              </Typography.Link>
            ) : null}
          </Space>
        ) : (
          scopeValue
        ),
    },
    {
      key: "cluster",
      label: t("field.cluster"),
      children: vmData?.cluster_name || "—",
    },
    {
      key: "operatingSystem",
      label: t("field.operating_system"),
      children: operatingSystemLabel ? (
        <Text className="selectable-inline-text">{operatingSystemLabel}</Text>
      ) : (
        "—"
      ),
    },
    {
      key: "environment",
      label: t("field.environment"),
      children: vmData?.environment ? <Tag>{vmData.environment}</Tag> : "—",
    },
    {
      key: "remoteAccess",
      label: t("field.remote_access"),
      children: remoteAccessMode ? (
        <Space direction="vertical" size={4}>
          <Tag color={remoteAccessMode === "RDP" ? "blue" : "green"}>
            {remoteAccessMode}
          </Tag>
          {remoteAccessCommand ? (
            <Text
              code={true}
              copyable={{ text: remoteAccessCommand }}
              className="selectable-inline-text"
            >
              {remoteAccessCommand}
            </Text>
          ) : null}
          {remoteAccessDescription ? (
            <Text type="secondary">{remoteAccessDescription}</Text>
          ) : null}
        </Space>
      ) : (
        <Text type="secondary">{t("remote_access.unavailable")}</Text>
      ),
    },
    {
      key: "resources",
      label: t("field.resources"),
      children:
        [
          formatCPU(vmData?.cpu_cores),
          formatMemory(vmData?.memory_gi ?? 0),
          formatDisk(vmData?.disk_gb),
        ]
          .filter((value) => value !== "—")
          .join(" · ") || "—",
    },
    {
      key: "consoleAccess",
      label: t("field.console_access"),
      children: consoleCapabilities ? (
        <Space wrap size={[8, 8]}>
          <Tag color={consoleCapabilities.serial_available ? "green" : "default"}>
            {consoleCapabilities.serial_available
              ? t("console.serial_available")
              : t("console.serial_disabled")}
          </Tag>
          <Tag color={consoleCapabilities.vnc_available ? "green" : "default"}>
            {consoleCapabilities.vnc_available
              ? t("console.vnc_available")
              : t("console.graphics_disabled")}
          </Tag>
          {consoleCapabilities.preferred_console_type ? (
            <Tag color="blue">
              {consoleCapabilities.preferred_console_type === "SERIAL"
                ? t("console.preferred_serial")
                : t("console.preferred_vnc")}
            </Tag>
          ) : (
            <Text type="secondary">{t("console.no_console_available")}</Text>
          )}
          {consoleStatus?.status ? (
            <Tag color="processing">
              {t(`console.request_status_${consoleStatus.status}`)}
            </Tag>
          ) : null}
          {consoleStatus?.ticket_id ? (
            <Text
              copyable={{ text: consoleStatus.ticket_id }}
              className="selectable-inline-text"
            >
              {consoleStatus.ticket_id}
            </Text>
          ) : null}
        </Space>
      ) : (
        "—"
      ),
    },
    {
      key: "instance",
      label: t("field.instance"),
      children: vmData?.instance || "—",
    },
    {
      key: "ticket",
      label: t("field.ticket"),
      children: vmData?.ticket_id ? (
        <Text
          copyable={{ text: vmData.ticket_id }}
          className="selectable-inline-text"
        >
          {vmData.ticket_id}
        </Text>
      ) : (
        "—"
      ),
    },
    {
      key: "createdAt",
      label: t("common:table.created_at"),
      children: (
        <Text type="secondary">
          {vmData?.created_at
            ? dayjs(vmData.created_at).format("YYYY-MM-DD HH:mm:ss")
            : "—"}
        </Text>
      ),
    },
  ].filter(Boolean) as DescriptionsProps["items"];

  return (
    <div data-testid="vm-detail-page">
      {messageContextHolder}
      <PageHeader
        title={
          <Space size="small">
            <DesktopOutlined style={{ color: "#531dab" }} />
            <Text
              strong={true}
              copyable={vmData?.name ? { text: vmData.name } : undefined}
              className="selectable-inline-text"
            >
              {vmData?.name ?? vmId}
            </Text>
          </Space>
        }
        subtitle={t("detail.subtitle")}
        actions={
          <Button
            icon={<ArrowLeftOutlined />}
            type="text"
            onClick={() => router.push("/vms")}
          >
            {t("common:button.back")}
          </Button>
        }
      />
      {isCodespacesDemo ? (
        <Alert
          type="warning"
          showIcon
          style={{ marginBottom: 16 }}
          message={t("demo.notice_title")}
          description={t("demo.notice_description")}
        />
      ) : null}

      <PageSurface loading={isLoading}>
        <Descriptions bordered column={2} items={detailItems} />
      </PageSurface>

      <PageSurface title={t("common:table.actions")}>
        <Space wrap className="copy-friendly-actions">
          <Popconfirm
            title={t("action.start_confirm")}
            okText={t("common:button.confirm")}
            cancelText={t("common:button.cancel")}
            disabled={!isStopped}
            onConfirm={() => powerMutation.mutate({ action: "start" })}
          >
            <Button
              type="primary"
              icon={<PlayCircleOutlined />}
              data-testid={`vm-action-start-${vmId}`}
              disabled={!isStopped}
              loading={powerMutation.isPending}
            >
              {t("action.start")}
            </Button>
          </Popconfirm>
          <Popconfirm
            title={t("action.stop_confirm")}
            okText={t("common:button.confirm")}
            cancelText={t("common:button.cancel")}
            disabled={!isStoppable}
            onConfirm={() => powerMutation.mutate({ action: "stop" })}
          >
            <Button
              icon={<PauseCircleOutlined />}
              data-testid={`vm-action-stop-${vmId}`}
              disabled={!isStoppable}
              loading={powerMutation.isPending}
            >
              {t("action.stop")}
            </Button>
          </Popconfirm>
          <Popconfirm
            title={t("action.restart_confirm")}
            okText={t("common:button.confirm")}
            cancelText={t("common:button.cancel")}
            disabled={!isRunning}
            onConfirm={() => powerMutation.mutate({ action: "restart" })}
          >
            <Button
              icon={<RedoOutlined />}
              data-testid={`vm-action-restart-${vmId}`}
              disabled={!isRunning}
              loading={powerMutation.isPending}
            >
              {t("action.restart")}
            </Button>
          </Popconfirm>
          <Button
            icon={<DesktopOutlined />}
            data-testid={`vm-action-console-${vmId}`}
            disabled={!isRunning || !consoleAvailable}
            loading={consolePending}
            onClick={openConsoleChooser}
          >
            {activeConsoleTarget ? t("action.switch_console") : t("action.console")}
          </Button>
          {canViewManifest ? (
            <Button
              icon={<FileTextOutlined />}
              data-testid={`vm-action-manifest-${vmId}`}
              onClick={() => setManifestViewerOpen(true)}
            >
              {t("action.view_manifest")}
            </Button>
          ) : null}
          <Button
            icon={<CopyOutlined />}
            data-testid={`vm-action-request-similar-${vmId}`}
            onClick={() =>
              router.push(`/vms?request=create&source_vm_id=${vmId}`)
            }
          >
            {t("action.request_similar")}
          </Button>
          <Button
            icon={<DesktopOutlined />}
            data-testid={`vm-console-status-${vmId}`}
            onClick={() => {
              void refetch();
              if (isRunning && consoleAvailable) {
                void refetchConsoleStatus();
              }
            }}
            loading={isLoading}
          >
            {t("action.refresh_status")}
          </Button>
          <Button
            danger
            icon={<DeleteOutlined />}
            data-testid={`vm-action-delete-${vmId}`}
            disabled={!canDelete || !vmData?.name}
            loading={deleteMutation.isPending}
            onClick={() => {
              setDeleteConfirmName("");
              setDeleteOpen(true);
            }}
          >
            {t("action.delete")}
          </Button>
        </Space>
      </PageSurface>
      <div
        ref={consoleSectionRef}
        id="vm-console-section"
        data-testid="vm-console-section"
      >
        <PageSurface
          title={t("field.console_access")}
          style={{ marginTop: 16 }}
        >
          <VMConsoleSessionPanel
            vmId={vmId}
            target={activeConsoleTarget}
            onReconnectConsole={reconnectConsoleSession}
          />
        </PageSurface>
      </div>
      <ConsoleModeModal
        open={consoleChooserOpen}
        loading={consolePending}
        vmName={vmData?.name}
        vm={vmData}
        capabilities={consoleCapabilities}
        value={selectedConsoleType}
        onCancel={closeConsoleChooser}
        onChange={setSelectedConsoleType}
        onConfirm={() => void confirmConsoleChooser()}
      />
      <Modal
        title={t("manifest.title")}
        open={manifestViewerOpen}
        onCancel={() => setManifestViewerOpen(false)}
        destroyOnHidden={true}
        width={960}
        footer={[
          <Button
            key="copy"
            icon={<CopyOutlined />}
            onClick={() => void copyManifestYaml()}
            disabled={!manifestYaml}
          >
            {t("manifest.copy")}
          </Button>,
          <Button
            key="close"
            type="primary"
            onClick={() => setManifestViewerOpen(false)}
          >
            {t("common:button.close")}
          </Button>,
        ]}
      >
        <Space direction="vertical" size={12} style={{ width: "100%" }}>
          <Text type="secondary">{t("manifest.description")}</Text>
          {manifestQuery.error ? (
            <Alert
              type="error"
              showIcon={true}
              message={t("manifest.unavailable")}
              description={translateApiError(t, manifestQuery.error)}
            />
          ) : null}
          <Input.TextArea
            data-testid="vm-manifest-content"
            value={
              manifestQuery.isLoading || manifestQuery.isFetching
                ? t("manifest.loading")
                : manifestYaml
            }
            readOnly={true}
            spellCheck={false}
            autoSize={{ minRows: 20, maxRows: 28 }}
            placeholder={t("manifest.empty")}
            style={{
              fontFamily:
                'SFMono-Regular, Consolas, "Liberation Mono", Menlo, monospace',
            }}
          />
        </Space>
      </Modal>
      {deleteOpen ? (
        <Modal
          title={
            <Space>
              <ExclamationCircleOutlined style={{ color: "#ff4d4f" }} />
              {t("action.delete_confirm")}
            </Space>
          }
          open={true}
          onOk={() => {
            if (!vmData?.name) {
              return;
            }
            if (requiresNameConfirm && deleteConfirmName !== vmData.name) {
              void messageApi.warning(t("action.delete_type_name_hint"));
              return;
            }
            deleteMutation.mutate();
          }}
          onCancel={() => {
            setDeleteOpen(false);
            setDeleteConfirmName("");
          }}
          confirmLoading={deleteMutation.isPending}
          okButtonProps={{ danger: true, disabled: !deleteConfirmMatched }}
          okText={t("common:button.delete")}
          data-testid="vm-delete-modal"
        >
          <Paragraph>
            {t("action.delete_confirm_name", { name: vmData?.name ?? vmId })}
          </Paragraph>
          {requiresNameConfirm && (
            <>
              <Paragraph type="secondary">
                {t("action.delete_type_name_hint")}
              </Paragraph>
              <Input
                value={deleteConfirmName}
                onChange={(e) => setDeleteConfirmName(e.target.value)}
                placeholder={vmData?.name ?? vmId}
                status={
                  deleteConfirmName && deleteConfirmName !== vmData?.name
                    ? "error"
                    : undefined
                }
              />
            </>
          )}
        </Modal>
      ) : null}
    </div>
  );
}
