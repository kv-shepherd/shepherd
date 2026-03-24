import { render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

const useApiGetMock = vi.fn();

vi.mock('react-i18next', () => ({
    useTranslation: () => ({
        t: (key: string, options?: { defaultValue?: string }) => {
            const labels: Record<string, string> = {
                'rate_limits.title': 'Rate Limits',
                'rate_limits.subtitle': 'Platform-wide request rate limiting status.',
                'common:button.refresh': 'Refresh',
                'rate_limits.empty': 'No rate limit data',
                'rate_limits.exemptions.title': 'Exemptions',
                'rate_limits.exemptions.empty': 'No exemptions configured',
                'rate_limits.exemptions.user': 'User',
                'rate_limits.exemptions.reason': 'Reason',
                'rate_limits.exemptions.expires_at': 'Expires At',
                'rate_limits.exemptions.created_at': 'Created At',
                'common:message.error': 'Error',
            };
            return labels[key] ?? options?.defaultValue ?? key;
        },
    }),
}));

vi.mock('@/lib/api/useApiGet', () => ({
    useApiGet: (...args: unknown[]) => useApiGetMock(...args),
}));

vi.mock('@/lib/api/client', () => ({
    api: {
        GET: vi.fn(),
    },
}));

import AdminRateLimitsPage from './page';

describe('AdminRateLimitsPage', () => {
    it('renders the page shell and both rate limit tables', () => {
        let callIndex = 0;
        useApiGetMock.mockImplementation(() => {
            callIndex += 1;
            if (callIndex === 1) {
                return {
                    data: {
                        items: [
                            {
                                user_id: 'alice',
                                username: 'alice',
                                display_name: 'Alice Ops',
                                email: 'alice@example.com',
                                exempted: false,
                                exemption_expires_at: null,
                                effective_max_pending_parents: 5,
                                effective_max_pending_children: 20,
                                effective_cooldown_seconds: 60,
                                current_pending_parents: 1,
                                current_pending_children: 2,
                                cooldown_remaining_seconds: 0,
                            },
                        ],
                        generated_at: '2026-03-17T00:00:00Z',
                    },
                    isLoading: false,
                    error: null,
                    refetch: vi.fn(),
                };
            }
            return {
                    data: {
                        items: [
                            {
                                user_id: 'bob',
                                username: 'bob',
                                display_name: 'Bob Ops',
                                email: 'bob@example.com',
                                exempted_by: 'admin',
                                reason: 'CI automation',
                                expires_at: '',
                                created_at: '2026-03-17T00:00:00Z',
                                updated_at: '2026-03-17T00:00:00Z',
                            },
                        ],
                    },
                isLoading: false,
                error: null,
                refetch: vi.fn(),
            };
        });

        render(<AdminRateLimitsPage />);

        expect(screen.getByTestId('rate-limit-status-page')).toBeVisible();
        expect(screen.getByText('Rate Limits')).toBeVisible();
        expect(screen.getByText('Alice Ops')).toBeVisible();
        expect(screen.getByText('Exemptions')).toBeVisible();
        expect(screen.getByText('Bob Ops')).toBeVisible();
    });
});
