import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const controllerState = vi.hoisted(() => ({
  overrides: {} as Record<string, unknown>,
  push: vi.fn(),
}));

vi.mock("next/navigation", () => ({
  useRouter: () => ({
    push: controllerState.push,
  }),
}));

function rootVolumeModeOptionKey(
  option:
    | {
        access_modes?: string[];
        volume_mode?: string;
      }
    | undefined,
): string {
  if (!option?.volume_mode || !option.access_modes?.length) {
    return "";
  }
  return `${option.volume_mode}|${[...option.access_modes].sort().join(",")}`;
}

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (
      key: string,
      options?:
        | string
        | { defaultValue?: string; count?: number; index?: number },
    ) => {
      if (typeof options === "string") {
        return options;
      }
      const labels: Record<string, string> = {
        "common:nav.approval_tasks": "Approval Tasks",
        "empty.pending_title": "No approvals waiting yet",
        "empty.pending_description":
          "Finish setup or submit the first VM request to exercise the approval flow.",
        "empty.open_vm_request": "Open VM Request",
        "empty.filtered_title": "No approvals match the current filters",
        "empty.filtered_description":
          "Reset the queue filters to see the broader approval backlog again.",
        "summary.overview_title": "Review Overview",
        "summary.scope_title": "Resource Context",
        "summary.change_title": "Requested Change",
        "summary.affected_items_title": "Affected Items",
        subtitle: "Review and process pending approval tasks",
        "summary.system": "System",
        "summary.service": "Service",
        "summary.namespace": "Namespace",
        "summary.cluster": "Cluster",
        "summary.cluster_environment": "Environment",
        "summary.virtual_machine": "Virtual Machine",
        "summary.virtual_machine_status": "VM Status",
        "summary.request_vm_status": "Request-Time Status",
        "summary.latest_vm_status": "Latest Status",
        "summary.status_changed": "Changed since request",
        "summary.template": "Template",
        "summary.instance_size": "Instance Size",
        "summary.current_resources": "Current Resources",
        "summary.target_resources": "Requested Resources",
        "summary.power_action": "Requested Action",
        "summary.batch_count": "Affected Requests",
        "summary.item": "Request",
        "summary.scope": "Scope",
        "summary.irreversible_title": "Irreversible request",
        "summary.irreversible_description":
          "Approving this request will permanently remove the virtual machine and related resources. Verify the system, service, namespace, cluster, and VM before continuing.",
        "vm:status.STOPPED": "Stopped",
        "vm:batch.retry_failed": "Retry failed",
        "vm:batch.detail_title": "Batch detail",
      };
      if (key === "summary.shape_cpu" && typeof options?.count === "number") {
        return `${options.count} vCPU`;
      }
      if (
        key === "summary.shape_memory" &&
        typeof options?.count === "number"
      ) {
        return `${options.count} Gi memory`;
      }
      if (key === "summary.shape_disk" && typeof options?.count === "number") {
        return `${options.count} Gi disk`;
      }
      if (
        key === "summary.item_fallback" &&
        typeof options?.index === "number"
      ) {
        return `Request #${options.index}`;
      }
      const template = labels[key] ?? options?.defaultValue ?? key;
      if (typeof template !== "string" || !options) {
        return template;
      }
      return Object.entries(options).reduce((result, [name, value]) => {
        if (
          name === "defaultValue" ||
          value === undefined ||
          value === null ||
          typeof value === "object"
        ) {
          return result;
        }
        return result.replaceAll(`{{${name}}}`, String(value));
      }, template);
    },
  }),
}));

vi.mock("@/components/feedback/ActionEmptyState", () => ({
  ActionEmptyState: ({
    title,
    description,
    actions,
  }: {
    title: string;
    description?: string;
    actions?: ReactNode;
  }) => (
    <section data-testid="action-empty-state">
      <h2>{title}</h2>
      {description ? <p>{description}</p> : null}
      {actions}
    </section>
  ),
}));

vi.mock("@/components/feedback/SummaryMetricCard", () => ({
  SummaryMetricCard: ({
    title,
    value,
    description,
    action,
  }: {
    title: ReactNode;
    value?: ReactNode;
    description?: ReactNode;
    action?: ReactNode;
  }) => (
    <section data-testid="summary-metric-card">
      <h2>{title}</h2>
      {value ? <div>{value}</div> : null}
      {description ? <div>{description}</div> : null}
      {action}
    </section>
  ),
}));

vi.mock("@/components/layouts/PageSection", () => ({
  PageHeader: ({
    title,
    subtitle,
    actions,
  }: {
    title: ReactNode;
    subtitle?: ReactNode;
    actions?: ReactNode;
  }) => (
    <header data-testid="page-header">
      <h1>{title}</h1>
      {subtitle ? <p>{subtitle}</p> : null}
      {actions}
    </header>
  ),
  PageSurface: ({ children }: { children?: ReactNode }) => (
    <section data-testid="page-surface">{children}</section>
  ),
}));

vi.mock("@/components/ui/LocalDateTimeText", () => ({
  LocalDateTimeText: ({ value }: { value: string }) => (
    <time dateTime={value}>{value}</time>
  ),
}));

vi.mock("@/components/workbench/WorkbenchDetailModal", () => ({
  WorkbenchDetailModal: ({
    open,
    title,
    children,
    footer,
  }: {
    open?: boolean;
    title?: ReactNode;
    children?: ReactNode;
    footer?: ReactNode;
  }) =>
    open ? (
      <section data-testid="mock-workbench-detail-modal">
        {title ? <div>{title}</div> : null}
        <div
          className="workbench-detail-modal__viewport"
          style={{ maxHeight: "calc(100vh - 180px)" }}
        >
          {children}
        </div>
        {footer ? <div>{footer}</div> : null}
      </section>
    ) : null,
}));

vi.mock("@/components/illustrations/DashboardIllustrations", () => ({
  QueueReviewGlyph: (props: Record<string, unknown>) => (
    <span {...props}>queue-glyph</span>
  ),
  RequestsOverviewGlyph: (props: Record<string, unknown>) => (
    <span {...props}>requests-glyph</span>
  ),
  ServiceWorkspaceGlyph: (props: Record<string, unknown>) => (
    <span {...props}>service-glyph</span>
  ),
  VirtualMachinesOverviewGlyph: (props: Record<string, unknown>) => (
    <span {...props}>vms-glyph</span>
  ),
}));

vi.mock("antd", async () => {
  const actual = await vi.importActual<typeof import("antd")>("antd");

  const Card = ({
    children,
    title,
    extra,
    ...rest
  }: {
    children?: ReactNode;
    title?: ReactNode;
    extra?: ReactNode;
    [key: string]: unknown;
  }) => (
    <section data-testid="mock-card" {...rest}>
      {title ? <div>{title}</div> : null}
      {extra ? <div>{extra}</div> : null}
      {children}
    </section>
  );

  const DescriptionsItem = ({
    label,
    children,
  }: {
    label?: ReactNode;
    children?: ReactNode;
  }) => (
    <div data-testid="mock-description-item">
      {label ? <dt>{label}</dt> : null}
      <dd>{children}</dd>
    </div>
  );

  const Descriptions = (({
    children,
    items,
  }: {
    children?: ReactNode;
    items?: Array<{ key?: string; label?: ReactNode; children?: ReactNode }>;
  }) => (
    <dl data-testid="mock-descriptions">
      {items?.map((item, index) => (
        <DescriptionsItem key={item.key ?? String(index)} label={item.label}>
          {item.children}
        </DescriptionsItem>
      ))}
      {children}
    </dl>
  )) as ((props: {
    children?: ReactNode;
    items?: Array<{ key?: string; label?: ReactNode; children?: ReactNode }>;
  }) => ReactNode) & {
    Item: typeof DescriptionsItem;
  };
  Descriptions.Item = DescriptionsItem;

  const Table = <T extends Record<string, unknown>>({
    dataSource,
    columns,
    locale,
  }: {
    dataSource?: T[];
    columns?: Array<{
      key?: string;
      title?: ReactNode;
      dataIndex?: string | string[];
      render?: (value: unknown, record: T, index: number) => ReactNode;
    }>;
    locale?: { emptyText?: ReactNode };
  }) => {
    if (!dataSource || dataSource.length === 0) {
      return <div>{locale?.emptyText ?? null}</div>;
    }
    return (
      <table data-testid="mock-table">
        <thead>
          <tr>
            {columns?.map((column, index) => (
              <th key={column.key ?? String(index)}>{column.title}</th>
            ))}
          </tr>
        </thead>
        <tbody>
          {dataSource.map((record, rowIndex) => (
            <tr key={String(record.id ?? rowIndex)}>
              {columns?.map((column, columnIndex) => {
                const rawValue = Array.isArray(column.dataIndex)
                  ? column.dataIndex.reduce<unknown>(
                      (value, key) =>
                        value && typeof value === "object"
                          ? (value as Record<string, unknown>)[key]
                          : undefined,
                      record,
                    )
                  : typeof column.dataIndex === "string"
                    ? record[column.dataIndex]
                    : undefined;
                const content = column.render
                  ? column.render(rawValue, record, rowIndex)
                  : rawValue;
                return (
                  <td key={column.key ?? String(columnIndex)}>
                    {content as ReactNode}
                  </td>
                );
              })}
            </tr>
          ))}
        </tbody>
      </table>
    );
  };

  const Popover = ({
    children,
    content,
  }: {
    children?: ReactNode;
    content?: ReactNode;
  }) => (
    <div data-testid="mock-popover">
      {children}
      {content ? <div>{content}</div> : null}
    </div>
  );

  return {
    ...actual,
    Card,
    Descriptions,
    Popover,
    Table,
  };
});

vi.mock("@/features/setup-guide/hooks/useSetupGuide", () => ({
  useSetupGuide: () => ({
    systemsTotal: 1,
    servicesTotal: 1,
    vmsTotal: 0,
    namespacesTotal: 1,
    templatesTotal: 1,
    instanceSizesTotal: 1,
    canCreateSystem: true,
    canCreateService: true,
    canCreateVM: true,
    canManageNamespaces: true,
    canManageTemplates: true,
    canManageInstanceSizes: true,
    systemReady: true,
    serviceReady: true,
    prerequisitesReady: true,
    vmRequestReady: true,
    hasRequestedFirstVM: false,
    isLoading: false,
  }),
}));

vi.mock("@/features/setup-guide/components/SetupGuideCard", () => ({
  SetupGuideCard: ({ variant }: { variant: string }) => (
    <div>{`setup-guide-${variant}`}</div>
  ),
}));

vi.mock("../hooks/useAdminApprovalsController", async () => {
  const { Form } = await import("antd");
  return {
    useAdminApprovalsController: () => {
      const [approveForm] = Form.useForm();
      const [rejectForm] = Form.useForm();
      const selectedRootVolumeResolution = (controllerState.overrides
        .selectedRootVolumeResolution as
        | Record<string, unknown>
        | undefined) ?? {
        state: "storage_class_required",
        message:
          "approval must select a target storage class before root volume provisioning can be resolved",
      };
      const rootVolumeModeOptions =
        (controllerState.overrides.rootVolumeModeOptions as
          | Array<{ access_modes?: string[]; volume_mode?: string }>
          | undefined) ??
        (selectedRootVolumeResolution.mode_options as
          | Array<{ access_modes?: string[]; volume_mode?: string }>
          | undefined) ??
        [];
      const effectiveSelectedRootVolumeMode =
        (controllerState.overrides.effectiveSelectedRootVolumeMode as
          | { access_modes?: string[]; volume_mode?: string }
          | undefined) ??
        (rootVolumeModeOptions.length === 1
          ? rootVolumeModeOptions[0]
          : selectedRootVolumeResolution.effective_volume_mode
            ? {
                access_modes:
                  (selectedRootVolumeResolution.effective_access_modes as
                    | string[]
                    | undefined) ?? [],
                volume_mode:
                  selectedRootVolumeResolution.effective_volume_mode as
                    | string
                    | undefined,
              }
            : undefined);
      return {
        messageContextHolder: null,
        statusFilter: "PENDING",
        changeStatusFilter: vi.fn(),
        searchFilter: "",
        changeSearchFilter: vi.fn(),
        operationFilter: "ALL",
        changeOperationFilter: vi.fn(),
        selectedClusterFilter: "",
        changeSelectedClusterFilter: vi.fn(),
        placementAdvisoryFilter: "",
        changePlacementAdvisoryFilter: vi.fn(),
        placementSnapshotFilter: "ALL",
        changePlacementSnapshotFilter: vi.fn(),
        resetFilters: vi.fn(),
        page: 1,
        pageSize: 20,
        setPage: vi.fn(),
        setPageSize: vi.fn(),
        data: {
          items: [
            {
              id: "ticket-1",
              event_id: "event-1",
              status: "PENDING",
              operation_type: "CREATE",
              requester: "alice",
              provisioning: {
                phase: "CloneInProgress",
                progress: "45%",
                clone_type: "copy",
                failure_message: "target pod restarted once",
              },
            },
          ],
          pagination: { page: 1, per_page: 20, total: 0, total_pages: 0 },
        },
        isLoading: false,
        listError: undefined,
        refetch: vi.fn(),
        approveModal: {
          id: "ticket-1",
          event_id: "event-1",
          status: "PENDING",
          operation_type: "CREATE",
          requester: "alice",
          reason: "scale up",
          ticket_payload: {
            namespace: "prod-a",
            template_id: "tpl-1",
            instance_size_id: "size-1",
            dedicated_cpu: true,
          },
          provisioning: {
            phase: "CloneInProgress",
            progress: "45%",
            claim_name: "target-root-pvc",
            pvc_phase: "Bound",
            clone_type: "copy",
            clone_phase: "Succeeded",
            clone_fallback_reason:
              "The volume modes of source and target are incompatible",
            failure_message: "target pod restarted once",
          },
        },
        rejectModal: null,
        approveForm,
        rejectForm,
        clustersData: {
          items: [
            {
              id: "cluster-a",
              display_name: "Production Cluster A",
              name: "Cluster A",
              environment: "prod",
              enabled: true,
              compatibility: {
                eligible: false,
                reason_code: "CLUSTER_POLICY_STORAGE_CLASS_REQUIRED",
                reason_message:
                  "cluster policy requires an explicit allowed storage class",
                root_volume_resolution: {
                  state: "storage_class_required",
                  message:
                    "approval must select a target storage class before root volume provisioning can be resolved",
                },
              },
            },
          ],
        },
        filterClusters: [
          {
            id: "cluster-a",
            display_name: "Production Cluster A",
            name: "Cluster A",
            environment: "prod",
            enabled: true,
          },
          {
            id: "cluster-b",
            display_name: "Test Cluster B",
            name: "Cluster B",
            environment: "test",
            enabled: true,
          },
        ],
        filterClustersLoading: false,
        clusterQueryError: undefined,
        clusterQueryLoading: false,
        selectedClusterId: "cluster-a",
        selectedCluster: {
          id: "cluster-a",
          name: "Cluster A",
          enabled: true,
          compatibility: {
            eligible: false,
            reason_code: "CLUSTER_POLICY_STORAGE_CLASS_REQUIRED",
            reason_message:
              "cluster policy requires an explicit allowed storage class",
            root_volume_resolution: {
              state: "storage_class_required",
              message:
                "approval must select a target storage class before root volume provisioning can be resolved",
            },
          },
        },
        selectedClusterPolicy: {
          id: "policy-1",
          cluster_id: "cluster-a",
          allow_cdi_clone: true,
          allowed_storage_classes: ["rook-ceph"],
        },
        selectedClusterPolicyLoading: false,
        selectedClusterStorageClassOptions: ["rook-ceph"],
        effectiveSelectedStorageClass: "rook-ceph",
        selectedRootVolumeResolution,
        rootVolumeModeOptions,
        canSelectRootVolumeMode:
          (controllerState.overrides.canSelectRootVolumeMode as
            | boolean
            | undefined) ?? rootVolumeModeOptions.length > 1,
        requiresManualRootVolumeModeInput:
          (controllerState.overrides.requiresManualRootVolumeModeInput as
            | boolean
            | undefined) ?? false,
        effectiveSelectedRootVolumeMode,
        effectiveSelectedRootVolumeModeKey:
          (controllerState.overrides.effectiveSelectedRootVolumeModeKey as
            | string
            | undefined) ??
          rootVolumeModeOptionKey(effectiveSelectedRootVolumeMode),
        approveCreateContext: {
          hasMixedSelection: false,
        },
        handleSelectedClusterChange: vi.fn(),
        handleSelectedStorageClassChange: vi.fn(),
        handleSelectedRootVolumeModeChange: vi.fn((value) => value),
        openApproveModal: vi.fn(),
        closeApproveModal: vi.fn(),
        openRejectModal: vi.fn(),
        closeRejectModal: vi.fn(),
        submitApprove: vi.fn(),
        submitReject: vi.fn(),
        submitCancel: vi.fn(),
        submitBatchRetry: vi.fn(),
        approvePending: false,
        rejectPending: false,
        cancelPending: false,
        retryBatchPending: false,
        ...controllerState.overrides,
      };
    },
  };
});

import { AdminApprovalsContent } from "./AdminApprovalsContent";
import { ApprovalProvisioningCard } from "./ApprovalProvisioningCard";

describe("AdminApprovalsContent", () => {
  beforeEach(() => {
    controllerState.overrides = {};
  });

  it("shows clone fallback details for create approvals with provisioning status", () => {
    render(
      <ApprovalProvisioningCard
        provisioning={{
          phase: "CloneInProgress",
          progress: "45%",
          claim_name: "target-root-pvc",
          pvc_phase: "Bound",
          clone_type: "copy",
          clone_phase: "Succeeded",
          clone_fallback_reason:
            "The volume modes of source and target are incompatible",
          failure_message: "target pod restarted once",
        }}
      />,
    );

    expect(
      screen.getByTestId("approval-provisioning-card"),
    ).toBeInTheDocument();
    expect(screen.getByTestId("approval-provisioning-phase")).toHaveTextContent(
      "CloneInProgress",
    );
    expect(
      screen.getByTestId("approval-provisioning-clone-type"),
    ).toHaveTextContent("Host-assisted copy");
    expect(
      screen.getByText(
        "The volume modes of source and target are incompatible",
      ),
    ).toBeInTheDocument();
    expect(screen.getByText("target pod restarted once")).toBeInTheDocument();
  });

  it("shows cluster compatibility query errors instead of silently rendering an empty list", () => {
    controllerState.overrides = {
      clusterQueryError: {
        code: "OVERCOMMIT_INVALID",
        message: "cpu request must not exceed cpu cores",
      },
      clustersData: undefined,
    };

    render(<AdminApprovalsContent />);

    expect(
      screen.getByText("Cluster compatibility check failed"),
    ).toBeInTheDocument();
    expect(
      screen.getByText("cpu request must not exceed cpu cores"),
    ).toBeInTheDocument();
  });

  it("shows parent batch approvals as a batch summary with labeled items", () => {
    controllerState.overrides = {
      approveModal: {
        id: "ticket-parent-1",
        event_id: "event-parent-1",
        status: "PENDING",
        operation_type: "CREATE",
        requester: "alice",
        ticket_payload: {
          batch_item_count: 2,
          batch_summary: {
            status: "PENDING",
            success_count: 0,
            failed_count: 0,
            pending_count: 2,
          },
          items: [
            {
              namespace: "prod-a",
              template_id: "tpl-1",
              template_label: "Ubuntu 22.04",
              instance_size_id: "size-1",
              instance_size_label: "M4 Large",
              instance_size_disk_gb: 80,
              instance_size_dedicated_cpu: true,
            },
            {
              namespace: "prod-a",
              template_id: "tpl-1",
              template_label: "Ubuntu 22.04",
              instance_size_id: "size-1",
              instance_size_label: "M4 Large",
              instance_size_disk_gb: 80,
              instance_size_dedicated_cpu: true,
            },
          ],
        },
      },
      approveCreateContext: {
        namespace: "prod-a",
        templateId: "tpl-1",
        instanceSizeId: "size-1",
        batchItemCount: 2,
        hasMixedSelection: false,
      },
    };

    render(<AdminApprovalsContent />);

    expect(screen.getByText("Affected Items")).toBeInTheDocument();
    expect(
      screen.getAllByText("Ubuntu 22.04 · M4 Large").length,
    ).toBeGreaterThan(0);
    expect(screen.getAllByText("Requested Resources").length).toBeGreaterThan(
      0,
    );
  });

  it("shows retry action for failed batch approvals", async () => {
    const user = userEvent.setup();
    const submitBatchRetry = vi.fn();
    controllerState.overrides = {
      data: {
        items: [
          {
            id: "ticket-batch-failed",
            event_id: "event-batch-failed",
            status: "FAILED",
            operation_type: "CREATE",
            requester: "alice",
            summary: {
              batch_count: 2,
              system_name: "shop",
              service_name: "redis",
            },
            ticket_payload: {
              items: [{ vm_name: "vm-a" }, { vm_name: "vm-b" }],
            },
          },
        ],
        pagination: { page: 1, per_page: 20, total: 1, total_pages: 1 },
      },
      submitBatchRetry,
    };

    render(<AdminApprovalsContent />);

    await user.click(screen.getByRole("button", { name: "Retry failed" }));
    expect(submitBatchRetry).toHaveBeenCalledWith("ticket-batch-failed");
  });

  it("keeps storage-class-resolvable clusters selectable and shows detected storage classes", () => {
    render(<AdminApprovalsContent />);

    expect(
      screen.getByText(
        "Exactly one eligible storage class was detected for this cluster and is auto-selected.",
      ),
    ).toBeInTheDocument();
  });

  it("asks the approver to select a root volume mode when the cluster exposes multiple StorageProfile combinations", () => {
    controllerState.overrides = {
      selectedCluster: {
        id: "cluster-a",
        name: "Cluster A",
        enabled: true,
        compatibility: {
          eligible: false,
          root_volume_resolution: {
            intent_mode: "auto",
            state: "mode_required",
            message:
              'storage class "rook-ceph" supports multiple root volume modes; approval must choose one explicit combination',
            mode_options: [
              { volume_mode: "Block", access_modes: ["ReadWriteMany"] },
              { volume_mode: "Block", access_modes: ["ReadWriteOnce"] },
            ],
          },
        },
      },
      selectedRootVolumeResolution: {
        intent_mode: "auto",
        state: "mode_required",
        message:
          'storage class "rook-ceph" supports multiple root volume modes; approval must choose one explicit combination',
        mode_options: [
          { volume_mode: "Block", access_modes: ["ReadWriteMany"] },
          { volume_mode: "Block", access_modes: ["ReadWriteOnce"] },
        ],
      },
    };

    render(<AdminApprovalsContent />);

    expect(
      screen.getAllByText("Select root volume mode").length,
    ).toBeGreaterThan(0);
    expect(screen.getByText("Root Volume Mode")).toBeInTheDocument();
    expect(
      screen.getByText("Recommended root volume mode"),
    ).toBeInTheDocument();
  });

  it("keeps root volume mode selectable after the cluster resolves a chosen mode", () => {
    controllerState.overrides = {
      selectedCluster: {
        id: "cluster-a",
        name: "Cluster A",
        enabled: true,
        compatibility: {
          eligible: true,
          root_volume_resolution: {
            intent_mode: "auto",
            state: "resolved",
            effective_storage_class: "rook-ceph-block",
            effective_volume_mode: "Block",
            effective_access_modes: ["ReadWriteMany"],
            mode_options: [
              { volume_mode: "Block", access_modes: ["ReadWriteMany"] },
              { volume_mode: "Block", access_modes: ["ReadWriteOnce"] },
            ],
          },
        },
      },
      selectedRootVolumeResolution: {
        intent_mode: "auto",
        state: "resolved",
        effective_storage_class: "rook-ceph-block",
        effective_volume_mode: "Block",
        effective_access_modes: ["ReadWriteMany"],
        mode_options: [
          { volume_mode: "Block", access_modes: ["ReadWriteMany"] },
          { volume_mode: "Block", access_modes: ["ReadWriteOnce"] },
        ],
      },
    };

    render(<AdminApprovalsContent />);

    expect(screen.getByText("Root Volume Mode")).toBeInTheDocument();
    expect(screen.getAllByText("Block + ReadWriteMany").length).toBeGreaterThan(
      0,
    );
  });

  it("prefers RWX combinations over RWO combinations when marking the recommended root volume mode", () => {
    controllerState.overrides = {
      selectedCluster: {
        id: "cluster-a",
        name: "Cluster A",
        enabled: true,
        compatibility: {
          eligible: false,
          root_volume_resolution: {
            intent_mode: "auto",
            state: "mode_required",
            message:
              'storage class "generic-sc" supports multiple root volume modes; approval must choose one explicit combination',
            mode_options: [
              { volume_mode: "Block", access_modes: ["ReadWriteOnce"] },
              { volume_mode: "Filesystem", access_modes: ["ReadWriteMany"] },
            ],
          },
        },
      },
      selectedRootVolumeResolution: {
        intent_mode: "auto",
        state: "mode_required",
        message:
          'storage class "generic-sc" supports multiple root volume modes; approval must choose one explicit combination',
        mode_options: [
          { volume_mode: "Block", access_modes: ["ReadWriteOnce"] },
          { volume_mode: "Filesystem", access_modes: ["ReadWriteMany"] },
        ],
      },
    };

    render(<AdminApprovalsContent />);

    expect(
      screen.getByText(/Filesystem \+ ReadWriteMany/),
    ).toBeInTheDocument();
  });

  it("allows manual root volume mode entry when StorageProfile claimPropertySets are missing", () => {
    controllerState.overrides = {
      requiresManualRootVolumeModeInput: true,
      selectedCluster: {
        id: "cluster-a",
        name: "Cluster A",
        enabled: true,
        compatibility: {
          eligible: false,
          root_volume_resolution: {
            intent_mode: "auto",
            state: "profile_incomplete",
            message:
              'storage class "block-sc" does not expose claimPropertySets in StorageProfile',
          },
        },
      },
      selectedRootVolumeResolution: {
        intent_mode: "auto",
        state: "profile_incomplete",
        message:
          'storage class "block-sc" does not expose claimPropertySets in StorageProfile',
      },
      effectiveSelectedStorageClass: "block-sc",
      selectedClusterStorageClassOptions: ["block-sc"],
      rootVolumeModeOptions: [],
    };

    render(<AdminApprovalsContent />);

    expect(
      screen.getByText("Select root volume mode manually"),
    ).toBeInTheDocument();
    expect(screen.getByText("Volume Mode")).toBeInTheDocument();
    expect(screen.getByText("Access Modes")).toBeInTheDocument();
    expect(
      screen.getByText(
        "Recommended: ReadWriteMany. If compatibility checks or provisioning cannot satisfy it, choose ReadWriteOnce instead.",
      ),
    ).toBeInTheDocument();
  });

  it("keeps the approve modal body scrollable when the content is long", () => {
    const { container } = render(<AdminApprovalsContent />);

    const modalViewport = container.ownerDocument.querySelector(
      ".workbench-detail-modal__viewport",
    );
    expect(modalViewport).not.toBeNull();
    expect(modalViewport).toHaveClass("workbench-detail-modal__viewport");
    expect(modalViewport).toHaveStyle({
      maxHeight: "calc(100vh - 180px)",
    });
  });

  it("uses the same title as the approval tasks navigation entry", () => {
    render(<AdminApprovalsContent />);

    expect(
      screen.getByRole("heading", { name: "Approval Tasks" }),
    ).toBeInTheDocument();
    expect(
      screen.getByText("Review and process pending approval tasks"),
    ).toBeInTheDocument();
  });

  it("uses searchable exact options for cluster and placement advisory filters", async () => {
    const user = userEvent.setup();

    render(<AdminApprovalsContent />);

    await user.click(screen.getByTestId("approvals-advanced-search-toggle"));

    expect(screen.getAllByRole("combobox").length).toBeGreaterThanOrEqual(4);
  });

  it("shows irreversible delete approvals with readable scope and current resource state", () => {
    controllerState.overrides = {
      approveModal: {
        id: "ticket-delete-1",
        event_id: "event-delete-1",
        status: "PENDING",
        operation_type: "DELETE",
        requester: "alice",
        reason: "cleanup old VM",
        target_vm_name: "vm-old-01",
        summary: {
          irreversible: true,
          system_name: "Payments",
          service_name: "Billing",
          namespace: "team-prod",
          cluster_name: "Prod Cluster A",
          cluster_environment: "prod",
          vm_name: "vm-old-01",
          request_vm_status: "STOPPED",
          latest_vm_status: "NOT_FOUND",
          template_name: "Ubuntu 22.04",
          instance_size_name: "M4 Large",
          current_cpu_cores: 4,
          current_memory_gi: 8,
          current_disk_gb: 80,
        },
      },
    };

    render(<AdminApprovalsContent />);

    expect(screen.getByText("Irreversible request")).toBeInTheDocument();
    expect(screen.getAllByText("Payments").length).toBeGreaterThan(0);
    expect(screen.getAllByText("Billing").length).toBeGreaterThan(0);
    expect(screen.getAllByText("Prod Cluster A").length).toBeGreaterThan(0);
    expect(screen.getAllByText("vm-old-01").length).toBeGreaterThan(0);
    expect(screen.getByText("Request-Time Status")).toBeInTheDocument();
    expect(screen.getByText("Latest Status")).toBeInTheDocument();
    expect(screen.getByText("Stopped")).toBeInTheDocument();
    expect(screen.getByText("NOT_FOUND")).toBeInTheDocument();
    expect(
      screen.getByText("4 vCPU · 8 Gi memory · 80 Gi disk"),
    ).toBeInTheDocument();
  });
});
