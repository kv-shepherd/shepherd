/**
 * Admin Flow Smoke Tests (mock-only, no live backend)
 *
 * Coverage map (master-flow.md):
 *   Stage 2.A   – RBAC: custom role create / delete
 *   Stage 2.A+  – User management: create / delete user
 *   Stage 3     – Cluster registration (UI smoke)
 *   Stage 3     – Namespace management: create / edit / delete
 *   Stage 5.B   – Approval / Rejection workflow
 *
 * All API calls are intercepted via page.route().
 * Tests are self-contained and order-independent.
 *
 * Implementation notes:
 *   - Ant Design Modal with forceRender keeps content in DOM (display:none).
 *     We use `.ant-modal-content:visible` to target only the open modal.
 *   - Approvals API path is /approvals (not /admin/approvals) with ?status=PENDING.
 *   - Approve/Reject API paths are /approvals/{id}/approve and /approvals/{id}/reject.
 *   - Reject modal has a required "reason" field.
 *   - Auth is injected via addInitScript so the app skips the login redirect.
 */

import { expect, test, type Page } from '@playwright/test';

// ---------------------------------------------------------------------------
// Auth helpers
// ---------------------------------------------------------------------------

function authStorageState() {
    return JSON.stringify({
        state: {
            token: 'test-token',
            user: {
                id: 'user-admin',
                username: 'admin',
                permissions: ['platform:admin'],
            },
            isAuthenticated: true,
        },
        version: 0,
    });
}

async function injectAuth(page: Page) {
    await page.addInitScript((storageValue: string) => {
        window.localStorage.setItem('shepherd-auth', storageValue);
    }, authStorageState());
}

// ---------------------------------------------------------------------------
// Baseline API mock
// ---------------------------------------------------------------------------

async function mountBaselineMock(page: Page) {
    await page.route('**/api/v1/**', async (route) => {
        const req = route.request();
        const url = new URL(req.url());
        const path = url.pathname;
        const method = req.method();

        const json = (body: unknown, status = 200) =>
            route.fulfill({
                status,
                contentType: 'application/json',
                body: JSON.stringify(body),
            });

        // ── Notifications (layout header) ──────────────────────────────────────
        if (method === 'GET' && path.endsWith('/notifications/unread-count')) {
            return json({ count: 0 });
        }
        if (method === 'GET' && path.endsWith('/notifications')) {
            return json({ items: [], pagination: { page: 1, per_page: 10, total: 0, total_pages: 0 } });
        }

        // ── RBAC roles ──────────────────────────────────────────────────────────
        if (method === 'GET' && path.endsWith('/admin/roles')) {
            return json({
                items: [
                    { id: 'role-platform-admin', name: 'PlatformAdmin', display_name: 'Platform Admin', built_in: true, enabled: true },
                    { id: 'role-custom-1', name: 'DevLead', display_name: 'Dev Lead', built_in: false, enabled: true },
                ],
                pagination: { page: 1, per_page: 20, total: 2, total_pages: 1 },
            });
        }
        if (method === 'GET' && path.endsWith('/admin/permissions')) {
            return json({
                items: [
                    { id: 'system:read', resource: 'system', action: 'read', name: 'View system' },
                    { id: 'vm:create', resource: 'vm', action: 'create', name: 'Create VM request' },
                ],
            });
        }
        if (method === 'GET' && path.endsWith('/admin/role-bindings')) {
            return json({ items: [], pagination: { page: 1, per_page: 20, total: 0, total_pages: 0 } });
        }

        // ── Users ───────────────────────────────────────────────────────────────
        if (method === 'GET' && path.endsWith('/admin/users')) {
            return json({
                items: [
                    { id: 'user-1', username: 'alice', display_name: 'Alice', email: 'alice@example.com', enabled: true, auth_type: 'local', created_at: new Date().toISOString() },
                ],
                pagination: { page: 1, per_page: 20, total: 1, total_pages: 1 },
            });
        }
        if (method === 'GET' && path.endsWith('/admin/members')) {
            return json({ items: [], pagination: { page: 1, per_page: 20, total: 0, total_pages: 0 } });
        }
        if (method === 'GET' && path.endsWith('/admin/rate-limits')) {
            return json({ items: [], pagination: { page: 1, per_page: 20, total: 0, total_pages: 0 } });
        }

        // ── Clusters ────────────────────────────────────────────────────────────
        if (method === 'GET' && path.endsWith('/admin/clusters')) {
            return json({
                items: [
                    { id: 'cluster-1', name: 'cluster-prod-01', display_name: 'Prod Cluster', status: 'HEALTHY', enabled: true, environment: 'prod', api_server_url: 'https://k8s.example.com', created_at: new Date().toISOString() },
                ],
                pagination: { page: 1, per_page: 20, total: 1, total_pages: 1 },
            });
        }

        // ── Namespaces ──────────────────────────────────────────────────────────
        if (method === 'GET' && path.endsWith('/admin/namespaces')) {
            return json({
                items: [
                    { id: 'ns-1', name: 'prod-shop', environment: 'prod', description: 'Production shop namespace', enabled: true, created_at: new Date().toISOString() },
                ],
                pagination: { page: 1, per_page: 20, total: 1, total_pages: 1 },
            });
        }
        // GET /admin/namespaces/{id} – used by openEditModal and openDeleteModal
        if (method === 'GET' && /\/admin\/namespaces\/[^/]+$/.test(path)) {
            return json({ id: 'ns-1', name: 'prod-shop', environment: 'prod', description: 'Production shop namespace', enabled: true, created_at: new Date().toISOString() });
        }

        // ── Approvals (path: /approvals, NOT /admin/approvals) ─────────────────
        // The hook calls api.GET('/approvals', { params: { query: { status: 'PENDING', ... } } })
        if (method === 'GET' && path.endsWith('/approvals')) {
            return json({
                items: [
                    {
                        id: 'ticket-pending-1',
                        status: 'PENDING',
                        operation_type: 'CREATE',
                        requester: 'alice',
                        requester_id: 'user-1',
                        created_at: new Date().toISOString(),
                        namespace: 'prod-shop',
                    },
                ],
                pagination: { page: 1, per_page: 20, total: 1, total_pages: 1 },
            });
        }

        // ── Approval actions (POST /approvals/{id}/approve|reject|cancel) ───────
        if (method === 'POST' && path.includes('/approvals/')) {
            return json({ ok: true }, 200);
        }

        // ── Generic write stubs ─────────────────────────────────────────────────
        if (method === 'POST') return json({ id: `new-${Date.now()}`, ok: true }, 201);
        if (method === 'PATCH' || method === 'PUT') return json({ ok: true }, 200);
        if (method === 'DELETE') return json({}, 204);

        return json({}, 200);
    });
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

/**
 * Get the currently visible Ant Design modal content panel.
 * Ant Design with forceRender keeps modals in DOM (display:none).
 * We use :visible CSS pseudo-class to target only the open one.
 */
function visibleModal(page: Page) {
    return page.locator('.ant-modal-content:visible');
}

// ---------------------------------------------------------------------------
// Test suite
// ---------------------------------------------------------------------------

test.describe('admin-flow mock smoke interactions', () => {
    test.beforeEach(async ({ page }) => {
        await injectAuth(page);
        await mountBaselineMock(page);
    });

    // ── Stage 2.A: RBAC role management ──────────────────────────────────────

    test('Stage 2.A – RBAC page renders role list and create button', async ({ page }) => {
        await page.goto('/admin/rbac');
        await expect(page.getByTestId('admin-rbac-page')).toBeVisible();
        await expect(page.getByTestId('rbac-role-create-button')).toBeVisible();

        // Built-in role edit/delete buttons should be disabled
        await expect(page.getByTestId('rbac-role-action-edit-role-platform-admin')).toBeDisabled();

        // Custom role edit/delete buttons should be enabled
        await expect(page.getByTestId('rbac-role-action-edit-role-custom-1')).toBeEnabled();
    });

    test('Stage 2.A – custom role create modal opens and submits', async ({ page }) => {
        const captured: Array<{ method: string }> = [];
        await page.route('**/api/v1/admin/roles', async (route) => {
            if (route.request().method() !== 'POST') return route.fallback();
            captured.push({ method: route.request().method() });
            await route.fulfill({ status: 201, contentType: 'application/json', body: JSON.stringify({ id: 'role-new', name: 'TestRole', built_in: false }) });
        });

        await page.goto('/admin/rbac');
        await page.getByTestId('rbac-role-create-button').click();

        const modal = visibleModal(page);
        await expect(modal).toBeVisible();
        await modal.getByRole('textbox').first().fill('TestRole');

        // Permissions is a required multi-select.
        // Locate it via its combobox (Ant Design renders a hidden <input role="combobox">)
        // inside the form item labelled "Permissions".
        const permissionsCombobox = modal.locator('combobox, [role="combobox"]').filter({ hasText: '' }).last();
        await permissionsCombobox.click({ force: true });
        // Wait for the dropdown to appear and select the first option
        const firstOption = page.locator('.ant-select-dropdown:visible .ant-select-item-option').first();
        await expect(firstOption).toBeVisible({ timeout: 5000 });
        await firstOption.click();
        // Close the dropdown by clicking the modal body (not the modal title which may not exist)
        await modal.locator('.ant-modal-body').click({ position: { x: 10, y: 10 } });
        await modal.getByRole('button', { name: 'OK' }).click();

        await expect.poll(() => captured.some((r) => r.method === 'POST'), { timeout: 5000 }).toBeTruthy();
    });

    test('Stage 2.A – custom role delete modal opens and submits', async ({ page }) => {
        const captured: string[] = [];
        await page.route('**/api/v1/admin/roles/**', async (route) => {
            if (route.request().method() !== 'DELETE') return route.fallback();
            captured.push(new URL(route.request().url()).pathname);
            await route.fulfill({ status: 204, body: '' });
        });

        await page.goto('/admin/rbac');
        await page.getByTestId('rbac-role-action-delete-role-custom-1').click();

        // RBAC delete uses a Modal (not Popconfirm), title is "Delete"
        const modal = visibleModal(page);
        await expect(modal).toBeVisible();
        await modal.getByRole('button', { name: 'OK' }).click();

        await expect.poll(() => captured.some((p) => p.includes('role-custom-1')), { timeout: 5000 }).toBeTruthy();
    });

    // ── Stage 2.A+: User management ──────────────────────────────────────────

    test('Stage 2.A+ – Users page renders user list and create button', async ({ page }) => {
        await page.goto('/admin/users');
        await expect(page.getByTestId('admin-users-page')).toBeVisible();
        await expect(page.getByTestId('user-create-button')).toBeVisible();
        await expect(page.locator('tr').filter({ hasText: 'alice' }).first()).toBeVisible();
    });

    test('Stage 2.A+ – user create modal opens and submits', async ({ page }) => {
        const captured: Array<{ method: string }> = [];
        await page.route('**/api/v1/admin/users', async (route) => {
            if (route.request().method() !== 'POST') return route.fallback();
            captured.push({ method: route.request().method() });
            await route.fulfill({ status: 201, contentType: 'application/json', body: JSON.stringify({ id: 'user-new', username: 'bob' }) });
        });

        await page.goto('/admin/users');
        await page.getByTestId('user-create-button').click();

        const modal = visibleModal(page);
        await expect(modal).toBeVisible();

        // Fill username and password (required fields)
        await modal.getByRole('textbox').nth(0).fill('bob');
        await modal.locator('input[type="password"]').fill('securepass123');
        await modal.getByRole('button', { name: 'OK' }).click();

        await expect.poll(() => captured.some((r) => r.method === 'POST'), { timeout: 5000 }).toBeTruthy();
    });

    test('Stage 2.A+ – user delete triggers API call via Popconfirm', async ({ page }) => {
        const captured: string[] = [];
        await page.route('**/api/v1/admin/users/**', async (route) => {
            if (route.request().method() !== 'DELETE') return route.fallback();
            captured.push(new URL(route.request().url()).pathname);
            await route.fulfill({ status: 204, body: '' });
        });

        await page.goto('/admin/users');
        await page.getByTestId('user-action-delete-user-1').click();

        // Popconfirm OK button
        await expect(page.locator('.ant-popconfirm')).toBeVisible();
        await page.locator('.ant-popconfirm').getByRole('button', { name: 'Confirm' }).click();

        await expect.poll(() => captured.some((p) => p.includes('user-1')), { timeout: 5000 }).toBeTruthy();
    });

    // ── Stage 3: Cluster registration ────────────────────────────────────────

    test('Stage 3 – Clusters page renders cluster list and create button', async ({ page }) => {
        await page.goto('/admin/clusters');
        await expect(page.getByTestId('admin-clusters-page')).toBeVisible();
        await expect(page.getByTestId('cluster-create-button')).toBeVisible();
        await expect(page.locator('tr').filter({ hasText: 'cluster-prod-01' }).first()).toBeVisible();
    });

    test('Stage 3 – cluster create modal opens and submits', async ({ page }) => {
        const captured: Array<{ method: string }> = [];
        await page.route('**/api/v1/admin/clusters', async (route) => {
            if (route.request().method() !== 'POST') return route.fallback();
            captured.push({ method: route.request().method() });
            await route.fulfill({ status: 201, contentType: 'application/json', body: JSON.stringify({ id: 'cluster-new', name: 'cluster-test-01' }) });
        });

        await page.goto('/admin/clusters');
        await page.getByTestId('cluster-create-button').click();

        const modal = visibleModal(page);
        await expect(modal).toBeVisible();

        await modal.getByRole('textbox').first().fill('cluster-test-01');
        await modal.locator('textarea').last().fill('dGVzdC1rdWJlY29uZmlnLWJhc2U2NA==');
        await modal.getByRole('button', { name: 'OK' }).click();

        await expect.poll(() => captured.some((r) => r.method === 'POST'), { timeout: 5000 }).toBeTruthy();
    });

    // ── Stage 3: Namespace management ────────────────────────────────────────

    test('Stage 3 – Namespaces page renders namespace list and create button', async ({ page }) => {
        await page.goto('/admin/namespaces');
        await expect(page.getByTestId('admin-namespaces-page')).toBeVisible();
        await expect(page.getByTestId('namespace-create-button')).toBeVisible();
        await expect(page.locator('tr').filter({ hasText: 'prod-shop' }).first()).toBeVisible();
    });

    test('Stage 3 – namespace create modal opens and submits', async ({ page }) => {
        const captured: Array<{ method: string }> = [];
        await page.route('**/api/v1/admin/namespaces', async (route) => {
            if (route.request().method() !== 'POST') return route.fallback();
            captured.push({ method: route.request().method() });
            await route.fulfill({ status: 201, contentType: 'application/json', body: JSON.stringify({ id: 'ns-new', name: 'dev-analytics' }) });
        });

        await page.goto('/admin/namespaces');
        await page.getByTestId('namespace-create-button').click();

        // Namespace modals use forceRender; target the visible one
        const modal = visibleModal(page);
        await expect(modal).toBeVisible();

        await modal.getByRole('textbox').first().fill('dev-analytics');
        // Select environment (Ant Design Select)
        await modal.locator('.ant-select-selector').first().click();
        await page.locator('.ant-select-item-option').filter({ hasText: /test/i }).first().click();
        await modal.getByRole('button', { name: 'OK' }).click();

        await expect.poll(() => captured.some((r) => r.method === 'POST'), { timeout: 5000 }).toBeTruthy();
    });

    test('Stage 3 – namespace edit modal opens and submits', async ({ page }) => {
        const captured: Array<{ method: string; path: string }> = [];
        // openEditModal first calls GET /admin/namespaces/{id} to fetch details
        await page.route('**/api/v1/admin/namespaces/ns-1', async (route) => {
            const method = route.request().method();
            if (method === 'GET') {
                await route.fulfill({
                    status: 200,
                    contentType: 'application/json',
                    body: JSON.stringify({ id: 'ns-1', name: 'prod-shop', environment: 'prod', description: 'Production shop namespace', enabled: true, created_at: new Date().toISOString() }),
                });
            } else if (method === 'PUT') {
                // Namespace update uses PUT (not PATCH)
                captured.push({ method, path: new URL(route.request().url()).pathname });
                await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ ok: true }) });
            } else {
                await route.fallback();
            }
        });

        await page.goto('/admin/namespaces');
        await page.getByTestId('namespace-action-edit-ns-1').click();

        const modal = visibleModal(page);
        await expect(modal).toBeVisible();

        await modal.locator('textarea').first().fill('Updated description');
        await modal.getByRole('button', { name: 'OK' }).click();

        await expect.poll(() => captured.some((r) => r.method === 'PUT' && r.path.includes('ns-1')), { timeout: 5000 }).toBeTruthy();
    });

    test('Stage 3 – namespace delete requires confirm_name and calls API', async ({ page }) => {
        const captured: string[] = [];
        // openDeleteModal first calls GET /admin/namespaces/{id} to fetch details
        await page.route('**/api/v1/admin/namespaces/ns-1', async (route) => {
            const method = route.request().method();
            if (method === 'GET') {
                await route.fulfill({
                    status: 200,
                    contentType: 'application/json',
                    body: JSON.stringify({ id: 'ns-1', name: 'prod-shop', environment: 'prod', description: 'Production shop namespace', enabled: true, created_at: new Date().toISOString() }),
                });
            } else if (method === 'DELETE') {
                captured.push(new URL(route.request().url()).pathname);
                await route.fulfill({ status: 204, body: '' });
            } else {
                await route.fallback();
            }
        });

        await page.goto('/admin/namespaces');
        await page.getByTestId('namespace-action-delete-ns-1').click();

        const modal = visibleModal(page);
        await expect(modal).toBeVisible();

        // Delete button should be disabled until name is typed
        // Note: Ant Design okButtonProps.disabled sets HTML disabled attribute
        const deleteBtn = modal.getByRole('button', { name: /delete/i });
        await expect(deleteBtn).toBeDisabled();

        // Use pressSequentially to trigger React synthetic onChange event
        const confirmInput = page.getByTestId('namespace-delete-confirm-input');
        await confirmInput.click();
        await confirmInput.pressSequentially('prod-shop', { delay: 50 });

        // Wait for React state to update and button to become enabled
        await expect(deleteBtn).toBeEnabled({ timeout: 5000 });

        await deleteBtn.click();
        await expect.poll(() => captured.some((p) => p.includes('ns-1')), { timeout: 5000 }).toBeTruthy();
    });

    // ── Stage 5.B: Approval / Rejection workflow ──────────────────────────────

    test('Stage 5.B – Approvals page renders pending ticket with approve/reject buttons', async ({ page }) => {
        await page.goto('/admin/approvals');
        // Wait for the table to load with mock data
        await expect(page.locator('tr').filter({ hasText: 'alice' }).first()).toBeVisible({ timeout: 10000 });
        await expect(page.getByTestId('approval-action-approve-ticket-pending-1')).toBeVisible();
        await expect(page.getByTestId('approval-action-reject-ticket-pending-1')).toBeVisible();
        await expect(page.getByTestId('approval-action-cancel-ticket-pending-1')).toBeVisible();
    });

    test('Stage 5.B – approve button opens modal and POSTs to /approvals/{id}/approve', async ({ page }) => {
        const captured: Array<{ method: string; path: string }> = [];
        // Approval actions go to /approvals/{id}/approve (not /admin/approvals)
        await page.route('**/api/v1/approvals/**', async (route) => {
            const req = route.request();
            captured.push({ method: req.method(), path: new URL(req.url()).pathname });
            await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ ok: true }) });
        });

        await page.goto('/admin/approvals');
        await expect(page.getByTestId('approval-action-approve-ticket-pending-1')).toBeVisible({ timeout: 10000 });
        await page.getByTestId('approval-action-approve-ticket-pending-1').click();

        // Approve modal is visible (no required fields for CREATE type)
        const modal = visibleModal(page);
        await expect(modal).toBeVisible();
        await modal.getByRole('button', { name: 'OK' }).click();

        await expect.poll(() =>
            captured.some((r) => r.method === 'POST' && r.path.includes('ticket-pending-1') && r.path.includes('/approve')),
            { timeout: 5000 }
        ).toBeTruthy();
    });

    test('Stage 5.B – reject button opens modal and POSTs to /approvals/{id}/reject', async ({ page }) => {
        const captured: Array<{ method: string; path: string }> = [];
        await page.route('**/api/v1/approvals/**', async (route) => {
            const req = route.request();
            captured.push({ method: req.method(), path: new URL(req.url()).pathname });
            await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ ok: true }) });
        });

        await page.goto('/admin/approvals');
        await expect(page.getByTestId('approval-action-reject-ticket-pending-1')).toBeVisible({ timeout: 10000 });
        await page.getByTestId('approval-action-reject-ticket-pending-1').click();

        const modal = visibleModal(page);
        await expect(modal).toBeVisible();

        // Reject modal has a required "reason" field
        await modal.locator('textarea').first().fill('Rejected by e2e test');
        await modal.getByRole('button', { name: 'OK' }).click();

        await expect.poll(() =>
            captured.some((r) => r.method === 'POST' && r.path.includes('ticket-pending-1') && r.path.includes('/reject')),
            { timeout: 5000 }
        ).toBeTruthy();
    });
});
