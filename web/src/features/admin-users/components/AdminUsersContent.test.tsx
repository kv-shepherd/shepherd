import { Form } from 'antd';
import { render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

vi.mock('react-i18next', () => ({
    useTranslation: () => ({
        t: (key: string, options?: { defaultValue?: string }) => {
            const labels: Record<string, string> = {
                'users.title': 'Users',
                'users.subtitle': 'Manage platform users and memberships',
                'users.directory.title': 'User Directory',
                'users.directory.subtitle': 'Create and manage accounts',
                'users.directory.add': 'Add User',
                'users.directory.manage_access': 'Manage Access',
                'users.directory.role_bindings': 'Role Bindings',
                'users.members.title': 'System Members',
                'users.members.subtitle': 'Manage per-system membership',
                'users.members.add': 'Add Member',
                'users.members.select_system': 'Select System',
                'users.members.select_system_placeholder': 'Select a system',
                'users.members.select_system_first': 'Select system first',
                'users.rate_limit.title': 'Rate Limits',
                'users.rate_limit.subtitle': 'Manage user-specific rate limits',
                'users.rate_limit.add_exemption': 'Add Exemption',
                'users.rate_limit.user': 'User',
                'users.rate_limit.user_id': 'User ID',
                'users.rate_limit.exempted': 'Exempted',
                'users.rate_limit.exempted_yes': 'Yes',
                'users.rate_limit.exempted_no': 'No',
                'users.rate_limit.effective': 'Effective',
                'users.rate_limit.current': 'Current',
                'users.rate_limit.max_parents': 'Max Parents',
                'users.rate_limit.max_children': 'Max Children',
                'users.rate_limit.cooldown': 'Cooldown',
                'users.rate_limit.pending_parents': 'Pending Parents',
                'users.rate_limit.pending_children': 'Pending Children',
                'users.rate_limit.remaining': 'Remaining',
                'users.table.username': 'Username',
                'users.table.email': 'Email',
                'users.table.roles': 'Roles',
                'users.summary.directory_title': 'User Directory',
                'users.status.enabled': 'Enabled',
                'users.status.disabled': 'Disabled',
                'common:table.status': 'Status',
                'common:table.created_at': 'Created',
                'common:table.actions': 'Actions',
                'common:button.refresh': 'Refresh',
                'common:button.edit': 'Edit',
                'common:button.delete': 'Delete',
                'common:button.confirm': 'Confirm',
                'common:button.cancel': 'Cancel',
                'common:table.total': 'Total',
                'common:message.no_data': 'No data',
            };
            return labels[key] ?? options?.defaultValue ?? key;
        },
    }),
}));

vi.mock('../hooks/useAdminUsersController', () => ({
    useAdminUsersController: () => {
        const [createUserForm] = Form.useForm();
        const [editUserForm] = Form.useForm();
        const [addForm] = Form.useForm();
        const [roleBindingCreateForm] = Form.useForm();

        return {
            messageContextHolder: null,
            users: {
                items: [
                    {
                        id: 'user-1',
                        username: 'alice',
                        display_name: 'Alice',
                        email: 'alice@example.com',
                        roles: ['platform-admin'],
                        enabled: true,
                        created_at: '2026-03-17T00:00:00Z',
                    },
                ],
                pagination: { total: 1, page: 1, per_page: 20 },
            },
            usersLoading: false,
            page: 1,
            perPage: 20,
            setPage: vi.fn(),
            setPerPage: vi.fn(),
            refetchUsers: vi.fn(),
            openCreateUserModal: vi.fn(),
            openEditUserModal: vi.fn(),
            openRoleBindingsModal: vi.fn(),
            deleteUser: vi.fn(),
            deleteUserPending: false,
            deletingUserId: '',
            createUserOpen: false,
            editUserOpen: false,
            createUserPending: false,
            updateUserPending: false,
            createUserForm,
            editUserForm,
            submitCreateUser: vi.fn(),
            submitEditUser: vi.fn(),
            closeCreateUserModal: vi.fn(),
            closeEditUserModal: vi.fn(),
            editingUserId: '',
            members: { items: [], pagination: { total: 0, page: 1, per_page: 20 } },
            membersLoading: false,
            selectedSystemId: '',
            systems: [{ id: 'sys-1', name: 'System A' }],
            systemsLoading: false,
            refetchMembers: vi.fn(),
            memberCandidates: { items: [], pagination: { total: 0, page: 1, per_page: 50 } },
            memberCandidatesLoading: false,
            refetchMemberCandidates: vi.fn(),
            memberCandidateSearch: '',
            setMemberCandidateSearch: vi.fn(),
            openAddModal: vi.fn(),
            addOpen: false,
            addPending: false,
            addForm,
            submitAddMember: vi.fn(),
            closeAddModal: vi.fn(),
            setSelectedSystemId: vi.fn(),
            updateMemberRole: vi.fn(),
            updatePending: false,
            removeMember: vi.fn(),
            removePending: false,
            rateLimitStatus: { items: [], pagination: { total: 0, page: 1, per_page: 20 } },
            rateLimitLoading: false,
            refetchRateLimitStatus: vi.fn(),
            rateLimitMutationPending: false,
            applyRateLimitExemption: vi.fn(),
            updateRateLimitOverride: vi.fn(),
            removeRateLimitExemption: vi.fn(),
            roleBindingsUserId: '',
            roleBindingsUserLabel: '',
            roleBindingsLoading: false,
            roleBindings: { items: [], pagination: { total: 0, page: 1, per_page: 20 } },
            openRoleBindingCreateModal: vi.fn(),
            closeRoleBindingsModal: vi.fn(),
            roleBindingCreateOpen: false,
            createRoleBindingPending: false,
            roleBindingCreateForm,
            submitCreateRoleBinding: vi.fn(),
            closeRoleBindingCreateModal: vi.fn(),
            rolesLoading: false,
            roles: { items: [{ id: 'role-1', name: 'platform-admin' }] },
            scopeTargetOptionsByType: {},
            scopeTargetLoadingByType: {},
            deletingBindingId: '',
            deleteRoleBindingPending: false,
            deleteRoleBinding: vi.fn(),
        };
    },
}));

import { AdminUsersContent } from './AdminUsersContent';

describe('AdminUsersContent', () => {
    it('renders the page shell and the three primary user management sections', () => {
        render(<AdminUsersContent />);

        expect(screen.getByTestId('admin-users-page')).toBeVisible();
        expect(screen.getByText('Users')).toBeVisible();
        expect(screen.getByTestId('user-create-button')).toBeVisible();
        expect(screen.getByTestId('member-add-button')).toBeVisible();
        expect(screen.getByTestId('rate-limit-exemption-create-button')).toBeVisible();
        expect(screen.getByText('Alice')).toBeVisible();
        expect(screen.getByText('Manage Access')).toBeVisible();
        expect(screen.getByTestId('users-system-selector')).toBeVisible();
    }, 20000);
});
