import { Form } from 'antd';
import { render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

vi.mock('react-i18next', () => ({
    useTranslation: () => ({
        t: (
            key: string,
            options?: string | { defaultValue?: string; [key: string]: unknown },
        ) => {
            const labels: Record<string, string> = {
                'clusters.title': 'Clusters',
                'clusters.subtitle': 'Manage registered clusters',
                'clusters.table.cluster': 'Cluster',
                'clusters.table.endpoint': 'API endpoint',
                'common:button.refresh': 'Refresh',
                'clusters.add': 'Add Cluster',
                'common:table.name': 'Name',
                'common:table.status': 'Status',
                'clusters.environment': 'Environment',
                'clusters.enabled': 'Enabled',
                'clusters.enabled_yes': 'Enabled',
                'clusters.enabled_no': 'Disabled',
                'clusters.api_server': 'API Server',
                'clusters.enabled_features': 'Enabled Features',
                'clusters.storage_classes': 'Storage Classes',
                'clusters.policy_summary': 'Policy Summary',
                'common:table.created_at': 'Created',
                'common:table.actions': 'Actions',
                'clusters.env_test': 'Test',
                'clusters.env_prod': 'Prod',
                'clusters.status.healthy': 'Healthy',
                'clusters.edit_policy': 'Edit Policy',
                'common:button.edit': 'Edit',
                'common:button.delete': 'Delete',
                'common:table.total': 'Total',
            };
            const fallback =
                typeof options === 'string' ? options : options?.defaultValue;
            let message = labels[key] ?? fallback ?? key;
            if (options && typeof options === 'object') {
                for (const [name, value] of Object.entries(options)) {
                    if (name === 'defaultValue') {
                        continue;
                    }
                    message = message.replaceAll(`{{${name}}}`, String(value));
                }
            }
            return message;
        },
    }),
}));

vi.mock('../hooks/useAdminClustersController', () => ({
    useAdminClustersController: () => {
        const [form] = Form.useForm();
        const [editForm] = Form.useForm();
        const [envForm] = Form.useForm();
        const [policyForm] = Form.useForm();

        return {
            messageContextHolder: null,
            data: {
                items: [
                    {
                        id: 'cluster-1',
                        name: 'cluster-a',
                        display_name: 'Cluster A',
                        status: 'READY',
                        enabled: true,
                        api_server_url: 'https://cluster-a.example.com',
                        enabled_features: ['clone', 'gpu'],
                        storage_classes: ['fast'],
                        default_storage_class: 'fast',
                        created_at: '2026-03-17T00:00:00Z',
                        environment: 'test',
                        policy_summary: null,
                    },
                ],
                pagination: { total: 1, page: 1, per_page: 20 },
            },
            isLoading: false,
            refetch: vi.fn(),
            openCreateModal: vi.fn(),
            createOpen: false,
            createPending: false,
            closeCreateModal: vi.fn(),
            submitCreate: vi.fn(),
            form,
            editOpen: false,
            editForm,
            editingClusterId: '',
            editingClusterName: '',
            editingCluster: null,
            deletingClusterId: '',
            editPending: false,
            openEditModal: vi.fn(),
            closeEditModal: vi.fn(),
            submitEdit: vi.fn(),
            envModalOpen: false,
            envForm,
            closeEnvModal: vi.fn(),
            submitEnvUpdate: vi.fn(),
            updateEnvironmentPending: false,
            policyModalOpen: false,
            policyForm,
            closePolicyModal: vi.fn(),
            submitPolicyUpdate: vi.fn(),
            upsertPolicyPending: false,
            policyLoading: false,
            selectedClusterName: '',
            selectedClusterId: '',
            selectedClusterPolicyExists: false,
            selectedClusterNamespaceOptions: [],
            selectedClusterStorageClasses: [],
            namespaceSuggestionsLoading: false,
            openEnvModal: vi.fn(),
            openPolicyModal: vi.fn(),
            deleteCluster: vi.fn(),
            deletePending: false,
        };
    },
}));

import { AdminClustersContent } from './AdminClustersContent';

describe('AdminClustersContent', () => {
    it('renders the page shell and cluster table actions', () => {
        render(<AdminClustersContent />);

        expect(screen.getByTestId('admin-clusters-page')).toBeVisible();
        expect(screen.getByText('Clusters')).toBeVisible();
        expect(screen.getByTestId('clusters-refresh-btn')).toBeVisible();
        expect(screen.getByTestId('cluster-create-button')).toBeVisible();
        expect(screen.getByText('Cluster A')).toBeVisible();
        expect(screen.getByText('cluster-a')).toBeVisible();
        expect(screen.getByTestId('cluster-action-edit-cluster-1')).toBeVisible();
        expect(screen.getByTestId('cluster-action-edit-policy-cluster-1')).toBeVisible();
        expect(screen.getByTestId('cluster-action-delete-cluster-1')).toBeVisible();
    });
});
