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

import { expect, type Locator, type Page } from '@playwright/test';

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
 * @param timeout - Max time to wait for option visibility (default: 5000ms)
 */
export async function selectAntOption(
    page: Page,
    combobox: Locator,
    optionFilter?: string | RegExp,
    timeout = 5000
): Promise<void> {
    // Step 1: Click to open dropdown
    await combobox.click();

    // Step 2: Wait for any OLD dropdown leave-animations to finish.
    // Ant Design adds `.ant-slide-up-leave` during close animation.
    // We must wait for these to disappear before locating the active dropdown,
    // otherwise `.ant-select-dropdown:visible` may resolve to 2 elements.
    await expect(
        page.locator('.ant-select-dropdown.ant-slide-up-leave')
    ).toHaveCount(0, { timeout });

    // Step 3: Locate the ACTIVE dropdown (exclude any lingering leave-animations).
    // Use Playwright visibility filtering instead of CSS :visible pseudo selectors.
    const dropdown = page
        .locator('.ant-select-dropdown:not(.ant-slide-up-leave):not(.ant-slide-up-leave-active)')
        .filter({ visible: true })
        .last();
    await expect(dropdown).toBeVisible({ timeout });

    // Step 4: Find the target option within the active dropdown
    let option: Locator;
    if (optionFilter) {
        option = dropdown.locator('.ant-select-item-option').filter({ hasText: optionFilter }).first();
    } else {
        option = dropdown.locator('.ant-select-item-option').first();
    }

    // Step 5: Wait for option to be visible and stable before clicking
    await expect(option).toBeVisible({ timeout });
    await option.click();
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
