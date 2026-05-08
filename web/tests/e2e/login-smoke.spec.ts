import { expect, test, type Page } from '@playwright/test';

const e2eBaseURL =
	process.env.PW_BASE_URL ??
	`http://127.0.0.1:${process.env.PW_WEB_PORT ?? '3210'}`;

const mockUser = {
	id: 'user-admin',
	username: 'admin',
	display_name: 'Platform Admin',
	permissions: ['platform:admin'],
	roles: ['PlatformAdmin'],
	force_password_change: false,
};

function jsonResponse(body: unknown, status = 200, headers?: Record<string, string>) {
	return {
		status,
		contentType: 'application/json',
		headers,
		body: JSON.stringify(body),
	};
}

async function mountLoginFlowApi(page: Page) {
	let authenticated = false;
	const authMeStatuses: number[] = [];
	let capturedLoginBody: unknown;
	let capturedSessionMode: string | undefined;

	await page.route('**/api/v1/**', async (route) => {
		const request = route.request();
		const url = new URL(request.url());
		const path = url.pathname;
		const method = request.method();

		if (method === 'GET' && path.endsWith('/auth/me')) {
			const status = authenticated ? 200 : 401;
			authMeStatuses.push(status);
			if (!authenticated) {
				return route.fulfill(jsonResponse({ code: 'UNAUTHORIZED' }, status));
			}
			return route.fulfill(jsonResponse(mockUser));
		}

		if (method === 'GET' && path.endsWith('/auth/providers')) {
			return route.fulfill(jsonResponse({ items: [] }));
		}

		if (method === 'POST' && path.endsWith('/auth/login')) {
			capturedLoginBody = request.postDataJSON();
			capturedSessionMode = request.headers()['x-shepherd-session-mode'];
			authenticated = true;
			return route.fulfill(jsonResponse(
				{
					token: 'redacted',
					expires_at: '2026-01-01T00:00:00Z',
					force_password_change: false,
				},
				200,
				{
					'set-cookie': 'shepherd_session=mock-session; Path=/; SameSite=Lax',
				},
			));
		}

		if (method === 'GET' && path.endsWith('/notifications/unread-count')) {
			return route.fulfill(jsonResponse({ count: 0 }));
		}

		if (method === 'GET' && path.endsWith('/notifications')) {
			return route.fulfill(jsonResponse({
				items: [],
				pagination: { page: 1, per_page: 10, total: 0, total_pages: 0 },
			}));
		}

		if (method === 'GET' && path.endsWith('/health/ready')) {
			return route.fulfill(jsonResponse({ status: 'ok', version: 'test' }));
		}

		if (method === 'GET' && path.endsWith('/health/live')) {
			return route.fulfill(jsonResponse({ status: 'ok' }));
		}

		if (method === 'GET' && path.endsWith('/systems')) {
			return route.fulfill(jsonResponse({
				items: [
					{
						id: 'sys-1',
						name: 'shop',
						description: 'Core shop system',
						created_by_display_name: 'Platform Admin',
						created_at: '2026-05-08T00:00:00Z',
					},
				],
				pagination: { page: 1, per_page: 4, total: 1, total_pages: 1 },
			}));
		}

		if (method === 'GET' && /\/systems\/[^/]+\/services$/.test(path)) {
			return route.fulfill(jsonResponse({
				items: [
					{ id: 'svc-1', system_id: 'sys-1', name: 'redis' },
				],
				pagination: { page: 1, per_page: 3, total: 1, total_pages: 1 },
			}));
		}

		if (method === 'GET' && path.endsWith('/services')) {
			return route.fulfill(jsonResponse({
				items: [
					{
						id: 'svc-1',
						system_id: 'sys-1',
						system_name: 'shop',
						name: 'redis',
						description: 'Cache service',
						next_instance_index: 1,
					},
				],
				pagination: { page: 1, per_page: 4, total: 1, total_pages: 1 },
			}));
		}

		if (method === 'GET' && path.endsWith('/vms')) {
			return route.fulfill(jsonResponse({
				items: [],
				pagination: { page: 1, per_page: 4, total: 0, total_pages: 0 },
			}));
		}

		if (method === 'GET' && path.endsWith('/tickets')) {
			const isPending = url.searchParams.get('status') === 'PENDING';
			return route.fulfill(jsonResponse({
				items: isPending ? [] : [
					{
						id: 'ticket-1',
						requester: 'user-admin',
						requester_display_name: 'Platform Admin',
						requester_username: 'admin',
						status: 'APPROVED',
						operation_type: 'CREATE',
						summary: { system_name: 'shop', service_name: 'redis' },
						created_at: '2026-05-08T00:00:00Z',
					},
				],
				pagination: {
					page: 1,
					per_page: isPending ? 1 : 4,
					total: isPending ? 0 : 1,
					total_pages: isPending ? 0 : 1,
				},
			}));
		}

		return route.fulfill(jsonResponse({}));
	});

	return {
		authMeStatuses,
		getCapturedLoginBody: () => capturedLoginBody,
		getCapturedSessionMode: () => capturedSessionMode,
	};
}

test.describe('login page', () => {
	test('renders credential form', async ({ page }) => {
		await mountLoginFlowApi(page);
		await page.goto('/login');

		await expect(page.getByRole('heading', { name: 'KubeVirt Shepherd' })).toBeVisible();
		await expect(page.getByPlaceholder('Username')).toBeVisible();
		await expect(page.getByPlaceholder('Password')).toBeVisible();
		await expect(page.getByRole('button', { name: 'Login' })).toBeVisible();
	});

	test('logs in with cookie-backed session and reaches protected dashboard API flow', async ({ page }) => {
		const apiState = await mountLoginFlowApi(page);

		await page.goto('/login');
		await expect(page.getByRole('button', { name: 'Login' })).toBeVisible();

		await page.getByPlaceholder('Username').fill('admin');
		await page.getByPlaceholder('Password').fill('securepass123');

		const loginResponse = page.waitForResponse((response) =>
			response.url().endsWith('/api/v1/auth/login') &&
			response.request().method() === 'POST' &&
			response.status() === 200,
		);
		const restoredUserResponse = page.waitForResponse((response) =>
			response.url().endsWith('/api/v1/auth/me') &&
			response.request().method() === 'GET' &&
			response.status() === 200,
		);

		await page.getByRole('button', { name: 'Login' }).click();
		await loginResponse;
		await restoredUserResponse;
		await page.waitForURL('**/dashboard');

		await expect(page.getByRole('heading', { name: 'Dashboard' })).toBeVisible();
		await expect(page.getByText('System overview and statistics')).toBeVisible();
		await expect(page.getByText('Core shop system')).toBeVisible();

		const cookies = await page.context().cookies(e2eBaseURL);
		expect(cookies.some((cookie) => cookie.name === 'shepherd_session' && cookie.value === 'mock-session')).toBe(true);
		expect(apiState.authMeStatuses).toContain(401);
		expect(apiState.authMeStatuses).toContain(200);
		expect(apiState.getCapturedLoginBody()).toEqual({
			username: 'admin',
			password: 'securepass123',
		});
		expect(apiState.getCapturedSessionMode()).toBe('cookie_only');
	});
});
