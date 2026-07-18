import { act, renderHook } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';

import { useBatchActionCooldown } from './useBatchActionCooldown';

describe('useBatchActionCooldown', () => {
    afterEach(() => {
        vi.useRealTimers();
    });

    it('counts down Retry-After and re-enables actions when the window expires', () => {
        vi.useFakeTimers();
        vi.setSystemTime(new Date('2026-07-19T00:00:00Z'));
        const { result } = renderHook(() => useBatchActionCooldown());

        act(() => {
            expect(
                result.current.capture({
                    code: 'BATCH_RATE_LIMITED',
                    retry_after_seconds: 3,
                    params: { contact_admin: true },
                }),
            ).toBe(true);
        });
        expect(result.current.isActive).toBe(true);
        expect(result.current.retryAfterSeconds).toBe(3);
        expect(result.current.contactAdmin).toBe(true);

        act(() => {
            vi.advanceTimersByTime(1_000);
        });
        expect(result.current.retryAfterSeconds).toBe(2);

        act(() => {
            vi.advanceTimersByTime(2_000);
        });
        expect(result.current.isActive).toBe(false);
        expect(result.current.retryAfterSeconds).toBe(0);
        expect(result.current.contactAdmin).toBe(false);
    });

    it('ignores non-rate-limit errors and malformed Retry-After values', () => {
        const { result } = renderHook(() => useBatchActionCooldown());

        act(() => {
            expect(result.current.capture({ code: 'BATCH_ACTION_NOT_APPLICABLE' })).toBe(false);
            expect(
                result.current.capture({
                    code: 'BATCH_RATE_LIMITED',
                    retry_after_seconds: 0,
                }),
            ).toBe(false);
        });
        expect(result.current.isActive).toBe(false);
        expect(result.current.contactAdmin).toBe(false);
    });

    it('clears contact-admin guidance together with the cooldown', () => {
        const { result } = renderHook(() => useBatchActionCooldown());

        act(() => {
            result.current.capture({
                code: 'BATCH_RATE_LIMITED',
                retry_after_seconds: 5,
                params: { contact_admin: true },
            });
        });
        expect(result.current.contactAdmin).toBe(true);

        act(() => result.current.clear());
        expect(result.current.isActive).toBe(false);
        expect(result.current.contactAdmin).toBe(false);
    });

    it('never shortens an active cooldown or drops administrator guidance', () => {
        vi.useFakeTimers();
        vi.setSystemTime(new Date('2026-07-19T00:00:00Z'));
        const { result } = renderHook(() => useBatchActionCooldown());

        act(() => {
            result.current.capture({
                code: 'BATCH_RATE_LIMITED',
                retry_after_seconds: 10,
                params: { contact_admin: true },
            });
            result.current.capture({
                code: 'BATCH_RATE_LIMITED',
                retry_after_seconds: 2,
            });
        });

        expect(result.current.retryAfterSeconds).toBe(10);
        expect(result.current.contactAdmin).toBe(true);
    });
});
