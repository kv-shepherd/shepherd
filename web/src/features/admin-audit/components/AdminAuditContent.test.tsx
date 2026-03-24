import { render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

vi.mock('react-i18next', () => ({
    useTranslation: () => ({
        t: (key: string, options?: { defaultValue?: string; total?: number }) => {
            const labels: Record<string, string> = {
                'audit.title': 'Audit Logs',
                'audit.subtitle': 'Review platform activity',
                'common:button.refresh': 'Refresh',
                'audit.filter.resource_type': 'Resource Type',
                'audit.filter.action': 'Action',
                'audit.filter.approval_decision': 'Approval Decision',
                'audit.filter.actor': 'Actor',
                'audit.filter.placement_advisory_code': 'Advisory Code',
                'audit.filter.placement_reason_code': 'Reason Code',
                'common:button.search': 'Search',
                'audit.action': 'Action',
                'audit.decision': 'Decision',
                'audit.actor': 'Actor',
                'audit.resource_type': 'Resource Type',
                'audit.resource_id': 'Resource ID',
                'audit.placement': 'Placement',
                'common:table.created_at': 'Created',
                'common:table.total': `Total ${options?.total ?? 0}`,
                'audit.action_code.create': 'Create',
                'audit.resource_option.vm': 'VM',
                'audit.placement.eligible': 'Eligible',
            };
            return labels[key] ?? options?.defaultValue ?? key;
        },
    }),
}));

vi.mock('@/hooks/useApiQuery', () => ({
    useApiGet: () => ({
        data: {
            items: [
                {
                    id: 'audit-1',
                    action: 'create',
                    actor: 'alice',
                    resource_type: 'vm',
                    resource_id: 'vm-1',
                    approval_decision: 'approved',
                    placement_summary: {
                        eligible: true,
                        selected_cluster_name: 'Cluster A',
                    },
                    created_at: '2026-03-17T00:00:00Z',
                    details: {
                        debug_id: 'raw-details',
                    },
                },
            ],
            pagination: { total: 1, page: 1, per_page: 20 },
        },
        isLoading: false,
        refetch: vi.fn(),
    }),
}));

vi.mock('@/lib/api/client', () => ({
    api: {
        GET: vi.fn(),
    },
}));

import { AdminAuditContent } from './AdminAuditContent';

describe('AdminAuditContent', () => {
    it('renders the page shell, filters, and audit table', () => {
        render(<AdminAuditContent />);

        expect(screen.getByText('Audit Logs')).toBeVisible();
        expect(screen.getByText('Refresh')).toBeVisible();
        expect(screen.getByText('Search')).toBeVisible();
        expect(screen.getByText('alice')).toBeVisible();
        expect(screen.getByText('Eligible')).toBeVisible();
    });
});
