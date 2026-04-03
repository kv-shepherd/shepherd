'use client';

import { App, Form } from 'antd';
import type { TFunction } from 'i18next';
import { useState } from 'react';

import { useApiAction, useApiGet, useApiMutation } from '@/hooks/useApiQuery';
import { applyApiFieldErrors } from '@/hooks/applyApiFieldErrors';
import { SETUP_GUIDE_INVALIDATION_KEYS } from '@/features/setup-guide/queryKeys';
import { api } from '@/lib/api/client';
import { translateApiError } from '@/lib/api/errorMessage';

import type {
    NamespaceCreateRequest,
    NamespaceRegistry,
    NamespaceRegistryList,
    NamespaceUpdateRequest,
} from '../types';

interface UseAdminNamespacesControllerArgs {
    t: TFunction;
    onCreateSuccess?: (namespace: NamespaceRegistry, context: { isFirstNamespace: boolean }) => boolean | void;
}

export function useAdminNamespacesController({
    t,
    onCreateSuccess,
}: UseAdminNamespacesControllerArgs) {
    const { message: messageApi } = App.useApp();
    const messageContextHolder = null;
    const [createOpen, setCreateOpen] = useState(false);
    const [editOpen, setEditOpen] = useState(false);
    const [deleteOpen, setDeleteOpen] = useState(false);
    const [editingNs, setEditingNs] = useState<NamespaceRegistry | null>(null);
    const [deletingNs, setDeletingNs] = useState<NamespaceRegistry | null>(null);
    const [deleteConfirmName, setDeleteConfirmName] = useState('');
    const [envFilter, setEnvFilter] = useState('');
    const [enabledFilter, setEnabledFilter] = useState<'' | 'enabled' | 'disabled'>('');
    const [search, setSearch] = useState('');
    const [page, setPage] = useState(1);
    const [createForm] = Form.useForm<NamespaceCreateRequest>();
    const [editForm] = Form.useForm<NamespaceUpdateRequest>();
    const trimmedSearch = search.trim();

    const namespaceListQuery = useApiGet<NamespaceRegistryList>(
        ['admin-namespaces', page, envFilter, enabledFilter, trimmedSearch],
        () => api.GET('/admin/namespaces', {
            params: {
                query: {
                    page,
                    per_page: 20,
                    ...(envFilter ? { environment: envFilter as 'test' | 'prod' } : {}),
                    ...(enabledFilter
                        ? { enabled: enabledFilter === 'enabled' }
                        : {}),
                    ...(trimmedSearch ? { search: trimmedSearch } : {}),
                },
            },
        })
    );
    const existingNamespacesTotal =
        namespaceListQuery.data?.pagination?.total ??
        namespaceListQuery.data?.items?.length ??
        0;
    const shouldContinueOnboarding = existingNamespacesTotal === 0;

    const createMutation = useApiMutation<NamespaceCreateRequest, NamespaceRegistry>(
        (req) => api.POST('/admin/namespaces', { body: req }),
        {
            invalidateKeys: [['admin-namespaces'], ...SETUP_GUIDE_INVALIDATION_KEYS],
            onSuccess: (namespace) => {
                setCreateOpen(false);
                createForm.resetFields();
                const handled = onCreateSuccess?.(namespace, {
                    isFirstNamespace: shouldContinueOnboarding,
                }) ?? false;
                if (handled) {
                    return;
                }
                messageApi.success(t('common:message.success'));
            },
            onError: (err) => {
                if (applyApiFieldErrors(createForm, err)) {
                    return;
                }
                if (err.code === 'NAMESPACE_NAME_EXISTS') {
                    messageApi.error(t('namespaces.error.name_exists'));
                    return;
                }
                messageApi.error(translateApiError(t, err));
            },
        }
    );

    const updateMutation = useApiMutation<
        { id: string; body: NamespaceUpdateRequest },
        NamespaceRegistry
    >(
        ({ id, body }) => api.PUT('/admin/namespaces/{namespace_id}', {
            params: { path: { namespace_id: id } },
            body,
        }),
        {
            invalidateKeys: [['admin-namespaces'], ...SETUP_GUIDE_INVALIDATION_KEYS],
            onSuccess: () => {
                messageApi.success(t('common:message.success'));
                setEditOpen(false);
                setEditingNs(null);
            },
            onError: (err) => {
                if (applyApiFieldErrors(editForm, err)) {
                    return;
                }
                messageApi.error(translateApiError(t, err));
            },
        }
    );

    const deleteMutation = useApiAction<{ id: string; confirmName: string }>(
        ({ id, confirmName }) => api.DELETE('/admin/namespaces/{namespace_id}', {
            params: {
                path: { namespace_id: id },
                query: { confirm_name: confirmName },
            },
        }),
        {
            invalidateKeys: [['admin-namespaces'], ...SETUP_GUIDE_INVALIDATION_KEYS],
            onSuccess: () => {
                messageApi.success(t('common:message.success'));
                setDeleteOpen(false);
                setDeletingNs(null);
                setDeleteConfirmName('');
            },
            onError: (err) => messageApi.error(translateApiError(t, err)),
        }
    );

    const openCreateModal = () => {
        setCreateOpen(true);
    };

    const closeCreateModal = () => {
        setCreateOpen(false);
        createForm.resetFields();
    };

    const openEditModal = (record: NamespaceRegistry) => {
        void api.GET('/admin/namespaces/{namespace_id}', {
            params: { path: { namespace_id: record.id } },
        }).then(({ data }) => {
            const resolved = data ?? record;
            setEditingNs(resolved);
            editForm.setFieldsValue({
                description: resolved.description ?? '',
                enabled: resolved.enabled ?? true,
            });
            setEditOpen(true);
        }).catch(() => {
            setEditingNs(record);
            editForm.setFieldsValue({
                description: record.description ?? '',
                enabled: record.enabled ?? true,
            });
            setEditOpen(true);
        });
    };

    const closeEditModal = () => {
        setEditOpen(false);
        setEditingNs(null);
    };

    const openDeleteModal = (record: NamespaceRegistry) => {
        void api.GET('/admin/namespaces/{namespace_id}', {
            params: { path: { namespace_id: record.id } },
        }).then(({ data }) => {
            setDeletingNs(data ?? record);
            setDeleteConfirmName('');
            setDeleteOpen(true);
        }).catch(() => {
            setDeletingNs(record);
            setDeleteConfirmName('');
            setDeleteOpen(true);
        });
    };

    const closeDeleteModal = () => {
        setDeleteOpen(false);
        setDeletingNs(null);
        setDeleteConfirmName('');
    };

    const submitCreate = async () => {
        const values = await createForm.validateFields();
        createMutation.mutate(values);
    };

    const submitUpdate = async () => {
        if (!editingNs) {
            return;
        }
        const values = await editForm.validateFields();
        updateMutation.mutate({ id: editingNs.id, body: values });
    };

    const submitDelete = () => {
        if (!deletingNs) {
            return;
        }
        deleteMutation.mutate({ id: deletingNs.id, confirmName: deleteConfirmName });
    };

    const changeEnvFilter = (value: string | undefined) => {
        setEnvFilter(value ?? '');
        setPage(1);
    };

    const changeEnabledFilter = (value: 'enabled' | 'disabled' | undefined) => {
        setEnabledFilter(value ?? '');
        setPage(1);
    };

    const changeSearch = (value: string) => {
        setSearch(value);
        setPage(1);
    };

    return {
        messageContextHolder,
        data: namespaceListQuery.data,
        isLoading: namespaceListQuery.isLoading,
        refetch: namespaceListQuery.refetch,
        createOpen,
        editOpen,
        deleteOpen,
        editingNs,
        deletingNs,
        deleteConfirmName,
        setDeleteConfirmName,
        envFilter,
        enabledFilter,
        search,
        changeSearch,
        changeEnvFilter,
        changeEnabledFilter,
        page,
        setPage,
        createForm,
        editForm,
        openCreateModal,
        closeCreateModal,
        openEditModal,
        closeEditModal,
        openDeleteModal,
        closeDeleteModal,
        submitCreate,
        submitUpdate,
        submitDelete,
        createPending: createMutation.isPending,
        updatePending: updateMutation.isPending,
        deletePending: deleteMutation.isPending,
    };
}
