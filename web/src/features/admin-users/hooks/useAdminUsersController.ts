'use client';

import { Form, message } from 'antd';
import type { TFunction } from 'i18next';
import { useMemo, useState } from 'react';

import { useApiAction, useApiGet, useApiMutation } from '@/hooks/useApiQuery';
import { api } from '@/lib/api/client';

import type {
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
} from '../types';

interface UseAdminUsersControllerArgs {
    t: TFunction;
}

export function useAdminUsersController({ t }: UseAdminUsersControllerArgs) {
    const [messageApi, messageContextHolder] = message.useMessage();
    const [page, setPage] = useState(1);
    const [perPage, setPerPage] = useState(20);
    const [selectedSystemId, setSelectedSystemId] = useState<string>();
    const [addOpen, setAddOpen] = useState(false);
    const [createUserOpen, setCreateUserOpen] = useState(false);
    const [editUserOpen, setEditUserOpen] = useState(false);
    const [deletingUserId, setDeletingUserId] = useState<string>('');
    const [editingUserId, setEditingUserId] = useState<string>('');

    const [addForm] = Form.useForm<SystemMemberCreateRequest>();
    const [createUserForm] = Form.useForm<UserCreateRequest>();
    const [editUserForm] = Form.useForm<UserUpdateRequest>();

    const usersQuery = useApiGet<UserList>(
        ['admin-users', page, perPage],
        () => api.GET('/admin/users', { params: { query: { page, per_page: perPage } } })
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

    const rateLimitStatusQuery = useApiGet<RateLimitStatusList>(
        ['admin-rate-limit-status'],
        () => api.GET('/admin/rate-limits/status')
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
            onError: (err) => messageApi.error(err.message || t('common:message.error')),
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
            onError: (err) => messageApi.error(err.message || t('common:message.error')),
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
                messageApi.error(err.message || t('common:message.error'));
            },
        }
    );

    const addMemberMutation = useApiMutation<SystemMemberCreateRequest, SystemMember>(
        (req) => api.POST('/systems/{system_id}/members', { params: { path: { system_id: selectedSystemId! } }, body: req }),
        {
            invalidateKeys: [['system-members', selectedSystemId]],
            onSuccess: () => {
                messageApi.success(t('common:message.success'));
                closeAddModal();
            },
            onError: (err) => messageApi.error(err.message || t('common:message.error')),
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
            onError: (err) => messageApi.error(err.message || t('common:message.error')),
        }
    );

    const removeMemberMutation = useApiAction<{ userId: string }>(
        (req) => api.DELETE('/systems/{system_id}/members/{user_id}', {
            params: { path: { system_id: selectedSystemId!, user_id: req.userId } },
        }),
        {
            invalidateKeys: [['system-members', selectedSystemId]],
            onSuccess: () => messageApi.success(t('common:message.success')),
            onError: (err) => messageApi.error(err.message || t('common:message.error')),
        }
    );

    const createExemptionMutation = useApiMutation<RateLimitExemptionCreateRequest, RateLimitExemption>(
        (req) => api.POST('/admin/rate-limits/exemptions', { body: req }),
        {
            invalidateKeys: [['admin-rate-limit-status']],
            onSuccess: () => messageApi.success(t('common:message.success')),
            onError: (err) => messageApi.error(err.message || t('common:message.error')),
        }
    );

    const deleteExemptionMutation = useApiAction<string>(
        (userID) => api.DELETE('/admin/rate-limits/exemptions/{user_id}', {
            params: { path: { user_id: userID } },
        }),
        {
            invalidateKeys: [['admin-rate-limit-status']],
            onSuccess: () => messageApi.success(t('common:message.success')),
            onError: (err) => messageApi.error(err.message || t('common:message.error')),
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
            onError: (err) => messageApi.error(err.message || t('common:message.error')),
        }
    );

    const openAddModal = () => {
        if (!selectedSystemId) {
            messageApi.warning(t('users.members.select_system_first'));
            return;
        }
        setAddOpen(true);
    };

    const closeAddModal = () => {
        setAddOpen(false);
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

    return {
        messageContextHolder,
        users: usersQuery.data,
        usersLoading: usersQuery.isLoading,
        page,
        perPage,
        setPage,
        setPerPage,
        refetchUsers: usersQuery.refetch,
        systems,
        systemsLoading: systemsQuery.isLoading,
        selectedSystemId,
        setSelectedSystemId,
        members: membersQuery.data,
        membersLoading: membersQuery.isLoading,
        refetchMembers: membersQuery.refetch,
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
    };
}
