/**
 * Admin Flow Live E2E Tests — Contract-Enforced (no mock)
 *
 * ┌─────────────────────────────────────────────────────────────────────────┐
 * │  REQUIRES: a running backend (db + server via docker-compose or local)  │
 * │  Every API response is validated against api/openapi.yaml schema.       │
 * │  Schema mismatch = CI failure = frontend/backend contract broken.       │
 * │  NO test.skip() — failures expose real frontend/backend problems.       │
 * └─────────────────────────────────────────────────────────────────────────┘
 *
 * Coverage map (master-flow.md) — operationId index:
 *   listRoles              Stage 2.A+ – GET /admin/roles
 *   createRole             Stage 2.A+ – POST /admin/roles
 *   deleteRole             Stage 2.A+ – DELETE /admin/roles/{id}
 *   listUsers              Stage 2 (Supplemental) – GET /admin/users
 *   createUser             Stage 2 (Supplemental) – POST /admin/users
 *   deleteUser             Stage 2 (Supplemental) – DELETE /admin/users/{id}
 *   listAuthProviderTypes  Stage 2.B  – GET /admin/auth-provider-types
 *   listAuthProviders      Stage 2.B  – GET /admin/auth-providers
 *   createAuthProvider     Stage 2.B  – POST /admin/auth-providers
 *   deleteAuthProvider     Stage 2.B  – DELETE /admin/auth-providers/{id}
 *   listAuthProviderGroupMappings Stage 2.C – GET /admin/auth-providers/{id}/group-mappings
 *   listClusters           Stage 3    – GET /admin/clusters
 *   createCluster          Stage 3    – POST /admin/clusters
 *   listNamespaces         Stage 3    – GET /admin/namespaces
 *   createNamespace        Stage 3    – POST /admin/namespaces
 *   updateNamespace        Stage 3    – PUT /admin/namespaces/{id}
 *   deleteNamespace        Stage 3    – DELETE /admin/namespaces/{id}
 *   listAdminTemplates     Stage 3    – GET /admin/templates
 *   createAdminTemplate    Stage 3    – POST /admin/templates
 *   listAdminInstanceSizes Stage 3    – GET /admin/instance-sizes
 *   createAdminInstanceSize Stage 3   – POST /admin/instance-sizes
 *   listApprovals          Stage 5.B  – GET /approvals
 *   approveTicket          Stage 5.B  – POST /approvals/{id}/approve
 *   rejectTicket           Stage 5.B  – POST /approvals/{id}/reject
 *   listAuditLogs          Audit      – GET /audit-logs
 *
 * Environment variables:
 *   E2E_USERNAME        – admin username (default: e2e-admin)
 *   E2E_PASSWORD        – admin password (default: e2e-admin-123)
 *   E2E_NEW_PASSWORD    – password used when force_password_change=true
 *   E2E_KUBECONFIG_B64  – base64-encoded kubeconfig for cluster registration test
 *
 * Run:
 *   PW_BASE_URL=http://localhost:3000 npx playwright test admin-flow-live
 */

import { expect, test, type APIRequestContext, type Page, type Response } from '@playwright/test';
import { validateApiResponse } from './lib/schema-validator';
import {
    fetchStatusWithStoredToken,
    getAntModal,
    getApiTokenWithForcePasswordSupport,
    loginWithForcePasswordSupport,
    selectAntOption,
    urlPathEndsWith,
    urlPathIncludes,
} from './lib/helpers';

// ── Config ────────────────────────────────────────────────────────────────────

const e2eUsername = process.env.E2E_USERNAME ?? 'e2e-admin';
const e2ePassword = process.env.E2E_PASSWORD ?? 'e2e-admin-123';
const e2eNewPassword = process.env.E2E_NEW_PASSWORD ?? (e2ePassword === 'admin' ? 'admin123' : `${e2ePassword}-new`);
const e2eKubeconfigB64 = process.env.E2E_KUBECONFIG_B64 ?? 'dGVzdC1rdWJlY29uZmlnLWJhc2U2NA==';
const e2eClusterName = process.env.E2E_CLUSTER ?? 'e2e-cluster';
const e2eSystemName = process.env.E2E_SYSTEM ?? 'e2e-system';
const e2eTemplateName = process.env.E2E_TEMPLATE ?? 'e2e-template';
const e2eSizeName = process.env.E2E_SIZE ?? 'e2e-small';
const e2eNamespace = process.env.E2E_NAMESPACE ?? 'e2e-test';
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

// ── Helper ────────────────────────────────────────────────────────────────────

async function expectSchema(
    respPromise: Promise<Response>,
    schemaName: string,
    expectedStatus: number | number[] = 200
): Promise<{ body: unknown; resp: Response }> {
    const resp = await respPromise;
    const statuses = Array.isArray(expectedStatus) ? expectedStatus : [expectedStatus];
    expect(statuses).toContain(resp.status());
    const body = await validateApiResponse(schemaName, resp);
    return { body, resp };
}

async function getAdminToken(request: APIRequestContext): Promise<string> {
    const auth = await getApiTokenWithForcePasswordSupport(request, {
        username: e2eUsername,
        primaryPassword: e2ePassword,
        secondaryPassword: e2eNewPassword,
        currentPasswordHint: activePassword,
    });
    activePassword = auth.password;
    return auth.token;
}

function pickIDByPreferredName<T extends { id?: string; name?: string }>(
    items: T[] | undefined,
    preferredName: string
): string {
    const preferred = (items ?? []).find((item) => (item.name ?? '').trim() === preferredName && Boolean(item.id));
    if (preferred?.id) {
        return preferred.id;
    }
    return (items ?? []).find((item) => Boolean(item.id))?.id ?? '';
}

function pickPreferredNamespace(namespaces: string[] | undefined, preferredName: string): string {
    return namespaces?.find((ns) => ns === preferredName) ?? namespaces?.[0] ?? preferredName;
}

function escapeRegExp(input: string): string {
    return input.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
}

function toLooseOptionFilter(rawName: string): RegExp {
    const tokens = rawName
        .trim()
        .split(/[\s_-]+/)
        .filter(Boolean)
        .map(escapeRegExp);
    if (tokens.length === 0) {
        return /.*/;
    }
    return new RegExp(tokens.join('\\s*[-_ ]*\\s*'), 'i');
}

async function resolveClusterOptionFilter(
    request: APIRequestContext,
    headers: { Authorization: string }
): Promise<RegExp> {
    const clustersResp = await request.get('/api/v1/admin/clusters?page=1&per_page=100', { headers });
    expect(clustersResp.status(), `GET /admin/clusters returned ${clustersResp.status()}`).toBe(200);
    const clustersBody = await validateApiResponse('ClusterList', clustersResp) as {
        items?: Array<{ id?: string; name?: string; display_name?: string; displayName?: string }>;
    };
    const clusters = clustersBody.items ?? [];
    expect(clusters.length, 'Stage 5.B requires at least one cluster option').toBeGreaterThan(0);

    const preferred =
        clusters.find((item) => (item.name ?? '').trim() === e2eClusterName) ??
        clusters[0];
    const label = String((preferred.display_name ?? preferred.displayName ?? preferred.name ?? '')).trim();
    expect(label, 'Cluster option label cannot be empty').toBeTruthy();
    return toLooseOptionFilter(label);
}

async function listVMBriefs(
    request: APIRequestContext,
    headers: { Authorization: string }
): Promise<Array<{ id: string; status: string }>> {
    const vmMap = new Map<string, { id: string; status: string }>();
    let page = 1;
    let totalPages = 1;
    do {
        const vmResp = await request.get(`/api/v1/vms?page=${page}&per_page=100`, { headers });
        expect(vmResp.status(), `GET /vms?page=${page} returned ${vmResp.status()}`).toBe(200);
        const vmBody = await validateApiResponse('VMList', vmResp) as {
            items?: Array<{ id?: string; status?: string }>;
            pagination?: { total_pages?: number };
        };
        for (const item of vmBody.items ?? []) {
            if (!item.id) {
                continue;
            }
            vmMap.set(item.id, { id: item.id, status: (item.status ?? '').toUpperCase() });
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
    expected: 'APPROVED' | 'REJECTED'
): Promise<void> {
    const expectedPattern = expected === 'APPROVED'
        ? /^(APPROVED|EXECUTING|SUCCESS)$/
        : /^REJECTED$/;

    await expect.poll(async () => {
        let page = 1;
        const perPage = 100;

        for (let guard = 0; guard < 20; guard += 1) {
            const listResp = await request.get(`/api/v1/approvals?page=${page}&per_page=${perPage}`, { headers });
            expect(listResp.status(), `GET /approvals returned ${listResp.status()}`).toBe(200);
            const listBody = await validateApiResponse('ApprovalTicketList', listResp) as {
                items?: Array<{ id?: string; status?: string }>;
                pagination?: { total_pages?: number };
            };
            const found = listBody.items?.find((item) => item.id === ticketID);
            if (found) {
                return (found.status ?? '').toUpperCase();
            }
            const totalPages = Number(listBody.pagination?.total_pages ?? 1);
            if (!Number.isFinite(totalPages) || page >= totalPages) {
                break;
            }
            page += 1;
        }

        return '';
    }, {
        timeout: 30_000,
        intervals: [500, 1000, 2000],
        message: `Ticket ${ticketID} did not match status pattern ${expectedPattern}`,
    }).toMatch(expectedPattern);
}

async function waitForApprovalNotification(
    request: APIRequestContext,
    headers: { Authorization: string },
    ticketID: string,
    expectedType: 'APPROVAL_COMPLETED' | 'APPROVAL_REJECTED'
): Promise<void> {
    await expect.poll(async () => {
        let page = 1;
        const perPage = 100;

        for (let guard = 0; guard < 20; guard += 1) {
            const listResp = await request.get(`/api/v1/notifications?page=${page}&per_page=${perPage}`, { headers });
            expect(listResp.status(), `GET /notifications returned ${listResp.status()}`).toBe(200);
            const listBody = await validateApiResponse('NotificationList', listResp) as {
                items?: Array<{ type?: string; resource_id?: string; resourceId?: string }>;
                pagination?: { total_pages?: number };
            };

            const found = (listBody.items ?? []).some((item) => {
                const type = (item.type ?? '').toUpperCase();
                const resourceID = String(item.resource_id ?? item.resourceId ?? '').trim();
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
    }, {
        timeout: 30_000,
        intervals: [1000, 2000, 3000],
        message: `Missing notification ${expectedType} for ticket ${ticketID}`,
    }).toBe(true);
}

async function waitForNewVMFromApproval(
    request: APIRequestContext,
    headers: { Authorization: string },
    baselineIDs: Set<string>
): Promise<string> {
    let createdVMID = '';
    await expect.poll(async () => {
        const vms = await listVMBriefs(request, headers);
        const created = vms.find((vm) => !baselineIDs.has(vm.id));
        createdVMID = created?.id ?? '';
        return createdVMID;
    }, {
        timeout: 60_000,
        intervals: [1000, 2000, 3000],
        message: 'Stage 5.C requires approved ticket to materialize a VM row',
    }).not.toBe('');
    return createdVMID;
}

async function summarizeClusterHealth(
    request: APIRequestContext,
    headers: { Authorization: string }
): Promise<string> {
    const clustersResp = await request.get('/api/v1/admin/clusters?page=1&per_page=100', { headers });
    if (clustersResp.status() !== 200) {
        return `clusters_http_${clustersResp.status()}`;
    }
    const clustersBody = await validateApiResponse('ClusterList', clustersResp) as {
        items?: Array<{ id?: string; name?: string; status?: string; enabled?: boolean }>;
    };
    const summary = (clustersBody.items ?? [])
        .map((cluster) => {
            const name = cluster.name ?? cluster.id ?? 'unknown';
            const status = (cluster.status ?? 'UNKNOWN').toUpperCase();
            const enabledSuffix = cluster.enabled === false ? ':DISABLED' : '';
            return `${name}:${status}${enabledSuffix}`;
        })
        .join(', ');
    return summary || 'no-clusters';
}

async function waitForVMExecutionOutcome(
    request: APIRequestContext,
    headers: { Authorization: string },
    vmID: string
): Promise<void> {
    await expect.poll(async () => {
        const vmResp = await request.get(`/api/v1/vms/${vmID}`, { headers });
        if (vmResp.status() !== 200) {
            return `HTTP_${vmResp.status()}`;
        }
        const vmBody = await validateApiResponse('VM', vmResp) as { status?: string };
        const status = (vmBody.status ?? '').toUpperCase();
        if (status === 'CREATING') {
            const health = await summarizeClusterHealth(request, headers);
            return `CREATING|${health}`;
        }
        return status;
    }, {
        timeout: 120_000,
        intervals: [1000, 2000, 4000, 8000],
        message: `VM ${vmID} did not reach RUNNING/FAILED from CREATING`,
    }).toMatch(/^(RUNNING|FAILED)$/);
}

async function seedPendingApprovalTicket(request: APIRequestContext): Promise<string> {
    const token = await getAdminToken(request);
    const headers = { Authorization: `Bearer ${token}` };

    const systemsResp = await request.get('/api/v1/systems', { headers });
    expect(systemsResp.status(), 'GET /systems must return 200 for approval seed').toBe(200);
    const systems = await validateApiResponse('SystemList', systemsResp) as { items?: Array<{ id?: string; name?: string }> };
    const systemId = pickIDByPreferredName(systems.items, e2eSystemName);
    expect(systemId, 'Approval seed requires at least one existing system').toBeTruthy();

    const approvalSeedServiceName = `s5b-${Date.now().toString(36).slice(-8)}`;
    const createServiceResp = await request.post(`/api/v1/systems/${systemId}/services`, {
        headers,
        data: {
            name: approvalSeedServiceName,
            description: 'temporary service for Stage 5.B approval seed isolation',
        },
    });
    expect(
        createServiceResp.status(),
        `POST /systems/{id}/services returned ${createServiceResp.status()} for Stage 5.B approval seed`
    ).toBe(201);
    await validateApiResponse('Service', createServiceResp);

    const [servicesResp, contextResp] = await Promise.all([
        request.get(`/api/v1/systems/${systemId}/services`, { headers }),
        request.get('/api/v1/vms/request-context', { headers }),
    ]);
    expect(servicesResp.status(), 'GET /systems/{id}/services must return 200 for approval seed').toBe(200);
    expect(contextResp.status(), 'GET /vms/request-context must return 200 for approval seed').toBe(200);

    const services = await validateApiResponse('ServiceList', servicesResp) as { items?: Array<{ id?: string; name?: string }> };
    const ctx = await validateApiResponse('VMRequestContext', contextResp) as {
        templates?: Array<{ id?: string; name?: string }>;
        instance_sizes?: Array<{ id?: string; name?: string }>;
        namespaces?: string[];
    };

    const serviceId = pickIDByPreferredName(services.items, approvalSeedServiceName);
    const templateId = pickIDByPreferredName(ctx.templates, e2eTemplateName);
    const sizeId = pickIDByPreferredName(ctx.instance_sizes, e2eSizeName);
    const namespace = pickPreferredNamespace(ctx.namespaces, e2eNamespace);
    expect(serviceId, 'Approval seed requires an existing service').toBeTruthy();
    expect(templateId, 'Approval seed requires an existing template').toBeTruthy();
    expect(sizeId, 'Approval seed requires an existing instance size').toBeTruthy();

    const createReqData = {
        service_id: serviceId,
        template_id: templateId,
        instance_size_id: sizeId,
        namespace,
    };

    const createResp = await request.post('/api/v1/vms/request', {
        headers,
        data: { ...createReqData, reason: `admin-flow approval seed ${Date.now()}` },
    });
    if (createResp.status() === 400) {
        const errBody = await validateApiResponse('Error', createResp) as {
            code?: string;
            message?: string;
            params?: Record<string, unknown>;
        };
        if (errBody.code === 'DUPLICATE_PENDING_REQUEST') {
            const existingTicketID =
                typeof errBody.params?.existing_ticket_id === 'string' ? errBody.params.existing_ticket_id.trim() : '';
            throw new Error(
                `unexpected DUPLICATE_PENDING_REQUEST in approval seed (service=${approvalSeedServiceName}, existing_ticket_id=${existingTicketID || 'unknown'})`
            );
        }
        throw new Error(
            `POST /vms/request failed in approval seed: ${errBody.code ?? 'UNKNOWN'} (${errBody.message ?? 'no message'})`
        );
    }

    expect(createResp.status(), 'POST /vms/request must return 202 for approval seed').toBe(202);
    const ticket = await validateApiResponse('ApprovalTicketResponse', createResp) as { ticket_id?: string; id?: string };
    const ticketID = ticket.ticket_id ?? ticket.id ?? '';
    expect(ticketID, 'ApprovalTicketResponse missing ticket id').toBeTruthy();
    return ticketID;
}

// ── Test suite ────────────────────────────────────────────────────────────────

test.describe('admin-flow live (contract-enforced, no mock)', () => {
    test.beforeEach(async ({ page }) => {
        await login(page);
    });

    // ── Stage 2.A+: RBAC custom role lifecycle ───────────────────────────────────

    test('Stage 2.A+ – listRoles + createRole + deleteRole (schema-validated)', async ({ page }) => {
        // operationId: listRoles, createRole, deleteRole
        const roleName = `e2e-role-${Date.now().toString(36).slice(-6)}`;
        let roleID = '';

        await test.step('Stage 2.A+ / Step 1: create custom role from role management page', async () => {
            // ── CONTRACT CHECK: listRoles → RoleList ──────────────────────────────
            const listRespPromise = page.waitForResponse(
                (r) => urlPathEndsWith(r.url(), '/api/v1/admin/roles') && r.request().method() === 'GET'
            );
            await page.goto('/admin/rbac');
            await expect(page.getByTestId('admin-rbac-page')).toBeVisible();
            await expectSchema(listRespPromise, 'RoleList', 200);

            // ── CONTRACT CHECK: createRole → Role ─────────────────────────────────
            const createRespPromise = page.waitForResponse(
                (r) => urlPathEndsWith(r.url(), '/api/v1/admin/roles') && r.request().method() === 'POST'
            );

            await page.getByTestId('rbac-role-create-button').click();
            const createModal = getAntModal(page, 'rbac-role-create-modal');
            await expect(createModal).toBeVisible();
            await createModal.getByRole('textbox').first().fill(roleName);
            await selectAntOption(page, createModal.locator('.ant-select-selector').first());
            await createModal.getByRole('button', { name: 'OK' }).click();

            const { body: created } = await expectSchema(createRespPromise, 'Role', 201);
            roleID = (created as { id?: string }).id ?? '';
            expect(roleID).toBeTruthy();
            await expect(page.locator('tr').filter({ hasText: roleName }).first()).toBeVisible();
        });

        await test.step('Stage 2.A+ / Step 2: role permission binding persists and role is deletable', async () => {
            // ── CONTRACT CHECK: deleteRole → 204 ──────────────────────────────────
            const deleteRespPromise = page.waitForResponse(
                (r) => urlPathIncludes(r.url(), `/api/v1/admin/roles/${roleID}`) && r.request().method() === 'DELETE'
            );
            await page.getByTestId(`rbac-role-action-delete-${roleID}`).click();
            const confirmBtn = page.getByRole('button', { name: /ok|confirm|delete/i }).last();
            await expect(confirmBtn).toBeVisible();
            await confirmBtn.click();

            const deleteResp = await deleteRespPromise;
            expect(deleteResp.status()).toBe(204);
            await expect(page.locator('tr').filter({ hasText: roleName })).toHaveCount(0);
        });
    });

    // ── Stage 2 (Supplemental): User management lifecycle ────────────────────────

    test('Stage 2 (Supplemental) – listUsers + createUser + deleteUser (schema-validated)', async ({ page }) => {
        // operationId: listUsers, createUser, deleteUser
        // ── CONTRACT CHECK: listUsers → UserList ──────────────────────────────────
        const listRespPromise = page.waitForResponse(
            (r) => urlPathEndsWith(r.url(), '/api/v1/admin/users') && r.request().method() === 'GET'
        );
        await page.goto('/admin/users');
        await expect(page.getByTestId('admin-users-page')).toBeVisible();
        await expectSchema(listRespPromise, 'UserList', 200);

        // ── CONTRACT CHECK: createUser → User ─────────────────────────────────────
        const username = `e2eu${Date.now().toString(36).slice(-6)}`;
        const createRespPromise = page.waitForResponse(
            (r) => urlPathEndsWith(r.url(), '/api/v1/admin/users') && r.request().method() === 'POST'
        );

        await page.getByTestId('user-create-button').click();
        const createModal = getAntModal(page, 'user-create-modal');
        await expect(createModal).toBeVisible();
        await createModal.getByRole('textbox').nth(0).fill(username);
        await createModal.locator('input[type="password"]').fill('Secure@Pass123');
        await createModal.getByRole('button', { name: 'OK' }).click();

        const { body: created } = await expectSchema(createRespPromise, 'User', 201);
        const userID = (created as { id?: string }).id ?? '';
        expect(userID).toBeTruthy();

        await expect(page.locator('tr').filter({ hasText: username }).first()).toBeVisible();

        // ── CONTRACT CHECK: deleteUser → 204 ──────────────────────────────────────
        const deleteRespPromise = page.waitForResponse(
            (r) => urlPathIncludes(r.url(), `/api/v1/admin/users/${userID}`) && r.request().method() === 'DELETE'
        );
        await page.getByTestId(`user-action-delete-${userID}`).click();
        const confirmBtn = page.getByRole('button', { name: /confirm/i }).last();
        await expect(confirmBtn).toBeVisible();
        await confirmBtn.click();

        const deleteResp = await deleteRespPromise;
        expect(deleteResp.status()).toBe(204);
        await expect(page.locator('tr').filter({ hasText: username })).toHaveCount(0);
    });

    // ── Stage 2.B: Auth Provider lifecycle ───────────────────────────────────────

    test('Stage 2.B – listAuthProviderTypes + listAuthProviders + createAuthProvider + deleteAuthProvider (schema-validated)', async ({ page }) => {
        // operationId: listAuthProviderTypes, listAuthProviders, createAuthProvider, deleteAuthProvider
        // ── CONTRACT CHECK: listAuthProviderTypes → AuthProviderTypeList ──────────
        const typesRespPromise = page.waitForResponse(
            (r) => urlPathEndsWith(r.url(), '/api/v1/admin/auth-provider-types') && r.request().method() === 'GET'
        );
        await page.goto('/admin/auth-providers');
        await expect(page.getByRole('heading', { name: 'Authentication Providers' })).toBeVisible();

        const { body: typesBody } = await expectSchema(typesRespPromise, 'AuthProviderTypeList', 200);
        const typesPayload = typesBody as { items?: Array<{ type?: string; display_name?: string }> };
        expect(Array.isArray(typesPayload.items)).toBeTruthy();
        expect((typesPayload.items ?? []).length).toBeGreaterThan(0);

        const preferredType =
            (typesPayload.items ?? []).find((item) => item.type === 'oidc') ??
            (typesPayload.items ?? []).find((item) => item.type === 'generic') ??
            typesPayload.items?.[0];
        expect(preferredType?.type).toBeTruthy();
        const providerTypeLabel = preferredType?.display_name ?? preferredType?.type ?? '';

        // ── CONTRACT CHECK: listAuthProviders → AuthProviderList ──────────────────
        const listRespPromise = page.waitForResponse(
            (r) => urlPathEndsWith(r.url(), '/api/v1/admin/auth-providers') && r.request().method() === 'GET'
        );
        await page.reload();
        await expectSchema(listRespPromise, 'AuthProviderList', 200);

        // ── CONTRACT CHECK: createAuthProvider → AuthProvider ─────────────────────
        const providerName = `e2e-auth-${Date.now().toString(36).slice(-6)}`;
        const createRespPromise = page.waitForResponse(
            (r) => urlPathEndsWith(r.url(), '/api/v1/admin/auth-providers') && r.request().method() === 'POST'
        );

        await page.getByTestId('auth-provider-create-button').click();
        const createModal = getAntModal(page, 'auth-provider-create-modal');
        await expect(createModal).toBeVisible();
        await createModal.getByRole('textbox').first().fill(providerName);
        await selectAntOption(page, createModal.locator('.ant-select-selector').first(), providerTypeLabel);
        const configText = '{"test_endpoint":"https://idp.example.com/healthz"}';
        const configInput = createModal.locator('textarea').first();
        await configInput.fill(configText);
        await expect(configInput).toHaveValue(configText);
        await createModal.getByRole('button', { name: 'OK' }).click();

        const { body: created } = await expectSchema(createRespPromise, 'AuthProvider', 201);
        const providerID = (created as { id?: string }).id ?? '';
        expect(providerID).toBeTruthy();

        await expect(page.locator('tr').filter({ hasText: providerName }).first()).toBeVisible();

        // ── CONTRACT CHECK: deleteAuthProvider → 204 ──────────────────────────────
        await page.getByTestId(`auth-provider-action-delete-${providerID}`).click();
        const deleteRespPromise = page.waitForResponse(
            (r) =>
                urlPathIncludes(r.url(), `/api/v1/admin/auth-providers/${providerID}`) &&
                r.request().method() === 'DELETE'
        );
        const deleteModal = getAntModal(page, 'auth-provider-delete-modal');
        await expect(deleteModal).toBeVisible();
        await deleteModal.getByRole('button', { name: 'OK' }).click();

        const deleteResp = await deleteRespPromise;
        expect(deleteResp.status()).toBe(204);
        await expect(page.locator('tr').filter({ hasText: providerName })).toHaveCount(0);
    });

    // ── Stage 2.C: IdP Group Mapping ─────────────────────────────────────────────

    test('Stage 2.C – listAuthProviderGroupMappings: IdP group mapping list conforms to IdPGroupMappingList schema', async ({ page }) => {
        // operationId: listAuthProviderGroupMappings
        await test.step('Stage 2.C / Step 1: fetch provider group-mapping sample list', async () => {
            // Contract: seed data MUST include at least one auth provider.
            await page.goto('/admin/auth-providers');
            await expect(page.getByRole('heading', { name: 'Authentication Providers' })).toBeVisible();

            const firstProviderRow = page.locator('tr[data-row-key]').first();
            await expect(firstProviderRow, 'No auth providers found — seed data must include at least one provider').toBeVisible();

            // Click into the provider's group mappings
            const mappingLink = firstProviderRow.locator('[data-testid^="auth-provider-action-mappings-"]').first();
            await expect(mappingLink, 'No group mapping action found on provider row — UI must expose this action').toBeVisible();

            const providerID = (await mappingLink.getAttribute('data-testid'))
                ?.replace('auth-provider-action-mappings-', '') ?? '';

            const mappingsRespPromise = page.waitForResponse(
                (r) =>
                    urlPathIncludes(r.url(), `/api/v1/admin/auth-providers/${providerID}/group-mappings`) &&
                    r.request().method() === 'GET'
            );
            await mappingLink.click();

            // ── CONTRACT CHECK: listAuthProviderGroupMappings → IdPGroupMappingList ─
            const mappingsResp = await mappingsRespPromise;
            expect(mappingsResp.status()).toBe(200);
            await validateApiResponse('IdPGroupMappingList', mappingsResp);
        });
    });

    // ── Stage 3: Namespace management lifecycle ───────────────────────────────────

    test('Stage 3 – listNamespaces + createNamespace + updateNamespace + deleteNamespace (schema-validated)', async ({ page }) => {
        // operationId: listNamespaces, createNamespace, updateNamespace, deleteNamespace
        await test.step('Stage 3 / Step 2: configure namespace registry with full lifecycle checks', async () => {
            // ── CONTRACT CHECK: listNamespaces → NamespaceRegistryList ───────────
            const listRespPromise = page.waitForResponse(
                (r) => urlPathEndsWith(r.url(), '/api/v1/admin/namespaces') && r.request().method() === 'GET'
            );
            await page.goto('/admin/namespaces');
            await expect(page.getByTestId('admin-namespaces-page')).toBeVisible();
            await expectSchema(listRespPromise, 'NamespaceRegistryList', 200);

            // ── CONTRACT CHECK: createNamespace → NamespaceRegistry ──────────────
            const nsName = `e2e-ns-${Date.now().toString(36).slice(-6)}`;
            const createRespPromise = page.waitForResponse(
                (r) => urlPathEndsWith(r.url(), '/api/v1/admin/namespaces') && r.request().method() === 'POST'
            );

            await page.getByTestId('namespace-create-button').click();
            const createModal = getAntModal(page, 'namespace-create-modal');
            await expect(createModal).toBeVisible();
            await createModal.getByRole('textbox').first().fill(nsName);
            await selectAntOption(page, createModal.locator('.ant-select-selector').first(), /test/i);
            await createModal.getByRole('button', { name: 'OK' }).click();

            const { body: created } = await expectSchema(createRespPromise, 'NamespaceRegistry', 201);
            const nsID = (created as { id?: string }).id ?? '';
            expect(nsID).toBeTruthy();

            await expect(page.locator('tr').filter({ hasText: nsName }).first()).toBeVisible();

            // ── CONTRACT CHECK: updateNamespace → NamespaceRegistry ──────────────
            // Note: OpenAPI spec uses PUT for updateNamespace
            const updateRespPromise = page.waitForResponse(
                (r) =>
                    urlPathIncludes(r.url(), `/api/v1/admin/namespaces/${nsID}`) &&
                    (r.request().method() === 'PUT' || r.request().method() === 'PATCH')
            );
            await page.getByTestId(`namespace-action-edit-${nsID}`).click();
            const editModal = getAntModal(page, 'namespace-edit-modal');
            await expect(editModal).toBeVisible();
            await editModal.locator('textarea').first().fill('Updated by live e2e test');
            await editModal.getByRole('button', { name: 'OK' }).click();

            await expectSchema(updateRespPromise, 'NamespaceRegistry', 200);

            // ── CONTRACT CHECK: deleteNamespace with confirm_name guard ───────────
            await page.getByTestId(`namespace-action-delete-${nsID}`).click();
            const deleteModal = getAntModal(page, 'namespace-delete-modal');
            await expect(deleteModal).toBeVisible();

            const deleteBtn = deleteModal.getByRole('button', { name: /delete/i });
            await expect(deleteBtn).toBeDisabled();

            await page.getByTestId('namespace-delete-confirm-input').fill(nsName);
            await expect(deleteBtn).toBeEnabled();

            const deleteRespPromise = page.waitForResponse(
                (r) =>
                    urlPathIncludes(r.url(), `/api/v1/admin/namespaces/${nsID}`) &&
                    r.request().method() === 'DELETE'
            );
            await deleteBtn.click();

            const deleteResp = await deleteRespPromise;
            expect(deleteResp.status()).toBe(204);
            await expect(page.locator('tr').filter({ hasText: nsName })).toHaveCount(0);
        });
    });

    // ── Stage 3: Cluster registration ────────────────────────────────────────────

    test('Stage 3 – listClusters: cluster list conforms to ClusterList schema', async ({ page }) => {
        // operationId: listClusters
        const listRespPromise = page.waitForResponse(
            (r) => urlPathEndsWith(r.url(), '/api/v1/admin/clusters') && r.request().method() === 'GET'
        );
        await page.goto('/admin/clusters');
        await expect(page.getByTestId('admin-clusters-page')).toBeVisible();
        // ── CONTRACT CHECK: listClusters → ClusterList ────────────────────────────
        await expectSchema(listRespPromise, 'ClusterList', 200);
    });

    test('Stage 3 – createCluster: cluster create must succeed with valid kubeconfig', async ({ page }) => {
        // operationId: createCluster
        await test.step('Stage 3 / Step 1: register cluster and verify auto-detection contract', async () => {
            await page.goto('/admin/clusters');
            await expect(page.getByTestId('admin-clusters-page')).toBeVisible();

            const clusterName = `e2e-cluster-${Date.now().toString(36).slice(-6)}`;
            const createRespPromise = page.waitForResponse(
                (r) => urlPathEndsWith(r.url(), '/api/v1/admin/clusters') && r.request().method() === 'POST'
            );

            await page.getByTestId('cluster-create-button').click();
            const createModal = getAntModal(page, 'cluster-create-modal');
            await expect(createModal).toBeVisible();
            await createModal.getByRole('textbox').first().fill(clusterName);
            await createModal.locator('textarea').last().fill(e2eKubeconfigB64);
            await createModal.getByRole('button', { name: 'OK' }).click();

            // ── CONTRACT CHECK: strict success path (must create cluster) ──────────
            const createResp = await createRespPromise;
            expect(createResp.status(), `POST /admin/clusters returned ${createResp.status()}`).toBe(201);
            await validateApiResponse('Cluster', createResp);
        });
    });

    // ── Stage 3: Template management ─────────────────────────────────────────────

    test('Stage 3 – listAdminTemplates + createAdminTemplate (schema-validated)', async ({ page }) => {
        // operationId: listAdminTemplates, createAdminTemplate
        await test.step('Stage 3 / Step 3: configure template catalog entry', async () => {
            // ── CONTRACT CHECK: listAdminTemplates → TemplateList ──────────────────
            const listRespPromise = page.waitForResponse(
                (r) => urlPathEndsWith(r.url(), '/api/v1/admin/templates') && r.request().method() === 'GET'
            );
            await page.goto('/admin/templates');
            await expect(page.getByRole('heading', { name: 'Templates' })).toBeVisible();
            await expectSchema(listRespPromise, 'TemplateList', 200);

            // ── CONTRACT CHECK: createAdminTemplate → Template ─────────────────────
            const templateName = `e2e-tpl-${Date.now().toString(36).slice(-6)}`;
            const createRespPromise = page.waitForResponse(
                (r) => urlPathEndsWith(r.url(), '/api/v1/admin/templates') && r.request().method() === 'POST'
            );

            await page.getByTestId('template-create-button').click();
            const createModal = getAntModal(page, 'template-create-modal');
            await expect(createModal).toBeVisible();
            await createModal.getByRole('textbox').first().fill(templateName);
            await createModal.getByLabel(/container image url/i).fill('quay.io/containerdisks/ubuntu:22.04');
            await createModal.getByRole('button', { name: 'OK' }).click();

            const createResp = await createRespPromise;
            expect(createResp.status(), `POST /admin/templates returned ${createResp.status()}`).toBe(201);
            await validateApiResponse('Template', createResp);
        });
    });

    // ── Stage 3: Instance Size management ────────────────────────────────────────

    test('Stage 3 – listAdminInstanceSizes + createAdminInstanceSize (schema-validated)', async ({ page }) => {
        // operationId: listAdminInstanceSizes, createAdminInstanceSize
        await test.step('Stage 3 / Step 4: create instance size from schema-driven form', async () => {
            // ── CONTRACT CHECK: listAdminInstanceSizes → InstanceSizeList ──────────
            const listRespPromise = page.waitForResponse(
                (r) => urlPathEndsWith(r.url(), '/api/v1/admin/instance-sizes') && r.request().method() === 'GET'
            );
            await page.goto('/admin/instance-sizes');
            await expect(page.getByRole('heading', { name: 'Instance Sizes' })).toBeVisible();
            await expectSchema(listRespPromise, 'InstanceSizeList', 200);

            // ── CONTRACT CHECK: createAdminInstanceSize → InstanceSize ─────────────
            const sizeName = `e2e-size-${Date.now().toString(36).slice(-6)}`;
            const createRespPromise = page.waitForResponse(
                (r) => urlPathEndsWith(r.url(), '/api/v1/admin/instance-sizes') && r.request().method() === 'POST'
            );

            await page.getByTestId('instance-size-create-button').click();
            const createModal = getAntModal(page, 'instance-size-create-modal');
            await expect(createModal).toBeVisible();
            await createModal.getByRole('textbox').first().fill(sizeName);
            // Fill CPU and memory fields using exact stable IDs.
            await createModal.locator('#cpu_cores').fill('4');
            await createModal.locator('#memory_gi').fill('4');
            await createModal.getByRole('button', { name: 'OK' }).click();

            const createResp = await createRespPromise;
            expect(createResp.status(), `POST /admin/instance-sizes returned ${createResp.status()}`).toBe(201);
            await validateApiResponse('InstanceSize', createResp);
        });
    });

    // ── Stage 5.B: Approval workflow ─────────────────────────────────────────────

    test('Stage 5.B – listApprovals: approval list conforms to ApprovalTicketList schema', async ({ page }) => {
        // operationId: listApprovals
        const listRespPromise = page.waitForResponse(
            (r) => urlPathIncludes(r.url(), '/api/v1/approvals') && r.request().method() === 'GET'
        );
        await page.goto('/admin/approvals');
        await expect(page.getByRole('heading', { name: /approval/i })).toBeVisible();

        // ── CONTRACT CHECK: listApprovals → ApprovalTicketList ───────────────────
        await expectSchema(listRespPromise, 'ApprovalTicketList', 200);
    });

    test('Stage 5.B – approveTicket: approve action calls real API', async ({ page, request }) => {
        // operationId: approveTicket
        const ticketID = await seedPendingApprovalTicket(request);
        const token = await getAdminToken(request);
        const headers = { Authorization: `Bearer ${token}` };
        const beforeVMs = await listVMBriefs(request, headers);
        const beforeVMIDs = new Set(beforeVMs.map((item) => item.id));

        await page.goto('/admin/approvals');
        await expect(page.getByRole('heading', { name: /approval/i })).toBeVisible();

        const approveBtn = page.getByTestId(`approval-action-approve-${ticketID}`);
        await expect(approveBtn, 'No pending approval tickets found — API setup may have failed').toBeVisible();

        await approveBtn.click();
        const modal = getAntModal(page, 'approve-modal');
        await expect(modal).toBeVisible();
    await expect(
        modal.locator('.ant-select-selector').first(),
        'Stage 5.B requires selecting a target cluster before approval'
    ).toBeVisible();
    const clusterFilter = await resolveClusterOptionFilter(request, headers);
    await selectAntOption(page, modal.locator('.ant-select-selector').first(), clusterFilter);

        const approveRespPromise = page.waitForResponse(
            (r) =>
                urlPathEndsWith(r.url(), `/api/v1/approvals/${ticketID}/approve`) &&
                r.request().method() === 'POST'
        );
        await modal.getByRole('button', { name: 'OK' }).click();

        // ── CONTRACT CHECK: approveTicket → 204 ──────────────────────────────────
        const approveResp = await approveRespPromise;
        expect(approveResp.status(), `POST /approvals/{id}/approve returned ${approveResp.status()}`).toBe(204);
        await waitForTicketStatus(request, headers, ticketID, 'APPROVED');
        await waitForApprovalNotification(request, headers, ticketID, 'APPROVAL_COMPLETED');

        // ── MASTER-FLOW Stage 5.C CHECK: approval must materialize VM + execute ──
        const createdVMID = await waitForNewVMFromApproval(request, headers, beforeVMIDs);
        await waitForVMExecutionOutcome(request, headers, createdVMID);
    });

    // ── Stage 5.B: Reject action ──────────────────────────────────────────────────

    test('Stage 5.B – rejectTicket: reject action calls real API', async ({ page, request }) => {
        // operationId: rejectTicket
        const ticketID = await seedPendingApprovalTicket(request);

        await page.goto('/admin/approvals');
        await expect(page.getByRole('heading', { name: /approval/i })).toBeVisible();

        const rejectBtn = page.getByTestId(`approval-action-reject-${ticketID}`);
        await expect(rejectBtn, 'No pending approval tickets found — API setup may have failed').toBeVisible();

        const rejectRespPromise = page.waitForResponse(
            (r) =>
                urlPathEndsWith(r.url(), `/api/v1/approvals/${ticketID}/reject`) &&
                r.request().method() === 'POST'
        );

        await rejectBtn.click();
        const modal = getAntModal(page, 'reject-modal');
        await expect(modal).toBeVisible();
        await modal.locator('textarea').first().fill('Rejected by live e2e test');
        await modal.getByRole('button', { name: 'OK' }).click();

        // ── CONTRACT CHECK: rejectTicket → 204 ───────────────────────────────────
        const rejectResp = await rejectRespPromise;
        expect(rejectResp.status(), `POST /approvals/{id}/reject returned ${rejectResp.status()}`).toBe(204);
        const token = await getAdminToken(request);
        const headers = { Authorization: `Bearer ${token}` };
        await waitForTicketStatus(request, headers, ticketID, 'REJECTED');
        await waitForApprovalNotification(request, headers, ticketID, 'APPROVAL_REJECTED');
    });

    // ── Audit Log ─────────────────────────────────────────────────────────────────

    test('listAuditLogs: audit log list conforms to AuditLogList schema', async ({ page }) => {
        // operationId: listAuditLogs
        const auditRespPromise = page.waitForResponse(
            (r) => urlPathEndsWith(r.url(), '/api/v1/audit-logs') && r.request().method() === 'GET'
        );

        await page.goto('/admin/audit');
        // Accept either a dedicated page or a section within admin
        await expect(page.locator('body')).toBeVisible();
        // Trigger explicit request to guarantee contract coverage even if page load
        // does not automatically request audit logs.
        const auditStatus = await fetchStatusWithStoredToken(page, '/api/v1/audit-logs', 'GET');
        expect(auditStatus).toBe(200);

        // ── CONTRACT CHECK: listAuditLogs → AuditLogList ──────────────────────────
        const auditResp = await auditRespPromise;
        expect(auditResp.status()).toBe(200);
        await validateApiResponse('AuditLogList', auditResp);
    });
});
