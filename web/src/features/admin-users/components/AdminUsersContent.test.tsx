import { Form } from 'antd';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import type { ReactNode } from 'react';
import { useState } from 'react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

const pushMock = vi.fn();
const setPageMock = vi.fn();
const setPerPageMock = vi.fn();
const setSearchMock = vi.fn();
const refetchUsersMock = vi.fn();
const openCreateUserModalMock = vi.fn();
const openEditUserModalMock = vi.fn();
const deleteUserMock = vi.fn();
const savePreferenceMock = vi.fn();
const resetPreferenceMock = vi.fn();
const openAddBindingModalMock = vi.fn();
const closeAddBindingModalMock = vi.fn();
const submitAddBindingMock = vi.fn();
const deleteRoleBindingMock = vi.fn();
const deleteRoleBindingsMock = vi.fn();
const resetRoleBindingsForUsersMock = vi.fn();
const authState = {
    user: {
        id: 'admin-1',
        username: 'admin',
        permissions: ['user:manage', 'rbac:manage'],
    },
};

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

vi.mock('next/navigation', () => ({
    useRouter: () => ({ push: pushMock }),
}));

vi.mock('@/stores/auth', () => ({
    useAuthStore: (selector: (state: typeof authState) => unknown) => selector(authState),
}));

vi.mock('react-i18next', () => ({
    useTranslation: () => ({
        t: (key: string, options?: { defaultValue?: string; [key: string]: unknown }) => {
            const labels: Record<string, string> = {
                'users.title': 'User Management',
                'users.subtitle': 'Manage platform accounts and per-user access bindings here. Systems and Rate Limits remain in their dedicated pages, while Access & Roles stays available for role catalog and elevated-access audit views.',
                'users.directory.title': 'User Directory',
                'users.directory.subtitle': 'Create and maintain platform accounts, and grant explicit access bindings here. System members and rate limits stay in their dedicated pages, while Access & Roles remains available for role catalog and elevated-access audit views.',
                'users.directory.add': 'Add User',
                'users.directory.add_title': 'Create User',
                'users.directory.edit_title': `Edit User: ${options?.username ?? ''}`,
                'users.directory.password': 'Reset Password',
                'users.directory.force_password_change': 'Force password change at next login',
                'users.directory.manage_access': 'Access Bindings',
                'users.directory.manage_access_title': `Access bindings: ${options?.user ?? ''}`,
                'users.directory.manage_access_help_title': 'Manage explicit access here',
                'users.directory.manage_access_help_description': 'Grant both runtime and elevated roles from this drawer. Elevated platform-facing roles are highlighted with stronger colors so administrators can spot them quickly before saving.',
                'users.directory.manage_access_bindings_title': 'Current bindings',
                'users.directory.manage_access_bindings_subtitle': 'Review the exact roles this user currently holds. Blue tags indicate runtime access, while purple tags indicate elevated platform-facing access.',
                'users.directory.manage_access_bindings_selected': `Selected ${options?.count ?? ''} bindings`,
                'users.directory.manage_access_empty': 'No explicit bindings for this user',
                'users.directory.manage_access_empty_description': 'Add the first role binding here and keep the scope and environments as narrow as practical.',
                'users.directory.manage_access_footer': 'Use role, scope, and allowed environments together to keep access narrow. Elevated platform-facing roles stay highlighted here, while Access & Roles remains available for cross-user audit.',
                'users.directory.add_binding_title': `Add binding: ${options?.user ?? ''}`,
                'users.directory.role_placeholder': 'Search roles',
                'users.directory.manage_access_summary.standard_title': 'Runtime bindings',
                'users.directory.manage_access_summary.standard_description': 'System, service, and virtual-machine access currently assigned on this user.',
                'users.directory.manage_access_summary.privileged_title': 'Elevated bindings',
                'users.directory.manage_access_summary.privileged_description': 'Platform administration or approval-sensitive access highlighted for extra review.',
                'users.directory.batch_manage_access': 'Batch access',
                'users.directory.batch_manage_access_help': 'Search by name, email, department, section, or job title, then select multiple users at once.',
                'users.directory.selected_users': `Selected ${options?.count ?? ''} users`,
                'users.directory.batch_delete_bindings': 'Remove selected',
                'users.directory.batch_reset_access': 'Reset access',
                'users.directory.select_users_placeholder': 'Search and select one or more users',
                'users.directory.no_matching_users': 'No matching users',
                'users.directory.open_rbac': 'Open Access & Roles',
                'users.directory.open_systems': 'Open Systems',
                'users.directory.open_rate_limits': 'Open Rate Limits',
                'users.directory.quick_search_placeholder': 'Quick search users, email, or directory fields',
                'users.directory.quick_search_help': 'Press Enter or click Search.',
                'users.directory.show_advanced_search': 'Advanced Search',
                'users.directory.hide_advanced_search': 'Hide Advanced Search',
                'users.directory.clear_search': 'Clear Search',
                'users.directory.advanced_search_title': 'Advanced Search',
                'users.directory.advanced_search_help': 'Choose one or more fields.',
                'users.directory.advanced_search_field': 'Search field',
                'users.directory.advanced_search_value': 'Search value',
                'users.directory.add_search_condition': 'Add Condition',
                'users.directory.remove_search_condition': 'Remove search condition',
                'users.directory.visible_columns_placeholder': 'Directory display config',
                'users.directory.columns_drawer_title': 'Customize directory display',
                'users.directory.columns_drawer_message': 'Account and Actions stay fixed.',
                'users.directory.columns_visible_title': 'Visible columns',
                'users.directory.columns_empty': 'No extra columns selected.',
                'users.directory.columns_add_title': 'Add column',
                'users.directory.columns_add_placeholder': 'Choose another column to show',
                'users.directory.columns_merge_title': 'Combined columns',
                'users.directory.columns_merge_help': 'Create one or more combined columns from the currently visible columns.',
                'users.directory.columns_merge_placeholder': 'Select columns to combine',
                'users.directory.columns_merge_label_placeholder': 'Name this combined column',
                'users.directory.columns_merge_group_title': 'Combined column',
                'users.directory.columns_merge_add': 'Add combined column',
                'users.directory.columns_merge_remove': 'Remove',
                'users.directory.columns_merge_show_labels_title': 'Show field labels inside the column',
                'users.directory.columns_merge_show_labels_help': 'Turn this off for a cleaner stacked value view.',
                'users.directory.merged_column_default_label': 'Combined details',
                'users.directory.columns_restore_defaults': 'Restore recommended display config',
                'users.directory.reset_columns': 'Reset config',
                'users.directory.move_column_up': 'Move column up',
                'users.directory.move_column_down': 'Move column down',
                'users.directory.hide_column': 'Hide column',
                'users.directory.no_roles': 'No global roles',
                'users.directory.empty': 'No users yet',
                'users.directory.empty_description': 'Create the first platform account before assigning explicit access, system membership, or rate-limit policy from their dedicated pages.',
                'users.directory.delete_confirm': `Delete user ${options?.username ?? ''}?`,
                'users.search.field.username': 'Username',
                'users.search.field.display_name': 'Display Name',
                'users.search.field.email': 'Email',
                'users.search.field.role': 'Role',
                'users.search.field.status': 'Status',
                'users.profile_fields.department': 'Department',
                'users.profile_fields.section': 'Section',
                'users.table.account': 'Account',
                'users.table.email': 'Email',
                'users.table.roles': 'Roles',
                'users.status.enabled': 'Enabled',
                'users.status.disabled': 'Disabled',
                'users.summary.directory_title': 'User directory',
                'users.summary.directory_description': 'Total user accounts currently available in the platform.',
                'users.summary.enabled_title': 'Enabled on this page',
                'users.summary.enabled_description': 'Accounts on the current page that can sign in right now.',
                'users.summary.roles_title': 'Users with explicit roles',
                'users.summary.roles_description': 'Accounts on this page that currently hold at least one explicit global role binding.',
                'users.summary.profile_title': 'Directory-backed users',
                'users.summary.profile_description': 'Accounts on this page that currently expose synced or projected directory profile fields.',
                'common:table.status': 'Status',
                'common:table.created_at': 'Created',
                'common:table.display_name': 'Display Name',
                'common:table.total': `Total ${options?.total ?? ''} items`,
                'common:table.actions': 'Actions',
                'common:button.search': 'Search',
                'common:button.clear': 'Clear',
                'common:button.save': 'Save',
                'common:button.cancel': 'Cancel',
                'common:button.close': 'Close',
                'common:button.confirm': 'Confirm',
                'common:button.refresh': 'Refresh',
                'common:button.edit': 'Edit',
                'common:button.delete': 'Delete',
                'common:message.no_data': 'No data',
                'common:message.success': 'Success',
                'common:auth.username': 'Username',
                'common:auth.password': 'Password',
                'common:validation.username_required': 'Username is required',
                'common:validation.username_min': 'Username must be at least 2 characters',
                'common:validation.password_required': 'Password is required',
                'common:validation.password_min': 'Password must be at least 8 characters',
                'rbac.bindings.add': 'Add Binding',
                'rbac.bindings.role': 'Role',
                'rbac.bindings.scope': 'Scope',
                'rbac.bindings.select_users': 'Users',
                'rbac.bindings.scope_type': 'Scope type',
                'rbac.bindings.scope_id': 'Scope ID',
                'rbac.bindings.scope_id_help': 'Choose a known target',
                'rbac.bindings.scope_id_placeholder': 'Paste the target resource ID',
                'rbac.bindings.scope_target_empty': 'No suggested targets yet',
                'rbac.bindings.allowed_envs': 'Allowed environments',
                'rbac.bindings.allowed_envs_help': 'This is the real test/prod boundary',
                'rbac.bindings.all_environments': 'All environments',
                'rbac.bindings.global_scope': 'All platform resources',
                'rbac.bindings.delete_confirm': 'Delete this role binding?',
                'rbac.bindings.role_policy_title': 'Typical assignment guidance',
                'rbac.env.test': 'Test',
                'rbac.env.prod': 'Prod',
                'rbac.scope.global': 'Global',
                'rbac.scope.system': 'System',
                'rbac.scope.service': 'Service',
                'rbac.scope.vm': 'VM',
            };
            return labels[key] ?? options?.defaultValue ?? key;
        },
    }),
}));

vi.mock('antd', async (importOriginal) => {
    const actual = await importOriginal<typeof import('antd')>();

    return {
        ...actual,
        Card: ({
            title,
            extra,
            children,
        }: {
            title?: ReactNode;
            extra?: ReactNode;
            children?: ReactNode;
        }) => (
            <section data-testid="antd-card">
                {title ? <header>{title}</header> : null}
                {extra ? <div>{extra}</div> : null}
                <div>{children}</div>
            </section>
        ),
        Drawer: ({
            open,
            title,
            children,
            footer,
        }: {
            open?: boolean;
            title?: ReactNode;
            children?: ReactNode;
            footer?: ReactNode;
        }) =>
            open ? (
                <section data-testid="antd-drawer">
                    {title ? <header>{title}</header> : null}
                    <div>{children}</div>
                    {footer ? <footer>{footer}</footer> : null}
                </section>
            ) : null,
        Table: ({
            columns = [],
            dataSource = [],
        }: {
            columns?: Array<{
                key?: string;
                title?: ReactNode;
                dataIndex?: string | string[];
                render?: (value: unknown, record: Record<string, unknown>, index: number) => ReactNode;
            }>;
            dataSource?: Array<Record<string, unknown>>;
        }) => (
            <table data-testid="antd-table">
                <thead>
                    <tr>
                        {columns.map((column, index) => (
                            <th key={String(column.key ?? column.dataIndex ?? index)}>{column.title}</th>
                        ))}
                    </tr>
                </thead>
                <tbody>
                    {dataSource.map((record, rowIndex) => (
                        <tr key={String(record.id ?? rowIndex)}>
                            {columns.map((column, columnIndex) => {
                                const value = Array.isArray(column.dataIndex)
                                    ? column.dataIndex.reduce<unknown>(
                                        (current, key) =>
                                            current && typeof current === 'object'
                                                ? (current as Record<string, unknown>)[key]
                                                : undefined,
                                        record,
                                    )
                                    : typeof column.dataIndex === 'string'
                                        ? record[column.dataIndex]
                                        : undefined;
                                const content = column.render
                                    ? column.render(value, record, rowIndex)
                                    : (value as ReactNode);
                                return (
                                    <td key={String(column.key ?? column.dataIndex ?? columnIndex)}>
                                        {content}
                                    </td>
                                );
                            })}
                        </tr>
                    ))}
                </tbody>
            </table>
        ),
    };
});

vi.mock('@/components/feedback/ActionEmptyState', () => ({
    ActionEmptyState: ({
        title,
        description,
        actions,
    }: {
        title: string;
        description?: string;
        actions?: ReactNode;
    }) => (
        <section data-testid="action-empty-state">
            <h2>{title}</h2>
            {description ? <p>{description}</p> : null}
            {actions}
        </section>
    ),
}));

vi.mock('@/components/feedback/SummaryMetricCard', () => ({
    SummaryMetricCard: ({
        title,
        value,
        description,
        action,
    }: {
        title: ReactNode;
        value?: ReactNode;
        description?: ReactNode;
        action?: ReactNode;
    }) => (
        <section data-testid="summary-metric-card">
            <h2>{title}</h2>
            {value ? <div>{value}</div> : null}
            {description ? <div>{description}</div> : null}
            {action}
        </section>
    ),
}));

vi.mock('@/components/layouts/PageSection', () => ({
    PageHeader: ({
        title,
        subtitle,
        actions,
    }: {
        title: ReactNode;
        subtitle?: ReactNode;
        actions?: ReactNode;
    }) => (
        <header data-testid="page-header">
            <h1>{title}</h1>
            {subtitle ? <p>{subtitle}</p> : null}
            {actions}
        </header>
    ),
    PageSurface: ({
        children,
    }: {
        children: ReactNode;
    }) => <section data-testid="page-surface">{children}</section>,
}));

vi.mock('@/components/ui/LocalDateTimeText', () => ({
    LocalDateTimeText: ({ value }: { value: string }) => <time dateTime={value}>{value}</time>,
}));

vi.mock('@/components/illustrations/DashboardIllustrations', () => ({
    AccessControlGlyph: (props: Record<string, unknown>) => <span {...props}>access-glyph</span>,
    QueueReviewGlyph: (props: Record<string, unknown>) => <span {...props}>queue-glyph</span>,
    RoleCatalogGlyph: (props: Record<string, unknown>) => <span {...props}>role-glyph</span>,
    UserDirectoryGlyph: (props: Record<string, unknown>) => <span {...props}>directory-glyph</span>,
}));

vi.mock('../hooks/useAdminUsersController', () => ({
    useAdminUsersController: () => {
        const [createUserForm] = Form.useForm();
        const [editUserForm] = Form.useForm();

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
                            section: 'Platform',
                        },
                        roles: ['DevelopmentEngineer', 'PlatformAdmin'],
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
            setPerPage: setPerPageMock,
            setSearch: setSearchMock,
            refetchUsers: refetchUsersMock,
            createUserOpen: false,
            editUserOpen: false,
            editingUserId: '',
            deletingUserId: '',
            createUserForm,
            editUserForm,
            openCreateUserModal: openCreateUserModalMock,
            closeCreateUserModal: vi.fn(),
            submitCreateUser: vi.fn(),
            openEditUserModal: openEditUserModalMock,
            closeEditUserModal: vi.fn(),
            submitEditUser: vi.fn(),
            deleteUser: deleteUserMock,
            createUserPending: false,
            updateUserPending: false,
            deleteUserPending: false,
            rolesLoading: false,
            roles: {
                items: [
                    {
                        id: 'role-1',
                        name: 'PlatformAdmin',
                        display_name: 'Platform Admin',
                        permissions: ['platform:admin'],
                        description: 'High privilege',
                        built_in: true,
                        enabled: true,
                    },
                    {
                        id: 'role-2',
                        name: 'DevelopmentEngineer',
                        display_name: 'Development Engineer',
                        permissions: ['vm:create', 'vm:operate'],
                        description: 'Standard access',
                        built_in: true,
                        enabled: true,
                    },
                ],
            },
        };
    },
}));

vi.mock('@/features/rbac-shared/useUserRoleBindingsManager', () => ({
    useUserRoleBindingsManager: () => {
        const [bindingForm] = Form.useForm();
        return {
            messageContextHolder: null,
            roleBindings: [
                {
                    id: 'binding-standard-1',
                    role_id: 'role-2',
                    role_name: 'DevelopmentEngineer',
                    role_display_name: 'Development Engineer',
                    scope_type: 'system',
                    scope_display_name: 'Payments',
                    allowed_environments: ['test'],
                    created_at: '2026-03-18T00:00:00Z',
                },
                {
                    id: 'binding-privileged-1',
                    role_id: 'role-1',
                    role_name: 'PlatformAdmin',
                    role_display_name: 'Platform Admin',
                    scope_type: 'global',
                    scope_display_name: 'All platform resources',
                    allowed_environments: [],
                    created_at: '2026-03-18T00:00:00Z',
                },
            ],
            roleBindingsLoading: false,
            refetchRoleBindings: vi.fn(),
            addBindingOpen: false,
            deletingBindingId: '',
            bindingForm,
            openAddBindingModal: openAddBindingModalMock,
            closeAddBindingModal: closeAddBindingModalMock,
            submitAddBinding: submitAddBindingMock,
            deleteRoleBinding: deleteRoleBindingMock,
            deleteRoleBindings: deleteRoleBindingsMock,
            resetRoleBindingsForUsers: resetRoleBindingsForUsersMock,
            createBindingPending: false,
            deleteBindingPending: false,
            bindingUserCandidates: [
                {
                    id: 'user-1',
                    username: 'alice',
                    display_name: 'Alice',
                    email: 'alice@example.com',
                    profile_attributes: {
                        department: 'Engineering',
                        section: 'Platform',
                    },
                },
            ],
            bindingUserCandidateProfileFields: [
                { key: 'department', label: 'Department', searchable: true },
                { key: 'section', label: 'Section', searchable: true },
            ],
            bindingUserCandidatesPagination: {
                page: 1,
                per_page: 20,
                total: 1,
                total_pages: 1,
            },
            bindingUserCandidatesLoading: false,
            bindingUserSearch: '',
            bindingUserSearchDraft: '',
            bindingUserPage: 1,
            bindingUserPerPage: 20,
            selectedBindingUserIds: [],
            selectedBindingUsers: [],
            effectiveSelectedBindingUserIds: [],
            setSelectedBindingUsers: vi.fn(),
            clearSelectedBindingUsers: vi.fn(),
            setBindingUserSearchDraft: vi.fn(),
            applyBindingUserSearch: vi.fn(),
            clearBindingUserSearch: vi.fn(),
            setBindingUserPagination: vi.fn(),
            scopeTargetOptionsByType: {},
            scopeTargetLoadingByType: {},
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
                    key: 'admin.users.columns.v4',
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

import {
    AdminUsersContent,
    buildOrderedUserTableDisplayColumns,
    buildUserTableColumnOptions,
    normalizeUserTableMergedColumns,
    normalizeUserTablePreferenceColumns,
} from './AdminUsersContent';

describe('AdminUsersContent', () => {
    const t = (key: string, options?: { defaultValue?: string }) => {
        const labels: Record<string, string> = {
            'users.table.email': 'Email',
            'users.table.roles': 'Roles',
            'common:table.status': 'Status',
            'common:table.created_at': 'Created',
            'users.profile_fields.department': 'Department',
            'users.profile_fields.section': 'Section',
        };
        return labels[key] ?? options?.defaultValue ?? key;
    };

    beforeEach(() => {
        pushMock.mockReset();
        setPageMock.mockReset();
        setPerPageMock.mockReset();
        setSearchMock.mockReset();
        refetchUsersMock.mockReset();
        openCreateUserModalMock.mockReset();
        openEditUserModalMock.mockReset();
        deleteUserMock.mockReset();
        openAddBindingModalMock.mockReset();
        closeAddBindingModalMock.mockReset();
        submitAddBindingMock.mockReset();
        deleteRoleBindingMock.mockReset();
        savePreferenceMock.mockReset();
        resetPreferenceMock.mockReset();
        userPreferenceState = undefined;
        authState.user.permissions = ['user:manage', 'rbac:manage'];
    });

    afterEach(() => {
        userPreferenceState = undefined;
    });

    it('renders the user directory shell and keeps access binding management on the users page', async () => {
        const user = userEvent.setup();

        render(<AdminUsersContent />);

        expect(screen.getByTestId('admin-users-page')).toBeVisible();
        expect(screen.getByText('User Management')).toBeVisible();
        expect(screen.queryByText('User Management is the primary workspace for accounts and access')).not.toBeInTheDocument();
        expect(screen.getByRole('button', { name: 'Open Access & Roles' })).toBeVisible();
        expect(screen.getByRole('button', { name: 'Open Systems' })).toBeVisible();
        expect(screen.getByRole('button', { name: 'Open Rate Limits' })).toBeVisible();
        expect(screen.getByTestId('user-batch-access-button')).toBeVisible();
        expect(screen.getByTestId('user-create-button')).toBeVisible();
        expect(screen.getByTestId('users-directory-search')).toBeVisible();
        expect(screen.getByTestId('users-directory-open-columns-drawer')).toBeVisible();
        expect(screen.getByText('Alice').closest('.admin-users-table__identity-primary')).toHaveClass(
            'admin-users-table__identity-primary--enabled',
        );
        expect(screen.getByText('alice@example.com')).toBeVisible();
        expect(screen.getByText('Engineering')).toBeVisible();
        expect(screen.getByText('Platform')).toBeVisible();
        expect(screen.getAllByText('Platform Admin').length).toBeGreaterThan(0);
        expect(screen.getAllByText('Development Engineer').length).toBeGreaterThan(0);
        expect(screen.getByTestId('user-action-role-bindings-user-1')).toBeVisible();
        expect(screen.queryByText('System Members')).not.toBeInTheDocument();
        expect(screen.queryByText('Rate Limits')).not.toBeInTheDocument();

        await user.click(screen.getByTestId('users-directory-advanced-search-toggle'));
        expect(screen.getByTestId('users-directory-search-condition-row-0')).toBeVisible();
        expect(screen.getByTestId('users-directory-search-condition-field-0')).toBeVisible();
        expect(screen.getByTestId('users-directory-search-condition-value-0')).toBeVisible();
    });

    it('applies quick search only after explicit submit', async () => {
        const user = userEvent.setup();
        render(<AdminUsersContent />);

        setPageMock.mockReset();
        setSearchMock.mockReset();

        const searchInput = screen.getByTestId('users-directory-search');
        await user.type(searchInput, 'alice');
        expect(setSearchMock).not.toHaveBeenCalled();

        await user.keyboard('{Enter}');

        await waitFor(() => {
            expect(setSearchMock).toHaveBeenLastCalledWith('alice');
        });
    });

    it('opens the column drawer, saves the customized order, and applies it immediately', async () => {
        const user = userEvent.setup();
        render(<AdminUsersContent />);

        expect(screen.getAllByText('Department').length).toBeGreaterThan(0);
        expect(screen.getAllByText('Section').length).toBeGreaterThan(0);

        await user.click(screen.getByTestId('users-directory-open-columns-drawer'));
        expect(screen.getByText('Customize directory display')).toBeVisible();

        await user.click(screen.getAllByLabelText('Hide column')[0]);
        await user.click(screen.getByTestId('users-directory-columns-save'));

        await waitFor(() => {
            expect(savePreferenceMock).toHaveBeenCalledTimes(1);
        });
        expect(savePreferenceMock).toHaveBeenCalledWith({
            value: {
                columns: ['profile:section', 'email', 'roles', 'created_at'],
                merged_columns: [],
            },
        });
        await waitFor(() => {
            expect(screen.queryAllByText('Department')).toHaveLength(0);
        });
        expect(screen.getAllByText('Section').length).toBeGreaterThan(0);
    });

    it('opens the access-bindings drawer for a selected user and shows runtime plus elevated roles explicitly', async () => {
        const user = userEvent.setup();
        render(<AdminUsersContent />);

        await user.click(screen.getByTestId('user-action-role-bindings-user-1'));

        expect(screen.getByText('Access bindings: Alice')).toBeVisible();
        expect(screen.getByText('Current bindings')).toBeVisible();
        expect(screen.getAllByText('Development Engineer').length).toBeGreaterThan(0);
        expect(screen.getAllByText('Platform Admin').length).toBeGreaterThan(0);
        expect(screen.getByText('Payments')).toBeVisible();
        expect(screen.queryByText('1 high-privilege')).not.toBeInTheDocument();
        expect(screen.getByTestId('user-binding-batch-delete-button')).toBeVisible();
    });

    it('keeps the user directory readable when only RBAC read access is present', async () => {
        const user = userEvent.setup();
        authState.user.permissions = ['rbac:read'];

        render(<AdminUsersContent />);

        expect(screen.queryByTestId('user-create-button')).not.toBeInTheDocument();
        expect(screen.queryByTestId('user-action-edit-user-1')).not.toBeInTheDocument();
        expect(screen.queryByTestId('user-action-delete-user-1')).not.toBeInTheDocument();
        expect(screen.getByTestId('user-action-role-bindings-user-1')).toBeVisible();

        await user.click(screen.getByTestId('user-action-role-bindings-user-1'));

        expect(screen.queryByTestId('user-binding-create-button')).not.toBeInTheDocument();
        expect(screen.queryByTestId('user-binding-batch-delete-button')).not.toBeInTheDocument();
        expect(screen.queryByTestId('user-binding-action-delete-binding-standard-1')).not.toBeInTheDocument();
    });

    it('builds selected columns inside a custom merged column', () => {
        const fields = [
            { key: 'department', label: 'Department', searchable: true },
            { key: 'section', label: 'Section', searchable: true },
        ];
        const options = buildUserTableColumnOptions(t, fields);
        const defaultColumns = ['profile:department', 'profile:section', 'email', 'roles', 'created_at'];
        const selectedColumns = normalizeUserTablePreferenceColumns(
            ['email', 'profile:department', 'status', 'created_at', 'roles'],
            options,
            defaultColumns,
        );
        const mergedColumns = normalizeUserTableMergedColumns(
            [
                {
                    column_keys: ['profile:department', 'status'],
                    label: 'Overview',
                },
            ],
            selectedColumns,
            options,
        );
        const visibleColumns = selectedColumns
            .map((key) => options.find((option) => option.key === key))
            .filter((option): option is NonNullable<typeof option> => Boolean(option));

        expect(buildOrderedUserTableDisplayColumns(
            visibleColumns,
            mergedColumns,
        )).toEqual([
            { kind: 'single', column: expect.objectContaining({ key: 'email' }) },
            {
                kind: 'merged',
                index: 0,
                label: 'Overview',
                columns: [
                    expect.objectContaining({ key: 'profile:department' }),
                    expect.objectContaining({ key: 'status' }),
                ],
                showLabels: true,
            },
            { kind: 'single', column: expect.objectContaining({ key: 'created_at' }) },
            { kind: 'single', column: expect.objectContaining({ key: 'roles' }) },
        ]);
    });

    it('builds multiple custom merged columns in display order', () => {
        const fields = [
            { key: 'department', label: 'Department', searchable: true },
            { key: 'section', label: 'Section', searchable: true },
        ];
        const options = buildUserTableColumnOptions(t, fields);
        const defaultColumns = ['profile:department', 'profile:section', 'email', 'roles', 'created_at'];
        const selectedColumns = normalizeUserTablePreferenceColumns(
            ['email', 'profile:department', 'status', 'profile:section', 'created_at', 'roles'],
            options,
            defaultColumns,
        );
        const mergedColumns = normalizeUserTableMergedColumns(
            [
                {
                    column_keys: ['profile:department', 'profile:section'],
                    label: 'Organization',
                },
                {
                    column_keys: ['status', 'created_at'],
                    label: 'Lifecycle',
                },
            ],
            selectedColumns,
            options,
        );
        const visibleColumns = selectedColumns
            .map((key) => options.find((option) => option.key === key))
            .filter((option): option is NonNullable<typeof option> => Boolean(option));

        expect(buildOrderedUserTableDisplayColumns(
            visibleColumns,
            mergedColumns,
        )).toEqual([
            { kind: 'single', column: expect.objectContaining({ key: 'email' }) },
            {
                kind: 'merged',
                index: 0,
                label: 'Organization',
                columns: [
                    expect.objectContaining({ key: 'profile:department' }),
                    expect.objectContaining({ key: 'profile:section' }),
                ],
                showLabels: true,
            },
            {
                kind: 'merged',
                index: 1,
                label: 'Lifecycle',
                columns: [
                    expect.objectContaining({ key: 'status' }),
                    expect.objectContaining({ key: 'created_at' }),
                ],
                showLabels: true,
            },
            { kind: 'single', column: expect.objectContaining({ key: 'roles' }) },
        ]);
    });
});
