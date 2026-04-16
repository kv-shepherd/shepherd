import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const pushMock = vi.fn();
const refetchMock = vi.fn();
const refetchConsoleStatusMock = vi.fn();
const useApiGetMock = vi.fn();
const useApiMutationMock = vi.fn();
const authState = {
  user: {
    permissions: ["platform:admin"],
  },
};

vi.mock("next/navigation", () => ({
  useParams: () => ({ id: "vm-1" }),
  useRouter: () => ({
    push: pushMock,
  }),
  useSearchParams: () => new URLSearchParams(""),
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string, options?: { defaultValue?: string }) => {
      const labels: Record<string, string> = {
        "common:button.back": "Back",
        "detail.subtitle":
          "Review VM state, power actions, and console access.",
        "field.name": "Name",
        "common:table.status": "Status",
        "field.namespace": "Namespace",
        "field.hostname": "Hostname",
        "field.host_ip": "Host IP",
        "field.node_name": "Node",
        "field.ip_address": "IP Address",
        "field.scope": "Scope",
        "field.cluster": "Cluster",
        "field.operating_system": "Operating System",
        "field.remote_access": "Remote Access",
        "field.environment": "Environment",
        "field.resources": "Resources",
        "field.console_access": "Console Access",
        "field.instance": "Instance",
        "field.ticket": "Ticket",
        "common:table.created_at": "Created",
        "common:table.actions": "Actions",
        "action.start": "Start",
        "action.stop": "Stop",
        "action.restart": "Restart",
        "action.console": "Console",
        "action.request_similar": "Request Similar VM",
        "action.refresh_status": "Refresh Status",
        "action.delete": "Delete",
        "console.pending_approval":
          "Console access request submitted. Track it in My Requests.",
        "console.unavailable": "Console access is not currently available",
        "console.opened": "Console session opened",
        "console.serial_available": "Serial available",
        "console.serial_disabled": "Serial disabled",
        "console.vnc_available": "Graphics available",
        "console.graphics_disabled": "Graphics disabled",
        "console.preferred_serial": "Preferred: Serial",
        "console.preferred_vnc": "Preferred: Graphics",
        "console.no_console_available":
          "No console channel is currently available for this VM.",
        "console.request_status_NOT_REQUESTED":
          "Console request not submitted",
        "console.request_status_PENDING_APPROVAL":
          "Console request pending approval",
        "console.request_status_APPROVED": "Console request approved",
        "console.request_status_REJECTED": "Console request rejected",
        "console.chooser_title": "Choose Console Mode",
        "console.chooser_subtitle":
          "Select how to connect to vm-alpha. Your confirmed choice will be reused next time.",
        "console.option_serial_title": "Serial Console",
        "console.option_serial_description":
          "Best for headless servers and SSH-first workloads.",
        "console.option_vnc_title": "noVNC Console",
        "console.option_vnc_description":
          "Use the graphical console when a graphics device is attached. Server images may still appear blank if the guest does not output to a graphical display.",
        "console.remote_access_note": "Daily remote access",
        "console.current_mode": "Current console",
        "console.current_mode_serial": "Serial session",
        "console.current_mode_vnc": "noVNC session",
        "console.vnc_guest_output_hint":
          "noVNC needs both a graphics device and guest display output. Headless server images may still show a blank screen.",
        "console.chooser_default_hint":
          "Your last confirmed choice becomes the default next time.",
        "common:message.error": "Error",
        "common:button.confirm": "Confirm",
        "common:button.cancel": "Cancel",
        "remote_access.unavailable":
          "No guest remote access hint is available yet.",
        "remote_access.ssh_help":
          "Use SSH for routine Linux server access when the guest network is reachable.",
        "remote_access.rdp_help":
          "Use the native Windows Remote Desktop client when the guest network is reachable.",
      };
      return labels[key] ?? options?.defaultValue ?? key;
    },
  }),
}));

vi.mock("@/lib/api/useApiGet", () => ({
  useApiGet: (...args: unknown[]) => useApiGetMock(...args),
}));

vi.mock("@/hooks/useApiQuery", () => ({
  useApiMutation: (...args: unknown[]) => useApiMutationMock(...args),
}));

vi.mock("@/lib/api/client", () => ({
  api: {
    GET: vi.fn(),
    POST: vi.fn(),
    DELETE: vi.fn(),
  },
}));

vi.mock("@/lib/hooks/useMessage", () => ({
  useMessage: () => ({
    messageApi: {
      success: vi.fn(),
      error: vi.fn(),
      warning: vi.fn(),
    },
    messageContextHolder: null,
  }),
}));

vi.mock("@/stores/auth", () => ({
  useAuthStore: (selector: (state: typeof authState) => unknown) =>
    selector(authState),
}));

vi.mock("@/features/vm-management/components/VMConsoleSessionPanel", () => ({
  VMConsoleSessionPanel: ({
    target,
  }: {
    target: { consoleType: string; consolePath: string } | null;
  }) => (
    <div data-testid="vm-console-panel">
      {target ? `${target.consoleType}:${target.consolePath}` : "idle"}
    </div>
  ),
}));

import VMDetailPage from "./VMDetailPageContent";
import { api } from "@/lib/api/client";

describe("VMDetailPage", () => {
  beforeEach(() => {
    pushMock.mockReset();
    refetchMock.mockReset();
    refetchConsoleStatusMock.mockReset();
    useApiGetMock.mockReset();
    authState.user.permissions = ["platform:admin"];
  });

  it("renders the unified page shell and VM action surface", () => {
    useApiGetMock.mockImplementation((queryKey: unknown[]) => {
      if (queryKey[0] === "vm-console-status") {
        return {
          data: {
            status: "APPROVED",
            ticket_id: "console-ticket-1",
          },
          isLoading: false,
          refetch: refetchConsoleStatusMock,
        };
      }
      if (queryKey[0] === "vm-manifest") {
        return {
          data: {
            vm_id: "vm-1",
            name: "vm-alpha",
            namespace: "team-prod",
            cluster_id: "cluster-1",
            yaml: "kind: VirtualMachine\nmetadata:\n  name: vm-alpha\n",
          },
          isLoading: false,
          isFetching: false,
          refetch: vi.fn(),
        };
      }
      return {
        data: {
          id: "vm-1",
          name: "vm-alpha",
          status: "RUNNING",
          namespace: "team-prod",
          hostname: "vm-alpha.internal",
          host_ip: "10.1.2.3",
          node_name: "worker-a",
          ip_address: "10.0.0.18",
          system_id: "sys-1",
          system_name: "Payments",
          service_id: "svc-1",
          service_name: "Billing API",
          cluster_name: "Production Cluster",
          os_name: "Ubuntu 24.04.2 LTS",
          os_version: "24.04",
          os_family: "linux",
          cpu_cores: 4,
          memory_gi: 8,
          disk_gb: 60,
          console_capabilities: {
            serial_available: true,
            vnc_available: false,
            preferred_console_type: "SERIAL",
          },
          instance: "01",
          ticket_id: "ticket-1",
          created_at: "2026-03-17T00:00:00Z",
          environment: "prod",
        },
        isLoading: false,
        refetch: refetchMock,
      };
    });
    useApiMutationMock.mockReturnValue({
      isPending: false,
      mutate: vi.fn(),
    });

    render(<VMDetailPage />);

    expect(screen.getByTestId("vm-detail-page")).toBeVisible();
    expect(screen.getAllByText("vm-alpha").length).toBeGreaterThan(0);
    expect(
      screen.getByText("Review VM state, power actions, and console access."),
    ).toBeVisible();
    expect(screen.getByText("Back")).toBeVisible();
    expect(screen.getByText("Actions")).toBeVisible();
    expect(screen.getByTestId("vm-action-start-vm-1")).toBeVisible();
    expect(screen.getByTestId("vm-action-console-vm-1")).toBeVisible();
    expect(screen.getByTestId("vm-action-manifest-vm-1")).toBeVisible();
    expect(screen.getByTestId("vm-action-request-similar-vm-1")).toBeVisible();
    expect(screen.getByText("team-prod")).toBeVisible();
    expect(screen.getByText("Payments")).toBeVisible();
    expect(screen.getByText("Billing API")).toBeVisible();
    expect(screen.getByText("Production Cluster")).toBeVisible();
    expect(screen.getByText("10.0.0.18")).toBeVisible();
    expect(screen.getByText("10.1.2.3")).toBeVisible();
    expect(screen.getByText("worker-a")).toBeVisible();
    expect(screen.getByText("Ubuntu 24.04.2 LTS")).toBeVisible();
    expect(screen.getByText("ssh <username>@10.0.0.18")).toBeVisible();
    expect(screen.getByText("4 vCPU · 8 Gi · 60 Gi")).toBeVisible();
    expect(screen.getByText("Serial available")).toBeVisible();
    expect(screen.getByText("Graphics disabled")).toBeVisible();
    expect(screen.getByText("Preferred: Serial")).toBeVisible();
    expect(screen.getByText("Console request approved")).toBeVisible();
    expect(screen.getByText("console-ticket-1")).toBeVisible();
    expect(screen.getByText("ticket-1")).toBeVisible();

    fireEvent.click(screen.getByText("Payments"));
    expect(pushMock).toHaveBeenCalledWith("/systems?detail_system_id=sys-1");

    fireEvent.click(screen.getByText("Billing API"));
    expect(pushMock).toHaveBeenCalledWith(
      "/services?system_id=sys-1&detail_service_id=svc-1",
    );

    fireEvent.click(screen.getByTestId("vm-action-request-similar-vm-1"));
    expect(pushMock).toHaveBeenCalledWith(
      "/vms?request=create&source_vm_id=vm-1",
    );
  });

  it("asks for the console mode before attaching the preferred serial console in page", async () => {
    useApiGetMock.mockImplementation((queryKey: unknown[]) => {
      if (queryKey[0] === "vm-console-status") {
        return {
          data: {
            status: "NOT_REQUESTED",
          },
          isLoading: false,
          refetch: refetchConsoleStatusMock,
        };
      }
      if (queryKey[0] === "vm-manifest") {
        return {
          data: undefined,
          isLoading: false,
          isFetching: false,
          refetch: vi.fn(),
        };
      }
      return {
        data: {
          id: "vm-1",
          name: "vm-alpha",
          status: "RUNNING",
          namespace: "team-prod",
        },
        isLoading: false,
        refetch: refetchMock,
      };
    });
    useApiMutationMock.mockReturnValue({
      isPending: false,
      mutate: vi.fn(),
    });
    vi.mocked(api.POST).mockResolvedValue({
      data: {
        status: "APPROVED",
        console_type: "SERIAL",
        console_url: "/api/v1/vms/vm-1/serial",
      },
      error: undefined,
      response: new Response(null, { status: 200 }),
    } as never);

    render(<VMDetailPage />);

    fireEvent.click(screen.getByTestId("vm-action-console-vm-1"));

    expect(screen.getByText("Choose Console Mode")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Confirm" }));

    await waitFor(() => {
      expect(screen.getByTestId("vm-console-panel")).toHaveTextContent(
        "SERIAL:/api/v1/vms/vm-1/serial",
      );
    });
  });

  it("allows stop while the VM is still starting", () => {
    useApiGetMock.mockImplementation((queryKey: unknown[]) => {
      if (queryKey[0] === "vm-console-status") {
        return {
          data: undefined,
          isLoading: false,
          isFetching: false,
          refetch: refetchConsoleStatusMock,
        };
      }
      if (queryKey[0] === "vm-manifest") {
        return {
          data: undefined,
          isLoading: false,
          isFetching: false,
          refetch: vi.fn(),
        };
      }
      return {
        data: {
          id: "vm-1",
          name: "vm-alpha",
          status: "STARTING",
          namespace: "team-prod",
        },
        isLoading: false,
        refetch: refetchMock,
      };
    });
    useApiMutationMock.mockReturnValue({
      isPending: false,
      mutate: vi.fn(),
    });

    render(<VMDetailPage />);

    expect(screen.getByTestId("vm-action-stop-vm-1")).toBeEnabled();
    expect(screen.getByTestId("vm-action-start-vm-1")).toBeDisabled();
  });

  it("shows the YAML manifest viewer for platform admins", () => {
    useApiGetMock.mockImplementation((queryKey: unknown[]) => {
      if (queryKey[0] === "vm-console-status") {
        return {
          data: undefined,
          isLoading: false,
          isFetching: false,
          refetch: refetchConsoleStatusMock,
        };
      }
      if (queryKey[0] === "vm-manifest") {
        return {
          data: {
            vm_id: "vm-1",
            name: "vm-alpha",
            namespace: "team-prod",
            cluster_id: "cluster-1",
            yaml: "kind: VirtualMachine\nmetadata:\n  name: vm-alpha\n",
          },
          isLoading: false,
          isFetching: false,
          refetch: vi.fn(),
        };
      }
      return {
        data: {
          id: "vm-1",
          name: "vm-alpha",
          status: "RUNNING",
          namespace: "team-prod",
        },
        isLoading: false,
        refetch: refetchMock,
      };
    });
    useApiMutationMock.mockReturnValue({
      isPending: false,
      mutate: vi.fn(),
    });

    render(<VMDetailPage />);

    fireEvent.click(screen.getByTestId("vm-action-manifest-vm-1"));

    expect(screen.getByTestId("vm-manifest-content")).toHaveValue(
      "kind: VirtualMachine\nmetadata:\n  name: vm-alpha\n",
    );
  });

  it("hides the YAML manifest viewer for non-platform-admin users", () => {
    authState.user.permissions = ["vm:read"];
    useApiGetMock.mockImplementation((queryKey: unknown[]) => {
      if (queryKey[0] === "vm-console-status") {
        return {
          data: undefined,
          isLoading: false,
          isFetching: false,
          refetch: refetchConsoleStatusMock,
        };
      }
      if (queryKey[0] === "vm-manifest") {
        return {
          data: undefined,
          isLoading: false,
          isFetching: false,
          refetch: vi.fn(),
        };
      }
      return {
        data: {
          id: "vm-1",
          name: "vm-alpha",
          status: "RUNNING",
          namespace: "team-prod",
        },
        isLoading: false,
        refetch: refetchMock,
      };
    });
    useApiMutationMock.mockReturnValue({
      isPending: false,
      mutate: vi.fn(),
    });

    render(<VMDetailPage />);

    expect(screen.queryByTestId("vm-action-manifest-vm-1")).not.toBeInTheDocument();
  });
});
