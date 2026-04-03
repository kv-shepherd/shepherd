'use client';

import { App, Form } from 'antd';
import type { TFunction } from 'i18next';
import { useDeferredValue, useMemo, useState } from 'react';

import { useApiAction, useApiGet, useApiMutation } from '@/hooks/useApiQuery';
import { api } from '@/lib/api/client';
import { translateApiError } from '@/lib/api/errorMessage';
import { useUserRoleBindingsManager } from '@/features/rbac-shared/useUserRoleBindingsManager';

import type {
    Permission,
    PermissionList,
    Role,
    RoleCreateRequest,
    RoleList,
    RoleUpdateRequest,
    User,
    UserList,
} from '../types';

interface UseAdminRbacControllerArgs {
    t: TFunction;
}

export function useAdminRbacController({ t }: UseAdminRbacControllerArgs) {
    const { message: messageApi } = App.useApp();
    const messageContextHolder = null;
    const [selectedUserId, setSelectedUserId] = useState<string>('');
    const [selectedUserLabel, setSelectedUserLabel] = useState('');
    const [userSearch, setUserSearch] = useState('');
    const deferredUserSearch = useDeferredValue(userSearch.trim());

    const [createRoleOpen, setCreateRoleOpen] = useState(false);
    const [editRoleOpen, setEditRoleOpen] = useState(false);
    const [deleteRoleOpen, setDeleteRoleOpen] = useState(false);
    const [editingRole, setEditingRole] = useState<Role | null>(null);
    const [deletingRole, setDeletingRole] = useState<Role | null>(null);

    const [roleCreateForm] = Form.useForm<RoleCreateRequest>();
    const [roleEditForm] = Form.useForm<RoleUpdateRequest>();

    const rolesQuery = useApiGet<RoleList>(
        ['admin-roles'],
        () => api.GET('/admin/roles')
    );

    const permissionsQuery = useApiGet<PermissionList>(
        ['admin-permissions'],
        () => api.GET('/admin/permissions')
    );

    const usersQuery = useApiGet<UserList>(
        ['admin-rbac-users', deferredUserSearch],
        () => api.GET('/admin/users', {
            params: {
                query: {
                    page: 1,
                    per_page: 50,
                    ...(deferredUserSearch ? { search: deferredUserSearch } : {}),
                },
            },
        })
    );

    const createRoleMutation = useApiMutation<RoleCreateRequest, Role>(
        (body) => api.POST('/admin/roles', { body }),
        {
            invalidateKeys: [['admin-roles']],
            onSuccess: () => {
                messageApi.success(t('common:message.success'));
                setCreateRoleOpen(false);
                roleCreateForm.resetFields();
            },
            onError: (err) => messageApi.error(translateApiError(t, err)),
        }
    );

    const updateRoleMutation = useApiMutation<{ roleId: string; body: RoleUpdateRequest }, Role>(
        ({ roleId, body }) => api.PATCH('/admin/roles/{role_id}', {
            params: { path: { role_id: roleId } },
            body,
        }),
        {
            invalidateKeys: [['admin-roles']],
            onSuccess: () => {
                messageApi.success(t('common:message.success'));
                setEditRoleOpen(false);
                setEditingRole(null);
                roleEditForm.resetFields();
            },
            onError: (err) => messageApi.error(translateApiError(t, err)),
        }
    );

    const deleteRoleMutation = useApiAction<string>(
        (roleId) => api.DELETE('/admin/roles/{role_id}', { params: { path: { role_id: roleId } } }),
        {
            invalidateKeys: [['admin-roles']],
            onSuccess: () => {
                messageApi.success(t('common:message.success'));
                setDeleteRoleOpen(false);
                setDeletingRole(null);
            },
            onError: (err) => messageApi.error(translateApiError(t, err)),
        }
    );

    const roles = useMemo<Role[]>(() => rolesQuery.data?.items ?? [], [rolesQuery.data?.items]);
    const permissions = useMemo<Permission[]>(() => permissionsQuery.data?.items ?? [], [permissionsQuery.data?.items]);
    const users = useMemo<User[]>(() => usersQuery.data?.items ?? [], [usersQuery.data?.items]);
    const bindings = useUserRoleBindingsManager({
        t,
        selectedUserId,
        messageApi,
    });

    const selectedUser = useMemo(
        () => users.find((user) => user.id === selectedUserId),
        [selectedUserId, users]
    );
    const selectedUserDisplayLabel = selectedUser?.display_name || selectedUser?.username || selectedUserLabel;
    const selectedUserValue = selectedUserId || undefined;

    const permissionOptions = useMemo(
        () => permissions.map((p) => ({
            value: p.key,
            label: p.description ? `${p.key} (${p.description})` : p.key,
        })),
        [permissions]
    );
    const userOptions = useMemo(
        () => users.map((user) => ({
            value: user.id,
            label: formatUserOptionLabel(user),
        })),
        [users]
    );

    const openCreateRoleModal = () => {
        roleCreateForm.resetFields();
        roleCreateForm.setFieldsValue({ enabled: true, permissions: [] });
        setCreateRoleOpen(true);
    };

    const closeCreateRoleModal = () => {
        setCreateRoleOpen(false);
        roleCreateForm.resetFields();
    };

    const submitCreateRole = async () => {
        const values = await roleCreateForm.validateFields();
        createRoleMutation.mutate(values);
    };

    const openEditRoleModal = (role: Role) => {
        setEditingRole(role);
        roleEditForm.resetFields();
        roleEditForm.setFieldsValue({
            display_name: role.display_name,
            description: role.description,
            permissions: role.permissions,
            enabled: role.enabled,
        });
        setEditRoleOpen(true);
    };

    const closeEditRoleModal = () => {
        setEditRoleOpen(false);
        setEditingRole(null);
        roleEditForm.resetFields();
    };

    const submitEditRole = async () => {
        if (!editingRole) {
            return;
        }
        const values = await roleEditForm.validateFields();
        updateRoleMutation.mutate({ roleId: editingRole.id, body: values });
    };

    const openDeleteRoleModal = (role: Role) => {
        setDeletingRole(role);
        setDeleteRoleOpen(true);
    };

    const closeDeleteRoleModal = () => {
        setDeleteRoleOpen(false);
        setDeletingRole(null);
    };

    const submitDeleteRole = () => {
        if (!deletingRole) {
            return;
        }
        deleteRoleMutation.mutate(deletingRole.id);
    };

    const selectUser = (userId: string, label = '') => {
        setSelectedUserId(userId);
        setSelectedUserLabel(userId ? label.trim() || userId : '');
    };

    return {
        messageContextHolder,

        roles,
        permissions,
        users,
        userOptions,
        roleBindings: bindings.roleBindings,
        selectedUser,
        selectedUserDisplayLabel,
        selectedUserId,
        selectedUserValue,
        selectUser,
        userSearch,
        setUserSearch,

        rolesLoading: rolesQuery.isLoading,
        permissionsLoading: permissionsQuery.isLoading,
        usersLoading: usersQuery.isLoading || usersQuery.isFetching,
        roleBindingsLoading: bindings.roleBindingsLoading,
        refetchRoles: rolesQuery.refetch,
        refetchPermissions: permissionsQuery.refetch,
        refetchUsers: usersQuery.refetch,
        refetchRoleBindings: bindings.refetchRoleBindings,

        permissionOptions,
        scopeTargetOptionsByType: bindings.scopeTargetOptionsByType,
        scopeTargetLoadingByType: bindings.scopeTargetLoadingByType,

        createRoleOpen,
        editRoleOpen,
        deleteRoleOpen,
        editingRole,
        deletingRole,
        roleCreateForm,
        roleEditForm,
        openCreateRoleModal,
        closeCreateRoleModal,
        submitCreateRole,
        openEditRoleModal,
        closeEditRoleModal,
        submitEditRole,
        openDeleteRoleModal,
        closeDeleteRoleModal,
        submitDeleteRole,
        createRolePending: createRoleMutation.isPending,
        updateRolePending: updateRoleMutation.isPending,
        deleteRolePending: deleteRoleMutation.isPending,

        addBindingOpen: bindings.addBindingOpen,
        deletingBindingId: bindings.deletingBindingId,
        bindingForm: bindings.bindingForm,
        openAddBindingModal: bindings.openAddBindingModal,
        closeAddBindingModal: bindings.closeAddBindingModal,
        submitAddBinding: bindings.submitAddBinding,
        deleteRoleBinding: bindings.deleteRoleBinding,
        createBindingPending: bindings.createBindingPending,
        deleteBindingPending: bindings.deleteBindingPending,
    };
}

function formatUserOptionLabel(user: User) {
    return user.display_name
        ? `${user.display_name} (${user.username})`
        : user.username;
}
