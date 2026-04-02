import { Form } from 'antd';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import type { ReactNode } from 'react';
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
    RateLimitGaugeGlyph: (props: Record<string, unknown>) => <span {...props}>rate-limit-glyph</span>,
    UserDirectoryGlyph: (props: Record<string, unknown>) => <span {...props}>directory-glyph</span>,
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
        savePreferenceMock.mockReset();
        resetPreferenceMock.mockReset();
        userPreferenceState = undefined;
    });

    afterEach(() => {
        savePreferenceMock.mockReset();
        resetPreferenceMock.mockReset();
        userPreferenceState = undefined;
    });

    it('renders the page shell, core sections, and advanced search builder', async () => {
        setPageMock.mockReset();
        setSearchMock.mockReset();
        const user = userEvent.setup();

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

        await user.click(screen.getByTestId('users-directory-advanced-search-toggle'));
        expect(screen.getByTestId('users-directory-search-condition-row-0')).toBeVisible();
        expect(screen.getByTestId('users-directory-search-condition-field-0')).toBeVisible();
        expect(screen.getByTestId('users-directory-search-condition-value-0')).toBeVisible();
        expect(screen.getByTestId('users-directory-advanced-search-toggle')).toBeVisible();
        expect(screen.getByTestId('users-directory-add-search-condition')).toBeVisible();
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
        expect(screen.getByText('Customize displayed columns')).toBeVisible();

        await user.click(screen.getAllByLabelText('Hide column')[0]);
        await user.click(screen.getByTestId('users-directory-columns-save'));

        await waitFor(() => {
            expect(savePreferenceMock).toHaveBeenCalledTimes(1);
        });
        expect(savePreferenceMock).toHaveBeenCalledWith({
            value: {
                columns: ['profile:section', 'email', 'roles', 'status', 'created_at'],
                merged_columns: [],
            },
        });
        await waitFor(() => {
            expect(screen.queryAllByText('Department')).toHaveLength(0);
        });
        expect(screen.getAllByText('Section').length).toBeGreaterThan(0);
    });

    it('builds selected columns inside a custom merged column', () => {
        const fields = [
            { key: 'department', label: 'Department', searchable: true },
            { key: 'section', label: 'Section', searchable: true },
        ];
        const options = buildUserTableColumnOptions(t, fields);
        const defaultColumns = ['profile:department', 'profile:section', 'email', 'roles', 'status', 'created_at'];
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
        const defaultColumns = ['profile:department', 'profile:section', 'email', 'roles', 'status', 'created_at'];
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
