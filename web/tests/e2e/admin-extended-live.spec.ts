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
 *   testAuthProviderConnection      – POST /admin/auth-providers/{id}/test-connection → AuthProviderConnectionTestResult
 *   getAuthProviderSample           – GET /admin/auth-providers/{id}/sample            → AuthProviderSampleResponse
 *   getAuthProviderDirectoryDescriptor – GET /admin/auth-providers/{id}/directory/descriptor → DirectorySyncDescriptor
 *   previewAuthProviderDirectory    – POST /admin/auth-providers/{id}/directory/preview → DirectorySyncPreview
 *   triggerAuthProviderDirectorySync – POST /admin/auth-providers/{id}/directory/sync   → DirectorySyncTriggerResponse
 *   listAuthProviderDirectorySyncJobs – GET /admin/auth-providers/{id}/directory/sync-jobs → DirectorySyncJobList
 *   getAuthProviderDirectorySyncJob – GET /admin/auth-providers/{id}/directory/sync-jobs/{id} → DirectorySyncJob
 *   createAuthProviderCohortMapping – POST /admin/auth-providers/{id}/cohort-mappings  → ExternalCohortMapping
 *   updateAuthProviderCohortMapping – PATCH /admin/auth-providers/{id}/cohort-mappings/{id} → ExternalCohortMapping
 *   deleteAuthProviderCohortMapping – DELETE /admin/auth-providers/{id}/cohort-mappings/{id} → 204
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
 *   E2E_NEW_PASSWORD – password used when force_password_change=true
 */

import { expect, test, type APIRequestContext, type Page, type Response } from '@playwright/test';
import { validateApiResponse } from './lib/schema-validator';
import {
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
let activePassword = e2ePassword;
let seededRateLimitUserID = '';

interface ApiList<T> {
    items?: T[];
}

async function getAdminAuthHeaders(request: APIRequestContext): Promise<{ Authorization: string }> {
    const auth = await getApiTokenWithForcePasswordSupport(request, {
        username: e2eUsername,
        primaryPassword: e2ePassword,
        secondaryPassword: e2eNewPassword,
        currentPasswordHint: activePassword,
    });
    activePassword = auth.password;
    return { Authorization: `Bearer ${auth.token}` };
}

async function createGenericAuthProvider(
    request: APIRequestContext,
    headers: { Authorization: string },
    overrides?: {
        name?: string;
        config?: Record<string, unknown>;
    }
): Promise<{ id: string; name: string }> {
    const name = overrides?.name ?? `e2e-auth-${Date.now().toString(36).slice(-6)}`;
    const createResp = await request.post('/api/v1/admin/auth-providers', {
        headers,
        data: {
            name,
            auth_type: 'generic',
            enabled: true,
            config: {
                test_endpoint: 'https://idp.example.com/healthz',
                sample_users: [
                    {
                        external_id: 'e2e-alice',
                        username: 'alice',
                        display_name: 'Alice Example',
                        email: 'alice@example.com',
                        groups: ['ops'],
                        department: 'platform',
                    },
                    {
                        external_id: 'e2e-bob',
                        username: 'bob',
                        display_name: 'Bob Example',
                        email: 'bob@example.com',
                        groups: ['dev'],
                        department: 'engineering',
                    },
                ],
                ...(overrides?.config ?? {}),
            },
        },
    });
    expect(createResp.status(), `POST /admin/auth-providers returned ${createResp.status()}`).toBe(201);
    const created = await validateApiResponse('AuthProvider', createResp) as { id?: string; name?: string };
    const id = created.id ?? '';
    expect(id, 'Created auth provider id is required').toBeTruthy();
    return { id, name };
}

async function deleteAuthProviderIfPresent(
    request: APIRequestContext,
    headers: { Authorization: string },
    providerID: string
): Promise<void> {
    if (!providerID) {
        return;
    }
    const resp = await request.delete(`/api/v1/admin/auth-providers/${providerID}`, { headers });
    expect([204, 404], `DELETE /admin/auth-providers/${providerID} returned ${resp.status()}`).toContain(resp.status());
}

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

async function seedRateLimitStatusUser(request: APIRequestContext, headers: { Authorization: string }): Promise<string> {
    const meResp = await request.get('/api/v1/auth/me', { headers });
    expect(meResp.status(), 'GET /api/v1/auth/me must return 200 in setup').toBe(200);
    const me = await validateApiResponse('UserInfo', meResp) as { id?: string };
    const userID = me.id ?? '';
    expect(userID, 'Auth user id is required for rate-limit seed').toBeTruthy();

    // Rate-limit status rows are built from batch tickets/exemptions/overrides.
    // Seed via explicit override upsert so this row is deterministic.
    const overrideResp = await request.put(`/api/v1/admin/rate-limits/users/${userID}`, {
        headers,
        data: {
            max_pending_parents: 8,
            reason: `admin-extended rate-limit seed ${Date.now()}`,
        },
    });
    expect(overrideResp.status(), 'PUT /admin/rate-limits/users/{id} must return 200 in setup').toBe(200);
    await validateApiResponse('RateLimitUserOverride', overrideResp);

    await expect.poll(async () => {
        const statusResp = await request.get('/api/v1/admin/rate-limits/status', { headers });
        expect(statusResp.status(), 'GET /admin/rate-limits/status must return 200 in setup').toBe(200);
        const statusBody = await validateApiResponse('RateLimitStatusList', statusResp) as {
            items?: Array<{ user_id?: string }>;
        };
        return statusBody.items?.find((item) => item.user_id === userID)?.user_id ?? '';
    }, {
        timeout: 20_000,
        intervals: [300, 500, 700, 1_000],
        message: `Rate-limit seed did not produce status row for user ${userID}`,
    }).toBe(userID);

    return userID;
}

// ── Test suite ────────────────────────────────────────────────────────────────

test.describe('admin-extended live (contract-enforced, no mock, no skip)', () => {
    test.beforeAll(async ({ request }) => {
        // API-first setup (explicit, fail-fast): ensure one provider and one cluster exist.
        const headers = await getAdminAuthHeaders(request);

        const authResp = await request.get('/api/v1/admin/auth-providers', { headers });
        expect(authResp.status(), 'GET /api/v1/admin/auth-providers must return 200 in setup').toBe(200);
        const authData = await validateApiResponse('AuthProviderList', authResp) as ApiList<{ id?: string }>;
        if ((authData.items ?? []).length === 0) {
            const typeResp = await request.get('/api/v1/admin/auth-provider-types', { headers });
            expect(typeResp.status(), 'GET /api/v1/admin/auth-provider-types must return 200 in setup').toBe(200);
            const typeData = await validateApiResponse('AuthProviderTypeList', typeResp) as ApiList<{ type?: string }>;
            const authType =
                typeData.items?.find((item) => item.type === 'oidc')?.type ??
                typeData.items?.find((item) => item.type === 'generic')?.type ??
                typeData.items?.[0]?.type ??
                'generic';
            const createAuthResp = await request.post('/api/v1/admin/auth-providers', {
                headers,
                data: {
                    name: `setup-auth-${Date.now().toString(36).slice(-5)}`,
                    auth_type: authType,
                    config: { test_endpoint: 'https://idp.example.com/healthz' },
                    enabled: true,
                },
            });
            if (createAuthResp.status() !== 201) {
                const bodyText = await createAuthResp.text();
                throw new Error(
                    `Seed setup failed: POST /api/v1/admin/auth-providers returned ${createAuthResp.status()}.\n` +
                    `Response body: ${bodyText}`
                );
            }
            await validateApiResponse('AuthProvider', createAuthResp);
        }

        const clusterResp = await request.get('/api/v1/admin/clusters', { headers });
        expect(clusterResp.status(), 'GET /api/v1/admin/clusters must return 200 in setup').toBe(200);
        const clusterData = await validateApiResponse('ClusterList', clusterResp) as ApiList<{ id?: string }>;
        if ((clusterData.items ?? []).length === 0) {
            const createClusterResp = await request.post('/api/v1/admin/clusters', {
                headers,
                data: {
                    name: `setup-cluster-${Date.now().toString(36).slice(-5)}`,
                    kubeconfig: e2eKubeconfigB64,
                },
            });
            if (createClusterResp.status() !== 201) {
                const bodyText = await createClusterResp.text();
                throw new Error(
                    `Seed setup failed: POST /api/v1/admin/clusters returned ${createClusterResp.status()}.\n` +
                    `Response body: ${bodyText}\n` +
                    'If kubeconfig validation fails, set E2E_KUBECONFIG_B64 to a valid base64 kubeconfig.'
                );
            }
            await validateApiResponse('Cluster', createClusterResp);
        }

        seededRateLimitUserID = await seedRateLimitStatusUser(request, headers);
    });

    test.beforeEach(async ({ page }) => {
        await login(page);
    });

    // ── listPermissions: GET /admin/permissions → PermissionList ──────────────
    //
    // NOTE: The /admin/permissions page uses STATIC_PERMISSIONS (hardcoded data)
    // and does NOT call the API. We test the API directly using request context.

    test('listPermissions – GET /admin/permissions conforms to PermissionList schema', async ({ request }) => {
        // operationId: listPermissions
        // The frontend page is static — test the API endpoint directly.
        const auth = await getApiTokenWithForcePasswordSupport(request, {
            username: e2eUsername,
            primaryPassword: e2ePassword,
            secondaryPassword: e2eNewPassword,
            currentPasswordHint: activePassword,
        });
        activePassword = auth.password;

        const resp = await request.get('/api/v1/admin/permissions', {
            headers: { Authorization: `Bearer ${auth.token}` },
        });
        expect(resp.status(), `GET /admin/permissions returned ${resp.status()}`).toBe(200);
        const body = await resp.json();
        // Validate against PermissionList schema using validateResponse (non-Playwright)
        const { validateResponse } = await import('./lib/schema-validator');
        validateResponse('PermissionList', body);
    });

    // ── updateRole: PATCH /admin/roles/{id} → Role ───────────────────────────

    test('updateRole – PATCH /admin/roles/{id} conforms to Role schema', async ({ page }) => {
        // operationId: updateRole
        // Create a role to update
        await page.goto('/admin/rbac');
        await expect(page.getByTestId('admin-rbac-page')).toBeVisible();

        const roleName = `e2e-upd-role-${Date.now().toString(36).slice(-5)}`;
        const createRespPromise = page.waitForResponse(
            (r) => urlPathEndsWith(r.url(), '/api/v1/admin/roles') && r.request().method() === 'POST'
        );
        await page.getByTestId('rbac-role-create-button').click();
        const createModal = getAntModal(page, 'rbac-role-create-modal');
        await expect(createModal).toBeVisible();
        await createModal.getByRole('textbox').first().fill(roleName);
        // Select a role from dropdown
        await selectAntOption(page, createModal.locator('.ant-select-selector').first());
        await createModal.getByRole('button', { name: 'OK' }).click();

        const { body: created } = await expectSchema(createRespPromise, 'Role', 201);
        const roleID = (created as { id?: string }).id ?? '';
        expect(roleID, 'POST /admin/roles response missing id').toBeTruthy();

        // ── PATCH /admin/roles/{id} → Role ────────────────────────────────────────
        const updateRespPromise = page.waitForResponse(
            (r) => urlPathIncludes(r.url(), `/api/v1/admin/roles/${roleID}`) && r.request().method() === 'PATCH'
        );
        await expect(page.getByTestId(`rbac-role-action-edit-${roleID}`)).toBeVisible();
        await page.getByTestId(`rbac-role-action-edit-${roleID}`).click();
        const editModal = getAntModal(page, 'rbac-role-edit-modal');
        await expect(editModal).toBeVisible();
        await editModal.getByRole('textbox').first().fill(`${roleName}-upd`);
        await editModal.locator('textarea').first().fill('Updated by live e2e test');
        await editModal.getByRole('button', { name: 'OK' }).click();

        await expectSchema(updateRespPromise, 'Role', 200);

        // Cleanup
        const deleteRespPromise = page.waitForResponse(
            (r) => urlPathIncludes(r.url(), `/api/v1/admin/roles/${roleID}`) && r.request().method() === 'DELETE'
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
        expect(userID, 'POST /admin/users response missing id').toBeTruthy();

        // ── PATCH /admin/users/{id} → User ───────────────────────────────────────
        const updateRespPromise = page.waitForResponse(
            (r) => urlPathIncludes(r.url(), `/api/v1/admin/users/${userID}`) && r.request().method() === 'PATCH'
        );
        await page.getByTestId(`user-action-edit-${userID}`).click();
        const editModal = getAntModal(page, 'user-edit-modal');
        await expect(editModal).toBeVisible();
        // Update display name or email
        const displayNameInput = editModal.getByRole('textbox').first();
        await displayNameInput.fill('Updated E2E User');
        await editModal.getByRole('button', { name: 'OK' }).click();

        await expectSchema(updateRespPromise, 'User', 200);

        // Cleanup
        const deleteRespPromise = page.waitForResponse(
            (r) => urlPathIncludes(r.url(), `/api/v1/admin/users/${userID}`) && r.request().method() === 'DELETE'
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
            (r) => urlPathEndsWith(r.url(), '/api/v1/admin/users') && r.request().method() === 'GET'
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
            (r) => urlPathIncludes(r.url(), `/api/v1/admin/users/${userID}/role-bindings`) && r.request().method() === 'GET'
        );
        await page.getByTestId(`user-action-role-bindings-${userID}`).click();
        await expectSchema(bindingsRespPromise, 'GlobalRoleBindingList', 200);
    });

    test('createUserRoleBinding + deleteUserRoleBinding – full lifecycle', async ({ page }) => {
        // operationId: createUserRoleBinding, deleteUserRoleBinding
        // Get first user (use validateApiResponse for single-read safety)
        const usersRespPromise = page.waitForResponse(
            (r) => urlPathEndsWith(r.url(), '/api/v1/admin/users') && r.request().method() === 'GET'
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
            (r) => urlPathIncludes(r.url(), `/api/v1/admin/users/${userID}/role-bindings`) && r.request().method() === 'POST'
        );
        await page.getByTestId('role-binding-create-button').click();
        const createModal = getAntModal(page, 'role-binding-create-modal');
        await expect(createModal).toBeVisible();
        // Select a role from dropdown
        await selectAntOption(page, createModal.locator('.ant-select-selector').first());
        await createModal.getByRole('button', { name: 'OK' }).click();

        const { body: created } = await expectSchema(createRespPromise, 'GlobalRoleBinding', 201);
        const bindingID = (created as { id?: string }).id ?? '';
        expect(bindingID, 'POST /admin/users/{id}/role-bindings response missing id').toBeTruthy();

        // ── DELETE /admin/users/{id}/role-bindings/{id} → 204 ────────────────────
        const deleteRespPromise = page.waitForResponse(
            (r) => urlPathIncludes(r.url(), `/api/v1/admin/users/${userID}/role-bindings/${bindingID}`) && r.request().method() === 'DELETE'
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
            (r) => urlPathEndsWith(r.url(), '/api/v1/admin/templates') && r.request().method() === 'POST'
        );
        await page.getByTestId('template-create-button').click();
        const createModal = getAntModal(page, 'template-create-modal');
        await expect(createModal).toBeVisible();
        await createModal.getByRole('textbox').first().fill(tplName);
        await createModal.getByLabel(/image url/i).fill('quay.io/containerdisks/ubuntu:22.04');
        await createModal.getByRole('button', { name: 'OK' }).click();

        const { body: created } = await expectSchema(createRespPromise, 'Template', 201);
        const tplID = (created as { id?: string }).id ?? '';
        expect(tplID, 'POST /admin/templates response missing id').toBeTruthy();

        // ── PATCH /admin/templates/{id} → Template ────────────────────────────────
        // First ensure the edit action button is visible before registering the
        // response promise — the button may take a moment to appear after list
        // re-render following POST creation.
        const editBtn = page.getByTestId(`template-action-edit-${tplID}`);
        await expect(editBtn).toBeVisible({ timeout: 10_000 });

        const updateRespPromise = page.waitForResponse(
            (r) => urlPathIncludes(r.url(), `/api/v1/admin/templates/${tplID}`) && r.request().method() === 'PATCH'
        );
        await editBtn.click();
        const editModal = getAntModal(page, 'template-edit-modal');
        await expect(editModal).toBeVisible();
        await editModal.locator('textarea').first().fill('Updated template description by live e2e test');
        await editModal.getByRole('button', { name: 'OK' }).click();

        await expectSchema(updateRespPromise, 'Template', 200);

        // ── DELETE /admin/templates/{id} → 204 ───────────────────────────────────
        const deleteRespPromise = page.waitForResponse(
            (r) => urlPathIncludes(r.url(), `/api/v1/admin/templates/${tplID}`) && r.request().method() === 'DELETE'
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
            (r) => urlPathEndsWith(r.url(), '/api/v1/admin/instance-sizes') && r.request().method() === 'POST'
        );
        await page.getByTestId('instance-size-create-button').click();
        const createModal = getAntModal(page, 'instance-size-create-modal');
        await expect(createModal).toBeVisible();
        await createModal.getByRole('textbox').first().fill(sizeName);
        // Use native field IDs to avoid picking wrong Antd spinbutton nodes.
        await createModal.locator('#cpu_cores').fill('2');
        await createModal.locator('#memory_gi').fill('4');
        await createModal.getByRole('button', { name: 'OK' }).click();

        const { body: created } = await expectSchema(createRespPromise, 'InstanceSize', 201);
        const sizeID = (created as { id?: string }).id ?? '';
        expect(sizeID, 'POST /admin/instance-sizes response missing id').toBeTruthy();

        // ── PATCH /admin/instance-sizes/{id} → InstanceSize ──────────────────────
        const updateRespPromise = page.waitForResponse(
            (r) => urlPathIncludes(r.url(), `/api/v1/admin/instance-sizes/${sizeID}`) && r.request().method() === 'PATCH'
        );
        await page.getByTestId(`instance-size-action-edit-${sizeID}`).click();
        const editModal = getAntModal(page, 'instance-size-edit-modal');
        await expect(editModal).toBeVisible();
        // Wait until edit form hydration finishes (destroyOnHidden + setFieldsValue timing).
        await expect(editModal.locator('#memory_gi')).toHaveValue(/4(?:\\.0)?/);
        await editModal.locator('#cpu_cores').fill('4');
        await editModal.getByRole('button', { name: 'OK' }).click();

        await expectSchema(updateRespPromise, 'InstanceSize', 200);

        // ── DELETE /admin/instance-sizes/{id} → 204 ──────────────────────────────
        const deleteRespPromise = page.waitForResponse(
            (r) => urlPathIncludes(r.url(), `/api/v1/admin/instance-sizes/${sizeID}`) && r.request().method() === 'DELETE'
        );
        await page.getByTestId(`instance-size-action-delete-${sizeID}`).click();
        const confirmBtn = page.getByRole('button', { name: /confirm|ok|delete/i }).last();
        await confirmBtn.click();
        expect((await deleteRespPromise).status()).toBe(204);
    });

    // ── Auth Provider: update + test-connection + sample + directory sync + cohort-mapping CRUD

    test('updateAuthProvider – PATCH /admin/auth-providers/{id} conforms to AuthProvider schema', async ({ page }) => {
        // operationId: updateAuthProvider
        const headers = await getAdminAuthHeaders(page.request);
        const provider = await createGenericAuthProvider(page.request, headers);
        await page.goto('/admin/auth-providers');
        await expect(page.getByRole('heading', { name: 'Authentication Providers' })).toBeVisible();

        // ── PATCH /admin/auth-providers/{id} → AuthProvider ──────────────────────
        const updateRespPromise = page.waitForResponse(
            (r) => urlPathIncludes(r.url(), `/api/v1/admin/auth-providers/${provider.id}`) && r.request().method() === 'PATCH'
        );
        await page.getByTestId(`auth-provider-action-edit-${provider.id}`).click();
        const editModal = getAntModal(page, 'auth-provider-edit-modal');
        await expect(editModal).toBeVisible();
        await editModal.getByLabel(/\*?\s*name/i).fill(`upd-auth-${Date.now().toString(36).slice(-5)}`);
        await editModal.getByLabel(/test endpoint/i).fill('https://idp.example.com/readyz');
        await editModal.getByRole('button', { name: 'OK' }).click();

        await expectSchema(updateRespPromise, 'AuthProvider', 200);
        await deleteAuthProviderIfPresent(page.request, headers, provider.id);
    });

    test('testAuthProviderConnection – POST /admin/auth-providers/{id}/test-connection conforms to AuthProviderConnectionTestResult schema', async ({ page }) => {
        // operationId: testAuthProviderConnection
        const headers = await getAdminAuthHeaders(page.request);
        const provider = await createGenericAuthProvider(page.request, headers);
        await page.goto('/admin/auth-providers');
        await expect(page.getByRole('heading', { name: 'Authentication Providers' })).toBeVisible();

        // ── POST /admin/auth-providers/{id}/test-connection ───────────────────────
        const testRespPromise = page.waitForResponse(
            (r) => urlPathEndsWith(r.url(), `/api/v1/admin/auth-providers/${provider.id}/test-connection`) && r.request().method() === 'POST'
        );
        await page.getByTestId(`auth-provider-action-test-${provider.id}`).click();
        await expectSchema(testRespPromise, 'AuthProviderConnectionTestResult', 200);
        await deleteAuthProviderIfPresent(page.request, headers, provider.id);
    });

    test('getAuthProviderDirectoryDescriptor + previewAuthProviderDirectory + triggerAuthProviderDirectorySync', async ({ request }) => {
        // operationId: getAuthProviderDirectoryDescriptor, previewAuthProviderDirectory, triggerAuthProviderDirectorySync,
        //              listAuthProviderDirectorySyncJobs, getAuthProviderDirectorySyncJob
        const headers = await getAdminAuthHeaders(request);
        const provider = await createGenericAuthProvider(request, headers);

        const descriptorResp = await request.get(`/api/v1/admin/auth-providers/${provider.id}/directory/descriptor`, {
            headers,
        });
        expect(descriptorResp.status(), `GET /directory/descriptor returned ${descriptorResp.status()}`).toBe(200);
        await validateApiResponse('DirectorySyncDescriptor', descriptorResp);

        const previewResp = await request.post(`/api/v1/admin/auth-providers/${provider.id}/directory/preview`, {
            headers,
            data: {
                provider_request: {
                    usernames: ['alice'],
                },
            },
        });
        expect(previewResp.status(), `POST /directory/preview returned ${previewResp.status()}`).toBe(200);
        await validateApiResponse('DirectorySyncPreview', previewResp);

        const triggerResp = await request.post(`/api/v1/admin/auth-providers/${provider.id}/directory/sync`, {
            headers,
            data: {
                provider_request: {
                    usernames: ['alice'],
                },
            },
        });
        expect(triggerResp.status(), `POST /directory/sync returned ${triggerResp.status()}`).toBe(202);
        const triggerBody = await validateApiResponse('DirectorySyncTriggerResponse', triggerResp) as { job_id?: string };
        const jobID = triggerBody.job_id ?? '';
        expect(jobID, 'Directory sync trigger response must include job_id').toBeTruthy();

        const jobsResp = await request.get(`/api/v1/admin/auth-providers/${provider.id}/directory/sync-jobs`, {
            headers,
        });
        expect(jobsResp.status(), `GET /directory/sync-jobs returned ${jobsResp.status()}`).toBe(200);
        await validateApiResponse('DirectorySyncJobList', jobsResp);

        const jobResp = await request.get(`/api/v1/admin/auth-providers/${provider.id}/directory/sync-jobs/${jobID}`, {
            headers,
        });
        expect(jobResp.status(), `GET /directory/sync-jobs/{job_id} returned ${jobResp.status()}`).toBe(200);
        await validateApiResponse('DirectorySyncJob', jobResp);

        await deleteAuthProviderIfPresent(request, headers, provider.id);
    });

    test('getAuthProviderSample – GET /admin/auth-providers/{id}/sample returns 200', async ({ page }) => {
        // operationId: getAuthProviderSample
        const headers = await getAdminAuthHeaders(page.request);
        const provider = await createGenericAuthProvider(page.request, headers);
        await page.goto('/admin/auth-providers');
        await expect(page.getByRole('heading', { name: 'Authentication Providers' })).toBeVisible();

        // ── GET /admin/auth-providers/{id}/sample ─────────────────────────────────
        // The sample is fetched automatically when the mappings modal is opened.
        const sampleRespPromise = page.waitForResponse(
            (r) => urlPathEndsWith(r.url(), `/api/v1/admin/auth-providers/${provider.id}/sample`) && r.request().method() === 'GET'
        );
        await page.getByTestId(`auth-provider-action-mappings-${provider.id}`).click();
        const mappingsPage = getAntModal(page, 'auth-provider-mappings-page');
        await expect(mappingsPage).toBeVisible();


        const sampleResp = await sampleRespPromise;
        expect(sampleResp.status(), `GET /admin/auth-providers/${provider.id}/sample returned ${sampleResp.status()}`).toBe(200);
        // ── CONTRACT CHECK: AuthProviderSampleResponse schema ─────────────────────
        await validateApiResponse('AuthProviderSampleResponse', sampleResp);
        await deleteAuthProviderIfPresent(page.request, headers, provider.id);
    });

    test('createAuthProviderCohortMapping + updateAuthProviderCohortMapping + deleteAuthProviderCohortMapping', async ({ page }) => {
        // operationId: createAuthProviderCohortMapping, updateAuthProviderCohortMapping, deleteAuthProviderCohortMapping
        const headers = await getAdminAuthHeaders(page.request);
        const provider = await createGenericAuthProvider(page.request, headers);
        await page.goto('/admin/auth-providers');
        await expect(page.getByRole('heading', { name: 'Authentication Providers' })).toBeVisible();

        // Navigate to cohort mappings
        await page.getByTestId(`auth-provider-action-mappings-${provider.id}`).click();
        const mappingsPage = getAntModal(page, 'auth-provider-mappings-page');
        await expect(mappingsPage).toBeVisible();

        // ── POST /admin/auth-providers/{id}/cohort-mappings → ExternalCohortMapping ─
        const createRespPromise = page.waitForResponse(
            (r) => urlPathIncludes(r.url(), `/api/v1/admin/auth-providers/${provider.id}/cohort-mappings`) && r.request().method() === 'POST'
        );
        await page.getByTestId('cohort-mapping-create-button').click();
        const createModal = getAntModal(page, 'cohort-mapping-create-modal');
        await expect(createModal).toBeVisible();
        await createModal.getByLabel(/cohort kind/i).fill('group');
        await createModal.getByLabel(/cohort key/i).fill(`e2e-group-${Date.now().toString(36).slice(-5)}`);
        await createModal.getByLabel(/cohort label/i).fill('E2E Group');
        await selectAntOption(page, createModal.locator('.ant-select-selector').first());
        await createModal.getByRole('button', { name: 'OK' }).click();

        const { body: created } = await expectSchema(createRespPromise, 'ExternalCohortMapping', 201);
        const mappingID = (created as { id?: string }).id ?? '';
        expect(mappingID, 'POST cohort-mappings response missing id').toBeTruthy();

        // ── PATCH /admin/auth-providers/{id}/cohort-mappings/{id} → ExternalCohortMapping
        const updateRespPromise = page.waitForResponse(
            (r) => urlPathIncludes(r.url(), `/api/v1/admin/auth-providers/${provider.id}/cohort-mappings/${mappingID}`) && r.request().method() === 'PATCH'
        );
        await page.getByTestId(`cohort-mapping-action-edit-${mappingID}`).click();
        const editModal = getAntModal(page, 'cohort-mapping-edit-modal');
        await expect(editModal).toBeVisible();
        await editModal.getByLabel(/cohort label/i).fill('Updated E2E Group');
        await editModal.getByRole('button', { name: 'OK' }).click();

        await expectSchema(updateRespPromise, 'ExternalCohortMapping', 200);

        // ── DELETE /admin/auth-providers/{id}/cohort-mappings/{id} → 204 ─────────
        const deleteRespPromise = page.waitForResponse(
            (r) => urlPathIncludes(r.url(), `/api/v1/admin/auth-providers/${provider.id}/cohort-mappings/${mappingID}`) && r.request().method() === 'DELETE'
        );
        await page.getByTestId(`cohort-mapping-action-delete-${mappingID}`).click();
        const confirmBtn = page.getByRole('button', { name: /confirm|ok/i }).last();
        await confirmBtn.click();
        expect((await deleteRespPromise).status()).toBe(204);
        await deleteAuthProviderIfPresent(page.request, headers, provider.id);
    });

    // ── Rate Limit: full coverage ─────────────────────────────────────────────

    test('listRateLimitExemptions – GET /admin/rate-limits/exemptions conforms to RateLimitExemptionList schema', async ({ page }) => {
        // operationId: listRateLimitExemptions
        const respPromise = page.waitForResponse(
            (r) => urlPathIncludes(r.url(), '/api/v1/admin/rate-limits/exemptions') && r.request().method() === 'GET'
                && !urlPathIncludes(r.url(), '/status') && !urlPathIncludes(r.url(), '/users/')
        );
        await page.goto('/admin/rate-limits');
        await expect(page.locator('body')).toBeVisible();
        // ── CONTRACT CHECK: RateLimitExemptionList schema ─────────────────────────
        await expectSchema(respPromise, 'RateLimitExemptionList', 200);
    });

    test('listRateLimitStatus – GET /admin/rate-limits/status conforms to RateLimitStatusList schema', async ({ page }) => {
        // operationId: listRateLimitStatus
        const respPromise = page.waitForResponse(
            (r) => urlPathEndsWith(r.url(), '/api/v1/admin/rate-limits/status') && r.request().method() === 'GET'
        );
        await page.goto('/admin/rate-limits');
        await expect(page.locator('body')).toBeVisible();
        await expectSchema(respPromise, 'RateLimitStatusList', 200);
    });

    test('createRateLimitExemption + deleteRateLimitExemption – full lifecycle', async ({ page }) => {
        // operationId: createRateLimitExemption, deleteRateLimitExemption
        expect(seededRateLimitUserID, 'Rate-limit seed user is missing').toBeTruthy();
        await page.goto('/admin/users');
        await expect(page.getByTestId('admin-users-page')).toBeVisible();
        const userID = seededRateLimitUserID;
        const exemptBtn = page.getByTestId(`ratelimit-action-exempt-${userID}`);
        await expect(exemptBtn, `No rate-limit exemption action found for seeded user ${userID}`).toBeVisible();

        // ── POST /admin/rate-limits/exemptions ────────────────────────────────────
        const createRespPromise = page.waitForResponse(
            (r) => urlPathEndsWith(r.url(), '/api/v1/admin/rate-limits/exemptions') && r.request().method() === 'POST'
        );

        await exemptBtn.click();

        const createModal = getAntModal(page, 'rate-limit-exemption-create-modal');
        await expect(createModal).toBeVisible();
        await createModal.getByRole('button', { name: 'OK' }).click();

        const createResp = await createRespPromise;
        expect(createResp.status(), `POST /admin/rate-limits/exemptions returned ${createResp.status()}`).toBe(200);
        // ── CONTRACT CHECK: RateLimitExemption schema ─────────────────────────────
        await validateApiResponse('RateLimitExemption', createResp);

        // ── DELETE /admin/rate-limits/exemptions/{user_id} → 204 ─────────────────
        const deleteRespPromise = page.waitForResponse(
            (r) => urlPathIncludes(r.url(), `/api/v1/admin/rate-limits/exemptions/${userID}`) && r.request().method() === 'DELETE'
        );
        await page.getByTestId(`rate-limit-exemption-action-delete-${userID}`).click();
        const confirmBtn = page.getByRole('button', { name: /confirm|ok/i }).last();
        await confirmBtn.click();
        expect((await deleteRespPromise).status()).toBe(204);
    });

    test('updateRateLimitUserOverrides – PUT /admin/rate-limits/users/{id} conforms to RateLimitUserOverride schema', async ({ page }) => {
        // operationId: updateRateLimitUserOverrides
        expect(seededRateLimitUserID, 'Rate-limit seed user is missing').toBeTruthy();
        await page.goto('/admin/users');
        await expect(page.getByTestId('admin-users-page')).toBeVisible();
        const userID = seededRateLimitUserID;
        const overrideBtn = page.getByTestId(`rate-limit-user-action-edit-${userID}`);
        await expect(overrideBtn, `No rate-limit override action found for seeded user ${userID}`).toBeVisible();

        const updateRespPromise = page.waitForResponse(
            (r) => urlPathIncludes(r.url(), `/api/v1/admin/rate-limits/users/${userID}`) && r.request().method() === 'PUT'
        );
        await overrideBtn.click();
        const editModal = getAntModal(page, 'rate-limit-user-edit-modal');
        await expect(editModal).toBeVisible();
        await editModal.getByRole('spinbutton', { name: /max parent batches/i }).fill('100');
        await editModal.getByRole('textbox', { name: /reason/i }).fill('Updated by live e2e test');
        await editModal.getByRole('button', { name: 'OK' }).click();

        await expectSchema(updateRespPromise, 'RateLimitUserOverride', 200);
    });

    // ── Cluster: updateClusterEnvironment ────────────────────────────────────

    test('updateClusterEnvironment – PUT /admin/clusters/{id}/environment conforms to Cluster schema', async ({ page }) => {
        // operationId: updateClusterEnvironment
        const listRespPromise = page.waitForResponse(
            (r) => urlPathEndsWith(r.url(), '/api/v1/admin/clusters') && r.request().method() === 'GET'
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
            (r) => urlPathIncludes(r.url(), `/api/v1/admin/clusters/${clusterID}/environment`) && r.request().method() === 'PUT'
        );
        await page.getByTestId(`cluster-action-set-environment-${clusterID}`).click();
        const editModal = getAntModal(page, 'cluster-environment-modal');
        await expect(editModal).toBeVisible();
        // Select environment type
        await selectAntOption(page, editModal.locator('.ant-select-selector').first(), /test|staging/i);
        await editModal.getByRole('button', { name: 'OK' }).click();

        await expectSchema(updateRespPromise, 'Cluster', 200);
    });
});
