import { expect, test, type Page } from '@playwright/test';
import { selectAntOption } from './lib/helpers';

// ─────────────────────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────────────────────

function authStorageState() {
  return JSON.stringify({
    state: {
      token: 'test-token',
      user: {
        id: 'user-1',
        username: 'alice',
        permissions: ['platform:admin'],
      },
      isAuthenticated: true,
    },
    version: 0,
  });
}

interface MockMasterFlowOptions {
  onRequest?: (method: string, path: string, body: unknown) => void;
}

function visibleModal(page: Page) {
  return page.locator('.ant-modal-content:visible');
}

/**
 * Baseline API mock covering all common GET endpoints used across master-flow stages.
 * Individual tests can add more specific route overrides AFTER calling this.
 */
async function mockMasterFlowBaselineApi(page: Page, options?: MockMasterFlowOptions) {
  await page.route('**/api/v1/**', async (route) => {
    const req = route.request();
    const url = new URL(req.url());
    const path = url.pathname;
    const method = req.method();
    let body: unknown = undefined;
    try {
      body = req.postDataJSON();
    } catch {
      body = req.postData();
    }
    options?.onRequest?.(method, path, body);

    const json = (data: unknown, status = 200) =>
      route.fulfill({
        status,
        contentType: 'application/json',
        body: JSON.stringify(data),
      });

    // ── Notifications ──────────────────────────────────────────────────────
    if (method === 'GET' && path.endsWith('/notifications/unread-count')) {
      return json({ count: 2 });
    }
    if (method === 'GET' && path.endsWith('/notifications')) {
      return json({
        items: [
          {
            id: 'notif-1',
            type: 'APPROVAL_PENDING',
            title: 'VM Request Submitted',
            message: 'Your VM request is pending approval',
            resource_type: 'ticket',
            resource_id: 'ticket-1',
            read: false,
            created_at: new Date().toISOString(),
          },
          {
            id: 'notif-2',
            type: 'APPROVAL_COMPLETED',
            title: 'VM Approved',
            message: 'Your VM request has been approved',
            resource_type: 'ticket',
            resource_id: 'ticket-1',
            read: false,
            created_at: new Date().toISOString(),
          },
        ],
        pagination: { page: 1, per_page: 10, total: 2, total_pages: 1 },
      });
    }

    // ── Systems ────────────────────────────────────────────────────────────
    if (method === 'GET' && path.endsWith('/systems')) {
      return json({
        items: [{ id: 'sys-1', name: 'shop', description: 'E-commerce core system', created_by: 'alice', created_at: new Date().toISOString() }],
        pagination: { page: 1, per_page: 20, total: 1, total_pages: 1 },
      });
    }
    if (method === 'GET' && /\/systems\/[^/]+$/.test(path)) {
      return json({ id: 'sys-1', name: 'shop', description: 'E-commerce core system', created_by: 'alice', created_at: new Date().toISOString() });
    }

    // ── System Members ─────────────────────────────────────────────────────
    if (method === 'GET' && /\/systems\/[^/]+\/members$/.test(path)) {
      return json({
        items: [
          { id: 'rrb-1', user_id: 'user-1', username: 'alice', role: 'owner', granted_by: 'alice', created_at: new Date().toISOString() },
          { id: 'rrb-2', user_id: 'user-2', username: 'bob', role: 'member', granted_by: 'alice', created_at: new Date().toISOString() },
        ],
        pagination: { page: 1, per_page: 20, total: 2, total_pages: 1 },
      });
    }

    // ── Services ───────────────────────────────────────────────────────────
    if (method === 'GET' && /\/systems\/[^/]+\/services$/.test(path)) {
      return json({
        items: [{ id: 'svc-1', system_id: 'sys-1', name: 'redis', description: 'Cache service', created_at: new Date().toISOString(), next_instance_index: 1 }],
        pagination: { page: 1, per_page: 20, total: 1, total_pages: 1 },
      });
    }
    if (method === 'GET' && path.endsWith('/services') && !path.includes('/systems/')) {
      // Global services list (used by ServicesManagementContent)
      return json({
        items: [{ id: 'svc-1', system_id: 'sys-1', name: 'redis', description: 'Cache service', created_at: new Date().toISOString(), next_instance_index: 1 }],
        pagination: { page: 1, per_page: 20, total: 1, total_pages: 1 },
      });
    }

    // ── VM Request Context ────────────────────────────────────────────────
    if (method === 'GET' && path.endsWith('/vms/request-context')) {
      return json({
        templates: [{ id: 'tpl-1', name: 'ubuntu-24', display_name: 'Ubuntu 24', enabled: true }],
        instance_sizes: [{ id: 'size-1', name: 'small', display_name: 'Small', cpu_cores: 2, memory_gi: 4, enabled: true }],
        namespaces: ['prod-shop'],
      });
    }

    // ── VMs ────────────────────────────────────────────────────────────────
    if (method === 'GET' && path.endsWith('/vms')) {
      return json({
        items: [],
        pagination: { page: 1, per_page: 20, total: 0, total_pages: 0 },
      });
    }

    // ── Templates ──────────────────────────────────────────────────────────
    if (method === 'GET' && path.endsWith('/templates')) {
      return json({
        items: [{ id: 'tpl-1', name: 'ubuntu-24', display_name: 'Ubuntu 24', enabled: true }],
        pagination: { page: 1, per_page: 20, total: 1, total_pages: 1 },
      });
    }

    // ── Instance Sizes ─────────────────────────────────────────────────────
    if (method === 'GET' && path.endsWith('/instance-sizes')) {
      return json({
        items: [{ id: 'size-1', name: 'small', display_name: 'Small', cpu_cores: 2, memory_gi: 4, enabled: true }],
        pagination: { page: 1, per_page: 20, total: 1, total_pages: 1 },
      });
    }

    // ── Namespaces ─────────────────────────────────────────────────────────
    if (method === 'GET' && path.endsWith('/admin/namespaces')) {
      return json({
        items: [{ id: 'ns-1', name: 'prod-shop', environment: 'prod', enabled: true }],
        pagination: { page: 1, per_page: 20, total: 1, total_pages: 1 },
      });
    }

    // ── Built-in approval tasks ────────────────────────────────────────────
    if (method === 'GET' && path.endsWith('/builtin-approval/tasks')) {
      return json({
        items: [{ id: 'ticket-1', status: 'PENDING', operation_type: 'CREATE', requester: 'alice', created_at: new Date().toISOString() }],
        pagination: { page: 1, per_page: 20, total: 1, total_pages: 1 },
      });
    }

    // ── Clusters ───────────────────────────────────────────────────────────
    if (method === 'GET' && path.endsWith('/admin/clusters')) {
      return json({
        items: [{ id: 'cluster-1', name: 'Cluster A', display_name: 'Cluster A', status: 'HEALTHY', enabled: true }],
        pagination: { page: 1, per_page: 20, total: 1, total_pages: 1 },
      });
    }

    // ── Users (for member search) ──────────────────────────────────────────
    if (method === 'GET' && path.endsWith('/admin/users')) {
      return json({
        items: [
          { id: 'user-1', username: 'alice', email: 'alice@example.com' },
          { id: 'user-2', username: 'bob', email: 'bob@example.com' },
        ],
        pagination: { page: 1, per_page: 20, total: 2, total_pages: 1 },
      });
    }

    // ── VM / Batch mutations with typed responses ─────────────────────────
    if (method === 'POST' && path.endsWith('/vms/request')) {
      return json({ ticket_id: 'ticket-vm-create-1', status: 'PENDING', operation_type: 'CREATE' }, 202);
    }
    if (method === 'POST' && path.endsWith('/vms/batch')) {
      return json({
        batch_id: 'batch-create-1',
        status: 'PENDING_APPROVAL',
        status_url: '/api/v1/vms/batch/batch-create-1',
        retry_after_seconds: 2,
      }, 202);
    }
    if (method === 'POST' && path.endsWith('/vms/batch/power')) {
      return json({
        batch_id: 'batch-power-1',
        status: 'PENDING_APPROVAL',
        status_url: '/api/v1/vms/batch/batch-power-1',
        retry_after_seconds: 2,
      }, 202);
    }
    if (method === 'POST' && path.endsWith('/vms/batch')) {
      return json({
        batch_id: 'batch-vm-1',
        status: 'PENDING_APPROVAL',
        status_url: '/api/v1/vms/batch/batch-vm-1',
        retry_after_seconds: 2,
      }, 202);
    }
    if (method === 'DELETE' && /\/vms\/[^/]+$/.test(path)) {
      return json({ ticket_id: 'ticket-delete-1', event_id: 'event-1', status: 'PENDING' }, 202);
    }

    // ── Catch-all for mutations ────────────────────────────────────────────
    if (method === 'POST' || method === 'PATCH' || method === 'PUT') {
      return json({ ok: true }, 200);
    }
    if (method === 'DELETE') {
      return json({}, 204);
    }
    return json({}, 200);
  });
}

// ─────────────────────────────────────────────────────────────────────────────
// Test Suite
// ─────────────────────────────────────────────────────────────────────────────

test.describe('master-flow mock smoke interactions', () => {

  // ── Auth Guard ────────────────────────────────────────────────────────────
  test('redirects unauthenticated users to login', async ({ page }) => {
    await page.goto('/systems');
    await page.waitForURL(/\/login$/, { timeout: 15000 });
    await expect(page).toHaveURL(/\/login$/);
  });

  // ── Stage 4/5 Navigation ──────────────────────────────────────────────────
  test('authenticated user can navigate core Stage 4/5 pages and open VM request wizard', async ({ page }) => {
    await page.addInitScript((storageValue) => {
      window.localStorage.setItem('shepherd-auth', storageValue);
    }, authStorageState());
    await mockMasterFlowBaselineApi(page);

    await page.goto('/systems');
    await expect(page.getByRole('heading', { name: 'Systems' })).toBeVisible();
    await expect(page.getByRole('button', { name: 'Create' })).toBeVisible();

    await page.goto('/services');
    await expect(page.getByRole('heading', { name: 'Services' })).toBeVisible();

    await page.goto('/vms');
    await expect(page.getByRole('heading', { name: 'Virtual Machines' })).toBeVisible();

    await expect(
      page.locator('button').filter({
        has: page.locator('.anticon-plus'),
      }).first()
    ).toBeVisible();
  });

  // ── Stage 4.A: User Creates System ───────────────────────────────────────
  test('Stage 4.A: user can create a System via modal (POST /systems)', async ({ page }) => {
    await page.addInitScript((storageValue) => {
      window.localStorage.setItem('shepherd-auth', storageValue);
    }, authStorageState());

    const captured: Array<{ method: string; path: string; body: unknown }> = [];
    await mockMasterFlowBaselineApi(page, {
      onRequest: (method, path, body) => {
        captured.push({ method, path, body });
      },
    });

    await page.goto('/systems');
    await expect(page.getByRole('heading', { name: 'Systems' })).toBeVisible();

    // Open create modal
    await page.getByTestId('system-create-button').click();

    const modal = page.locator('.ant-modal-content:visible');
    await expect(modal).toBeVisible();

    // Fill in system name
    await modal.locator('input[placeholder]').first().fill('myapp');

    // Submit
    await modal.getByRole('button', { name: 'OK' }).click();

    // Verify POST /systems was called
    await expect.poll(() =>
      captured.some((r) => r.method === 'POST' && r.path.endsWith('/systems'))
    ).toBeTruthy();
  });

  // ── Stage 4.A: Edit System (PATCH /systems/{id}) ─────────────────────────
  test('Stage 4.A: user can edit System description (PATCH /systems/{id})', async ({ page }) => {
    await page.addInitScript((storageValue) => {
      window.localStorage.setItem('shepherd-auth', storageValue);
    }, authStorageState());

    const captured: Array<{ method: string; path: string }> = [];
    await mockMasterFlowBaselineApi(page, {
      onRequest: (method, path) => {
        captured.push({ method, path });
      },
    });

    await page.goto('/systems');
    await expect(page.getByRole('heading', { name: 'Systems' })).toBeVisible();

    // Click edit button for sys-1
    await page.getByTestId('system-action-edit-sys-1').click();

    const modal = page.locator('.ant-modal-content:visible');
    await expect(modal).toBeVisible();

    // Update description
    const textarea = modal.locator('textarea').first();
    await textarea.clear();
    await textarea.fill('Updated description');

    // Submit
    await modal.getByRole('button', { name: 'OK' }).click();

    // Verify PATCH /systems/{id} was called
    await expect.poll(() =>
      captured.some((r) => r.method === 'PATCH' && /\/systems\/[^/]+$/.test(r.path))
    ).toBeTruthy();
  });

  // ── Stage 4.A: Delete System ──────────────────────────────────────────────
  test('Stage 4.A: user can delete System with name confirmation (DELETE /systems/{id})', async ({ page }) => {
    await page.addInitScript((storageValue) => {
      window.localStorage.setItem('shepherd-auth', storageValue);
    }, authStorageState());

    const captured: Array<{ method: string; path: string }> = [];
    await mockMasterFlowBaselineApi(page, {
      onRequest: (method, path) => {
        captured.push({ method, path });
      },
    });

    await page.goto('/systems');
    await expect(page.getByRole('heading', { name: 'Systems' })).toBeVisible();

    // Click delete button for sys-1
    await page.getByTestId('system-action-delete-sys-1').click();

    const modal = page.locator('.ant-modal-content:visible');
    await expect(modal).toBeVisible();

    // Type the system name to confirm
    const confirmInput = modal.locator('input').last();
    await expect(confirmInput).toBeVisible();
    await confirmInput.fill('shop');

    // Delete button should now be enabled
    const deleteBtn = modal.getByRole('button', { name: 'Delete' });
    await expect(deleteBtn).toBeEnabled({ timeout: 5000 });
    await deleteBtn.click();

    // Verify DELETE /systems/{id} was called
    await expect.poll(() =>
      captured.some((r) => r.method === 'DELETE' && /\/systems\/[^/]+$/.test(r.path))
    ).toBeTruthy();
  });

  // ── Stage 4.A+: Resource Member Management ───────────────────────────────
  test('Stage 4.A+: owner can open System members modal', async ({ page }) => {
    await page.addInitScript((storageValue) => {
      window.localStorage.setItem('shepherd-auth', storageValue);
    }, authStorageState());

    await mockMasterFlowBaselineApi(page);

    await page.goto('/systems');
    await expect(page.getByRole('heading', { name: 'Systems' })).toBeVisible();

    // Click members button for sys-1
    await page.getByTestId('system-action-members-sys-1').click();

    // Members modal should open (look for modal with members-related content)
    const modal = page.locator('.ant-modal-content:visible');
    await expect(modal).toBeVisible({ timeout: 5000 });
  });

  // ── Stage 4.B: User Creates Service ──────────────────────────────────────
  test('Stage 4.B: user can create a Service under a System (POST /systems/{id}/services)', async ({ page }) => {
    await page.addInitScript((storageValue) => {
      window.localStorage.setItem('shepherd-auth', storageValue);
    }, authStorageState());

    const captured: Array<{ method: string; path: string; body: unknown }> = [];
    await mockMasterFlowBaselineApi(page, {
      onRequest: (method, path, body) => {
        captured.push({ method, path, body });
      },
    });

    await page.goto('/services');
    await expect(page.getByRole('heading', { name: 'Services' })).toBeVisible();

    // Open create modal
    await page.getByTestId('service-create-button').click();

    const modal = visibleModal(page);
    await expect(modal).toBeVisible();

    // Fill service name
    await modal.locator('input#create-service_name').fill('redis');

    // Submit
    await modal.getByRole('button', { name: 'OK' }).click();

    // Verify POST to services endpoint was called
    await expect.poll(() =>
      captured.some((r) => r.method === 'POST' && /\/systems\/[^/]+\/services$/.test(r.path))
    ).toBeTruthy();
  });

  // ── Stage 4.B: Edit Service (PATCH /systems/{id}/services/{id}) ──────────
  test('Stage 4.B: user can edit Service description', async ({ page }) => {
    await page.addInitScript((storageValue) => {
      window.localStorage.setItem('shepherd-auth', storageValue);
    }, authStorageState());

    const captured: Array<{ method: string; path: string }> = [];
    await mockMasterFlowBaselineApi(page, {
      onRequest: (method, path) => {
        captured.push({ method, path });
      },
    });

    await page.goto('/services');
    await expect(page.getByRole('heading', { name: 'Services' })).toBeVisible();

    // Click edit button for svc-1
    await page.getByTestId('service-action-edit-svc-1').click();

    const modal = page.locator('.ant-modal-content:visible');
    await expect(modal).toBeVisible();

    // Update description
    const textarea = modal.locator('textarea').first();
    await textarea.clear();
    await textarea.fill('Updated cache service description');

    // Submit
    await modal.getByRole('button', { name: 'OK' }).click();

    // Verify PATCH was called on services endpoint
    await expect.poll(() =>
      captured.some((r) => r.method === 'PATCH' && /\/services\/[^/]+$/.test(r.path))
    ).toBeTruthy();
  });

  // ── Stage 4.B: Delete Service (Popconfirm) ────────────────────────────────
  test('Stage 4.B: user can delete Service via Popconfirm', async ({ page }) => {
    await page.addInitScript((storageValue) => {
      window.localStorage.setItem('shepherd-auth', storageValue);
    }, authStorageState());

    const captured: Array<{ method: string; path: string }> = [];
    await mockMasterFlowBaselineApi(page, {
      onRequest: (method, path) => {
        captured.push({ method, path });
      },
    });

    await page.goto('/services');
    await expect(page.getByRole('heading', { name: 'Services' })).toBeVisible();

    // Click delete button for svc-1 (triggers Popconfirm)
    await page.getByTestId('service-action-delete-svc-1').click();

    // Confirm in Popconfirm
    const popconfirm = page.locator('.ant-popover:visible');
    await expect(popconfirm).toBeVisible({ timeout: 5000 });
    await popconfirm.getByRole('button', { name: 'Confirm' }).click();

    // Verify DELETE was called
    await expect.poll(() =>
      captured.some((r) => r.method === 'DELETE' && /\/services\/[^/]+$/.test(r.path))
    ).toBeTruthy();
  });

  // ── Stage 5.A: VM Request Wizard ─────────────────────────────────────────
  test('Stage 5.A: VM request wizard opens and submits POST /vms/request', async ({ page }) => {
    await page.addInitScript((storageValue) => {
      window.localStorage.setItem('shepherd-auth', storageValue);
    }, authStorageState());

    const captured: Array<{ method: string; path: string; body: unknown }> = [];
    await mockMasterFlowBaselineApi(page, {
      onRequest: (method, path, body) => {
        captured.push({ method, path, body });
      },
    });

    await page.goto('/vms');
    await expect(page.getByRole('heading', { name: 'Virtual Machines' })).toBeVisible();

    // Open VM request wizard via the + button
    const createBtn = page.locator('button').filter({ has: page.locator('.anticon-plus') }).first();
    await createBtn.click();

    // Wizard modal should open
    const modal = visibleModal(page);
    await expect(modal).toBeVisible({ timeout: 5000 });

    // Step 0: Select System
    await selectAntOption(page, modal.locator('.ant-select').nth(0));

    // Select Service
    await selectAntOption(page, modal.locator('.ant-select').nth(1));

    // Click Next
    await modal.getByRole('button', { name: 'Next' }).click();

    // Step 1: Select Template
    await selectAntOption(page, modal.locator('.ant-select').nth(0));

    // Click Next
    await modal.getByRole('button', { name: 'Next' }).click();

    // Step 2: Select Instance Size
    await selectAntOption(page, modal.locator('.ant-select').nth(0));

    // Click Next
    await modal.getByRole('button', { name: 'Next' }).click();

    // Step 3: Namespace + Reason
    const namespaceInput = modal.locator('input#vm-request-wizard_namespace');
    await namespaceInput.fill('prod-shop');

    const reasonTextarea = modal.locator('textarea#vm-request-wizard_reason');
    await reasonTextarea.fill('Production deployment for e-commerce');

    // Click Next
    await modal.getByRole('button', { name: 'Next' }).click();

    // Step 4: Confirm & Submit
    await expect(modal.getByRole('button', { name: 'Submit' })).toBeVisible({ timeout: 5000 });
    await modal.getByRole('button', { name: 'Submit' }).click();

    // Verify POST /vms/request was called
    await expect.poll(() =>
      captured.some((r) => r.method === 'POST' && r.path.endsWith('/vms/request'))
    ).toBeTruthy();
  });

  // ── Stage 5.D: Delete VM (with ticket) ────────────────────────────────────
  test('Stage 5.D: VM delete action triggers approval flow (POST /vms/{id}/delete)', async ({ page }) => {
    await page.addInitScript((storageValue) => {
      window.localStorage.setItem('shepherd-auth', storageValue);
    }, authStorageState());

    const captured: Array<{ method: string; path: string }> = [];
    await mockMasterFlowBaselineApi(page, {
      onRequest: (method, path) => {
        captured.push({ method, path });
      },
    });

    // Override VMs list to have a running VM
    await page.route('**/api/v1/vms**', async (route) => {
      if (route.request().method() !== 'GET') return route.fallback();
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          items: [
            { id: 'vm-1', name: 'vm-1', namespace: 'prod-shop', status: 'STOPPED', created_at: new Date().toISOString() },
          ],
          pagination: { page: 1, per_page: 20, total: 1, total_pages: 1 },
        }),
      });
    });

    await page.goto('/vms');
    await expect(page.getByRole('heading', { name: 'Virtual Machines' })).toBeVisible();

    // Click delete action for vm-1
    await page.getByTestId('vm-action-delete-vm-1').click();

    // Confirm in Popconfirm or modal
    const confirmEl = page.locator('.ant-popover:visible, .ant-modal-content:visible').first();
    await expect(confirmEl).toBeVisible({ timeout: 5000 });

    const confirmInput = confirmEl.locator('input');
    if (await confirmInput.count()) {
      await confirmInput.last().fill('vm-1');
    }

    // Click confirm/OK button
    const confirmBtn = confirmEl.getByRole('button', { name: /confirm|ok|delete/i }).first();
    await expect(confirmBtn).toBeEnabled({ timeout: 5000 });
    await confirmBtn.click();

    // Verify DELETE or POST to delete endpoint was called
    await expect.poll(() =>
      captured.some((r) =>
        (r.method === 'DELETE' || r.method === 'POST') &&
        /\/vms\/[^/]+/.test(r.path)
      )
    ).toBeTruthy();
  });

  // ── Stage 5.D: Delete System cascade guard ────────────────────────────────
  test('Stage 5.D: System delete requires name confirmation (cascade guard)', async ({ page }) => {
    await page.addInitScript((storageValue) => {
      window.localStorage.setItem('shepherd-auth', storageValue);
    }, authStorageState());

    await mockMasterFlowBaselineApi(page);

    await page.goto('/systems');
    await expect(page.getByRole('heading', { name: 'Systems' })).toBeVisible();

    // Open delete modal
    await page.getByTestId('system-action-delete-sys-1').click();

    const modal = page.locator('.ant-modal-content:visible');
    await expect(modal).toBeVisible();

    // Delete button should be disabled before typing name
    const deleteBtn = modal.getByRole('button', { name: 'Delete' });
    await expect(deleteBtn).toBeDisabled();

    // Type wrong name – still disabled
    const confirmInput = modal.locator('input').last();
    await expect(confirmInput).toBeVisible();
    await confirmInput.fill('wrong-name');
    await expect(deleteBtn).toBeDisabled();

    // Clear and type correct name
    await confirmInput.clear();
    await confirmInput.fill('shop');
    await expect(deleteBtn).toBeEnabled({ timeout: 5000 });
  });

  // ── Stage 5.E: Batch Power Actions ───────────────────────────────────────
  test('Stage 5.E: batch power action from VM page triggers POST /vms/batch/power', async ({ page }) => {
    await page.addInitScript((storageValue) => {
      window.localStorage.setItem('shepherd-auth', storageValue);
    }, authStorageState());

    const captured: Array<{ method: string; path: string; body: unknown }> = [];
    await mockMasterFlowBaselineApi(page, {
      onRequest: (method, path, body) => {
        captured.push({ method, path, body });
      },
    });

    await page.route('**/api/v1/vms**', async (route) => {
      if (route.request().method() !== 'GET') {
        return route.fallback();
      }
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          items: [
            { id: 'vm-1', name: 'vm-1', namespace: 'test', status: 'STOPPED', created_at: new Date().toISOString() },
            { id: 'vm-2', name: 'vm-2', namespace: 'test', status: 'STOPPED', created_at: new Date().toISOString() },
          ],
          pagination: { page: 1, per_page: 20, total: 2, total_pages: 1 },
        }),
      });
    });

    await page.goto('/vms');
    await expect(page.getByRole('heading', { name: 'Virtual Machines' })).toBeVisible();
    const vmRow = page.locator('tr').filter({ hasText: 'vm-1' }).first();
    await vmRow.getByRole('checkbox').check();
    await page.getByRole('button', { name: 'Start Selected', exact: true }).click();

    await expect.poll(() => captured.some((r) => r.method === 'POST' && r.path.endsWith('/vms/batch/power'))).toBeTruthy();
  });

  // ── Stage 5.E: Batch VM Request (POST /vms/batch) ────────────────────────
  test('Stage 5.E: batch VM request triggers POST /vms/batch', async ({ page }) => {
    await page.addInitScript((storageValue) => {
      window.localStorage.setItem('shepherd-auth', storageValue);
    }, authStorageState());

    const captured: Array<{ method: string; path: string; body: unknown }> = [];
    await mockMasterFlowBaselineApi(page, {
      onRequest: (method, path, body) => {
        captured.push({ method, path, body });
      },
    });

    await page.goto('/vms');
    await expect(page.getByRole('heading', { name: 'Virtual Machines' })).toBeVisible();

    // Open VM request wizard
    const createBtn = page.locator('button').filter({ has: page.locator('.anticon-plus') }).first();
    await createBtn.click();

    const modal = visibleModal(page);
    await expect(modal).toBeVisible({ timeout: 5000 });

    // Step 0: Select System + Service
    await selectAntOption(page, modal.locator('.ant-select').nth(0));
    await selectAntOption(page, modal.locator('.ant-select').nth(1));

    await modal.getByRole('button', { name: 'Next' }).click();

    // Step 1: Template
    await selectAntOption(page, modal.locator('.ant-select').nth(0));
    await modal.getByRole('button', { name: 'Next' }).click();

    // Step 2: Instance Size
    await selectAntOption(page, modal.locator('.ant-select').nth(0));
    await modal.getByRole('button', { name: 'Next' }).click();

    // Step 3: Namespace + Reason + Batch Count > 1
    const namespaceInput = modal.locator('input#vm-request-wizard_namespace');
    await namespaceInput.fill('prod-shop');

    const reasonTextarea = modal.locator('textarea#vm-request-wizard_reason');
    await reasonTextarea.fill('Batch deployment');

    // Set batch count to 3
    const batchCountInput = modal.locator('input#vm-request-wizard_batch_count');
    await batchCountInput.click();
    await batchCountInput.press('Control+A');
    await batchCountInput.fill('3');
    await batchCountInput.press('Enter');
    await expect(batchCountInput).toHaveValue('3');

    await modal.getByRole('button', { name: 'Next' }).click();

    // Step 4: Submit
    await expect(modal.getByRole('button', { name: 'Submit' })).toBeVisible({ timeout: 5000 });
    await modal.getByRole('button', { name: 'Submit' }).click();

    // Verify POST /vms/batch was called
    await expect.poll(() =>
      captured.some((r) => r.method === 'POST' && r.path.endsWith('/vms/batch'))
    ).toBeTruthy();
  });

  // ── Stage 5.F: Notification Bell ─────────────────────────────────────────
  test('Stage 5.F: notification bell shows unread count and navigates to notifications page', async ({ page }) => {
    await page.addInitScript((storageValue) => {
      window.localStorage.setItem('shepherd-auth', storageValue);
    }, authStorageState());
    await mockMasterFlowBaselineApi(page);

    await page.goto('/dashboard');
    await expect(page.getByRole('heading', { name: 'Dashboard' })).toBeVisible();
    await page.getByTestId('notification-bell-trigger').click();
    const popover = page.locator('.ant-popover:visible').last();
    await expect(popover.getByTestId('notification-view-all')).toBeVisible({ timeout: 5000 });
    await Promise.all([
      page.waitForURL(/\/notifications$/),
      popover.getByTestId('notification-view-all').click(),
    ]);

    await expect(page.getByRole('heading', { name: 'Notifications' })).toBeVisible();
  });

  // ── Stage 5.F: Notifications list shows items ────────────────────────────
  test('Stage 5.F: notifications page renders notification items from API', async ({ page }) => {
    await page.addInitScript((storageValue) => {
      window.localStorage.setItem('shepherd-auth', storageValue);
    }, authStorageState());
    await mockMasterFlowBaselineApi(page);

    await page.goto('/notifications');
    await expect(page.getByRole('heading', { name: 'Notifications' })).toBeVisible();

    // Notifications list should contain items from mock
    await expect(page.getByText('VM Request Submitted')).toBeVisible({ timeout: 5000 });
    await expect(page.getByText('VM Approved')).toBeVisible({ timeout: 5000 });
  });

  // ── Stage 6: VM Console Access ────────────────────────────────────────────
  test('Stage 6: VM console request follows request → open flow', async ({ page }) => {
    await page.addInitScript((storageValue) => {
      window.localStorage.setItem('shepherd-auth', storageValue);
      // avoid browser popup interference in CI
      window.open = () => null;
    }, authStorageState());

    const seen = {
      vmDetail: false,
      consoleRequest: false,
      vncSession: false,
    };
    await mockMasterFlowBaselineApi(page, {
      onRequest: () => { },
    });

    await page.route('**/api/v1/vms**', async (route) => {
      if (route.request().method() !== 'GET') {
        return route.fallback();
      }
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          items: [
            { id: 'vm-1', name: 'vm-1', namespace: 'test', status: 'RUNNING', created_at: new Date().toISOString() },
          ],
          pagination: { page: 1, per_page: 20, total: 1, total_pages: 1 },
        }),
      });
    });

    await page.route('**/api/v1/vms/vm-1', async (route) => {
      if (route.request().method() !== 'GET') {
        return route.fallback();
      }
      seen.vmDetail = true;
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ id: 'vm-1', name: 'vm-1', namespace: 'test', status: 'RUNNING' }),
      });
    });

    await page.route('**/api/v1/vms/vm-1/console/request', async (route) => {
      seen.consoleRequest = true;
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ status: 'APPROVED' }),
      });
    });

    await page.route('**/api/v1/vms/vm-1/vnc', async (route) => {
      seen.vncSession = true;
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          status: 'SESSION_READY',
          vm_id: 'vm-1',
          websocket_path: '/api/v1/vms/vm-1/vnc',
        }),
      });
    });

    await page.goto('/vms');
    await expect(page.getByRole('heading', { name: 'Virtual Machines' })).toBeVisible();
    await page.getByTestId('vm-action-console-vm-1').click();

    await expect.poll(() => seen.vmDetail).toBeTruthy();
    await expect.poll(() => seen.consoleRequest).toBeTruthy();
    await expect.poll(() => seen.vncSession).toBeTruthy();
  });

  // ── Stage 6: VM Start/Stop individual actions ─────────────────────────────
  test('Stage 6: VM start action triggers POST /vms/{id}/start', async ({ page }) => {
    await page.addInitScript((storageValue) => {
      window.localStorage.setItem('shepherd-auth', storageValue);
    }, authStorageState());

    const captured: Array<{ method: string; path: string }> = [];
    await mockMasterFlowBaselineApi(page, {
      onRequest: (method, path) => {
        captured.push({ method, path });
      },
    });

    await page.route('**/api/v1/vms**', async (route) => {
      if (route.request().method() !== 'GET') return route.fallback();
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          items: [
            { id: 'vm-1', name: 'vm-1', namespace: 'test', status: 'STOPPED', created_at: new Date().toISOString() },
          ],
          pagination: { page: 1, per_page: 20, total: 1, total_pages: 1 },
        }),
      });
    });

    await page.goto('/vms');
    await expect(page.getByRole('heading', { name: 'Virtual Machines' })).toBeVisible();

    await page.getByTestId('vm-action-start-vm-1').click();

    await expect.poll(() =>
      captured.some((r) => r.method === 'POST' && /\/vms\/vm-1\/start$/.test(r.path))
    ).toBeTruthy();
  });

  test('Stage 6: VM stop action triggers POST /vms/{id}/stop', async ({ page }) => {
    await page.addInitScript((storageValue) => {
      window.localStorage.setItem('shepherd-auth', storageValue);
    }, authStorageState());

    const captured: Array<{ method: string; path: string }> = [];
    await mockMasterFlowBaselineApi(page, {
      onRequest: (method, path) => {
        captured.push({ method, path });
      },
    });

    await page.route('**/api/v1/vms**', async (route) => {
      if (route.request().method() !== 'GET') return route.fallback();
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          items: [
            { id: 'vm-1', name: 'vm-1', namespace: 'test', status: 'RUNNING', created_at: new Date().toISOString() },
          ],
          pagination: { page: 1, per_page: 20, total: 1, total_pages: 1 },
        }),
      });
    });

    await page.goto('/vms');
    await expect(page.getByRole('heading', { name: 'Virtual Machines' })).toBeVisible();

    await page.getByTestId('vm-action-stop-vm-1').click();

    await expect.poll(() =>
      captured.some((r) => r.method === 'POST' && /\/vms\/vm-1\/stop$/.test(r.path))
    ).toBeTruthy();
  });

  // ── Approvals page (Stage 5.B) ────────────────────────────────────────────
  test('Stage 5.B: built-in approval task page renders pending tickets', async ({ page }) => {
    await page.addInitScript((storageValue) => {
      window.localStorage.setItem('shepherd-auth', storageValue);
    }, authStorageState());
    await mockMasterFlowBaselineApi(page);

    await page.goto('/admin/approval-tasks');
    await expect(page.getByTestId('admin-approvals-page')).toBeVisible({ timeout: 5000 });

    // Should show the pending ticket from mock
    await expect(page.getByText('ticket-1')).toBeVisible({ timeout: 5000 });
  });
});
