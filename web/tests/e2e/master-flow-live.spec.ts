/**
 * Master Flow Live E2E Tests — Contract-Enforced (no mock)
 *
 * ┌─────────────────────────────────────────────────────────────────────────┐
 * │  REQUIRES: a running backend (db + server via docker-compose or local)  │
 * │  Every API response is validated against api/openapi.yaml schema.       │
 * │  Schema mismatch = CI failure = frontend/backend contract broken.       │
 * │  NO test.skip() — failures expose real frontend/backend problems.       │
 * └─────────────────────────────────────────────────────────────────────────┘
 *
 * Coverage map (master-flow.md) — operationId index:
 *   getCurrentUser         Stage 2.D  – GET /auth/me
 *   listSystems            Stage 4.A  – GET /systems
 *   createSystem           Stage 4.A  – POST /systems
 *   listSystemMembers      Stage 4.A+ – GET /systems/{id}/members
 *   createService          Stage 4.B  – POST /systems/{id}/services
 *   getService             Stage 4.C  – GET /systems/{id}/services/{id}
 *   updateService          Stage 4.B  – PATCH /systems/{id}/services/{id}
 *   deleteService          Stage 4.B  – DELETE /systems/{id}/services/{id}
 *   listVMs                Stage 5    – GET /vms
 *   getVMRequestContext    Stage 5.A  – GET /vms/request-context
 *   createVMRequest        Stage 5.A  – POST /vms/request
 *   listApprovals          Stage 5.B  – GET /builtin-approval/tasks
 *   approveTicket          Stage 5.B  – POST /builtin-approval/tasks/{id}/approve
 *   rejectTicket           Stage 5.B  – POST /builtin-approval/tasks/{id}/reject
 *   submitVMBatchPower     Stage 5.E  – POST /vms/batch/power
 *   listNotifications      Stage 5.F  – GET /notifications
 *   getUnreadCount         Stage 5.F  – GET /notifications/unread-count
 *   requestVMConsoleAccess Stage 6    – POST /vms/{id}/console/request
 *   openVMVNC              Stage 6    – GET /vms/{id}/vnc
 *   listAdminTemplates     Stage 3    – GET /admin/templates
 *   listAdminInstanceSizes Stage 3    – GET /admin/instance-sizes
 *   listNamespaces         Stage 3    – GET /admin/namespaces
 *   listRoles              Stage 2.A  – GET /admin/roles
 *   listAuthProviderTypes  Stage 2.B  – GET /admin/auth-provider-types
 *   listAuthProviders      Stage 2.B  – GET /admin/auth-providers
 *   listUsers              Stage 2 (Supplemental) – GET /admin/users
 *
 * Environment variables:
 *   E2E_USERNAME        – admin username (default: e2e-admin)
 *   E2E_PASSWORD        – admin password (default: e2e-admin-123)
 *   E2E_NEW_PASSWORD    – password used when force_password_change=true
 *   E2E_SYSTEM          – pre-existing system name for cascade tests
 *   E2E_SERVICE         – pre-existing service name (has child VMs)
 *   E2E_VM_RUNNING_ID   – ID of a running VM for console test
 *
 * Run:
 *   PW_BASE_URL=http://localhost:3000 npx playwright test master-flow-live
 */

import {
  expect,
  test,
  type APIRequestContext,
  type Page,
} from "@playwright/test";
import { validateApiResponse } from "./lib/schema-validator";
import {
  ensureSeedSystemAndService,
  expectSchemaResponse as expectSchema,
  fetchStatusWithStoredToken,
  getAntModal,
  getApiAuthHeadersWithForcePasswordSupport,
  loginWithForcePasswordSupport,
  pickIDByPreferredName,
  pickPreferredNamespace,
  resolveClusterOptionFilter,
  selectApprovalRootVolumeModeIfRequired,
  selectAntOption,
  selectServicesSystemFilter,
  toLooseOptionFilter,
  urlPathEndsWith,
  urlPathIncludes,
} from "./lib/helpers";

// ── Config ────────────────────────────────────────────────────────────────────

const e2eUsername = process.env.E2E_USERNAME ?? "e2e-admin";
const e2ePassword = process.env.E2E_PASSWORD ?? "e2e-admin-123";
const e2eNewPassword =
  process.env.E2E_NEW_PASSWORD ??
  (e2ePassword === "admin" ? "ShepherdLive!2026" : `${e2ePassword}-new`);
const e2eClusterName = process.env.E2E_CLUSTER ?? "e2e-cluster";
const e2eSystemName = process.env.E2E_SYSTEM ?? "e2e-system";
const e2eServiceName = process.env.E2E_SERVICE ?? "e2e-service";
const e2eTemplateName = process.env.E2E_TEMPLATE ?? "e2e-template";
const e2eSizeName = process.env.E2E_SIZE ?? "e2e-small";
const e2eNamespace = process.env.E2E_NAMESPACE ?? "e2e-test";
const e2eTemplatePVCStorageClass =
  process.env.E2E_TEMPLATE_PVC_STORAGE_CLASS?.trim() ?? "";
const e2eTemplatePVCAccessMode =
  process.env.E2E_TEMPLATE_PVC_ACCESS_MODE?.trim() ?? "";
const e2eTemplatePVCVolumeMode =
  process.env.E2E_TEMPLATE_PVC_VOLUME_MODE?.trim() ?? "";
const runningVMID = process.env.E2E_VM_RUNNING_ID ?? "";
let activePassword = e2ePassword;

// ── Auth helper ───────────────────────────────────────────────────────────────

async function login(page: Page): Promise<void> {
  activePassword = await loginWithForcePasswordSupport(page, {
    username: e2eUsername,
    primaryPassword: e2ePassword,
    secondaryPassword: e2eNewPassword,
    currentPasswordHint: activePassword,
  });
}

// ── Helpers ───────────────────────────────────────────────────────────────────

async function getAdminToken(request: APIRequestContext): Promise<string> {
  const auth = await getApiAuthHeadersWithForcePasswordSupport(request, {
    username: e2eUsername,
    primaryPassword: e2ePassword,
    secondaryPassword: e2eNewPassword,
    currentPasswordHint: activePassword,
  });
  activePassword = auth.password;
  return auth.token;
}

async function resolveHealthyClusterID(
  request: APIRequestContext,
  headers: { Authorization: string },
): Promise<string> {
  const clustersResp = await request.get(
    "/api/v1/admin/clusters?page=1&per_page=100",
    { headers },
  );
  expect(
    clustersResp.status(),
    `GET /admin/clusters returned ${clustersResp.status()}`,
  ).toBe(200);
  const clustersBody = (await validateApiResponse(
    "ClusterList",
    clustersResp,
  )) as {
    items?: Array<{ id?: string; status?: string; enabled?: boolean }>;
  };
  const clusters = clustersBody.items ?? [];
  const healthyEnabled = clusters.find(
    (item) =>
      item.id &&
      item.enabled !== false &&
      (item.status ?? "").toUpperCase() === "HEALTHY",
  );
  if (healthyEnabled?.id) {
    return healthyEnabled.id;
  }
  return clusters.find((item) => Boolean(item.id))?.id ?? "";
}

async function resolveCreateVMRequestData(
  request: APIRequestContext,
  headers: { Authorization: string },
  scope: string,
  preferredServiceName = e2eServiceName,
): Promise<{
  service_id: string;
  template_id: string;
  instance_size_id: string;
  namespace: string;
}> {
  const systemsResp = await request.get("/api/v1/systems", { headers });
  expect(
    systemsResp.status(),
    `GET /systems must return 200 for ${scope}`,
  ).toBe(200);
  const systems = (await validateApiResponse("SystemList", systemsResp)) as {
    items?: Array<{ id?: string; name?: string }>;
  };
  const systemId = pickIDByPreferredName(systems.items, e2eSystemName);
  expect(
    systemId,
    `${scope} requires at least one existing system`,
  ).toBeTruthy();

  const [servicesResp, contextResp] = await Promise.all([
    request.get(`/api/v1/systems/${systemId}/services`, { headers }),
    request.get("/api/v1/vms/request-context", { headers }),
  ]);
  expect(
    servicesResp.status(),
    `GET /systems/{id}/services must return 200 for ${scope}`,
  ).toBe(200);
  expect(
    contextResp.status(),
    `GET /vms/request-context must return 200 for ${scope}`,
  ).toBe(200);

  const services = (await validateApiResponse("ServiceList", servicesResp)) as {
    items?: Array<{ id?: string; name?: string }>;
  };
  const ctx = (await validateApiResponse("VMRequestContext", contextResp)) as {
    templates?: Array<{ id?: string; name?: string }>;
    instance_sizes?: Array<{ id?: string; name?: string }>;
    namespaces?: string[];
  };

  const serviceId = pickIDByPreferredName(services.items, preferredServiceName);
  const templateId = pickIDByPreferredName(ctx.templates, e2eTemplateName);
  const sizeId = pickIDByPreferredName(ctx.instance_sizes, e2eSizeName);
  const namespace = pickPreferredNamespace(ctx.namespaces, e2eNamespace);
  expect(serviceId, `${scope} requires an existing service`).toBeTruthy();
  expect(templateId, `${scope} requires an existing template`).toBeTruthy();
  expect(sizeId, `${scope} requires an existing instance size`).toBeTruthy();

  return {
    service_id: serviceId,
    template_id: templateId,
    instance_size_id: sizeId,
    namespace,
  };
}

async function createUniqueServiceForStage5A(
  request: APIRequestContext,
): Promise<string> {
  const token = await getAdminToken(request);
  const headers = { Authorization: `Bearer ${token}` };

  const systemsResp = await request.get("/api/v1/systems", { headers });
  expect(
    systemsResp.status(),
    "GET /systems must return 200 for Stage 5.A setup",
  ).toBe(200);
  const systems = (await validateApiResponse("SystemList", systemsResp)) as {
    items?: Array<{ id?: string; name?: string }>;
  };
  const systemId = pickIDByPreferredName(systems.items, e2eSystemName);
  expect(
    systemId,
    "Stage 5.A setup requires at least one existing system",
  ).toBeTruthy();

  // OpenAPI ServiceCreateRequest.name: maxLength=15, pattern ^[a-z]([a-z0-9-]*[a-z0-9])?$
  const serviceName = `s5a-${Date.now().toString(36).slice(-8)}`;
  const createServiceResp = await request.post(
    `/api/v1/systems/${systemId}/services`,
    {
      headers,
      data: {
        name: serviceName,
        description:
          "temporary service for Stage 5.A createVMRequest isolation",
      },
    },
  );
  expect(
    createServiceResp.status(),
    `POST /systems/{id}/services returned ${createServiceResp.status()} for Stage 5.A setup`,
  ).toBe(201);
  await validateApiResponse("Service", createServiceResp);

  return serviceName;
}

async function createUniqueServiceForApprovalSeed(
  request: APIRequestContext,
): Promise<string> {
  const token = await getAdminToken(request);
  const headers = { Authorization: `Bearer ${token}` };

  const systemsResp = await request.get("/api/v1/systems", { headers });
  expect(
    systemsResp.status(),
    "GET /systems must return 200 for Stage 5.B setup",
  ).toBe(200);
  const systems = (await validateApiResponse("SystemList", systemsResp)) as {
    items?: Array<{ id?: string; name?: string }>;
  };
  const systemId = pickIDByPreferredName(systems.items, e2eSystemName);
  expect(
    systemId,
    "Stage 5.B setup requires at least one existing system",
  ).toBeTruthy();

  const serviceName = `s5b-${Date.now().toString(36).slice(-8)}`;
  const createServiceResp = await request.post(
    `/api/v1/systems/${systemId}/services`,
    {
      headers,
      data: {
        name: serviceName,
        description: "temporary service for Stage 5.B approval seed isolation",
      },
    },
  );
  expect(
    createServiceResp.status(),
    `POST /systems/{id}/services returned ${createServiceResp.status()} for Stage 5.B setup`,
  ).toBe(201);
  await validateApiResponse("Service", createServiceResp);
  return serviceName;
}

async function listVMBriefs(
  request: APIRequestContext,
  headers: { Authorization: string },
): Promise<Array<{ id: string; status: string }>> {
  const vmMap = new Map<string, { id: string; status: string }>();
  let page = 1;
  let totalPages = 1;
  do {
    const vmResp = await request.get(`/api/v1/vms?page=${page}&per_page=100`, {
      headers,
    });
    expect(
      vmResp.status(),
      `GET /vms?page=${page} returned ${vmResp.status()}`,
    ).toBe(200);
    const vmBody = (await validateApiResponse("VMList", vmResp)) as {
      items?: Array<{ id?: string; status?: string }>;
      pagination?: { total_pages?: number };
    };
    for (const item of vmBody.items ?? []) {
      if (!item.id) {
        continue;
      }
      vmMap.set(item.id, {
        id: item.id,
        status: (item.status ?? "").toUpperCase(),
      });
    }
    totalPages = Number(vmBody.pagination?.total_pages ?? 1) || 1;
    page += 1;
  } while (page <= totalPages);
  return Array.from(vmMap.values());
}

async function waitForTicketStatus(
  request: APIRequestContext,
  headers: { Authorization: string },
  ticketID: string,
  expected: "PENDING" | "APPROVED" | "REJECTED",
): Promise<void> {
  let expectedPattern: RegExp;
  switch (expected) {
    case "PENDING":
      expectedPattern = /^PENDING$/;
      break;
    case "APPROVED":
      expectedPattern = /^(APPROVED|EXECUTING|SUCCESS)$/;
      break;
    case "REJECTED":
      expectedPattern = /^REJECTED$/;
      break;
    default:
      expectedPattern = /^$/;
  }

  await expect
    .poll(
      async () => {
        let page = 1;
        const perPage = 100;

        for (let guard = 0; guard < 20; guard += 1) {
          const listResp = await request.get(
            `/api/v1/builtin-approval/tasks?page=${page}&per_page=${perPage}`,
            { headers },
          );
          expect(
            listResp.status(),
            `GET /builtin-approval/tasks returned ${listResp.status()}`,
          ).toBe(200);
          const listBody = (await validateApiResponse(
            "TicketList",
            listResp,
          )) as {
            items?: Array<{ id?: string; status?: string }>;
            pagination?: { total_pages?: number };
          };
          const found = listBody.items?.find((item) => item.id === ticketID);
          if (found) {
            return (found.status ?? "").toUpperCase();
          }
          const totalPages = Number(listBody.pagination?.total_pages ?? 1);
          if (!Number.isFinite(totalPages) || page >= totalPages) {
            break;
          }
          page += 1;
        }

        return "";
      },
      {
        timeout: 30_000,
        intervals: [500, 1000, 2000],
        message: `Ticket ${ticketID} did not match status pattern ${expectedPattern}`,
      },
    )
    .toMatch(expectedPattern);
}

async function findApprovalTicket(
  request: APIRequestContext,
  headers: { Authorization: string },
  ticketID: string,
): Promise<{
  status: string;
  ticket_payload?: Record<string, unknown>;
} | null> {
  let page = 1;
  const perPage = 100;
  for (let guard = 0; guard < 20; guard += 1) {
    const listResp = await request.get(
      `/api/v1/builtin-approval/tasks?page=${page}&per_page=${perPage}`,
      { headers },
    );
    if (listResp.status() !== 200) {
      return null;
    }
    const listBody = (await validateApiResponse("TicketList", listResp)) as {
      items?: Array<{
        id?: string;
        status?: string;
        ticket_payload?: Record<string, unknown>;
      }>;
      pagination?: { total_pages?: number };
    };
    const found = (listBody.items ?? []).find((item) => item.id === ticketID);
    if (found) {
      return {
        status: String(found.status ?? "").toUpperCase(),
        ticket_payload: found.ticket_payload,
      };
    }
    const totalPages = Number(listBody.pagination?.total_pages ?? 1);
    if (!Number.isFinite(totalPages) || page >= totalPages) {
      break;
    }
    page += 1;
  }
  return null;
}

function extractNamespaceFromTicketPayload(
  payload: Record<string, unknown> | undefined,
): string {
  if (!payload) {
    return "";
  }
  const namespace = payload.namespace;
  return typeof namespace === "string" ? namespace.trim() : "";
}

async function waitForApprovalNotification(
  request: APIRequestContext,
  headers: { Authorization: string },
  ticketID: string,
  expectedType: "APPROVAL_COMPLETED" | "APPROVAL_REJECTED",
): Promise<void> {
  await expect
    .poll(
      async () => {
        let page = 1;
        const perPage = 100;

        for (let guard = 0; guard < 20; guard += 1) {
          const listResp = await request.get(
            `/api/v1/notifications?page=${page}&per_page=${perPage}`,
            { headers },
          );
          expect(
            listResp.status(),
            `GET /notifications returned ${listResp.status()}`,
          ).toBe(200);
          const listBody = (await validateApiResponse(
            "NotificationList",
            listResp,
          )) as {
            items?: Array<{
              type?: string;
              resource_id?: string;
              resourceId?: string;
            }>;
            pagination?: { total_pages?: number };
          };

          const found = (listBody.items ?? []).some((item) => {
            const type = (item.type ?? "").toUpperCase();
            const resourceID = String(
              item.resource_id ?? item.resourceId ?? "",
            ).trim();
            return type === expectedType && resourceID === ticketID;
          });
          if (found) {
            return true;
          }

          const totalPages = Number(listBody.pagination?.total_pages ?? 1);
          if (!Number.isFinite(totalPages) || page >= totalPages) {
            break;
          }
          page += 1;
        }

        return false;
      },
      {
        timeout: 30_000,
        intervals: [1000, 2000, 3000],
        message: `Missing notification ${expectedType} for ticket ${ticketID}`,
      },
    )
    .toBe(true);
}

async function waitForNewVMFromApproval(
  request: APIRequestContext,
  headers: { Authorization: string },
  baselineIDs: Set<string>,
): Promise<string> {
  let createdVMID = "";
  await expect
    .poll(
      async () => {
        const vms = await listVMBriefs(request, headers);
        const created = vms.find((vm) => !baselineIDs.has(vm.id));
        createdVMID = created?.id ?? "";
        return createdVMID;
      },
      {
        timeout: 60_000,
        intervals: [1000, 2000, 3000],
        message: "Stage 5.C requires approved ticket to materialize a VM row",
      },
    )
    .not.toBe("");
  return createdVMID;
}

function buildCreateApprovalData(clusterID: string): Record<string, unknown> {
  const data: Record<string, unknown> = { selected_cluster_id: clusterID };
  if (e2eTemplatePVCStorageClass) {
    data.selected_storage_class = e2eTemplatePVCStorageClass;
  }
  if (e2eTemplatePVCAccessMode) {
    data.selected_dv_access_modes = [e2eTemplatePVCAccessMode];
  }
  if (e2eTemplatePVCVolumeMode) {
    data.selected_dv_volume_mode = e2eTemplatePVCVolumeMode;
  }
  return data;
}

async function waitForVMRunning(
  request: APIRequestContext,
  headers: { Authorization: string },
  vmID: string,
): Promise<void> {
  await expect
    .poll(
      async () => {
        const vmResp = await request.get(`/api/v1/vms/${vmID}`, { headers });
        if (vmResp.status() !== 200) {
          return `HTTP_${vmResp.status()}`;
        }
        const vmBody = (await validateApiResponse("VM", vmResp)) as {
          status?: string;
        };
        const status = (vmBody.status ?? "").toUpperCase();
        if (status === "CREATING") {
          const health = await summarizeClusterHealth(request, headers);
          return `CREATING|${health}`;
        }
        return status;
      },
      {
        timeout: 180_000,
        intervals: [1000, 2000, 4000, 8000],
        message: `VM ${vmID} did not reach RUNNING for console test`,
      },
    )
    .toBe("RUNNING");
}

async function createRunningVMForConsole(
  request: APIRequestContext,
  headers: { Authorization: string },
): Promise<string> {
  const baselineIDs = new Set((await listVMBriefs(request, headers)).map((vm) => vm.id));
  const serviceName = await createUniqueServiceForApprovalSeed(request);
  const createReqData = await resolveCreateVMRequestData(
    request,
    headers,
    "Stage 6 console VM setup",
    serviceName,
  );
  const createResp = await request.post("/api/v1/vms/request", {
    headers,
    data: {
      ...createReqData,
      reason: `Stage 6 console VM setup ${Date.now()}`,
    },
  });
  expect(
    createResp.status(),
    `POST /vms/request returned ${createResp.status()} for console setup`,
  ).toBe(202);
  const createBody = (await validateApiResponse(
    "TicketResponse",
    createResp,
  )) as { ticket_id?: string; id?: string };
  const ticketID = String(createBody.ticket_id ?? createBody.id ?? "").trim();
  expect(ticketID, "console setup requires a create ticket id").toBeTruthy();

  const clusterID = await resolveHealthyClusterID(request, headers);
  expect(clusterID, "console setup requires at least one cluster").toBeTruthy();
  const approveResp = await request.post(
    `/api/v1/builtin-approval/tasks/${ticketID}/approve`,
    {
      headers,
      data: buildCreateApprovalData(clusterID),
    },
  );
  expect(
    approveResp.status(),
    `POST /builtin-approval/tasks/{id}/approve returned ${approveResp.status()} for console setup`,
  ).toBe(204);

  const vmID = await waitForNewVMFromApproval(request, headers, baselineIDs);
  await waitForVMRunning(request, headers, vmID);
  return vmID;
}

async function summarizeClusterHealth(
  request: APIRequestContext,
  headers: { Authorization: string },
): Promise<string> {
  const clustersResp = await request.get(
    "/api/v1/admin/clusters?page=1&per_page=100",
    { headers },
  );
  if (clustersResp.status() !== 200) {
    return `clusters_http_${clustersResp.status()}`;
  }
  const clustersBody = (await validateApiResponse(
    "ClusterList",
    clustersResp,
  )) as {
    items?: Array<{
      id?: string;
      name?: string;
      status?: string;
      enabled?: boolean;
    }>;
  };
  const summary = (clustersBody.items ?? [])
    .map((cluster) => {
      const name = cluster.name ?? cluster.id ?? "unknown";
      const status = (cluster.status ?? "UNKNOWN").toUpperCase();
      const enabledSuffix = cluster.enabled === false ? ":DISABLED" : "";
      return `${name}:${status}${enabledSuffix}`;
    })
    .join(", ");
  return summary || "no-clusters";
}

async function waitForVMExecutionOutcome(
  request: APIRequestContext,
  headers: { Authorization: string },
  vmID: string,
): Promise<void> {
  await expect
    .poll(
      async () => {
        const vmResp = await request.get(`/api/v1/vms/${vmID}`, { headers });
        if (vmResp.status() !== 200) {
          return `HTTP_${vmResp.status()}`;
        }
        const vmBody = (await validateApiResponse("VM", vmResp)) as {
          status?: string;
        };
        const status = (vmBody.status ?? "").toUpperCase();
        if (status === "CREATING") {
          const health = await summarizeClusterHealth(request, headers);
          return `CREATING|${health}`;
        }
        return status;
      },
      {
        timeout: 120_000,
        intervals: [1000, 2000, 4000, 8000],
        message: `VM ${vmID} did not reach RUNNING/FAILED from CREATING`,
      },
    )
    .toMatch(/^(RUNNING|FAILED)$/);
}

interface ApprovalSeedResult {
  ticketID: string;
  namespace: string;
}

async function seedPendingApprovalTicket(
  request: APIRequestContext,
): Promise<ApprovalSeedResult> {
  const token = await getAdminToken(request);
  const headers = { Authorization: `Bearer ${token}` };
  const approvalSeedServiceName =
    await createUniqueServiceForApprovalSeed(request);
  const createReqData = await resolveCreateVMRequestData(
    request,
    headers,
    "approval seed",
    approvalSeedServiceName,
  );

  const createResp = await request.post("/api/v1/vms/request", {
    headers,
    data: {
      ...createReqData,
      reason: `master-flow approval seed ${Date.now()}`,
    },
  });
  if (createResp.status() === 400) {
    const errBody = (await validateApiResponse("Error", createResp)) as {
      code?: string;
      message?: string;
      params?: Record<string, unknown>;
    };
    if (errBody.code === "DUPLICATE_PENDING_REQUEST") {
      const existingTicketID =
        typeof errBody.params?.existing_ticket_id === "string"
          ? errBody.params.existing_ticket_id.trim()
          : "";
      throw new Error(
        `unexpected DUPLICATE_PENDING_REQUEST in approval seed (service=${approvalSeedServiceName}, existing_ticket_id=${existingTicketID || "unknown"})`,
      );
    }
    throw new Error(
      `POST /vms/request failed in approval seed: ${errBody.code ?? "UNKNOWN"} (${errBody.message ?? "no message"})`,
    );
  }

  expect(
    createResp.status(),
    "POST /vms/request must return 202 for approval seed",
  ).toBe(202);
  const ticket = (await validateApiResponse("TicketResponse", createResp)) as {
    ticket_id?: string;
    id?: string;
  };
  const ticketID = ticket.ticket_id ?? ticket.id ?? "";
  expect(ticketID, "TicketResponse missing ticket id").toBeTruthy();
  return {
    ticketID,
    namespace: createReqData.namespace,
  };
}

// ── Test suite ────────────────────────────────────────────────────────────────

test.describe("master-flow live (contract-enforced, no mock)", () => {
  test.beforeAll(async ({ request }) => {
    // Ensure seed system + service exist (idempotent, API-first).
    const seed = await ensureSeedSystemAndService(request, {
      username: e2eUsername,
      primaryPassword: e2ePassword,
      secondaryPassword: e2eNewPassword,
      currentPasswordHint: activePassword,
      systemName: e2eSystemName,
      serviceName: e2eServiceName,
    });
    activePassword = seed.password;
  });

  test.beforeEach(async ({ page }) => {
    await login(page);
  });

  // ── Stage 1: Platform Initialization ────────────────────────────────────────

  test("Stage 1 – getDynamicSchema: schema+mask contract with degradation metadata", async ({
    request,
  }) => {
    await test.step("Stage 1 / Step 1: public schema endpoint returns DynamicSchemaResponse without auth", async () => {
      const schemaResp = await request.get("/api/v1/schemas/instancesize");
      expect(
        schemaResp.status(),
        `GET /schemas/instancesize returned ${schemaResp.status()}`,
      ).toBe(200);
      const body = (await validateApiResponse(
        "DynamicSchemaResponse",
        schemaResp,
      )) as {
        schema?: Record<string, unknown>;
        mask?: { quick_fields?: unknown[]; advanced_fields?: unknown[] };
        source?: string;
        degraded?: boolean;
        schema_version?: string;
      };
      expect(body.schema, "dynamic schema payload is required").toBeTruthy();
      expect(typeof body.schema, "dynamic schema must be object-like").toBe(
        "object",
      );
      expect(body.mask, "dynamic schema mask is required").toBeTruthy();
      expect(
        Array.isArray(body.mask?.quick_fields),
        "mask.quick_fields must be an array",
      ).toBe(true);
      expect(
        Array.isArray(body.mask?.advanced_fields),
        "mask.advanced_fields must be an array",
      ).toBe(true);
    });

    await test.step("Stage 1 / Step 2: schema source/degraded metadata matches ADR-0023 cache contract", async () => {
      const schemaResp = await request.get("/api/v1/schemas/instancesize");
      expect(
        schemaResp.status(),
        `GET /schemas/instancesize returned ${schemaResp.status()}`,
      ).toBe(200);
      const body = (await validateApiResponse(
        "DynamicSchemaResponse",
        schemaResp,
      )) as {
        source?: string;
        degraded?: boolean;
        schema_version?: string;
      };
      if (typeof body.source !== "undefined") {
        expect(
          ["cache", "embedded", "remote"],
          "dynamic schema source must expose cache-degradation provenance when provided",
        ).toContain(String(body.source).toLowerCase());
      }
      if (typeof body.degraded !== "undefined") {
        expect(
          typeof body.degraded,
          "dynamic schema degraded flag must be boolean when provided",
        ).toBe("boolean");
      }
      if (typeof body.schema_version !== "undefined") {
        expect(
          String(body.schema_version).trim(),
          "dynamic schema version should be non-empty when provided",
        ).toBeTruthy();
      }
    });
  });

  // ── Stage 1.5: First Deployment Bootstrap ───────────────────────────────────

  test("Stage 1.5 – forced password rotation on first login is enforced end-to-end", async ({
    request,
  }) => {
    const adminToken = await getAdminToken(request);
    const adminHeaders = { Authorization: `Bearer ${adminToken}` };
    const suffix = Date.now().toString(36).slice(-8);
    const username = `s15-${suffix}`;
    const initialPassword = `Init-${suffix}-a1`;
    const rotatedPassword = `Rot-${suffix}-b2`;
    let userID = "";

    try {
      await test.step("Stage 1.5 / Step 1: bootstrap-created user can be flagged force_password_change=true", async () => {
        const createResp = await request.post("/api/v1/admin/users", {
          headers: adminHeaders,
          data: {
            username,
            password: initialPassword,
            enabled: true,
            force_password_change: true,
          },
        });
        expect(
          createResp.status(),
          `POST /admin/users returned ${createResp.status()}`,
        ).toBe(201);
        const createBody = (await validateApiResponse("User", createResp)) as {
          id?: string;
          username?: string;
        };
        userID = String(createBody.id ?? "").trim();
        expect(userID, "created user id is required for cleanup").toBeTruthy();
        expect(String(createBody.username ?? "").trim()).toBe(username);
      });

      let loginToken = "";
      await test.step("Stage 1.5 / Step 2: first login returns force_password_change=true", async () => {
        const loginResp = await request.post("/api/v1/auth/login", {
          data: { username, password: initialPassword },
        });
        expect(
          loginResp.status(),
          `POST /auth/login returned ${loginResp.status()}`,
        ).toBe(200);
        const loginBody = (await validateApiResponse(
          "LoginResponse",
          loginResp,
        )) as {
          token?: string;
          force_password_change?: boolean;
        };
        loginToken = String(loginBody.token ?? "").trim();
        expect(loginToken, "initial login token is required").toBeTruthy();
        expect(
          loginBody.force_password_change,
          "first login must enforce password rotation",
        ).toBe(true);
      });

      await test.step("Stage 1.5 / Step 3: password change clears force_password_change on next login", async () => {
        const changeResp = await request.post("/api/v1/auth/change-password", {
          headers: { Authorization: `Bearer ${loginToken}` },
          data: {
            old_password: initialPassword,
            new_password: rotatedPassword,
          },
        });
        expect(
          changeResp.status(),
          `POST /auth/change-password returned ${changeResp.status()}`,
        ).toBe(204);

        const reloginResp = await request.post("/api/v1/auth/login", {
          data: { username, password: rotatedPassword },
        });
        expect(
          reloginResp.status(),
          `POST /auth/login after password change returned ${reloginResp.status()}`,
        ).toBe(200);
        const reloginBody = (await validateApiResponse(
          "LoginResponse",
          reloginResp,
        )) as {
          token?: string;
          force_password_change?: boolean;
        };
        const reloginToken = String(reloginBody.token ?? "").trim();
        expect(
          reloginToken,
          "relogin token is required after password change",
        ).toBeTruthy();
        expect(
          reloginBody.force_password_change ?? false,
          "force_password_change should be false (or omitted) after rotation",
        ).toBe(false);

        const meResp = await request.get("/api/v1/auth/me", {
          headers: { Authorization: `Bearer ${reloginToken}` },
        });
        expect(
          meResp.status(),
          `GET /auth/me returned ${meResp.status()}`,
        ).toBe(200);
        await validateApiResponse("UserInfo", meResp);
      });
    } finally {
      if (!userID) {
        return;
      }
      const cleanupToken = await getAdminToken(request);
      const deleteResp = await request.delete(`/api/v1/admin/users/${userID}`, {
        headers: { Authorization: `Bearer ${cleanupToken}` },
      });
      expect(
        [204, 404],
        `cleanup delete user returned ${deleteResp.status()}`,
      ).toContain(deleteResp.status());
    }
  });

  // ── Stage 2: Security Configuration (Top-level) ─────────────────────────────

  test("Stage 2 – security baseline: login principal and admin security surfaces are coherent", async ({
    request,
  }) => {
    const token = await getAdminToken(request);
    const headers = { Authorization: `Bearer ${token}` };

    await test.step("Stage 2 / Step 1: authenticated principal contract is valid via /auth/me", async () => {
      const meResp = await request.get("/api/v1/auth/me", { headers });
      expect(meResp.status(), `GET /auth/me returned ${meResp.status()}`).toBe(
        200,
      );
      const meBody = (await validateApiResponse("UserInfo", meResp)) as {
        id?: string;
        username?: string;
        roles?: string[];
      };
      expect(
        String(meBody.id ?? "").trim(),
        "authenticated user id is required",
      ).toBeTruthy();
      expect(
        String(meBody.username ?? "").trim(),
        "authenticated username is required",
      ).toBeTruthy();
      expect(
        Array.isArray(meBody.roles),
        "authenticated roles must be an array",
      ).toBe(true);
    });

    await test.step("Stage 2 / Step 2: role and provider security catalogs are reachable after auth", async () => {
      const [rolesResp, providerTypesResp] = await Promise.all([
        request.get("/api/v1/admin/roles", { headers }),
        request.get("/api/v1/admin/auth-provider-types", { headers }),
      ]);
      expect(
        rolesResp.status(),
        `GET /admin/roles returned ${rolesResp.status()}`,
      ).toBe(200);
      expect(
        providerTypesResp.status(),
        `GET /admin/auth-provider-types returned ${providerTypesResp.status()}`,
      ).toBe(200);
      await validateApiResponse("RoleList", rolesResp);
      await validateApiResponse("AuthProviderTypeList", providerTypesResp);
    });
  });

  // ── Stage 2.D: Auth ─────────────────────────────────────────────────────────

  test("Stage 2.D – getCurrentUser: /auth/me returns UserInfo schema", async ({
    page,
  }) => {
    // operationId: login, getCurrentUser
    // login() in beforeEach already validates LoginResponse schema.
    // This test additionally verifies /auth/me returns UserInfo schema.
    // Use explicit fetch in page context to guarantee the endpoint is exercised.
    await test.step("Stage 2.D / Step 1: verify /auth/me after dashboard navigation", async () => {
      const meRespPromise = page.waitForResponse(
        (r) =>
          urlPathEndsWith(r.url(), "/api/v1/auth/me") &&
          r.request().method() === "GET",
      );
      await page.goto("/dashboard");
      await expect(
        page.getByRole("heading", { name: "Dashboard" }),
      ).toBeVisible();
      const meStatus = await fetchStatusWithStoredToken(
        page,
        "/api/v1/auth/me",
        "GET",
      );
      expect(meStatus).toBe(200);

      const meResp = await meRespPromise;
      expect(meResp.status()).toBe(200);
      // ── CONTRACT CHECK: UserInfo schema (getCurrentUser) ──────────────────
      await validateApiResponse("UserInfo", meResp);
    });
  });

  // ── Stage 4.A: System CRUD ──────────────────────────────────────────────────

  test("Stage 4.A – listSystems + createSystem + updateSystem + deleteSystem (schema-validated)", async ({
    page,
  }) => {
    // operationId: listSystems, createSystem, updateSystem, deleteSystem
    await test.step("Stage 4.A / Step 1: execute system CRUD flow with contract checks", async () => {
      // ── CONTRACT CHECK: listSystems → SystemList ──────────────────────────────
      // Per Playwright best practice: register Promise BEFORE the action that
      // triggers the network request, to avoid missing a fast response.
      const listRespPromise = page.waitForResponse(
        (r) =>
          urlPathIncludes(r.url(), "/api/v1/systems") &&
          r.request().method() === "GET" &&
          !urlPathIncludes(r.url(), "/members"),
      );
      const [listResp] = await Promise.all([
        listRespPromise,
        page.goto("/systems"),
      ]);
      expect(listResp.status()).toBe(200);
      await validateApiResponse("SystemList", listResp);
      await expect(
        page.getByRole("heading", { name: "Systems" }),
      ).toBeVisible();

      // ── CONTRACT CHECK: createSystem → System ────────────────────────────────
      const systemName = `e2es${Date.now().toString(36).slice(-6)}`;
      const createRespPromise = page.waitForResponse(
        (r) =>
          urlPathEndsWith(r.url(), "/api/v1/systems") &&
          r.request().method() === "POST",
      );

      await page.getByTestId("system-create-button").click();
      const createModal = getAntModal(page, "system-create-modal");
      await expect(createModal).toBeVisible();
      await createModal
        .locator('input[maxlength="15"]')
        .first()
        .fill(systemName);
      await createModal.locator("textarea").first().fill("created by live e2e");
      await createModal.getByRole("button", { name: "OK" }).click();

      const { body: createdSystem } = await expectSchema(
        createRespPromise,
        "System",
        201,
      );
      const systemID = (createdSystem as { id?: string }).id ?? "";
      expect(systemID).toBeTruthy();

      await expect(
        page.locator("tr").filter({ hasText: systemName }).first(),
      ).toBeVisible();

      // ── CONTRACT CHECK: updateSystem → System ────────────────────────────────
      const updateRespPromise = page.waitForResponse(
        (r) =>
          urlPathIncludes(r.url(), `/api/v1/systems/${systemID}`) &&
          r.request().method() === "PATCH",
      );
      await page.getByTestId(`system-action-edit-${systemID}`).click();
      const editModal = getAntModal(page, "system-edit-modal");
      await expect(editModal).toBeVisible();
      await editModal.locator("textarea").first().fill("updated by live e2e");
      await editModal.getByRole("button", { name: "OK" }).click();

      await expectSchema(updateRespPromise, "System", 200);

      // ── CONTRACT CHECK: deleteSystem with confirm_name guard ──────────────────
      await page.getByTestId(`system-action-delete-${systemID}`).click();
      const deleteModal = getAntModal(page, "system-delete-modal");
      await expect(deleteModal).toBeVisible();

      const deleteBtn = deleteModal.getByRole("button", { name: /delete/i });
      await expect(deleteBtn).toBeDisabled();

      // Wrong name → still disabled (cascade guard UI test)
      await deleteModal.getByRole("textbox").first().fill("wrong-name");
      await expect(deleteBtn).toBeDisabled();

      // Correct name → enabled
      await deleteModal.getByRole("textbox").first().fill(systemName);
      await expect(deleteBtn).toBeEnabled();

      const deleteRespPromise = page.waitForResponse(
        (r) =>
          urlPathIncludes(r.url(), `/api/v1/systems/${systemID}`) &&
          r.request().method() === "DELETE",
      );
      await deleteBtn.click();

      const deleteResp = await deleteRespPromise;
      expect(deleteResp.status()).toBe(204);
      // Verify confirm_name was sent as query param (ADR-0015 §13)
      expect(deleteResp.url()).toContain(`confirm_name=${systemName}`);

      await expect(
        page.locator("tr").filter({ hasText: systemName }),
      ).toHaveCount(0);
    });
  });

  // ── Stage 4.A+: System Member Management ────────────────────────────────────

  test("Stage 4.A+ – listSystemMembers: system member list (schema-validated)", async ({
    page,
  }) => {
    // operationId: listSystemMembers
    await test.step("Stage 4.A+ / Step 1: verify member list endpoint for a system", async () => {
      // Create a temporary system to test member management
      await page.goto("/systems");
      await expect(
        page.getByRole("heading", { name: "Systems" }),
      ).toBeVisible();

      const systemName = `e2em${Date.now().toString(36).slice(-6)}`;
      const createRespPromise = page.waitForResponse(
        (r) =>
          urlPathEndsWith(r.url(), "/api/v1/systems") &&
          r.request().method() === "POST",
      );
      await page.getByTestId("system-create-button").click();
      const createModal = getAntModal(page, "system-create-modal");
      await expect(createModal).toBeVisible();
      await createModal
        .locator('input[maxlength="15"]')
        .first()
        .fill(systemName);
      await createModal.getByRole("button", { name: "OK" }).click();

      const { body: createdSystem } = await expectSchema(
        createRespPromise,
        "System",
        201,
      );
      const systemID = (createdSystem as { id?: string }).id ?? "";
      expect(systemID).toBeTruthy();

      // ── CONTRACT CHECK: listSystemMembers → SystemMemberList ──────────────────
      const membersRespPromise = page.waitForResponse(
        (r) =>
          urlPathIncludes(r.url(), `/api/v1/systems/${systemID}/members`) &&
          r.request().method() === "GET",
      );
      await page.getByTestId(`system-action-members-${systemID}`).click();
      const membersModal = getAntModal(page, "system-members-modal");
      await expect(membersModal).toBeVisible();

      const membersResp = await membersRespPromise;
      expect(membersResp.status()).toBe(200);
      await validateApiResponse("SystemMemberList", membersResp);

      // Close modal and clean up
      await membersModal
        .getByRole("button", { name: /cancel|close/i })
        .first()
        .click();

      // Delete the temp system
      await page.getByTestId(`system-action-delete-${systemID}`).click();
      const deleteModal = getAntModal(page, "system-delete-modal");
      await expect(deleteModal).toBeVisible();
      await deleteModal.getByRole("textbox").first().fill(systemName);
      const deleteRespPromise = page.waitForResponse(
        (r) =>
          urlPathIncludes(r.url(), `/api/v1/systems/${systemID}`) &&
          r.request().method() === "DELETE",
      );
      await deleteModal.getByRole("button", { name: /delete/i }).click();
      const deleteResp = await deleteRespPromise;
      expect(deleteResp.status()).toBe(204);
    });
  });

  // ── Stage 4.B: Service CRUD ──────────────────────────────────────────────────

  test("Stage 4.B – createService + updateService + deleteService (schema-validated)", async ({
    page,
  }) => {
    // operationId: createService, updateService, deleteService
    await test.step("Stage 4.B / Step 1: execute service CRUD flow with system ownership", async () => {
      // First create a system to own the service
      await page.goto("/systems");
      const systemName = `e2esvc${Date.now().toString(36).slice(-5)}`;
      const createSysRespPromise = page.waitForResponse(
        (r) =>
          urlPathEndsWith(r.url(), "/api/v1/systems") &&
          r.request().method() === "POST",
      );
      await page.getByTestId("system-create-button").click();
      const createSysModal = getAntModal(page, "system-create-modal");
      await expect(createSysModal).toBeVisible();
      await createSysModal
        .locator('input[maxlength="15"]')
        .first()
        .fill(systemName);
      await createSysModal.getByRole("button", { name: "OK" }).click();

      const { body: sys } = await expectSchema(
        createSysRespPromise,
        "System",
        201,
      );
      const systemID = (sys as { id?: string }).id ?? "";
      expect(systemID).toBeTruthy();

      // Navigate to services and select the new system
      await page.goto("/services");
      await expect(
        page.getByRole("heading", { name: "Services" }),
      ).toBeVisible();
      await selectServicesSystemFilter(page, systemName);

      // ── CONTRACT CHECK: createService → Service ───────────────────────────────
      const serviceName = `e2e-svc-${Date.now().toString(36).slice(-5)}`;
      const createSvcRespPromise = page.waitForResponse(
        (r) =>
          urlPathIncludes(r.url(), `/api/v1/systems/${systemID}/services`) &&
          r.request().method() === "POST",
      );
      await page.getByTestId("service-create-button").click();
      const createSvcModal = getAntModal(page, "service-create-modal");
      await expect(createSvcModal).toBeVisible();
      await createSvcModal
        .getByPlaceholder("e.g. web, api-gateway")
        .fill(serviceName);
      await createSvcModal
        .locator("textarea")
        .first()
        .fill("service created by live e2e");
      await createSvcModal.getByRole("button", { name: "OK" }).click();

      const { body: createdSvc } = await expectSchema(
        createSvcRespPromise,
        "Service",
        201,
      );
      const serviceID = (createdSvc as { id?: string }).id ?? "";
      expect(serviceID).toBeTruthy();

      const serviceListRespPromise = page.waitForResponse(
        (r) =>
          urlPathEndsWith(r.url(), `/api/v1/systems/${systemID}/services`) &&
          r.request().method() === "GET",
      );
      await page.goto(`/services?system_id=${systemID}`);
      await serviceListRespPromise;
      await expect(
        page.getByRole("heading", { name: "Services" }),
      ).toBeVisible();

      const createdServiceRow = page
        .locator("tr")
        .filter({ hasText: serviceName })
        .first();
      await expect(
        createdServiceRow,
      ).toBeVisible();

      // ── CONTRACT CHECK: updateService → Service ───────────────────────────────
      await createdServiceRow.getByTestId(`service-action-edit-${serviceID}`).click();
      const editSvcModal = getAntModal(page, "service-edit-modal");
      await expect(editSvcModal).toBeVisible();
      await editSvcModal
        .locator("textarea")
        .first()
        .fill("updated by live e2e");
      const updateSvcRespPromise = page.waitForResponse(
        (r) =>
          urlPathIncludes(
            r.url(),
            `/api/v1/systems/${systemID}/services/${serviceID}`,
          ) && r.request().method() === "PATCH",
      );
      await editSvcModal.getByRole("button", { name: "OK" }).click();

      await expectSchema(updateSvcRespPromise, "Service", 200);

      // ── CONTRACT CHECK: deleteService with confirm=true ───────────────────────
      const updatedServiceRow = page
        .locator("tr")
        .filter({ hasText: serviceName })
        .first();
      await expect(updatedServiceRow).toBeVisible();
      const deleteSvcRespPromise = page.waitForResponse(
        (r) =>
          urlPathIncludes(
            r.url(),
            `/api/v1/systems/${systemID}/services/${serviceID}`,
          ) && r.request().method() === "DELETE",
      );
      await updatedServiceRow.getByTestId(`service-action-delete-${serviceID}`).click();
      const popconfirm = page.locator(".ant-popover:visible");
      await expect(popconfirm).toBeVisible();
      await popconfirm.getByRole("button", { name: /confirm/i }).click();

      const deleteSvcResp = await deleteSvcRespPromise;
      expect(deleteSvcResp.status()).toBe(204);
      // Verify confirm=true was sent (ADR-0015 §13)
      expect(deleteSvcResp.url()).toContain("confirm=true");

      await expect(
        page.locator("tr").filter({ hasText: serviceName }),
      ).toHaveCount(0);

      // Clean up: delete the system
      await page.goto("/systems");
      await page.getByTestId(`system-action-delete-${systemID}`).click();
      const deleteModal = getAntModal(page, "system-delete-modal");
      await expect(deleteModal).toBeVisible();
      await deleteModal.getByRole("textbox").first().fill(systemName);
      const deleteSysRespPromise = page.waitForResponse(
        (r) =>
          urlPathIncludes(r.url(), `/api/v1/systems/${systemID}`) &&
          r.request().method() === "DELETE",
      );
      await deleteModal.getByRole("button", { name: /delete/i }).click();
      expect((await deleteSysRespPromise).status()).toBe(204);
    });
  });

  // ── Stage 4.C: Service Detail & Update Operations ───────────────────────────

  test("Stage 4.C – getService + updateService: service detail/update contract", async ({
    page,
    request,
  }) => {
    const token = await getAdminToken(request);
    const headers = { Authorization: `Bearer ${token}` };

    const systemName = `s4c${Date.now().toString(36).slice(-8)}`;
    const serviceName = `s4svc${Date.now().toString(36).slice(-7)}`;
    const initialDescription = "stage4c service detail fixture";
    let systemID = "";
    let serviceID = "";

    try {
      const createSystemResp = await request.post("/api/v1/systems", {
        headers,
        data: {
          name: systemName,
          description: "stage4c system fixture",
        },
      });
      expect(
        createSystemResp.status(),
        `POST /systems returned ${createSystemResp.status()}`,
      ).toBe(201);
      const systemBody = (await validateApiResponse(
        "System",
        createSystemResp,
      )) as { id?: string };
      systemID = String(systemBody.id ?? "").trim();
      expect(systemID).toBeTruthy();

      const createServiceResp = await request.post(
        `/api/v1/systems/${systemID}/services`,
        {
          headers,
          data: {
            name: serviceName,
            description: initialDescription,
          },
        },
      );
      expect(
        createServiceResp.status(),
        `POST /systems/${systemID}/services returned ${createServiceResp.status()}`,
      ).toBe(201);
      const serviceBody = (await validateApiResponse(
        "Service",
        createServiceResp,
      )) as { id?: string };
      serviceID = String(serviceBody.id ?? "").trim();
      expect(serviceID).toBeTruthy();

      await test.step("Stage 4.C / Step 1: open service detail payload and validate immutable identity fields", async () => {
        await page.goto("/services");
        await expect(
          page.getByRole("heading", { name: "Services" }),
        ).toBeVisible();
        await selectServicesSystemFilter(page, systemName);

        const serviceRow = page
          .locator("tr")
          .filter({ hasText: serviceName })
          .first();
        await expect(serviceRow).toBeVisible();

        const detailRespPromise = page.waitForResponse(
          (r) =>
            urlPathIncludes(
              r.url(),
              `/api/v1/systems/${systemID}/services/${serviceID}`,
            ) && r.request().method() === "GET",
        );
        await page.getByTestId(`service-action-edit-${serviceID}`).click();

        const detailResp = await detailRespPromise;
        expect(
          detailResp.status(),
          `GET /systems/${systemID}/services/${serviceID} returned ${detailResp.status()}`,
        ).toBe(200);
        const detailBody = (await validateApiResponse(
          "Service",
          detailResp,
        )) as {
          id?: string;
          name?: string;
          description?: string;
          system_id?: string;
        };
        expect(detailBody.id).toBe(serviceID);
        expect(detailBody.system_id).toBe(systemID);
        expect(detailBody.name).toBe(serviceName);
        expect(detailBody.description).toBe(initialDescription);

        const editModal = getAntModal(page, "service-edit-modal");
        await expect(editModal).toBeVisible();
      });

      await test.step("Stage 4.C / Step 2: update description-only mutation keeps service name immutable", async () => {
        const editModal = getAntModal(page, "service-edit-modal");
        await expect(editModal).toBeVisible();

        const updatedDescription = `stage4c updated ${Date.now().toString(36).slice(-6)}`;
        const patchRespPromise = page.waitForResponse(
          (r) =>
            urlPathIncludes(
              r.url(),
              `/api/v1/systems/${systemID}/services/${serviceID}`,
            ) && r.request().method() === "PATCH",
        );

        await editModal.locator("textarea").first().fill(updatedDescription);
        await editModal.getByRole("button", { name: "OK" }).click();

        const patchResp = await patchRespPromise;
        expect(
          patchResp.status(),
          `PATCH /systems/${systemID}/services/${serviceID} returned ${patchResp.status()}`,
        ).toBe(200);
        const patchBody = (await validateApiResponse("Service", patchResp)) as {
          id?: string;
          name?: string;
          description?: string;
          system_id?: string;
        };

        const patchReqRaw = patchResp.request().postData() ?? "";
        let patchReqBody: Record<string, unknown> = {};
        if (patchReqRaw.trim() !== "") {
          try {
            patchReqBody = JSON.parse(patchReqRaw) as Record<string, unknown>;
          } catch (err) {
            throw new Error(
              `PATCH request payload is not valid JSON: ${String(err)}`,
            );
          }
        }
        expect(patchReqBody).toEqual({ description: updatedDescription });

        expect(patchBody.id).toBe(serviceID);
        expect(patchBody.system_id).toBe(systemID);
        expect(patchBody.name).toBe(serviceName);
        expect(patchBody.description).toBe(updatedDescription);

        await expect(
          page.locator("tr").filter({ hasText: serviceName }).first(),
        ).toBeVisible();
      });
    } finally {
      if (serviceID && systemID) {
        await request.delete(
          `/api/v1/systems/${systemID}/services/${serviceID}?confirm=true`,
          { headers },
        );
      }
      if (systemID) {
        await request.delete(
          `/api/v1/systems/${systemID}?confirm_name=${encodeURIComponent(systemName)}`,
          { headers },
        );
      }
    }
  });

  // ── Stage 5.D: Cascade guard – Service with child VMs → 409 ─────────────────

  test("Stage 5.D – deleteService returns 409 when child VMs exist (cascade guard)", async ({
    page,
  }) => {
    // operationId: deleteService (cascade guard path)
    await test.step("Stage 5.D / Step 1: service delete is blocked when child VMs exist", async () => {
      await page.goto("/services");
      await expect(
        page.getByRole("heading", { name: "Services" }),
      ).toBeVisible();

      await selectServicesSystemFilter(page, e2eSystemName);

      const serviceRow = page
        .locator("tr")
        .filter({ hasText: e2eServiceName })
        .first();
      await expect(serviceRow).toBeVisible();

      const deleteRespPromise = page.waitForResponse(
        (r) =>
          urlPathIncludes(r.url(), "/api/v1/systems/") &&
          urlPathIncludes(r.url(), "/services/") &&
          r.request().method() === "DELETE",
      );
      await serviceRow
        .locator('[data-testid^="service-action-delete-"]')
        .first()
        .click();
      await page.getByRole("button", { name: /confirm/i }).click();

      const deleteResp = await deleteRespPromise;
      expect(deleteResp.status()).toBe(409);
      expect(deleteResp.url()).toContain("confirm=true");

      // ── CONTRACT CHECK: Error response schema ─────────────────────────────────
      // Reuse body from validateApiResponse to avoid double-read of response body
      const body = (await validateApiResponse("Error", deleteResp)) as {
        code?: string;
      };
      expect(body.code).toBe("SERVICE_HAS_VMS");
    });
  });

  // ── Stage 5.A: VM Request Submission ────────────────────────────────────────

  test("Stage 5.A – getVMRequestContext + createVMRequest (schema-validated)", async ({
    page,
    request,
  }) => {
    // operationId: getVMRequestContext, createVMRequest
    // API-first isolation: use a fresh service name to avoid duplicate pending-request collisions.
    const stage5AServiceName = await createUniqueServiceForStage5A(request);
    let ticketID = "";
    const token = await getAdminToken(request);
    const headers = { Authorization: `Bearer ${token}` };

    await test.step("Stage 5.A / Step 1: open request wizard and validate request-context contract", async () => {
      const contextRespPromise = page.waitForResponse(
        (r) =>
          urlPathEndsWith(r.url(), "/api/v1/vms/request-context") &&
          r.request().method() === "GET",
      );

      await page.goto("/vms");
      await expect(
        page.getByRole("heading", { name: "Virtual Machines" }),
      ).toBeVisible();

      // Open VM request wizard
      await page.getByRole("button", { name: "Request VM" }).click();
      await expect(page.getByText("Create VM Request")).toBeVisible();

      const contextResp = await contextRespPromise;
      expect(contextResp.status()).toBe(200);
      await validateApiResponse("VMRequestContext", contextResp);
    });

    await test.step("Stage 5.A / Step 2: submit createVMRequest and capture ticket reference", async () => {
      // Step 0: Select System
      const systemSelect = getAntModal(page, "vm-request-wizard-modal")
        .locator('[role="combobox"]')
        .first();
      await selectAntOption(
        page,
        systemSelect,
        toLooseOptionFilter(e2eSystemName),
      );

      // Select Service
      const serviceSelect = getAntModal(page, "vm-request-wizard-modal")
        .locator('[role="combobox"]')
        .nth(1);
      await selectAntOption(
        page,
        serviceSelect,
        toLooseOptionFilter(stage5AServiceName),
      );

      await getAntModal(page, "vm-request-wizard-modal")
        .getByRole("button", { name: "Next" })
        .click();

      // Step 1: Template
      const templateSelect = getAntModal(page, "vm-request-wizard-modal")
        .locator('[role="combobox"]')
        .first();
      await selectAntOption(
        page,
        templateSelect,
        toLooseOptionFilter(e2eTemplateName),
      );
      await getAntModal(page, "vm-request-wizard-modal")
        .getByRole("button", { name: "Next" })
        .click();

      // Step 2: Instance Size
      const sizeSelect = getAntModal(page, "vm-request-wizard-modal")
        .locator('[role="combobox"]')
        .first();
      await selectAntOption(page, sizeSelect, toLooseOptionFilter(e2eSizeName));
      await getAntModal(page, "vm-request-wizard-modal")
        .getByRole("button", { name: "Next" })
        .click();

      // Step 3: Namespace + Reason
      await getAntModal(page, "vm-request-wizard-modal")
        .locator("#vm-request-wizard_namespace")
        .fill(e2eNamespace);
      await getAntModal(page, "vm-request-wizard-modal")
        .locator("#vm-request-wizard_reason")
        .fill("live e2e test request");
      await getAntModal(page, "vm-request-wizard-modal")
        .getByRole("button", { name: "Next" })
        .click();

      // Step 4: Submit
      const submitBtn = getAntModal(page, "vm-request-wizard-modal").getByRole(
        "button",
        { name: "Submit" },
      );
      await expect(submitBtn).toBeVisible({ timeout: 5000 });

      const submitRespPromise = page.waitForResponse(
        (r) =>
          urlPathEndsWith(r.url(), "/api/v1/vms/request") &&
          r.request().method() === "POST",
      );
      await submitBtn.click();

      const submitResp = await submitRespPromise;
      expect(
        submitResp.status(),
        `POST /vms/request returned ${submitResp.status()}`,
      ).toBe(202);
      const submitBody = (await validateApiResponse(
        "TicketResponse",
        submitResp,
      )) as {
        ticket_id?: string;
        id?: string;
        status?: string;
        operation_type?: string;
      };
      ticketID = String(submitBody.ticket_id ?? submitBody.id ?? "").trim();
      expect(
        ticketID,
        "createVMRequest must return ticket reference for polling",
      ).toBeTruthy();

      await test.step("Stage 2.E / Step 1: user submission returns canonical pending ticket", async () => {
        expect(String(submitBody.status ?? "").toUpperCase()).toBe("PENDING");
        if (typeof submitBody.operation_type !== "undefined") {
          expect(String(submitBody.operation_type).toUpperCase()).toBe(
            "CREATE",
          );
        }
      });
    });

    await test.step("Stage 5.A / Step 3: newly created ticket is persisted as pending", async () => {
      await test.step("Stage 2.E / Step 2: ticket enters PENDING state without external provider selection input", async () => {
        await waitForTicketStatus(request, headers, ticketID, "PENDING");
      });
    });
  });

  // ── Stage 5: VM List ─────────────────────────────────────────────────────────

  test("Stage 5 – listVMs: VM list conforms to VMList schema", async ({
    page,
  }) => {
    // operationId: listVMs
    const vmListRespPromise = page.waitForResponse(
      (r) =>
        urlPathIncludes(r.url(), "/api/v1/vms") &&
        r.request().method() === "GET" &&
        !urlPathIncludes(r.url(), "/batch") &&
        !urlPathIncludes(r.url(), "/request") &&
        !urlPathIncludes(r.url(), "/console"),
    );
    const [vmListResp] = await Promise.all([
      vmListRespPromise,
      page.goto("/vms"),
    ]);
    expect(vmListResp.status()).toBe(200);
    // ── CONTRACT CHECK: listVMs → VMList ──────────────────────────────────
    await validateApiResponse("VMList", vmListResp);
    await expect(
      page.getByRole("heading", { name: "Virtual Machines" }),
    ).toBeVisible();
  });

  // ── Stage 5.B: Approvals ─────────────────────────────────────────────────────

  test("Stage 5.B – listApprovals: approval task list conforms to TicketList schema", async ({
    page,
  }) => {
    // operationId: listApprovals
    await test.step("Stage 5.B / Step 1: open approval list and validate pending-ticket surface", async () => {
      const listRespPromise = page.waitForResponse(
        (r) =>
          urlPathIncludes(r.url(), "/api/v1/builtin-approval/tasks") &&
          r.request().method() === "GET",
      );
      const [listResp] = await Promise.all([
        listRespPromise,
        page.goto("/admin/approval-tasks"),
      ]);
      expect(listResp.status()).toBe(200);
      await validateApiResponse("TicketList", listResp);
      await expect(
        page.getByRole("heading", { name: /approval/i }),
      ).toBeVisible();
    });
  });

  test("Stage 5.B – approveTicket: approve action calls real API", async ({
    page,
    request,
  }) => {
    // operationId: approveTicket
    const seeded = await seedPendingApprovalTicket(request);
    const ticketID = seeded.ticketID;
    const token = await getAdminToken(request);
    const headers = { Authorization: `Bearer ${token}` };
    const beforeVMs = await listVMBriefs(request, headers);
    const beforeVMIDs = new Set(beforeVMs.map((item) => item.id));

    await test.step("Stage 5.B / Step 2: approve path updates ticket/event and triggers execution", async () => {
      await page.goto("/admin/approval-tasks");
      await expect(
        page.getByRole("heading", { name: /approval/i }),
      ).toBeVisible();

      const approveBtn = page.getByTestId(
        `approval-action-approve-${ticketID}`,
      );
      await expect(
        approveBtn,
        "No pending approval tasks found — API setup may have failed",
      ).toBeVisible();

      await approveBtn.click();
      const modal = getAntModal(page, "approve-modal");
      await expect(modal).toBeVisible();
      await expect(
        modal.locator(".ant-select-selector").first(),
        "Stage 5.B requires selecting a target cluster before approval",
      ).toBeVisible();
      const clusterFilter = await resolveClusterOptionFilter(request, headers, e2eClusterName);
      await selectAntOption(
        page,
        modal.locator(".ant-select-selector").first(),
        clusterFilter,
      );
      await selectApprovalRootVolumeModeIfRequired(page, modal);

      const approveRespPromise = page.waitForResponse(
        (r) =>
          urlPathEndsWith(
            r.url(),
            `/api/v1/builtin-approval/tasks/${ticketID}/approve`,
          ) && r.request().method() === "POST",
      );
      await modal.getByRole("button", { name: /approve/i }).click();

      const approveResp = await approveRespPromise;
      expect(
        approveResp.status(),
        `POST /builtin-approval/tasks/{id}/approve returned ${approveResp.status()}`,
      ).toBe(204);
      await test.step("Stage 2.E / Step 3: approver decision transitions ticket to APPROVED state", async () => {
        await waitForTicketStatus(request, headers, ticketID, "APPROVED");
      });
      await waitForApprovalNotification(
        request,
        headers,
        ticketID,
        "APPROVAL_COMPLETED",
      );

      await test.step("Stage 2.E / Step 4: approved decision executes workload path and persists approval audit", async () => {
        // ── MASTER-FLOW Stage 5.C CHECK: approval must materialize VM + execute ──
        const createdVMID = await waitForNewVMFromApproval(
          request,
          headers,
          beforeVMIDs,
        );
        await waitForVMExecutionOutcome(request, headers, createdVMID);

        await test.step("Stage 3.C / Step 1: ticket payload keeps namespace immutable across decision and execution", async () => {
          await expect
            .poll(
              async () => {
                const found = await findApprovalTicket(
                  request,
                  headers,
                  ticketID,
                );
                if (!found) {
                  return "";
                }
                return extractNamespaceFromTicketPayload(found.ticket_payload);
              },
              {
                timeout: 20_000,
                intervals: [500, 1000, 2000],
                message: `ticket payload namespace not found for ticket ${ticketID}`,
              },
            )
            .toBe(seeded.namespace);
        });

        await test.step("Stage 3.JIT Namespace / Step 1: materialized VM stays in requested namespace", async () => {
          const vmResp = await request.get(`/api/v1/vms/${createdVMID}`, {
            headers,
          });
          expect(
            vmResp.status(),
            `GET /vms/${createdVMID} returned ${vmResp.status()}`,
          ).toBe(200);
          const vmBody = (await validateApiResponse("VM", vmResp)) as {
            namespace?: string;
          };
          expect(
            String(vmBody.namespace ?? "").trim(),
            "approved VM namespace should match request payload namespace",
          ).toBe(seeded.namespace);
        });

        await test.step("Stage 3.JIT Namespace / Step 2: requested namespace remains registered in logical namespace catalog", async () => {
          const namespacesResp = await request.get(
            "/api/v1/admin/namespaces?page=1&per_page=100",
            { headers },
          );
          expect(
            namespacesResp.status(),
            `GET /admin/namespaces returned ${namespacesResp.status()}`,
          ).toBe(200);
          const namespacesBody = (await validateApiResponse(
            "NamespaceRegistryList",
            namespacesResp,
          )) as {
            items?: Array<{ name?: string }>;
          };
          const exists = (namespacesBody.items ?? []).some(
            (item) => String(item.name ?? "").trim() === seeded.namespace,
          );
          expect(
            exists,
            `namespace ${seeded.namespace} should remain in logical namespace registry`,
          ).toBe(true);
        });

        await expect
          .poll(
            async () => {
              const auditResp = await request.get(
                `/api/v1/audit-logs?page=1&per_page=100&resource_type=ticket&resource_id=${encodeURIComponent(ticketID)}`,
                { headers },
              );
              if (auditResp.status() !== 200) {
                return `HTTP_${auditResp.status()}`;
              }
              const auditBody = (await validateApiResponse(
                "AuditLogList",
                auditResp,
              )) as {
                items?: Array<{ action?: string }>;
              };
              const hasApproved = (auditBody.items ?? []).some(
                (item) =>
                  String(item.action ?? "").toLowerCase() ===
                  "approval.approved",
              );
              return hasApproved ? "FOUND" : "MISSING";
            },
            {
              timeout: 20_000,
              intervals: [500, 1000, 2000],
              message: `expected approval.approved audit log for ticket ${ticketID}`,
            },
          )
          .toBe("FOUND");
      });
    });
  });

  test("Stage 5.B – rejectTicket: reject action calls real API", async ({
    page,
    request,
  }) => {
    // operationId: rejectTicket
    const seeded = await seedPendingApprovalTicket(request);
    const ticketID = seeded.ticketID;

    await test.step("Stage 5.B / Step 3: reject path closes ticket without creating VM", async () => {
      await page.goto("/admin/approval-tasks");
      await expect(
        page.getByRole("heading", { name: /approval/i }),
      ).toBeVisible();

      const actionsBtn = page.getByTestId(`approval-action-more-${ticketID}`);
      await expect(
        actionsBtn,
        "No pending approval tasks found — API setup may have failed",
      ).toBeVisible();
      await actionsBtn.click();

      const rejectBtn = page.getByTestId(`approval-action-reject-${ticketID}`);
      await expect(rejectBtn).toBeVisible();
      await rejectBtn.click();
      const modal = getAntModal(page, "reject-modal");
      await expect(modal).toBeVisible();
      await modal.locator("textarea").first().fill("Rejected by live e2e test");

      const rejectRespPromise = page.waitForResponse(
        (r) =>
          urlPathEndsWith(
            r.url(),
            `/api/v1/builtin-approval/tasks/${ticketID}/reject`,
          ) && r.request().method() === "POST",
      );
      await modal.getByRole("button", { name: /^OK$/i }).click();

      const rejectResp = await rejectRespPromise;
      expect(
        rejectResp.status(),
        `POST /builtin-approval/tasks/{id}/reject returned ${rejectResp.status()}`,
      ).toBe(204);
      const token = await getAdminToken(request);
      const headers = { Authorization: `Bearer ${token}` };
      await waitForTicketStatus(request, headers, ticketID, "REJECTED");
      await waitForApprovalNotification(
        request,
        headers,
        ticketID,
        "APPROVAL_REJECTED",
      );
    });
  });

  test("Stage 5.B Constraint – dedicated CPU with overcommit is blocked before execution", async ({
    request,
  }) => {
    const token = await getAdminToken(request);
    const headers = { Authorization: `Bearer ${token}` };
    const clusterID = await resolveHealthyClusterID(request, headers);
    expect(
      clusterID,
      "Stage 5.B constraint check requires at least one cluster",
    ).toBeTruthy();

    const instanceSizeName = `s5bc-${Date.now().toString(36).slice(-7)}`;
    const serviceName = await createUniqueServiceForApprovalSeed(request);
    let instanceSizeID = "";
    let ticketID = "";
    let catalogBlockedConflict = false;

    try {
      await test.step("Stage 5.B Constraint / Step 1: construct dedicated+overcommit conflict through staged update", async () => {
        const createSizeResp = await request.post(
          "/api/v1/admin/instance-sizes",
          {
            headers,
            data: {
              name: instanceSizeName,
              cpu_cores: 4,
              memory_gi: 8,
              cpu_request: 2,
              dedicated_cpu: false,
              enabled: true,
            },
          },
        );
        expect(
          createSizeResp.status(),
          `POST /admin/instance-sizes returned ${createSizeResp.status()}`,
        ).toBe(201);
        const createdSize = (await validateApiResponse(
          "InstanceSize",
          createSizeResp,
        )) as {
          id?: string;
          dedicated_cpu?: boolean;
          cpu_cores?: number;
          cpu_request?: number;
        };
        instanceSizeID = String(createdSize.id ?? "").trim();
        expect(
          instanceSizeID,
          "created instance size id is required",
        ).toBeTruthy();
        if (typeof createdSize.dedicated_cpu !== "undefined") {
          expect(createdSize.dedicated_cpu).toBe(false);
        }
        expect(Number(createdSize.cpu_cores ?? 0)).toBe(4);
        if (typeof createdSize.cpu_request !== "undefined") {
          expect(Number(createdSize.cpu_request)).toBe(2);
        }

        const patchSizeResp = await request.patch(
          `/api/v1/admin/instance-sizes/${instanceSizeID}`,
          {
            headers,
            data: {
              dedicated_cpu: true,
            },
          },
        );
        if (patchSizeResp.status() === 400) {
          catalogBlockedConflict = true;
          const errBody = (await validateApiResponse(
            "Error",
            patchSizeResp,
          )) as { code?: string; message?: string };
          expect(
            `${errBody.code ?? ""} ${errBody.message ?? ""}`,
            "catalog update must hard-block dedicated+overcommit conflict",
          ).toMatch(/DEDICATED_CPU_OVERCOMMIT_CONFLICT|dedicated CPU|overcommit/i);
          return;
        }
        expect(
          patchSizeResp.status(),
          `PATCH /admin/instance-sizes/{id} returned ${patchSizeResp.status()}`,
        ).toBe(200);
        const patchedSize = (await validateApiResponse(
          "InstanceSize",
          patchSizeResp,
        )) as {
          dedicated_cpu?: boolean;
          cpu_cores?: number;
          cpu_request?: number;
        };
        expect(
          patchedSize.dedicated_cpu,
          "staged patch should enable dedicated_cpu",
        ).toBe(true);
        expect(Number(patchedSize.cpu_cores ?? 0)).toBe(4);
        if (typeof patchedSize.cpu_request !== "undefined") {
          expect(Number(patchedSize.cpu_request)).toBe(2);
        }
      });

      await test.step("Stage 5.B Constraint / Step 2: approval API blocks with canonical DEDICATED_CPU_OVERCOMMIT_CONFLICT", async () => {
        if (catalogBlockedConflict) {
          return;
        }
        const createReqData = await resolveCreateVMRequestData(
          request,
          headers,
          "Stage 5.B constraint request",
          serviceName,
        );
        const createResp = await request.post("/api/v1/vms/request", {
          headers,
          data: {
            ...createReqData,
            instance_size_id: instanceSizeID,
            reason: `Stage 5.B dedicated-vs-overcommit conflict ${Date.now()}`,
          },
        });
        expect(
          createResp.status(),
          `POST /vms/request returned ${createResp.status()}`,
        ).toBe(202);
        const createBody = (await validateApiResponse(
          "TicketResponse",
          createResp,
        )) as { ticket_id?: string; id?: string };
        ticketID = String(createBody.ticket_id ?? createBody.id ?? "").trim();
        expect(
          ticketID,
          "ticket id is required for conflict approval assertion",
        ).toBeTruthy();

        const approveResp = await request.post(
          `/api/v1/builtin-approval/tasks/${ticketID}/approve`,
          {
            headers,
            data: {
              selected_cluster_id: clusterID,
            },
          },
        );
        expect(
          approveResp.status(),
          `POST /builtin-approval/tasks/{id}/approve returned ${approveResp.status()}`,
        ).toBe(400);
        const errBody = (await validateApiResponse("Error", approveResp)) as {
          code?: string;
        };
        expect(
          String(errBody.code ?? ""),
          "approval must hard-block dedicated+overcommit conflict",
        ).toBe("DEDICATED_CPU_OVERCOMMIT_CONFLICT");
        await waitForTicketStatus(request, headers, ticketID, "PENDING");
      });
    } finally {
      if (ticketID) {
        const rejectResp = await request.post(
          `/api/v1/builtin-approval/tasks/${ticketID}/reject`,
          {
            headers,
            data: { reason: "cleanup after Stage 5.B constraint test" },
          },
        );
        expect(
          [204, 400, 404],
          `cleanup reject ticket returned ${rejectResp.status()}`,
        ).toContain(rejectResp.status());
      }
      if (instanceSizeID) {
        const deleteResp = await request.delete(
          `/api/v1/admin/instance-sizes/${instanceSizeID}`,
          { headers },
        );
        expect(
          [204, 404, 409],
          `cleanup delete instance size returned ${deleteResp.status()}`,
        ).toContain(deleteResp.status());
      }
    }
  });

  // ── Stage 5.E: Batch Power Action ────────────────────────────────────────────

  test("Stage 5.E – submitVMBatchPower: batch power action → VMBatchSubmitResponse schema", async ({
    page,
    request,
  }) => {
    // operationId: submitVMBatchPower
    let batchID = "";

    await test.step("Stage 5.E / Step 1: select a STOPPED VM row from list page", async () => {
      await page.goto("/vms");
      await expect(
        page.getByRole("heading", { name: "Virtual Machines" }),
      ).toBeVisible();

      const stoppedRow = page
        .locator("tr")
        .filter({ hasText: /stopped/i })
        .first();
      await expect(
        stoppedRow,
        "No stopped VMs available for batch power test",
      ).toBeVisible();
      await stoppedRow.getByRole("checkbox").check();
    });

    await test.step("Stage 5.E / Step 2-5: submit batch power request and validate 202 payload", async () => {
      const batchRespPromise = page.waitForResponse(
        (r) =>
          urlPathEndsWith(r.url(), "/api/v1/vms/batch/power") &&
          r.request().method() === "POST",
      );
      await page
        .getByRole("button", { name: "Start Selected", exact: true })
        .click();
      await page.getByRole("button", { name: "Confirm", exact: true }).click();

      const batchResp = await batchRespPromise;
      expect(
        batchResp.status(),
        `POST /vms/batch/power returned ${batchResp.status()}`,
      ).toBe(202);
      const body = (await validateApiResponse(
        "VMBatchSubmitResponse",
        batchResp,
      )) as {
        batch_id?: string;
        status?: string;
        status_url?: string;
        retry_after_seconds?: unknown;
      };

      batchID = String(body.batch_id ?? "").trim();
      const statusURL = String(body.status_url ?? "").trim();
      expect(batchID, "submitVMBatchPower must return batch_id").toBeTruthy();
      expect(
        statusURL,
        "submitVMBatchPower must return canonical status_url for polling",
      ).toBe(`/api/v1/vms/batch/${batchID}`);
      expect(
        Number(body.retry_after_seconds),
        "retry_after_seconds must be a positive integer",
      ).toBeGreaterThan(0);
      expect(
        String(body.status ?? "").toUpperCase(),
        "compatibility power submit should enter execution pipeline directly",
      ).not.toBe("PENDING_APPROVAL");
    });

    await test.step("Stage 5.E / Step 6: track returned status_url and validate aggregate counters", async () => {
      const token = await getAdminToken(request);
      const headers = { Authorization: `Bearer ${token}` };

      const statusResp = await request.get(`/api/v1/vms/batch/${batchID}`, {
        headers,
      });
      expect(
        statusResp.status(),
        `GET /vms/batch/${batchID} returned ${statusResp.status()}`,
      ).toBe(200);
      const statusBody = (await validateApiResponse(
        "VMBatchStatusResponse",
        statusResp,
      )) as {
        batch_id?: string;
        operation?: string;
        child_count?: unknown;
        success_count?: unknown;
        failed_count?: unknown;
        pending_count?: unknown;
      };

      expect(
        statusBody.batch_id,
        "status endpoint must echo same batch_id",
      ).toBe(batchID);
      expect(
        String(statusBody.operation ?? "").toUpperCase(),
        "batch operation must be POWER",
      ).toBe("POWER");
      const childCount = Number(statusBody.child_count ?? 0);
      const successCount = Number(statusBody.success_count ?? 0);
      const failedCount = Number(statusBody.failed_count ?? 0);
      const pendingCount = Number(statusBody.pending_count ?? 0);
      expect(childCount, "batch child_count must be positive").toBeGreaterThan(
        0,
      );
      expect(successCount + failedCount + pendingCount).toBe(childCount);

      await page.goto("/vms/batch");
      await expect(
        page.getByTestId(`batch-action-detail-${batchID}`),
      ).toBeVisible();
    });
  });

  // ── Stage 5.F: Notifications ─────────────────────────────────────────────────

  test("Stage 5.F – listNotifications: notification list conforms to NotificationList schema", async ({
    page,
  }) => {
    // operationId: listNotifications
    await test.step("Stage 5.F / Step 2: open notifications page and fetch notification list", async () => {
      const notifRespPromise = page.waitForResponse(
        (r) =>
          urlPathIncludes(r.url(), "/api/v1/notifications") &&
          r.request().method() === "GET" &&
          !urlPathIncludes(r.url(), "unread"),
      );
      const [notifResp] = await Promise.all([
        notifRespPromise,
        page.goto("/notifications"),
      ]);
      expect(
        notifResp.status(),
        `GET /notifications returned ${notifResp.status()}`,
      ).toBe(200);
      await validateApiResponse("NotificationList", notifResp);
      await expect(
        page.getByRole("heading", { name: "Notifications" }),
      ).toBeVisible();
    });
  });

  test("Stage 5.F – getUnreadCount: unread count endpoint returns valid integer", async ({
    page,
    request,
  }) => {
    // operationId: getUnreadCount
    let unreadCount = 0;

    await test.step("Stage 5.F / Step 1: dashboard polls unread count endpoint", async () => {
      const countRespPromise = page.waitForResponse(
        (r) =>
          urlPathEndsWith(r.url(), "/api/v1/notifications/unread-count") &&
          r.request().method() === "GET",
      );
      const [countResp] = await Promise.all([
        countRespPromise,
        page.goto("/dashboard"),
      ]);
      expect(
        countResp.status(),
        `GET /notifications/unread-count returned ${countResp.status()}`,
      ).toBe(200);
      const body = (await validateApiResponse("UnreadCount", countResp)) as {
        count?: unknown;
      };
      expect(typeof body.count, "unread count should be numeric").toBe(
        "number",
      );
      unreadCount = Number(body.count ?? 0);
      expect(
        unreadCount,
        "unread count should not be negative",
      ).toBeGreaterThanOrEqual(0);
      await expect(
        page.getByRole("heading", { name: "Dashboard" }),
      ).toBeVisible();
    });

    await test.step("Stage 5.F / Step 2: fetch notifications page list for user inbox", async () => {
      const notifRespPromise = page.waitForResponse(
        (r) =>
          urlPathIncludes(r.url(), "/api/v1/notifications") &&
          r.request().method() === "GET" &&
          !urlPathIncludes(r.url(), "unread"),
      );
      const [notifResp] = await Promise.all([
        notifRespPromise,
        page.goto("/notifications"),
      ]);
      expect(
        notifResp.status(),
        `GET /notifications returned ${notifResp.status()}`,
      ).toBe(200);
      await validateApiResponse("NotificationList", notifResp);
      await expect(
        page.getByRole("heading", { name: "Notifications" }),
      ).toBeVisible();
    });

    await test.step("Stage 5.F / Step 3: unread-count is consistent with unread items on first page", async () => {
      const token = await getAdminToken(request);
      const headers = { Authorization: `Bearer ${token}` };

      const notifResp = await request.get(
        "/api/v1/notifications?page=1&per_page=50",
        { headers },
      );
      expect(
        notifResp.status(),
        `GET /notifications?page=1&per_page=50 returned ${notifResp.status()}`,
      ).toBe(200);
      const notifBody = (await validateApiResponse(
        "NotificationList",
        notifResp,
      )) as {
        items?: Array<{ read?: boolean }>;
      };
      const unreadOnPage = (notifBody.items ?? []).filter(
        (item) => item.read === false,
      ).length;
      expect(
        unreadCount,
        "unread-count endpoint should not be smaller than unread items already visible on first page",
      ).toBeGreaterThanOrEqual(unreadOnPage);
    });
  });

  // ── Stage 6: VM Console ───────────────────────────────────────────────────────

  test("Stage 6 – requestVMConsoleAccess + openVMVNC: VM console request flow", async ({
    page,
    request,
  }) => {
    // operationId: requestVMConsoleAccess, openVMVNC
    await test.step("Stage 6 / Step 1: open VM list page and verify list contract", async () => {
      const vmListRespPromise = page.waitForResponse(
        (r) =>
          urlPathIncludes(r.url(), "/api/v1/vms") &&
          r.request().method() === "GET" &&
          !urlPathIncludes(r.url(), "/batch"),
      );
      const [vmListResp] = await Promise.all([
        vmListRespPromise,
        page.goto("/vms"),
      ]);
      expect(
        vmListResp.status(),
        `GET /vms returned ${vmListResp.status()}`,
      ).toBe(200);
      await validateApiResponse("VMList", vmListResp);
      await expect(
        page.getByRole("heading", { name: "Virtual Machines" }),
      ).toBeVisible();
    });

    let targetVMID = runningVMID;
    let targetOpenedOnDetail = false;
    await test.step("Stage 6 / Step 2: resolve runnable VM target from UI action button", async () => {
      const configuredButton = targetVMID
        ? page.getByTestId(`vm-action-console-${targetVMID}`)
        : null;
      const configuredVisible = configuredButton
        ? await configuredButton.isVisible().catch(() => false)
        : false;
      const configuredEnabled = configuredButton
        ? await configuredButton.isEnabled().catch(() => false)
        : false;
      if (!targetVMID || !configuredVisible || !configuredEnabled) {
        const enabledConsoleButton = page
          .locator('button[data-testid^="vm-action-console-"]:not([disabled])')
          .first();
        if (await enabledConsoleButton.isVisible().catch(() => false)) {
          const testId = await enabledConsoleButton.getAttribute("data-testid");
          targetVMID = testId?.replace("vm-action-console-", "") ?? "";
        } else {
          const token = await getAdminToken(request);
          const headers = { Authorization: `Bearer ${token}` };
          targetVMID = await createRunningVMForConsole(request, headers);
          await page.goto(`/vms/${targetVMID}`);
          targetOpenedOnDetail = true;
        }
      }
      expect(
        targetVMID,
        "Failed to resolve VM id from console action button",
      ).toBeTruthy();
    });

    const consoleRespPromise = page.waitForResponse(
      (r) =>
        urlPathEndsWith(r.url(), `/api/v1/vms/${targetVMID}/console/request`) &&
        r.request().method() === "POST",
    );
    if (!targetOpenedOnDetail) {
      await page.getByTestId(`vm-action-console-${targetVMID}`).click();
      await expect(page).toHaveURL(new RegExp(`/vms/${targetVMID}`));
    }
    await expect(page.getByTestId("vm-console-section")).toBeVisible();
    const detailConsoleButton = page.getByTestId(`vm-action-console-${targetVMID}`);
    await expect(detailConsoleButton).toBeEnabled({ timeout: 30_000 });
    await detailConsoleButton.click();
    await page.getByRole("button", { name: "Confirm" }).click();

    let consoleStatus = "";
    let consoleTicketID = "";
    let consolePath = "";
    let consoleType = "";
    await test.step("Stage 6 / Step 3: validate requestVMConsoleAccess branch by environment", async () => {
      const consoleResp = await consoleRespPromise;
      expect(
        [200, 202],
        `POST /vms/${targetVMID}/console/request returned unexpected ${consoleResp.status()}`,
      ).toContain(consoleResp.status());
      const consoleBody = (await validateApiResponse(
        "VMConsoleRequestResponse",
        consoleResp,
      )) as {
        status?: string;
        ticket_id?: string | null;
        console_type?: string | null;
        console_url?: string | null;
        vnc_url?: string | null;
      };
      consoleStatus = String(consoleBody.status ?? "").toUpperCase();
      consoleTicketID = String(consoleBody.ticket_id ?? "").trim();
      consoleType = String(consoleBody.console_type ?? "").toUpperCase();
      consolePath = String(
        consoleBody.console_url ?? consoleBody.vnc_url ?? "",
      ).trim();

      if (consoleResp.status() === 200) {
        expect(consoleStatus, "test-env direct path must be APPROVED").toBe(
          "APPROVED",
        );
        expect(
          consolePath,
          "APPROVED response must include a console path",
        ).toBeTruthy();
        expect(["SERIAL", "VNC"]).toContain(consoleType);
      } else {
        expect(consoleStatus, "prod path must return pending status").toBe(
          "PENDING_APPROVAL",
        );
        expect(
          consoleTicketID,
          "pending approval response must include ticket_id",
        ).toBeTruthy();
        expect(consolePath, "pending response must not include a console path").toBe("");
      }
    });

    await test.step("Stage 6 / Step 4: poll console status API to confirm persisted state", async () => {
      const token = await getAdminToken(request);
      const headers = { Authorization: `Bearer ${token}` };
      const statusResp = await request.get(
        `/api/v1/vms/${targetVMID}/console/status`,
        { headers },
      );
      expect(
        statusResp.status(),
        `GET /vms/${targetVMID}/console/status returned ${statusResp.status()}`,
      ).toBe(200);
      const statusBody = (await validateApiResponse(
        "VMConsoleStatusResponse",
        statusResp,
      )) as {
        status?: string;
        ticket_id?: string | null;
        console_type?: string | null;
        console_url?: string | null;
        vnc_url?: string | null;
      };
      const polledStatus = String(statusBody.status ?? "").toUpperCase();
      const polledTicketID = String(statusBody.ticket_id ?? "").trim();
      const polledConsolePath = String(
        statusBody.console_url ?? statusBody.vnc_url ?? "",
      ).trim();

      if (consoleStatus === "APPROVED") {
        expect(
          polledStatus,
          "approved request must remain approved in status poll",
        ).toBe("APPROVED");
        expect(
          polledConsolePath,
          "approved status payload must include a console path",
        ).toBe(consolePath);
      } else {
        expect(
          polledStatus,
          "pending request must remain pending until approval",
        ).toBe("PENDING_APPROVAL");
        expect(
          polledTicketID,
          "pending status payload must include ticket_id",
        ).toBe(consoleTicketID);
      }
    });

    await test.step("Stage 6 / Step 5: validate in-page console flow for approved flow", async () => {
      if (consoleStatus !== "APPROVED") {
        return;
      }
      await expect(page).toHaveURL(new RegExp(`/vms/${targetVMID}`));
      await expect(page.getByTestId("vm-console-section")).toBeVisible();
      expect(
        consolePath,
        "requestVMConsoleAccess approved path must align with the selected console endpoint",
      ).toBe(
        consoleType === "SERIAL"
          ? `/api/v1/vms/${targetVMID}/serial`
          : `/api/v1/vms/${targetVMID}/vnc`,
      );
    });
  });

  // ── Stage 3: Admin config (GET schemas) ──────────────────────────────────────

  test("Stage 3 – listAdminTemplates: admin template list conforms to TemplateList schema", async ({
    page,
  }) => {
    // operationId: listAdminTemplates
    await test.step("Stage 3 / Step 3: admin template catalog is reachable and schema-valid", async () => {
      const tplRespPromise = page.waitForResponse(
        (r) =>
          urlPathEndsWith(r.url(), "/api/v1/admin/templates") &&
          r.request().method() === "GET",
      );
      const [tplResp] = await Promise.all([
        tplRespPromise,
        page.goto("/admin/templates"),
      ]);
      expect(tplResp.status()).toBe(200);
      await validateApiResponse("TemplateList", tplResp);
      await expect(
        page.getByRole("heading", { name: "Templates" }),
      ).toBeVisible();
    });
  });

  test("Stage 3 – listAdminInstanceSizes: instance-size list conforms to InstanceSizeList schema", async ({
    page,
  }) => {
    // operationId: listAdminInstanceSizes
    await test.step("Stage 3 / Step 4: instance-size catalog is reachable and schema-valid", async () => {
      const sizeRespPromise = page.waitForResponse(
        (r) =>
          urlPathEndsWith(r.url(), "/api/v1/admin/instance-sizes") &&
          r.request().method() === "GET",
      );
      const [sizeResp] = await Promise.all([
        sizeRespPromise,
        page.goto("/admin/instance-sizes"),
      ]);
      expect(sizeResp.status()).toBe(200);
      await validateApiResponse("InstanceSizeList", sizeResp);
      await expect(
        page.getByRole("heading", { name: "Instance Sizes" }),
      ).toBeVisible();
    });
  });

  test("Stage 3 – listNamespaces: admin namespace list conforms to NamespaceRegistryList schema", async ({
    page,
  }) => {
    // operationId: listNamespaces
    await test.step("Stage 3 / Step 2: namespace registry is reachable and schema-valid", async () => {
      const nsRespPromise = page.waitForResponse(
        (r) =>
          urlPathEndsWith(r.url(), "/api/v1/admin/namespaces") &&
          r.request().method() === "GET",
      );
      const [nsResp] = await Promise.all([
        nsRespPromise,
        page.goto("/admin/namespaces"),
      ]);
      expect(nsResp.status()).toBe(200);
      await validateApiResponse("NamespaceRegistryList", nsResp);
      await expect(page.getByTestId("admin-namespaces-page")).toBeVisible();
    });
  });

  // ── Stage 2.A: RBAC ──────────────────────────────────────────────────────────

  test("Stage 2.A – listRoles: role list conforms to RoleList schema", async ({
    page,
  }) => {
    // operationId: listRoles
    await test.step("Stage 2.A / Step 1: role list endpoint is reachable and schema-valid", async () => {
      const rolesRespPromise = page.waitForResponse(
        (r) =>
          urlPathEndsWith(r.url(), "/api/v1/admin/roles") &&
          r.request().method() === "GET",
      );
      const [rolesResp] = await Promise.all([
        rolesRespPromise,
        page.goto("/admin/rbac"),
      ]);
      expect(rolesResp.status()).toBe(200);
      await validateApiResponse("RoleList", rolesResp);
      await expect(page.getByTestId("admin-rbac-page")).toBeVisible();
    });
  });

  // ── Stage 2.B: Auth Providers ────────────────────────────────────────────────

  test("Stage 2.B – listAuthProviderTypes: auth provider type list conforms to AuthProviderTypeList schema", async ({
    page,
  }) => {
    // operationId: listAuthProviderTypes
    await test.step("Stage 2.B / Step 1: auth provider type list is reachable and schema-valid", async () => {
      const typesRespPromise = page.waitForResponse(
        (r) =>
          urlPathEndsWith(r.url(), "/api/v1/admin/auth-provider-types") &&
          r.request().method() === "GET",
      );
      const [typesResp] = await Promise.all([
        typesRespPromise,
        page.goto("/admin/auth-providers"),
      ]);
      expect(typesResp.status()).toBe(200);
      await validateApiResponse("AuthProviderTypeList", typesResp);
      await expect(
        page.getByRole("heading", { name: "Authentication Providers" }),
      ).toBeVisible();
    });
  });

  test("Stage 2.B – listAuthProviders: auth provider list conforms to AuthProviderList schema", async ({
    page,
  }) => {
    // operationId: listAuthProviders
    await test.step("Stage 2.B / Step 2: auth provider list is reachable and schema-valid", async () => {
      const listRespPromise = page.waitForResponse(
        (r) =>
          urlPathEndsWith(r.url(), "/api/v1/admin/auth-providers") &&
          r.request().method() === "GET",
      );
      const [listResp] = await Promise.all([
        listRespPromise,
        page.goto("/admin/auth-providers"),
      ]);
      expect(listResp.status()).toBe(200);
      await validateApiResponse("AuthProviderList", listResp);
      await expect(
        page.getByRole("heading", { name: "Authentication Providers" }),
      ).toBeVisible();
    });
  });

  // ── Stage 2 (Supplemental): User Management ──────────────────────────────────

  test("Stage 2 (Supplemental) – listUsers: user list conforms to UserList schema", async ({
    page,
  }) => {
    // operationId: listUsers
    await test.step("Stage 2 (Supplemental) / User Mgmt Step 1: user list endpoint is reachable and schema-valid", async () => {
      const usersRespPromise = page.waitForResponse(
        (r) =>
          urlPathEndsWith(r.url(), "/api/v1/admin/users") &&
          r.request().method() === "GET",
      );
      const [usersResp] = await Promise.all([
        usersRespPromise,
        page.goto("/admin/users"),
      ]);
      expect(usersResp.status()).toBe(200);
      await validateApiResponse("UserList", usersResp);
      await expect(page.getByTestId("admin-users-page")).toBeVisible();
    });
  });
});
