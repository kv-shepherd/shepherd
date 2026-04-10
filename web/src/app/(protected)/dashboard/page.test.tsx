import { render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

const useApiGetMock = vi.fn();
const searchParamsState = new URLSearchParams();
const pushMock = vi.fn();

vi.mock('react-i18next', () => ({
    useTranslation: () => ({
        t: (key: string) => {
            const labels: Record<string, string> = {
                'nav.dashboard': 'Dashboard',
                'dashboard.subtitle': 'Operational overview of the platform',
                'nav.systems': 'Systems',
                'nav.services': 'Services',
                'nav.vms': 'Virtual Machines',
                'nav.my_requests': 'My Requests',
                'dashboard.status.ready': 'Ready',
                'dashboard.status.live': 'Live',
                'dashboard.status.version': 'Version',
                'dashboard.overview.systems_description': 'Recent systems and ownership boundaries.',
                'dashboard.overview.services_description': 'Recent services and service allocation context.',
                'dashboard.overview.vms_description': 'Latest visible virtual machines and runtime state.',
                'dashboard.overview.requests_description': 'Recent requests from your workbench.',
                'dashboard.overview.empty_systems': 'No systems are available yet.',
                'dashboard.overview.empty_services': 'No services are available yet.',
                'dashboard.overview.empty_vms': 'No virtual machines are available yet.',
                'dashboard.overview.empty_requests': 'You do not have recent requests yet.',
                'dashboard.overview.pending_badge': '2 pending',
                'dashboard.overview.service_total': '3 services',
                'dashboard.overview.more_items': '+1 more',
                'dashboard.overview.next_vm_number_label': 'Next VM',
                'dashboard.overview.workspace_hint': 'Use the workspace button to review all',
                'dashboard.action.open_systems': 'Open systems',
                'dashboard.action.open_services': 'Open services',
                'dashboard.action.open_vms': 'Open VM workspace',
                'dashboard.action.open_requests': 'Open my requests',
                'approval:status.PENDING': 'Pending',
                'approval:status.APPROVED': 'Approved',
                'vm:status.RUNNING': 'Running',
                'vm:field.hostname': 'Hostname',
                'vm:field.ip_address': 'IP Address',
                'vm:field.system': 'System',
            };
            return labels[key] ?? key;
        },
    }),
}));

vi.mock('next/navigation', () => ({
    useSearchParams: () => searchParamsState,
    useRouter: () => ({ push: pushMock }),
}));

vi.mock('@/hooks/useApiQuery', () => ({
    useApiGet: (...args: unknown[]) => useApiGetMock(...args),
}));

vi.mock('@/lib/api/client', () => ({
    api: {
        GET: vi.fn(),
    },
}));

vi.mock('@/components/ui/LocalDateTimeText', () => ({
    LocalDateTimeText: ({ value }: { value: string }) => <time dateTime={value}>{value}</time>,
}));

vi.mock('@/features/setup-guide/components/SetupGuideCard', () => ({
    SetupGuideCard: ({ focusAction }: { focusAction?: string | null }) => (
        <div>{focusAction ? `setup-guide-card-${focusAction}` : 'setup-guide-card'}</div>
    ),
}));

import DashboardPage from './page';

describe('DashboardPage', () => {
    it('renders compact status chips, four overview cards, and the setup card', () => {
        searchParamsState.set('setup_action', 'create-instance-size');
        useApiGetMock.mockImplementation((queryKey: unknown[]) => {
            const key = queryKey as string[];
            if (key[0] === 'health') {
                return { data: { status: 'ok', version: '1.2.3' }, isLoading: false };
            }
            if (key[0] === 'health-live') {
                return { data: { status: 'ok' }, isLoading: false };
            }
            if (key[0] === 'systems' && key[1] === 'dashboard') {
                return {
                    data: {
                        items: [
                            { id: 'system-1', name: 'shop', description: 'Primary shopping system', created_by_display_name: 'Alex', created_at: '2026-04-01T00:00:00Z' },
                            { id: 'system-2', name: 'billing', description: 'Billing control plane', created_by_display_name: 'Alex', created_at: '2026-04-01T00:00:00Z' },
                            { id: 'system-3', name: 'risk', description: 'Risk services', created_by_display_name: 'Alex', created_at: '2026-04-01T00:00:00Z' },
                            { id: 'system-4', name: 'ops', description: 'Operations core', created_by_display_name: 'Alex', created_at: '2026-04-01T00:00:00Z' },
                        ],
                        pagination: { total: 5 },
                    },
                    isLoading: false,
                };
            }
            if (key[0] === 'dashboard' && key[1] === 'system-service-previews') {
                return {
                    data: {
                        'system-1': {
                            total: 3,
                            names: ['redis', 'payments'],
                        },
                    },
                    isLoading: false,
                };
            }
            if (key[0] === 'services' && key[1] === 'dashboard') {
                return {
                    data: {
                        items: [{ id: 'service-1', name: 'redis', system_name: 'shop', next_instance_index: 2, description: 'Caching tier for checkout' }],
                        pagination: { total: 7 },
                    },
                    isLoading: false,
                };
            }
            if (key[0] === 'vms' && key[1] === 'dashboard') {
                return {
                    data: {
                        items: [{
                            id: 'vm-1',
                            name: 'vm-1',
                            hostname: 'vm-1.internal',
                            status: 'RUNNING',
                            system_name: 'shop',
                            service_name: 'redis',
                            ip_address: '10.0.0.1',
                            os_name: 'openEuler',
                        }],
                        pagination: { total: 12 },
                    },
                    isLoading: false,
                };
            }
            if (key[0] === 'tickets' && key[1] === 'dashboard' && key[2] === 'mine' && key[3] !== 'pending') {
                return {
                    data: {
                        items: [{
                            id: 'ticket-1',
                            requester: 'user-1',
                            requester_display_name: 'Alex',
                            requester_username: 'alex',
                            status: 'APPROVED',
                            operation_type: 'CREATE',
                            summary: { system_name: 'shop', service_name: 'redis', instance_size_name: 'm4.large' },
                            created_at: '2026-04-01T00:00:00Z',
                        }],
                        pagination: { total: 4 },
                    },
                    isLoading: false,
                };
            }
            if (key[0] === 'tickets' && key[1] === 'dashboard' && key[2] === 'mine' && key[3] === 'pending') {
                return { data: { pagination: { total: 2 } }, isLoading: false };
            }
            return { data: { pagination: { total: 0 } }, isLoading: false };
        });

        render(<DashboardPage />);

        expect(screen.getByText('Dashboard')).toBeVisible();
        expect(screen.getByText('Operational overview of the platform')).toBeVisible();
        expect(screen.getAllByText('Ready').length).toBeGreaterThan(0);
        expect(screen.getAllByText('Live').length).toBeGreaterThan(0);
        expect(screen.queryByText('Platform Health')).not.toBeInTheDocument();
        expect(screen.getAllByText('Systems').length).toBeGreaterThan(0);
        expect(screen.getAllByText('Services').length).toBeGreaterThan(0);
        expect(screen.getAllByText('Virtual Machines').length).toBeGreaterThan(0);
        expect(screen.getAllByText('My Requests').length).toBeGreaterThan(0);
        expect(screen.getAllByText('Open systems').length).toBeGreaterThan(0);
        expect(screen.getAllByText('Open services').length).toBeGreaterThan(0);
        expect(screen.getAllByText('Open VM workspace').length).toBeGreaterThan(0);
        expect(screen.getAllByText('Open my requests').length).toBeGreaterThan(0);
        expect(screen.getByText('3 services')).toBeVisible();
        expect(screen.getAllByText('redis').length).toBeGreaterThan(0);
        expect(screen.getAllByText('+1 more').length).toBeGreaterThan(0);
        expect(screen.getByText('Use the workspace button to review all')).toBeVisible();
        expect(screen.getByText('vm-1.internal')).toBeVisible();
        expect(screen.getByText('openEuler')).toBeVisible();
        expect(screen.getByText('10.0.0.1')).toBeVisible();
        expect(screen.queryByText('Alex · alex')).not.toBeInTheDocument();
        expect(screen.getByText('setup-guide-card-create-instance-size')).toBeVisible();
    });
});
