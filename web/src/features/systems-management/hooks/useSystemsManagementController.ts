'use client';

import { Form, message } from 'antd';
import type { TFunction } from 'i18next';
import { useState } from 'react';

import { useApiAction, useApiGet, useApiMutation } from '@/hooks/useApiQuery';
import { applyApiFieldErrors } from '@/hooks/applyApiFieldErrors';
import { SETUP_GUIDE_INVALIDATION_KEYS } from '@/features/setup-guide/queryKeys';
import { api } from '@/lib/api/client';
import { translateApiError } from '@/lib/api/errorMessage';

import type { System, SystemCreateRequest, SystemFilterOptions, SystemList } from '../types';
import type { SystemUpdateRequest } from '../types';

interface UseSystemsManagementControllerArgs {
    t: TFunction;
    onCreateSuccess?: (system: System, context: { isFirstSystem: boolean }) => boolean | void;
}

interface SystemSearchFilters {
    search: string;
    createdBy: string;
    serviceId: string;
    memberId: string;
}

const EMPTY_SYSTEM_SEARCH_FILTERS: SystemSearchFilters = {
    search: '',
    createdBy: '',
    serviceId: '',
    memberId: '',
};

function normalizeSystemSearchFilters(nextFilters: Partial<SystemSearchFilters>): SystemSearchFilters {
    return {
        search: nextFilters.search?.trim() ?? '',
        createdBy: nextFilters.createdBy?.trim() ?? '',
        serviceId: nextFilters.serviceId?.trim() ?? '',
        memberId: nextFilters.memberId?.trim() ?? '',
    };
}

export function useSystemsManagementController({
    t,
    onCreateSuccess,
}: UseSystemsManagementControllerArgs) {
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
    const [filters, setFilters] = useState<SystemSearchFilters>(EMPTY_SYSTEM_SEARCH_FILTERS);

    const systemsQuery = useApiGet<SystemList>(
        ['systems', page, pageSize, filters.search, filters.createdBy, filters.serviceId, filters.memberId],
        () => api.GET('/systems', {
            params: {
                query: {
                    page,
                    per_page: pageSize,
                    ...(filters.search ? { search: filters.search } : {}),
                    ...(filters.createdBy ? { created_by_exact: filters.createdBy } : {}),
                    ...(filters.serviceId ? { service_id: filters.serviceId } : {}),
                    ...(filters.memberId ? { member_id: filters.memberId } : {}),
                },
            },
        })
    );
    const systemFilterOptionsQuery = useApiGet<SystemFilterOptions>(
        ['systems', 'filter-options'],
        () => api.GET('/systems/filter-options'),
    );

    const existingSystemsTotal =
        systemsQuery.data?.pagination?.total ??
        systemsQuery.data?.items?.length ??
        0;
    const shouldContinueToFirstService = existingSystemsTotal === 0;

    const createMutation = useApiMutation<SystemCreateRequest, System>(
        (req) => api.POST('/systems', { body: req }),
        {
            invalidateKeys: [['systems'], ...SETUP_GUIDE_INVALIDATION_KEYS],
            onSuccess: (system) => {
                closeCreateModal();
                const handled = onCreateSuccess?.(system, {
                    isFirstSystem: shouldContinueToFirstService,
                }) ?? false;
                if (handled) {
                    return;
                }
                messageApi.success(t('message.success'));
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
            invalidateKeys: [['systems'], ...SETUP_GUIDE_INVALIDATION_KEYS],
            onSuccess: () => {
                messageApi.success(t('message.success'));
                closeDeleteModal();
            },
            onError: (err) => messageApi.error(translateApiError(t, err, 'message.error')),
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
            invalidateKeys: [['systems'], ...SETUP_GUIDE_INVALIDATION_KEYS],
            onSuccess: () => {
                messageApi.success(t('message.success'));
                closeEditModal();
            },
            onError: (err) => {
                if (applyApiFieldErrors(editForm, err)) {
                    return;
                }
                messageApi.error(translateApiError(t, err, 'message.error'));
            },
        }
    );

    const openCreateModal = () => {
        setCreateOpen(true);
    };

    const applyFilters = (nextFilters: Partial<SystemSearchFilters>) => {
        setFilters(normalizeSystemSearchFilters(nextFilters));
        setPage(1);
    };

    const clearFilters = () => {
        setFilters(EMPTY_SYSTEM_SEARCH_FILTERS);
        setPage(1);
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
        filters,
        hasActiveFilters: Object.values(filters).some((value) => value !== ''),
        page,
        pageSize,
        setPage,
        setPageSize,
        data: systemsQuery.data,
        systemFilterOptions: systemFilterOptionsQuery.data,
        systemFilterOptionsLoading: systemFilterOptionsQuery.isLoading,
        isLoading: systemsQuery.isLoading,
        refetch: systemsQuery.refetch,
        openCreateModal,
        applyFilters,
        clearFilters,
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
