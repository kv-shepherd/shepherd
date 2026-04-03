'use client';

import { App, Form } from 'antd';
import type { TFunction } from 'i18next';
import { useDeferredValue, useState } from 'react';

import { useApiAction, useApiGet, useApiMutation } from '@/hooks/useApiQuery';
import { api } from '@/lib/api/client';
import { translateApiError } from '@/lib/api/errorMessage';

import type {
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
    const { message: messageApi } = App.useApp();
    const messageContextHolder = null;
    const [page, setPage] = useState(1);
    const [perPage, setPerPage] = useState(20);
    const [search, setSearch] = useState('');
    const [createUserOpen, setCreateUserOpen] = useState(false);
    const [editUserOpen, setEditUserOpen] = useState(false);
    const [deletingUserId, setDeletingUserId] = useState<string>('');
    const [editingUserId, setEditingUserId] = useState<string>('');

    const [createUserForm] = Form.useForm<UserCreateRequest>();
    const [editUserForm] = Form.useForm<UserUpdateRequest>();
    const deferredSearch = useDeferredValue(search.trim());

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

    return {
        messageApi,
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
        roles: rolesQuery.data,
        rolesLoading: rolesQuery.isLoading,
    };
}
