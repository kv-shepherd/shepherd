'use client';

import { Form, message } from 'antd';
import type { TFunction } from 'i18next';
import { useDeferredValue, useMemo, useState } from 'react';

import { useApiAction, useApiGet, useApiMutation } from '@/hooks/useApiQuery';
import { api } from '@/lib/api/client';
import { translateApiError } from '@/lib/api/errorMessage';
import { useScopeTargetCatalog } from '@/features/rbac-shared/useScopeTargetCatalog';

import type {
    GlobalRoleBinding,
    GlobalRoleBindingCreateRequest,
    GlobalRoleBindingList,
    RateLimitExemption,
    RateLimitExemptionCreateRequest,
    RateLimitStatusList,
    RateLimitUserOverride,
    RateLimitUserOverrideRequest,
    System,
    SystemList,
    SystemMember,
    SystemMemberCreateRequest,
    SystemMemberList,
    SystemMemberRoleUpdateRequest,
    User,
    UserCreateRequest,
    UserList,
    UserUpdateRequest,
    RoleList,
} from '../types';

interface UseAdminUsersControllerArgs {
    t: TFunction;
}

export function useAdminUsersController({ t }: UseAdminUsersControllerArgs) {
    const [messageApi, messageContextHolder] = message.useMessage();
    const [page, setPage] = useState(1);
    const [perPage, setPerPage] = useState(20);
    const [search, setSearch] = useState('');
    const [selectedSystemId, setSelectedSystemId] = useState<string>();
    const [addOpen, setAddOpen] = useState(false);
    const [memberCandidateSearch, setMemberCandidateSearch] = useState('');
    const [createUserOpen, setCreateUserOpen] = useState(false);
    const [editUserOpen, setEditUserOpen] = useState(false);
    const [deletingUserId, setDeletingUserId] = useState<string>('');
    const [editingUserId, setEditingUserId] = useState<string>('');
    // Role bindings drawer
    const [roleBindingsUserId, setRoleBindingsUserId] = useState<string>('');
    const [roleBindingsUserLabel, setRoleBindingsUserLabel] = useState('');
    const [roleBindingCreateOpen, setRoleBindingCreateOpen] = useState(false);
    const [deletingBindingId, setDeletingBindingId] = useState<string>('');

    const [addForm] = Form.useForm<SystemMemberCreateRequest>();
    const [createUserForm] = Form.useForm<UserCreateRequest>();
    const [editUserForm] = Form.useForm<UserUpdateRequest>();
    const [roleBindingCreateForm] = Form.useForm<GlobalRoleBindingCreateRequest>();
    const deferredSearch = useDeferredValue(search.trim());
    const deferredMemberCandidateSearch = useDeferredValue(memberCandidateSearch.trim());
    const { scopeTargetOptionsByType, scopeTargetLoadingByType } = useScopeTargetCatalog(roleBindingCreateOpen);

    const usersQuery = useApiGet<UserList>(
        ['admin-users', page, perPage, deferredSearch],
        () => api.GET('/admin/users', {
            params: {
                query: {
                    page,
                    per_page: perPage,
                    ...(deferredSearch ? { search: deferredSearch } : {}),
                },
            },
        })
    );

    const systemsQuery = useApiGet<SystemList>(
        ['member-systems'],
        () => api.GET('/systems', { params: { query: { page: 1, per_page: 200 } } })
    );

    const membersQuery = useApiGet<SystemMemberList>(
        ['system-members', selectedSystemId],
        () => api.GET('/systems/{system_id}/members', { params: { path: { system_id: selectedSystemId! } } }),
        { enabled: Boolean(selectedSystemId) }
    );

    const memberCandidatesQuery = useApiGet<UserList>(
        ['system-member-candidates', selectedSystemId, deferredMemberCandidateSearch],
        () => api.GET('/systems/{system_id}/member-candidates', {
            params: {
                path: { system_id: selectedSystemId! },
                query: {
                    page: 1,
                    per_page: 50,
                    ...(deferredMemberCandidateSearch ? { search: deferredMemberCandidateSearch } : {}),
                },
            },
        }),
        { enabled: addOpen && Boolean(selectedSystemId) }
    );

    const rateLimitStatusQuery = useApiGet<RateLimitStatusList>(
        ['admin-rate-limit-status'],
        () => api.GET('/admin/rate-limits/status')
    );

    const roleBindingsQuery = useApiGet<GlobalRoleBindingList>(
        ['user-role-bindings', roleBindingsUserId],
        () => api.GET('/admin/users/{user_id}/role-bindings', { params: { path: { user_id: roleBindingsUserId } } }),
        { enabled: Boolean(roleBindingsUserId) }
    );

    const rolesQuery = useApiGet<RoleList>(
        ['admin-roles-dropdown'],
        () => api.GET('/admin/roles')
    );

    const createUserMutation = useApiMutation<UserCreateRequest, User>(
        (req) => api.POST('/admin/users', { body: req }),
        {
            invalidateKeys: [['admin-users']],
            onSuccess: () => {
                messageApi.success(t('common:message.success'));
                setCreateUserOpen(false);
                createUserForm.resetFields();
            },
            onError: (err) => messageApi.error(translateApiError(t, err)),
        }
    );

    const updateUserMutation = useApiMutation<{ userId: string; body: UserUpdateRequest }, User>(
        ({ userId, body }) => api.PATCH('/admin/users/{user_id}', {
            params: { path: { user_id: userId } },
            body,
        }),
        {
            invalidateKeys: [['admin-users']],
            onSuccess: () => {
                messageApi.success(t('common:message.success'));
                setEditUserOpen(false);
                setEditingUserId('');
                editUserForm.resetFields();
            },
            onError: (err) => messageApi.error(translateApiError(t, err)),
        }
    );

    const deleteUserMutation = useApiAction<string>(
        (userId) => api.DELETE('/admin/users/{user_id}', { params: { path: { user_id: userId } } }),
        {
            invalidateKeys: [['admin-users']],
            onSuccess: () => {
                messageApi.success(t('common:message.success'));
                setDeletingUserId('');
            },
            onError: (err) => {
                setDeletingUserId('');
                messageApi.error(translateApiError(t, err));
            },
        }
    );

    const addMemberMutation = useApiMutation<SystemMemberCreateRequest, SystemMember>(
        (req) => api.POST('/systems/{system_id}/members', { params: { path: { system_id: selectedSystemId! } }, body: req }),
        {
            invalidateKeys: [['system-members', selectedSystemId], ['system-member-candidates', selectedSystemId]],
            onSuccess: () => {
                messageApi.success(t('common:message.success'));
                closeAddModal();
            },
            onError: (err) => messageApi.error(translateApiError(t, err)),
        }
    );

    const updateRoleMutation = useApiMutation<
        { userId: string; role: NonNullable<SystemMemberRoleUpdateRequest['role']> },
        SystemMember
    >(
        (req) => api.PATCH('/systems/{system_id}/members/{user_id}', {
            params: { path: { system_id: selectedSystemId!, user_id: req.userId } },
            body: { role: req.role },
        }),
        {
            invalidateKeys: [['system-members', selectedSystemId]],
            onSuccess: () => messageApi.success(t('common:message.success')),
            onError: (err) => messageApi.error(translateApiError(t, err)),
        }
    );

    const removeMemberMutation = useApiAction<{ userId: string }>(
        (req) => api.DELETE('/systems/{system_id}/members/{user_id}', {
            params: { path: { system_id: selectedSystemId!, user_id: req.userId } },
        }),
        {
            invalidateKeys: [['system-members', selectedSystemId]],
            onSuccess: () => messageApi.success(t('common:message.success')),
            onError: (err) => messageApi.error(translateApiError(t, err)),
        }
    );

    const createExemptionMutation = useApiMutation<RateLimitExemptionCreateRequest, RateLimitExemption>(
        (req) => api.POST('/admin/rate-limits/exemptions', { body: req }),
        {
            invalidateKeys: [['admin-rate-limit-status']],
            onSuccess: () => messageApi.success(t('common:message.success')),
            onError: (err) => messageApi.error(translateApiError(t, err)),
        }
    );

    const deleteExemptionMutation = useApiAction<string>(
        (userID) => api.DELETE('/admin/rate-limits/exemptions/{user_id}', {
            params: { path: { user_id: userID } },
        }),
        {
            invalidateKeys: [['admin-rate-limit-status']],
            onSuccess: () => messageApi.success(t('common:message.success')),
            onError: (err) => messageApi.error(translateApiError(t, err)),
        }
    );

    const updateUserOverrideMutation = useApiMutation<
        { userId: string; body: RateLimitUserOverrideRequest },
        RateLimitUserOverride
    >(
        ({ userId, body }) => api.PUT('/admin/rate-limits/users/{user_id}', {
            params: { path: { user_id: userId } },
            body,
        }),
        {
            invalidateKeys: [['admin-rate-limit-status']],
            onSuccess: () => messageApi.success(t('common:message.success')),
            onError: (err) => messageApi.error(translateApiError(t, err)),
        }
    );

    const createRoleBindingMutation = useApiMutation<GlobalRoleBindingCreateRequest, GlobalRoleBinding>(
        (req) => api.POST('/admin/users/{user_id}/role-bindings', {
            params: { path: { user_id: roleBindingsUserId } },
            body: req,
        }),
        {
            invalidateKeys: [['user-role-bindings', roleBindingsUserId]],
            onSuccess: () => messageApi.success(t('common:message.success')),
            onError: (err) => messageApi.error(translateApiError(t, err)),
        }
    );

    const deleteRoleBindingMutation = useApiAction<{ bindingId: string }>(
        ({ bindingId }) => api.DELETE('/admin/users/{user_id}/role-bindings/{binding_id}', {
            params: { path: { user_id: roleBindingsUserId, binding_id: bindingId } },
        }),
        {
            invalidateKeys: [['user-role-bindings', roleBindingsUserId]],
            onSuccess: () => {
                messageApi.success(t('common:message.success'));
                setDeletingBindingId('');
            },
            onError: (err) => {
                setDeletingBindingId('');
                messageApi.error(translateApiError(t, err));
            },
        }
    );

    const openAddModal = () => {
        if (!selectedSystemId) {
            messageApi.warning(t('users.members.select_system_first'));
            return;
        }
        setMemberCandidateSearch('');
        setAddOpen(true);
    };

    const closeAddModal = () => {
        setAddOpen(false);
        setMemberCandidateSearch('');
        addForm.resetFields();
    };

    const submitAddMember = async () => {
        if (!selectedSystemId) {
            messageApi.warning(t('users.members.select_system_first'));
            return;
        }
        const values = await addForm.validateFields();
        addMemberMutation.mutate(values);
    };

    const openCreateUserModal = () => {
        createUserForm.resetFields();
        createUserForm.setFieldsValue({ enabled: true, force_password_change: true });
        setCreateUserOpen(true);
    };

    const closeCreateUserModal = () => {
        setCreateUserOpen(false);
        createUserForm.resetFields();
    };

    const submitCreateUser = async () => {
        const values = await createUserForm.validateFields();
        createUserMutation.mutate(values);
    };

    const openEditUserModal = (user: User) => {
        setEditingUserId(user.id);
        editUserForm.setFieldsValue({
            email: user.email,
            display_name: user.display_name,
            enabled: user.enabled,
        });
        setEditUserOpen(true);
    };

    const closeEditUserModal = () => {
        setEditUserOpen(false);
        setEditingUserId('');
        editUserForm.resetFields();
    };

    const submitEditUser = async () => {
        if (!editingUserId) {
            return;
        }
        const values = await editUserForm.validateFields();
        updateUserMutation.mutate({ userId: editingUserId, body: values });
    };

    const deleteUser = (userId: string) => {
        setDeletingUserId(userId);
        deleteUserMutation.mutate(userId);
    };

    const updateMemberRole = (userId: string, role: NonNullable<SystemMemberRoleUpdateRequest['role']>) => {
        if (!selectedSystemId) {
            messageApi.warning(t('users.members.select_system_first'));
            return;
        }
        updateRoleMutation.mutate({ userId, role });
    };

    const removeMember = (userId: string) => {
        if (!selectedSystemId) {
            messageApi.warning(t('users.members.select_system_first'));
            return;
        }
        removeMemberMutation.mutate({ userId });
    };

    const systems = useMemo<System[]>(() => systemsQuery.data?.items ?? [], [systemsQuery.data?.items]);

    const applyRateLimitExemption = (req: RateLimitExemptionCreateRequest) => {
        createExemptionMutation.mutate(req);
    };

    const removeRateLimitExemption = (userID: string) => {
        deleteExemptionMutation.mutate(userID);
    };

    const updateRateLimitOverride = (userID: string, body: RateLimitUserOverrideRequest) => {
        updateUserOverrideMutation.mutate({ userId: userID, body });
    };

    const openRoleBindingsModal = (user: Pick<User, 'id' | 'username' | 'display_name'>) => {
        setRoleBindingsUserId(user.id);
        setRoleBindingsUserLabel(formatUserDisplayLabel(user));
    };

    const closeRoleBindingsModal = () => {
        setRoleBindingsUserId('');
        setRoleBindingsUserLabel('');
        setRoleBindingCreateOpen(false);
        roleBindingCreateForm.resetFields();
    };

    const openRoleBindingCreateModal = () => {
        roleBindingCreateForm.resetFields();
        roleBindingCreateForm.setFieldsValue({ scope_type: 'global' });
        setRoleBindingCreateOpen(true);
    };

    const closeRoleBindingCreateModal = () => {
        setRoleBindingCreateOpen(false);
        roleBindingCreateForm.resetFields();
    };

    const submitCreateRoleBinding = async () => {
        const values = await roleBindingCreateForm.validateFields();
        await createRoleBindingMutation.mutateAsync(values);
        await roleBindingsQuery.refetch();
        setRoleBindingCreateOpen(false);
        roleBindingCreateForm.resetFields();
    };

    const deleteRoleBinding = (bindingId: string) => {
        setDeletingBindingId(bindingId);
        deleteRoleBindingMutation.mutate({ bindingId });
    };

    return {
        messageContextHolder,
        users: usersQuery.data,
        usersLoading: usersQuery.isLoading,
        page,
        perPage,
        search,
        setPage,
        setPerPage,
        setSearch,
        refetchUsers: usersQuery.refetch,
        systems,
        systemsLoading: systemsQuery.isLoading,
        selectedSystemId,
        setSelectedSystemId,
        members: membersQuery.data,
        membersLoading: membersQuery.isLoading,
        refetchMembers: membersQuery.refetch,
        memberCandidates: memberCandidatesQuery.data,
        memberCandidatesLoading: memberCandidatesQuery.isLoading || memberCandidatesQuery.isFetching,
        refetchMemberCandidates: memberCandidatesQuery.refetch,
        memberCandidateSearch,
        setMemberCandidateSearch,
        addOpen,
        addForm,
        openAddModal,
        closeAddModal,
        submitAddMember,
        addPending: addMemberMutation.isPending,
        updateMemberRole,
        updatePending: updateRoleMutation.isPending,
        removeMember,
        removePending: removeMemberMutation.isPending,
        rateLimitStatus: rateLimitStatusQuery.data,
        rateLimitLoading: rateLimitStatusQuery.isLoading,
        refetchRateLimitStatus: rateLimitStatusQuery.refetch,
        applyRateLimitExemption,
        removeRateLimitExemption,
        updateRateLimitOverride,
        rateLimitMutationPending: createExemptionMutation.isPending || deleteExemptionMutation.isPending || updateUserOverrideMutation.isPending,
        createUserOpen,
        editUserOpen,
        editingUserId,
        deletingUserId,
        createUserForm,
        editUserForm,
        openCreateUserModal,
        closeCreateUserModal,
        submitCreateUser,
        openEditUserModal,
        closeEditUserModal,
        submitEditUser,
        deleteUser,
        createUserPending: createUserMutation.isPending,
        updateUserPending: updateUserMutation.isPending,
        deleteUserPending: deleteUserMutation.isPending,
        // Role bindings
        roleBindingsUserId,
        roleBindingsUserLabel,
        roleBindings: roleBindingsQuery.data,
        roleBindingsLoading: roleBindingsQuery.isLoading,
        roles: rolesQuery.data,
        rolesLoading: rolesQuery.isLoading,
        scopeTargetOptionsByType,
        scopeTargetLoadingByType,
        roleBindingCreateOpen,
        roleBindingCreateForm,
        deletingBindingId,
        openRoleBindingsModal,
        closeRoleBindingsModal,
        openRoleBindingCreateModal,
        closeRoleBindingCreateModal,
        submitCreateRoleBinding,
        deleteRoleBinding,
        createRoleBindingPending: createRoleBindingMutation.isPending,
        deleteRoleBindingPending: deleteRoleBindingMutation.isPending,
    };
}

function formatUserDisplayLabel(user: Pick<User, 'username' | 'display_name'>) {
    return user.display_name?.trim() || user.username;
}
