'use client';

import { Form, message } from 'antd';
import type { TFunction } from 'i18next';
import { useState } from 'react';

import { useApiAction, useApiGet, useApiMutation } from '@/hooks/useApiQuery';
import { applyApiFieldErrors } from '@/hooks/applyApiFieldErrors';
import { api } from '@/lib/api/client';

import type {
    NamespaceCreateRequest,
    NamespaceRegistry,
    NamespaceRegistryList,
    NamespaceUpdateRequest,
} from '../types';

interface UseAdminNamespacesControllerArgs {
    t: TFunction;
}

export function useAdminNamespacesController({ t }: UseAdminNamespacesControllerArgs) {
    const [messageApi, messageContextHolder] = message.useMessage();
    const [createOpen, setCreateOpen] = useState(false);
    const [editOpen, setEditOpen] = useState(false);
    const [deleteOpen, setDeleteOpen] = useState(false);
    const [editingNs, setEditingNs] = useState<NamespaceRegistry | null>(null);
    const [deletingNs, setDeletingNs] = useState<NamespaceRegistry | null>(null);
    const [deleteConfirmName, setDeleteConfirmName] = useState('');
    const [envFilter, setEnvFilter] = useState('');
    const [page, setPage] = useState(1);
    const [createForm] = Form.useForm<NamespaceCreateRequest>();
    const [editForm] = Form.useForm<NamespaceUpdateRequest>();

    const namespaceListQuery = useApiGet<NamespaceRegistryList>(
        ['admin-namespaces', page, envFilter],
        () => api.GET('/admin/namespaces', {
            params: {
                query: {
                    page,
                    per_page: 20,
                    ...(envFilter ? { environment: envFilter as 'test' | 'prod' } : {}),
                },
            },
        })
    );

    const createMutation = useApiMutation<NamespaceCreateRequest, NamespaceRegistry>(
        (req) => api.POST('/admin/namespaces', { body: req }),
        {
            invalidateKeys: [['admin-namespaces']],
            onSuccess: () => {
                messageApi.success(t('common:message.success'));
                setCreateOpen(false);
                createForm.resetFields();
            },
            onError: (err) => {
                if (applyApiFieldErrors(createForm, err)) {
                    return;
                }
                if (err.code === 'NAMESPACE_NAME_EXISTS') {
                    messageApi.error(t('namespaces.error.name_exists'));
                    return;
                }
                messageApi.error(err.message || t('common:message.error'));
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
            invalidateKeys: [['admin-namespaces']],
            onSuccess: () => {
                messageApi.success(t('common:message.success'));
                setEditOpen(false);
                setEditingNs(null);
            },
            onError: (err) => {
                if (applyApiFieldErrors(editForm, err)) {
                    return;
                }
                messageApi.error(err.message || t('common:message.error'));
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
            invalidateKeys: [['admin-namespaces']],
            onSuccess: () => {
                messageApi.success(t('common:message.success'));
                setDeleteOpen(false);
                setDeletingNs(null);
                setDeleteConfirmName('');
            },
            onError: (err) => messageApi.error(err.message || t('common:message.error')),
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
        changeEnvFilter,
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
