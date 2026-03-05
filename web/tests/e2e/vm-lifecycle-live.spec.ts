/**
 * VM Lifecycle Live E2E Tests — Contract-Enforced (no mock, no skip)
 *
 * ┌─────────────────────────────────────────────────────────────────────────┐
 * │  REQUIRES: a running backend (db + server via docker-compose or local)  │
 * │  NO test.skip() — failures expose real frontend/backend problems.       │
 * │  Every API response is validated against api/openapi.yaml schema.       │
 * └─────────────────────────────────────────────────────────────────────────┘
 *
 * Coverage (all previously uncovered VM endpoints):
 *   getVM              – GET /vms/{id}                  → VM schema
 *   startVM            – POST /vms/{id}/start           → 202 (no body schema)
 *   stopVM             – POST /vms/{id}/stop            → 202 (no body schema)
 *   restartVM          – POST /vms/{id}/restart         → 202 (no body schema)
 *   deleteVM           – DELETE /vms/{id}               → DeleteVMResponse schema
 *   powerVM            – POST /vms/{id}/power           → VMPowerAcceptedResponse schema (202)
 *   listVMBatches      – GET /vms/batch                 → VMBatchList schema
 *   submitVMBatch      – POST /vms/batch                → VMBatchSubmitResponse schema
 *   getVMBatch         – GET /vms/batch/{id}            → VMBatchStatusResponse schema
 *   retryVMBatch       – POST /vms/batch/{id}/retry     → VMBatchActionResponse schema
 *   cancelVMBatch      – POST /vms/batch/{id}/cancel    → VMBatchActionResponse schema
 *   getVMConsoleStatus – GET /vms/{id}/console/status   → VMConsoleStatusResponse schema
 *
 * Strategy:
 *   - First create a VM via the approval flow (Stage 5.A → 5.B → 5.C).
 *   - Then exercise all lifecycle operations on that VM.
 *   - Failures mean the backend or frontend does not implement the contract.
 *
 * Environment variables:
 *   E2E_USERNAME  – admin username (default: e2e-admin)
 *   E2E_PASSWORD  – admin password (default: e2e-admin-123)
 *   E2E_NEW_PASSWORD – password used when force_password_change=true
 *   E2E_SYSTEM    – pre-existing system with at least one service (default: e2e-system)
 *   E2E_SERVICE   – pre-existing service name (default: e2e-service)
 */

import { expect, test, type APIRequestContext, type Page, type Response } from '@playwright/test';
import { validateApiResponse } from './lib/schema-validator';
import {
    ensureBatchSubmitPolicyForUser,
    fetchStatusWithStoredToken,
    getApiTokenWithForcePasswordSupport,
    loginWithForcePasswordSupport,
    urlPathEndsWith,
    urlPathIncludes,
} from './lib/helpers';

// ── Config ────────────────────────────────────────────────────────────────────

const e2eUsername = process.env.E2E_USERNAME ?? 'e2e-admin';
const e2ePassword = process.env.E2E_PASSWORD ?? 'e2e-admin-123';
const e2eNewPassword = process.env.E2E_NEW_PASSWORD ?? (e2ePassword === 'admin' ? 'admin123' : `${e2ePassword}-new`);
const e2eSystemName = process.env.E2E_SYSTEM ?? 'e2e-system';
const e2eServiceName = process.env.E2E_SERVICE ?? 'e2e-service';
const e2eTemplateName = process.env.E2E_TEMPLATE ?? 'e2e-template';
const e2eSizeName = process.env.E2E_SIZE ?? 'e2e-small';
const e2eNamespace = process.env.E2E_NAMESPACE ?? 'e2e-test';
const seededRunningVMID = process.env.E2E_VM_RUNNING_ID ?? 'vm-e2e-running';
const seededStoppedVMID = process.env.E2E_VM_STOPPED_ID ?? 'vm-e2e-stopped';
const seededRunningVMName = process.env.E2E_VM_RUNNING_NAME ?? 'vm-running';
const seededStoppedVMName = process.env.E2E_VM_STOPPED_NAME ?? 'vm-stopped';
const seededVMIDs = new Set([seededRunningVMID, seededStoppedVMID]);
const seededVMNames = new Set([
    seededRunningVMName.trim().toLowerCase(),
    seededStoppedVMName.trim().toLowerCase(),
    'vm-e2e-running',
    'vm-e2e-stopped',
]);
let activePassword = e2ePassword;
let lifecycleVMID = '';

// ── Auth helper ───────────────────────────────────────────────────────────────

async function login(page: Page): Promise<void> {
    activePassword = await loginWithForcePasswordSupport(page, {
        username: e2eUsername,
        primaryPassword: e2ePassword,
        secondaryPassword: e2eNewPassword,
        currentPasswordHint: activePassword,
    });
}

async function expectSchema(
    respPromise: Promise<Response>,
    schemaName: string,
    expectedStatus: number | number[] = 200
): Promise<{ body: unknown; resp: Response }> {
    const resp = await respPromise;
    const statuses = Array.isArray(expectedStatus) ? expectedStatus : [expectedStatus];
    expect(statuses, `Expected HTTP ${statuses.join('/')} but got ${resp.status()} for ${resp.url()}`).toContain(resp.status());
    const body = await validateApiResponse(schemaName, resp);
    return { body, resp };
}

type VMBrief = { id: string; name: string; status: string };
type ActionName = 'start' | 'stop' | 'restart' | 'delete';

function isSeededVMID(vmId: string): boolean {
    return seededVMIDs.has(vmId.trim());
}

function isLikelySeededVM(vm: Pick<VMBrief, 'id' | 'name'>): boolean {
    if (isSeededVMID(vm.id)) {
        return true;
    }
    const vmName = vm.name.trim().toLowerCase();
    return vmName !== '' && seededVMNames.has(vmName);
}

function pickIDByPreferredName(
    items: Array<{ id?: string; name?: string }> | undefined,
    preferredName: string
): string {
    const list = items ?? [];
    const exact = list.find((item) => (item.name ?? '').trim() === preferredName && item.id);
    if (exact?.id) return exact.id;
    const first = list.find((item) => Boolean(item.id));
    return first?.id ?? '';
}

function pickPreferredNamespace(namespaces: string[] | undefined, preferred: string): string {
    const list = (namespaces ?? []).map((ns) => ns.trim()).filter(Boolean);
    if (list.includes(preferred)) return preferred;
    return list[0] ?? preferred;
}

async function getAdminHeaders(request: APIRequestContext): Promise<{ Authorization: string }> {
    const auth = await getApiTokenWithForcePasswordSupport(request, {
        username: e2eUsername,
        primaryPassword: e2ePassword,
        secondaryPassword: e2eNewPassword,
        currentPasswordHint: activePassword,
    });
    activePassword = auth.password;
    return { Authorization: `Bearer ${auth.token}` };
}

async function listVMBriefs(request: APIRequestContext, headers: { Authorization: string }): Promise<VMBrief[]> {
    const vmMap = new Map<string, VMBrief>();
    let pageNum = 1;
    let totalPages = 1;

    do {
        const vmResp = await request.get(`/api/v1/vms?page=${pageNum}&per_page=100`, { headers });
        expect(vmResp.status(), `GET /vms?page=${pageNum} returned ${vmResp.status()}`).toBe(200);
        const vmBody = await validateApiResponse('VMList', vmResp) as {
            items?: Array<{ id?: string; name?: string; status?: string }>;
            pagination?: { total_pages?: number };
        };
        for (const item of vmBody.items ?? []) {
            if (!item.id) {
                continue;
            }
            vmMap.set(item.id, {
                id: item.id,
                name: item.name ?? '',
                status: (item.status ?? '').toUpperCase(),
            });
        }
        totalPages = Number(vmBody.pagination?.total_pages ?? 1) || 1;
        pageNum += 1;
    } while (pageNum <= totalPages);

    return Array.from(vmMap.values());
}

async function waitForVMStatus(
    request: APIRequestContext,
    headers: { Authorization: string },
    vmId: string,
    expected: 'RUNNING' | 'STOPPED'
): Promise<void> {
    await expect.poll(async () => {
        const vmResp = await request.get(`/api/v1/vms/${vmId}`, { headers });
        if (vmResp.status() !== 200) return `HTTP_${vmResp.status()}`;
        const vmBody = await validateApiResponse('VM', vmResp) as { status?: string };
        return (vmBody.status ?? '').toUpperCase();
    }, {
        timeout: 120_000,
        intervals: [1000, 2000, 4000, 8000],
        message: `VM ${vmId} did not reach ${expected}`,
    }).toBe(expected);
}

async function pickNonSeededVM(
    request: APIRequestContext,
    headers: { Authorization: string },
    preferredStatus?: 'RUNNING' | 'STOPPED'
): Promise<VMBrief> {
    const vms = await listVMBriefs(request, headers);
    const real = vms.filter((vm) => !isLikelySeededVM(vm));
    expect(real.length, 'No non-seeded VM available for vm-lifecycle action').toBeGreaterThan(0);
    if (preferredStatus) {
        const hit = real.find((vm) => vm.status === preferredStatus);
        if (hit) return hit;
    }
    return real[0];
}

async function ensureVMReadyForAction(
    request: APIRequestContext,
    headers: { Authorization: string },
    action: 'start' | 'stop' | 'restart',
    preferredVMID?: string
): Promise<string> {
    const targetStatus = action === 'start' ? 'STOPPED' : 'RUNNING';
    const vmList = await listVMBriefs(request, headers);
    const preferredVM = preferredVMID ? vmList.find((vm) => vm.id === preferredVMID) : undefined;
    const vm = preferredVM ?? (await pickNonSeededVM(request, headers, targetStatus));
    if (vm.status === targetStatus) {
        return vm.id;
    }

    if (targetStatus === 'STOPPED') {
        const stopResp = await request.post(`/api/v1/vms/${vm.id}/stop`, { headers });
        expect([202, 409], `pre-stop /vms/${vm.id}/stop returned ${stopResp.status()}`).toContain(stopResp.status());
        await waitForVMStatus(request, headers, vm.id, 'STOPPED');
        return vm.id;
    }

    const startResp = await request.post(`/api/v1/vms/${vm.id}/start`, { headers });
    expect([202, 409], `pre-start /vms/${vm.id}/start returned ${startResp.status()}`).toContain(startResp.status());
    await waitForVMStatus(request, headers, vm.id, 'RUNNING');
    return vm.id;
}

async function resolveApprovalClusterID(request: APIRequestContext, headers: { Authorization: string }): Promise<string> {
    const clustersResp = await request.get('/api/v1/admin/clusters?page=1&per_page=100', { headers });
    expect(clustersResp.status(), `GET /admin/clusters returned ${clustersResp.status()}`).toBe(200);
    const clustersBody = await validateApiResponse('ClusterList', clustersResp) as {
        items?: Array<{ id?: string; status?: string; enabled?: boolean }>;
    };
    const clusters = clustersBody.items ?? [];
    const healthy = clusters.find((cluster) => cluster.id && cluster.enabled !== false && (cluster.status ?? '').toUpperCase() === 'HEALTHY');
    if (healthy?.id) return healthy.id;
    const enabled = clusters.find((cluster) => cluster.id && cluster.enabled !== false);
    return enabled?.id ?? '';
}

async function resolveCreateVMRequestData(
    request: APIRequestContext,
    headers: { Authorization: string }
): Promise<{ service_id: string; template_id: string; instance_size_id: string; namespace: string }> {
    const systemsResp = await request.get('/api/v1/systems', { headers });
    expect(systemsResp.status(), `GET /systems returned ${systemsResp.status()}`).toBe(200);
    const systems = await validateApiResponse('SystemList', systemsResp) as { items?: Array<{ id?: string; name?: string }> };
    const systemID = pickIDByPreferredName(systems.items, e2eSystemName);
    expect(systemID, 'VM lifecycle setup requires at least one system').toBeTruthy();

    const [servicesResp, contextResp] = await Promise.all([
        request.get(`/api/v1/systems/${systemID}/services`, { headers }),
        request.get('/api/v1/vms/request-context', { headers }),
    ]);
    expect(servicesResp.status(), `GET /systems/${systemID}/services returned ${servicesResp.status()}`).toBe(200);
    expect(contextResp.status(), `GET /vms/request-context returned ${contextResp.status()}`).toBe(200);

    const services = await validateApiResponse('ServiceList', servicesResp) as { items?: Array<{ id?: string; name?: string }> };
    const ctx = await validateApiResponse('VMRequestContext', contextResp) as {
        templates?: Array<{ id?: string; name?: string }>;
        instance_sizes?: Array<{ id?: string; name?: string }>;
        namespaces?: string[];
    };

    const serviceID = pickIDByPreferredName(services.items, e2eServiceName);
    const templateID = pickIDByPreferredName(ctx.templates, e2eTemplateName);
    const sizeID = pickIDByPreferredName(ctx.instance_sizes, e2eSizeName);
    const namespace = pickPreferredNamespace(ctx.namespaces, e2eNamespace);
    expect(serviceID, 'VM lifecycle setup requires a service').toBeTruthy();
    expect(templateID, 'VM lifecycle setup requires a template').toBeTruthy();
    expect(sizeID, 'VM lifecycle setup requires an instance size').toBeTruthy();

    return {
        service_id: serviceID,
        template_id: templateID,
        instance_size_id: sizeID,
        namespace,
    };
}

async function submitOrReusePendingCreateTicket(
    request: APIRequestContext,
    headers: { Authorization: string }
): Promise<string> {
    const createReqData = await resolveCreateVMRequestData(request, headers);
    const createResp = await request.post('/api/v1/vms/request', {
        headers,
        data: { ...createReqData, reason: `vm-lifecycle setup ${Date.now()}` },
    });
    if (createResp.status() === 202) {
        const ticket = await validateApiResponse('ApprovalTicketResponse', createResp) as { ticket_id?: string; id?: string };
        const ticketID = ticket.ticket_id ?? ticket.id ?? '';
        expect(ticketID, 'ApprovalTicketResponse missing ticket id').toBeTruthy();
        return ticketID;
    }
    if (createResp.status() === 400) {
        const errBody = await validateApiResponse('Error', createResp) as {
            code?: string;
            params?: Record<string, unknown>;
            message?: string;
        };
        const existingTicketID =
            typeof errBody.params?.existing_ticket_id === 'string' ? errBody.params.existing_ticket_id.trim() : '';
        if (errBody.code === 'DUPLICATE_PENDING_REQUEST' && existingTicketID) {
            return existingTicketID;
        }
        throw new Error(
            `POST /vms/request failed in vm-lifecycle setup: ${errBody.code ?? 'UNKNOWN'} (${errBody.message ?? 'no message'})`
        );
    }
    throw new Error(`POST /vms/request returned unexpected status ${createResp.status()} in vm-lifecycle setup`);
}

async function ensureRealVMForLifecycle(request: APIRequestContext): Promise<string> {
    const headers = await getAdminHeaders(request);
    const existing = await listVMBriefs(request, headers);
    const existingReal = existing.find((vm) => !isLikelySeededVM(vm));
    if (existingReal?.id) {
        return existingReal.id;
    }

    const approvalsResp = await request.get('/api/v1/approvals?page=1&per_page=100', { headers });
    expect(approvalsResp.status(), `GET /approvals returned ${approvalsResp.status()}`).toBe(200);
    const approvals = await validateApiResponse('ApprovalTicketList', approvalsResp) as {
        items?: Array<{ id?: string; status?: string; operation_type?: string; operationType?: string }>;
    };
    const pendingCreate = (approvals.items ?? []).find((item) =>
        Boolean(item.id) &&
        (item.id ?? '').startsWith('tkt-batch-') &&
        (item.status ?? '').toUpperCase() === 'PENDING' &&
        ((item.operation_type ?? item.operationType ?? '').toUpperCase() === 'CREATE')
    );
    const ticketID = pendingCreate?.id ?? await submitOrReusePendingCreateTicket(request, headers);
    expect(ticketID, 'VM lifecycle setup requires a pending CREATE ticket').toBeTruthy();

    const clusterID = await resolveApprovalClusterID(request, headers);
    expect(clusterID, 'VM lifecycle setup requires at least one enabled cluster').toBeTruthy();

    const approveResp = await request.post(`/api/v1/approvals/${ticketID}/approve`, {
        headers,
        data: { selected_cluster_id: clusterID },
    });
    expect(
        [204, 409],
        `POST /approvals/${ticketID}/approve returned ${approveResp.status()} during vm-lifecycle setup`
    ).toContain(approveResp.status());

    await expect.poll(async () => {
        const vms = await listVMBriefs(request, headers);
        return vms.filter((vm) => !isLikelySeededVM(vm)).length;
    }, {
        timeout: 120_000,
        intervals: [1000, 2000, 4000, 8000],
        message: 'vm-lifecycle setup did not produce a non-seeded VM after approval',
    }).toBeGreaterThan(0);

    const real = await pickNonSeededVM(request, headers);
    return real.id;
}

// ── Helpers: get first VM ID from list ───────────────────────────────────────

async function getFirstVMId(page: Page): Promise<string> {
    const listRespPromise = page.waitForResponse(
        (r) => urlPathIncludes(r.url(), '/api/v1/vms') && r.request().method() === 'GET' && !urlPathIncludes(r.url(), '/vms/')
    );
    const [listResp] = await Promise.all([
        listRespPromise,
        page.goto('/vms'),
    ]);
    expect(listResp.status(), `GET /vms returned ${listResp.status()}`).toBe(200);
    // validateApiResponse returns the parsed body — reuse it to avoid double-read
    // (Playwright response body can only be consumed once; a second .json() call
    //  may throw "Protocol error: No resource with given identifier found").
    const body = await validateApiResponse('VMList', listResp) as { items?: Array<{ id?: string }> };
    await expect(page.getByRole('heading', { name: 'Virtual Machines' })).toBeVisible();
    const items = body.items ?? [];
    expect(items.length, 'No VMs exist — seed data must include at least one VM').toBeGreaterThan(0);
    const preferred = items.find((item) => item.id && !isSeededVMID(item.id))?.id ?? '';
    const id = preferred || (items[0]?.id ?? '');
    expect(id, 'First VM has no id field').toBeTruthy();
    return id;
}

// ── Test suite ────────────────────────────────────────────────────────────────

test.describe('vm-lifecycle live (contract-enforced, no mock, no skip)', () => {
    test.beforeAll(async ({ request }) => {
        const setup = await ensureBatchSubmitPolicyForUser(request, {
            username: e2eUsername,
            primaryPassword: e2ePassword,
            secondaryPassword: e2eNewPassword,
            currentPasswordHint: activePassword,
            reasonPrefix: 'vm-lifecycle live',
        });
        activePassword = setup.password;
        lifecycleVMID = await ensureRealVMForLifecycle(request);
    });

    test.beforeEach(async ({ page }) => {
        await page.addInitScript(() => { window.open = () => null; });
        await login(page);
    });

    // ── getVM: GET /vms/{id} → VM ─────────────────────────────────────────────

    test('getVM – GET /vms/{id} conforms to VM schema', async ({ page }) => {
        // operationId: getVM
        const vmId = await getFirstVMId(page);

        const getRespPromise = page.waitForResponse(
            (r) => urlPathIncludes(r.url(), `/api/v1/vms/${vmId}`) && r.request().method() === 'GET'
                && !urlPathIncludes(r.url(), '/console') && !urlPathIncludes(r.url(), '/vnc')
        );
        // Navigate to VM detail page (triggers GET /vms/{id})
        await page.getByTestId(`vm-action-detail-${vmId}`).click();
        await expectSchema(getRespPromise, 'VM', 200);
    });

    // ── startVM: POST /vms/{id}/start → 202 ──────────────────────────────────

    test('startVM – POST /vms/{id}/start returns 202', async ({ page, request }) => {
        // operationId: startVM
        const headers = await getAdminHeaders(request);
        const vmId = await ensureVMReadyForAction(request, headers, 'start', lifecycleVMID);
        await page.goto(`/vms/${vmId}`);
        await expect(page.locator('body')).toBeVisible();

        const startRespPromise = page.waitForResponse(
            (r) => urlPathEndsWith(r.url(), `/api/v1/vms/${vmId}/start`) && r.request().method() === 'POST'
        );
        const startStatusPromise = fetchStatusWithStoredToken(page, `/api/v1/vms/${vmId}/start`, 'POST');
        const [startResp, startStatus] = await Promise.all([startRespPromise, startStatusPromise]);
        expect(startStatus, `POST /vms/${vmId}/start returned ${startStatus}`).toBe(202);
        expect(startResp.status(), `POST /vms/${vmId}/start returned ${startResp.status()}`).toBe(202);
        // spec: 202 has no response body schema — just verify status
    });

    // ── stopVM: POST /vms/{id}/stop → 202 ────────────────────────────────────

    test('stopVM – POST /vms/{id}/stop returns 202', async ({ page, request }) => {
        // operationId: stopVM
        const headers = await getAdminHeaders(request);
        const vmId = await ensureVMReadyForAction(request, headers, 'stop', lifecycleVMID);
        await page.goto(`/vms/${vmId}`);
        await expect(page.locator('body')).toBeVisible();

        const stopRespPromise = page.waitForResponse(
            (r) => urlPathEndsWith(r.url(), `/api/v1/vms/${vmId}/stop`) && r.request().method() === 'POST'
        );
        const stopStatusPromise = fetchStatusWithStoredToken(page, `/api/v1/vms/${vmId}/stop`, 'POST');
        const [stopResp, stopStatus] = await Promise.all([stopRespPromise, stopStatusPromise]);
        expect(stopStatus, `POST /vms/${vmId}/stop returned ${stopStatus}`).toBe(202);
        expect(stopResp.status(), `POST /vms/${vmId}/stop returned ${stopResp.status()}`).toBe(202);
    });

    // ── restartVM: POST /vms/{id}/restart → 202 ──────────────────────────────

    test('restartVM – POST /vms/{id}/restart returns 202', async ({ page, request }) => {
        // operationId: restartVM
        const headers = await getAdminHeaders(request);
        const vmId = await ensureVMReadyForAction(request, headers, 'restart', lifecycleVMID);
        await page.goto(`/vms/${vmId}`);
        await expect(page.locator('body')).toBeVisible();

        const restartRespPromise = page.waitForResponse(
            (r) => urlPathEndsWith(r.url(), `/api/v1/vms/${vmId}/restart`) && r.request().method() === 'POST'
        );
        const restartStatusPromise = fetchStatusWithStoredToken(page, `/api/v1/vms/${vmId}/restart`, 'POST');
        const [restartResp, restartStatus] = await Promise.all([restartRespPromise, restartStatusPromise]);
        expect(restartStatus, `POST /vms/${vmId}/restart returned ${restartStatus}`).toBe(202);
        expect(restartResp.status(), `POST /vms/${vmId}/restart returned ${restartResp.status()}`).toBe(202);
    });

    // ── powerVM: POST /vms/{id}/power → VMPowerAcceptedResponse ───────────────

    test('powerVM – POST /vms/{id}/power conforms to VMPowerAcceptedResponse schema (202)', async ({ page, request }) => {
        // operationId: powerVM — uses the generic /power endpoint from VM detail page
        const headers = await getAdminHeaders(request);
        const vmId = await ensureVMReadyForAction(request, headers, 'start', lifecycleVMID);
        const action: ActionName = 'start';

        // Navigate to VM detail page where power buttons use POST /vms/{id}/power
        await page.goto(`/vms/${vmId}`);
        await expect(page.locator('body')).toBeVisible();

        // The detail page power buttons call POST /vms/{vm_id}/power with action body
        const powerRespPromise = page.waitForResponse(
            (r) => urlPathEndsWith(r.url(), `/api/v1/vms/${vmId}/power`) && r.request().method() === 'POST'
        );

        const powerBtn = page.getByTestId(`vm-action-${action}-${vmId}`);
        await expect(powerBtn).toBeEnabled();
        await powerBtn.click();

        // Confirm if confirmation dialog appears
        const confirmBtn = page.locator('.ant-popover:visible, .ant-modal-content:visible')
            .getByRole('button', { name: /confirm|ok|yes/i }).first();
        if (await confirmBtn.count() > 0) await confirmBtn.click();

        // ── CONTRACT CHECK: VMPowerAcceptedResponse schema ────────────────────────
        const powerResp = await powerRespPromise;
        expect(powerResp.status(), `POST /vms/${vmId}/power returned ${powerResp.status()}`).toBe(202);
        await validateApiResponse('VMPowerAcceptedResponse', powerResp);
    });

    // ── deleteVM: DELETE /vms/{id} → DeleteVMResponse ────────────────────────

    test('deleteVM – DELETE /vms/{id} conforms to DeleteVMResponse schema', async ({ page, request }) => {
        // operationId: deleteVM
        const headers = await getAdminHeaders(request);
        const vmId = await ensureVMReadyForAction(request, headers, 'start', lifecycleVMID);
        const vmName = (await listVMBriefs(request, headers)).find((vm) => vm.id === vmId)?.name?.trim() ?? '';
        expect(vmName, `Could not resolve VM name for ${vmId}`).toBeTruthy();
        await page.goto(`/vms/${vmId}`);
        await expect(page.locator('body')).toBeVisible();
        const button = page.getByTestId(`vm-action-delete-${vmId}`);
        await expect(button, `Delete action button not found on VM detail page for ${vmId}`).toBeVisible();

        const deleteRespPromise = page.waitForResponse(
            (r) => urlPathIncludes(r.url(), `/api/v1/vms/${vmId}`) && r.request().method() === 'DELETE'
        );
        await button.click();

        // Fill confirm_name guard (ADR-0015 §13)
        const deleteModal = page.locator('.ant-modal-content').filter({ hasText: /delete/i }).last();
        await expect(deleteModal).toBeVisible();
        const confirmInput = deleteModal.getByRole('textbox').first();
        if (await confirmInput.count() > 0) {
            await confirmInput.fill(vmName);
        }
        await deleteModal.getByRole('button', { name: /delete|confirm|ok/i }).last().click();

        // ── CONTRACT CHECK: strict success path (must enqueue deletion) ───────────
        const deleteResp = await deleteRespPromise;
        expect(deleteResp.status(), `DELETE /vms/${vmId} returned unexpected ${deleteResp.status()}`).toBe(202);
        await validateApiResponse('DeleteVMResponse', deleteResp);
    });

    // ── listVMBatches: GET /vms/batch → VMBatchList ─────────────────────────

    test('listVMBatches – GET /vms/batch conforms to VMBatchList schema', async ({ page }) => {
        // operationId: listVMBatches
        const listRespPromise = page.waitForResponse(
            (r) => urlPathIncludes(r.url(), '/api/v1/vms/batch') && r.request().method() === 'GET'
                && !urlPathIncludes(r.url(), '/vms/batch/') // Exclude batch/{id} detail
        );
        const [listResp] = await Promise.all([
            listRespPromise,
            page.goto('/vms/batch'),
        ]);
        await expect(page.locator('body')).toBeVisible();
        // ── CONTRACT CHECK: VMBatchList schema ────────────────────────────────────
        expect(listResp.status()).toBe(200);
        await validateApiResponse('VMBatchList', listResp);
    });

    // ── submitVMBatch: POST /vms/batch → VMBatchSubmitResponse ────────────────

    test('submitVMBatch – POST /vms/batch conforms to VMBatchSubmitResponse schema', async ({ page, request }) => {
        // operationId: submitVMBatch
        let batchID = '';

        await test.step('Stage 5.E / Step 1: select VM targets from list page', async () => {
            await page.goto('/vms');
            await expect(page.getByRole('heading', { name: 'Virtual Machines' })).toBeVisible();

            const rowCheckbox = page.locator('tbody input[type="checkbox"]').first();
            await expect(rowCheckbox, 'No VM row checkbox found for batch action').toBeVisible();
            await rowCheckbox.check();
        });

        await test.step('Stage 5.E / Step 2-5: submit batch request and validate accepted payload', async () => {
            const batchRespPromise = page.waitForResponse(
                (r) => urlPathEndsWith(r.url(), '/api/v1/vms/batch') && r.request().method() === 'POST'
            );

            // Stage 5.E on list page exposes explicit action buttons (Start/Stop/Restart/Delete Selected).
            const batchBtn = page.getByRole('button', { name: /delete selected/i }).first();
            await expect(batchBtn, 'Delete Selected batch button not found in VM list').toBeVisible();
            await batchBtn.click();

            const batchResp = await batchRespPromise;
            expect(batchResp.status(), `POST /vms/batch returned ${batchResp.status()}`).toBe(202);
            const submitBody = await validateApiResponse('VMBatchSubmitResponse', batchResp) as {
                batch_id?: string;
                status?: string;
                status_url?: string;
                retry_after_seconds?: unknown;
            };

            batchID = String(submitBody.batch_id ?? '').trim();
            const statusURL = String(submitBody.status_url ?? '').trim();
            expect(batchID, 'submitVMBatch must return batch_id').toBeTruthy();
            expect(
                String(submitBody.status ?? '').toUpperCase(),
                'canonical /vms/batch submit must enter approval queue'
            ).toBe('PENDING_APPROVAL');
            expect(statusURL, 'submitVMBatch must return canonical status_url').toBe(`/api/v1/vms/batch/${batchID}`);
            expect(Number(submitBody.retry_after_seconds), 'retry_after_seconds must be positive').toBeGreaterThan(0);
        });

        await test.step('Stage 5.E / Step 6: follow status_url and validate parent-child aggregate view', async () => {
            const headers = await getAdminHeaders(request);
            const detailResp = await request.get(`/api/v1/vms/batch/${batchID}`, { headers });
            expect(detailResp.status(), `GET /vms/batch/${batchID} returned ${detailResp.status()}`).toBe(200);
            const detailBody = await validateApiResponse('VMBatchStatusResponse', detailResp) as {
                batch_id?: string;
                operation?: string;
                child_count?: unknown;
                success_count?: unknown;
                failed_count?: unknown;
                pending_count?: unknown;
            };
            expect(detailBody.batch_id).toBe(batchID);
            expect(String(detailBody.operation ?? '').toUpperCase(), 'Delete Selected should map to DELETE batch op').toBe('DELETE');
            const childCount = Number(detailBody.child_count ?? 0);
            const successCount = Number(detailBody.success_count ?? 0);
            const failedCount = Number(detailBody.failed_count ?? 0);
            const pendingCount = Number(detailBody.pending_count ?? 0);
            expect(childCount, 'batch child_count must be positive').toBeGreaterThan(0);
            expect(successCount + failedCount + pendingCount).toBe(childCount);

            await page.goto('/vms/batch');
            await expect(page.getByTestId(`batch-action-detail-${batchID}`)).toBeVisible();
        });
    });

    // ── getVMBatch: GET /vms/batch/{id} → VMBatchStatusResponse ──────────────

    test('getVMBatch – GET /vms/batch/{id} conforms to VMBatchStatusResponse schema', async ({ page }) => {
        // operationId: getVMBatch
        const listRespPromise = page.waitForResponse(
            (r) => urlPathEndsWith(r.url(), '/api/v1/vms/batch') && r.request().method() === 'GET'
        );
        const [listResp] = await Promise.all([
            listRespPromise,
            page.goto('/vms/batch'),
        ]);
        await expect(page.locator('body')).toBeVisible();
        expect(listResp.status(), `GET /vms/batch returned ${listResp.status()}`).toBe(200);
        const listBody = await validateApiResponse('VMBatchList', listResp) as { items?: Array<{ id?: string }> };
        const batchId = listBody.items?.find((item) => Boolean(item.id))?.id ?? '';
        expect(batchId, 'No VM batch found — seed data must include at least one batch').toBeTruthy();

        // ── CONTRACT CHECK: VMBatchStatusResponse schema ──────────────────────────
        const statusRespPromise = page.waitForResponse(
            (r) => urlPathIncludes(r.url(), `/api/v1/vms/batch/${batchId}`) && r.request().method() === 'GET'
        );
        await page.getByTestId(`batch-action-detail-${batchId}`).click();
        await expectSchema(statusRespPromise, 'VMBatchStatusResponse', 200);
    });

    // ── retryVMBatch: POST /vms/batch/{id}/retry → VMBatchActionResponse ─────

    test('retryVMBatch – POST /vms/batch/{id}/retry conforms to VMBatchActionResponse schema', async ({ page }) => {
        // operationId: retryVMBatch
        await page.goto('/vms/batch');
        await expect(page.locator('body')).toBeVisible();

        const retryBtn = page.locator('button[data-testid^="batch-action-retry-"]:not([disabled])').first();
        await expect(retryBtn, 'No retryable batch found — seed data must include FAILED or PARTIAL_SUCCESS batch').toBeVisible();
        const retryTestId = await retryBtn.getAttribute('data-testid');
        const batchId = retryTestId?.replace('batch-action-retry-', '') ?? '';
        expect(batchId, 'Could not extract batch_id from retry button').toBeTruthy();

        const retryRespPromise = page.waitForResponse(
            (r) => urlPathEndsWith(r.url(), `/api/v1/vms/batch/${batchId}/retry`) && r.request().method() === 'POST'
        );
        await retryBtn.click();
        const confirmBtn = page.locator('.ant-popover:visible, .ant-modal-content:visible')
            .getByRole('button', { name: /confirm|ok|retry/i }).first();
        if (await confirmBtn.count() > 0) await confirmBtn.click();

        // ── CONTRACT CHECK: VMBatchActionResponse schema ──────────────────────────
        const { body: retryBody } = await expectSchema(retryRespPromise, 'VMBatchActionResponse', 200);
        const affected = Number((retryBody as { affected_count?: number }).affected_count ?? 0);
        expect(
            affected,
            'Retry must affect at least one failed child ticket (master-flow Stage 5.E)'
        ).toBeGreaterThan(0);
    });

    // ── cancelVMBatch: POST /vms/batch/{id}/cancel → VMBatchActionResponse ───

    test('cancelVMBatch – POST /vms/batch/{id}/cancel conforms to VMBatchActionResponse schema', async ({ page }) => {
        // operationId: cancelVMBatch
        await page.goto('/vms/batch');
        await expect(page.locator('body')).toBeVisible();

        const cancelBtn = page.locator('button[data-testid^="batch-action-cancel-"]:not([disabled])').first();
        await expect(cancelBtn, 'No cancellable batch found — seed data must include PENDING_APPROVAL or IN_PROGRESS batch').toBeVisible();
        const cancelTestId = await cancelBtn.getAttribute('data-testid');
        const batchId = cancelTestId?.replace('batch-action-cancel-', '') ?? '';
        expect(batchId, 'Could not extract batch_id from cancel button').toBeTruthy();

        const cancelRespPromise = page.waitForResponse(
            (r) => urlPathEndsWith(r.url(), `/api/v1/vms/batch/${batchId}/cancel`) && r.request().method() === 'POST'
        );
        await cancelBtn.click();
        const confirmBtn = page.locator('.ant-popover:visible, .ant-modal-content:visible')
            .getByRole('button', { name: /confirm|ok|cancel/i }).first();
        if (await confirmBtn.count() > 0) await confirmBtn.click();

        // ── CONTRACT CHECK: VMBatchActionResponse schema ──────────────────────────
        const { body: cancelBody } = await expectSchema(cancelRespPromise, 'VMBatchActionResponse', 200);
        const affected = Number((cancelBody as { affected_count?: number }).affected_count ?? 0);
        expect(
            affected,
            'Cancel must affect at least one pending child ticket (master-flow Stage 5.E)'
        ).toBeGreaterThan(0);
    });

    // ── getVMConsoleStatus: GET /vms/{id}/console/status → VMConsoleStatusResponse

    test('getVMConsoleStatus – RUNNING VM returns VMConsoleStatusResponse (200)', async ({ page, request }) => {
        // operationId: getVMConsoleStatus
        const headers = await getAdminHeaders(request);
        const vmId = await ensureVMReadyForAction(request, headers, 'stop', lifecycleVMID || undefined);

        // Navigate to VM detail which should show console status
        await page.goto(`/vms/${vmId}`);
        await expect(page.locator('body')).toBeVisible();

        // Trigger console status check explicitly and wait for the exact response.
        const statusRespPromise = page.waitForResponse(
            (r) => urlPathEndsWith(r.url(), `/api/v1/vms/${vmId}/console/status`) && r.request().method() === 'GET'
        );
        const statusCode = await fetchStatusWithStoredToken(page, `/api/v1/vms/${vmId}/console/status`, 'GET');
        const statusResp = await statusRespPromise;
        expect(statusCode, `GET /vms/${vmId}/console/status returned ${statusCode}`).toBe(200);
        expect(statusResp.status(), `GET /vms/${vmId}/console/status returned ${statusResp.status()}`).toBe(200);
        await validateApiResponse('VMConsoleStatusResponse', statusResp);
    });

    test('getVMConsoleStatus – non-RUNNING VM returns conflict Error (409)', async ({ page, request }) => {
        // operationId: getVMConsoleStatus
        const headers = await getAdminHeaders(request);
        const vmId = await ensureVMReadyForAction(request, headers, 'start', lifecycleVMID || undefined);

        await page.goto(`/vms/${vmId}`);
        await expect(page.locator('body')).toBeVisible();

        const statusRespPromise = page.waitForResponse(
            (r) => urlPathEndsWith(r.url(), `/api/v1/vms/${vmId}/console/status`) && r.request().method() === 'GET'
        );
        const statusCode = await fetchStatusWithStoredToken(page, `/api/v1/vms/${vmId}/console/status`, 'GET');
        const statusResp = await statusRespPromise;
        expect(statusCode, `GET /vms/${vmId}/console/status returned ${statusCode}`).toBe(409);
        expect(statusResp.status(), `GET /vms/${vmId}/console/status returned ${statusResp.status()}`).toBe(409);
        await validateApiResponse('Error', statusResp);
    });
});
