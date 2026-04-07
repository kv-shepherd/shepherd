import { Form } from 'antd';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';

const selectUserMock = vi.fn();
const searchParamsState = new URLSearchParams();

vi.mock('next/navigation', () => ({
    useSearchParams: () => searchParamsState,
}));

vi.mock('react-i18next', () => ({
    useTranslation: () => ({
        t: (key: string, options?: { defaultValue?: string; [key: string]: unknown }) => {
            const labels: Record<string, string> = {
                'rbac.title': 'Access & Roles',
                'rbac.subtitle': 'Define reusable roles, audit elevated access bindings, and review the permission catalog in one place.',
                'rbac.roles.title': 'Role Catalog',
                'rbac.roles.subtitle': 'Manage role definitions',
                'rbac.roles.add': 'Add Role',
                'rbac.roles.help_title': 'Define capabilities here, grant access below',
                'rbac.roles.help_description': 'Use this section to bundle permission keys into reusable roles.',
                'rbac.bindings.title': 'Access Bindings',
                'rbac.bindings.subtitle': 'Audit elevated platform-facing access across users. Everyday runtime bindings remain easiest to manage from the Users page.',
                'rbac.bindings.add': 'Add Binding',
                'rbac.bindings.help_title': 'Bindings are where access is actually granted',
                'rbac.bindings.help_description': 'This binding view focuses on elevated platform-facing access. Use the Users page for complete per-user binding management.',
                'rbac.bindings.scope': 'Scope',
                'rbac.bindings.all_environments': 'All environments',
                'rbac.bindings.select_user': 'Target user',
                'rbac.bindings.select_user_placeholder': 'Search by username, display name, or email',
                'rbac.bindings.select_user_first': 'Please select a user first',
                'rbac.bindings.select_user_help': 'Choose a user first, then review their elevated platform-facing access here.',
                'rbac.bindings.selected_user_hint': 'You are viewing the elevated platform-facing bindings assigned to this user.',
                'rbac.permissions.title': 'Permissions',
                'rbac.permissions.subtitle': 'Reference available permissions',
                'rbac.permissions.help_title': 'Permission directory',
                'rbac.permissions.help_description': 'Use this catalog to understand which permission keys can be bundled into roles.',
                'rbac.summary.roles_title': 'Role Catalog',
                'rbac.summary.roles_description': 'All roles currently available for assignment.',
                'rbac.summary.custom_roles_title': 'Custom roles',
                'rbac.summary.custom_roles_description': 'Roles created specifically for your platform, excluding built-ins.',
                'rbac.summary.bindings_title': 'Elevated bindings',
                'rbac.summary.bindings_description': 'Select a user to review their elevated platform-facing bindings.',
                'rbac.summary.bindings_description_selected': `Elevated bindings currently attached to ${options?.user ?? ''}.`,
                'rbac.summary.permissions_title': 'Permission keys',
                'rbac.summary.permissions_description': 'Permission definitions available when composing roles.',
                'rbac.search_placeholder': 'Search roles, bindings, or permissions',
                'rbac.search_help': 'Quick search filters the role catalog, elevated bindings, and permission directory on this page.',
                'rbac.advanced_search_title': 'Advanced search',
                'rbac.advanced_search_help': 'Choose exact RBAC filters here.',
                'rbac.roles.permissions': 'Permissions',
                'rbac.roles.built_in': 'Built In',
                'rbac.boolean.yes': 'Yes',
                'rbac.boolean.no': 'No',
                'common:status.active': 'Active',
                'common:status.disabled': 'Disabled',
                'common:button.refresh': 'Refresh',
                'common:button.edit': 'Edit',
                'common:button.delete': 'Delete',
                'common:button.search': 'Search',
                'common:button.confirm': 'Confirm',
                'common:button.cancel': 'Cancel',
                'common:message.loading': 'Loading...',
                'common:message.no_data': 'No data',
                'common:table.name': 'Name',
                'common:table.description': 'Description',
                'common:table.created_at': 'Created',
                'common:table.status': 'Status',
                'common:table.actions': 'Actions',
            };
            return labels[key] ?? options?.defaultValue ?? key;
        },
    }),
}));

vi.mock('../hooks/useAdminRbacController', () => ({
    useAdminRbacController: () => {
        const [roleCreateForm] = Form.useForm();
        const [roleEditForm] = Form.useForm();
        const [bindingForm] = Form.useForm();

        return {
            messageContextHolder: null,
            roles: [
                {
                    id: 'role-1',
                    name: 'PlatformAdmin',
                    display_name: 'Platform Admin',
                    description: 'Admin role',
                    permissions: ['vm:create'],
                    built_in: true,
                    enabled: true,
                },
                {
                    id: 'role-2',
                    name: 'Viewer',
                    display_name: 'Viewer',
                    description: 'Read only',
                    permissions: ['vm:read'],
                    built_in: true,
                    enabled: true,
                },
            ],
            rolesLoading: false,
            permissions: [
                { key: 'vm:create', description: 'Create VMs' },
                { key: 'vm:read', description: 'Read VMs' },
            ],
            permissionsLoading: false,
            roleBindings: [{
                id: 'binding-1',
                role_id: 'role-1',
                role_name: 'PlatformAdmin',
                role_display_name: 'Platform Admin',
                scope_type: 'global',
                scope_display_name: 'All platform resources',
                allowed_environments: ['test'],
                created_at: '2026-03-18T00:00:00Z',
            }],
            roleBindingsLoading: false,
            userOptions: [{ value: 'user-1', label: 'Alice (alice)' }],
            users: [{ id: 'user-1', username: 'alice', display_name: 'Alice' }],
            usersLoading: false,
            selectedUserId: '',
            selectedUser: null,
            selectedUserDisplayLabel: '',
            selectedUserValue: undefined,
            userSearch: '',
            permissionOptions: [{ label: 'vm:create', value: 'vm:create' }],
            refetchRoles: vi.fn(),
            refetchPermissions: vi.fn(),
            refetchUsers: vi.fn(),
            refetchRoleBindings: vi.fn(),
            openCreateRoleModal: vi.fn(),
            openEditRoleModal: vi.fn(),
            openDeleteRoleModal: vi.fn(),
            openAddBindingModal: vi.fn(),
            selectUser: selectUserMock,
            setUserSearch: vi.fn(),
            deleteRoleBinding: vi.fn(),
            createRoleOpen: false,
            editRoleOpen: false,
            deleteRoleOpen: false,
            addBindingOpen: false,
            createRolePending: false,
            updateRolePending: false,
            deleteRolePending: false,
            createBindingPending: false,
            deleteBindingPending: false,
            deletingBindingId: '',
            deletingRole: null,
            editingRole: null,
            roleCreateForm,
            roleEditForm,
            bindingForm,
            closeCreateRoleModal: vi.fn(),
            closeEditRoleModal: vi.fn(),
            closeDeleteRoleModal: vi.fn(),
            closeAddBindingModal: vi.fn(),
            submitCreateRole: vi.fn(),
            submitEditRole: vi.fn(),
            submitDeleteRole: vi.fn(),
            submitAddBinding: vi.fn(),
            scopeTargetOptionsByType: {},
            scopeTargetLoadingByType: {},
        };
    },
}));

import { AdminRbacContent } from './AdminRbacContent';

describe('AdminRbacContent', () => {
    beforeEach(() => {
        searchParamsState.delete('user_id');
        searchParamsState.delete('user_label');
        selectUserMock.mockReset();
    });

    it('renders the page shell and core access-management surfaces', () => {
        render(<AdminRbacContent />);

        expect(screen.getByTestId('admin-rbac-page')).toBeVisible();
        expect(screen.getByText('Access & Roles')).toBeVisible();
        expect(screen.getByTestId('rbac-role-create-button')).toBeVisible();
        expect(screen.getByTestId('rbac-binding-create-button')).toBeVisible();
        expect(screen.getAllByText('Platform Admin').length).toBeGreaterThan(0);
        expect(screen.getByTestId('rbac-user-selector')).toBeVisible();
        expect(screen.getByText('Please select a user first')).toBeVisible();
    });

    it('preselects a user from query parameters', async () => {
        searchParamsState.set('user_id', 'user-1');
        searchParamsState.set('user_label', 'Alice');

        render(<AdminRbacContent />);

        await waitFor(() => {
            expect(selectUserMock).toHaveBeenCalledWith('user-1', 'Alice');
        });
    });

    it('filters the role and permission tables with the shared quick search', async () => {
        render(<AdminRbacContent />);

        fireEvent.change(screen.getByTestId('rbac-quick-search'), {
            target: { value: 'viewer' },
        });
        await userEvent.setup().click(screen.getAllByRole('button', { name: /search/i })[0]);

        expect(screen.getAllByText('Viewer').length).toBeGreaterThan(0);
        expect(screen.queryByText('Platform Admin')).not.toBeInTheDocument();
    });
});
