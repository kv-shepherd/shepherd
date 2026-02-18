import { expect, test, type Page } from '@playwright/test';

const e2eUsername = process.env.E2E_USERNAME ?? 'e2e-admin';
const e2ePassword = process.env.E2E_PASSWORD ?? 'e2e-admin-123';
const e2eSystemName = process.env.E2E_SYSTEM ?? 'e2e-system';
const e2eServiceName = process.env.E2E_SERVICE ?? 'e2e-service';
const runningVMID = process.env.E2E_VM_RUNNING_ID ?? 'vm-e2e-running';
const stoppedVMName = 'vm-stopped';

async function login(page: Page) {
  await page.goto('/login');
  await expect(page.getByRole('heading', { name: 'KubeVirt Shepherd' })).toBeVisible();

  await page.getByPlaceholder('Username').fill(e2eUsername);
  await page.getByPlaceholder('Password').fill(e2ePassword);

  const loginResponse = page.waitForResponse((resp) =>
    resp.url().endsWith('/api/v1/auth/login') && resp.request().method() === 'POST'
  );
  await page.getByRole('button', { name: 'Login' }).click();

  const resp = await loginResponse;
  expect(resp.status()).toBe(200);
  await expect(page).toHaveURL(/\/dashboard$/);
}

test.describe('master-flow live interactions (no mock)', () => {
  test.beforeEach(async ({ page }) => {
    await page.addInitScript(() => {
      window.open = () => null;
    });
    await login(page);
  });

  test('navigates core Stage 4/5 pages and opens VM request wizard', async ({ page }) => {
    await page.goto('/systems');
    await expect(page.getByRole('heading', { name: 'Systems' })).toBeVisible();
    await expect(page.getByRole('button', { name: 'Create' })).toBeVisible();

    await page.goto('/services');
    await expect(page.getByRole('heading', { name: 'Services' })).toBeVisible();

    await page.goto('/vms');
    await expect(page.getByRole('heading', { name: 'Virtual Machines' })).toBeVisible();

    await page.getByRole('button', { name: 'Request VM' }).click();
    await expect(page.getByText('Create VM Request')).toBeVisible();
  });

  test('notification bell navigates to notifications page (Stage 5.F)', async ({ page }) => {
    await page.goto('/dashboard');
    await page.getByTestId('notification-bell-trigger').click();
    await page.getByTestId('notification-view-all').click();

    await expect(page).toHaveURL(/\/notifications$/);
    await expect(page.getByRole('heading', { name: 'Notifications' })).toBeVisible();
  });

  test('batch power action triggers real Stage 5.E API', async ({ page }) => {
    await page.goto('/vms');
    await expect(page.getByRole('heading', { name: 'Virtual Machines' })).toBeVisible();

    const vmRow = page.locator('tr').filter({ hasText: stoppedVMName }).first();
    await expect(vmRow).toBeVisible();
    await vmRow.getByRole('checkbox').check();

    const powerRespPromise = page.waitForResponse((resp) =>
      resp.url().endsWith('/api/v1/vms/batch/power') &&
      resp.request().method() === 'POST'
    );
    await page.getByRole('button', { name: 'Start Selected', exact: true }).click();

    const powerResp = await powerRespPromise;
    expect(powerResp.status()).toBe(202);
    await expect(page.getByText('Current Batch')).toBeVisible();
  });

  test('console request follows real Stage 6 request -> open flow', async ({ page }) => {
    await page.goto('/vms');
    await expect(page.getByRole('heading', { name: 'Virtual Machines' })).toBeVisible();

    const requestRespPromise = page.waitForResponse((resp) =>
      resp.url().endsWith(`/api/v1/vms/${runningVMID}/console/request`) &&
      resp.request().method() === 'POST'
    );
    const vncRespPromise = page.waitForResponse((resp) =>
      resp.url().endsWith(`/api/v1/vms/${runningVMID}/vnc`) &&
      resp.request().method() === 'GET'
    );

    await page.getByTestId(`vm-action-console-${runningVMID}`).click();

    const requestResp = await requestRespPromise;
    expect(requestResp.status()).toBe(200);

    const vncResp = await vncRespPromise;
    expect(vncResp.status()).toBe(200);
  });

  test('system delete enforces confirm_name and calls real Stage 5.D API', async ({ page }) => {
    await page.goto('/systems');
    await expect(page.getByRole('heading', { name: 'Systems' })).toBeVisible();

    const createRespPromise = page.waitForResponse((resp) =>
      resp.url().endsWith('/api/v1/systems') && resp.request().method() === 'POST'
    );
    await page.getByRole('button', { name: 'Create' }).click();

    const createModal = page.locator('.ant-modal-content').filter({ hasText: 'Create System' });
    await expect(createModal).toBeVisible();

    const systemName = `e2ed${Date.now().toString(36).slice(-6)}`;
    await createModal.getByRole('textbox').first().fill(systemName);
    await createModal.getByRole('textbox').nth(1).fill('e2e delete flow test');
    await createModal.getByRole('button', { name: 'OK' }).click();

    const createResp = await createRespPromise;
    expect(createResp.status()).toBe(201);
    const createPayload = (await createResp.json()) as { id?: string };
    expect(createPayload.id).toBeTruthy();
    const systemId = createPayload.id ?? '';

    await expect(page.locator('tr').filter({ hasText: systemName }).first()).toBeVisible();
    await page.getByTestId(`system-action-delete-${systemId}`).click();

    const deleteModal = page.locator('.ant-modal-content').filter({ hasText: 'Delete System' });
    await expect(deleteModal).toBeVisible();

    const deleteConfirmButton = deleteModal.getByRole('button', { name: 'Delete' });
    await expect(deleteConfirmButton).toBeDisabled();

    const confirmInput = deleteModal.getByRole('textbox').first();
    await confirmInput.fill('wrong-name');
    await expect(deleteConfirmButton).toBeDisabled();
    await confirmInput.fill(systemName);
    await expect(deleteConfirmButton).toBeEnabled();

    const deleteRespPromise = page.waitForResponse((resp) =>
      resp.url().includes(`/api/v1/systems/${systemId}`) && resp.request().method() === 'DELETE'
    );
    await deleteConfirmButton.click();

    const deleteResp = await deleteRespPromise;
    expect(deleteResp.status()).toBe(204);
    expect(deleteResp.url()).toContain(`confirm_name=${systemName}`);
    await expect(page.locator('tr').filter({ hasText: systemName })).toHaveCount(0);
  });

  test('system/service create-update-delete follows Stage 4 + Stage 5.D success paths', async ({ page }) => {
    await page.goto('/systems');
    await expect(page.getByRole('heading', { name: 'Systems' })).toBeVisible();

    const systemName = `e2es${Date.now().toString(36).slice(-6)}`;
    const createSystemRespPromise = page.waitForResponse((resp) =>
      resp.url().endsWith('/api/v1/systems') && resp.request().method() === 'POST'
    );

    await page.getByTestId('system-create-button').click();
    const createSystemModal = page.locator('.ant-modal-content').filter({ hasText: 'Create System' });
    await expect(createSystemModal).toBeVisible();
    await createSystemModal.locator('input[maxlength="15"]').first().fill(systemName);
    await createSystemModal.locator('textarea').first().fill('system created by e2e');
    await createSystemModal.getByRole('button', { name: 'OK' }).click();

    const createSystemResp = await createSystemRespPromise;
    expect(createSystemResp.status()).toBe(201);
    const createdSystem = (await createSystemResp.json()) as { id?: string };
    expect(createdSystem.id).toBeTruthy();
    const systemID = createdSystem.id ?? '';
    await expect(page.locator('tr').filter({ hasText: systemName }).first()).toBeVisible();

    const updateSystemRespPromise = page.waitForResponse((resp) =>
      resp.url().includes(`/api/v1/systems/${systemID}`) && resp.request().method() === 'PATCH'
    );
    await page.getByTestId(`system-action-edit-${systemID}`).click();
    const editSystemModal = page.locator('.ant-modal-content').filter({ hasText: 'Edit System' });
    await expect(editSystemModal).toBeVisible();
    await editSystemModal.locator('textarea').first().fill('system updated by e2e');
    await editSystemModal.getByRole('button', { name: 'OK' }).click();
    const updateSystemResp = await updateSystemRespPromise;
    expect(updateSystemResp.status()).toBe(200);

    await page.goto('/services');
    await expect(page.getByRole('heading', { name: 'Services' })).toBeVisible();
    await page.getByTestId('services-system-selector').click();
    await page.locator('.ant-select-item-option').filter({ hasText: systemName }).first().click();

    const serviceName = `e2e-svc-${Date.now().toString(36).slice(-5)}`;
    const createServiceRespPromise = page.waitForResponse((resp) =>
      resp.url().includes(`/api/v1/systems/${systemID}/services`) &&
      resp.request().method() === 'POST'
    );
    await page.getByTestId('service-create-button').click();
    const createServiceModal = page.locator('.ant-modal-content').filter({ hasText: 'Create Service' });
    await expect(createServiceModal).toBeVisible();
    await createServiceModal.getByPlaceholder('e.g. web, api-gateway').fill(serviceName);
    await createServiceModal.locator('textarea').first().fill('service created by e2e');
    await createServiceModal.getByRole('button', { name: 'OK' }).click();

    const createServiceResp = await createServiceRespPromise;
    expect(createServiceResp.status()).toBe(201);
    const createdService = (await createServiceResp.json()) as { id?: string };
    expect(createdService.id).toBeTruthy();
    const serviceID = createdService.id ?? '';
    await expect(page.locator('tr').filter({ hasText: serviceName }).first()).toBeVisible();

    const updateServiceRespPromise = page.waitForResponse((resp) =>
      resp.url().includes(`/api/v1/systems/${systemID}/services/${serviceID}`) &&
      resp.request().method() === 'PATCH'
    );
    await page.getByTestId(`service-action-edit-${serviceID}`).click();
    const editServiceModal = page.locator('.ant-modal-content').filter({ hasText: 'Edit Service' });
    await expect(editServiceModal).toBeVisible();
    await editServiceModal.locator('textarea').first().fill('service updated by e2e');
    await editServiceModal.getByRole('button', { name: 'OK' }).click();
    const updateServiceResp = await updateServiceRespPromise;
    expect(updateServiceResp.status()).toBe(200);

    const deleteServiceRespPromise = page.waitForResponse((resp) =>
      resp.url().includes(`/api/v1/systems/${systemID}/services/${serviceID}`) &&
      resp.request().method() === 'DELETE'
    );
    await page.getByTestId(`service-action-delete-${serviceID}`).click();
    await page.getByRole('button', { name: 'Confirm' }).click();
    const deleteServiceResp = await deleteServiceRespPromise;
    expect(deleteServiceResp.status()).toBe(204);
    await expect(page.locator('tr').filter({ hasText: serviceName })).toHaveCount(0);

    await page.goto('/systems');
    await page.getByTestId(`system-action-delete-${systemID}`).click();
    const deleteSystemModal = page.locator('.ant-modal-content').filter({ hasText: 'Delete System' });
    await expect(deleteSystemModal).toBeVisible();
    await deleteSystemModal.getByRole('textbox').first().fill(systemName);
    const deleteSystemRespPromise = page.waitForResponse((resp) =>
      resp.url().includes(`/api/v1/systems/${systemID}`) &&
      resp.request().method() === 'DELETE'
    );
    await deleteSystemModal.getByRole('button', { name: 'Delete' }).click();
    const deleteSystemResp = await deleteSystemRespPromise;
    expect(deleteSystemResp.status()).toBe(204);
  });

  test('service delete sends confirm=true and returns conflict when child VMs exist', async ({ page }) => {
    await page.goto('/services');
    await expect(page.getByRole('heading', { name: 'Services' })).toBeVisible();

    await page.getByTestId('services-system-selector').click();
    await page.locator('.ant-select-item-option').filter({ hasText: e2eSystemName }).first().click();

    const serviceRow = page.locator('tr').filter({ hasText: e2eServiceName }).first();
    await expect(serviceRow).toBeVisible();

    await serviceRow.locator('[data-testid^="service-action-delete-"]').first().click();

    const deleteRespPromise = page.waitForResponse((resp) =>
      resp.url().includes('/api/v1/systems/') &&
      resp.url().includes('/services/') &&
      resp.request().method() === 'DELETE'
    );
    await page.getByRole('button', { name: 'Confirm' }).click();

    const deleteResp = await deleteRespPromise;
    expect(deleteResp.status()).toBe(409);
    expect(deleteResp.url()).toContain('confirm=true');
    const body = (await deleteResp.json()) as { code?: string };
    expect(body.code).toBe('SERVICE_HAS_VMS');
  });

  test('admin template flow performs create/delete against real Stage 3 API', async ({ page }) => {
    await page.goto('/admin/templates');
    await expect(page.getByRole('heading', { name: 'Templates' })).toBeVisible();

    const templateName = `e2e-template-${Date.now().toString(36).slice(-6)}`;
    const createRespPromise = page.waitForResponse((resp) =>
      resp.url().endsWith('/api/v1/admin/templates') &&
      resp.request().method() === 'POST'
    );

    await page.getByTestId('admin-template-create-button').click();
    const createModal = page.locator('.ant-modal-content').last();
    await createModal.getByRole('textbox').first().fill(templateName);
    await createModal.getByRole('button', { name: 'OK' }).click();

    const createResp = await createRespPromise;
    expect(createResp.status()).toBe(201);
    const created = (await createResp.json()) as { id?: string };
    expect(created.id).toBeTruthy();
    const templateID = created.id ?? '';

    await expect(page.locator('tr').filter({ hasText: templateName }).first()).toBeVisible();
    await page.getByTestId(`admin-template-action-delete-${templateID}`).click();

    const deleteRespPromise = page.waitForResponse((resp) =>
      resp.url().includes(`/api/v1/admin/templates/${templateID}`) &&
      resp.request().method() === 'DELETE'
    );
    await page.locator('.ant-modal-content').last().getByRole('button', { name: 'OK' }).click();

    const deleteResp = await deleteRespPromise;
    expect(deleteResp.status()).toBe(204);
    await expect(page.locator('tr').filter({ hasText: templateName })).toHaveCount(0);
  });

  test('admin instance-size flow performs create/delete against real Stage 3 API', async ({ page }) => {
    await page.goto('/admin/instance-sizes');
    await expect(page.getByRole('heading', { name: 'Instance Sizes' })).toBeVisible();

    const sizeName = `e2e-size-${Date.now().toString(36).slice(-6)}`;
    const createRespPromise = page.waitForResponse((resp) =>
      resp.url().endsWith('/api/v1/admin/instance-sizes') &&
      resp.request().method() === 'POST'
    );

    await page.getByTestId('admin-instance-size-create-button').click();
    const createModal = page.locator('.ant-modal-content').last();
    await createModal.getByRole('textbox').first().fill(sizeName);
    const numberInputs = createModal.getByRole('spinbutton');
    await numberInputs.nth(0).fill('4');
    await numberInputs.nth(1).fill('8192');
    await createModal.getByRole('button', { name: 'OK' }).click();

    const createResp = await createRespPromise;
    expect(createResp.status()).toBe(201);
    const created = (await createResp.json()) as { id?: string };
    expect(created.id).toBeTruthy();
    const instanceSizeID = created.id ?? '';

    await expect(page.locator('tr').filter({ hasText: sizeName }).first()).toBeVisible();
    await page.getByTestId(`admin-instance-size-action-delete-${instanceSizeID}`).click();

    const deleteRespPromise = page.waitForResponse((resp) =>
      resp.url().includes(`/api/v1/admin/instance-sizes/${instanceSizeID}`) &&
      resp.request().method() === 'DELETE'
    );
    await page.locator('.ant-modal-content').last().getByRole('button', { name: 'OK' }).click();

    const deleteResp = await deleteRespPromise;
    expect(deleteResp.status()).toBe(204);
    await expect(page.locator('tr').filter({ hasText: sizeName })).toHaveCount(0);
  });

  test('auth provider flow uses discovered types and performs create/delete (Stage 2.B/2.C)', async ({ page }) => {
    const providerTypesRespPromise = page.waitForResponse((resp) =>
      resp.url().endsWith('/api/v1/admin/auth-provider-types') &&
      resp.request().method() === 'GET'
    );

    await page.goto('/admin/auth-providers');
    await expect(page.getByRole('heading', { name: 'Authentication Providers' })).toBeVisible();

    const providerTypesResp = await providerTypesRespPromise;
    expect(providerTypesResp.status()).toBe(200);
    const providerTypesPayload = (await providerTypesResp.json()) as {
      items?: Array<{ type?: string; display_name?: string }>;
    };
    expect(Array.isArray(providerTypesPayload.items)).toBeTruthy();
    expect((providerTypesPayload.items ?? []).length).toBeGreaterThan(0);
    const preferredProviderType =
      (providerTypesPayload.items ?? []).find((item) => item.type === 'generic') ??
      providerTypesPayload.items?.[0];
    expect(preferredProviderType?.type).toBeTruthy();
    const providerTypeLabel = preferredProviderType?.display_name ?? preferredProviderType?.type ?? '';

    const providerName = `e2e-auth-${Date.now().toString(36).slice(-6)}`;
    const createRespPromise = page.waitForResponse((resp) =>
      resp.url().endsWith('/api/v1/admin/auth-providers') &&
      resp.request().method() === 'POST'
    );

    await page.getByTestId('auth-provider-create-button').click();
    const createModal = page.locator('.ant-modal-content').filter({ hasText: 'Create Authentication Provider' });
    await expect(createModal).toBeVisible();
    await createModal.getByRole('textbox').first().fill(providerName);
    await createModal.locator('.ant-select-selector').first().click();
    await page.locator('.ant-select-item-option').filter({ hasText: providerTypeLabel }).first().click();
    await createModal.getByRole('textbox').nth(1).fill('{"issuer":"https://idp.example.com"}');
    await createModal.getByRole('button', { name: 'OK' }).click();

    const createResp = await createRespPromise;
    expect(createResp.status()).toBe(201);
    const createPayload = (await createResp.json()) as { id?: string };
    expect(createPayload.id).toBeTruthy();
    const providerID = createPayload.id ?? '';

    await expect(page.locator('tr').filter({ hasText: providerName }).first()).toBeVisible();
    await page.getByTestId(`auth-provider-action-delete-${providerID}`).click();

    const deleteRespPromise = page.waitForResponse((resp) =>
      resp.url().includes(`/api/v1/admin/auth-providers/${providerID}`) &&
      resp.request().method() === 'DELETE'
    );
    const deleteModal = page.locator('.ant-modal-content').filter({ hasText: providerName });
    await expect(deleteModal).toBeVisible();
    await deleteModal.getByRole('button', { name: 'OK' }).click();

    const deleteResp = await deleteRespPromise;
    expect(deleteResp.status()).toBe(204);
    await expect(page.locator('tr').filter({ hasText: providerName })).toHaveCount(0);
  });
});
