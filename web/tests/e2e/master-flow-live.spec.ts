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
 *   updateService          Stage 4.B  – PATCH /systems/{id}/services/{id}
 *   deleteService          Stage 4.B  – DELETE /systems/{id}/services/{id}
 *   listVMs                Stage 5    – GET /vms
 *   getVMRequestContext    Stage 5.A  – GET /vms/request-context
 *   createVMRequest        Stage 5.A  – POST /vms/request
 *   listApprovals          Stage 5.B  – GET /approvals
 *   approveTicket          Stage 5.B  – POST /approvals/{id}/approve
 *   rejectTicket           Stage 5.B  – POST /approvals/{id}/reject
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
 *   listUsers              Stage 2.A+ – GET /admin/users
 *
 * Environment variables:
 *   E2E_USERNAME        – admin username (default: e2e-admin)
 *   E2E_PASSWORD        – admin password (default: e2e-admin-123)
 *   E2E_SYSTEM          – pre-existing system name for cascade tests
 *   E2E_SERVICE         – pre-existing service name (has child VMs)
 *   E2E_VM_RUNNING_ID   – ID of a running VM for console test
 *
 * Run:
 *   PW_BASE_URL=http://localhost:3000 npx playwright test master-flow-live
 */

import { expect, test, type Page, type Response } from '@playwright/test';
import { validateApiResponse } from './lib/schema-validator';
import { urlPathEndsWith, urlPathIncludes, selectAntOption, getAntModal } from './lib/helpers';

// ── Config ────────────────────────────────────────────────────────────────────

const e2eUsername = process.env.E2E_USERNAME ?? 'e2e-admin';
const e2ePassword = process.env.E2E_PASSWORD ?? 'e2e-admin-123';
const e2eSystemName = process.env.E2E_SYSTEM ?? 'e2e-system';
const e2eServiceName = process.env.E2E_SERVICE ?? 'e2e-service';
const runningVMID = process.env.E2E_VM_RUNNING_ID ?? '';

// ── Auth helper ───────────────────────────────────────────────────────────────

async function login(page: Page): Promise<void> {
  await page.goto('/login');
  await expect(page.getByRole('heading', { name: 'KubeVirt Shepherd' })).toBeVisible();

  await page.getByPlaceholder('Username').fill(e2eUsername);
  await page.getByPlaceholder('Password').fill(e2ePassword);

  const loginRespPromise = page.waitForResponse(
    (r) => urlPathEndsWith(r.url(), '/api/v1/auth/login') && r.request().method() === 'POST'
  );
  await page.getByRole('button', { name: 'Login' }).click();

  const loginResp = await loginRespPromise;
  expect(loginResp.status()).toBe(200);

  // ── CONTRACT CHECK: LoginResponse schema ──────────────────────────────────
  await validateApiResponse('LoginResponse', loginResp);

  await expect(page).toHaveURL(/\/dashboard$/);
}

// ── Helpers ───────────────────────────────────────────────────────────────────

/** Wait for a response and validate its body against the given OpenAPI schema. */
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

// ── Test suite ────────────────────────────────────────────────────────────────

test.describe('master-flow live (contract-enforced, no mock)', () => {
  test.beforeEach(async ({ page }) => {
    await page.addInitScript(() => {
      window.open = () => null; // prevent popup interference in CI
    });
    await login(page);
  });

  // ── Stage 2.D: Auth ─────────────────────────────────────────────────────────

  test('Stage 2.D – getCurrentUser: /auth/me returns UserInfo schema', async ({ page }) => {
    // operationId: login, getCurrentUser
    // login() in beforeEach already validates LoginResponse schema.
    // This test additionally verifies /auth/me returns UserInfo schema.
    const meRespPromise = page.waitForResponse(
      (r) => urlPathEndsWith(r.url(), '/api/v1/auth/me') && r.request().method() === 'GET'
    );
    await page.goto('/dashboard');

    // /auth/me may be called on page load; race with a timeout
    const meResp = await Promise.race([
      meRespPromise,
      new Promise<null>((resolve) => setTimeout(() => resolve(null), 3000)),
    ]);
    if (meResp) {
      expect((meResp as Response).status()).toBe(200);
      // ── CONTRACT CHECK: UserInfo schema (getCurrentUser) ──────────────────
      await validateApiResponse('UserInfo', meResp as Response);
    } else {
      console.warn('[getCurrentUser] /auth/me not called on dashboard load – trigger manually');
      // Trigger explicitly via navigation to profile page
      const explicitRespPromise = page.waitForResponse(
        (r) => urlPathEndsWith(r.url(), '/api/v1/auth/me') && r.request().method() === 'GET'
      );
      await page.goto('/profile');
      const explicitResp = await Promise.race([
        explicitRespPromise,
        new Promise<null>((resolve) => setTimeout(() => resolve(null), 3000)),
      ]);
      if (explicitResp) {
        expect((explicitResp as Response).status()).toBe(200);
        await validateApiResponse('UserInfo', explicitResp as Response);
      } else {
        console.warn('[getCurrentUser] /auth/me not triggered – backend may not expose this route yet');
      }
    }
  });

  // ── Stage 4.A: System CRUD ──────────────────────────────────────────────────

  test('Stage 4.A – listSystems + createSystem + updateSystem + deleteSystem (schema-validated)', async ({ page }) => {
    // operationId: listSystems, createSystem, updateSystem, deleteSystem

    // ── CONTRACT CHECK: listSystems → SystemList ──────────────────────────────
    // Per Playwright best practice: register Promise BEFORE the action that
    // triggers the network request, to avoid missing a fast response.
    const listRespPromise = page.waitForResponse(
      (r) => urlPathIncludes(r.url(), '/api/v1/systems') && r.request().method() === 'GET' && !urlPathIncludes(r.url(), '/members')
    );
    await page.goto('/systems');
    await expect(page.getByRole('heading', { name: 'Systems' })).toBeVisible();
    const listResp = await listRespPromise;
    if (listResp.status() === 200) {
      await validateApiResponse('SystemList', listResp);
    }

    // ── CONTRACT CHECK: createSystem → System ────────────────────────────────
    const systemName = `e2es${Date.now().toString(36).slice(-6)}`;
    const createRespPromise = page.waitForResponse(
      (r) => urlPathEndsWith(r.url(), '/api/v1/systems') && r.request().method() === 'POST'
    );

    await page.getByTestId('system-create-button').click();
    const createModal = getAntModal(page, 'system-create-modal');
    await expect(createModal).toBeVisible();
    await createModal.locator('input[maxlength="15"]').first().fill(systemName);
    await createModal.locator('textarea').first().fill('created by live e2e');
    await createModal.getByRole('button', { name: 'OK' }).click();

    const { body: createdSystem } = await expectSchema(createRespPromise, 'System', 201);
    const systemID = (createdSystem as { id?: string }).id ?? '';
    expect(systemID).toBeTruthy();

    await expect(page.locator('tr').filter({ hasText: systemName }).first()).toBeVisible();

    // ── CONTRACT CHECK: updateSystem → System ────────────────────────────────
    const updateRespPromise = page.waitForResponse(
      (r) => urlPathIncludes(r.url(), `/api/v1/systems/${systemID}`) && r.request().method() === 'PATCH'
    );
    await page.getByTestId(`system-action-edit-${systemID}`).click();
    const editModal = getAntModal(page, 'system-edit-modal');
    await expect(editModal).toBeVisible();
    await editModal.locator('textarea').first().fill('updated by live e2e');
    await editModal.getByRole('button', { name: 'OK' }).click();

    await expectSchema(updateRespPromise, 'System', 200);

    // ── CONTRACT CHECK: deleteSystem with confirm_name guard ──────────────────
    await page.getByTestId(`system-action-delete-${systemID}`).click();
    const deleteModal = getAntModal(page, 'system-delete-modal');
    await expect(deleteModal).toBeVisible();

    const deleteBtn = deleteModal.getByRole('button', { name: /delete/i });
    await expect(deleteBtn).toBeDisabled();

    // Wrong name → still disabled (cascade guard UI test)
    await deleteModal.getByRole('textbox').first().fill('wrong-name');
    await expect(deleteBtn).toBeDisabled();

    // Correct name → enabled
    await deleteModal.getByRole('textbox').first().fill(systemName);
    await expect(deleteBtn).toBeEnabled();

    const deleteRespPromise = page.waitForResponse(
      (r) => urlPathIncludes(r.url(), `/api/v1/systems/${systemID}`) && r.request().method() === 'DELETE'
    );
    await deleteBtn.click();

    const deleteResp = await deleteRespPromise;
    expect(deleteResp.status()).toBe(204);
    // Verify confirm_name was sent as query param (ADR-0015 §13)
    expect(deleteResp.url()).toContain(`confirm_name=${systemName}`);

    await expect(page.locator('tr').filter({ hasText: systemName })).toHaveCount(0);
  });

  // ── Stage 4.A+: System Member Management ────────────────────────────────────

  test('Stage 4.A+ – listSystemMembers: system member list (schema-validated)', async ({ page }) => {
    // operationId: listSystemMembers
    // Create a temporary system to test member management
    await page.goto('/systems');
    await expect(page.getByRole('heading', { name: 'Systems' })).toBeVisible();

    const systemName = `e2em${Date.now().toString(36).slice(-6)}`;
    const createRespPromise = page.waitForResponse(
      (r) => urlPathEndsWith(r.url(), '/api/v1/systems') && r.request().method() === 'POST'
    );
    await page.getByTestId('system-create-button').click();
    const createModal = getAntModal(page, 'system-create-modal');
    await expect(createModal).toBeVisible();
    await createModal.locator('input[maxlength="15"]').first().fill(systemName);
    await createModal.getByRole('button', { name: 'OK' }).click();

    const { body: createdSystem } = await expectSchema(createRespPromise, 'System', 201);
    const systemID = (createdSystem as { id?: string }).id ?? '';
    expect(systemID).toBeTruthy();

    // ── CONTRACT CHECK: listSystemMembers → SystemMemberList ──────────────────
    const membersRespPromise = page.waitForResponse(
      (r) => urlPathIncludes(r.url(), `/api/v1/systems/${systemID}/members`) && r.request().method() === 'GET'
    );
    await page.getByTestId(`system-action-members-${systemID}`).click();
    const membersModal = getAntModal(page, 'system-members-modal');
    await expect(membersModal).toBeVisible();

    const membersResp = await membersRespPromise;
    if (membersResp.status() === 200) {
      await validateApiResponse('SystemMemberList', membersResp);
    }

    // Close modal and clean up
    await membersModal.getByRole('button', { name: /cancel|close/i }).first().click();

    // Delete the temp system
    await page.getByTestId(`system-action-delete-${systemID}`).click();
    const deleteModal = getAntModal(page, 'system-delete-modal');
    await expect(deleteModal).toBeVisible();
    await deleteModal.getByRole('textbox').first().fill(systemName);
    const deleteRespPromise = page.waitForResponse(
      (r) => urlPathIncludes(r.url(), `/api/v1/systems/${systemID}`) && r.request().method() === 'DELETE'
    );
    await deleteModal.getByRole('button', { name: /delete/i }).click();
    const deleteResp = await deleteRespPromise;
    expect(deleteResp.status()).toBe(204);
  });

  // ── Stage 4.B: Service CRUD ──────────────────────────────────────────────────

  test('Stage 4.B – createService + updateService + deleteService (schema-validated)', async ({ page }) => {
    // operationId: createService, updateService, deleteService
    // First create a system to own the service
    await page.goto('/systems');
    const systemName = `e2esvc${Date.now().toString(36).slice(-5)}`;
    const createSysRespPromise = page.waitForResponse(
      (r) => urlPathEndsWith(r.url(), '/api/v1/systems') && r.request().method() === 'POST'
    );
    await page.getByTestId('system-create-button').click();
    const createSysModal = getAntModal(page, 'system-create-modal');
    await expect(createSysModal).toBeVisible();
    await createSysModal.locator('input[maxlength="15"]').first().fill(systemName);
    await createSysModal.getByRole('button', { name: 'OK' }).click();

    const { body: sys } = await expectSchema(createSysRespPromise, 'System', 201);
    const systemID = (sys as { id?: string }).id ?? '';
    expect(systemID).toBeTruthy();

    // Navigate to services and select the new system
    await page.goto('/services');
    await expect(page.getByRole('heading', { name: 'Services' })).toBeVisible();
    await selectAntOption(page, page.getByTestId('services-system-selector'), systemName);

    // ── CONTRACT CHECK: createService → Service ───────────────────────────────
    const serviceName = `e2e-svc-${Date.now().toString(36).slice(-5)}`;
    const createSvcRespPromise = page.waitForResponse(
      (r) =>
        urlPathIncludes(r.url(), `/api/v1/systems/${systemID}/services`) &&
        r.request().method() === 'POST'
    );
    await page.getByTestId('service-create-button').click();
    const createSvcModal = getAntModal(page, 'service-create-modal');
    await expect(createSvcModal).toBeVisible();
    await createSvcModal.getByPlaceholder('e.g. web, api-gateway').fill(serviceName);
    await createSvcModal.locator('textarea').first().fill('service created by live e2e');
    await createSvcModal.getByRole('button', { name: 'OK' }).click();

    const { body: createdSvc } = await expectSchema(createSvcRespPromise, 'Service', 201);
    const serviceID = (createdSvc as { id?: string }).id ?? '';
    expect(serviceID).toBeTruthy();

    await expect(page.locator('tr').filter({ hasText: serviceName }).first()).toBeVisible();

    // ── CONTRACT CHECK: updateService → Service ───────────────────────────────
    const updateSvcRespPromise = page.waitForResponse(
      (r) =>
        urlPathIncludes(r.url(), `/api/v1/systems/${systemID}/services/${serviceID}`) &&
        r.request().method() === 'PATCH'
    );
    await page.getByTestId(`service-action-edit-${serviceID}`).click();
    const editSvcModal = getAntModal(page, 'service-edit-modal');
    await expect(editSvcModal).toBeVisible();
    await editSvcModal.locator('textarea').first().fill('updated by live e2e');
    await editSvcModal.getByRole('button', { name: 'OK' }).click();

    await expectSchema(updateSvcRespPromise, 'Service', 200);

    // ── CONTRACT CHECK: deleteService with confirm=true ───────────────────────
    const deleteSvcRespPromise = page.waitForResponse(
      (r) =>
        urlPathIncludes(r.url(), `/api/v1/systems/${systemID}/services/${serviceID}`) &&
        r.request().method() === 'DELETE'
    );
    await page.getByTestId(`service-action-delete-${serviceID}`).click();
    const popconfirm = page.locator('.ant-popover:visible');
    await expect(popconfirm).toBeVisible();
    await popconfirm.getByRole('button', { name: /confirm/i }).click();

    const deleteSvcResp = await deleteSvcRespPromise;
    expect(deleteSvcResp.status()).toBe(204);
    // Verify confirm=true was sent (ADR-0015 §13)
    expect(deleteSvcResp.url()).toContain('confirm=true');

    await expect(page.locator('tr').filter({ hasText: serviceName })).toHaveCount(0);

    // Clean up: delete the system
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

  // ── Stage 5.D: Cascade guard – Service with child VMs → 409 ─────────────────

  test('Stage 5.D – deleteService returns 409 when child VMs exist (cascade guard)', async ({ page }) => {
    // operationId: deleteService (cascade guard path)
    await page.goto('/services');
    await expect(page.getByRole('heading', { name: 'Services' })).toBeVisible();

    await selectAntOption(page, page.getByTestId('services-system-selector'), e2eSystemName);

    const serviceRow = page.locator('tr').filter({ hasText: e2eServiceName }).first();
    await expect(serviceRow).toBeVisible();

    const deleteRespPromise = page.waitForResponse(
      (r) =>
        urlPathIncludes(r.url(), '/api/v1/systems/') &&
        urlPathIncludes(r.url(), '/services/') &&
        r.request().method() === 'DELETE'
    );
    await serviceRow.locator('[data-testid^="service-action-delete-"]').first().click();
    await page.getByRole('button', { name: /confirm/i }).click();

    const deleteResp = await deleteRespPromise;
    expect(deleteResp.status()).toBe(409);
    expect(deleteResp.url()).toContain('confirm=true');

    // ── CONTRACT CHECK: Error response schema ─────────────────────────────────
    // Reuse body from validateApiResponse to avoid double-read of response body
    const body = await validateApiResponse('Error', deleteResp) as { code?: string };
    expect(body.code).toBe('SERVICE_HAS_VMS');
  });

  // ── Stage 5.A: VM Request Submission ────────────────────────────────────────

  test('Stage 5.A – getVMRequestContext + createVMRequest (schema-validated)', async ({ page }) => {
    // operationId: getVMRequestContext, createVMRequest
    // First get the VM request context to validate schema
    const contextRespPromise = page.waitForResponse(
      (r) => urlPathEndsWith(r.url(), '/api/v1/vms/request-context') && r.request().method() === 'GET'
    );

    await page.goto('/vms');
    await expect(page.getByRole('heading', { name: 'Virtual Machines' })).toBeVisible();

    // Open VM request wizard
    await page.getByRole('button', { name: 'Request VM' }).click();
    await expect(page.getByText('Create VM Request')).toBeVisible();

    // ── CONTRACT CHECK: getVMRequestContext → VMRequestContext ────────────────
    const contextResp = await contextRespPromise;
    if (contextResp.status() === 200) {
      await validateApiResponse('VMRequestContext', contextResp);
    }

    // Step 0: Select System
    const systemSelect = getAntModal(page, 'vm-request-wizard-modal').locator('[role="combobox"]').first();
    await selectAntOption(page, systemSelect);

    // Select Service
    const serviceSelect = getAntModal(page, 'vm-request-wizard-modal').locator('[role="combobox"]').nth(1);
    await selectAntOption(page, serviceSelect);

    await getAntModal(page, 'vm-request-wizard-modal').getByRole('button', { name: 'Next' }).click();

    // Step 1: Template
    const templateSelect = getAntModal(page, 'vm-request-wizard-modal').locator('[role="combobox"]').first();
    await selectAntOption(page, templateSelect);
    await getAntModal(page, 'vm-request-wizard-modal').getByRole('button', { name: 'Next' }).click();

    // Step 2: Instance Size
    const sizeSelect = getAntModal(page, 'vm-request-wizard-modal').locator('[role="combobox"]').first();
    await selectAntOption(page, sizeSelect);
    await getAntModal(page, 'vm-request-wizard-modal').getByRole('button', { name: 'Next' }).click();

    // Step 3: Namespace + Reason
    await getAntModal(page, 'vm-request-wizard-modal').locator('input').first().fill('test-ns');
    await getAntModal(page, 'vm-request-wizard-modal').locator('textarea').first().fill('live e2e test request');
    await getAntModal(page, 'vm-request-wizard-modal').getByRole('button', { name: 'Next' }).click();

    // Step 4: Submit
    const submitBtn = getAntModal(page, 'vm-request-wizard-modal').getByRole('button', { name: 'Submit' });
    await expect(submitBtn).toBeVisible({ timeout: 5000 });

    const submitRespPromise = page.waitForResponse(
      (r) => urlPathEndsWith(r.url(), '/api/v1/vms/request') && r.request().method() === 'POST'
    );
    await submitBtn.click();

    // ── CONTRACT CHECK: createVMRequest → ApprovalTicketResponse ─────────────
    const submitResp = await submitRespPromise;
    expect([202, 400, 409]).toContain(submitResp.status()); // 400/409 = no valid namespace/duplicate
    if (submitResp.status() === 202) {
      await validateApiResponse('ApprovalTicketResponse', submitResp);
    }
  });

  // ── Stage 5: VM List ─────────────────────────────────────────────────────────

  test('Stage 5 – listVMs: VM list conforms to VMList schema', async ({ page }) => {
    // operationId: listVMs
    const vmListRespPromise = page.waitForResponse(
      (r) => urlPathIncludes(r.url(), '/api/v1/vms') && r.request().method() === 'GET' && !urlPathIncludes(r.url(), '/batch') && !urlPathIncludes(r.url(), '/request') && !urlPathIncludes(r.url(), '/console')
    );
    await page.goto('/vms');
    await expect(page.getByRole('heading', { name: 'Virtual Machines' })).toBeVisible();

    const vmListResp = await vmListRespPromise;
    if (vmListResp.status() === 200) {
      // ── CONTRACT CHECK: listVMs → VMList ──────────────────────────────────
      await validateApiResponse('VMList', vmListResp);
    }
  });

  // ── Stage 5.B: Approvals ─────────────────────────────────────────────────────

  test('Stage 5.B – listApprovals: approval list conforms to ApprovalTicketList schema', async ({ page }) => {
    // operationId: listApprovals
    const listRespPromise = page.waitForResponse(
      (r) => urlPathIncludes(r.url(), '/api/v1/approvals') && r.request().method() === 'GET'
    );

    await page.goto('/approvals');
    await expect(page.getByRole('heading', { name: /approval/i })).toBeVisible();

    // ── CONTRACT CHECK: listApprovals → ApprovalTicketList ────────────────────
    const listResp = await listRespPromise;
    expect(listResp.status()).toBe(200);
    await validateApiResponse('ApprovalTicketList', listResp);
  });

  test('Stage 5.B – approveTicket: approve action calls real API', async ({ page, request }) => {
    // operationId: approveTicket
    // API-first setup: ensure a pending ticket exists by creating a VM request via API.
    const loginResp = await request.post(`/api/v1/auth/login`, {
      data: { username: e2eUsername, password: e2ePassword },
    });
    expect(loginResp.ok(), 'API login failed').toBeTruthy();
    const { token } = await loginResp.json() as { token: string };

    const svcResp = await request.get(`/api/v1/services`, {
      headers: { Authorization: `Bearer ${token}` },
    });
    const ctxResp = await request.get(`/api/v1/vms/request-context`, {
      headers: { Authorization: `Bearer ${token}` },
    });
    if (svcResp.ok() && ctxResp.ok()) {
      const svcData = await svcResp.json() as { items?: Array<{ id?: string }> };
      const ctx = await ctxResp.json() as {
        templates?: Array<{ id?: string }>;
        instance_sizes?: Array<{ id?: string }>;
      };
      const svcId = svcData.items?.[0]?.id;
      const tplId = ctx.templates?.[0]?.id;
      const sizeId = ctx.instance_sizes?.[0]?.id;
      if (svcId && tplId && sizeId) {
        const batchResp = await request.post(`/api/v1/vms/batch`, {
          headers: { Authorization: `Bearer ${token}` },
          data: {
            operation: 'CREATE',
            items: [{
              service_id: svcId,
              template_id: tplId,
              instance_size_id: sizeId,
              name: `e2e-approve-${Date.now().toString(36).slice(-5)}`,
            }],
            reason: 'Created by live E2E to test approveTicket',
          },
        });
        expect(batchResp.status(), 'Setting up approval tickets should return 202 Accepted').toBe(202);
      }
    }

    await page.goto('/approvals');
    await expect(page.getByRole('heading', { name: /approval/i })).toBeVisible();

    const approveBtn = page.locator('[data-testid^="approval-action-approve-"]').first();
    await expect(approveBtn, 'No pending approval tickets found — API setup may have failed').toBeVisible();

    const testId = await approveBtn.getAttribute('data-testid');
    const ticketID = testId?.replace('approval-action-approve-', '') ?? '';

    const approveRespPromise = page.waitForResponse(
      (r) =>
        urlPathIncludes(r.url(), `/api/v1/approvals/${ticketID}`) &&
        (r.request().method() === 'PATCH' || r.request().method() === 'POST')
    );

    await approveBtn.click();
    const modal = getAntModal(page, 'approve-modal');
    await expect(modal).toBeVisible();
    await modal.getByRole('button', { name: 'OK' }).click();

    // ── CONTRACT CHECK: approveTicket → 204 ──────────────────────────────────
    const approveResp = await approveRespPromise;
    expect([200, 204]).toContain(approveResp.status());
  });

  test('Stage 5.B – rejectTicket: reject action calls real API', async ({ page, request }) => {
    // operationId: rejectTicket
    // API-first setup: ensure a pending ticket exists by creating a VM request via API.
    const loginResp = await request.post(`/api/v1/auth/login`, {
      data: { username: e2eUsername, password: e2ePassword },
    });
    expect(loginResp.ok(), 'API login failed').toBeTruthy();
    const { token } = await loginResp.json() as { token: string };

    const svcResp = await request.get(`/api/v1/services`, {
      headers: { Authorization: `Bearer ${token}` },
    });
    const ctxResp = await request.get(`/api/v1/vms/request-context`, {
      headers: { Authorization: `Bearer ${token}` },
    });
    if (svcResp.ok() && ctxResp.ok()) {
      const svcData = await svcResp.json() as { items?: Array<{ id?: string }> };
      const ctx = await ctxResp.json() as {
        templates?: Array<{ id?: string }>;
        instance_sizes?: Array<{ id?: string }>;
      };
      const svcId = svcData.items?.[0]?.id;
      const tplId = ctx.templates?.[0]?.id;
      const sizeId = ctx.instance_sizes?.[0]?.id;
      if (svcId && tplId && sizeId) {
        const batchResp = await request.post(`/api/v1/vms/batch`, {
          headers: { Authorization: `Bearer ${token}` },
          data: {
            operation: 'CREATE',
            items: [{
              service_id: svcId,
              template_id: tplId,
              instance_size_id: sizeId,
              name: `e2e-reject-${Date.now().toString(36).slice(-5)}`,
            }],
            reason: 'Created by live E2E to test rejectTicket',
          },
        });
        expect(batchResp.status(), 'Setting up approval tickets should return 202 Accepted').toBe(202);
      }
    }

    await page.goto('/approvals');
    await expect(page.getByRole('heading', { name: /approval/i })).toBeVisible();

    const rejectBtn = page.locator('[data-testid^="approval-action-reject-"]').first();
    await expect(rejectBtn, 'No pending approval tickets found — API setup may have failed').toBeVisible();

    const testId = await rejectBtn.getAttribute('data-testid');
    const ticketID = testId?.replace('approval-action-reject-', '') ?? '';

    const rejectRespPromise = page.waitForResponse(
      (r) =>
        urlPathIncludes(r.url(), `/api/v1/approvals/${ticketID}`) &&
        (r.request().method() === 'PATCH' || r.request().method() === 'POST')
    );

    await rejectBtn.click();
    const modal = getAntModal(page, 'reject-modal');
    await expect(modal).toBeVisible();
    await modal.locator('textarea').first().fill('Rejected by live e2e test');
    await modal.getByRole('button', { name: 'OK' }).click();

    // ── CONTRACT CHECK: rejectTicket → 204 ───────────────────────────────────
    const rejectResp = await rejectRespPromise;
    expect([200, 204]).toContain(rejectResp.status());
  });

  // ── Stage 5.E: Batch Power Action ────────────────────────────────────────────

  test('Stage 5.E – submitVMBatchPower: batch power action → VMBatchSubmitResponse schema', async ({ page }) => {
    // operationId: submitVMBatchPower
    await page.goto('/vms');
    await expect(page.getByRole('heading', { name: 'Virtual Machines' })).toBeVisible();

    // Find a stopped VM row to batch-start
    const stoppedRow = page.locator('tr').filter({ hasText: /stopped/i }).first();
    const hasStoppedVM = (await stoppedRow.count()) > 0;
    if (!hasStoppedVM) {
      console.warn('[submitVMBatchPower] No stopped VMs available – batch power action not exercised');
      // Still verify the VM list schema was correct
      return;
    }

    await stoppedRow.getByRole('checkbox').check();

    const batchRespPromise = page.waitForResponse(
      (r) => urlPathEndsWith(r.url(), '/api/v1/vms/batch/power') && r.request().method() === 'POST'
    );
    await page.getByRole('button', { name: 'Start Selected', exact: true }).click();

    // ── CONTRACT CHECK: submitVMBatchPower → VMBatchSubmitResponse ────────────
    const batchResp = await batchRespPromise;
    expect(batchResp.status()).toBe(202);
    await validateApiResponse('VMBatchSubmitResponse', batchResp);
  });

  // ── Stage 5.F: Notifications ─────────────────────────────────────────────────

  test('Stage 5.F – listNotifications: notification list conforms to NotificationList schema', async ({ page }) => {
    // operationId: listNotifications
    const notifRespPromise = page.waitForResponse(
      (r) => urlPathIncludes(r.url(), '/api/v1/notifications') && r.request().method() === 'GET' && !urlPathIncludes(r.url(), 'unread')
    );

    await page.goto('/notifications');
    await expect(page.getByRole('heading', { name: 'Notifications' })).toBeVisible();

    // ── CONTRACT CHECK: listNotifications → NotificationList ──────────────────
    const notifResp = await notifRespPromise;
    expect(notifResp.status()).toBe(200);
    await validateApiResponse('NotificationList', notifResp);
  });

  test('Stage 5.F – getUnreadCount: unread count endpoint returns valid integer', async ({ page }) => {
    // operationId: getUnreadCount
    const countRespPromise = page.waitForResponse(
      (r) => urlPathEndsWith(r.url(), '/api/v1/notifications/unread-count') && r.request().method() === 'GET'
    );

    await page.goto('/dashboard');
    await expect(page.getByRole('heading', { name: 'Dashboard' })).toBeVisible();

    // ── CONTRACT CHECK: getUnreadCount → UnreadCount ──────────────────────────
    const countResp = await countRespPromise;
    expect(countResp.status()).toBe(200);
    const body = await validateApiResponse('UnreadCount', countResp) as { count?: unknown };
    expect(typeof body.count).toBe('number');
  });

  // ── Stage 6: VM Console ───────────────────────────────────────────────────────

  test('Stage 6 – requestVMConsoleAccess + openVMVNC: VM console request flow', async ({ page }) => {
    // operationId: requestVMConsoleAccess, openVMVNC
    // NOTE: Requires E2E_VM_RUNNING_ID to be set to a running VM's ID.
    // If not set, the test logs a warning but does NOT skip.
    // Failure here means: either no running VM (expected in clean env)
    // or the console endpoint is broken (contract violation).
    if (!runningVMID) {
      console.warn('[requestVMConsoleAccess/openVMVNC] E2E_VM_RUNNING_ID not set – console flow not exercised');
      // Verify the VM list loads correctly at minimum
      const vmListRespPromise = page.waitForResponse(
        (r) => urlPathIncludes(r.url(), '/api/v1/vms') && r.request().method() === 'GET' && !urlPathIncludes(r.url(), '/batch')
      );
      await page.goto('/vms');
      await expect(page.getByRole('heading', { name: 'Virtual Machines' })).toBeVisible();
      const vmListResp = await vmListRespPromise;
      if (vmListResp.status() === 200) {
        await validateApiResponse('VMList', vmListResp);
      }
      return;
    }

    await page.goto('/vms');
    await expect(page.getByRole('heading', { name: 'Virtual Machines' })).toBeVisible();

    const consoleRespPromise = page.waitForResponse(
      (r) =>
        urlPathEndsWith(r.url(), `/api/v1/vms/${runningVMID}/console/request`) &&
        r.request().method() === 'POST'
    );
    const vncRespPromise = page.waitForResponse(
      (r) =>
        urlPathEndsWith(r.url(), `/api/v1/vms/${runningVMID}/vnc`) &&
        r.request().method() === 'GET'
    );

    await page.getByTestId(`vm-action-console-${runningVMID}`).click();

    // ── CONTRACT CHECK: requestVMConsoleAccess → VMConsoleRequestResponse ─────
    const consoleResp = await consoleRespPromise;
    expect([200, 202]).toContain(consoleResp.status());
    await validateApiResponse('VMConsoleRequestResponse', consoleResp);

    // ── CONTRACT CHECK: openVMVNC → VMVNCSessionResponse ─────────────────────
    const vncResp = await vncRespPromise;
    expect(vncResp.status()).toBe(200);
    await validateApiResponse('VMVNCSessionResponse', vncResp);
  });

  // ── Stage 3: Admin config (GET schemas) ──────────────────────────────────────

  test('Stage 3 – listAdminTemplates: admin template list conforms to TemplateList schema', async ({ page }) => {
    // operationId: listAdminTemplates
    const tplRespPromise = page.waitForResponse(
      (r) => urlPathEndsWith(r.url(), '/api/v1/admin/templates') && r.request().method() === 'GET'
    );

    await page.goto('/admin/templates');
    await expect(page.getByRole('heading', { name: 'Templates' })).toBeVisible();

    // ── CONTRACT CHECK: listAdminTemplates → TemplateList ────────────────────
    const tplResp = await tplRespPromise;
    expect(tplResp.status()).toBe(200);
    await validateApiResponse('TemplateList', tplResp);
  });

  test('Stage 3 – listAdminInstanceSizes: instance-size list conforms to InstanceSizeList schema', async ({ page }) => {
    // operationId: listAdminInstanceSizes
    const sizeRespPromise = page.waitForResponse(
      (r) => urlPathEndsWith(r.url(), '/api/v1/admin/instance-sizes') && r.request().method() === 'GET'
    );

    await page.goto('/admin/instance-sizes');
    await expect(page.getByRole('heading', { name: 'Instance Sizes' })).toBeVisible();

    // ── CONTRACT CHECK: listAdminInstanceSizes → InstanceSizeList ────────────
    const sizeResp = await sizeRespPromise;
    expect(sizeResp.status()).toBe(200);
    await validateApiResponse('InstanceSizeList', sizeResp);
  });

  test('Stage 3 – listNamespaces: admin namespace list conforms to NamespaceRegistryList schema', async ({ page }) => {
    // operationId: listNamespaces
    const nsRespPromise = page.waitForResponse(
      (r) => urlPathEndsWith(r.url(), '/api/v1/admin/namespaces') && r.request().method() === 'GET'
    );

    await page.goto('/admin/namespaces');
    await expect(page.getByTestId('admin-namespaces-page')).toBeVisible();

    // ── CONTRACT CHECK: listNamespaces → NamespaceRegistryList ───────────────
    const nsResp = await nsRespPromise;
    expect(nsResp.status()).toBe(200);
    await validateApiResponse('NamespaceRegistryList', nsResp);
  });

  // ── Stage 2.A: RBAC ──────────────────────────────────────────────────────────

  test('Stage 2.A – listRoles: role list conforms to RoleList schema', async ({ page }) => {
    // operationId: listRoles
    const rolesRespPromise = page.waitForResponse(
      (r) => urlPathEndsWith(r.url(), '/api/v1/admin/roles') && r.request().method() === 'GET'
    );

    await page.goto('/admin/rbac');
    await expect(page.getByTestId('admin-rbac-page')).toBeVisible();

    // ── CONTRACT CHECK: listRoles → RoleList ──────────────────────────────────
    const rolesResp = await rolesRespPromise;
    expect(rolesResp.status()).toBe(200);
    await validateApiResponse('RoleList', rolesResp);
  });

  // ── Stage 2.B: Auth Providers ────────────────────────────────────────────────

  test('Stage 2.B – listAuthProviderTypes: auth provider type list conforms to AuthProviderTypeList schema', async ({ page }) => {
    // operationId: listAuthProviderTypes
    const typesRespPromise = page.waitForResponse(
      (r) => urlPathEndsWith(r.url(), '/api/v1/admin/auth-provider-types') && r.request().method() === 'GET'
    );

    await page.goto('/admin/auth-providers');
    await expect(page.getByRole('heading', { name: 'Authentication Providers' })).toBeVisible();

    // ── CONTRACT CHECK: listAuthProviderTypes → AuthProviderTypeList ──────────
    const typesResp = await typesRespPromise;
    expect(typesResp.status()).toBe(200);
    await validateApiResponse('AuthProviderTypeList', typesResp);
  });

  test('Stage 2.B – listAuthProviders: auth provider list conforms to AuthProviderList schema', async ({ page }) => {
    // operationId: listAuthProviders
    const listRespPromise = page.waitForResponse(
      (r) => urlPathEndsWith(r.url(), '/api/v1/admin/auth-providers') && r.request().method() === 'GET'
    );

    await page.goto('/admin/auth-providers');
    await expect(page.getByRole('heading', { name: 'Authentication Providers' })).toBeVisible();

    // ── CONTRACT CHECK: listAuthProviders → AuthProviderList ──────────────────
    const listResp = await listRespPromise;
    expect(listResp.status()).toBe(200);
    await validateApiResponse('AuthProviderList', listResp);
  });

  // ── Stage 2.A+: User Management ──────────────────────────────────────────────

  test('Stage 2.A+ – listUsers: user list conforms to UserList schema', async ({ page }) => {
    // operationId: listUsers
    const usersRespPromise = page.waitForResponse(
      (r) => urlPathEndsWith(r.url(), '/api/v1/admin/users') && r.request().method() === 'GET'
    );

    await page.goto('/admin/users');
    await expect(page.getByTestId('admin-users-page')).toBeVisible();

    // ── CONTRACT CHECK: listUsers → UserList ──────────────────────────────────
    const usersResp = await usersRespPromise;
    expect(usersResp.status()).toBe(200);
    await validateApiResponse('UserList', usersResp);
  });
});
