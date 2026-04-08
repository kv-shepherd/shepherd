import { fireEvent, render, screen } from '@testing-library/react';
import type { TFunction } from 'i18next';
import { describe, expect, it, vi } from 'vitest';

import { VMListTable } from './VMListTable';
import type { VM } from '../types';

describe('VMListTable', () => {
    it('renders the VM list inside the shared page surface with key row actions', () => {
        const t = ((key: string, options?: { total?: number }) => {
            const labels: Record<string, string> = {
                'field.name': 'Name',
                'common:table.status': 'Status',
                'field.namespace': 'Namespace',
                'field.hostname': 'Hostname',
                'field.host_ip': 'Host IP',
                'field.ip_address': 'IP Address',
                'field.operating_system': 'Operating System',
                'field.system': 'System',
                'field.scope': 'Scope',
                'field.placement': 'Runtime location',
                'field.cluster': 'Cluster',
                'field.resources': 'Resources',
                'field.cpu': 'CPU',
                'field.memory': 'Memory',
                'field.disk': 'Disk',
                'field.created_at': 'Created',
                'common:table.created_at': 'Created',
                'common:table.actions': 'Actions',
                'context.row_badge': 'Current service',
                'action.start': 'Start',
                'action.stop': 'Stop',
                'action.restart': 'Restart',
                'action.console': 'Console',
                'action.console_serial': 'Open Serial Console',
                'action.console_vnc': 'Open noVNC Console',
                'action.delete': 'Delete',
                'action.request_modify': 'Request Resource Change',
                'action.view_details': 'View Details',
                'action.request_similar': 'Request Similar VM',
                'status.RUNNING': 'Running',
                'common:table.total': `Total ${options?.total ?? 0}`,
            };
            return labels[key] ?? key;
        }) as unknown as TFunction;

        const onOpenSystem = vi.fn();
        const onOpenService = vi.fn();
        const vmItem = {
            id: 'vm-1',
            name: 'vm-alpha',
            status: 'RUNNING',
            namespace: 'team-prod',
            cluster_id: 'cluster-prod-1',
            cluster_name: 'Production Cluster',
            hostname: 'vm-alpha',
            host_ip: '10.1.2.3',
            ip_address: '10.0.0.18',
            system_id: 'sys-1',
            system_name: 'Payments',
            service_id: 'svc-1',
            service_name: 'Billing API',
            os_name: 'Ubuntu 24.04.2 LTS',
            os_version: '24.04',
            os_family: 'linux',
            cpu_cores: 4,
            memory_gi: 8,
            disk_gb: 60,
            created_at: '2026-03-17T00:00:00Z',
            environment: 'prod',
        } as VM;
        render(
            <VMListTable
                t={t}
                vmData={{
                    items: [vmItem],
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
                onOpenSystem={onOpenSystem}
                onOpenService={onOpenService}
                contextSystemId="sys-1"
                contextServiceId="svc-1"
                selectedRowKeys={[]}
                onSelectionChange={vi.fn()}
            />
        );

        expect(screen.getAllByText('vm-alpha').length).toBeGreaterThan(0);
        expect(screen.getByText('Running')).toBeVisible();
        expect(screen.getByText(/team-prod/)).toBeVisible();
        expect(screen.getByText(/Production Cluster/)).toBeVisible();
        expect(screen.getByText(/10\.0\.0\.18/)).toBeVisible();
        expect(screen.getByText('Payments')).toBeVisible();
        expect(screen.getByText('Billing API')).toBeVisible();
        expect(screen.getByText('Current service')).toBeVisible();
        expect(screen.getByText(/Ubuntu 24\.04\.2 LTS/)).toBeVisible();
        expect(screen.getByText(/4 vCPU/)).toBeVisible();
        expect(screen.getByText(/8 Gi/)).toBeVisible();
        expect(screen.getByText(/60 Gi/)).toBeVisible();
        expect(screen.getByTestId('vm-name-copy-vm-1')).toBeVisible();
        expect(screen.getByTestId('vm-action-console-vm-1')).toBeVisible();
        expect(screen.getByTestId('vm-action-detail-vm-1')).toBeVisible();
        expect(screen.getByTestId('vm-action-more-vm-1')).toBeVisible();

        fireEvent.click(screen.getByText('Payments'));
        expect(onOpenSystem).toHaveBeenCalledWith('sys-1');

        fireEvent.click(screen.getByText('Billing API'));
        expect(onOpenService).toHaveBeenCalledWith('sys-1', 'svc-1');
    });

    it('disables the console action when no console capability is available', () => {
        const t = ((key: string) => {
            const labels: Record<string, string> = {
                'field.name': 'Name',
                'common:table.status': 'Status',
                'field.namespace': 'Namespace',
                'field.hostname': 'Hostname',
                'field.host_ip': 'Host IP',
                'field.ip_address': 'IP Address',
                'field.operating_system': 'Operating System',
                'field.system': 'System',
                'field.scope': 'Scope',
                'field.placement': 'Runtime location',
                'field.cluster': 'Cluster',
                'field.resources': 'Resources',
                'field.cpu': 'CPU',
                'field.memory': 'Memory',
                'field.disk': 'Disk',
                'field.created_at': 'Created',
                'common:table.created_at': 'Created',
                'common:table.actions': 'Actions',
                'action.start': 'Start',
                'action.stop': 'Stop',
                'action.restart': 'Restart',
                'action.console': 'Console',
                'action.console_serial': 'Open Serial Console',
                'action.console_vnc': 'Open noVNC Console',
                'action.delete': 'Delete',
                'action.request_modify': 'Request Resource Change',
                'action.view_details': 'View Details',
                'action.request_similar': 'Request Similar VM',
                'status.RUNNING': 'Running',
            };
            return labels[key] ?? key;
        }) as unknown as TFunction;

        render(
            <VMListTable
                t={t}
                vmData={{
                    items: [{
                        id: 'vm-2',
                        name: 'vm-headless',
                        status: 'RUNNING',
                        namespace: 'team-prod',
                        created_at: '2026-03-17T00:00:00Z',
                        console_capabilities: {
                            serial_available: false,
                            vnc_available: false,
                        },
                    } as VM],
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
                onOpenSystem={vi.fn()}
                onOpenService={vi.fn()}
                selectedRowKeys={[]}
                onSelectionChange={vi.fn()}
            />,
        );

        expect(screen.getByTestId('vm-action-console-vm-2')).toBeDisabled();
    });

    it('allows stop for VMs that are still starting', () => {
        const t = ((key: string) => {
            const labels: Record<string, string> = {
                'field.name': 'Name',
                'common:table.status': 'Status',
                'field.namespace': 'Namespace',
                'field.system': 'System',
                'field.host_ip': 'Host IP',
                'field.ip_address': 'IP Address',
                'field.operating_system': 'Operating System',
                'field.scope': 'Scope',
                'field.placement': 'Runtime location',
                'field.cluster': 'Cluster',
                'field.resources': 'Resources',
                'field.cpu': 'CPU',
                'field.memory': 'Memory',
                'field.disk': 'Disk',
                'field.created_at': 'Created',
                'common:table.created_at': 'Created',
                'common:table.actions': 'Actions',
                'action.start': 'Start',
                'action.stop': 'Stop',
                'action.stop_confirm': 'Stop this VM?',
                'action.restart': 'Restart',
                'action.console': 'Console',
                'action.delete': 'Delete',
                'action.request_modify': 'Request Resource Change',
                'action.view_details': 'View Details',
                'action.request_similar': 'Request Similar VM',
                'status.STARTING': 'Starting',
            };
            return labels[key] ?? key;
        }) as unknown as TFunction;

        render(
            <VMListTable
                t={t}
                vmData={{
                    items: [{
                        id: 'vm-3',
                        name: 'vm-starting',
                        status: 'STARTING',
                        namespace: 'team-prod',
                        created_at: '2026-03-17T00:00:00Z',
                    } as VM],
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
                onOpenSystem={vi.fn()}
                onOpenService={vi.fn()}
                selectedRowKeys={[]}
                onSelectionChange={vi.fn()}
            />,
        );

        expect(screen.getByTestId('vm-action-stop-vm-3')).toBeEnabled();
        expect(screen.getByTestId('vm-action-start-vm-3')).toBeDisabled();
    });
});
