'use client';

import { Form, message } from 'antd';
import type { TFunction } from 'i18next';
import { useState } from 'react';

import { useApiAction, useApiGet, useApiMutation } from '@/hooks/useApiQuery';
import { applyApiFieldErrors } from '@/hooks/applyApiFieldErrors';
import { api } from '@/lib/api/client';

import type { System, SystemCreateRequest, SystemList } from '../types';
import type { SystemUpdateRequest } from '../types';

interface UseSystemsManagementControllerArgs {
    t: TFunction;
}

export function useSystemsManagementController({ t }: UseSystemsManagementControllerArgs) {
    const [messageApi, messageContextHolder] = message.useMessage();
    const [createOpen, setCreateOpen] = useState(false);
    const [editOpen, setEditOpen] = useState(false);
    const [editingSystem, setEditingSystem] = useState<System | null>(null);
    const [deleteOpen, setDeleteOpen] = useState(false);
    const [deletingSystem, setDeletingSystem] = useState<System | null>(null);
    const [deleteConfirmName, setDeleteConfirmName] = useState('');
    const [membersOpen, setMembersOpen] = useState(false);
    const [membersSystem, setMembersSystem] = useState<System | null>(null);
    const [form] = Form.useForm<SystemCreateRequest>();
    const [editForm] = Form.useForm<SystemUpdateRequest>();
    const [page, setPage] = useState(1);
    const [pageSize, setPageSize] = useState(20);

    const systemsQuery = useApiGet<SystemList>(
        ['systems', page, pageSize],
        () => api.GET('/systems', { params: { query: { page, per_page: pageSize } } })
    );

    const createMutation = useApiMutation<SystemCreateRequest, System>(
        (req) => api.POST('/systems', { body: req }),
        {
            invalidateKeys: [['systems']],
            onSuccess: () => {
                messageApi.success(t('message.success'));
                closeCreateModal();
            },
            onError: (err) => {
                if (applyApiFieldErrors(form, err)) {
                    return;
                }
                messageApi.error(
                    err.code === 'SYSTEM_NAME_EXISTS' || err.code === 'CONFLICT'
                        ? t('systems.error.name_exists')
                        : t('message.error')
                );
            },
        }
    );

    const deleteMutation = useApiAction<{ id: string; confirmName: string }>(
        ({ id, confirmName }) => api.DELETE('/systems/{system_id}', {
            params: {
                path: { system_id: id },
                query: { confirm_name: confirmName },
            },
        }),
        {
            invalidateKeys: [['systems']],
            onSuccess: () => {
                messageApi.success(t('message.success'));
                closeDeleteModal();
            },
            onError: (err) => messageApi.error(err.message || t('message.error')),
        }
    );

    const updateMutation = useApiMutation<
        { id: string; body: SystemUpdateRequest },
        System
    >(
        ({ id, body }) => api.PATCH('/systems/{system_id}', {
            params: { path: { system_id: id } },
            body,
        }),
        {
            invalidateKeys: [['systems']],
            onSuccess: () => {
                messageApi.success(t('message.success'));
                closeEditModal();
            },
            onError: (err) => {
                if (applyApiFieldErrors(editForm, err)) {
                    return;
                }
                messageApi.error(err.message || t('message.error'));
            },
        }
    );

    const openCreateModal = () => {
        setCreateOpen(true);
    };

    const closeCreateModal = () => {
        setCreateOpen(false);
        form.resetFields();
    };

    const openDeleteModal = (system: System) => {
        void api.GET('/systems/{system_id}', {
            params: { path: { system_id: system.id } },
        }).then(({ data }) => {
            setDeletingSystem(data ?? system);
            setDeleteConfirmName('');
            setDeleteOpen(true);
        }).catch(() => {
            setDeletingSystem(system);
            setDeleteConfirmName('');
            setDeleteOpen(true);
        });
    };

    const openEditModal = (system: System) => {
        void api.GET('/systems/{system_id}', {
            params: { path: { system_id: system.id } },
        }).then(({ data }) => {
            const resolved = data ?? system;
            setEditingSystem(resolved);
            editForm.setFieldsValue({ description: resolved.description || '' });
            setEditOpen(true);
        }).catch(() => {
            setEditingSystem(system);
            editForm.setFieldsValue({ description: system.description || '' });
            setEditOpen(true);
        });
    };

    const closeEditModal = () => {
        setEditOpen(false);
        setEditingSystem(null);
        editForm.resetFields();
    };

    const closeDeleteModal = () => {
        setDeleteOpen(false);
        setDeletingSystem(null);
        setDeleteConfirmName('');
    };

    const openMembersModal = (system: System) => {
        setMembersSystem(system);
        setMembersOpen(true);
    };

    const closeMembersModal = () => {
        setMembersOpen(false);
        setMembersSystem(null);
    };

    const submitCreate = async () => {
        const values = await form.validateFields();
        createMutation.mutate(values);
    };

    const submitDelete = () => {
        if (!deletingSystem) {
            return;
        }
        deleteMutation.mutate({ id: deletingSystem.id, confirmName: deleteConfirmName });
    };

    const submitEdit = async () => {
        if (!editingSystem) {
            return;
        }
        const values = await editForm.validateFields();
        updateMutation.mutate({ id: editingSystem.id, body: values });
    };

    return {
        messageContextHolder,
        createOpen,
        editOpen,
        editingSystem,
        deleteOpen,
        deletingSystem,
        deleteConfirmName,
        setDeleteConfirmName,
        form,
        editForm,
        page,
        pageSize,
        setPage,
        setPageSize,
        data: systemsQuery.data,
        isLoading: systemsQuery.isLoading,
        refetch: systemsQuery.refetch,
        openCreateModal,
        closeCreateModal,
        openDeleteModal,
        openEditModal,
        closeEditModal,
        closeDeleteModal,
        submitCreate,
        submitEdit,
        submitDelete,
        createPending: createMutation.isPending,
        updatePending: updateMutation.isPending,
        deletePending: deleteMutation.isPending,
        membersOpen,
        membersSystem,
        openMembersModal,
        closeMembersModal,
    };
}
