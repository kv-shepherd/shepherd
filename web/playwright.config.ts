import { defineConfig, devices } from '@playwright/test';

const e2eRunId = process.env.PW_E2E_RUN_ID ?? 'local';
const defaultWebPort = 3210;
const webPort = Number(process.env.PW_WEB_PORT ?? defaultWebPort);
const baseURL = process.env.PW_BASE_URL ?? `http://127.0.0.1:${webPort}`;
const baseHostname = new URL(baseURL).hostname;
const devAllowedOrigins = Array.from(
	new Set(
		(process.env.DEV_ALLOWED_ORIGINS ?? '')
			.split(',')
			.map((origin) => origin.trim())
			.filter(Boolean)
			.concat(baseHostname),
	),
).join(',');
const e2eDistDir = `.next-e2e/${e2eRunId}`;
const e2eCacheDir = `${e2eDistDir}/cache`;
const e2eTsconfigPath = `tsconfig.e2e.${e2eRunId}.json`;
const e2eTsconfigBackupPath = `tsconfig.e2e.${e2eRunId}.backup.json`;

// Live E2E tests run against a real backend.
// Set LIVE_E2E=true to include them in the test run.
// In CI, they are controlled by the separate `test:e2e:live` script.
const isLive = !!process.env.LIVE_E2E;

export default defineConfig({
	testDir: './tests/e2e',
	fullyParallel: true,
	// forbidOnly is always on: `.only` must never be merged to any branch.
	forbidOnly: true,
	// Per Playwright docs, CI should fail if a test only passes after retry.
	failOnFlakyTests: !!process.env.CI,
	// Global retries: smoke uses 2 in CI; live overrides to 1 at project level
	// (live tests are stateful – excessive retries cause duplicate side-effects)
	retries: process.env.CI ? 2 : 0,
	workers: process.env.CI ? 1 : undefined,
	reporter: process.env.CI ? [['github'], ['html', { open: 'never' }]] : 'list',
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
			retries: process.env.CI ? 1 : 0,
			// Per-test timeout: 90 s is enough for all CRUD flows + schema validation.
			// waitForResponse() inherits this; the previous default of 30 s was too
			// short for slow CI environments.
			timeout: 90_000,
			use: {
				...devices['Desktop Chrome'],
				// Give real backend more time for action interactions (clicks, fills)
				actionTimeout: isLive || process.env.CI ? 20_000 : 15_000,
				// Navigation to pages that trigger API calls on mount needs more time
				navigationTimeout: isLive || process.env.CI ? 45_000 : 30_000,
			},
		},
	],
	webServer: {
		name: 'Next.js (dev)',
		// Always run a dedicated production server for E2E. Smoke tests do not need
		// HMR or file watching, so building once and serving with `next start`
		// avoids dev-server watcher pressure and keeps local/remote behavior closer.
		// Force a dedicated port, dist dir, and copied tsconfig so E2E never
		// contends with an already-running local Next instance or mutates the
		// repository tsconfig during `next build`.
		// When launched via run_e2e_live.sh, the process stdout is already captured
		// by the shell (nohup redirect in background mode, or tee in foreground).
		command: `sh -c 'trap "if [ -f ${e2eTsconfigBackupPath} ]; then mv ${e2eTsconfigBackupPath} tsconfig.json; fi; rm -f ${e2eTsconfigPath}" EXIT && mkdir -p ${e2eCacheDir} && find ${e2eDistDir} -mindepth 1 -maxdepth 1 ! -name cache -exec rm -rf {} + && rm -f ${e2eTsconfigPath} ${e2eTsconfigBackupPath} && cp tsconfig.json ${e2eTsconfigBackupPath} && cp tsconfig.json ${e2eTsconfigPath} && DEV_ALLOWED_ORIGINS=${devAllowedOrigins} NEXT_DIST_DIR=${e2eDistDir} NEXT_TSCONFIG_PATH=${e2eTsconfigPath} npx next build --webpack && DEV_ALLOWED_ORIGINS=${devAllowedOrigins} NEXT_DIST_DIR=${e2eDistDir} NEXT_TSCONFIG_PATH=${e2eTsconfigPath} npx next start --port ${webPort}'`,
		url: baseURL,
		// MUST be false unconditionally: reusing an existing server risks pointing at
		// a stale process bound to a different backend URL or a different database.
		// Playwright docs: "if false, Playwright throws an error if it detects
		// an existing process on the port" — this is the safe, isolated behaviour.
		reuseExistingServer: false,
		// Pipe both streams so that they flow through to the parent shell process,
		// which is responsible for capturing logs (nohup / tee in run_e2e_live.sh).
		stdout: 'pipe',
		stderr: 'pipe',
		// Account for a full webpack production build before `next start`.
		timeout: 300_000,
	},
});
