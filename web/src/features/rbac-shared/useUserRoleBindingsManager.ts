'use client';

import { Form } from 'antd';
import type { MessageInstance } from 'antd/es/message/interface';
import type { TFunction } from 'i18next';
import { useState } from 'react';

import { useApiAction, useApiGet, useApiMutation } from '@/hooks/useApiQuery';
import { api } from '@/lib/api/client';
import { translateApiError } from '@/lib/api/errorMessage';
import type { components } from '@/types/api.gen';
import { useScopeTargetCatalog } from './useScopeTargetCatalog';

type GlobalRoleBinding = components['schemas']['GlobalRoleBinding'];
type GlobalRoleBindingCreateRequest = components['schemas']['GlobalRoleBindingCreateRequest'];
type GlobalRoleBindingList = components['schemas']['GlobalRoleBindingList'];

interface BindingFormValues {
    role_id: string;
    scope_type: string;
    scope_id?: string;
    allowed_environments?: Array<'test' | 'prod'>;
}

interface UseUserRoleBindingsManagerArgs {
    t: TFunction;
    selectedUserId: string;
    messageApi: MessageInstance;
    enabled?: boolean;
}

export function useUserRoleBindingsManager({
    t,
    selectedUserId,
    messageApi,
    enabled = true,
}: UseUserRoleBindingsManagerArgs) {
    const [addBindingOpen, setAddBindingOpen] = useState(false);
    const [deletingBindingId, setDeletingBindingId] = useState('');
    const [bindingForm] = Form.useForm<BindingFormValues>();
    const bindingEnabled = enabled && selectedUserId.length > 0;
    const { scopeTargetOptionsByType, scopeTargetLoadingByType } = useScopeTargetCatalog(addBindingOpen);

    const roleBindingsQuery = useApiGet<GlobalRoleBindingList>(
        ['admin-user-role-bindings', selectedUserId],
        () => api.GET('/admin/users/{user_id}/role-bindings', {
            params: { path: { user_id: selectedUserId } },
        }),
        { enabled: bindingEnabled }
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
            onError: (err) => messageApi.error(translateApiError(t, err)),
        }
    );

    const deleteBindingMutation = useApiAction<{ userId: string; bindingId: string }>(
        ({ userId, bindingId }) => api.DELETE('/admin/users/{user_id}/role-bindings/{binding_id}', {
            params: { path: { user_id: userId, binding_id: bindingId } },
        }),
        {
            invalidateKeys: [
                ['admin-user-role-bindings', selectedUserId],
                ['admin-users'],
                ['admin-rbac-users'],
            ],
            onSuccess: () => {
                setDeletingBindingId('');
                messageApi.success(t('common:message.success'));
            },
            onError: (err) => {
                setDeletingBindingId('');
                messageApi.error(translateApiError(t, err));
            },
        }
    );

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
            allowed_environments:
                values.allowed_environments && values.allowed_environments.length > 0
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
        roleBindings: roleBindingsQuery.data?.items ?? [],
        roleBindingsLoading: roleBindingsQuery.isLoading,
        refetchRoleBindings: roleBindingsQuery.refetch,
        addBindingOpen,
        deletingBindingId,
        bindingForm,
        openAddBindingModal,
        closeAddBindingModal,
        submitAddBinding,
        deleteRoleBinding,
        createBindingPending: createBindingMutation.isPending,
        deleteBindingPending: deleteBindingMutation.isPending,
        scopeTargetOptionsByType,
        scopeTargetLoadingByType,
    };
}
