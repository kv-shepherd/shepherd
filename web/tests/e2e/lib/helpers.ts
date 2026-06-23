/**
 * E2E Test Helpers — Ant Design + URL matching utilities
 *
 * ── Problem ──────────────────────────────────────────────────────────────────
 * 1. `url.endsWith('/api/v1/admin/users')` fails when query params are present
 *    (e.g. `/api/v1/admin/users?page=1&per_page=20`).
 * 2. Ant Design Select dropdowns are prone to race conditions where the
 *    dropdown opens and then immediately closes due to React re-renders.
 *
 * ── Solution (per Playwright best practices) ──────────────────────────────────
 * 1. `urlPathEndsWith(url, path)` — strips query string before matching.
 * 2. `urlPathIncludes(url, path)` — strips query string before matching.
 * 3. `selectAntOption(page, combobox, optionFilter?)` — stable Select interaction
 *    using web-first assertions (`expect().toBeVisible()`) before clicking.
 *
 * Reference: https://playwright.dev/docs/best-practices
 */

import { expect, type APIRequestContext, type APIResponse, type Locator, type Page, type Response } from '@playwright/test';
import { validateApiResponse, validateResponse } from './schema-validator';

// ── URL Path Matching (query-param safe) ─────────────────────────────────────

/**
 * Check if the pathname portion of `url` ends with `path`.
 * Unlike `url.endsWith(path)`, this is safe for URLs with query strings.
 *
 * @example
 *   urlPathEndsWith('http://localhost/api/v1/admin/users?page=1', '/api/v1/admin/users')
 *   // → true (endsWith would return false!)
 */
export function urlPathEndsWith(url: string, path: string): boolean {
    try {
        const { pathname } = new URL(url);
        return pathname.endsWith(path);
    } catch {
        // Fallback for non-absolute URLs: strip query string manually
        const pathPart = url.split('?')[0];
        return pathPart.endsWith(path);
    }
}

/**
 * Check if the pathname portion of `url` includes `path`.
 * Unlike `url.includes(path)`, this does NOT match query-string content.
 *
 * @example
 *   urlPathIncludes('http://localhost/api/v1/systems/123/members', '/systems/')
 *   // → true
 */
export function urlPathIncludes(url: string, path: string): boolean {
    try {
        const { pathname } = new URL(url);
        return pathname.includes(path);
    } catch {
        const pathPart = url.split('?')[0];
        return pathPart.includes(path);
    }
}

/**
 * Safely select an option from an Ant Design Select (combobox) component.
 *
 * Per Playwright best practices (Context7: /microsoft/playwright.dev):
 * 1. Click the combobox to open the dropdown
 * 2. Wait for any OLD dropdown leave-animations to finish (prevents strict mode violation)
 * 3. Wait for the ACTIVE dropdown to be visible (web-first assertion)
 * 4. Wait for the first matching option to be visible and stable
 * 5. Click the option
 *
 * Problem solved:
 * Ant Design's Select dropdown uses CSS animation classes:
 *   - Opening: `.ant-slide-up-appear` / `.ant-slide-up-appear-prepare`
 *   - Closing: `.ant-slide-up-leave` / `.ant-slide-up-leave-active`
 * During the leave animation, the old dropdown is still `:visible` in the DOM.
 * If a new dropdown opens before the old one finishes closing,
 * `page.locator('.ant-select-dropdown:visible')` matches 2 elements → strict mode violation.
 *
 * Fix (based on Context7 best practices):
 *   - Use `.filter({ visible: true })` (Playwright recommended) instead of CSS `:visible`
 *   - Exclude dropdowns with leave-animation classes using `:not()` CSS pseudo-class
 *   - Use `.last()` to always target the most recently opened dropdown
 *
 * @param page - Playwright Page instance
 * @param combobox - Locator for the combobox trigger element
 * @param optionFilter - Optional: filter options by text content (regex or string)
 * @param timeout - Max time to wait for option visibility (default: 15000ms)
 */
export async function selectAntOption(
    page: Page,
    combobox: Locator,
    optionFilter?: string | RegExp,
    timeout = 15_000,
    options?: { keepOpen?: boolean; reuseOpenDropdown?: boolean }
): Promise<void> {
    const trigger = combobox.first();
    const selectRoot = trigger
        .locator('xpath=ancestor-or-self::*[contains(concat(" ", normalize-space(@class), " "), " ant-select ")][1]')
        .first();
    const isMultipleSelect = await selectRoot
        .evaluate((element) => element.classList.contains('ant-select-multiple'))
        .catch(() => false);
    await expect(trigger).toBeVisible({ timeout });
    await expect(selectRoot).not.toHaveClass(/ant-select-disabled/, { timeout });

    // Step 1: Wait for any OLD dropdown leave-animations to finish.
    // Ant Design adds `.ant-slide-up-leave` during close animation.
    // We must wait for these to disappear before locating the active dropdown,
    // otherwise `.ant-select-dropdown:visible` may resolve to 2 elements.
    await expect(
        page.locator('.ant-select-dropdown.ant-slide-up-leave')
    ).toHaveCount(0, { timeout });

    // Step 2: Locate any currently active dropdown before clicking.
    const activeDropdowns = page
        .locator('.ant-select-dropdown:not(.ant-slide-up-leave):not(.ant-slide-up-leave-active)')
        .filter({ visible: true });

    let dropdown = activeDropdowns.last();
    const shouldReuseOpenDropdown = !!options?.reuseOpenDropdown;
    if (!shouldReuseOpenDropdown || await activeDropdowns.count() === 0) {
        // Default to opening the target select explicitly. Only reuse an
        // already-open dropdown when the caller intentionally keeps it open
        // across consecutive selections (for example Ant multi-select).
        await trigger.click();
        await expect(
            page.locator('.ant-select-dropdown.ant-slide-up-leave')
        ).toHaveCount(0, { timeout });
        dropdown = activeDropdowns.last();
    }

    await expect(dropdown).toBeVisible({ timeout });

    // Step 3: If the Select supports showSearch and a string filter is given,
    // type the filter text into the search input. This narrows the virtual list
    // to only matching options, preventing items outside the viewport from being
    // invisible in the DOM. We use .pressSequentially() for realistic key-by-key
    // input that triggers Ant's onSearch handler.
    if (typeof optionFilter === 'string' && optionFilter.length > 0) {
        const searchInput = selectRoot.locator('input[type="search"]').first();
        if (await searchInput.count() > 0 && await searchInput.isVisible().catch(() => false)) {
            await searchInput.focus();
            await page.keyboard.press(process.platform === 'darwin' ? 'Meta+A' : 'Control+A');
            await page.keyboard.press('Delete');
            await searchInput.pressSequentially(optionFilter, { delay: 30 });
            await expect
                .poll(
                    async () =>
                        dropdown
                            .locator('[role="option"], .ant-select-item-option')
                            .filter({ visible: true })
                            .count(),
                    { timeout },
                )
                .toBeGreaterThan(0);
        }
    }

    const optionCandidates = dropdown.locator('[role="option"], .ant-select-item-option');
    const visibleOptions = optionCandidates.filter({ visible: true });

    // Step 4: Wait for options to be ready. Async option loading is common in
    // Ant Design Select, and dependent fields may briefly show an empty list or
    // "No data" before the backing request finishes. Only treat "no data" as a
    // failure if the list never becomes ready within the timeout window.
    try {
        await expect
            .poll(async () => visibleOptions.count(), { timeout })
            .toBeGreaterThan(0);
    } catch (error) {
        const noDataVisible = await dropdown
            .getByText(/^(no data|not found|no options)$/i)
            .first()
            .isVisible()
            .catch(() => false);
        if (noDataVisible) {
            throw new Error(
                'Ant Select has no selectable options ("No data"/"Not Found"). ' +
                'For approval flow this usually means no HEALTHY+enabled cluster is available.'
            );
        }
        throw error;
    }

    // Step 5: Find the target option within the active dropdown
    let option: Locator;
    if (optionFilter) {
        option = visibleOptions.filter({ hasText: optionFilter }).first();
    } else {
        option = visibleOptions.first();
    }

    // Step 6: Wait for the target option itself to exist. Ant Select dropdowns
    // can momentarily render an empty list during async refreshes or after a
    // previous search term was cleared, so checking only the dropdown readiness
    // is not stable enough.
    await expect
        .poll(async () => option.count(), { timeout })
        .toBeGreaterThan(0);

    const matchCount = await option.count();
    if (matchCount === 0) {
        const available = (await visibleOptions.allTextContents())
            .map((text) => text.trim())
            .filter(Boolean)
            .join(' | ');
        throw new Error(
            `Ant Select option not found for filter ${String(optionFilter)}. ` +
            `Available options: ${available || '(none)'}`
        );
    }

    await expect(option).toBeVisible({ timeout });
    await option.click();

    // Multi-select dropdowns may remain open after selecting one item and can block modal buttons.
    // Press Escape once to close the dropdown explicitly when it is still visible.
    if (!options?.keepOpen && !isMultipleSelect && await dropdown.isVisible().catch(() => false)) {
        await page.keyboard.press('Escape');
    }
}

export async function selectServicesSystemFilter(
    page: Page,
    systemName: string,
    timeout = 15_000
): Promise<Response> {
    const systemSelector = page.getByTestId('services-system-selector');
    if (!await systemSelector.isVisible().catch(() => false)) {
        const advancedSearchToggle = page.getByTestId('services-advanced-search-toggle');
        await expect(advancedSearchToggle).toBeVisible({ timeout });
        await advancedSearchToggle.click();
    }

    await expect(systemSelector).toBeVisible({ timeout });
    await selectAntOption(page, systemSelector, systemName, timeout);

    const servicesResponsePromise = page.waitForResponse((response) => {
        if (response.request().method() !== 'GET') {
            return false;
        }
        try {
            return /\/api\/v1\/systems\/[^/]+\/services$/.test(new URL(response.url()).pathname);
        } catch {
            return false;
        }
    }, { timeout });

    await page.getByTestId('services-advanced-search-submit').click();
    return servicesResponsePromise;
}

// ── Ant Design Modal Locator Helper ──────────────────────────────────────────

/**
 * Get a stable Playwright locator for an Ant Design Modal identified by data-testid.
 *
 * ── Problem ────────────────────────────────────────────────────────────────
 * Ant Design Modal's DOM structure (from rc-dialog source):
 *
 *   <div class="ant-modal-root" data-testid="xxx">     ← getByTestId lands HERE
 *     <div class="ant-modal-mask" />                    ← backdrop overlay
 *     <div class="ant-modal-wrap" style="display:...">  ← VISIBILITY CONTROLLED HERE
 *       <div class="ant-modal" role="dialog">           ← actual dialog content
 *         <div class="ant-modal-content">
 *           <div class="ant-modal-body"> ... </div>
 *           <div class="ant-modal-footer"> OK | Cancel </div>
 *   ...
 *
 * The `.ant-modal-root` is a zero-height wrapper div. Even when the Modal is
 * open, its bounding box may read as empty → Playwright `toBeVisible()` returns
 * false ("hidden").
 *
 * The actual visibility is controlled by `.ant-modal-wrap`'s inline style:
 *   - Modal closed: `display: none`   (rc-dialog: `!animatedVisible ? 'none' : null`)
 *   - Modal open:   `display: null`   → block (default)
 *
 * ── Solution (per Playwright best practices via Context7) ──────────────────
 * Locate the `.ant-modal-wrap` *inside* the testid root. This element:
 *   1. Has a non-empty bounding box when visible (full-screen overlay)
 *   2. Correctly responds to toBeVisible() / toBeHidden()
 *   3. Contains the `role="dialog"` element with all buttons (OK, Cancel)
 *
 * @param page     Playwright Page instance
 * @param testId   The data-testid value set on the <Modal> component
 * @returns        Locator pointing to .ant-modal-wrap (visible when modal is open)
 *
 * @example
 *   const modal = getAntModal(page, 'rbac-role-create-modal');
 *   await expect(modal).toBeVisible();
 *   await modal.getByRole('textbox').first().fill('admin');
 *   await modal.getByRole('button', { name: 'OK' }).click();
 */
export function getAntModal(page: Page, testId: string): Locator {
    return page.getByTestId(testId).locator('.ant-modal-wrap');
}

export async function selectApprovalRootVolumeModeIfRequired(
    page: Page,
    modal: Locator,
    timeout = 15_000
): Promise<void> {
    const rootVolumeModeCombobox = modal
        .getByRole('combobox', { name: /root volume mode/i })
        .first();
    const isRequired = await rootVolumeModeCombobox
        .waitFor({ state: 'visible', timeout: Math.min(timeout, 5_000) })
        .then(() => true)
        .catch(() => false);

    if (!isRequired) {
        return;
    }

    await selectAntOption(page, rootVolumeModeCombobox, undefined, timeout);
}

/**
 * Issue a same-origin fetch using the JWT persisted by the app in localStorage.
 * Useful in E2E tests when we need deterministic API triggering from page context.
 */
export async function fetchStatusWithStoredToken(
    page: Page,
    path: string,
    method: 'GET' | 'POST' | 'PATCH' | 'PUT' | 'DELETE' = 'GET'
): Promise<number> {
    return page.evaluate(async ({ path: requestPath, method: requestMethod }) => {
        let token = '';
        try {
            const raw = window.localStorage.getItem('shepherd-auth');
            const parsed = raw ? JSON.parse(raw) : null;
            token = typeof parsed?.state?.token === 'string' ? parsed.state.token : '';
        } catch {
            token = '';
        }

        const headers: Record<string, string> = {};
        if (token) {
            headers.Authorization = `Bearer ${token}`;
        }

        const resp = await fetch(requestPath, {
            method: requestMethod,
            headers,
        });
        return resp.status;
    }, { path, method });
}

interface LoginResponsePayload {
    token?: string;
    force_password_change?: boolean;
}

interface AuthFlowOptions {
    username: string;
    primaryPassword: string;
    secondaryPassword?: string;
    currentPasswordHint?: string;
}

type AuthHeaders = { Authorization: string };

interface BatchRateLimitSetupOptions extends AuthFlowOptions {
    reasonPrefix?: string;
    maxPendingParents?: number;
    maxPendingChildren?: number;
    cooldownSeconds?: number;
}

export async function expectSchemaResponse(
    respPromise: Promise<Response | APIResponse>,
    schemaName: string,
    expectedStatus: number | number[] = 200
): Promise<{ body: unknown; resp: Response | APIResponse }> {
    const resp = await respPromise;
    const statuses = Array.isArray(expectedStatus) ? expectedStatus : [expectedStatus];
    expect(statuses, `Expected HTTP ${statuses.join('/')} but got ${resp.status()} for ${resp.url()}`).toContain(resp.status());
    const body = await validateApiResponse(schemaName, resp);
    return { body, resp };
}

export function pickIDByPreferredName<T extends { id?: string; name?: string }>(
    items: T[] | undefined,
    preferredName: string
): string {
    const normalizedPreferred = preferredName.trim();
    if (normalizedPreferred !== '') {
        const preferred = (items ?? []).find((item) => (item.name ?? '').trim() === normalizedPreferred && Boolean(item.id));
        if (preferred?.id) {
            return preferred.id;
        }
    }
    return (items ?? []).find((item) => Boolean(item.id))?.id ?? '';
}

export function pickPreferredNamespace(namespaces: string[] | undefined, preferredName: string): string {
    const preferred = preferredName.trim();
    const list = (namespaces ?? []).map((ns) => ns.trim()).filter(Boolean);
    if (preferred !== '' && list.includes(preferred)) {
        return preferred;
    }
    return list[0] ?? preferredName;
}

function escapeRegExp(input: string): string {
    return input.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
}

export function toLooseOptionFilter(rawName: string): RegExp {
    const tokens = rawName
        .trim()
        .split(/[\s_-]+/)
        .filter(Boolean)
        .map(escapeRegExp);
    if (tokens.length === 0) {
        return /.*/;
    }
    return new RegExp(tokens.join('\\s*[-_ ]*\\s*'), 'i');
}

export async function resolveClusterOptionFilter(
    request: APIRequestContext,
    headers: AuthHeaders,
    preferredClusterName: string
): Promise<RegExp> {
    const clustersResp = await request.get('/api/v1/admin/clusters?page=1&per_page=100', { headers });
    expect(clustersResp.status(), `GET /admin/clusters returned ${clustersResp.status()}`).toBe(200);
    const clustersBody = await validateApiResponse('ClusterList', clustersResp) as {
        items?: Array<{ id?: string; name?: string; display_name?: string; displayName?: string }>;
    };
    const clusters = clustersBody.items ?? [];
    expect(clusters.length, 'Cluster option filter requires at least one cluster').toBeGreaterThan(0);

    const preferred =
        clusters.find((item) => (item.name ?? '').trim() === preferredClusterName) ??
        clusters[0];
    const label = String((preferred.display_name ?? preferred.displayName ?? preferred.name ?? '')).trim();
    expect(label, 'Cluster option label cannot be empty').toBeTruthy();
    return toLooseOptionFilter(label);
}

function uniqueCandidates(values: Array<string | undefined>): string[] {
    const seen = new Set<string>();
    const out: string[] = [];
    for (const value of values) {
        const v = (value ?? '').trim();
        if (!v || seen.has(v)) continue;
        seen.add(v);
        out.push(v);
    }
    return out;
}

function resolveNextPassword(current: string, preferred?: string): string {
    const candidate = (preferred ?? '').trim();
    if (candidate && candidate !== current) {
        return candidate;
    }
    if (current === 'admin') {
        return 'ShepherdLive!2026';
    }
    return `${current}-new`;
}

async function submitUILogin(page: Page, username: string, password: string) {
    await page.goto('/login');
    await expect(page.getByRole('heading', { name: /shepherd/i })).toBeVisible();
    await page.locator('#login_username').fill(username);
    await page.locator('#login_password').fill(password);

    const loginRespPromise = page.waitForResponse(
        (r) => urlPathEndsWith(r.url(), '/api/v1/auth/login') && r.request().method() === 'POST'
    );
    await page.getByRole('button', { name: 'Login' }).click();
    return loginRespPromise;
}

async function completeForcedPasswordChangeUI(page: Page, currentPassword: string, newPassword: string): Promise<void> {
    await expect(page).toHaveURL(/\/auth\/change-password(?:\?.*)?$/);
    await expect(page.locator('#change-password_current_password')).toBeVisible();
    await page.locator('#change-password_current_password').fill(currentPassword);
    await page.locator('#change-password_new_password').fill(newPassword);
    await page.locator('#change-password_confirm_password').fill(newPassword);
    const submitButton = page.getByRole('button', { name: /change password/i }).first();
    await expect(submitButton).toBeVisible();
    await expect(submitButton).toBeEnabled();

    const changeRespPromise = page.waitForResponse(
        (r) => urlPathEndsWith(r.url(), '/api/v1/auth/change-password') && r.request().method() === 'POST'
    );
    await submitButton.click();
    const changeResp = await changeRespPromise;
    expect(changeResp.status(), `POST /auth/change-password returned ${changeResp.status()}`).toBe(204);
    // In live runs, first-time compile of /dashboard can take several seconds.
    await expect(page).toHaveURL(/\/dashboard$/, { timeout: 45_000 });
}

/**
 * UI login helper that supports first-login forced password change and password fallback.
 *
 * Returns the password that is valid after login flow completes.
 */
export async function loginWithForcePasswordSupport(
    page: Page,
    options: AuthFlowOptions
): Promise<string> {
    const candidates = uniqueCandidates([
        options.currentPasswordHint,
        options.primaryPassword,
        options.secondaryPassword,
    ]);
    let lastStatus = 0;

    for (const password of candidates) {
        const loginResp = await submitUILogin(page, options.username, password);
        lastStatus = loginResp.status();
        if (lastStatus === 401) {
            continue;
        }

        expect(lastStatus, `POST /auth/login returned unexpected status ${lastStatus}`).toBe(200);
        const loginBody = await loginResp.json() as LoginResponsePayload;
        validateResponse('LoginResponse', loginBody);

        if (!loginBody.force_password_change) {
            // In live runs, first-time compile of /dashboard can take several seconds.
            await expect(page).toHaveURL(/\/dashboard$/, { timeout: 45_000 });
            return password;
        }

        const nextPassword = resolveNextPassword(password, options.secondaryPassword);
        await completeForcedPasswordChangeUI(page, password, nextPassword);
        return nextPassword;
    }

    throw new Error(
        `Unable to log in user "${options.username}" with candidate passwords; ` +
        `last status: ${lastStatus}`
    );
}

/**
 * API login helper with the same semantics as UI login helper:
 * fallback candidate passwords + auto handling force_password_change.
 */
async function getApiTokenWithForcePasswordSupport(
    request: APIRequestContext,
    options: AuthFlowOptions
): Promise<{ token: string; password: string }> {
    const candidates = uniqueCandidates([
        options.currentPasswordHint,
        options.primaryPassword,
        options.secondaryPassword,
    ]);
    let lastStatus = 0;

    for (const password of candidates) {
        const loginResp = await request.post('/api/v1/auth/login', {
            data: { username: options.username, password },
        });
        lastStatus = loginResp.status();
        if (lastStatus === 401) {
            continue;
        }

        expect(lastStatus, `POST /auth/login returned unexpected status ${lastStatus}`).toBe(200);
        const loginBody = await loginResp.json() as LoginResponsePayload;
        validateResponse('LoginResponse', loginBody);
        const token = loginBody.token ?? '';
        expect(token, 'LoginResponse.token is required').toBeTruthy();

        if (!loginBody.force_password_change) {
            return { token, password };
        }

        const nextPassword = resolveNextPassword(password, options.secondaryPassword);
        const changeResp = await request.post('/api/v1/auth/change-password', {
            headers: { Authorization: `Bearer ${token}` },
            data: {
                old_password: password,
                new_password: nextPassword,
            },
        });
        expect(changeResp.status(), `POST /auth/change-password returned ${changeResp.status()}`).toBe(204);

        const reloginResp = await request.post('/api/v1/auth/login', {
            data: { username: options.username, password: nextPassword },
        });
        expect(reloginResp.status(), `POST /auth/login after password change returned ${reloginResp.status()}`).toBe(200);
        const reloginBody = await reloginResp.json() as LoginResponsePayload;
        validateResponse('LoginResponse', reloginBody);
        expect(reloginBody.force_password_change, 'force_password_change should be false after password change').not.toBeTruthy();
        const reloginToken = reloginBody.token ?? '';
        expect(reloginToken, 'LoginResponse.token is required after password change').toBeTruthy();
        return { token: reloginToken, password: nextPassword };
    }

    throw new Error(
        `Unable to log in user "${options.username}" via API with candidate passwords; ` +
        `last status: ${lastStatus}`
    );
}

export async function getApiAuthHeadersWithForcePasswordSupport(
    request: APIRequestContext,
    options: AuthFlowOptions
): Promise<{ headers: AuthHeaders; password: string; token: string }> {
    const auth = await getApiTokenWithForcePasswordSupport(request, options);
    return {
        headers: { Authorization: `Bearer ${auth.token}` },
        password: auth.password,
        token: auth.token,
    };
}

export async function createTempAdminUser(
    request: APIRequestContext,
    headers: AuthHeaders,
    options?: {
        prefix?: string;
        password?: string;
        displayName?: string;
    }
): Promise<{ id: string; username: string }> {
    const prefix = options?.prefix ?? 'e2e-user';
    const username = `${prefix}-${Date.now().toString(36).slice(-8)}`.slice(0, 30);
    const createResp = await request.post('/api/v1/admin/users', {
        headers,
        data: {
            username,
            password: options?.password ?? 'ShepherdLive!2026',
            display_name: options?.displayName ?? 'Live E2E User',
            enabled: true,
            force_password_change: false,
        },
    });
    expect(createResp.status(), `POST /admin/users returned ${createResp.status()}`).toBe(201);
    const created = await validateApiResponse('User', createResp) as { id?: string; username?: string };
    const id = created.id ?? '';
    expect(id, 'Created user id is required').toBeTruthy();
    return { id, username: created.username ?? username };
}

export async function deleteAdminUserIfPresent(
    request: APIRequestContext,
    headers: AuthHeaders,
    userID: string
): Promise<void> {
    if (!userID) {
        return;
    }
    const deleteResp = await request.delete(`/api/v1/admin/users/${userID}`, { headers });
    expect([204, 404], `DELETE /admin/users/${userID} returned ${deleteResp.status()}`).toContain(deleteResp.status());
}

export async function createTempService(
    request: APIRequestContext,
    headers: AuthHeaders,
    systemID: string,
    options?: {
        prefix?: string;
        description?: string;
    }
): Promise<{ id: string; name: string }> {
    const prefix = options?.prefix ?? 'svc';
    const name = `${prefix}-${Date.now().toString(36).slice(-8)}`.slice(0, 15);
    const createResp = await request.post(`/api/v1/systems/${systemID}/services`, {
        headers,
        data: {
            name,
            description: options?.description ?? 'temporary live e2e service',
        },
    });
    expect(createResp.status(), `POST /systems/{id}/services returned ${createResp.status()}`).toBe(201);
    const created = await validateApiResponse('Service', createResp) as { id?: string; name?: string };
    const id = created.id ?? '';
    expect(id, 'Created service id is required').toBeTruthy();
    return { id, name: created.name ?? name };
}

export async function deleteServiceIfPresent(
    request: APIRequestContext,
    headers: AuthHeaders,
    systemID: string,
    serviceID: string
): Promise<void> {
    if (!systemID || !serviceID) {
        return;
    }
    const deleteResp = await request.delete(`/api/v1/systems/${systemID}/services/${serviceID}?confirm=true`, { headers });
    expect([204, 404, 409], `DELETE /systems/{id}/services/{id} returned ${deleteResp.status()}`).toContain(deleteResp.status());
}

export async function createOIDCAuthProvider(
    request: APIRequestContext,
    headers: AuthHeaders,
    overrides?: {
        name?: string;
        config?: Record<string, unknown>;
    }
): Promise<{ id: string; name: string }> {
    const name = overrides?.name ?? `e2e-auth-${Date.now().toString(36).slice(-6)}`;
    const createResp = await request.post('/api/v1/admin/auth-providers', {
        headers,
        data: {
            name,
            auth_type: 'oidc',
            enabled: true,
            config: {
                issuer_url: 'https://idp.example.com',
                client_id: 'shepherd-e2e',
                client_secret: 'secret',
                scopes: ['openid', 'profile', 'email'],
                sample_users: [
                    {
                        external_id: 'e2e-alice',
                        username: 'alice',
                        display_name: 'Alice Example',
                        email: 'alice@example.com',
                        groups: ['ops'],
                        department: 'platform',
                    },
                    {
                        external_id: 'e2e-bob',
                        username: 'bob',
                        display_name: 'Bob Example',
                        email: 'bob@example.com',
                        groups: ['dev'],
                        department: 'engineering',
                    },
                ],
                ...(overrides?.config ?? {}),
            },
        },
    });
    expect(createResp.status(), `POST /admin/auth-providers returned ${createResp.status()}`).toBe(201);
    const created = await validateApiResponse('AuthProvider', createResp) as { id?: string; name?: string };
    const id = created.id ?? '';
    expect(id, 'Created auth provider id is required').toBeTruthy();
    return { id, name };
}

export async function deleteAuthProviderIfPresent(
    request: APIRequestContext,
    headers: AuthHeaders,
    providerID: string
): Promise<void> {
    if (!providerID) {
        return;
    }
    const resp = await request.delete(`/api/v1/admin/auth-providers/${providerID}`, { headers });
    expect([204, 404], `DELETE /admin/auth-providers/${providerID} returned ${resp.status()}`).toContain(resp.status());
}

/**
 * Ensure batch-submit happy-path tests are deterministic by setting explicit
 * per-user batch policy through admin APIs (API-first setup, no assertion weakening).
 */
export async function ensureBatchSubmitPolicyForUser(
    request: APIRequestContext,
    options: BatchRateLimitSetupOptions
): Promise<{ password: string; userID: string }> {
    const auth = await getApiTokenWithForcePasswordSupport(request, options);
    const headers = { Authorization: `Bearer ${auth.token}` };

    const meResp = await request.get('/api/v1/auth/me', { headers });
    expect(meResp.status(), `GET /auth/me returned ${meResp.status()}`).toBe(200);
    const meBody = await meResp.json() as { id?: string };
    validateResponse('UserInfo', meBody);
    const userID = (meBody.id ?? '').trim();
    expect(userID, 'Authenticated user id is required for batch policy setup').toBeTruthy();

    const reasonPrefix = (options.reasonPrefix ?? 'live-e2e batch policy setup').trim();
    const reason = `${reasonPrefix} ${Date.now()}`;

    const exemptionResp = await request.post('/api/v1/admin/rate-limits/exemptions', {
        headers,
        data: {
            user_id: userID,
            reason,
        },
    });
    expect(
        exemptionResp.status(),
        `POST /admin/rate-limits/exemptions returned ${exemptionResp.status()}`
    ).toBe(200);
    validateResponse('RateLimitExemption', await exemptionResp.json());

    const overrideResp = await request.put(`/api/v1/admin/rate-limits/users/${userID}`, {
        headers,
        data: {
            max_pending_parents: options.maxPendingParents ?? 128,
            max_pending_children: options.maxPendingChildren ?? 4096,
            cooldown_seconds: options.cooldownSeconds ?? 0,
            reason,
        },
    });
    expect(
        overrideResp.status(),
        `PUT /admin/rate-limits/users/{user_id} returned ${overrideResp.status()}`
    ).toBe(200);
    validateResponse('RateLimitUserOverride', await overrideResp.json());

    return { password: auth.password, userID };
}

/**
 * Idempotent API-first seed: ensure the named system and service exist.
 *
 * Uses GET-then-POST pattern — safe to call from multiple test files.
 * Returns the system and service IDs for downstream consumption.
 */
export async function ensureSeedSystemAndService(
    request: APIRequestContext,
    options: AuthFlowOptions & {
        systemName: string;
        serviceName: string;
    }
): Promise<{ password: string; systemId: string; serviceId: string }> {
    const auth = await getApiTokenWithForcePasswordSupport(request, options);
    const headers = { Authorization: `Bearer ${auth.token}` };

    // ── Ensure system exists ─────────────────────────────────────────────────
    const systemsResp = await request.get('/api/v1/systems', { headers });
    expect(systemsResp.status(), 'GET /systems must return 200 in seed setup').toBe(200);
    const systemsBody = await systemsResp.json() as {
        items?: Array<{ id?: string; name?: string }>;
    };
    validateResponse('SystemList', systemsBody);

    let system = (systemsBody.items ?? []).find(
        (item) => (item.name ?? '').trim() === options.systemName
    );
    if (!system?.id) {
        const createResp = await request.post('/api/v1/systems', {
            headers,
            data: {
                name: options.systemName,
                description: `Auto-seeded by E2E test setup`,
            },
        });
        expect(
            createResp.status(),
            `POST /systems returned ${createResp.status()} for seed system "${options.systemName}"`
        ).toBe(201);
        const created = await createResp.json() as { id?: string; name?: string };
        validateResponse('System', created);
        system = created;
    }
    const systemId = (system?.id ?? '').trim();
    expect(systemId, `Seed system "${options.systemName}" must have an id`).toBeTruthy();

    // ── Ensure service exists under the system ───────────────────────────────
    const servicesResp = await request.get(`/api/v1/systems/${systemId}/services`, { headers });
    expect(servicesResp.status(), 'GET /systems/{id}/services must return 200 in seed setup').toBe(200);
    const servicesBody = await servicesResp.json() as {
        items?: Array<{ id?: string; name?: string }>;
    };
    validateResponse('ServiceList', servicesBody);

    let service = (servicesBody.items ?? []).find(
        (item) => (item.name ?? '').trim() === options.serviceName
    );
    if (!service?.id) {
        const createResp = await request.post(`/api/v1/systems/${systemId}/services`, {
            headers,
            data: {
                name: options.serviceName,
                description: `Auto-seeded by E2E test setup`,
            },
        });
        expect(
            createResp.status(),
            `POST /systems/${systemId}/services returned ${createResp.status()} for seed service "${options.serviceName}"`
        ).toBe(201);
        const created = await createResp.json() as { id?: string; name?: string };
        validateResponse('Service', created);
        service = created;
    }
    const serviceId = (service?.id ?? '').trim();
    expect(serviceId, `Seed service "${options.serviceName}" must have an id`).toBeTruthy();

    return { password: auth.password, systemId, serviceId };
}
