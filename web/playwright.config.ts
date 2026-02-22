import { defineConfig, devices } from '@playwright/test';

const isCI = !!process.env.CI;
const webPort = Number(process.env.PW_WEB_PORT ?? 3000);
const baseURL = process.env.PW_BASE_URL ?? `http://127.0.0.1:${webPort}`;

// Live E2E tests run against a real backend.
// Set LIVE_E2E=true to include them in the test run.
// In CI, they are controlled by the separate `test:e2e:live` script.
const isLive = !!process.env.LIVE_E2E;

export default defineConfig({
	testDir: './tests/e2e',
	fullyParallel: true,
	forbidOnly: isCI,
	// Global retries: smoke uses 2 in CI; live overrides to 1 at project level
	// (live tests are stateful – excessive retries cause duplicate side-effects)
	retries: isCI ? 2 : 0,
	workers: isCI ? 1 : undefined,
	reporter: isCI ? [['github'], ['html', { open: 'never' }]] : 'list',
	use: {
		baseURL,
		trace: 'on-first-retry',
		screenshot: 'only-on-failure',
		// Base timeouts (smoke). Live tests override these at project level.
		actionTimeout: 10_000,
		navigationTimeout: 15_000,
	},
	projects: [
		// ── Smoke (mock) tests – always run, no backend required ──────────────
		{
			name: 'smoke-chromium',
			testMatch: /.*-smoke\.spec\.ts/,
			use: { ...devices['Desktop Chrome'] },
		},
		// ── Live (contract-enforced) tests – require a running backend ─────────
		// Per Playwright docs: page.waitForResponse() is controlled by the
		// per-test `timeout` (not actionTimeout). Live tests wait on real
		// backend I/O, so we raise both the per-test timeout and action/nav
		// timeouts here at project scope.
		{
			name: 'live-chromium',
			testMatch: /.*-live\.spec\.ts/,
			// Live tests: only 1 retry — they are stateful (create/delete records)
			// and re-running a failed test may hit duplicate-key errors.
			retries: isCI ? 1 : 0,
			// Per-test timeout: 90 s is enough for all CRUD flows + schema validation.
			// waitForResponse() inherits this; the previous default of 30 s was too
			// short for slow CI environments.
			timeout: 90_000,
			use: {
				...devices['Desktop Chrome'],
				// Give real backend more time for action interactions (clicks, fills)
				actionTimeout: isLive || isCI ? 20_000 : 15_000,
				// Navigation to pages that trigger API calls on mount needs more time
				navigationTimeout: isLive || isCI ? 45_000 : 30_000,
			},
		},
	],
	webServer: {
		command: isCI
			? `npm run build && npm run start -- --port ${webPort}`
			: `npm run dev -- --port ${webPort}`,
		url: baseURL,
		reuseExistingServer: !isCI,
		// 180 s was enough for Next.js dev; prod build may need more — bumped to 300 s
		timeout: 300_000,
	},
});
