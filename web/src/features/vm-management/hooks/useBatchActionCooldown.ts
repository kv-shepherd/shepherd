'use client';

import { useCallback, useEffect, useState } from 'react';

import type { ApiErrorResponse } from '@/hooks/useApiQuery';
import { extractBatchRetryAfterSeconds } from '@/features/vm-management/batchActions';

export function useBatchActionCooldown() {
    const [cooldown, setCooldown] = useState({ untilMs: 0, contactAdmin: false });
    const [nowMs, setNowMs] = useState(() => Date.now());

    useEffect(() => {
        if (cooldown.untilMs <= Date.now()) {
            return;
        }
        const timer = window.setInterval(() => {
            const now = Date.now();
            setNowMs(now);
            setCooldown((current) => (now >= current.untilMs ? { untilMs: 0, contactAdmin: false } : current));
        }, 1_000);
        return () => window.clearInterval(timer);
    }, [cooldown.untilMs]);

    const capture = useCallback((error: ApiErrorResponse): boolean => {
        if (error.code !== 'BATCH_RATE_LIMITED') {
            return false;
        }
        const seconds = extractBatchRetryAfterSeconds(error);
        if (seconds <= 0) {
            return false;
        }
        const now = Date.now();
        setNowMs(now);
        const requestedUntilMs = now + seconds * 1_000;
        const contactAdmin = error.params?.contact_admin === true;
        setCooldown((current) => ({
            untilMs: Math.max(current.untilMs, requestedUntilMs),
            contactAdmin: (current.untilMs > now && current.contactAdmin) || contactAdmin,
        }));
        return true;
    }, []);

    const retryAfterSeconds = Math.max(0, Math.ceil((cooldown.untilMs - nowMs) / 1_000));
    return {
        capture,
        isActive: retryAfterSeconds > 0,
        retryAfterSeconds,
        contactAdmin: cooldown.contactAdmin,
        clear: () => {
            setCooldown({ untilMs: 0, contactAdmin: false });
        },
    };
}
