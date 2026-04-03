import { App } from 'antd';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';

const useApiGetMock = vi.fn();
const apiGetMock = vi.fn();
const apiPostMock = vi.fn();
const apiPutMock = vi.fn();
const apiDeleteMock = vi.fn();

vi.mock('react-i18next', () => ({
    useTranslation: () => ({
        t: (key: string, options?: { defaultValue?: string }) => {
            const labels: Record<string, string> = {
                'rate_limits.title': 'Rate Limits',
                'rate_limits.subtitle': 'Platform-wide request rate limiting status.',
                'common:button.refresh': 'Refresh',
                'common:button.search': 'Search',
                'common:button.clear_filters': 'Clear filters',
                'common:button.save': 'Save',
                'common:button.cancel': 'Cancel',
                'common:button.edit': 'Edit',
                'common:button.delete': 'Delete',
                'common:table.actions': 'Actions',
                'rate_limits.empty': 'No rate limit data',
                'rate_limits.exemptions.title': 'Exemptions',
                'rate_limits.exemptions.empty': 'No exemptions configured',
                'rate_limits.exemptions.user': 'User',
                'rate_limits.exemptions.reason': 'Reason',
                'rate_limits.exemptions.expires_at': 'Expires At',
                'rate_limits.exemptions.created_at': 'Created At',
                'rate_limits.exemptions.add_action': 'Add exemption',
                'rate_limits.exemptions.edit_action': 'Edit exemption',
                'rate_limits.exemptions.modal_title': 'Manage rate-limit exemption',
                'rate_limits.exemptions.delete_confirm_title': 'Remove exemption?',
                'rate_limits.overrides.add_action': 'Set user override',
                'rate_limits.overrides.edit_action': 'Set override',
                'rate_limits.overrides.modal_title': 'Manage user rate-limit override',
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
        GET: (...args: unknown[]) => apiGetMock(...args),
        POST: (...args: unknown[]) => apiPostMock(...args),
        PUT: (...args: unknown[]) => apiPutMock(...args),
        DELETE: (...args: unknown[]) => apiDeleteMock(...args),
    },
}));

import AdminRateLimitsPage from './page';

function renderPage() {
    const queryClient = new QueryClient({
        defaultOptions: {
            queries: {
                retry: false,
            },
            mutations: {
                retry: false,
            },
        },
    });
    return render(
        <QueryClientProvider client={queryClient}>
            <App>
                <AdminRateLimitsPage />
            </App>
        </QueryClientProvider>,
    );
}

function installRateLimitQueries() {
    useApiGetMock.mockImplementation((queryKey: unknown) => {
        const key = Array.isArray(queryKey) ? queryKey[0] : queryKey;
        if (key === 'admin-rate-limits-status') {
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
                refetch: vi.fn().mockResolvedValue(undefined),
            };
        }
        if (key === 'admin-rate-limits-exemptions') {
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
                refetch: vi.fn().mockResolvedValue(undefined),
            };
        }
        return {
            data: {
                items: [
                    {
                        id: 'alice',
                        username: 'alice',
                        display_name: 'Alice Ops',
                        email: 'alice@example.com',
                    },
                    {
                        id: 'bob',
                        username: 'bob',
                        display_name: 'Bob Ops',
                        email: 'bob@example.com',
                    },
                ],
            },
            isLoading: false,
            error: null,
            refetch: vi.fn().mockResolvedValue(undefined),
        };
    });
}

describe('AdminRateLimitsPage', () => {
    it('renders the page shell and both rate limit tables', () => {
        installRateLimitQueries();

        renderPage();

        expect(screen.getByTestId('rate-limit-status-page')).toBeVisible();
        expect(screen.getByText('Rate Limits')).toBeVisible();
        expect(screen.getByText('Alice Ops')).toBeVisible();
        expect(screen.getByText('Exemptions')).toBeVisible();
        expect(screen.getByText('Bob Ops')).toBeVisible();
    });

    it('filters rate limit tables through quick search only after submit', async () => {
        const user = userEvent.setup();
        installRateLimitQueries();

        renderPage();

        await user.type(screen.getByTestId('rate-limits-quick-search'), 'bob');
        expect(screen.getByText('Alice Ops')).toBeVisible();
        expect(screen.getAllByText('Bob Ops').length).toBeGreaterThan(0);

        await user.keyboard('{Enter}');
        await waitFor(() => {
            expect(screen.queryByText('Alice Ops')).not.toBeInTheDocument();
        });
        expect(screen.getAllByText('Bob Ops').length).toBeGreaterThan(0);
    });

});
