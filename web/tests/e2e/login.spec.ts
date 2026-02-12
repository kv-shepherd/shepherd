import { expect, test } from '@playwright/test';

test.describe('login page', () => {
	test('renders credential form', async ({ page }) => {
		await page.goto('/login');

		await expect(page.getByRole('heading', { name: 'KubeVirt Shepherd' })).toBeVisible();
		await expect(page.getByPlaceholder('Username')).toBeVisible();
		await expect(page.getByPlaceholder('Password')).toBeVisible();
		await expect(page.getByRole('button', { name: 'Login' })).toBeVisible();
	});
});

