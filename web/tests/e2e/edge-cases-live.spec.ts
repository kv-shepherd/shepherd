/**
 * Edge Cases Live E2E Tests — Contract-Enforced & UI Validation
 *
 * ┌─────────────────────────────────────────────────────────────────────────┐
 * │  This suite focuses STRICTLY on edge cases, negative testing,           │
 * │  and UI validations to achieve 100% branch path coverage.               │
 * └─────────────────────────────────────────────────────────────────────────┘
 *
 * Targets:
 *   - Form Validations (Empty fields, Max length, Invalid format combinations)
 *   - 401 Unauthorized (Invalid logins)
 *   - Modal safety guards (Delete guards require explicit names)
 *   - Invalid API responses checking (Simulating 400 Bad Request if bypassed)
 */

import { expect, test, type Page } from '@playwright/test';
import { getAntModal, loginWithForcePasswordSupport, selectAntOption, urlPathEndsWith } from './lib/helpers';

const e2eUsername = process.env.E2E_USERNAME ?? 'e2e-admin';
const e2ePassword = process.env.E2E_PASSWORD ?? 'e2e-admin-123';
const e2eNewPassword = process.env.E2E_NEW_PASSWORD ?? (e2ePassword === 'admin' ? 'admin123' : `${e2ePassword}-new`);
const e2eSystemName = process.env.E2E_SYSTEM ?? 'e2e-system';
const e2eServiceName = process.env.E2E_SERVICE ?? 'e2e-service';
let activePassword = e2ePassword;

// ── Shared UI Actions ──
async function fillInvalidLogin(page: Page): Promise<void> {
    await page.goto('/login');
    await expect(page.getByRole('heading', { name: /shepherd/i })).toBeVisible();
    await page.getByPlaceholder('Username').fill(e2eUsername);
    await page.getByPlaceholder('Password').fill('WRONG_PASSWORD_12345');

    // Catch the login response
    const loginRespPromise = page.waitForResponse(
        (r) => urlPathEndsWith(r.url(), '/api/v1/auth/login') && r.request().method() === 'POST'
    );
    await page.getByRole('button', { name: 'Login' }).click();

    const loginResp = await loginRespPromise;
    expect(loginResp.status(), 'Login with wrong password should fail').toBe(401);

    // UI should expose an auth error (inline alert OR top message toast).
    const inlineError = page.locator('.ant-alert-error').first();
    const toastError = page.locator('.ant-message .ant-message-error').first();
    await expect
        .poll(async () => (await inlineError.isVisible()) || (await toastError.isVisible()), {
            timeout: 10000,
            message: 'Expected visible auth error after 401 login response',
        })
        .toBeTruthy();
}

async function loginAdmin(page: Page): Promise<void> {
    activePassword = await loginWithForcePasswordSupport(page, {
        username: e2eUsername,
        primaryPassword: e2ePassword,
        secondaryPassword: e2eNewPassword,
        currentPasswordHint: activePassword,
    });
}

test.describe('Edge Cases / Negative Paths', () => {

    test('Auth - Invalid Login 401 Edge Case', async ({ page }) => {
        await fillInvalidLogin(page);
    });

    test.describe('With Admin Auth', () => {
        test.beforeEach(async ({ page }) => {
            await page.addInitScript(() => { window.open = () => null; });
            await loginAdmin(page);
        });

        test('System Form - Validation Boundaries (RFC1035 & Max Length)', async ({ page }) => {
            await page.goto('/systems');
            await expect(page.getByRole('heading', { name: /systems/i })).toBeVisible();
            await page.getByTestId('system-create-button').click();
            const modal = getAntModal(page, 'system-create-modal');
            await expect(modal).toBeVisible();

            // 1. Empty submit
            await modal.getByRole('button', { name: /ok|create|submit/i }).click();
            await expect(modal.locator('.ant-form-item-explain-error').first()).toBeVisible();

            // 2. Max length logic (15 chars limit)
            const input = modal.locator('#create-system_name');
            const tooLong = 'this-name-is-way-too-long-for-system';
            await input.fill(tooLong);
            // Ant design limits at input level generally via maxLength attribute
            const val = await input.inputValue();
            expect(val.length).toBeLessThanOrEqual(15);

            // 3. Regex Invalid RFC1035 (capital letters / spaces)
            await input.fill('Invalid Name_123!');
            await input.blur(); // Trigger validation
            await modal.getByRole('button', { name: /ok|create|submit/i }).click();
            await expect(modal.locator('.ant-form-item-explain-error').first()).toBeVisible();

            await modal.getByRole('button', { name: /cancel/i }).click();
        });

        test('System Form - Delete Object Guard Validator Edge Case', async ({ page }) => {
            // Find a system to click delete on
            await page.goto('/systems');
            await expect(page.getByRole('heading', { name: /systems/i })).toBeVisible();

            // Just open the modal
            const firstDeleteBtn = page.locator('[data-testid^="system-action-delete-"]').first();
            await expect(firstDeleteBtn).toBeVisible({ timeout: 10000 });
            await firstDeleteBtn.click();

            const deleteModal = getAntModal(page, 'system-delete-modal');
            await expect(deleteModal).toBeVisible();

            const confirmInput = deleteModal.getByRole('textbox').first();
            const okBtn = deleteModal.getByRole('button', { name: /delete/i });

            // Button disabled by default
            await expect(okBtn).toBeDisabled();

            // Typing wrong name
            await confirmInput.fill('wrong-name-validation');
            await expect(okBtn).toBeDisabled();

            await deleteModal.getByRole('button', { name: /cancel/i }).click();
        });

        test('VM Request - Batch Count Edge Cases', async ({ page }) => {
            await page.goto('/vms');
            await expect(page.getByRole('heading', { name: /virtual machines/i })).toBeVisible();
            await page.getByRole('button', { name: /request vm/i }).click();
            const wizardModal = getAntModal(page, 'vm-request-wizard-modal');
            await expect(wizardModal).toBeVisible();

            // Walk to config step with valid selections (Stage 5.A wizard flow).
            await selectAntOption(page, wizardModal.locator('[role="combobox"]').first(), e2eSystemName);
            await selectAntOption(page, wizardModal.locator('[role="combobox"]').nth(1), e2eServiceName);
            await wizardModal.getByRole('button', { name: /next/i }).click();

            await selectAntOption(page, wizardModal.locator('[role="combobox"]').first());
            await wizardModal.getByRole('button', { name: /next/i }).click();

            await selectAntOption(page, wizardModal.locator('[role="combobox"]').first());
            await wizardModal.getByRole('button', { name: /next/i }).click();

            await wizardModal.locator('#vm-request-wizard_namespace').fill('edge-ns');
            await wizardModal.locator('#vm-request-wizard_reason').fill('edge case: batch_count validation');

            const batchCountInput = wizardModal.locator('#vm-request-wizard_batch_count');

            // InputNumber enforces min/max bounds at input level (1..50).
            await batchCountInput.fill('0');
            await batchCountInput.blur();
            await expect(batchCountInput).toHaveValue('1');

            await batchCountInput.fill('51');
            await batchCountInput.blur();
            await expect(batchCountInput).toHaveValue('50');

            // valid value should allow entering confirm step.
            await batchCountInput.fill('1');
            await wizardModal.getByRole('button', { name: /next/i }).click();
            await expect(wizardModal.getByText(/requested vm count/i)).toBeVisible();

            await page.keyboard.press('Escape');
        });
    });
});
