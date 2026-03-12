import { render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const controllerState = vi.hoisted(() => ({
  overrides: {} as Record<string, unknown>,
}));

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
              },
            },
          ],
        },
        selectedClusterId: "cluster-a",
        selectedClusterPolicy: {
          id: "policy-1",
          cluster_id: "cluster-a",
          allow_cdi_clone: true,
          allowed_storage_classes: ["rook-ceph"],
        },
        selectedClusterPolicyLoading: false,
        selectedClusterStorageClassOptions: ["rook-ceph"],
        approveCreateContext: {
          hasMixedSelection: false,
        },
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
  });

  it("keeps storage-class-resolvable clusters selectable and shows detected storage classes", () => {
    render(<AdminApprovalsContent />);

    expect(
      screen.getByText(
        "Auto-detected from the selected cluster. You can change it before approving.",
      ),
    ).toBeInTheDocument();
  });
});
