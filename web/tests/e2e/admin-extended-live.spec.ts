/**
 * Admin Extended Live E2E Tests — Contract-Enforced (no mock, no skip)
 *
 * ┌─────────────────────────────────────────────────────────────────────────┐
 * │  REQUIRES: a running backend (db + server via docker-compose or local)  │
 * │  NO test.skip() — failures expose real frontend/backend problems.       │
 * │  Every API response is validated against api/openapi.yaml schema.       │
 * └─────────────────────────────────────────────────────────────────────────┘
 *
 * Coverage (all previously uncovered admin endpoints):
 *   updateRole                  – PATCH /admin/roles/{id}                    → Role
 *   updateUser                  – PATCH /admin/users/{id}                    → User
 *   listUserRoleBindings        – GET /admin/users/{id}/role-bindings        → GlobalRoleBindingList
 *   createUserRoleBinding       – POST /admin/users/{id}/role-bindings       → GlobalRoleBinding
 *   deleteUserRoleBinding       – DELETE /admin/users/{id}/role-bindings/{id}→ 204
 *   listPermissions             – GET /admin/permissions                     → PermissionList
 *   updateAdminTemplate         – PATCH /admin/templates/{id}                → Template
 *   deleteAdminTemplate         – DELETE /admin/templates/{id}               → 204
 *   updateAdminInstanceSize     – PATCH /admin/instance-sizes/{id}           → InstanceSize
 *   deleteAdminInstanceSize     – DELETE /admin/instance-sizes/{id}          → 204
 *   testAuthProviderConnection  – POST /admin/auth-providers/{id}/test-connection → AuthProviderConnectionTestResult
 *   syncAuthProviderGroups      – POST /admin/auth-providers/{id}/sync       → AuthProviderGroupSyncResponse
 *   getAuthProviderSample       – GET /admin/auth-providers/{id}/sample      → (schema check)
 *   createAuthProviderGroupMapping  – POST /admin/auth-providers/{id}/group-mappings → IdPGroupMapping
 *   updateAuthProviderGroupMapping  – PATCH /admin/auth-providers/{id}/group-mappings/{id} → IdPGroupMapping
 *   deleteAuthProviderGroupMapping  – DELETE /admin/auth-providers/{id}/group-mappings/{id} → 204
 *   createRateLimitExemption    – POST /admin/rate-limits/exemptions         → 200
 *   deleteRateLimitExemption    – DELETE /admin/rate-limits/exemptions/{id}  → 204
 *   listRateLimitExemptions     – GET /admin/rate-limits/exemptions          → RateLimitExemptionList
 *   updateRateLimitUserOverrides– PUT /admin/rate-limits/users/{id}          → RateLimitUserOverride
 *   listRateLimitStatus         – GET /admin/rate-limits/status              → RateLimitStatusList
 *   updateClusterEnvironment    – PUT /admin/clusters/{id}/environment       → Cluster
 *   updateAuthProvider          – PATCH /admin/auth-providers/{id}           → AuthProvider
 *
 * Environment variables:
 *   E2E_USERNAME  – admin username (default: e2e-admin)
 *   E2E_PASSWORD  – admin password (default: e2e-admin-123)
 */

import { expect, test, type Page, type Response } from '@playwright/test';
import { validateApiResponse } from './lib/schema-validator';

// ── Config ────────────────────────────────────────────────────────────────────

const e2eUsername = process.env.E2E_USERNAME ?? 'e2e-admin';
const e2ePassword = process.env.E2E_PASSWORD ?? 'e2e-admin-123';

// ── Auth helper ───────────────────────────────────────────────────────────────

async function login(page: Page): Promise<void> {
    await page.goto('/login');
    await expect(page.getByRole('heading', { name: 'KubeVirt Shepherd' })).toBeVisible();
    await page.getByPlaceholder('Username').fill(e2eUsername);
    await page.getByPlaceholder('Password').fill(e2ePassword);
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

test.describe('admin-extended live (contract-enforced, no mock, no skip)', () => {
    test.beforeEach(async ({ page }) => {
        await login(page);
    });

    // ── listPermissions: GET /admin/permissions → PermissionList ──────────────

    test('listPermissions – GET /admin/permissions conforms to PermissionList schema', async ({ page }) => {
        // operationId: listPermissions
        const respPromise = page.waitForResponse(
            (r) => r.url().endsWith('/api/v1/admin/permissions') && r.request().method() === 'GET'
        );
        await page.goto('/admin/permissions');
        await expect(page.locator('body')).toBeVisible();
        await expectSchema(respPromise, 'PermissionList', 200);
    });

    // ── updateRole: PATCH /admin/roles/{id} → Role ───────────────────────────

    test('updateRole – PATCH /admin/roles/{id} conforms to Role schema', async ({ page }) => {
        // operationId: updateRole
        // Create a role to update
        await page.goto('/admin/rbac');
        await expect(page.getByTestId('admin-rbac-page')).toBeVisible();

        const roleName = `e2e-upd-role-${Date.now().toString(36).slice(-5)}`;
        const createRespPromise = page.waitForResponse(
            (r) => r.url().endsWith('/api/v1/admin/roles') && r.request().method() === 'POST'
        );
        await page.getByTestId('rbac-role-create-button').click();
        const createModal = page.locator('.ant-modal-content').filter({ hasText: /role/i }).last();
        await expect(createModal).toBeVisible();
        await createModal.getByRole('textbox').first().fill(roleName);
        await createModal.getByRole('button', { name: 'OK' }).click();

        const { body: created } = await expectSchema(createRespPromise, 'Role', 201);
        const roleID = (created as { id?: string }).id ?? '';
        expect(roleID, 'POST /admin/roles response missing id').toBeTruthy();

        // ── PATCH /admin/roles/{id} → Role ────────────────────────────────────────
        const updateRespPromise = page.waitForResponse(
            (r) => r.url().includes(`/api/v1/admin/roles/${roleID}`) && r.request().method() === 'PATCH'
        );
        await page.getByTestId(`rbac-role-action-edit-${roleID}`).click();
        const editModal = page.locator('.ant-modal-content').filter({ hasText: /edit.*role/i }).last();
        await expect(editModal).toBeVisible();
        await editModal.locator('textarea').first().fill('Updated by live e2e test');
        await editModal.getByRole('button', { name: 'OK' }).click();

        await expectSchema(updateRespPromise, 'Role', 200);

        // Cleanup
        const deleteRespPromise = page.waitForResponse(
            (r) => r.url().includes(`/api/v1/admin/roles/${roleID}`) && r.request().method() === 'DELETE'
        );
        await page.getByTestId(`rbac-role-action-delete-${roleID}`).click();
        const confirmBtn = page.getByRole('button', { name: /ok|confirm|delete/i }).last();
        await confirmBtn.click();
        expect((await deleteRespPromise).status()).toBe(204);
    });

    // ── updateUser + listUserRoleBindings + createUserRoleBinding + deleteUserRoleBinding

    test('updateUser – PATCH /admin/users/{id} conforms to User schema', async ({ page }) => {
        // operationId: updateUser
        // Create a user to update
        await page.goto('/admin/users');
        await expect(page.getByTestId('admin-users-page')).toBeVisible();

        const username = `e2eupd${Date.now().toString(36).slice(-5)}`;
        const createRespPromise = page.waitForResponse(
            (r) => r.url().endsWith('/api/v1/admin/users') && r.request().method() === 'POST'
        );
        await page.getByTestId('user-create-button').click();
        const createModal = page.getByTestId('user-create-modal');
        await expect(createModal).toBeVisible();
        await createModal.getByRole('textbox').nth(0).fill(username);
        await createModal.locator('input[type="password"]').fill('Secure@Pass123');
        await createModal.getByRole('button', { name: 'OK' }).click();

        const { body: created } = await expectSchema(createRespPromise, 'User', 201);
        const userID = (created as { id?: string }).id ?? '';
        expect(userID, 'POST /admin/users response missing id').toBeTruthy();

        // ── PATCH /admin/users/{id} → User ───────────────────────────────────────
        const updateRespPromise = page.waitForResponse(
            (r) => r.url().includes(`/api/v1/admin/users/${userID}`) && r.request().method() === 'PATCH'
        );
        await page.getByTestId(`user-action-edit-${userID}`).click();
        const editModal = page.getByTestId('user-edit-modal');
        await expect(editModal).toBeVisible();
        // Update display name or email
        const displayNameInput = editModal.getByRole('textbox').first();
        await displayNameInput.fill('Updated E2E User');
        await editModal.getByRole('button', { name: 'OK' }).click();

        await expectSchema(updateRespPromise, 'User', 200);

        // Cleanup
        const deleteRespPromise = page.waitForResponse(
            (r) => r.url().includes(`/api/v1/admin/users/${userID}`) && r.request().method() === 'DELETE'
        );
        await page.getByTestId(`user-action-delete-${userID}`).click();
        const confirmBtn = page.getByRole('button', { name: /confirm/i }).last();
        await confirmBtn.click();
        expect((await deleteRespPromise).status()).toBe(204);
    });

    test('listUserRoleBindings – GET /admin/users/{id}/role-bindings conforms to GlobalRoleBindingList schema', async ({ page }) => {
        // operationId: listUserRoleBindings
        // Get first user from list
        const listRespPromise = page.waitForResponse(
            (r) => r.url().endsWith('/api/v1/admin/users') && r.request().method() === 'GET'
        );
        await page.goto('/admin/users');
        await expect(page.getByTestId('admin-users-page')).toBeVisible();
        // Use validateApiResponse to parse body (single-read safe)
        const listBody = await validateApiResponse('UserList', await listRespPromise) as { items?: Array<{ id?: string }> };
        const items = listBody.items ?? [];
        expect(items.length, 'No users found — seed data must include at least one user').toBeGreaterThan(0);
        const userID = items[0]?.id ?? '';
        expect(userID).toBeTruthy();

        // ── GET /admin/users/{id}/role-bindings → GlobalRoleBindingList ───────────
        const bindingsRespPromise = page.waitForResponse(
            (r) => r.url().includes(`/api/v1/admin/users/${userID}/role-bindings`) && r.request().method() === 'GET'
        );
        await page.getByTestId(`user-action-role-bindings-${userID}`).click();
        await expectSchema(bindingsRespPromise, 'GlobalRoleBindingList', 200);
    });

    test('createUserRoleBinding + deleteUserRoleBinding – full lifecycle', async ({ page }) => {
        // operationId: createUserRoleBinding, deleteUserRoleBinding
        // Get first user (use validateApiResponse for single-read safety)
        const usersRespPromise = page.waitForResponse(
            (r) => r.url().endsWith('/api/v1/admin/users') && r.request().method() === 'GET'
        );
        await page.goto('/admin/users');
        await expect(page.getByTestId('admin-users-page')).toBeVisible();
        const usersBody = await validateApiResponse('UserList', await usersRespPromise) as { items?: Array<{ id?: string }> };
        const userID = usersBody.items?.[0]?.id ?? '';
        expect(userID, 'No users found for role binding test').toBeTruthy();

        // Navigate to user role bindings
        await page.getByTestId(`user-action-role-bindings-${userID}`).click();
        const bindingsPage = page.getByTestId('user-role-bindings-page');
        await expect(bindingsPage).toBeVisible();

        // ── POST /admin/users/{id}/role-bindings → GlobalRoleBinding ─────────────
        const createRespPromise = page.waitForResponse(
            (r) => r.url().includes(`/api/v1/admin/users/${userID}/role-bindings`) && r.request().method() === 'POST'
        );
        await page.getByTestId('role-binding-create-button').click();
        const createModal = page.getByTestId('role-binding-create-modal');
        await expect(createModal).toBeVisible();
        // Select a role from dropdown
        await createModal.locator('.ant-select-selector').first().click();
        await page.locator('.ant-select-item-option').first().click();
        await createModal.getByRole('button', { name: 'OK' }).click();

        const { body: created } = await expectSchema(createRespPromise, 'GlobalRoleBinding', 201);
        const bindingID = (created as { id?: string }).id ?? '';
        expect(bindingID, 'POST /admin/users/{id}/role-bindings response missing id').toBeTruthy();

        // ── DELETE /admin/users/{id}/role-bindings/{id} → 204 ────────────────────
        const deleteRespPromise = page.waitForResponse(
            (r) => r.url().includes(`/api/v1/admin/users/${userID}/role-bindings/${bindingID}`) && r.request().method() === 'DELETE'
        );
        await page.getByTestId(`role-binding-action-delete-${bindingID}`).click();
        const confirmBtn = page.getByRole('button', { name: /confirm|ok/i }).last();
        await confirmBtn.click();
        expect((await deleteRespPromise).status()).toBe(204);
    });

    // ── updateAdminTemplate + deleteAdminTemplate ─────────────────────────────

    test('updateAdminTemplate – PATCH /admin/templates/{id} conforms to Template schema', async ({ page }) => {
        // operationId: updateAdminTemplate, deleteAdminTemplate
        // Create a template to update
        await page.goto('/admin/templates');
        await expect(page.getByRole('heading', { name: 'Templates' })).toBeVisible();

        const tplName = `e2e-tpl-${Date.now().toString(36).slice(-5)}`;
        const createRespPromise = page.waitForResponse(
            (r) => r.url().endsWith('/api/v1/admin/templates') && r.request().method() === 'POST'
        );
        await page.getByTestId('template-create-button').click();
        const createModal = page.getByTestId('template-create-modal');
        await expect(createModal).toBeVisible();
        await createModal.getByRole('textbox').first().fill(tplName);
        // Fill minimal required YAML
        const yamlEditor = createModal.locator('textarea').last();
        await yamlEditor.fill('apiVersion: kubevirt.io/v1\nkind: VirtualMachineInstance\nmetadata:\n  name: test\nspec: {}');
        await createModal.getByRole('button', { name: 'OK' }).click();

        const { body: created } = await expectSchema(createRespPromise, 'Template', 201);
        const tplID = (created as { id?: string }).id ?? '';
        expect(tplID, 'POST /admin/templates response missing id').toBeTruthy();

        // ── PATCH /admin/templates/{id} → Template ────────────────────────────────
        const updateRespPromise = page.waitForResponse(
            (r) => r.url().includes(`/api/v1/admin/templates/${tplID}`) && r.request().method() === 'PATCH'
        );
        await page.getByTestId(`template-action-edit-${tplID}`).click();
        const editModal = page.getByTestId('template-edit-modal');
        await expect(editModal).toBeVisible();
        await editModal.locator('textarea').first().fill('Updated by live e2e test');
        await editModal.getByRole('button', { name: 'OK' }).click();

        await expectSchema(updateRespPromise, 'Template', 200);

        // ── DELETE /admin/templates/{id} → 204 ───────────────────────────────────
        const deleteRespPromise = page.waitForResponse(
            (r) => r.url().includes(`/api/v1/admin/templates/${tplID}`) && r.request().method() === 'DELETE'
        );
        await page.getByTestId(`template-action-delete-${tplID}`).click();
        const confirmBtn = page.getByRole('button', { name: /confirm|ok|delete/i }).last();
        await confirmBtn.click();
        expect((await deleteRespPromise).status()).toBe(204);
    });

    // ── updateAdminInstanceSize + deleteAdminInstanceSize ─────────────────────

    test('updateAdminInstanceSize – PATCH /admin/instance-sizes/{id} conforms to InstanceSize schema', async ({ page }) => {
        // operationId: updateAdminInstanceSize, deleteAdminInstanceSize
        await page.goto('/admin/instance-sizes');
        await expect(page.getByRole('heading', { name: 'Instance Sizes' })).toBeVisible();

        const sizeName = `e2e-size-${Date.now().toString(36).slice(-5)}`;
        const createRespPromise = page.waitForResponse(
            (r) => r.url().endsWith('/api/v1/admin/instance-sizes') && r.request().method() === 'POST'
        );
        await page.getByTestId('instance-size-create-button').click();
        const createModal = page.getByTestId('instance-size-create-modal');
        await expect(createModal).toBeVisible();
        await createModal.getByRole('textbox').first().fill(sizeName);
        // Fill CPU and memory fields
        await createModal.locator('input[type="number"]').nth(0).fill('2');
        await createModal.locator('input[type="number"]').nth(1).fill('4');
        await createModal.getByRole('button', { name: 'OK' }).click();

        const { body: created } = await expectSchema(createRespPromise, 'InstanceSize', 201);
        const sizeID = (created as { id?: string }).id ?? '';
        expect(sizeID, 'POST /admin/instance-sizes response missing id').toBeTruthy();

        // ── PATCH /admin/instance-sizes/{id} → InstanceSize ──────────────────────
        const updateRespPromise = page.waitForResponse(
            (r) => r.url().includes(`/api/v1/admin/instance-sizes/${sizeID}`) && r.request().method() === 'PATCH'
        );
        await page.getByTestId(`instance-size-action-edit-${sizeID}`).click();
        const editModal = page.getByTestId('instance-size-edit-modal');
        await expect(editModal).toBeVisible();
        await editModal.locator('input[type="number"]').nth(0).fill('4');
        await editModal.getByRole('button', { name: 'OK' }).click();

        await expectSchema(updateRespPromise, 'InstanceSize', 200);

        // ── DELETE /admin/instance-sizes/{id} → 204 ──────────────────────────────
        const deleteRespPromise = page.waitForResponse(
            (r) => r.url().includes(`/api/v1/admin/instance-sizes/${sizeID}`) && r.request().method() === 'DELETE'
        );
        await page.getByTestId(`instance-size-action-delete-${sizeID}`).click();
        const confirmBtn = page.getByRole('button', { name: /confirm|ok|delete/i }).last();
        await confirmBtn.click();
        expect((await deleteRespPromise).status()).toBe(204);
    });

    // ── Auth Provider: update + test-connection + sync + sample + group-mapping CRUD

    test('updateAuthProvider – PATCH /admin/auth-providers/{id} conforms to AuthProvider schema', async ({ page }) => {
        // operationId: updateAuthProvider
        // Get first provider (use validateApiResponse for single-read safety)
        const listRespPromise = page.waitForResponse(
            (r) => r.url().endsWith('/api/v1/admin/auth-providers') && r.request().method() === 'GET'
        );
        await page.goto('/admin/auth-providers');
        await expect(page.getByRole('heading', { name: 'Authentication Providers' })).toBeVisible();
        const listBody = await validateApiResponse('AuthProviderList', await listRespPromise) as { items?: Array<{ id?: string }> };
        const items = listBody.items ?? [];
        expect(items.length, 'No auth providers found — seed data must include at least one provider').toBeGreaterThan(0);
        const providerID = items[0]?.id ?? '';
        expect(providerID).toBeTruthy();

        // ── PATCH /admin/auth-providers/{id} → AuthProvider ──────────────────────
        const updateRespPromise = page.waitForResponse(
            (r) => r.url().includes(`/api/v1/admin/auth-providers/${providerID}`) && r.request().method() === 'PATCH'
        );
        await page.getByTestId(`auth-provider-action-edit-${providerID}`).click();
        const editModal = page.locator('.ant-modal-content').filter({ hasText: /edit.*provider|update.*provider/i }).last();
        await expect(editModal).toBeVisible();
        await editModal.locator('textarea').first().fill('{"issuer":"https://updated-idp.example.com"}');
        await editModal.getByRole('button', { name: 'OK' }).click();

        await expectSchema(updateRespPromise, 'AuthProvider', 200);
    });

    test('testAuthProviderConnection – POST /admin/auth-providers/{id}/test-connection conforms to AuthProviderConnectionTestResult schema', async ({ page }) => {
        // operationId: testAuthProviderConnection
        const listRespPromise = page.waitForResponse(
            (r) => r.url().endsWith('/api/v1/admin/auth-providers') && r.request().method() === 'GET'
        );
        await page.goto('/admin/auth-providers');
        await expect(page.getByRole('heading', { name: 'Authentication Providers' })).toBeVisible();
        const listBody = await validateApiResponse('AuthProviderList', await listRespPromise) as { items?: Array<{ id?: string }> };
        const providerID = listBody.items?.[0]?.id ?? '';
        expect(providerID, 'No auth providers found').toBeTruthy();

        // ── POST /admin/auth-providers/{id}/test-connection ───────────────────────
        const testRespPromise = page.waitForResponse(
            (r) => r.url().endsWith(`/api/v1/admin/auth-providers/${providerID}/test-connection`) && r.request().method() === 'POST'
        );
        await page.getByTestId(`auth-provider-action-test-${providerID}`).click();
        await expectSchema(testRespPromise, 'AuthProviderConnectionTestResult', 200);
    });

    test('syncAuthProviderGroups – POST /admin/auth-providers/{id}/sync conforms to AuthProviderGroupSyncResponse schema', async ({ page }) => {
        // operationId: syncAuthProviderGroups
        const listRespPromise = page.waitForResponse(
            (r) => r.url().endsWith('/api/v1/admin/auth-providers') && r.request().method() === 'GET'
        );
        await page.goto('/admin/auth-providers');
        await expect(page.getByRole('heading', { name: 'Authentication Providers' })).toBeVisible();
        const listBody = await validateApiResponse('AuthProviderList', await listRespPromise) as { items?: Array<{ id?: string }> };
        const providerID = listBody.items?.[0]?.id ?? '';
        expect(providerID, 'No auth providers found').toBeTruthy();

        // ── POST /admin/auth-providers/{id}/sync ──────────────────────────────────
        const syncRespPromise = page.waitForResponse(
            (r) => r.url().endsWith(`/api/v1/admin/auth-providers/${providerID}/sync`) && r.request().method() === 'POST'
        );
        await page.getByTestId(`auth-provider-action-sync-${providerID}`).click();
        const confirmBtn = page.locator('.ant-popover:visible, .ant-modal-content:visible')
            .getByRole('button', { name: /confirm|ok|sync/i }).first();
        if (await confirmBtn.count() > 0) await confirmBtn.click();
        await expectSchema(syncRespPromise, 'AuthProviderGroupSyncResponse', 200);
    });

    test('getAuthProviderSample – GET /admin/auth-providers/{id}/sample returns 200', async ({ page }) => {
        // operationId: getAuthProviderSample
        const listRespPromise = page.waitForResponse(
            (r) => r.url().endsWith('/api/v1/admin/auth-providers') && r.request().method() === 'GET'
        );
        await page.goto('/admin/auth-providers');
        await expect(page.getByRole('heading', { name: 'Authentication Providers' })).toBeVisible();
        const listBody = await validateApiResponse('AuthProviderList', await listRespPromise) as { items?: Array<{ id?: string }> };
        const providerID = listBody.items?.[0]?.id ?? '';
        expect(providerID, 'No auth providers found').toBeTruthy();

        // ── GET /admin/auth-providers/{id}/sample ─────────────────────────────────
        const sampleRespPromise = page.waitForResponse(
            (r) => r.url().endsWith(`/api/v1/admin/auth-providers/${providerID}/sample`) && r.request().method() === 'GET'
        );
        await page.getByTestId(`auth-provider-action-sample-${providerID}`).click();
        const sampleResp = await sampleRespPromise;
        expect(sampleResp.status(), `GET /admin/auth-providers/${providerID}/sample returned ${sampleResp.status()}`).toBe(200);
        // ── CONTRACT CHECK: AuthProviderSampleResponse schema ─────────────────────
        await validateApiResponse('AuthProviderSampleResponse', sampleResp);
    });

    test('createAuthProviderGroupMapping + updateAuthProviderGroupMapping + deleteAuthProviderGroupMapping', async ({ page }) => {
        // operationId: createAuthProviderGroupMapping, updateAuthProviderGroupMapping, deleteAuthProviderGroupMapping
        const listRespPromise = page.waitForResponse(
            (r) => r.url().endsWith('/api/v1/admin/auth-providers') && r.request().method() === 'GET'
        );
        await page.goto('/admin/auth-providers');
        await expect(page.getByRole('heading', { name: 'Authentication Providers' })).toBeVisible();
        const listBody = await validateApiResponse('AuthProviderList', await listRespPromise) as { items?: Array<{ id?: string }> };
        const providerID = listBody.items?.[0]?.id ?? '';
        expect(providerID, 'No auth providers found').toBeTruthy();

        // Navigate to group mappings
        await page.getByTestId(`auth-provider-action-mappings-${providerID}`).click();
        const mappingsPage = page.getByTestId('auth-provider-mappings-page');
        await expect(mappingsPage).toBeVisible();

        // ── POST /admin/auth-providers/{id}/group-mappings → IdPGroupMapping ──────
        const createRespPromise = page.waitForResponse(
            (r) => r.url().includes(`/api/v1/admin/auth-providers/${providerID}/group-mappings`) && r.request().method() === 'POST'
        );
        await page.getByTestId('group-mapping-create-button').click();
        const createModal = page.getByTestId('group-mapping-create-modal');
        await expect(createModal).toBeVisible();
        await createModal.getByRole('textbox').first().fill(`e2e-group-${Date.now().toString(36).slice(-5)}`);
        await createModal.locator('.ant-select-selector').first().click();
        await page.locator('.ant-select-item-option').first().click();
        await createModal.getByRole('button', { name: 'OK' }).click();

        const { body: created } = await expectSchema(createRespPromise, 'IdPGroupMapping', 201);
        const mappingID = (created as { id?: string }).id ?? '';
        expect(mappingID, 'POST group-mappings response missing id').toBeTruthy();

        // ── PATCH /admin/auth-providers/{id}/group-mappings/{id} → IdPGroupMapping
        const updateRespPromise = page.waitForResponse(
            (r) => r.url().includes(`/api/v1/admin/auth-providers/${providerID}/group-mappings/${mappingID}`) && r.request().method() === 'PATCH'
        );
        await page.getByTestId(`group-mapping-action-edit-${mappingID}`).click();
        const editModal = page.getByTestId('group-mapping-edit-modal');
        await expect(editModal).toBeVisible();
        await editModal.locator('.ant-select-selector').first().click();
        await page.locator('.ant-select-item-option').last().click();
        await editModal.getByRole('button', { name: 'OK' }).click();

        await expectSchema(updateRespPromise, 'IdPGroupMapping', 200);

        // ── DELETE /admin/auth-providers/{id}/group-mappings/{id} → 204 ──────────
        const deleteRespPromise = page.waitForResponse(
            (r) => r.url().includes(`/api/v1/admin/auth-providers/${providerID}/group-mappings/${mappingID}`) && r.request().method() === 'DELETE'
        );
        await page.getByTestId(`group-mapping-action-delete-${mappingID}`).click();
        const confirmBtn = page.getByRole('button', { name: /confirm|ok/i }).last();
        await confirmBtn.click();
        expect((await deleteRespPromise).status()).toBe(204);
    });

    // ── Rate Limit: full coverage ─────────────────────────────────────────────

    test('listRateLimitExemptions – GET /admin/rate-limits/exemptions conforms to RateLimitExemptionList schema', async ({ page }) => {
        // operationId: listRateLimitExemptions
        const respPromise = page.waitForResponse(
            (r) => r.url().includes('/api/v1/admin/rate-limits/exemptions') && r.request().method() === 'GET'
                && !r.url().includes('/status') && !r.url().includes('/users/')
        );
        await page.goto('/admin/rate-limits');
        await expect(page.locator('body')).toBeVisible();
        // ── CONTRACT CHECK: RateLimitExemptionList schema ─────────────────────────
        await expectSchema(respPromise, 'RateLimitExemptionList', 200);
    });

    test('listRateLimitStatus – GET /admin/rate-limits/status conforms to RateLimitStatusList schema', async ({ page }) => {
        // operationId: listRateLimitStatus
        const respPromise = page.waitForResponse(
            (r) => r.url().endsWith('/api/v1/admin/rate-limits/status') && r.request().method() === 'GET'
        );
        await page.goto('/admin/rate-limits');
        await expect(page.locator('body')).toBeVisible();
        await expectSchema(respPromise, 'RateLimitStatusList', 200);
    });

    test('createRateLimitExemption + deleteRateLimitExemption – full lifecycle', async ({ page }) => {
        // operationId: createRateLimitExemption, deleteRateLimitExemption
        // Get first user to exempt (use validateApiResponse for single-read safety)
        const usersRespPromise = page.waitForResponse(
            (r) => r.url().endsWith('/api/v1/admin/users') && r.request().method() === 'GET'
        );
        await page.goto('/admin/users');
        await expect(page.getByTestId('admin-users-page')).toBeVisible();
        const usersBody = await validateApiResponse('UserList', await usersRespPromise) as { items?: Array<{ id?: string }> };
        const userID = usersBody.items?.[0]?.id ?? '';
        expect(userID, 'No users found for rate limit exemption test').toBeTruthy();

        // Navigate to rate limits page
        await page.goto('/admin/rate-limits');
        await expect(page.locator('body')).toBeVisible();

        // ── POST /admin/rate-limits/exemptions ────────────────────────────────────
        const createRespPromise = page.waitForResponse(
            (r) => r.url().endsWith('/api/v1/admin/rate-limits/exemptions') && r.request().method() === 'POST'
        );
        await page.getByTestId('rate-limit-exemption-create-button').click();
        const createModal = page.getByTestId('rate-limit-exemption-create-modal');
        await expect(createModal).toBeVisible();
        // Select user
        await createModal.locator('.ant-select-selector').first().click();
        await page.locator('.ant-select-item-option').first().click();
        await createModal.getByRole('button', { name: 'OK' }).click();

        const createResp = await createRespPromise;
        expect(createResp.status(), `POST /admin/rate-limits/exemptions returned ${createResp.status()}`).toBe(200);
        // ── CONTRACT CHECK: RateLimitExemption schema ─────────────────────────────
        await validateApiResponse('RateLimitExemption', createResp);

        // ── DELETE /admin/rate-limits/exemptions/{user_id} → 204 ─────────────────
        const deleteRespPromise = page.waitForResponse(
            (r) => r.url().includes(`/api/v1/admin/rate-limits/exemptions/${userID}`) && r.request().method() === 'DELETE'
        );
        await page.getByTestId(`rate-limit-exemption-action-delete-${userID}`).click();
        const confirmBtn = page.getByRole('button', { name: /confirm|ok/i }).last();
        await confirmBtn.click();
        expect((await deleteRespPromise).status()).toBe(204);
    });

    test('updateRateLimitUserOverrides – PUT /admin/rate-limits/users/{id} conforms to RateLimitUserOverride schema', async ({ page }) => {
        // operationId: updateRateLimitUserOverrides
        // Get first user (use validateApiResponse for single-read safety)
        const usersRespPromise = page.waitForResponse(
            (r) => r.url().endsWith('/api/v1/admin/users') && r.request().method() === 'GET'
        );
        await page.goto('/admin/users');
        await expect(page.getByTestId('admin-users-page')).toBeVisible();
        const usersBody = await validateApiResponse('UserList', await usersRespPromise) as { items?: Array<{ id?: string }> };
        const userID = usersBody.items?.[0]?.id ?? '';
        expect(userID, 'No users found for rate limit override test').toBeTruthy();

        // Navigate to rate limit override for this user
        await page.goto('/admin/rate-limits');
        await expect(page.locator('body')).toBeVisible();

        const updateRespPromise = page.waitForResponse(
            (r) => r.url().includes(`/api/v1/admin/rate-limits/users/${userID}`) && r.request().method() === 'PUT'
        );
        await page.getByTestId(`rate-limit-user-action-edit-${userID}`).click();
        const editModal = page.getByTestId('rate-limit-user-edit-modal');
        await expect(editModal).toBeVisible();
        await editModal.locator('input[type="number"]').first().fill('100');
        await editModal.getByRole('button', { name: 'OK' }).click();

        await expectSchema(updateRespPromise, 'RateLimitUserOverride', 200);
    });

    // ── Cluster: updateClusterEnvironment ────────────────────────────────────

    test('updateClusterEnvironment – PUT /admin/clusters/{id}/environment conforms to Cluster schema', async ({ page }) => {
        // operationId: updateClusterEnvironment
        const listRespPromise = page.waitForResponse(
            (r) => r.url().endsWith('/api/v1/admin/clusters') && r.request().method() === 'GET'
        );
        await page.goto('/admin/clusters');
        await expect(page.getByTestId('admin-clusters-page')).toBeVisible();
        const listBody = await validateApiResponse('ClusterList', await listRespPromise) as { items?: Array<{ id?: string }> };
        const items = listBody.items ?? [];
        expect(items.length, 'No clusters found — seed data must include at least one cluster').toBeGreaterThan(0);
        const clusterID = items[0]?.id ?? '';
        expect(clusterID).toBeTruthy();

        // ── PUT /admin/clusters/{id}/environment → Cluster ────────────────────────
        const updateRespPromise = page.waitForResponse(
            (r) => r.url().includes(`/api/v1/admin/clusters/${clusterID}/environment`) && r.request().method() === 'PUT'
        );
        await page.getByTestId(`cluster-action-set-environment-${clusterID}`).click();
        const editModal = page.getByTestId('cluster-environment-modal');
        await expect(editModal).toBeVisible();
        // Select environment type
        await editModal.locator('.ant-select-selector').first().click();
        await page.locator('.ant-select-item-option').filter({ hasText: /test|staging/i }).first().click();
        await editModal.getByRole('button', { name: 'OK' }).click();

        await expectSchema(updateRespPromise, 'Cluster', 200);
    });
});
