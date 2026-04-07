import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';

vi.mock('react-i18next', () => ({
    useTranslation: () => ({
        t: (key: string, options?: { defaultValue?: string; total?: number; count?: number }) => {
            const labels: Record<string, string> = {
                'audit.title': 'Audit Logs',
                'audit.subtitle': 'Review platform activity',
                'audit.feed': 'Activity Feed',
                'audit.context_title': 'Context',
                'audit.view_details': 'Details',
                'audit.drawer_title': 'Audit Event Details',
                'common:button.refresh': 'Refresh',
                'audit.filter.resource_type': 'Resource Type',
                'audit.filter.action': 'Action',
                'audit.filter.approval_decision': 'Approval Decision',
                'audit.filter.actor': 'Actor',
                'audit.filter.placement_advisory_code': 'Advisory Code',
                'audit.filter.placement_reason_code': 'Reason Code',
                'audit.filter.resource_id': 'Resource ID',
                'audit.search_placeholder': 'Search action, actor, resource type, or resource ID',
                'audit.search_help': 'Quick search matches action, actor, resource type, and resource ID.',
                'audit.preset.label': 'Quick views',
                'audit.preset.all': 'All activity',
                'audit.preset.requests': 'Requests',
                'audit.preset.approvals': 'Approvals',
                'audit.preset.resource_changes': 'Resource changes',
                'audit.preset.system_tasks': 'System tasks',
                'audit.context.batch_items': `${options?.count ?? 0} items`,
                'audit.context.scope': 'Scope',
                'audit.context.requester': 'Requester',
                'audit.context.approver': 'Approver',
                'audit.context.owner': 'Owner',
                'audit.context.target': 'Target',
                'audit.context.requested_change': 'Requested change',
                'audit.batch_item.pending_vm': `Pending VM #${options?.count ?? 0}`,
                'audit.advanced_search_title': 'Advanced search',
                'audit.advanced_search_help': 'Use advanced search for approval decisions, placement diagnostics, and exact resource context.',
                'common:button.search': 'Search',
                'common:button.clear': 'Clear',
                'common:search.advanced': 'Advanced search',
                'common:search.hide_advanced': 'Hide advanced search',
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
                    action: 'vm.create_requested',
                    actor: 'alice',
                    actor_summary: {
                        display_name: 'Alice Chen',
                        secondary: 'alice · alice@example.com',
                    },
                    resource_type: 'vm',
                    resource_id: 'vm-1',
                    resource_summary: {
                        display_name: 'vm-frontend-1',
                        secondary: 'Platform / Checkout',
                        tertiary: 'team-test · kubevirt-test02',
                    },
                    approval_decision: 'approved',
                    ticket_summary: {
                        system_name: 'Platform',
                        service_name: 'Checkout',
                        namespace: 'team-test',
                        cluster_name: 'kubevirt-test02',
                        template_name: 'Ubuntu 22.04',
                        instance_size_name: 'M4 Large',
                        target_cpu_cores: 4,
                        target_memory_gi: 8,
                        target_disk_gb: 80,
                    },
                    placement_summary: {
                        eligible: true,
                        selected_cluster_name: 'Cluster A',
                    },
                    created_at: '2026-03-17T00:00:00Z',
                    details: {
                        debug_id: 'raw-details',
                    },
                },
                {
                    id: 'audit-2',
                    action: 'approval.batch_approved',
                    actor: 'admin-1',
                    actor_summary: {
                        display_name: 'Default Administrator',
                        secondary: 'admin',
                    },
                    resource_type: 'ticket',
                    resource_id: '019d4d16-f738-7266-a8a7-99148343435f',
                    approval_decision: 'batch_approved',
                    ticket_summary: {
                        system_name: 'shop',
                        service_name: 'redis',
                        batch_count: 2,
                        requester_display_name: 'Default Administrator',
                        requester_username: 'admin',
                        approver_display_name: 'Default Administrator',
                        approver_username: 'admin',
                        owner_display_name: 'Default Administrator',
                        owner_username: 'admin',
                        namespace: 'gtest1',
                        template_name: 'Ubuntu 22.04',
                        instance_size_name: 'M4 Large',
                        target_cpu_cores: 4,
                        target_memory_gi: 8,
                        target_disk_gb: 80,
                        items: [
                            {
                                system_name: 'shop',
                                service_name: 'redis',
                                owner_display_name: 'Default Administrator',
                                owner_username: 'admin',
                                namespace: 'gtest1',
                                cluster_name: 'kubevirt-test02',
                                cluster_environment: 'test',
                                template_name: 'Ubuntu 22.04',
                                instance_size_name: 'M4 Large',
                                target_cpu_cores: 4,
                                target_memory_gi: 8,
                                target_disk_gb: 80,
                            },
                            {
                                system_name: 'shop',
                                service_name: 'redis',
                                owner_display_name: 'Default Administrator',
                                owner_username: 'admin',
                                namespace: 'gtest1',
                                cluster_name: 'kubevirt-test02',
                                cluster_environment: 'test',
                                template_name: 'Ubuntu 22.04',
                                instance_size_name: 'M4 Large',
                                target_cpu_cores: 4,
                                target_memory_gi: 8,
                                target_disk_gb: 80,
                            },
                        ],
                    },
                    created_at: '2026-04-02T04:41:00Z',
                    details: {},
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
        expect(screen.getAllByText('Refresh')[0]).toBeVisible();
        expect(screen.getByPlaceholderText('Search action, actor, resource type, or resource ID')).toBeVisible();
        expect(screen.getByText('Quick views')).toBeVisible();
        expect(screen.getByRole('button', { name: 'Requests' })).toBeVisible();
        expect(screen.getByText('Advanced search')).toBeVisible();
        expect(screen.getByText('vm-frontend-1')).toBeVisible();
        expect(screen.getByText((content) => content.includes('Alice Chen'))).toBeVisible();
        expect(screen.getAllByText('2 items').length).toBeGreaterThan(0);
        expect(screen.queryByText('019d4d16-f738-7266-a8a7-99148343435f')).not.toBeInTheDocument();
        expect(screen.getAllByText('shop · redis').length).toBeGreaterThan(0);
        expect(screen.getByText('Cluster A')).toBeVisible();
    });

    it('opens a detail drawer with readable sections', async () => {
        const user = userEvent.setup();
        render(<AdminAuditContent />);

        await user.click(screen.getAllByRole('button', { name: /details/i })[0]);

        expect(screen.getByText('Audit Event Details')).toBeVisible();
        expect(screen.getByText('Platform')).toBeVisible();
        expect(screen.getByText('Ubuntu 22.04')).toBeVisible();
    });

    it('shows readable batch item details with scope owner and configuration', async () => {
        const user = userEvent.setup();
        render(<AdminAuditContent />);

        await user.click(screen.getAllByRole('button', { name: /details/i })[1]);

        expect(screen.getByText('Pending VM #1')).toBeVisible();
        expect(screen.getAllByText('shop · redis').length).toBeGreaterThan(0);
        expect(screen.getAllByText('Default Administrator · admin').length).toBeGreaterThan(0);
        expect(screen.getAllByText('gtest1 · kubevirt-test02 · test').length).toBeGreaterThan(0);
        expect(screen.queryByText('019d4d16-f738-7266-a8a7-99148343435f')).not.toBeInTheDocument();
    });
});
