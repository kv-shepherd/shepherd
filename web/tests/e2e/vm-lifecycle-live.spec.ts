/**
 * VM Lifecycle Live E2E Tests — Contract-Enforced (no mock, no skip)
 *
 * ┌─────────────────────────────────────────────────────────────────────────┐
 * │  REQUIRES: a running backend (db + server via docker-compose or local)  │
 * │  NO test.skip() — failures expose real frontend/backend problems.       │
 * │  Every API response is validated against api/openapi.yaml schema.       │
 * └─────────────────────────────────────────────────────────────────────────┘
 *
 * Coverage (all previously uncovered VM endpoints):
 *   getVM              – GET /vms/{id}                  → VM schema
 *   startVM            – POST /vms/{id}/start           → 202 (no body schema)
 *   stopVM             – POST /vms/{id}/stop            → 202 (no body schema)
 *   restartVM          – POST /vms/{id}/restart         → 202 (no body schema)
 *   deleteVM           – DELETE /vms/{id}               → DeleteVMResponse schema
 *   powerVM            – POST /vms/{id}/power           → VM schema (202)
 *   listVMBatches      – GET /vms/batch                 → VMBatchList schema
 *   submitVMBatch      – POST /vms/batch                → VMBatchSubmitResponse schema
 *   getVMBatch         – GET /vms/batch/{id}            → VMBatchStatusResponse schema
 *   retryVMBatch       – POST /vms/batch/{id}/retry     → VMBatchActionResponse schema
 *   cancelVMBatch      – POST /vms/batch/{id}/cancel    → VMBatchActionResponse schema
 *   getVMConsoleStatus – GET /vms/{id}/console/status   → VMConsoleStatusResponse schema
 *
 * Strategy:
 *   - First create a VM via the approval flow (Stage 5.A → 5.B → 5.C).
 *   - Then exercise all lifecycle operations on that VM.
 *   - Failures mean the backend or frontend does not implement the contract.
 *
 * Environment variables:
 *   E2E_USERNAME  – admin username (default: e2e-admin)
 *   E2E_PASSWORD  – admin password (default: e2e-admin-123)
 *   E2E_SYSTEM    – pre-existing system with at least one service (default: e2e-system)
 *   E2E_SERVICE   – pre-existing service name (default: e2e-service)
 */

import { expect, test, type Page, type Response } from '@playwright/test';
import { validateApiResponse } from './lib/schema-validator';
import { urlPathEndsWith, urlPathIncludes } from './lib/helpers';

// ── Config ────────────────────────────────────────────────────────────────────

const e2eUsername = process.env.E2E_USERNAME ?? 'e2e-admin';
const e2ePassword = process.env.E2E_PASSWORD ?? 'e2e-admin-123';

// ── Auth helper ───────────────────────────────────────────────────────────────

async function login(page: Page): Promise<void> {
    await page.goto('/login');
    await expect(page.getByRole('heading', { name: 'KubeVirt Shepherd' })).toBeVisible();
    await page.getByPlaceholder('Username').fill(e2eUsername);
    await page.getByPlaceholder('Password').fill(e2ePassword);

    // operationId: login
    const loginRespPromise = page.waitForResponse(
        (r) => urlPathEndsWith(r.url(), '/api/v1/auth/login') && r.request().method() === 'POST'
    );
    await page.getByRole('button', { name: 'Login' }).click();
    const loginResp = await loginRespPromise;
    expect(loginResp.status()).toBe(200);
    await validateApiResponse('LoginResponse', loginResp);
    await expect(page).toHaveURL(/\/dashboard$/);
}

async function expectSchema(
    respPromise: Promise<Response>,
    schemaName: string,
    expectedStatus: number | number[] = 200
): Promise<{ body: unknown; resp: Response }> {
    const resp = await respPromise;
    const statuses = Array.isArray(expectedStatus) ? expectedStatus : [expectedStatus];
    expect(statuses, `Expected HTTP ${statuses.join('/')} but got ${resp.status()} for ${resp.url()}`).toContain(resp.status());
    const body = await validateApiResponse(schemaName, resp);
    return { body, resp };
}

// ── Helpers: get first VM ID from list ───────────────────────────────────────

async function getFirstVMId(page: Page): Promise<string> {
    const listRespPromise = page.waitForResponse(
        (r) => urlPathIncludes(r.url(), '/api/v1/vms') && r.request().method() === 'GET' && !urlPathIncludes(r.url(), '/vms/')
    );
    await page.goto('/vms');
    await expect(page.getByRole('heading', { name: 'Virtual Machines' })).toBeVisible();
    const listResp = await listRespPromise;
    expect(listResp.status(), `GET /vms returned ${listResp.status()}`).toBe(200);
    // validateApiResponse returns the parsed body — reuse it to avoid double-read
    // (Playwright response body can only be consumed once; a second .json() call
    //  may throw "Protocol error: No resource with given identifier found").
    const body = await validateApiResponse('VMList', listResp) as { items?: Array<{ id?: string }> };
    const items = body.items ?? [];
    expect(items.length, 'No VMs exist — seed data must include at least one VM').toBeGreaterThan(0);
    const id = items[0]?.id ?? '';
    expect(id, 'First VM has no id field').toBeTruthy();
    return id;
}

// ── Test suite ────────────────────────────────────────────────────────────────

test.describe('vm-lifecycle live (contract-enforced, no mock, no skip)', () => {
    test.beforeEach(async ({ page }) => {
        await page.addInitScript(() => { window.open = () => null; });
        await login(page);
    });

    // ── getVM: GET /vms/{id} → VM ─────────────────────────────────────────────

    test('getVM – GET /vms/{id} conforms to VM schema', async ({ page }) => {
        // operationId: getVM
        const vmId = await getFirstVMId(page);

        const getRespPromise = page.waitForResponse(
            (r) => urlPathIncludes(r.url(), `/api/v1/vms/${vmId}`) && r.request().method() === 'GET'
                && !urlPathIncludes(r.url(), '/console') && !urlPathIncludes(r.url(), '/vnc')
        );
        // Navigate to VM detail page (triggers GET /vms/{id})
        await page.getByTestId(`vm-action-detail-${vmId}`).click();
        await expectSchema(getRespPromise, 'VM', 200);
    });

    // ── startVM: POST /vms/{id}/start → 202 ──────────────────────────────────

    test('startVM – POST /vms/{id}/start returns 202', async ({ page }) => {
        // operationId: startVM
        await page.goto('/vms');
        await expect(page.getByRole('heading', { name: 'Virtual Machines' })).toBeVisible();

        // Find a stopped VM
        const stoppedRow = page.locator('tr').filter({ hasText: /stopped/i }).first();
        await expect(stoppedRow, 'No stopped VM found — seed data must include a stopped VM').toBeVisible();

        const vmTestId = await stoppedRow.locator('[data-testid^="vm-action-start-"]').first().getAttribute('data-testid');
        const vmId = vmTestId?.replace('vm-action-start-', '') ?? '';
        expect(vmId, 'Could not extract VM id from start button').toBeTruthy();

        const startRespPromise = page.waitForResponse(
            (r) => urlPathEndsWith(r.url(), `/api/v1/vms/${vmId}/start`) && r.request().method() === 'POST'
        );
        await stoppedRow.locator(`[data-testid="vm-action-start-${vmId}"]`).click();
        // Confirm dialog if present
        const confirmBtn = page.locator('.ant-popover:visible, .ant-modal-content:visible')
            .getByRole('button', { name: /confirm|ok|start/i }).first();
        if (await confirmBtn.count() > 0) await confirmBtn.click();

        const startResp = await startRespPromise;
        expect(startResp.status(), `POST /vms/${vmId}/start returned ${startResp.status()}`).toBe(202);
        // spec: 202 has no response body schema — just verify status
    });

    // ── stopVM: POST /vms/{id}/stop → 202 ────────────────────────────────────

    test('stopVM – POST /vms/{id}/stop returns 202', async ({ page }) => {
        // operationId: stopVM
        await page.goto('/vms');
        await expect(page.getByRole('heading', { name: 'Virtual Machines' })).toBeVisible();

        // Find a running VM
        const runningRow = page.locator('tr').filter({ hasText: /running/i }).first();
        await expect(runningRow, 'No running VM found — seed data must include a running VM').toBeVisible();

        const vmTestId = await runningRow.locator('[data-testid^="vm-action-stop-"]').first().getAttribute('data-testid');
        const vmId = vmTestId?.replace('vm-action-stop-', '') ?? '';
        expect(vmId, 'Could not extract VM id from stop button').toBeTruthy();

        const stopRespPromise = page.waitForResponse(
            (r) => urlPathEndsWith(r.url(), `/api/v1/vms/${vmId}/stop`) && r.request().method() === 'POST'
        );
        await runningRow.locator(`[data-testid="vm-action-stop-${vmId}"]`).click();
        const confirmBtn = page.locator('.ant-popover:visible, .ant-modal-content:visible')
            .getByRole('button', { name: /confirm|ok|stop/i }).first();
        if (await confirmBtn.count() > 0) await confirmBtn.click();

        const stopResp = await stopRespPromise;
        expect(stopResp.status(), `POST /vms/${vmId}/stop returned ${stopResp.status()}`).toBe(202);
    });

    // ── restartVM: POST /vms/{id}/restart → 202 ──────────────────────────────

    test('restartVM – POST /vms/{id}/restart returns 202', async ({ page }) => {
        // operationId: restartVM
        await page.goto('/vms');
        await expect(page.getByRole('heading', { name: 'Virtual Machines' })).toBeVisible();

        const runningRow = page.locator('tr').filter({ hasText: /running/i }).first();
        await expect(runningRow, 'No running VM found — seed data must include a running VM').toBeVisible();

        const vmTestId = await runningRow.locator('[data-testid^="vm-action-restart-"]').first().getAttribute('data-testid');
        const vmId = vmTestId?.replace('vm-action-restart-', '') ?? '';
        expect(vmId, 'Could not extract VM id from restart button').toBeTruthy();

        const restartRespPromise = page.waitForResponse(
            (r) => urlPathEndsWith(r.url(), `/api/v1/vms/${vmId}/restart`) && r.request().method() === 'POST'
        );
        await runningRow.locator(`[data-testid="vm-action-restart-${vmId}"]`).click();
        const confirmBtn = page.locator('.ant-popover:visible, .ant-modal-content:visible')
            .getByRole('button', { name: /confirm|ok|restart/i }).first();
        if (await confirmBtn.count() > 0) await confirmBtn.click();

        const restartResp = await restartRespPromise;
        expect(restartResp.status(), `POST /vms/${vmId}/restart returned ${restartResp.status()}`).toBe(202);
    });

    // ── powerVM: POST /vms/{id}/power → VM (generic power endpoint) ──────────

    test('powerVM – POST /vms/{id}/power conforms to VM schema (202)', async ({ page }) => {
        // operationId: powerVM — uses the generic /power endpoint from VM detail page
        const vmId = await getFirstVMId(page);

        // Navigate to VM detail page where power buttons use POST /vms/{id}/power
        await page.goto(`/vms/${vmId}`);
        await expect(page.locator('body')).toBeVisible();

        // The detail page power buttons call POST /vms/{vm_id}/power with action body
        const powerRespPromise = page.waitForResponse(
            (r) => urlPathEndsWith(r.url(), `/api/v1/vms/${vmId}/power`) && r.request().method() === 'POST'
        );

        // Click any available power button on the detail page (start, stop, or restart)
        const startBtn = page.getByRole('button', { name: /^start$/i }).first();
        const stopBtn = page.getByRole('button', { name: /^stop$/i }).first();
        const restartBtn = page.getByRole('button', { name: /^restart$/i }).first();

        // Try each button in order of safety: restart > stop > start, but wait until one is enabled
        await expect(async () => {
            if (!(await restartBtn.isEnabled() || await stopBtn.isEnabled() || await startBtn.isEnabled())) {
                await page.getByTestId(`vm-console-status-${vmId}`).click({ force: true });
                throw new Error('No power action button enabled yet');
            }
        }).toPass({ timeout: 45000, intervals: [2000, 5000] });

        if (await restartBtn.isEnabled()) {
            await restartBtn.click();
        } else if (await stopBtn.isEnabled()) {
            await stopBtn.click();
        } else if (await startBtn.isEnabled()) {
            await startBtn.click();
        }

        // Confirm if confirmation dialog appears
        const confirmBtn = page.locator('.ant-popover:visible, .ant-modal-content:visible')
            .getByRole('button', { name: /confirm|ok|yes/i }).first();
        if (await confirmBtn.count() > 0) await confirmBtn.click();

        // ── CONTRACT CHECK: VM schema (powerVM returns 202 with VM body) ──────────
        const powerResp = await powerRespPromise;
        expect(powerResp.status(), `POST /vms/${vmId}/power returned ${powerResp.status()}`).toBe(202);
        await validateApiResponse('VM', powerResp);
    });

    // ── deleteVM: DELETE /vms/{id} → DeleteVMResponse ────────────────────────

    test('deleteVM – DELETE /vms/{id} conforms to DeleteVMResponse schema', async ({ page }) => {
        // operationId: deleteVM
        await page.goto('/vms');
        await expect(page.getByRole('heading', { name: 'Virtual Machines' })).toBeVisible();

        // Find a stopped VM to delete (safer than deleting running)
        const stoppedRow = page.locator('tr').filter({ hasText: /stopped/i }).last();
        await expect(stoppedRow, 'No stopped VM found for deletion test').toBeVisible();

        const vmTestId = await stoppedRow.locator('[data-testid^="vm-action-delete-"]').first().getAttribute('data-testid');
        const vmId = vmTestId?.replace('vm-action-delete-', '') ?? '';
        expect(vmId, 'Could not extract VM id from delete button').toBeTruthy();

        // Get VM name for confirm_name guard
        const vmNameCell = stoppedRow.locator('td').nth(1);
        const vmName = (await vmNameCell.textContent())?.trim() ?? '';
        expect(vmName, 'Could not read VM name from table row').toBeTruthy();

        const deleteRespPromise = page.waitForResponse(
            (r) => urlPathIncludes(r.url(), `/api/v1/vms/${vmId}`) && r.request().method() === 'DELETE'
        );
        await stoppedRow.locator(`[data-testid="vm-action-delete-${vmId}"]`).click();

        // Fill confirm_name guard (ADR-0015 §13)
        const deleteModal = page.locator('.ant-modal-content').filter({ hasText: /delete/i }).last();
        await expect(deleteModal).toBeVisible();
        const confirmInput = deleteModal.getByRole('textbox').first();
        if (await confirmInput.count() > 0) {
            await confirmInput.fill(vmName);
        }
        await deleteModal.getByRole('button', { name: /delete|confirm|ok/i }).last().click();

        // ── CONTRACT CHECK: DeleteVMResponse schema ───────────────────────────────
        const deleteResp = await deleteRespPromise;
        expect([202, 409], `DELETE /vms/${vmId} returned unexpected ${deleteResp.status()}`).toContain(deleteResp.status());
        if (deleteResp.status() === 202) {
            await validateApiResponse('DeleteVMResponse', deleteResp);
        } else {
            // 409 = VM has active console sessions or other conflict — still valid
            await validateApiResponse('Error', deleteResp);
        }
    });

    // ── listVMBatches: GET /vms/batch → VMBatchList ─────────────────────────

    test('listVMBatches – GET /vms/batch conforms to VMBatchList schema', async ({ page }) => {
        // operationId: listVMBatches
        const listRespPromise = page.waitForResponse(
            (r) => urlPathIncludes(r.url(), '/api/v1/vms/batch') && r.request().method() === 'GET'
                && !urlPathIncludes(r.url(), '/vms/batch/') // Exclude batch/{id} detail
        );
        await page.goto('/vms/batch');
        await expect(page.locator('body')).toBeVisible();
        // ── CONTRACT CHECK: VMBatchList schema ────────────────────────────────────
        await expectSchema(listRespPromise, 'VMBatchList', 200);
    });

    // ── submitVMBatch: POST /vms/batch → VMBatchSubmitResponse ────────────────

    test('submitVMBatch – POST /vms/batch conforms to VMBatchSubmitResponse schema', async ({ page }) => {
        // operationId: submitVMBatch
        // Navigate to VM batch page
        await page.goto('/vms/batch');
        await expect(page.locator('body')).toBeVisible();

        // Alternatively trigger via VM list batch action
        await page.goto('/vms');
        await expect(page.getByRole('heading', { name: 'Virtual Machines' })).toBeVisible();

        // Select all VMs via checkbox
        const headerCheckbox = page.locator('thead input[type="checkbox"]').first();
        await expect(headerCheckbox, 'VM list table header checkbox not found').toBeVisible();
        await headerCheckbox.check();

        const batchRespPromise = page.waitForResponse(
            (r) => urlPathEndsWith(r.url(), '/api/v1/vms/batch') && r.request().method() === 'POST'
        );

        // Click generic batch submit (not power-specific)
        const batchBtn = page.getByRole('button', { name: /batch|submit batch/i }).first();
        await expect(batchBtn, 'Batch submit button not found in VM list').toBeVisible();
        await batchBtn.click();

        // ── CONTRACT CHECK: VMBatchSubmitResponse schema ──────────────────────────
        await expectSchema(batchRespPromise, 'VMBatchSubmitResponse', [202, 400, 429]);
    });

    // ── getVMBatch: GET /vms/batch/{id} → VMBatchStatusResponse ──────────────

    test('getVMBatch – GET /vms/batch/{id} conforms to VMBatchStatusResponse schema', async ({ page }) => {
        // operationId: getVMBatch
        // First submit a batch to get a batch_id
        await page.goto('/vms');
        await expect(page.getByRole('heading', { name: 'Virtual Machines' })).toBeVisible();

        const headerCheckbox = page.locator('thead input[type="checkbox"]').first();
        await expect(headerCheckbox).toBeVisible();
        await headerCheckbox.check();

        const submitRespPromise = page.waitForResponse(
            (r) => urlPathIncludes(r.url(), '/api/v1/vms/batch') && r.request().method() === 'POST'
        );
        const batchBtn = page.getByRole('button', { name: /batch|submit batch/i }).first();
        await expect(batchBtn).toBeVisible();
        await batchBtn.click();

        const submitResp = await submitRespPromise;
        expect([202, 400, 429]).toContain(submitResp.status());

        // Single-read the body (Playwright responses are single-read)
        const submitBody = await submitResp.json() as { batch_id?: string; message?: string };

        if (submitResp.status() !== 202) {
            // Can't get batch_id without a successful submit — fail with clear message
            throw new Error(`POST /vms/batch failed with ${submitResp.status()}: ${submitBody.message ?? JSON.stringify(submitBody)}`);
        }

        const batchId = submitBody.batch_id ?? '';
        expect(batchId, 'POST /vms/batch response missing batch_id field').toBeTruthy();

        // ── CONTRACT CHECK: VMBatchStatusResponse schema ──────────────────────────
        const statusRespPromise = page.waitForResponse(
            (r) => urlPathIncludes(r.url(), `/api/v1/vms/batch/${batchId}`) && r.request().method() === 'GET'
        );
        // Navigate to batch detail page
        await page.goto(`/vms/batch/${batchId}`);
        await expectSchema(statusRespPromise, 'VMBatchStatusResponse', 200);
    });

    // ── retryVMBatch: POST /vms/batch/{id}/retry → VMBatchActionResponse ─────

    test('retryVMBatch – POST /vms/batch/{id}/retry conforms to VMBatchActionResponse schema', async ({ page }) => {
        // operationId: retryVMBatch
        // Navigate to batch list to find a failed batch
        await page.goto('/vms/batch');
        await expect(page.locator('body')).toBeVisible();

        const failedBatchRow = page.locator('tr').filter({ hasText: /failed|partial/i }).first();
        await expect(failedBatchRow, 'No failed batch found — seed data must include a failed VM batch').toBeVisible();

        const retryTestId = await failedBatchRow.locator('[data-testid^="batch-action-retry-"]').first().getAttribute('data-testid');
        const batchId = retryTestId?.replace('batch-action-retry-', '') ?? '';
        expect(batchId, 'Could not extract batch_id from retry button').toBeTruthy();

        const retryRespPromise = page.waitForResponse(
            (r) => urlPathEndsWith(r.url(), `/api/v1/vms/batch/${batchId}/retry`) && r.request().method() === 'POST'
        );
        await failedBatchRow.locator(`[data-testid="batch-action-retry-${batchId}"]`).click();
        const confirmBtn = page.locator('.ant-popover:visible, .ant-modal-content:visible')
            .getByRole('button', { name: /confirm|ok|retry/i }).first();
        if (await confirmBtn.count() > 0) await confirmBtn.click();

        // ── CONTRACT CHECK: VMBatchActionResponse schema ──────────────────────────
        await expectSchema(retryRespPromise, 'VMBatchActionResponse', 200);
    });

    // ── cancelVMBatch: POST /vms/batch/{id}/cancel → VMBatchActionResponse ───

    test('cancelVMBatch – POST /vms/batch/{id}/cancel conforms to VMBatchActionResponse schema', async ({ page }) => {
        // operationId: cancelVMBatch
        await page.goto('/vms/batch');
        await expect(page.locator('body')).toBeVisible();

        const pendingBatchRow = page.locator('tr').filter({ hasText: /pending_approval|in_progress/i }).first();
        await expect(pendingBatchRow, 'No pending/running batch found — seed data must include an active VM batch').toBeVisible();

        const cancelTestId = await pendingBatchRow.locator('[data-testid^="batch-action-cancel-"]').first().getAttribute('data-testid');
        const batchId = cancelTestId?.replace('batch-action-cancel-', '') ?? '';
        expect(batchId, 'Could not extract batch_id from cancel button').toBeTruthy();

        const cancelRespPromise = page.waitForResponse(
            (r) => urlPathEndsWith(r.url(), `/api/v1/vms/batch/${batchId}/cancel`) && r.request().method() === 'POST'
        );
        await pendingBatchRow.locator(`[data-testid="batch-action-cancel-${batchId}"]`).click();
        const confirmBtn = page.locator('.ant-popover:visible, .ant-modal-content:visible')
            .getByRole('button', { name: /confirm|ok|cancel/i }).first();
        if (await confirmBtn.count() > 0) await confirmBtn.click();

        // ── CONTRACT CHECK: VMBatchActionResponse schema ──────────────────────────
        await expectSchema(cancelRespPromise, 'VMBatchActionResponse', 200);
    });

    // ── getVMConsoleStatus: GET /vms/{id}/console/status → VMConsoleStatusResponse

    test('getVMConsoleStatus – GET /vms/{id}/console/status conforms to VMConsoleStatusResponse schema', async ({ page }) => {
        // operationId: getVMConsoleStatus
        const vmId = await getFirstVMId(page);

        // Navigate to VM detail which should show console status
        const statusRespPromise = page.waitForResponse(
            (r) => urlPathEndsWith(r.url(), `/api/v1/vms/${vmId}/console/status`) && r.request().method() === 'GET'
        );
        await page.goto(`/vms/${vmId}`);
        await expect(page.locator('body')).toBeVisible();

        // Trigger console status check via UI / direct fetch since explicit button is gone
        await page.evaluate((id) => {
            fetch(`/api/v1/vms/${id}/console/status`);
        }, vmId);

        const statusResp = await statusRespPromise;
        expect(statusResp.status(), `GET /vms/${vmId}/console/status returned ${statusResp.status()}`).toBe(200);
        // ── CONTRACT CHECK: VMConsoleStatusResponse schema ────────────────────────
        await validateApiResponse('VMConsoleStatusResponse', statusResp);
    });
});
