import { render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const controllerState = vi.hoisted(() => ({
  overrides: {} as Record<string, unknown>,
}));

function rootVolumeModeOptionKey(option: {
  access_modes?: string[];
  volume_mode?: string;
} | undefined): string {
  if (!option?.volume_mode || !option.access_modes?.length) {
    return "";
  }
  return `${option.volume_mode}|${[...option.access_modes].sort().join(",")}`;
}

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string, fallback?: string | { defaultValue?: string }) => {
      if (typeof fallback === "string") {
        return fallback;
      }
      return fallback?.defaultValue ?? key;
    },
  }),
}));

vi.mock("../hooks/useAdminApprovalsController", async () => {
  const { Form } = await import("antd");
  return {
    useAdminApprovalsController: () => {
      const [approveForm] = Form.useForm();
      const [rejectForm] = Form.useForm();
      const selectedRootVolumeResolution =
        (controllerState.overrides.selectedRootVolumeResolution as
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
        ((selectedRootVolumeResolution.mode_options as
          | Array<{ access_modes?: string[]; volume_mode?: string }>
          | undefined) ??
          []);
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
                volume_mode: selectedRootVolumeResolution.effective_volume_mode as
                  | string
                  | undefined,
              }
            : undefined);
      return {
        messageContextHolder: null,
        statusFilter: "PENDING",
        changeStatusFilter: vi.fn(),
        operationFilter: "ALL",
        changeOperationFilter: vi.fn(),
        selectedClusterFilter: "",
        changeSelectedClusterFilter: vi.fn(),
        placementSnapshotFilter: "ALL",
        changePlacementSnapshotFilter: vi.fn(),
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
          ],
        },
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
          (controllerState.overrides.canSelectRootVolumeMode as boolean | undefined) ??
          rootVolumeModeOptions.length > 1,
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
        approvePending: false,
        rejectPending: false,
        cancelPending: false,
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

    expect(screen.getByTestId("approval-provisioning-card")).toBeInTheDocument();
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

    expect(screen.getByText("Batch Request")).toBeInTheDocument();
    expect(screen.getAllByText("Ubuntu 22.04").length).toBeGreaterThan(0);
    expect(screen.getAllByText("M4 Large").length).toBeGreaterThan(0);
    expect(
      screen.getByText(
        "One approval decision will apply to every VM in this batch.",
      ),
    ).toBeInTheDocument();
    expect(screen.getAllByText("80 GB").length).toBeGreaterThan(0);
  }, 10000);

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
              "storage class \"rook-ceph\" supports multiple root volume modes; approval must choose one explicit combination",
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
          "storage class \"rook-ceph\" supports multiple root volume modes; approval must choose one explicit combination",
        mode_options: [
          { volume_mode: "Block", access_modes: ["ReadWriteMany"] },
          { volume_mode: "Block", access_modes: ["ReadWriteOnce"] },
        ],
      },
    };

    render(<AdminApprovalsContent />);

    expect(screen.getAllByText("Select root volume mode").length).toBeGreaterThan(0);
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
    expect(screen.getAllByText("Block + ReadWriteMany").length).toBeGreaterThan(0);
  });
});
