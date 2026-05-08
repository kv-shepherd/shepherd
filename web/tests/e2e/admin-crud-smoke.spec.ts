import { expect, test, type Page } from '@playwright/test';

import { getAntModal } from './lib/helpers';

const e2eBaseURL =
	process.env.PW_BASE_URL ??
	`http://127.0.0.1:${process.env.PW_WEB_PORT ?? '3210'}`;

const mockAuthUser = {
	id: 'user-admin',
	username: 'admin',
	display_name: 'Platform Admin',
	permissions: ['platform:admin'],
	roles: ['PlatformAdmin'],
};

type CapturedCall = {
	method: string;
	path: string;
	body: unknown;
};

function jsonBody(body: unknown) {
	return JSON.stringify(body);
}

function visibleModal(page: Page) {
	return page.locator('.ant-modal-content:visible');
}

async function fillTemplateImageURL(modal: ReturnType<typeof getAntModal>) {
	const imageURL = 'docker://quay.io/containerdisks/ubuntu:22.04';
	const imageField = modal.locator('.ant-form-item').filter({ hasText: /^Image URL/ }).last();
	const customButton = imageField.getByRole('button', { name: 'Custom' });
	if (await customButton.isVisible().catch(() => false)) {
		await customButton.click();
	}
	const input = imageField.locator(`input[placeholder="${imageURL}"]`).filter({ visible: true }).last();
	await expect(input).toBeVisible();
	await input.fill(imageURL);
}

async function injectAuth(page: Page) {
	await page.context().addCookies([
		{
			name: 'shepherd_session',
			value: 'test-session',
			url: e2eBaseURL,
			httpOnly: true,
			sameSite: 'Lax',
		},
	]);
}

async function mountAdminCrudApi(page: Page, calls: CapturedCall[]) {
	const now = '2026-05-08T00:00:00Z';
	const templates = [
		{
			id: 'tpl-1',
			name: 'ubuntu-24',
			display_name: 'Ubuntu 24',
			description: 'Ubuntu base image',
			catalog_scope: 'test',
			source_type: 'cdi_image_import',
			image_url: 'docker://quay.io/containerdisks/ubuntu:22.04',
			os_family: 'linux',
			os_version: 'Ubuntu 24.04',
			cloud_init: '#cloud-config\nhostname: ubuntu-24',
			enabled: true,
			created_at: now,
			updated_at: now,
		},
	];
	const instanceSizes = [
		{
			id: 'size-1',
			name: 'small',
			display_name: 'Small',
			description: 'Small general purpose profile',
			catalog_scope: 'test',
			cpu_cores: 2,
			memory_gi: 4,
			disk_gb: 40,
			sort_order: 10,
			spec_overrides: {},
			enabled: true,
			created_at: now,
			updated_at: now,
		},
	];
	const externalApprovalSystems = [
		{
			id: 'external-approval-1',
			name: 'Example Approval',
			type: 'webhook',
			provider_type: 'webhook',
			enabled: true,
			webhook_url: 'https://approval.example.com/webhook',
			webhook_headers: { 'X-Shepherd-Source': 'shepherd' },
			timeout_seconds: 30,
			retry_count: 3,
			retry_backoff_seconds: 2,
			signing_key_set: true,
			sort_order: 0,
			created_at: now,
			updated_at: now,
		},
	];

	await page.route('**/api/v1/**', async (route) => {
		const request = route.request();
		const url = new URL(request.url());
		const path = url.pathname;
		const method = request.method();
		let body: unknown;
		try {
			body = request.postDataJSON();
		} catch {
			body = request.postData();
		}
		calls.push({ method, path, body });

		const json = (data: unknown, status = 200) =>
			route.fulfill({
				status,
				contentType: 'application/json',
				body: jsonBody(data),
			});

		if (method === 'GET' && path.endsWith('/auth/me')) {
			return json(mockAuthUser);
		}
		if (method === 'GET' && path.endsWith('/notifications/unread-count')) {
			return json({ count: 0 });
		}
		if (method === 'GET' && path.endsWith('/notifications')) {
			return json({ items: [], pagination: { page: 1, per_page: 10, total: 0, total_pages: 0 } });
		}
		if (method === 'GET' && path.endsWith('/schemas/instancesize')) {
			return json({
				schema: { type: 'object', properties: {} },
				mask: [],
				schema_version: 'test',
				source: 'embedded',
				degraded: false,
				fetched_at: now,
			});
		}

		if (path.endsWith('/admin/templates')) {
			if (method === 'GET') {
				return json({ items: templates, pagination: { page: 1, per_page: 20, total: templates.length, total_pages: 1 } });
			}
			if (method === 'POST') {
				const next = { ...templates[0], id: 'tpl-new', name: 'e2e-template', display_name: 'E2E Template' };
				templates.push(next);
				return json(next, 201);
			}
		}
		if (/\/admin\/templates\/[^/]+$/.test(path)) {
			if (method === 'PATCH') {
				return json({ ...templates[0], display_name: 'Ubuntu 24 LTS' });
			}
			if (method === 'DELETE') {
				return route.fulfill({ status: 204, body: '' });
			}
		}

		if (path.endsWith('/admin/instance-sizes')) {
			if (method === 'GET') {
				return json({ items: instanceSizes, pagination: { page: 1, per_page: 20, total: instanceSizes.length, total_pages: 1 } });
			}
			if (method === 'POST') {
				const next = { ...instanceSizes[0], id: 'size-new', name: 'e2e-small', display_name: 'E2E Small' };
				instanceSizes.push(next);
				return json(next, 201);
			}
		}
		if (/\/admin\/instance-sizes\/[^/]+$/.test(path)) {
			if (method === 'PATCH') {
				return json({ ...instanceSizes[0], memory_gi: 6 });
			}
			if (method === 'DELETE') {
				return route.fulfill({ status: 204, body: '' });
			}
		}

		if (path.endsWith('/admin/external-approval-systems')) {
			if (method === 'GET') {
				return json({ items: externalApprovalSystems, pagination: { page: 1, per_page: 20, total: externalApprovalSystems.length, total_pages: 1 } });
			}
			if (method === 'POST') {
				const next = {
					...externalApprovalSystems[0],
					id: 'external-approval-new',
					name: 'Example Approval New',
				};
				externalApprovalSystems.push(next);
				return json(next, 201);
			}
		}
		if (/\/admin\/external-approval-systems\/[^/]+$/.test(path)) {
			if (method === 'PATCH') {
				return json({ ...externalApprovalSystems[0], name: 'Example Approval Updated' });
			}
			if (method === 'DELETE') {
				return route.fulfill({ status: 204, body: '' });
			}
		}

		return json({});
	});
}

test.describe('admin CRUD mock smoke interactions', () => {
	let calls: CapturedCall[];

	test.beforeEach(async ({ page }) => {
		calls = [];
		await injectAuth(page);
		await mountAdminCrudApi(page, calls);
	});

	test('Stage 3 - template catalog supports create, edit, and delete from UI', async ({ page }) => {
		await page.goto('/admin/templates');
		await expect(page.getByRole('heading', { name: 'Templates' })).toBeVisible();
		await expect(page.locator('tr').filter({ hasText: 'ubuntu-24' }).first()).toBeVisible();

		await page.getByTestId('template-create-button').click();
		const createModal = getAntModal(page, 'template-create-modal');
		await expect(createModal).toBeVisible();
		await createModal.getByPlaceholder('centos7-standard').fill('e2e-template');
		await fillTemplateImageURL(createModal);
		await createModal.getByRole('button', { name: /ok|create|save|submit/i }).click();

		await expect.poll(() =>
			calls.some((call) => call.method === 'POST' && call.path.endsWith('/admin/templates')),
		).toBeTruthy();

		await page.getByTestId('template-action-edit-tpl-1').click();
		const editModal = getAntModal(page, 'template-edit-modal');
		await expect(editModal).toBeVisible();
		await editModal.getByLabel(/display name/i).fill('Ubuntu 24 LTS');
		await fillTemplateImageURL(editModal);
		await editModal.getByRole('button', { name: /ok|create|save|submit/i }).click();

		await expect.poll(() =>
			calls.some((call) => call.method === 'PATCH' && /\/admin\/templates\/tpl-1$/.test(call.path)),
		).toBeTruthy();

		await page.getByTestId('template-action-delete-tpl-1').click();
		const deleteModal = visibleModal(page);
		await expect(deleteModal).toBeVisible();
		await deleteModal.getByRole('button', { name: /ok|delete/i }).click();

		await expect.poll(() =>
			calls.some((call) => call.method === 'DELETE' && /\/admin\/templates\/tpl-1$/.test(call.path)),
		).toBeTruthy();
	});

	test('Stage 3 - instance size catalog supports create, edit, and delete from UI', async ({ page }) => {
		await page.goto('/admin/instance-sizes');
		await expect(page.getByRole('heading', { name: 'Instance Sizes' })).toBeVisible();
		await expect(page.locator('tr').filter({ hasText: 'small' }).first()).toBeVisible();

		await page.getByTestId('instance-size-create-button').click();
		const createModal = getAntModal(page, 'instance-size-create-modal');
		await expect(createModal).toBeVisible();
		await createModal.getByRole('textbox').first().fill('e2e-small');
		await createModal.locator('#cpu_cores').fill('4');
		await createModal.locator('#memory_gi').fill('8');
		await createModal.getByRole('button', { name: /ok|create|save|submit/i }).click();

		await expect.poll(() =>
			calls.some((call) => call.method === 'POST' && call.path.endsWith('/admin/instance-sizes')),
		).toBeTruthy();

		await page.getByTestId('instance-size-action-edit-size-1').click();
		const editModal = getAntModal(page, 'instance-size-edit-modal');
		await expect(editModal).toBeVisible();
		await editModal.locator('#memory_gi').fill('6');
		await editModal.getByRole('button', { name: /ok|create|save|submit/i }).click();

		await expect.poll(() =>
			calls.some((call) => call.method === 'PATCH' && /\/admin\/instance-sizes\/size-1$/.test(call.path)),
		).toBeTruthy();

		await page.getByTestId('instance-size-action-delete-size-1').click();
		const deleteModal = visibleModal(page);
		await expect(deleteModal).toBeVisible();
		await deleteModal.getByRole('button', { name: /ok|delete/i }).click();

		await expect.poll(() =>
			calls.some((call) => call.method === 'DELETE' && /\/admin\/instance-sizes\/size-1$/.test(call.path)),
		).toBeTruthy();
	});

	test('Stage 2.E - external approval systems support create, edit, and delete from UI', async ({ page }) => {
		await page.goto('/admin/external-approval-systems');
		await expect(page.getByTestId('external-approval-systems-page')).toBeVisible();
		await expect(page.locator('tr').filter({ hasText: 'Example Approval' }).first()).toBeVisible();

		await page.getByTestId('external-approval-system-create-button').click();
		const createModal = getAntModal(page, 'external-approval-system-create-modal');
		await expect(createModal).toBeVisible();
		await createModal.getByLabel(/^Name$/i).fill('Example Approval New');
		await createModal.getByLabel(/webhook url/i).fill('https://approval.example.com/new-webhook');
		await createModal.getByLabel(/signing key/i).fill('example-signing-material');
		await createModal.getByRole('button', { name: /ok|create|save|submit/i }).click();

		await expect.poll(() =>
			calls.some((call) => call.method === 'POST' && call.path.endsWith('/admin/external-approval-systems')),
		).toBeTruthy();

		await page.getByTestId('external-approval-system-action-edit-external-approval-1').click();
		const editModal = getAntModal(page, 'external-approval-system-edit-modal');
		await expect(editModal).toBeVisible();
		await editModal.getByLabel(/^Name$/i).fill('Example Approval Updated');
		await editModal.getByRole('button', { name: /ok|create|save|submit/i }).click();

		await expect.poll(() =>
			calls.some((call) =>
				call.method === 'PATCH' &&
				/\/admin\/external-approval-systems\/external-approval-1$/.test(call.path),
			),
		).toBeTruthy();

		await page.getByTestId('external-approval-system-action-delete-external-approval-1').click();
		const popconfirm = page.locator('.ant-popconfirm:visible');
		await expect(popconfirm).toBeVisible();
		await popconfirm.getByRole('button', { name: /delete|ok|confirm/i }).click();

		await expect.poll(() =>
			calls.some((call) =>
				call.method === 'DELETE' &&
				/\/admin\/external-approval-systems\/external-approval-1$/.test(call.path),
			),
		).toBeTruthy();
	});
});
