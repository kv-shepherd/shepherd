import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';

vi.mock('react-i18next', () => ({
    useTranslation: () => ({
        t: (key: string, options?: { defaultValue?: string }) => {
            const labels: Record<string, string> = {
                'rbac.title': 'RBAC',
                'rbac.subtitle': 'Manage roles and bindings',
                'rbac.roles.title': 'Roles',
                'rbac.roles.subtitle': 'Manage role definitions',
                'rbac.roles.add': 'Add Role',
                'rbac.bindings.title': 'Bindings',
                'rbac.bindings.subtitle': 'Assign roles to users',
                'rbac.bindings.add': 'Add Binding',
                'rbac.bindings.scope': 'Scope',
                'rbac.bindings.all_environments': 'All environments',
                'rbac.bindings.select_user': 'Select User',
                'rbac.permissions.title': 'Permissions',
                'rbac.permissions.subtitle': 'Reference available permissions',
                'rbac.summary.roles_title': 'Role Catalog',
                'common:button.refresh': 'Refresh',
                'common:button.edit': 'Edit',
                'common:button.delete': 'Delete',
                'common:table.name': 'Name',
                'common:table.description': 'Description',
                'common:table.created_at': 'Created',
                'common:table.status': 'Status',
                'common:table.actions': 'Actions',
                'rbac.roles.permissions': 'Permissions',
                'rbac.roles.built_in': 'Built In',
                'rbac.boolean.yes': 'Yes',
                'rbac.boolean.no': 'No',
                'common:status.active': 'Active',
                'common:status.disabled': 'Disabled',
                'rbac.bindings.role': 'Role',
                'rbac.bindings.scope_type': 'Scope Type',
                'rbac.bindings.scope_id': 'Scope ID',
                'rbac.bindings.allowed_envs': 'Allowed Environments',
                'rbac.bindings.select_user_placeholder': 'Select a user',
                'rbac.bindings.select_user_first': 'Select user first',
            };
            return labels[key] ?? options?.defaultValue ?? key;
        },
    }),
}));

vi.mock('../hooks/useAdminRbacController', () => ({
    useAdminRbacController: () => ({
        messageContextHolder: null,
        roles: [
            {
                id: 'role-1',
                name: 'platform-admin',
                display_name: 'Platform Admin',
                description: 'Admin role',
                permissions: ['vm:create'],
                built_in: false,
                enabled: true,
            },
            {
                id: 'role-2',
                name: 'viewer',
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
            role_name: 'platform-admin',
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
        selectedUserId: 'user-1',
        selectedUser: null,
        selectedUserDisplayLabel: 'Alice',
        selectedUserValue: { value: 'user-1', label: 'Alice (alice)' },
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
        selectUser: vi.fn(),
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
        roleCreateForm: undefined,
        roleEditForm: undefined,
        bindingForm: undefined,
        closeCreateRoleModal: vi.fn(),
        closeEditRoleModal: vi.fn(),
        closeDeleteRoleModal: vi.fn(),
        closeAddBindingModal: vi.fn(),
        submitCreateRole: vi.fn(),
        submitEditRole: vi.fn(),
        submitDeleteRole: vi.fn(),
        submitAddBinding: vi.fn(),
    }),
}));

import { AdminRbacContent } from './AdminRbacContent';

describe('AdminRbacContent', () => {
    it('renders the page shell and core RBAC surfaces', () => {
        render(<AdminRbacContent />);

        expect(screen.getByTestId('admin-rbac-page')).toBeVisible();
        expect(screen.getByText('RBAC')).toBeVisible();
        expect(screen.getByTestId('rbac-role-create-button')).toBeVisible();
        expect(screen.getByTestId('rbac-binding-create-button')).toBeVisible();
        expect(screen.getAllByText('Platform Admin').length).toBeGreaterThan(0);
        expect(screen.getByTestId('rbac-user-selector')).toBeVisible();
        expect(screen.getByText('All platform resources')).toBeVisible();
    });

    it('filters the role and permission tables with the shared quick search', async () => {
        const user = userEvent.setup();
        render(<AdminRbacContent />);

        await user.type(screen.getByTestId('rbac-quick-search'), 'viewer');
        await user.click(screen.getAllByRole('button', { name: /search/i })[0]);

        expect(screen.getAllByText('Viewer').length).toBeGreaterThan(0);
        expect(screen.queryByText('Platform Admin')).not.toBeInTheDocument();
    });
});
