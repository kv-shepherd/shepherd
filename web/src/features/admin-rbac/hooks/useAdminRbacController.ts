'use client';

import { Form, message } from 'antd';
import type { TFunction } from 'i18next';
import { useMemo, useState } from 'react';

import { useApiAction, useApiGet, useApiMutation } from '@/hooks/useApiQuery';
import { api } from '@/lib/api/client';

import type {
    GlobalRoleBinding,
    GlobalRoleBindingCreateRequest,
    GlobalRoleBindingList,
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

interface BindingFormValues {
    role_id: string;
    scope_type: string;
    scope_id?: string;
    allowed_environments?: Array<'test' | 'prod'>;
}

export function useAdminRbacController({ t }: UseAdminRbacControllerArgs) {
    const [messageApi, messageContextHolder] = message.useMessage();
    const [selectedUserId, setSelectedUserId] = useState<string>('');

    const [createRoleOpen, setCreateRoleOpen] = useState(false);
    const [editRoleOpen, setEditRoleOpen] = useState(false);
    const [deleteRoleOpen, setDeleteRoleOpen] = useState(false);
    const [editingRole, setEditingRole] = useState<Role | null>(null);
    const [deletingRole, setDeletingRole] = useState<Role | null>(null);

    const [addBindingOpen, setAddBindingOpen] = useState(false);
    const [deletingBindingId, setDeletingBindingId] = useState<string>('');

    const [roleCreateForm] = Form.useForm<RoleCreateRequest>();
    const [roleEditForm] = Form.useForm<RoleUpdateRequest>();
    const [bindingForm] = Form.useForm<BindingFormValues>();

    const rolesQuery = useApiGet<RoleList>(
        ['admin-roles'],
        () => api.GET('/admin/roles')
    );

    const permissionsQuery = useApiGet<PermissionList>(
        ['admin-permissions'],
        () => api.GET('/admin/permissions')
    );

    const usersQuery = useApiGet<UserList>(
        ['admin-rbac-users'],
        () => api.GET('/admin/users', { params: { query: { page: 1, per_page: 200 } } })
    );

    const roleBindingsQuery = useApiGet<GlobalRoleBindingList>(
        ['admin-user-role-bindings', selectedUserId],
        () => api.GET('/admin/users/{user_id}/role-bindings', {
            params: { path: { user_id: selectedUserId } },
        }),
        { enabled: selectedUserId.length > 0 }
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
            onError: (err) => messageApi.error(err.message || t('common:message.error')),
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
            onError: (err) => messageApi.error(err.message || t('common:message.error')),
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
            onError: (err) => messageApi.error(err.message || t('common:message.error')),
        }
    );

    const createBindingMutation = useApiMutation<GlobalRoleBindingCreateRequest, GlobalRoleBinding>(
        (body) => api.POST('/admin/users/{user_id}/role-bindings', {
            params: { path: { user_id: selectedUserId } },
            body,
        }),
        {
            invalidateKeys: [
                ['admin-user-role-bindings', selectedUserId],
                ['admin-users'],
                ['admin-rbac-users'],
            ],
            onSuccess: () => {
                messageApi.success(t('common:message.success'));
                setAddBindingOpen(false);
                bindingForm.resetFields();
            },
            onError: (err) => messageApi.error(err.message || t('common:message.error')),
        }
    );

    const deleteBindingMutation = useApiAction<{ userId: string; bindingId: string }>(
        ({ userId, bindingId }) => api.DELETE('/admin/users/{user_id}/role-bindings/{binding_id}', {
            params: { path: { user_id: userId, binding_id: bindingId } },
        }),
        {
            invalidateKeys: [['admin-user-role-bindings', selectedUserId]],
            onSuccess: () => {
                setDeletingBindingId('');
                messageApi.success(t('common:message.success'));
            },
            onError: (err) => {
                setDeletingBindingId('');
                messageApi.error(err.message || t('common:message.error'));
            },
        }
    );

    const roles = useMemo<Role[]>(() => rolesQuery.data?.items ?? [], [rolesQuery.data?.items]);
    const permissions = useMemo<Permission[]>(() => permissionsQuery.data?.items ?? [], [permissionsQuery.data?.items]);
    const users = useMemo<User[]>(() => usersQuery.data?.items ?? [], [usersQuery.data?.items]);
    const roleBindings = useMemo<GlobalRoleBinding[]>(() => roleBindingsQuery.data?.items ?? [], [roleBindingsQuery.data?.items]);

    const selectedUser = useMemo(
        () => users.find((user) => user.id === selectedUserId),
        [selectedUserId, users]
    );

    const permissionOptions = useMemo(
        () => permissions.map((p) => ({
            value: p.key,
            label: p.description ? `${p.key} (${p.description})` : p.key,
        })),
        [permissions]
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

    const openAddBindingModal = () => {
        if (!selectedUserId) {
            messageApi.warning(t('rbac.bindings.select_user_first'));
            return;
        }
        bindingForm.resetFields();
        bindingForm.setFieldsValue({ scope_type: 'global' });
        setAddBindingOpen(true);
    };

    const closeAddBindingModal = () => {
        setAddBindingOpen(false);
        bindingForm.resetFields();
    };

    const submitAddBinding = async () => {
        if (!selectedUserId) {
            messageApi.warning(t('rbac.bindings.select_user_first'));
            return;
        }
        const values = await bindingForm.validateFields();
        createBindingMutation.mutate({
            role_id: values.role_id,
            scope_type: values.scope_type || 'global',
            scope_id: values.scope_id?.trim() || undefined,
            allowed_environments: values.allowed_environments && values.allowed_environments.length > 0
                ? values.allowed_environments
                : undefined,
        });
    };

    const deleteRoleBinding = (bindingId: string) => {
        if (!selectedUserId) {
            return;
        }
        setDeletingBindingId(bindingId);
        deleteBindingMutation.mutate({ userId: selectedUserId, bindingId });
    };

    return {
        messageContextHolder,

        roles,
        permissions,
        users,
        roleBindings,
        selectedUser,
        selectedUserId,
        setSelectedUserId,

        rolesLoading: rolesQuery.isLoading,
        permissionsLoading: permissionsQuery.isLoading,
        usersLoading: usersQuery.isLoading,
        roleBindingsLoading: roleBindingsQuery.isLoading,
        refetchRoles: rolesQuery.refetch,
        refetchPermissions: permissionsQuery.refetch,
        refetchUsers: usersQuery.refetch,
        refetchRoleBindings: roleBindingsQuery.refetch,

        permissionOptions,

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

        addBindingOpen,
        deletingBindingId,
        bindingForm,
        openAddBindingModal,
        closeAddBindingModal,
        submitAddBinding,
        deleteRoleBinding,
        createBindingPending: createBindingMutation.isPending,
        deleteBindingPending: deleteBindingMutation.isPending,
    };
}
