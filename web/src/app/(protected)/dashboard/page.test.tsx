import { render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

const useApiGetMock = vi.fn();
const searchParamsState = new URLSearchParams();

vi.mock('react-i18next', () => ({
    useTranslation: () => ({
        t: (key: string) => {
            const labels: Record<string, string> = {
                'nav.dashboard': 'Dashboard',
                'dashboard.subtitle': 'Operational overview of the platform',
                'nav.systems': 'Systems',
                'nav.vms': 'Virtual Machines',
                'nav.my_requests': 'My Requests',
                'approval:status.PENDING': 'Pending',
            };
            return labels[key] ?? key;
        },
    }),
}));

vi.mock('next/navigation', () => ({
    useSearchParams: () => searchParamsState,
}));

vi.mock('@/hooks/useApiQuery', () => ({
    useApiGet: (...args: unknown[]) => useApiGetMock(...args),
}));

vi.mock('@/lib/api/client', () => ({
    api: {
        GET: vi.fn(),
    },
}));

vi.mock('@/features/setup-guide/components/SetupGuideCard', () => ({
    SetupGuideCard: ({ focusAction }: { focusAction?: string | null }) => (
        <div>{focusAction ? `setup-guide-card-${focusAction}` : 'setup-guide-card'}</div>
    ),
}));

import DashboardPage from './page';

describe('DashboardPage', () => {
    it('renders the page shell, health banner, stats, and pending requests alert', () => {
        searchParamsState.set('setup_action', 'create-instance-size');
        let callIndex = 0;
        useApiGetMock.mockImplementation(() => {
            callIndex += 1;
            if (callIndex === 1) {
                return { data: { status: 'ok', version: '1.2.3' }, isLoading: false };
            }
            if (callIndex === 2) {
                return { data: { status: 'ok' }, isLoading: false };
            }
            if (callIndex === 3) {
                return { data: { pagination: { total: 5 } }, isLoading: false };
            }
            if (callIndex === 4) {
                return { data: { pagination: { total: 12 } }, isLoading: false };
            }
            return { data: { pagination: { total: 2 } }, isLoading: false };
        });

        render(<DashboardPage />);

        expect(screen.getByText('Dashboard')).toBeVisible();
        expect(screen.getByText('Operational overview of the platform')).toBeVisible();
        expect(screen.getByText('Platform Health')).toBeVisible();
        expect(screen.getByText('Systems')).toBeVisible();
        expect(screen.getByText('Virtual Machines')).toBeVisible();
        expect(screen.getByText('My Requests')).toBeVisible();
        expect(screen.getByText('2 pending request(s) are still in progress')).toBeVisible();
        expect(screen.getByText('setup-guide-card-create-instance-size')).toBeVisible();
    });
});
