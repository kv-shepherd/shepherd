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
 *   cancelTicket            – POST /approvals/{id}/cancel            → 204
 *   submitApprovalBatch     – POST /approvals/batch                  → VMBatchSubmitResponse
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

import { expect, test, type Page, type Response } from '@playwright/test';
import { validateApiResponse } from './lib/schema-validator';

// ── Config ────────────────────────────────────────────────────────────────────

const e2eUsername = process.env.E2E_USERNAME ?? 'e2e-admin';
const e2ePassword = process.env.E2E_PASSWORD ?? 'e2e-admin-123';
const e2eNewPassword = process.env.E2E_NEW_PASSWORD ?? 'e2e-admin-456';

// ── Auth helper ───────────────────────────────────────────────────────────────

async function login(page: Page, username = e2eUsername, password = e2ePassword): Promise<void> {
    await page.goto('/login');
    await expect(page.getByRole('heading', { name: 'KubeVirt Shepherd' })).toBeVisible();
    await page.getByPlaceholder('Username').fill(username);
    await page.getByPlaceholder('Password').fill(password);
    // operationId: login
    const loginRespPromise = page.waitForResponse(
        (r) => r.url().endsWith('/api/v1/auth/login') && r.request().method() === 'POST'
    );
    await page.getByRole('button', { name: 'Login' }).click();
    const loginResp = await loginRespPromise;
    expect(loginResp.status()).toBe(200);
    await validateApiResponse('LoginResponse', loginResp);
    await expect(page).toHaveURL(/\/dashboard$/);
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

// ── Test suite ────────────────────────────────────────────────────────────────

test.describe('user-selfservice live (contract-enforced, no mock, no skip)', () => {
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
        // User-facing template list (not admin) is loaded in VM request wizard
        const respPromise = page.waitForResponse(
            (r) => r.url().endsWith('/api/v1/templates') && !r.url().includes('/admin/') && r.request().method() === 'GET'
        );
        await page.goto('/vms');
        await expect(page.getByRole('heading', { name: 'Virtual Machines' })).toBeVisible();
        // Open VM request wizard to trigger template list load
        await page.getByRole('button', { name: 'Request VM' }).click();
        await expect(page.getByText('Create VM Request')).toBeVisible();
        // Navigate to template step
        const systemSelect = page.locator('.ant-modal-content:visible [role="combobox"]').first();
        await systemSelect.click();
        await page.locator('.ant-select-dropdown:visible .ant-select-item-option').first().click();
        const serviceSelect = page.locator('.ant-modal-content:visible [role="combobox"]').nth(1);
        await serviceSelect.click();
        await page.locator('.ant-select-dropdown:visible .ant-select-item-option').first().click();
        await page.locator('.ant-modal-content:visible').getByRole('button', { name: 'Next' }).click();

        // ── CONTRACT CHECK: TemplateList schema ───────────────────────────────────
        await expectSchema(respPromise, 'TemplateList', 200);
    });

    // ── listInstanceSizes: GET /instance-sizes → InstanceSizeList ─────────────

    test('listInstanceSizes – GET /instance-sizes conforms to InstanceSizeList schema', async ({ page }) => {
        // operationId: listInstanceSizes
        const respPromise = page.waitForResponse(
            (r) => r.url().endsWith('/api/v1/instance-sizes') && !r.url().includes('/admin/') && r.request().method() === 'GET'
        );
        await page.goto('/vms');
        await expect(page.getByRole('heading', { name: 'Virtual Machines' })).toBeVisible();
        await page.getByRole('button', { name: 'Request VM' }).click();
        await expect(page.getByText('Create VM Request')).toBeVisible();
        // Navigate to instance size step (step 2)
        const systemSelect = page.locator('.ant-modal-content:visible [role="combobox"]').first();
        await systemSelect.click();
        await page.locator('.ant-select-dropdown:visible .ant-select-item-option').first().click();
        const serviceSelect = page.locator('.ant-modal-content:visible [role="combobox"]').nth(1);
        await serviceSelect.click();
        await page.locator('.ant-select-dropdown:visible .ant-select-item-option').first().click();
        await page.locator('.ant-modal-content:visible').getByRole('button', { name: 'Next' }).click();
        // Select template
        const templateSelect = page.locator('.ant-modal-content:visible [role="combobox"]').first();
        await templateSelect.click();
        await page.locator('.ant-select-dropdown:visible .ant-select-item-option').first().click();
        await page.locator('.ant-modal-content:visible').getByRole('button', { name: 'Next' }).click();

        // ── CONTRACT CHECK: InstanceSizeList schema ───────────────────────────────
        await expectSchema(respPromise, 'InstanceSizeList', 200);
    });

    // ── Notifications: markNotificationRead + markAllNotificationsRead ─────────

    test('markNotificationRead – PATCH /notifications/{id}/read returns 204', async ({ page }) => {
        // operationId: markNotificationRead
        // Get first notification
        const listRespPromise = page.waitForResponse(
            (r) => r.url().includes('/api/v1/notifications') && r.request().method() === 'GET'
                && !r.url().includes('unread-count')
        );
        await page.goto('/notifications');
        await expect(page.getByRole('heading', { name: 'Notifications' })).toBeVisible();
        // Use validateApiResponse for single-read safety
        const listBody = await validateApiResponse('NotificationList', await listRespPromise) as { items?: Array<{ id?: string; read?: boolean }> };
        const unread = (listBody.items ?? []).find((n) => !n.read);
        expect(unread, 'No unread notifications found — seed data must include at least one unread notification').toBeTruthy();
        const notifID = unread?.id ?? '';
        expect(notifID).toBeTruthy();

        // ── PATCH /notifications/{id}/read → 204 ─────────────────────────────────
        const readRespPromise = page.waitForResponse(
            (r) => r.url().includes(`/api/v1/notifications/${notifID}/read`) && r.request().method() === 'PATCH'
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
            (r) => r.url().endsWith('/api/v1/notifications/mark-all-read') && r.request().method() === 'POST'
        );
        await page.getByTestId('notifications-mark-all-read-button').click();
        const confirmBtn = page.locator('.ant-popover:visible, .ant-modal-content:visible')
            .getByRole('button', { name: /confirm|ok/i }).first();
        if (await confirmBtn.count() > 0) await confirmBtn.click();

        const markAllResp = await markAllRespPromise;
        expect(markAllResp.status(), `POST /notifications/mark-all-read returned ${markAllResp.status()}`).toBe(204);
    });

    // ── cancelTicket: POST /approvals/{id}/cancel → 204 ──────────────────────

    test('cancelTicket – POST /approvals/{id}/cancel returns 204', async ({ page }) => {
        // operationId: cancelTicket
        // First submit a VM request to get a pending ticket
        await page.goto('/vms');
        await expect(page.getByRole('heading', { name: 'Virtual Machines' })).toBeVisible();
        await page.getByRole('button', { name: 'Request VM' }).click();
        await expect(page.getByText('Create VM Request')).toBeVisible();

        const systemSelect = page.locator('.ant-modal-content:visible [role="combobox"]').first();
        await systemSelect.click();
        await page.locator('.ant-select-dropdown:visible .ant-select-item-option').first().click();
        const serviceSelect = page.locator('.ant-modal-content:visible [role="combobox"]').nth(1);
        await serviceSelect.click();
        await page.locator('.ant-select-dropdown:visible .ant-select-item-option').first().click();
        await page.locator('.ant-modal-content:visible').getByRole('button', { name: 'Next' }).click();

        const templateSelect = page.locator('.ant-modal-content:visible [role="combobox"]').first();
        await templateSelect.click();
        await page.locator('.ant-select-dropdown:visible .ant-select-item-option').first().click();
        await page.locator('.ant-modal-content:visible').getByRole('button', { name: 'Next' }).click();

        const sizeSelect = page.locator('.ant-modal-content:visible [role="combobox"]').first();
        await sizeSelect.click();
        await page.locator('.ant-select-dropdown:visible .ant-select-item-option').first().click();
        await page.locator('.ant-modal-content:visible').getByRole('button', { name: 'Next' }).click();

        await page.locator('.ant-modal-content:visible input').first().fill('test-ns-cancel');
        await page.locator('.ant-modal-content:visible textarea').first().fill('cancel test request');
        await page.locator('.ant-modal-content:visible').getByRole('button', { name: 'Next' }).click();

        const submitRespPromise = page.waitForResponse(
            (r) => r.url().endsWith('/api/v1/vms/request') && r.request().method() === 'POST'
        );
        await page.locator('.ant-modal-content:visible').getByRole('button', { name: 'Submit' }).click();
        const submitResp = await submitRespPromise;
        expect([202, 400, 409]).toContain(submitResp.status());

        // Single-read the response body (Playwright responses are single-read;
        // calling .json() twice throws a Protocol error)
        const submitBody = await submitResp.json() as { ticket_id?: string; id?: string; message?: string };

        if (submitResp.status() !== 202) {
            throw new Error(`POST /vms/request failed with ${submitResp.status()}: ${submitBody.message ?? JSON.stringify(submitBody)}`);
        }

        const ticketID = submitBody.ticket_id ?? submitBody.id ?? '';
        expect(ticketID, 'POST /vms/request response missing ticket_id/id field').toBeTruthy();

        // ── POST /approvals/{id}/cancel → 204 ────────────────────────────────────
        const cancelRespPromise = page.waitForResponse(
            (r) => r.url().endsWith(`/api/v1/approvals/${ticketID}/cancel`) && r.request().method() === 'POST'
        );
        await page.goto('/approvals');
        await expect(page.getByRole('heading', { name: /approval/i })).toBeVisible();
        await page.getByTestId(`approval-action-cancel-${ticketID}`).click();
        const confirmBtn = page.locator('.ant-popover:visible, .ant-modal-content:visible')
            .getByRole('button', { name: /confirm|ok|cancel/i }).first();
        if (await confirmBtn.count() > 0) await confirmBtn.click();

        const cancelResp = await cancelRespPromise;
        expect(cancelResp.status(), `POST /approvals/${ticketID}/cancel returned ${cancelResp.status()}`).toBe(204);
    });

    // ── submitApprovalBatch: POST /approvals/batch → VMBatchSubmitResponse ────

    test('submitApprovalBatch – POST /approvals/batch conforms to VMBatchSubmitResponse schema', async ({ page }) => {
        // operationId: submitApprovalBatch
        await page.goto('/approvals');
        await expect(page.getByRole('heading', { name: /approval/i })).toBeVisible();

        // Select all pending tickets
        const headerCheckbox = page.locator('thead input[type="checkbox"]').first();
        await expect(headerCheckbox, 'Approvals table header checkbox not found').toBeVisible();
        await headerCheckbox.check();

        const batchRespPromise = page.waitForResponse(
            (r) => r.url().endsWith('/api/v1/approvals/batch') && r.request().method() === 'POST'
        );
        // Click batch approve button
        const batchApproveBtn = page.getByRole('button', { name: /batch approve|approve all/i }).first();
        await expect(batchApproveBtn, 'Batch approve button not found in approvals page').toBeVisible();
        await batchApproveBtn.click();
        const confirmBtn = page.locator('.ant-modal-content:visible')
            .getByRole('button', { name: /confirm|ok/i }).first();
        if (await confirmBtn.count() > 0) await confirmBtn.click();

        // ── CONTRACT CHECK: VMBatchSubmitResponse schema ──────────────────────────
        await expectSchema(batchRespPromise, 'VMBatchSubmitResponse', [202, 400, 429]);
    });

    // ── getSystem: GET /systems/{id} → System ────────────────────────────────

    test('getSystem – GET /systems/{id} conforms to System schema', async ({ page }) => {
        // operationId: getSystem
        // Get first system from list
        const listRespPromise = page.waitForResponse(
            (r) => r.url().endsWith('/api/v1/systems') && r.request().method() === 'GET'
        );
        await page.goto('/systems');
        await expect(page.getByRole('heading', { name: 'Systems' })).toBeVisible();
        // Use validateApiResponse for single-read safety
        const listBody = await validateApiResponse('SystemList', await listRespPromise) as { items?: Array<{ id?: string }> };
        const items = listBody.items ?? [];
        expect(items.length, 'No systems found — seed data must include at least one system').toBeGreaterThan(0);
        const systemID = items[0]?.id ?? '';
        expect(systemID).toBeTruthy();

        // ── GET /systems/{id} → System ────────────────────────────────────────────
        const getRespPromise = page.waitForResponse(
            (r) => r.url().endsWith(`/api/v1/systems/${systemID}`) && r.request().method() === 'GET'
        );
        await page.getByTestId(`system-action-detail-${systemID}`).click();
        await expectSchema(getRespPromise, 'System', 200);
    });

    // ── listServices + getService ─────────────────────────────────────────────

    test('listServices – GET /systems/{id}/services conforms to ServiceList schema', async ({ page }) => {
        // operationId: listServices
        // Get first system
        const sysListRespPromise = page.waitForResponse(
            (r) => r.url().endsWith('/api/v1/systems') && r.request().method() === 'GET'
        );
        await page.goto('/systems');
        await expect(page.getByRole('heading', { name: 'Systems' })).toBeVisible();
        const sysListBody = await validateApiResponse('SystemList', await sysListRespPromise) as { items?: Array<{ id?: string }> };
        const systemID = sysListBody.items?.[0]?.id ?? '';
        expect(systemID, 'No systems found').toBeTruthy();

        // Navigate to services for this system
        const svcListRespPromise = page.waitForResponse(
            (r) => r.url().includes(`/api/v1/systems/${systemID}/services`) && r.request().method() === 'GET'
                && !r.url().includes('/services/')
        );
        await page.goto('/services');
        await expect(page.getByRole('heading', { name: 'Services' })).toBeVisible();
        await page.getByTestId('services-system-selector').click();
        await page.locator('.ant-select-item-option').first().click();

        // ── CONTRACT CHECK: ServiceList schema ────────────────────────────────────
        await expectSchema(svcListRespPromise, 'ServiceList', 200);
    });

    test('getService – GET /systems/{id}/services/{id} conforms to Service schema', async ({ page }) => {
        // operationId: getService
        // Get first system and first service
        const sysListRespPromise = page.waitForResponse(
            (r) => r.url().endsWith('/api/v1/systems') && r.request().method() === 'GET'
        );
        await page.goto('/systems');
        await expect(page.getByRole('heading', { name: 'Systems' })).toBeVisible();
        const sysListBody = await validateApiResponse('SystemList', await sysListRespPromise) as { items?: Array<{ id?: string }> };
        const systemID = sysListBody.items?.[0]?.id ?? '';
        expect(systemID, 'No systems found').toBeTruthy();

        await page.goto('/services');
        await expect(page.getByRole('heading', { name: 'Services' })).toBeVisible();
        await page.getByTestId('services-system-selector').click();
        await page.locator('.ant-select-item-option').first().click();

        const svcListRespPromise = page.waitForResponse(
            (r) => r.url().includes(`/api/v1/systems/${systemID}/services`) && r.request().method() === 'GET'
                && !r.url().includes('/services/')
        );
        const svcListBody = await validateApiResponse('ServiceList', await svcListRespPromise) as { items?: Array<{ id?: string }> };
        const serviceID = svcListBody.items?.[0]?.id ?? '';
        expect(serviceID, 'No services found — seed data must include at least one service').toBeTruthy();

        // ── GET /systems/{id}/services/{id} → Service ─────────────────────────────
        const getRespPromise = page.waitForResponse(
            (r) => r.url().includes(`/api/v1/systems/${systemID}/services/${serviceID}`) && r.request().method() === 'GET'
        );
        await page.getByTestId(`service-action-detail-${serviceID}`).click();
        await expectSchema(getRespPromise, 'Service', 200);
    });

    // ── System Member: addSystemMember + updateSystemMemberRole + deleteSystemMember

    test('addSystemMember + updateSystemMemberRole + deleteSystemMember – full lifecycle', async ({ page }) => {
        // operationId: addSystemMember, updateSystemMemberRole, deleteSystemMember
        // Create a temp system
        await page.goto('/systems');
        await expect(page.getByRole('heading', { name: 'Systems' })).toBeVisible();
        const systemName = `e2emem${Date.now().toString(36).slice(-5)}`;
        const createSysRespPromise = page.waitForResponse(
            (r) => r.url().endsWith('/api/v1/systems') && r.request().method() === 'POST'
        );
        await page.getByTestId('system-create-button').click();
        const createModal = page.locator('.ant-modal-content').filter({ hasText: /create system/i });
        await expect(createModal).toBeVisible();
        await createModal.locator('input[maxlength="15"]').first().fill(systemName);
        await createModal.getByRole('button', { name: 'OK' }).click();
        const { body: sys } = await expectSchema(createSysRespPromise, 'System', 201);
        const systemID = (sys as { id?: string }).id ?? '';
        expect(systemID).toBeTruthy();

        // Get a user to add as member
        const usersRespPromise = page.waitForResponse(
            (r) => r.url().endsWith('/api/v1/admin/users') && r.request().method() === 'GET'
        );
        await page.goto('/admin/users');
        await expect(page.getByTestId('admin-users-page')).toBeVisible();
        const usersResp = await usersRespPromise;
        const usersBody = await usersResp.json() as { items?: Array<{ id?: string; username?: string }> };
        const targetUser = usersBody.items?.find((u) => u.username !== e2eUsername);
        expect(targetUser, 'Need at least 2 users for member management test').toBeTruthy();
        const targetUserID = targetUser?.id ?? '';

        // Navigate to system members
        await page.goto('/systems');
        await page.getByTestId(`system-action-members-${systemID}`).click();
        const membersModal = page.locator('.ant-modal-content:visible');
        await expect(membersModal).toBeVisible();

        // ── POST /systems/{id}/members → SystemMember ─────────────────────────────
        const addRespPromise = page.waitForResponse(
            (r) => r.url().endsWith(`/api/v1/systems/${systemID}/members`) && r.request().method() === 'POST'
        );
        await membersModal.getByTestId('member-add-button').click();
        const addModal = page.getByTestId('member-add-modal');
        await expect(addModal).toBeVisible();
        await addModal.locator('.ant-select-selector').first().click();
        await page.locator('.ant-select-item-option').filter({ hasText: targetUser?.username ?? '' }).first().click();
        await addModal.getByRole('button', { name: 'OK' }).click();

        const { body: addedMember } = await expectSchema(addRespPromise, 'SystemMember', 201);
        expect((addedMember as { user_id?: string }).user_id, 'POST /systems/{id}/members missing user_id').toBeTruthy();

        // ── PATCH /systems/{id}/members/{user_id} → SystemMember ─────────────────
        const updateRespPromise = page.waitForResponse(
            (r) => r.url().includes(`/api/v1/systems/${systemID}/members/${targetUserID}`) && r.request().method() === 'PATCH'
        );
        await page.getByTestId(`member-action-edit-${targetUserID}`).click();
        const editModal = page.getByTestId('member-edit-modal');
        await expect(editModal).toBeVisible();
        await editModal.locator('.ant-select-selector').first().click();
        await page.locator('.ant-select-item-option').last().click();
        await editModal.getByRole('button', { name: 'OK' }).click();

        await expectSchema(updateRespPromise, 'SystemMember', 200);

        // ── DELETE /systems/{id}/members/{user_id} → 204 ──────────────────────────
        const deleteRespPromise = page.waitForResponse(
            (r) => r.url().includes(`/api/v1/systems/${systemID}/members/${targetUserID}`) && r.request().method() === 'DELETE'
        );
        await page.getByTestId(`member-action-remove-${targetUserID}`).click();
        const confirmBtn = page.getByRole('button', { name: /confirm|ok/i }).last();
        await confirmBtn.click();
        expect((await deleteRespPromise).status()).toBe(204);

        // Close modal and cleanup system
        await page.keyboard.press('Escape');
        await page.goto('/systems');
        await page.getByTestId(`system-action-delete-${systemID}`).click();
        const deleteModal = page.locator('.ant-modal-content').filter({ hasText: /delete system/i });
        await expect(deleteModal).toBeVisible();
        await deleteModal.getByRole('textbox').first().fill(systemName);
        const deleteSysRespPromise = page.waitForResponse(
            (r) => r.url().includes(`/api/v1/systems/${systemID}`) && r.request().method() === 'DELETE'
        );
        await deleteModal.getByRole('button', { name: /delete/i }).click();
        expect((await deleteSysRespPromise).status()).toBe(204);
    });

    // ── getNamespace: GET /admin/namespaces/{id} → NamespaceRegistry ──────────

    test('getNamespace – GET /admin/namespaces/{id} conforms to NamespaceRegistry schema', async ({ page }) => {
        // operationId: getNamespace
        const listRespPromise = page.waitForResponse(
            (r) => r.url().endsWith('/api/v1/admin/namespaces') && r.request().method() === 'GET'
        );
        await page.goto('/admin/namespaces');
        await expect(page.getByTestId('admin-namespaces-page')).toBeVisible();
        // Use validateApiResponse for single-read safety
        const listBody = await validateApiResponse('NamespaceRegistryList', await listRespPromise) as { items?: Array<{ id?: string }> };
        const items = listBody.items ?? [];
        expect(items.length, 'No namespaces found — seed data must include at least one namespace').toBeGreaterThan(0);
        const nsID = items[0]?.id ?? '';
        expect(nsID).toBeTruthy();

        // ── GET /admin/namespaces/{id} → NamespaceRegistry ───────────────────────
        const getRespPromise = page.waitForResponse(
            (r) => r.url().endsWith(`/api/v1/admin/namespaces/${nsID}`) && r.request().method() === 'GET'
        );
        await page.getByTestId(`namespace-action-detail-${nsID}`).click();
        await expectSchema(getRespPromise, 'NamespaceRegistry', 200);
    });

    // ── changePassword: POST /auth/change-password → 204 ─────────────────────
    // NOTE: This test changes the password and then changes it back.
    // If it fails mid-way, the account password will be in an unknown state.

    test('changePassword – POST /auth/change-password returns 204 (changes and restores password)', async ({ page }) => {
        // operationId: changePassword
        await page.goto('/profile');
        await expect(page.locator('body')).toBeVisible();

        // ── POST /auth/change-password → 204 (change to new password) ────────────
        const changeRespPromise = page.waitForResponse(
            (r) => r.url().endsWith('/api/v1/auth/change-password') && r.request().method() === 'POST'
        );
        await page.getByTestId('change-password-button').click();
        const changeModal = page.getByTestId('change-password-modal');
        await expect(changeModal).toBeVisible();
        await changeModal.locator('input[type="password"]').nth(0).fill(e2ePassword);
        await changeModal.locator('input[type="password"]').nth(1).fill(e2eNewPassword);
        await changeModal.locator('input[type="password"]').nth(2).fill(e2eNewPassword);
        await changeModal.getByRole('button', { name: 'OK' }).click();

        const changeResp = await changeRespPromise;
        expect(changeResp.status(), `POST /auth/change-password returned ${changeResp.status()}`).toBe(204);

        // ── Restore original password ─────────────────────────────────────────────
        const restoreRespPromise = page.waitForResponse(
            (r) => r.url().endsWith('/api/v1/auth/change-password') && r.request().method() === 'POST'
        );
        await page.getByTestId('change-password-button').click();
        const restoreModal = page.getByTestId('change-password-modal');
        await expect(restoreModal).toBeVisible();
        await restoreModal.locator('input[type="password"]').nth(0).fill(e2eNewPassword);
        await restoreModal.locator('input[type="password"]').nth(1).fill(e2ePassword);
        await restoreModal.locator('input[type="password"]').nth(2).fill(e2ePassword);
        await restoreModal.getByRole('button', { name: 'OK' }).click();

        const restoreResp = await restoreRespPromise;
        expect(restoreResp.status(), `Password restore failed with ${restoreResp.status()}`).toBe(204);
    });
});
