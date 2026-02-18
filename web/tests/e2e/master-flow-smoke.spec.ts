import { expect, test, type Page } from '@playwright/test';

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

    const json = (body: unknown, status = 200) =>
      route.fulfill({
        status,
        contentType: 'application/json',
        body: JSON.stringify(body),
      });

    if (method === 'GET' && path.endsWith('/notifications/unread-count')) {
      return json({ count: 0 });
    }
    if (method === 'GET' && path.endsWith('/notifications')) {
      return json({ items: [], pagination: { page: 1, per_page: 10, total: 0, total_pages: 0 } });
    }
    if (method === 'GET' && path.endsWith('/systems')) {
      return json({
        items: [{ id: 'sys-1', name: 'System A', description: 'demo', created_by: 'alice', created_at: new Date().toISOString() }],
        pagination: { page: 1, per_page: 20, total: 1, total_pages: 1 },
      });
    }
    if (method === 'GET' && path.match(/\/systems\/[^/]+\/services$/)) {
      return json({
        items: [{ id: 'svc-1', system_id: 'sys-1', name: 'Service A', description: '', created_at: new Date().toISOString(), next_instance_index: 1 }],
        pagination: { page: 1, per_page: 20, total: 1, total_pages: 1 },
      });
    }
    if (method === 'GET' && path.endsWith('/vms')) {
      return json({
        items: [],
        pagination: { page: 1, per_page: 20, total: 0, total_pages: 0 },
      });
    }
    if (method === 'GET' && path.endsWith('/templates')) {
      return json({
        items: [{ id: 'tpl-1', name: 'ubuntu-24', display_name: 'Ubuntu 24', enabled: true }],
        pagination: { page: 1, per_page: 20, total: 1, total_pages: 1 },
      });
    }
    if (method === 'GET' && path.endsWith('/instance-sizes')) {
      return json({
        items: [{ id: 'size-1', name: 'small', display_name: 'Small', cpu_cores: 2, memory_mb: 4096, enabled: true }],
        pagination: { page: 1, per_page: 20, total: 1, total_pages: 1 },
      });
    }
    if (method === 'GET' && path.endsWith('/approvals')) {
      return json({
        items: [{ id: 'ticket-1', status: 'PENDING', operation_type: 'CREATE', requester: 'alice', created_at: new Date().toISOString() }],
        pagination: { page: 1, per_page: 20, total: 1, total_pages: 1 },
      });
    }
    if (method === 'GET' && path.endsWith('/admin/clusters')) {
      return json({
        items: [{ id: 'cluster-1', name: 'Cluster A', display_name: 'Cluster A', status: 'HEALTHY', enabled: true }],
        pagination: { page: 1, per_page: 20, total: 1, total_pages: 1 },
      });
    }

    if (method === 'POST' || method === 'PATCH' || method === 'PUT') {
      return json({ ok: true }, 200);
    }
    if (method === 'DELETE') {
      return json({}, 204);
    }
    return json({}, 200);
  });
}

test.describe('master-flow mock smoke interactions', () => {
  test('redirects unauthenticated users to login', async ({ page }) => {
    await page.goto('/systems');
    await expect(page).toHaveURL(/\/login$/);
  });

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

  test('notification bell can navigate to full notifications page (Stage 5.F)', async ({ page }) => {
    await page.addInitScript((storageValue) => {
      window.localStorage.setItem('shepherd-auth', storageValue);
    }, authStorageState());
    await mockMasterFlowBaselineApi(page);

    await page.goto('/dashboard');
    await expect(page.getByRole('heading', { name: 'Dashboard' })).toBeVisible();
    await page.getByTestId('notification-bell-trigger').click();
    await page.getByTestId('notification-view-all').click();

    await expect(page).toHaveURL(/\/notifications$/);
    await expect(page.getByRole('heading', { name: 'Notifications' })).toBeVisible();
  });

  test('batch power action from VM page triggers Stage 5.E API', async ({ page }) => {
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

  test('VM console request follows Stage 6 request -> open flow', async ({ page }) => {
    await page.addInitScript((storageValue) => {
      window.localStorage.setItem('shepherd-auth', storageValue);
      // avoid browser popup interference in CI
      window.open = () => null;
    }, authStorageState());

    const captured: Array<{ method: string; path: string }> = [];
    const seen = {
      vmDetail: false,
      consoleRequest: false,
      vncSession: false,
    };
    await mockMasterFlowBaselineApi(page, {
      onRequest: (method, path) => {
        captured.push({ method, path });
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
});
