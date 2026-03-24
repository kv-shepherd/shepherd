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

import { expect, test, type APIRequestContext, type Locator, type Page, type Response } from '@playwright/test';
import { validateApiResponse } from './lib/schema-validator';
import {
    ensureBatchSubmitPolicyForUser,
    ensureSeedSystemAndService,
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
const e2eSystemName = process.env.E2E_SYSTEM ?? 'e2e-system';
const e2eServiceName = process.env.E2E_SERVICE ?? 'e2e-service';
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

async function getApiAuthHeaders(request: APIRequestContext): Promise<{ Authorization: string }> {
    const auth = await getApiTokenWithForcePasswordSupport(request, {
        username: e2eUsername,
        primaryPassword: e2ePassword,
        secondaryPassword: e2eNewPassword,
        currentPasswordHint: activePassword,
    });
    activePassword = auth.password;
    return { Authorization: `Bearer ${auth.token}` };
}

async function findTicketCancelButtonAcrossPages(
    page: Page,
    ticketID: string,
    maxPages = 12
): Promise<Locator | null> {
    const targetTestId = `approval-action-cancel-${ticketID}`;

    for (let pageIndex = 0; pageIndex < maxPages; pageIndex += 1) {
        const cancelBtn = page.getByTestId(targetTestId).first();
        const visible = await cancelBtn.isVisible().catch(() => false);
        if (visible) {
            return cancelBtn;
        }

        const nextPageBtn = page
            .locator('.ant-pagination-next:not(.ant-pagination-disabled) button, .ant-pagination-next:not(.ant-pagination-disabled) a')
            .first();
        if (await nextPageBtn.count() === 0) {
            return null;
        }

        const listRespPromise = page.waitForResponse(
            (r) => urlPathEndsWith(r.url(), '/api/v1/tickets') && r.request().method() === 'GET'
        );
        await nextPageBtn.click();
        await listRespPromise;
    }

    return null;
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

    test('cancelTicket – POST /tickets/{id}/cancel returns 204', async ({ page }) => {
        // operationId: cancelTicket
        // First submit a VM request to get a pending ticket
        const uniqueSuffix = `${Date.now()}`;
        const requestNamespace = e2eNamespace;
        await page.goto('/vms');
        await expect(page.getByRole('heading', { name: 'Virtual Machines' })).toBeVisible();
        await page.getByRole('button', { name: 'Request VM' }).click();
        await expect(page.getByText('Create VM Request')).toBeVisible();

        const systemSelect = getAntModal(page, 'vm-request-wizard-modal').locator('[role="combobox"]').first();
        await selectAntOption(page, systemSelect, e2eSystemName);
        const serviceSelect = getAntModal(page, 'vm-request-wizard-modal').locator('[role="combobox"]').nth(1);
        await selectAntOption(page, serviceSelect, e2eServiceName);
        await getAntModal(page, 'vm-request-wizard-modal').getByRole('button', { name: 'Next' }).click();

        const templateSelect = getAntModal(page, 'vm-request-wizard-modal').locator('[role="combobox"]').first();
        await selectAntOption(page, templateSelect);
        await getAntModal(page, 'vm-request-wizard-modal').getByRole('button', { name: 'Next' }).click();

        const sizeSelect = getAntModal(page, 'vm-request-wizard-modal').locator('[role="combobox"]').first();
        await selectAntOption(page, sizeSelect);
        await getAntModal(page, 'vm-request-wizard-modal').getByRole('button', { name: 'Next' }).click();

        await getAntModal(page, 'vm-request-wizard-modal').locator('#vm-request-wizard_namespace').fill(requestNamespace);
        await getAntModal(page, 'vm-request-wizard-modal').locator('#vm-request-wizard_reason').fill(`cancel test request ${uniqueSuffix}`);
        await getAntModal(page, 'vm-request-wizard-modal').getByRole('button', { name: 'Next' }).click();

        const submitRespPromise = page.waitForResponse(
            (r) => urlPathEndsWith(r.url(), '/api/v1/vms/request') && r.request().method() === 'POST'
        );
        await getAntModal(page, 'vm-request-wizard-modal').getByRole('button', { name: 'Submit' }).click();
        const submitResp = await submitRespPromise;
        let ticketID = '';
        if (submitResp.status() === 202) {
            const submitBody = await validateApiResponse('TicketResponse', submitResp) as { ticket_id?: string; id?: string };
            ticketID = submitBody.ticket_id ?? submitBody.id ?? '';
        } else if (submitResp.status() === 400) {
            const errBody = await validateApiResponse('Error', submitResp) as {
                code?: string;
                message?: string;
                params?: Record<string, unknown>;
            };
            const existingTicketID =
                typeof errBody.params?.existing_ticket_id === 'string' ? errBody.params.existing_ticket_id.trim() : '';
            if (errBody.code === 'DUPLICATE_PENDING_REQUEST' && existingTicketID) {
                ticketID = existingTicketID;
            } else {
                throw new Error(
                    `POST /vms/request returned 400: ${errBody.code ?? 'UNKNOWN'} (${errBody.message ?? 'no message'})`
                );
            }
        } else {
            expect(submitResp.status(), `POST /vms/request returned ${submitResp.status()}`).toBe(202);
        }
        expect(ticketID, 'POST /vms/request response missing ticket_id/id field').toBeTruthy();

        // ── POST /tickets/{id}/cancel → 204 ──────────────────────────────────────
        const listRespPromise = page.waitForResponse(
            (r) => urlPathEndsWith(r.url(), '/api/v1/tickets') && r.request().method() === 'GET'
        );
        await page.goto('/tickets');
        await listRespPromise;
        await expect(page.getByRole('heading', { name: /request/i })).toBeVisible();

        const cancelActionBtn = await findTicketCancelButtonAcrossPages(page, ticketID);
        if (!cancelActionBtn) {
            throw new Error(`Ticket ${ticketID} should be visible and cancellable in the ticket list`);
        }
        await expect(cancelActionBtn).toBeVisible({ timeout: 20000 });
        await expect(cancelActionBtn).toBeEnabled();

        const cancelRespPromise = page.waitForResponse(
            (r) => urlPathEndsWith(r.url(), `/api/v1/tickets/${ticketID}/cancel`) && r.request().method() === 'POST'
        );
        await cancelActionBtn.click();
        const confirmBtn = page.locator('.ant-popover:visible, .ant-modal-content:visible')
            .getByRole('button', { name: /confirm|ok|yes/i }).first();
        if (await confirmBtn.count() > 0) await confirmBtn.click();

        const cancelResp = await cancelRespPromise;
        expect(cancelResp.status(), `POST /tickets/${ticketID}/cancel returned ${cancelResp.status()}`).toBe(204);
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
        const svcListRespPromise = page.waitForResponse(
            (r) => {
                if (r.request().method() !== 'GET') return false;
                const path = new URL(r.url()).pathname;
                return /\/api\/v1\/systems\/[^/]+\/services$/.test(path);
            }
        );
        const [svcListResp] = await Promise.all([
            svcListRespPromise,
            page.goto('/services'),
        ]);
        await expect(page.getByRole('heading', { name: 'Services' })).toBeVisible();

        // ── CONTRACT CHECK: ServiceList schema ────────────────────────────────────
        await expectSchema(Promise.resolve(svcListResp), 'ServiceList', 200);
    });

    test('getService – GET /systems/{id}/services/{id} conforms to Service schema', async ({ page }) => {
        // operationId: getService
        const readServiceListResponse = () => page.waitForResponse(
            (r) => {
                if (r.request().method() !== 'GET') return false;
                const path = new URL(r.url()).pathname;
                return /\/api\/v1\/systems\/[^/]+\/services$/.test(path);
            }
        );
        let svcListResp: Response;
        [svcListResp] = await Promise.all([
            readServiceListResponse(),
            page.goto('/services'),
        ]);
        await expect(page.getByRole('heading', { name: 'Services' })).toBeVisible();

        let path = new URL(svcListResp.url()).pathname;
        let pathMatch = path.match(/\/api\/v1\/systems\/([^/]+)\/services$/);
        let systemID = pathMatch?.[1] ?? '';
        expect(systemID, `Could not parse system_id from URL path: ${path}`).toBeTruthy();

        let svcListBody = await validateApiResponse('ServiceList', svcListResp) as { items?: Array<{ id?: string; name?: string }> };
        if ((svcListBody.items?.length ?? 0) === 0) {
            const svcListRespPromise = readServiceListResponse();
            await selectAntOption(page, page.getByTestId('services-system-selector'), e2eSystemName);
            svcListResp = await svcListRespPromise;
            path = new URL(svcListResp.url()).pathname;
            pathMatch = path.match(/\/api\/v1\/systems\/([^/]+)\/services$/);
            systemID = pathMatch?.[1] ?? '';
            expect(systemID, `Could not parse system_id from URL path: ${path}`).toBeTruthy();
            svcListBody = await validateApiResponse('ServiceList', svcListResp) as { items?: Array<{ id?: string; name?: string }> };
        }

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

    test('addSystemMember + updateSystemMemberRole + deleteSystemMember – full lifecycle', async ({ page }) => {
        // operationId: addSystemMember, updateSystemMemberRole, deleteSystemMember
        // Create a temp system
        await page.goto('/systems');
        await expect(page.getByRole('heading', { name: 'Systems' })).toBeVisible();
        const systemName = `e2emem${Date.now().toString(36).slice(-5)}`;
        const createSysRespPromise = page.waitForResponse(
            (r) => urlPathEndsWith(r.url(), '/api/v1/systems') && r.request().method() === 'POST'
        );
        await page.getByTestId('system-create-button').click();
        const createModal = getAntModal(page, 'system-create-modal');
        await expect(createModal).toBeVisible();
        await createModal.locator('input[maxlength="15"]').first().fill(systemName);
        await createModal.getByRole('button', { name: 'OK' }).click();
        const { body: sys } = await expectSchema(createSysRespPromise, 'System', 201);
        const systemID = (sys as { id?: string }).id ?? '';
        expect(systemID).toBeTruthy();

        // Get a user to add as member
        const usersRespPromise = page.waitForResponse(
            (r) => urlPathEndsWith(r.url(), '/api/v1/admin/users') && r.request().method() === 'GET'
        );
        const [usersResp] = await Promise.all([
            usersRespPromise,
            page.goto('/admin/users'),
        ]);
        await expect(page.getByTestId('admin-users-page')).toBeVisible();
        const usersBody = await validateApiResponse('UserList', usersResp) as { items?: Array<{ id?: string; username?: string }> };
        const targetUser = usersBody.items?.find((u) => u.username !== e2eUsername);
        expect(targetUser, 'Need at least 2 users for member management test').toBeTruthy();
        const targetUserID = targetUser?.id ?? '';

        // Navigate to system members
        await page.goto('/systems');
        await page.getByTestId(`system-action-members-${systemID}`).click();
        const membersModal = getAntModal(page, 'system-members-modal');
        await expect(membersModal).toBeVisible();

        // ── POST /systems/{id}/members → SystemMember ─────────────────────────────
        const addRespPromise = page.waitForResponse(
            (r) => urlPathEndsWith(r.url(), `/api/v1/systems/${systemID}/members`) && r.request().method() === 'POST'
        );
        await membersModal.getByTestId('member-add-button').click();
        const addModal = getAntModal(page, 'member-add-modal');
        await expect(addModal).toBeVisible();
        await addModal.locator('#add-system-member_user_id').fill(targetUserID);
        await selectAntOption(page, addModal.locator('.ant-select-selector').first(), /member|viewer|admin|owner/i);
        await addModal.getByRole('button', { name: 'OK' }).click();

        const { body: addedMember } = await expectSchema(addRespPromise, 'SystemMember', 201);
        expect((addedMember as { user_id?: string }).user_id, 'POST /systems/{id}/members missing user_id').toBeTruthy();

        // ── PATCH /systems/{id}/members/{user_id} → SystemMember ─────────────────
        const updateRespPromise = page.waitForResponse(
            (r) => urlPathIncludes(r.url(), `/api/v1/systems/${systemID}/members/${targetUserID}`) && r.request().method() === 'PATCH'
        );
        const roleSelector = membersModal.getByTestId(`member-action-edit-${targetUserID}`);
        await expect(roleSelector).toBeVisible();
        await selectAntOption(page, roleSelector, /viewer/i);

        await expectSchema(updateRespPromise, 'SystemMember', 200);

        // ── DELETE /systems/{id}/members/{user_id} → 204 ──────────────────────────
        const deleteRespPromise = page.waitForResponse(
            (r) => urlPathIncludes(r.url(), `/api/v1/systems/${systemID}/members/${targetUserID}`) && r.request().method() === 'DELETE'
        );
        await membersModal.getByTestId(`member-action-remove-${targetUserID}`).click();
        const confirmBtn = page.getByRole('button', { name: /confirm|ok/i }).last();
        await confirmBtn.click();
        expect((await deleteRespPromise).status()).toBe(204);

        // Close modal and cleanup system
        await page.keyboard.press('Escape');
        await page.goto('/systems');
        await page.getByTestId(`system-action-delete-${systemID}`).click();
        const deleteModal = getAntModal(page, 'system-delete-modal');
        await expect(deleteModal).toBeVisible();
        await deleteModal.getByRole('textbox').first().fill(systemName);
        const deleteSysRespPromise = page.waitForResponse(
            (r) => urlPathIncludes(r.url(), `/api/v1/systems/${systemID}`) && r.request().method() === 'DELETE'
        );
        await deleteModal.getByRole('button', { name: /delete/i }).click();
        expect((await deleteSysRespPromise).status()).toBe(204);
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

    test('changePassword – POST /auth/change-password returns 204 (changes and restores password)', async ({ page }) => {
        // operationId: changePassword
        await page.goto('/profile');
        await expect(page.locator('body')).toBeVisible();
        const originalPassword = activePassword;
        const stablePassword = e2eNewPassword.length >= 8 ? e2eNewPassword : 'admin123';
        let changedPassword = `${stablePassword}-tmp`;
        if (changedPassword === originalPassword) {
            changedPassword = `${stablePassword}-tmp2`;
        }

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
        activePassword = changedPassword;

        // ── Restore stable password (must satisfy UI min length constraints) ─────
        const restoreRespPromise = page.waitForResponse(
            (r) => urlPathEndsWith(r.url(), '/api/v1/auth/change-password') && r.request().method() === 'POST'
        );
        await page.getByTestId('change-password-button').click();
        const restoreModal = getAntModal(page, 'change-password-modal');
        await expect(restoreModal).toBeVisible();
        await restoreModal.getByTestId('change-password-current-input').fill(changedPassword);
        await restoreModal.getByTestId('change-password-new-input').fill(stablePassword);
        await restoreModal.getByTestId('change-password-confirm-input').fill(stablePassword);
        await restoreModal.getByRole('button', { name: 'OK' }).click();

        const restoreResp = await restoreRespPromise;
        expect(restoreResp.status(), `Password restore failed with ${restoreResp.status()}`).toBe(204);
        activePassword = stablePassword;
    });
});
