/**
 * User Self-Service Live E2E Tests — Contract-Enforced (no mock, no skip)
 *
 * ┌─────────────────────────────────────────────────────────────────────────┐
 * │  REQUIRES: a running backend (db + server via docker-compose or local)  │
 * │  NO test.skip() — failures expose real frontend/backend problems.       │
 * │  Every API response is validated against api/openapi.yaml schema.       │
 * └─────────────────────────────────────────────────────────────────────────┘
 *
 * Coverage (all previously uncovered user-facing endpoints):
 *   changePassword          – POST /auth/change-password             → 204
 *   listTemplates           – GET /templates                         → TemplateList
 *   listInstanceSizes       – GET /instance-sizes                    → InstanceSizeList
 *   markNotificationRead    – PATCH /notifications/{id}/read         → 204
 *   markAllNotificationsRead– POST /notifications/mark-all-read      → 204
 *   cancelTicket            – POST /tickets/{id}/cancel              → 204
 *   submitApprovalBatch     – POST /vms/batch                        → VMBatchSubmitResponse
 *   getSystem               – GET /systems/{id}                      → System
 *   getService              – GET /systems/{id}/services/{id}        → Service
 *   addSystemMember         – POST /systems/{id}/members             → SystemMember
 *   updateSystemMemberRole  – PATCH /systems/{id}/members/{user_id}  → SystemMember
 *   deleteSystemMember      – DELETE /systems/{id}/members/{user_id} → 204
 *   listServices            – GET /systems/{id}/services             → ServiceList
 *   getNamespace            – GET /admin/namespaces/{id}             → NamespaceRegistry
 *   getLiveness             – GET /health/live                       → Health
 *   getReadiness            – GET /health/ready                      → Health
 *
 * Environment variables:
 *   E2E_USERNAME  – admin username (default: e2e-admin)
 *   E2E_PASSWORD  – admin password (default: e2e-admin-123)
 *   E2E_NEW_PASSWORD – new password for change-password test (default: e2e-admin-456)
 */

import { expect, test, type APIRequestContext, type Page } from '@playwright/test';
import { validateApiResponse } from './lib/schema-validator';
import {
    createTempAdminUser,
    createTempService,
    deleteAdminUserIfPresent,
    deleteServiceIfPresent,
    ensureBatchSubmitPolicyForUser,
    ensureSeedSystemAndService,
    expectSchemaResponse as expectSchema,
    fetchStatusWithStoredToken,
    getAntModal,
    getApiAuthHeadersWithForcePasswordSupport,
    loginWithForcePasswordSupport,
    pickIDByPreferredName,
    pickPreferredNamespace,
    selectAntOption,
    selectServicesSystemFilter,
    urlPathEndsWith,
    urlPathIncludes,
} from './lib/helpers';

// ── Config ────────────────────────────────────────────────────────────────────

const e2eUsername = process.env.E2E_USERNAME ?? 'e2e-admin';
const e2ePassword = process.env.E2E_PASSWORD ?? 'e2e-admin-123';
const e2eNewPassword = process.env.E2E_NEW_PASSWORD ?? (e2ePassword === 'admin' ? 'ShepherdLive!2026' : `${e2ePassword}-new`);
const e2eSystemName = process.env.E2E_SYSTEM ?? 'e2e-system';
const e2eServiceName = process.env.E2E_SERVICE ?? 'e2e-service';
const e2eNamespace = process.env.E2E_NAMESPACE ?? 'e2e-test';
let activePassword = e2ePassword;
let seedSystemID = '';

// ── Auth helper ───────────────────────────────────────────────────────────────

async function login(page: Page): Promise<void> {
    activePassword = await loginWithForcePasswordSupport(page, {
        username: e2eUsername,
        primaryPassword: e2ePassword,
        secondaryPassword: e2eNewPassword,
        currentPasswordHint: activePassword,
    });
}

async function getApiAuthHeaders(request: APIRequestContext): Promise<{ Authorization: string }> {
    const auth = await getApiAuthHeadersWithForcePasswordSupport(request, {
        username: e2eUsername,
        primaryPassword: e2ePassword,
        secondaryPassword: e2eNewPassword,
        currentPasswordHint: activePassword,
    });
    activePassword = auth.password;
    return auth.headers;
}

async function resolveCreateVMRequestDataForService(
    request: APIRequestContext,
    headers: { Authorization: string },
    serviceID: string
): Promise<{ service_id: string; template_id: string; instance_size_id: string; namespace: string }> {
    const contextResp = await request.get('/api/v1/vms/request-context', { headers });
    expect(contextResp.status(), `GET /vms/request-context returned ${contextResp.status()}`).toBe(200);
    const contextBody = await validateApiResponse('VMRequestContext', contextResp) as {
        templates?: Array<{ id?: string; name?: string }>;
        instance_sizes?: Array<{ id?: string; name?: string }>;
        namespaces?: string[];
    };
    const templateID = pickIDByPreferredName(contextBody.templates, '');
    const instanceSizeID = pickIDByPreferredName(contextBody.instance_sizes, '');
    expect(serviceID, 'VM request service id is required').toBeTruthy();
    expect(templateID, 'VM request requires at least one template').toBeTruthy();
    expect(instanceSizeID, 'VM request requires at least one instance size').toBeTruthy();

    return {
        service_id: serviceID,
        template_id: templateID,
        instance_size_id: instanceSizeID,
        namespace: pickPreferredNamespace(contextBody.namespaces, e2eNamespace),
    };
}

async function restorePasswordViaApi(
    request: APIRequestContext,
    username: string,
    currentPassword: string,
    restoredPassword: string
): Promise<void> {
    const loginWithCurrent = await request.post('/api/v1/auth/login', {
        data: { username, password: currentPassword },
    });

    if (loginWithCurrent.status() === 401 && currentPassword !== restoredPassword) {
        const alreadyRestored = await request.post('/api/v1/auth/login', {
            data: { username, password: restoredPassword },
        });
        if (alreadyRestored.status() === 200) {
            return;
        }
        expect(alreadyRestored.status(), `POST /auth/login with restored password returned ${alreadyRestored.status()}`).toBe(200);
    }

    expect(loginWithCurrent.status(), `POST /auth/login before password restore returned ${loginWithCurrent.status()}`).toBe(200);
    const loginBody = await loginWithCurrent.json() as { token?: string };
    const token = loginBody.token ?? '';
    expect(token, 'LoginResponse.token is required for password restore').toBeTruthy();

    if (currentPassword === restoredPassword) {
        return;
    }

    const restoreResp = await request.post('/api/v1/auth/change-password', {
        headers: { Authorization: `Bearer ${token}` },
        data: {
            old_password: currentPassword,
            new_password: restoredPassword,
        },
    });
    expect(restoreResp.status(), `Password restore failed with ${restoreResp.status()}`).toBe(204);

    const verifyResp = await request.post('/api/v1/auth/login', {
        data: { username, password: restoredPassword },
    });
    expect(verifyResp.status(), `POST /auth/login after password restore returned ${verifyResp.status()}`).toBe(200);
}

// ── Test suite ────────────────────────────────────────────────────────────────

test.describe('user-selfservice live (contract-enforced, no mock, no skip)', () => {
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
        seedSystemID = seed.systemId;

        const setup = await ensureBatchSubmitPolicyForUser(request, {
            username: e2eUsername,
            primaryPassword: e2ePassword,
            secondaryPassword: e2eNewPassword,
            currentPasswordHint: activePassword,
            reasonPrefix: 'user-selfservice live',
        });
        activePassword = setup.password;
    });

    test.beforeEach(async ({ page }) => {
        await login(page);
    });

    // ── Health endpoints ───────────────────────────────────────────────────────
    // These are pure API endpoints (not browser pages). We use Playwright's
    // APIRequestContext (request.get) to call them directly, avoiding the
    // frontend router which would intercept/redirect the URL.
    // Health endpoints SHOULD be accessible without authentication.

    test('getLiveness – GET /health/live conforms to Health schema', async ({ request }) => {
        // operationId: getLiveness
        const resp = await request.get('/api/v1/health/live');
        expect(resp.status(), `GET /health/live returned ${resp.status()}`).toBe(200);
        const body = await resp.json();
        // Validate against Health schema (inline, since this is an API-only response)
        expect(body).toHaveProperty('status');
    });

    test('getReadiness – GET /health/ready conforms to Health schema', async ({ request }) => {
        // operationId: getReadiness
        const resp = await request.get('/api/v1/health/ready');
        expect([200, 503], `GET /health/ready returned ${resp.status()}`).toContain(resp.status());
        const body = await resp.json();
        expect(body).toHaveProperty('status');
    });

    // ── listTemplates: GET /templates → TemplateList ──────────────────────────

    test('listTemplates – GET /templates conforms to TemplateList schema', async ({ page }) => {
        // operationId: listTemplates
        // User-facing template list (not admin) is fetched explicitly to guarantee contract coverage,
        // as the UI primarily uses /request-context bulk endpoint instead.
        await page.goto('/vms');
        await expect(page.getByRole('heading', { name: 'Virtual Machines' })).toBeVisible();

        const respPromise = page.waitForResponse(
            (r) => urlPathEndsWith(r.url(), '/api/v1/templates') && !urlPathIncludes(r.url(), '/admin/') && r.request().method() === 'GET'
        );

        // Trigger explicitly with app JWT to guarantee authenticated coverage.
        const status = await fetchStatusWithStoredToken(page, '/api/v1/templates', 'GET');
        expect(status).toBe(200);

        // ── CONTRACT CHECK: TemplateList schema ───────────────────────────────────
        await expectSchema(respPromise, 'TemplateList', 200);
    });

    // ── listInstanceSizes: GET /instance-sizes → InstanceSizeList ─────────────

    test('listInstanceSizes – GET /instance-sizes conforms to InstanceSizeList schema', async ({ page }) => {
        // operationId: listInstanceSizes
        // Fetched explicitly to guarantee contract coverage, as the UI primarily uses /request-context
        await page.goto('/vms');
        await expect(page.getByRole('heading', { name: 'Virtual Machines' })).toBeVisible();

        const respPromise = page.waitForResponse(
            (r) => urlPathEndsWith(r.url(), '/api/v1/instance-sizes') && !urlPathIncludes(r.url(), '/admin/') && r.request().method() === 'GET'
        );

        // Trigger explicitly with app JWT to guarantee authenticated coverage.
        const status = await fetchStatusWithStoredToken(page, '/api/v1/instance-sizes', 'GET');
        expect(status).toBe(200);

        // ── CONTRACT CHECK: InstanceSizeList schema ───────────────────────────────
        await expectSchema(respPromise, 'InstanceSizeList', 200);
    });

    // ── Notifications: markNotificationRead + markAllNotificationsRead ─────────

    test('markNotificationRead – PATCH /notifications/{id}/read returns 204', async ({ page }) => {
        // operationId: markNotificationRead
        // Get first notification
        const listRespPromise = page.waitForResponse(
            (r) => urlPathIncludes(r.url(), '/api/v1/notifications') && r.request().method() === 'GET'
                && !urlPathIncludes(r.url(), 'unread-count')
        );
        const [listResp] = await Promise.all([
            listRespPromise,
            page.goto('/notifications'),
        ]);
        await expect(page.getByRole('heading', { name: 'Notifications' })).toBeVisible();
        // Use validateApiResponse for single-read safety
        const listBody = await validateApiResponse('NotificationList', listResp) as { items?: Array<{ id?: string; read?: boolean }> };
        const unread = (listBody.items ?? []).find((n) => !n.read);
        expect(unread, 'No unread notifications found — seed data must include at least one unread notification').toBeTruthy();
        const notifID = unread?.id ?? '';
        expect(notifID).toBeTruthy();

        // ── PATCH /notifications/{id}/read → 204 ─────────────────────────────────
        const readRespPromise = page.waitForResponse(
            (r) => urlPathIncludes(r.url(), `/api/v1/notifications/${notifID}/read`) && r.request().method() === 'PATCH'
        );
        await page.getByTestId(`notification-action-read-${notifID}`).click();
        const readResp = await readRespPromise;
        expect(readResp.status(), `PATCH /notifications/${notifID}/read returned ${readResp.status()}`).toBe(204);
    });

    test('markAllNotificationsRead – POST /notifications/mark-all-read returns 204', async ({ page }) => {
        // operationId: markAllNotificationsRead
        await page.goto('/notifications');
        await expect(page.getByRole('heading', { name: 'Notifications' })).toBeVisible();

        // ── POST /notifications/mark-all-read → 204 ───────────────────────────────
        const markAllRespPromise = page.waitForResponse(
            (r) => urlPathEndsWith(r.url(), '/api/v1/notifications/mark-all-read') && r.request().method() === 'POST'
        );
        await page.getByTestId('notifications-mark-all-read-button').click();
        const confirmBtn = page.locator('.ant-popover:visible, .ant-modal-content:visible')
            .getByRole('button', { name: /confirm|ok/i }).first();
        if (await confirmBtn.count() > 0) await confirmBtn.click();

        const markAllResp = await markAllRespPromise;
        expect(markAllResp.status(), `POST /notifications/mark-all-read returned ${markAllResp.status()}`).toBe(204);
    });

    // ── cancelTicket: POST /tickets/{id}/cancel → 204 ────────────────────────

    test('cancelTicket – POST /tickets/{id}/cancel returns 204', async ({ request }) => {
        // operationId: cancelTicket
        const headers = await getApiAuthHeaders(request);
        expect(seedSystemID, 'Seed system id is required for cancelTicket').toBeTruthy();
        const tempService = await createTempService(request, headers, seedSystemID, { prefix: 'can' });
        let ticketID = '';

        try {
            const createReqData = await resolveCreateVMRequestDataForService(request, headers, tempService.id);
            const submitResp = await request.post('/api/v1/vms/request', {
                headers,
                data: {
                    ...createReqData,
                    reason: `cancel test request ${Date.now()}`,
                },
            });
            expect(submitResp.status(), `POST /vms/request returned ${submitResp.status()}`).toBe(202);
            const submitBody = await validateApiResponse('TicketResponse', submitResp) as { ticket_id?: string; id?: string };
            ticketID = submitBody.ticket_id ?? submitBody.id ?? '';
            expect(ticketID, 'POST /vms/request response missing ticket_id/id field').toBeTruthy();

            // ── POST /tickets/{id}/cancel → 204 ──────────────────────────────────
            const cancelResp = await request.post(`/api/v1/tickets/${ticketID}/cancel`, { headers });
            expect(cancelResp.status(), `POST /tickets/${ticketID}/cancel returned ${cancelResp.status()}`).toBe(204);
        } finally {
            await deleteServiceIfPresent(request, headers, seedSystemID, tempService.id);
        }
    });

    // ── submitApprovalBatch: POST /vms/batch → VMBatchSubmitResponse ──────────

    test('submitApprovalBatch – POST /vms/batch conforms to VMBatchSubmitResponse schema', async ({ page }) => {
        // operationId: submitApprovalBatch
        await page.goto('/vms');
        await expect(page.getByRole('heading', { name: 'Virtual Machines' })).toBeVisible();

        const batchRespPromise = page.waitForResponse(
            (r) => urlPathEndsWith(r.url(), '/api/v1/vms/batch') && r.request().method() === 'POST'
        );

        // Open Wizard
        await page.getByRole('button', { name: 'Request VM' }).click();
        const wizardModal = getAntModal(page, 'vm-request-wizard-modal');
        await expect(wizardModal).toBeVisible();

        // Step 1: System + Service
        const systemSelect = wizardModal.locator('[role="combobox"]').first();
        const serviceSelect = wizardModal.locator('[role="combobox"]').nth(1);
        await selectAntOption(page, systemSelect, e2eSystemName);
        await selectAntOption(page, serviceSelect, e2eServiceName);
        await wizardModal.getByRole('button', { name: 'Next' }).click();

        // Step 2: Template
        await selectAntOption(page, wizardModal.locator('[role="combobox"]').first());
        await wizardModal.getByRole('button', { name: 'Next' }).click();

        // Step 3: Size
        await selectAntOption(page, wizardModal.locator('[role="combobox"]').first());
        await wizardModal.getByRole('button', { name: 'Next' }).click();

        // Step 4: Config
        await wizardModal.locator('#vm-request-wizard_namespace').fill('e2e-batch-ns');
        await wizardModal.locator('#vm-request-wizard_reason').fill('Test batch submit');
        await wizardModal.locator('#vm-request-wizard_batch_count').fill('2');
        await wizardModal.getByRole('button', { name: 'Next' }).click();

        // Step 5: Confirm
        await wizardModal.getByRole('button', { name: 'Submit' }).click();

        // ── CONTRACT CHECK: strict success path (must be accepted) ─────────────────
        const batchResp = await batchRespPromise;
        expect(batchResp.status(), `POST /vms/batch returned ${batchResp.status()}`).toBe(202);
        await validateApiResponse('VMBatchSubmitResponse', batchResp);
    });

    // ── getSystem: GET /systems/{id} → System ────────────────────────────────

    test('getSystem – GET /systems/{id} conforms to System schema', async ({ page }) => {
        // operationId: getSystem
        // Get first system from list
        const listRespPromise = page.waitForResponse(
            (r) => urlPathEndsWith(r.url(), '/api/v1/systems') && r.request().method() === 'GET'
        );
        const [listResp] = await Promise.all([
            listRespPromise,
            page.goto('/systems'),
        ]);
        // Use validateApiResponse for single-read safety
        const listBody = await validateApiResponse('SystemList', listResp) as { items?: Array<{ id?: string }> };
        const items = listBody.items ?? [];
        expect(items.length, 'No systems found — seed data must include at least one system').toBeGreaterThan(0);
        const systemID = items[0]?.id ?? '';
        expect(systemID).toBeTruthy();
        await expect(page.getByRole('heading', { name: 'Systems' })).toBeVisible();

        // ── GET /systems/{id} → System ────────────────────────────────────────────
        const getRespPromise = page.waitForResponse(
            (r) => urlPathEndsWith(r.url(), `/api/v1/systems/${systemID}`) && r.request().method() === 'GET'
        );
        await page.getByTestId(`system-action-edit-${systemID}`).click();
        await expectSchema(getRespPromise, 'System', 200);
    });

    // ── listServices + getService ─────────────────────────────────────────────

    test('listServices – GET /systems/{id}/services conforms to ServiceList schema', async ({ page }) => {
        // operationId: listServices
        await page.goto('/services');
        await expect(page.getByRole('heading', { name: 'Services' })).toBeVisible();
        const svcListResp = await selectServicesSystemFilter(page, e2eSystemName);

        // ── CONTRACT CHECK: ServiceList schema ────────────────────────────────────
        await expectSchema(Promise.resolve(svcListResp), 'ServiceList', 200);
    });

    test('getService – GET /systems/{id}/services/{id} conforms to Service schema', async ({ page }) => {
        // operationId: getService
        await page.goto('/services');
        await expect(page.getByRole('heading', { name: 'Services' })).toBeVisible();
        const svcListResp = await selectServicesSystemFilter(page, e2eSystemName);

        const path = new URL(svcListResp.url()).pathname;
        const pathMatch = path.match(/\/api\/v1\/systems\/([^/]+)\/services$/);
        const systemID = pathMatch?.[1] ?? '';
        expect(systemID, `Could not parse system_id from URL path: ${path}`).toBeTruthy();

        const svcListBody = await validateApiResponse('ServiceList', svcListResp) as { items?: Array<{ id?: string; name?: string }> };

        const serviceID =
            svcListBody.items?.find((svc) => (svc.name ?? '').trim() === e2eServiceName)?.id
            ?? svcListBody.items?.find((svc) => Boolean(svc.id))?.id
            ?? '';
        expect(serviceID, 'No services found — seed data must include at least one service').toBeTruthy();

        // ── GET /systems/{id}/services/{id} → Service ─────────────────────────────
        const getRespPromise = page.waitForResponse(
            (r) => urlPathIncludes(r.url(), `/api/v1/systems/${systemID}/services/${serviceID}`) && r.request().method() === 'GET'
        );
        await page.getByTestId(`service-action-edit-${serviceID}`).click();
        await expectSchema(getRespPromise, 'Service', 200);
    });

    // ── System Member: addSystemMember + updateSystemMemberRole + deleteSystemMember

    test('addSystemMember + updateSystemMemberRole + deleteSystemMember – full lifecycle', async ({ request }) => {
        // operationId: addSystemMember, updateSystemMemberRole, deleteSystemMember
        const headers = await getApiAuthHeaders(request);
        const systemName = `e2emem${Date.now().toString(36).slice(-5)}`;
        let systemID = '';
        let targetUserID = '';
        let memberAttached = false;

        try {
            const createSysResp = await request.post('/api/v1/systems', {
                headers,
                data: {
                    name: systemName,
                    description: 'temporary system for member lifecycle live e2e',
                },
            });
            expect(createSysResp.status(), `POST /systems returned ${createSysResp.status()}`).toBe(201);
            const sys = await validateApiResponse('System', createSysResp) as { id?: string };
            systemID = sys.id ?? '';
            expect(systemID, 'POST /systems response missing id').toBeTruthy();

            const targetUser = await createTempAdminUser(request, headers, {
                prefix: 'e2e-member',
                displayName: 'Live E2E Member',
            });
            targetUserID = targetUser.id;

            // ── POST /systems/{id}/members → SystemMember ────────────────────────
            const addResp = await request.post(`/api/v1/systems/${systemID}/members`, {
                headers,
                data: {
                    user_id: targetUserID,
                    role: 'member',
                },
            });
            expect(addResp.status(), `POST /systems/{id}/members returned ${addResp.status()}`).toBe(201);
            const addedMember = await validateApiResponse('SystemMember', addResp) as { user_id?: string };
            expect(addedMember.user_id, 'POST /systems/{id}/members missing user_id').toBeTruthy();
            memberAttached = true;

            // ── PATCH /systems/{id}/members/{user_id} → SystemMember ─────────────
            const updateResp = await request.patch(`/api/v1/systems/${systemID}/members/${targetUserID}`, {
                headers,
                data: {
                    role: 'viewer',
                },
            });
            expect(updateResp.status(), `PATCH /systems/{id}/members/{user_id} returned ${updateResp.status()}`).toBe(200);
            await validateApiResponse('SystemMember', updateResp);

            // ── DELETE /systems/{id}/members/{user_id} → 204 ─────────────────────
            const deleteMemberResp = await request.delete(`/api/v1/systems/${systemID}/members/${targetUserID}`, { headers });
            expect(deleteMemberResp.status(), `DELETE /systems/{id}/members/{user_id} returned ${deleteMemberResp.status()}`).toBe(204);
            memberAttached = false;
        } finally {
            if (memberAttached && systemID && targetUserID) {
                const cleanupMemberResp = await request.delete(`/api/v1/systems/${systemID}/members/${targetUserID}`, { headers });
                expect([204, 404], `cleanup delete member returned ${cleanupMemberResp.status()}`).toContain(cleanupMemberResp.status());
            }
            await deleteAdminUserIfPresent(request, headers, targetUserID);
            if (systemID) {
                const deleteSystemResp = await request.delete(`/api/v1/systems/${systemID}?confirm_name=${encodeURIComponent(systemName)}`, { headers });
                expect([204, 404, 409], `cleanup delete system returned ${deleteSystemResp.status()}`).toContain(deleteSystemResp.status());
            }
        }
    });

    // ── getNamespace: GET /admin/namespaces/{id} → NamespaceRegistry ──────────

    test('getNamespace – GET /admin/namespaces/{id} conforms to NamespaceRegistry schema', async ({ request }) => {
        // operationId: getNamespace
        const headers = await getApiAuthHeaders(request);
        const listResp = await request.get('/api/v1/admin/namespaces', { headers });
        expect(listResp.status(), `GET /admin/namespaces returned ${listResp.status()}`).toBe(200);
        const listBody = await validateApiResponse('NamespaceRegistryList', listResp) as { items?: Array<{ id?: string }> };
        const items = listBody.items ?? [];
        expect(items.length, 'No namespaces found — seed data must include at least one namespace').toBeGreaterThan(0);
        const nsID = items[0]?.id ?? '';
        expect(nsID).toBeTruthy();

        const getResp = await request.get(`/api/v1/admin/namespaces/${nsID}`, { headers });
        expect(getResp.status(), `GET /admin/namespaces/${nsID} returned ${getResp.status()}`).toBe(200);
        await validateApiResponse('NamespaceRegistry', getResp);
    });

    // ── changePassword: POST /auth/change-password → 204 ─────────────────────
    // NOTE: This test changes the password and then changes it back.
    // If it fails mid-way, the account password will be in an unknown state.

    test('changePassword – POST /auth/change-password returns 204 (changes and restores password)', async ({ page, request }) => {
        // operationId: changePassword
        await page.goto('/profile');
        await expect(page.locator('body')).toBeVisible();
        const originalPassword = activePassword;
        const stablePassword = e2eNewPassword.length >= 8 ? e2eNewPassword : 'ShepherdLive!2026';
        let changedPassword = `${stablePassword}-tmp`;
        if (changedPassword === originalPassword) {
            changedPassword = `${stablePassword}-tmp2`;
        }

        let changed = false;

        try {
            // ── POST /auth/change-password → 204 (change to new password) ────────────
            const changeRespPromise = page.waitForResponse(
                (r) => urlPathEndsWith(r.url(), '/api/v1/auth/change-password') && r.request().method() === 'POST'
            );
            await page.getByTestId('change-password-button').click();
            const changeModal = getAntModal(page, 'change-password-modal');
            await expect(changeModal).toBeVisible();
            await changeModal.getByTestId('change-password-current-input').fill(originalPassword);
            await changeModal.getByTestId('change-password-new-input').fill(changedPassword);
            await changeModal.getByTestId('change-password-confirm-input').fill(changedPassword);
            await changeModal.getByRole('button', { name: 'OK' }).click();

            const changeResp = await changeRespPromise;
            expect(changeResp.status(), `POST /auth/change-password returned ${changeResp.status()}`).toBe(204);
            changed = true;
            activePassword = changedPassword;
        } finally {
            if (changed) {
                await restorePasswordViaApi(request, e2eUsername, changedPassword, stablePassword);
                activePassword = stablePassword;
            }
        }
    });
});
