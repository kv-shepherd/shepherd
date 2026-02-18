import { defineConfig, devices } from '@playwright/test';

const isCI = !!process.env.CI;
const webPort = Number(process.env.PW_WEB_PORT ?? 3000);
const baseURL = process.env.PW_BASE_URL ?? `http://127.0.0.1:${webPort}`;

export default defineConfig({
	testDir: './tests/e2e',
	fullyParallel: true,
	forbidOnly: isCI,
	retries: isCI ? 2 : 0,
	workers: isCI ? 1 : undefined,
	reporter: isCI ? [['github'], ['html', { open: 'never' }]] : 'list',
	use: {
		baseURL,
		trace: 'on-first-retry',
		screenshot: 'only-on-failure',
	},
	projects: [
		{
			name: 'chromium',
			use: { ...devices['Desktop Chrome'] },
		},
	],
	webServer: {
		command: isCI ? `npm run build && npm run start -- --port ${webPort}` : `npm run dev -- --port ${webPort}`,
		url: baseURL,
		reuseExistingServer: !isCI,
		timeout: 180_000,
	},
});
