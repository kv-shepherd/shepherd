import { Form } from 'antd';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { useState } from 'react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

const setPageMock = vi.fn();
const setSearchMock = vi.fn();
const savePreferenceMock = vi.fn();
const resetPreferenceMock = vi.fn();
let userPreferenceState:
    | {
        columns?: string[];
        merged_columns?: Array<{
            label?: string;
            column_keys?: string[];
            show_labels?: boolean;
        }>;
    }
    | undefined;

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
                'users.directory.merged_column_default_label': 'Combined details',
                'users.directory.columns_merge_add': 'Add combined column',
                'users.directory.columns_merge_remove': 'Remove',
                'users.directory.columns_merge_group_title': 'Combined column',
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
                        profile_attributes: {
                            department: 'Engineering',
                        },
                        roles: ['platform-admin'],
                        enabled: true,
                        created_at: '2026-03-17T00:00:00Z',
                    },
                ],
                profile_fields: [
                    {
                        key: 'department',
                        label: 'Department',
                        searchable: true,
                    },
                    {
                        key: 'section',
                        label: 'Section',
                        searchable: true,
                    },
                ],
                pagination: { total: 1, page: 1, per_page: 20 },
            },
            usersLoading: false,
            page: 1,
            perPage: 20,
            search: '',
            setPage: setPageMock,
            setPerPage: vi.fn(),
            setSearch: setSearchMock,
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

vi.mock('@/hooks/useUserPreference', () => ({
    useUserPreference: function useUserPreferenceMock() {
        const [value, setValue] = useState(userPreferenceState);
        return {
            exists: Boolean(value),
            value,
            savePreference: async (payload: { value: typeof userPreferenceState }) => {
                savePreferenceMock(payload);
                userPreferenceState = payload.value;
                setValue(payload.value);
                return {
                    key: 'admin.users.columns.v1',
                    updated_at: '2026-03-31T00:00:00Z',
                    value: payload.value,
                };
            },
            resetPreference: async () => {
                resetPreferenceMock();
                userPreferenceState = undefined;
                setValue(undefined);
            },
            savePending: false,
            resetPending: false,
        };
    },
}));

import { AdminUsersContent } from './AdminUsersContent';

describe('AdminUsersContent', () => {
    beforeEach(() => {
        savePreferenceMock.mockReset();
        resetPreferenceMock.mockReset();
        userPreferenceState = undefined;
    });

    afterEach(() => {
        savePreferenceMock.mockReset();
        resetPreferenceMock.mockReset();
        userPreferenceState = undefined;
    });

    it('shows default profile columns while exposing an advanced search builder', () => {
        setPageMock.mockReset();
        setSearchMock.mockReset();

        render(<AdminUsersContent />);

        expect(screen.getAllByText('Department').length).toBeGreaterThan(0);
        expect(screen.getAllByText('Section').length).toBeGreaterThan(0);

        fireEvent.click(screen.getByTestId('users-directory-advanced-search-toggle'));
        expect(screen.getByTestId('users-directory-search-condition-row-0')).toBeVisible();
        expect(screen.getByTestId('users-directory-search-condition-field-0')).toBeVisible();
        expect(screen.getByTestId('users-directory-search-condition-value-0')).toBeVisible();
        expect(screen.getByTestId('users-directory-advanced-search-toggle')).toBeVisible();
        expect(screen.getByTestId('users-directory-add-search-condition')).toBeVisible();
    }, 180000);

    it('renders the page shell and the three primary user management sections', () => {
        setPageMock.mockReset();
        setSearchMock.mockReset();

        render(<AdminUsersContent />);

        expect(screen.getByTestId('admin-users-page')).toBeVisible();
        expect(screen.getByText('Users')).toBeVisible();
        expect(screen.getByTestId('user-create-button')).toBeVisible();
        expect(screen.getByTestId('users-directory-search')).toBeVisible();
        expect(screen.getByTestId('users-directory-open-columns-drawer')).toBeVisible();
        expect(screen.getByTestId('member-add-button')).toBeVisible();
        expect(screen.getByTestId('rate-limit-exemption-create-button')).toBeVisible();
        expect(screen.getByText('Alice')).toBeVisible();
        expect(screen.getAllByText('Email').length).toBeGreaterThan(0);
        expect(screen.getByText('alice@example.com')).toBeVisible();
        expect(screen.getAllByText('Department').length).toBeGreaterThan(0);
        expect(screen.getByText('Engineering')).toBeVisible();
        expect(screen.getAllByText('Section').length).toBeGreaterThan(0);
        expect(screen.getByText('Manage Access')).toBeVisible();
        expect(screen.getByTestId('users-system-selector')).toBeVisible();
    }, 20000);

    it('opens the column drawer and saves the customized order', () => {
        render(<AdminUsersContent />);

        fireEvent.click(screen.getByTestId('users-directory-open-columns-drawer'));
        expect(screen.getByText('Customize displayed columns')).toBeVisible();

        fireEvent.click(screen.getAllByLabelText('Hide column')[0]);
        fireEvent.click(screen.getByTestId('users-directory-columns-save'));

        expect(savePreferenceMock).toHaveBeenCalledTimes(1);
        expect(savePreferenceMock).toHaveBeenCalledWith({
            value: {
                columns: ['profile:section', 'email', 'roles', 'status', 'created_at'],
                merged_columns: [],
            },
        });
    });

    it('applies the saved column selection immediately to the table', async () => {
        render(<AdminUsersContent />);

        expect(screen.getAllByText('Department').length).toBeGreaterThan(0);
        expect(screen.getAllByText('Section').length).toBeGreaterThan(0);

        fireEvent.click(screen.getByTestId('users-directory-open-columns-drawer'));
        fireEvent.click(screen.getAllByLabelText('Hide column')[0]);
        fireEvent.click(screen.getByTestId('users-directory-columns-save'));

        expect(savePreferenceMock).toHaveBeenCalledTimes(1);
        await waitFor(() => {
            expect(screen.queryAllByText('Department')).toHaveLength(0);
        });
        expect(screen.getAllByText('Section').length).toBeGreaterThan(0);
    });

    it('renders selected columns inside a custom merged column', () => {
        userPreferenceState = {
            columns: ['email', 'profile:department', 'status', 'created_at', 'roles'],
            merged_columns: [
                {
                    column_keys: ['profile:department', 'status'],
                    label: 'Overview',
                },
            ],
        };

        render(<AdminUsersContent />);

        const headerTexts = screen.getAllByRole('columnheader').map((header) => header.textContent ?? '');
        expect(headerTexts).toContain('Overview');
        expect(headerTexts).not.toContain('Department');
        expect(headerTexts).not.toContain('Status');
        expect(screen.getByText('Engineering')).toBeVisible();
        expect(screen.getByText('Enabled')).toBeVisible();
    }, 30000);

    it('renders multiple custom merged columns in display order', () => {
        userPreferenceState = {
            columns: ['email', 'profile:department', 'status', 'profile:section', 'created_at', 'roles'],
            merged_columns: [
                {
                    column_keys: ['profile:department', 'profile:section'],
                    label: 'Organization',
                },
                {
                    column_keys: ['status', 'created_at'],
                    label: 'Lifecycle',
                },
            ],
        };

        render(<AdminUsersContent />);

        const headerTexts = screen.getAllByRole('columnheader').map((header) => header.textContent ?? '');
        expect(headerTexts).toContain('Organization');
        expect(headerTexts).toContain('Lifecycle');
        expect(headerTexts).not.toContain('Department');
        expect(headerTexts).not.toContain('Section');
        expect(headerTexts).not.toContain('Status');
        expect(headerTexts).not.toContain('Created');
        expect(screen.getByText('Engineering')).toBeVisible();
        expect(screen.getByText('Enabled')).toBeVisible();
    }, 30000);
});
