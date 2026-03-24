import { render, screen } from '@testing-library/react';
import type { TFunction } from 'i18next';
import { describe, expect, it, vi } from 'vitest';

import { VMListTable } from './VMListTable';

describe('VMListTable', () => {
    it('renders the VM list inside the shared page surface with key row actions', () => {
        const t = ((key: string, options?: { total?: number }) => {
            const labels: Record<string, string> = {
                'field.name': 'Name',
                'common:table.status': 'Status',
                'field.namespace': 'Namespace',
                'field.hostname': 'Hostname',
                'common:table.created_at': 'Created',
                'common:table.actions': 'Actions',
                'action.start': 'Start',
                'action.stop': 'Stop',
                'action.restart': 'Restart',
                'action.console': 'Console',
                'action.delete': 'Delete',
                'action.request_modify': 'Request Resource Change',
                'action.view_details': 'View Details',
                'action.request_similar': 'Request Similar VM',
                'status.RUNNING': 'Running',
                'common:table.total': `Total ${options?.total ?? 0}`,
            };
            return labels[key] ?? key;
        }) as unknown as TFunction;

        render(
            <VMListTable
                t={t}
                vmData={{
                    items: [
                        {
                            id: 'vm-1',
                            name: 'vm-alpha',
                            status: 'RUNNING',
                            namespace: 'team-prod',
                            cluster_id: 'cluster-prod-1',
                            cluster_name: 'Production Cluster',
                            hostname: 'vm-alpha.internal',
                            created_at: '2026-03-17T00:00:00Z',
                            environment: 'prod',
                        },
                    ],
                    pagination: { total: 1, page: 1, per_page: 20 },
                }}
                isLoading={false}
                page={1}
                pageSize={20}
                onPageChange={vi.fn()}
                onStart={vi.fn()}
                onStop={vi.fn()}
                onRestart={vi.fn()}
                onConsole={vi.fn()}
                onDelete={vi.fn()}
                onModify={vi.fn()}
                onRequestSimilar={vi.fn()}
                onDetail={vi.fn()}
                selectedRowKeys={[]}
                onSelectionChange={vi.fn()}
            />
        );

        expect(screen.getByText('vm-alpha')).toBeVisible();
        expect(screen.getByText('Running')).toBeVisible();
        expect(screen.getByText('team-prod')).toBeVisible();
        expect(screen.getByText('Production Cluster')).toBeVisible();
        expect(screen.getByTestId('vm-action-console-vm-1')).toBeVisible();
        expect(screen.getByTestId('vm-action-detail-vm-1')).toBeVisible();
        expect(screen.getByTestId('vm-action-more-vm-1')).toBeVisible();
    });
});
