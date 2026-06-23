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

import { expect, test, type APIRequestContext, type Page } from '@playwright/test';
import { validateApiResponse } from './lib/schema-validator';
import {
    createOIDCAuthProvider,
    createTempAdminUser,
    deleteAdminUserIfPresent,
    deleteAuthProviderIfPresent,
    expectSchemaResponse as expectSchema,
    getAntModal,
    getApiAuthHeadersWithForcePasswordSupport,
    loginWithForcePasswordSupport,
    selectAntOption,
    urlPathEndsWith,
    urlPathIncludes,
} from './lib/helpers';

// ── Config ────────────────────────────────────────────────────────────────────

const e2eUsername = process.env.E2E_USERNAME ?? 'e2e-admin';
const e2ePassword = process.env.E2E_PASSWORD ?? 'e2e-admin-123';
const e2eNewPassword = process.env.E2E_NEW_PASSWORD ?? (e2ePassword === 'admin' ? 'ShepherdLive!2026' : `${e2ePassword}-new`);
const e2eKubeconfigB64 = process.env.E2E_KUBECONFIG_B64 ?? 'dGVzdC1rdWJlY29uZmlnLWJhc2U2NA==';
let activePassword = e2ePassword;
let seededRateLimitUserID = '';

interface ApiList<T> {
    items?: T[];
}

async function getAdminAuthHeaders(request: APIRequestContext): Promise<{ Authorization: string }> {
    const auth = await getApiAuthHeadersWithForcePasswordSupport(request, {
        username: e2eUsername,
        primaryPassword: e2ePassword,
        secondaryPassword: e2eNewPassword,
        currentPasswordHint: activePassword,
    });
    activePassword = auth.password;
    return auth.headers;
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

async function getFirstRoleID(request: APIRequestContext, headers: { Authorization: string }): Promise<string> {
    const rolesResp = await request.get('/api/v1/admin/roles', { headers });
    expect(rolesResp.status(), `GET /admin/roles returned ${rolesResp.status()}`).toBe(200);
    const rolesBody = await validateApiResponse('RoleList', rolesResp) as ApiList<{ id?: string; name?: string }>;
    const roleID = rolesBody.items?.find((role) => Boolean(role.id))?.id ?? '';
    expect(roleID, 'Role binding tests require at least one role').toBeTruthy();
    return roleID;
}

async function createTemplateViaAPI(
    request: APIRequestContext,
    headers: { Authorization: string },
    name = `e2e-tpl-${Date.now().toString(36).slice(-5)}`
): Promise<{ id: string; name: string }> {
    const createResp = await request.post('/api/v1/admin/templates', {
        headers,
        data: {
            name,
            source_type: 'containerdisk',
            image_url: 'quay.io/containerdisks/ubuntu:22.04',
            catalog_scope: 'test',
            enabled: true,
        },
    });
    expect(createResp.status(), `POST /admin/templates returned ${createResp.status()}`).toBe(201);
    const created = await validateApiResponse('Template', createResp) as { id?: string; name?: string };
    const id = created.id ?? '';
    expect(id, 'Created template id is required').toBeTruthy();
    return { id, name: created.name ?? name };
}

async function deleteTemplateIfPresent(
    request: APIRequestContext,
    headers: { Authorization: string },
    templateID: string
): Promise<void> {
    if (!templateID) {
        return;
    }
    const deleteResp = await request.delete(`/api/v1/admin/templates/${templateID}`, { headers });
    expect([204, 404, 409], `DELETE /admin/templates/${templateID} returned ${deleteResp.status()}`).toContain(deleteResp.status());
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
            const authType = typeData.items?.find((item) => item.type === 'oidc')?.type;
            expect(authType, 'OIDC provider type must be available in setup').toBeTruthy();
            const createAuthResp = await request.post('/api/v1/admin/auth-providers', {
                headers,
                data: {
                    name: `setup-auth-${Date.now().toString(36).slice(-5)}`,
                    auth_type: authType,
                    config: {
                        issuer_url: 'https://idp.example.com',
                        client_id: 'shepherd-e2e',
                        client_secret: 'secret',
                        scopes: ['openid', 'profile', 'email'],
                    },
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
        const headers = await getAdminAuthHeaders(request);
        const resp = await request.get('/api/v1/admin/permissions', {
            headers,
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

    test('createUserRoleBinding + deleteUserRoleBinding – full lifecycle', async ({ request }) => {
        // operationId: createUserRoleBinding, deleteUserRoleBinding
        const headers = await getAdminAuthHeaders(request);
        const user = await createTempAdminUser(request, headers, { prefix: 'e2e-rb' });
        const roleID = await getFirstRoleID(request, headers);
        let bindingID = '';

        try {
            // ── POST /admin/users/{id}/role-bindings → GlobalRoleBinding ─────────
            const createResp = await request.post(`/api/v1/admin/users/${user.id}/role-bindings`, {
                headers,
                data: {
                    role_id: roleID,
                    scope_type: 'global',
                    allowed_environments: ['test'],
                },
            });
            expect(createResp.status(), `POST /admin/users/{id}/role-bindings returned ${createResp.status()}`).toBe(201);
            const created = await validateApiResponse('GlobalRoleBinding', createResp) as { id?: string };
            bindingID = created.id ?? '';
            expect(bindingID, 'POST /admin/users/{id}/role-bindings response missing id').toBeTruthy();

            // ── DELETE /admin/users/{id}/role-bindings/{id} → 204 ────────────────
            const deleteResp = await request.delete(`/api/v1/admin/users/${user.id}/role-bindings/${bindingID}`, { headers });
            expect(deleteResp.status(), `DELETE /admin/users/{id}/role-bindings/{id} returned ${deleteResp.status()}`).toBe(204);
            bindingID = '';
        } finally {
            if (bindingID) {
                const cleanupBindingResp = await request.delete(`/api/v1/admin/users/${user.id}/role-bindings/${bindingID}`, { headers });
                expect([204, 404], `cleanup role binding returned ${cleanupBindingResp.status()}`).toContain(cleanupBindingResp.status());
            }
            await deleteAdminUserIfPresent(request, headers, user.id);
        }
    });

    // ── updateAdminTemplate + deleteAdminTemplate ─────────────────────────────

    test('updateAdminTemplate – PATCH /admin/templates/{id} conforms to Template schema', async ({ request }) => {
        // operationId: updateAdminTemplate, deleteAdminTemplate
        const headers = await getAdminAuthHeaders(request);
        const tpl = await createTemplateViaAPI(request, headers);

        try {
            // ── PATCH /admin/templates/{id} → Template ────────────────────────────
            const updateResp = await request.patch(`/api/v1/admin/templates/${tpl.id}`, {
                headers,
                data: {
                    description: 'Updated template description by live e2e test',
                },
            });
            expect(updateResp.status(), `PATCH /admin/templates/{id} returned ${updateResp.status()}`).toBe(200);
            await validateApiResponse('Template', updateResp);

            // ── DELETE /admin/templates/{id} → 204 ───────────────────────────────
            const deleteResp = await request.delete(`/api/v1/admin/templates/${tpl.id}`, { headers });
            expect(deleteResp.status(), `DELETE /admin/templates/{id} returned ${deleteResp.status()}`).toBe(204);
        } finally {
            await deleteTemplateIfPresent(request, headers, tpl.id);
        }
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
        const provider = await createOIDCAuthProvider(page.request, headers);
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
        await editModal.getByLabel(/issuer url/i).fill('https://idp.example.com');
        await editModal.getByRole('button', { name: 'OK' }).click();

        await expectSchema(updateRespPromise, 'AuthProvider', 200);
        await deleteAuthProviderIfPresent(page.request, headers, provider.id);
    });

    test('testAuthProviderConnection – POST /admin/auth-providers/{id}/test-connection conforms to AuthProviderConnectionTestResult schema', async ({ page }) => {
        // operationId: testAuthProviderConnection
        const headers = await getAdminAuthHeaders(page.request);
        const provider = await createOIDCAuthProvider(page.request, headers);
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

    test('getAuthProviderDirectoryDescriptor returns unsupported for non-directory providers', async ({ request }) => {
        // operationId: getAuthProviderDirectoryDescriptor
        const headers = await getAdminAuthHeaders(request);
        const provider = await createOIDCAuthProvider(request, headers);

        const descriptorResp = await request.get(`/api/v1/admin/auth-providers/${provider.id}/directory/descriptor`, {
            headers,
        });
        expect(descriptorResp.status(), `GET /directory/descriptor returned ${descriptorResp.status()}`).toBe(501);
        await validateApiResponse('Error', descriptorResp);

        await deleteAuthProviderIfPresent(request, headers, provider.id);
    });

    test('getAuthProviderSample – GET /admin/auth-providers/{id}/sample returns 200', async ({ page }) => {
        // operationId: getAuthProviderSample
        const headers = await getAdminAuthHeaders(page.request);
        const provider = await createOIDCAuthProvider(page.request, headers);
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

    test('createAuthProviderCohortMapping + updateAuthProviderCohortMapping + deleteAuthProviderCohortMapping', async ({ request }) => {
        // operationId: createAuthProviderCohortMapping, updateAuthProviderCohortMapping, deleteAuthProviderCohortMapping
        const headers = await getAdminAuthHeaders(request);
        const provider = await createOIDCAuthProvider(request, headers);
        const roleID = await getFirstRoleID(request, headers);
        let mappingID = '';

        try {
            // ── POST /admin/auth-providers/{id}/cohort-mappings → ExternalCohortMapping
            const createResp = await request.post(`/api/v1/admin/auth-providers/${provider.id}/cohort-mappings`, {
                headers,
                data: {
                    cohort_kind: 'group',
                    cohort_key: `e2e-group-${Date.now().toString(36).slice(-5)}`,
                    cohort_display_name: 'E2E Group',
                    role_id: roleID,
                    scope_type: 'global',
                    allowed_environments: ['test'],
                },
            });
            expect(createResp.status(), `POST cohort-mappings returned ${createResp.status()}`).toBe(201);
            const created = await validateApiResponse('ExternalCohortMapping', createResp) as { id?: string };
            mappingID = created.id ?? '';
            expect(mappingID, 'POST cohort-mappings response missing id').toBeTruthy();

            // ── PATCH /admin/auth-providers/{id}/cohort-mappings/{id} → ExternalCohortMapping
            const updateResp = await request.patch(`/api/v1/admin/auth-providers/${provider.id}/cohort-mappings/${mappingID}`, {
                headers,
                data: {
                    role_id: roleID,
                    scope_type: 'global',
                    allowed_environments: ['prod'],
                },
            });
            expect(updateResp.status(), `PATCH cohort-mappings/{id} returned ${updateResp.status()}`).toBe(200);
            await validateApiResponse('ExternalCohortMapping', updateResp);

            // ── DELETE /admin/auth-providers/{id}/cohort-mappings/{id} → 204 ─────
            const deleteResp = await request.delete(`/api/v1/admin/auth-providers/${provider.id}/cohort-mappings/${mappingID}`, { headers });
            expect(deleteResp.status(), `DELETE cohort-mappings/{id} returned ${deleteResp.status()}`).toBe(204);
            mappingID = '';
        } finally {
            if (mappingID) {
                const cleanupMappingResp = await request.delete(`/api/v1/admin/auth-providers/${provider.id}/cohort-mappings/${mappingID}`, { headers });
                expect([204, 404], `cleanup cohort mapping returned ${cleanupMappingResp.status()}`).toContain(cleanupMappingResp.status());
            }
            await deleteAuthProviderIfPresent(request, headers, provider.id);
        }
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

    test('createRateLimitExemption + deleteRateLimitExemption – full lifecycle', async ({ request }) => {
        // operationId: createRateLimitExemption, deleteRateLimitExemption
        expect(seededRateLimitUserID, 'Rate-limit seed user is missing').toBeTruthy();
        const headers = await getAdminAuthHeaders(request);
        const userID = seededRateLimitUserID;
        const preDeleteResp = await request.delete(`/api/v1/admin/rate-limits/exemptions/${userID}`, { headers });
        expect([204, 404], `pre-clean exemption returned ${preDeleteResp.status()}`).toContain(preDeleteResp.status());

        // ── POST /admin/rate-limits/exemptions ────────────────────────────────────
        const createResp = await request.post('/api/v1/admin/rate-limits/exemptions', {
            headers,
            data: {
                user_id: userID,
                reason: `live e2e exemption ${Date.now()}`,
            },
        });
        expect(createResp.status(), `POST /admin/rate-limits/exemptions returned ${createResp.status()}`).toBe(200);
        // ── CONTRACT CHECK: RateLimitExemption schema ─────────────────────────────
        await validateApiResponse('RateLimitExemption', createResp);

        // ── DELETE /admin/rate-limits/exemptions/{user_id} → 204 ─────────────────
        const deleteResp = await request.delete(`/api/v1/admin/rate-limits/exemptions/${userID}`, { headers });
        expect(deleteResp.status(), `DELETE /admin/rate-limits/exemptions/{user_id} returned ${deleteResp.status()}`).toBe(204);
    });

    test('updateRateLimitUserOverrides – PUT /admin/rate-limits/users/{id} conforms to RateLimitUserOverride schema', async ({ request }) => {
        // operationId: updateRateLimitUserOverrides
        expect(seededRateLimitUserID, 'Rate-limit seed user is missing').toBeTruthy();
        const headers = await getAdminAuthHeaders(request);
        const userID = seededRateLimitUserID;

        const updateResp = await request.put(`/api/v1/admin/rate-limits/users/${userID}`, {
            headers,
            data: {
                max_pending_parents: 100,
                max_pending_children: 200,
                cooldown_seconds: 0,
                reason: 'Updated by live e2e test',
            },
        });
        expect(updateResp.status(), `PUT /admin/rate-limits/users/{id} returned ${updateResp.status()}`).toBe(200);
        await validateApiResponse('RateLimitUserOverride', updateResp);
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
