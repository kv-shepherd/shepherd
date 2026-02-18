'use client';

import { Form, message } from 'antd';
import type { TFunction } from 'i18next';
import { useDeferredValue, useMemo, useState } from 'react';

import { useApiAction, useApiGet, useApiMutation } from '@/hooks/useApiQuery';
import { api } from '@/lib/api/client';

import {
    getCapabilityLabels,
    type InstanceSize,
    type InstanceSizeCreateRequest,
    type InstanceSizeList,
    type InstanceSizeUpdateRequest,
} from '../types';

interface UseAdminInstanceSizesControllerArgs {
    t: TFunction;
}

interface InstanceSizeCreateFormValues extends InstanceSizeCreateRequest {
    spec_overrides_text?: string;
}

interface InstanceSizeEditFormValues extends InstanceSizeUpdateRequest {
    spec_overrides_text?: string;
}

function parseJSONMap(raw: string, onError: () => void): Record<string, unknown> | undefined {
    const text = raw.trim();
    if (!text) {
        return undefined;
    }
    try {
        const parsed = JSON.parse(text) as unknown;
        if (parsed === null || Array.isArray(parsed) || typeof parsed !== 'object') {
            onError();
            return undefined;
        }
        return parsed as Record<string, unknown>;
    } catch {
        onError();
        return undefined;
    }
}

export function useAdminInstanceSizesController({ t }: UseAdminInstanceSizesControllerArgs) {
    const [messageApi, messageContextHolder] = message.useMessage();
    const [globalSearch, setGlobalSearch] = useState('');
    const deferredSearch = useDeferredValue(globalSearch);
    const isStale = globalSearch !== deferredSearch;

    const [searchedColumn, setSearchedColumn] = useState('');
    const [searchText, setSearchText] = useState('');

    const [createOpen, setCreateOpen] = useState(false);
    const [editOpen, setEditOpen] = useState(false);
    const [deleteOpen, setDeleteOpen] = useState(false);
    const [editingItem, setEditingItem] = useState<InstanceSize | null>(null);
    const [deletingItem, setDeletingItem] = useState<InstanceSize | null>(null);

    const [createForm] = Form.useForm<InstanceSizeCreateFormValues>();
    const [editForm] = Form.useForm<InstanceSizeEditFormValues>();

    const instanceSizesQuery = useApiGet<InstanceSizeList>(
        ['admin-instance-sizes'],
        () => api.GET('/admin/instance-sizes')
    );

    const createMutation = useApiMutation<InstanceSizeCreateRequest, InstanceSize>(
        (body) => api.POST('/admin/instance-sizes', { body }),
        {
            invalidateKeys: [['admin-instance-sizes']],
            onSuccess: () => {
                messageApi.success(t('common:message.success'));
                setCreateOpen(false);
                createForm.resetFields();
            },
            onError: (err) => messageApi.error(err.message || t('common:message.error')),
        }
    );

    const updateMutation = useApiMutation<{ id: string; body: InstanceSizeUpdateRequest }, InstanceSize>(
        ({ id, body }) => api.PATCH('/admin/instance-sizes/{instance_size_id}', {
            params: { path: { instance_size_id: id } },
            body,
        }),
        {
            invalidateKeys: [['admin-instance-sizes']],
            onSuccess: () => {
                messageApi.success(t('common:message.success'));
                setEditOpen(false);
                setEditingItem(null);
                editForm.resetFields();
            },
            onError: (err) => messageApi.error(err.message || t('common:message.error')),
        }
    );

    const deleteMutation = useApiAction<string>(
        (id) => api.DELETE('/admin/instance-sizes/{instance_size_id}', { params: { path: { instance_size_id: id } } }),
        {
            invalidateKeys: [['admin-instance-sizes']],
            onSuccess: () => {
                messageApi.success(t('common:message.success'));
                setDeleteOpen(false);
                setDeletingItem(null);
            },
            onError: (err) => messageApi.error(err.message || t('common:message.error')),
        }
    );

    const filteredItems = useMemo(() => {
        const items = instanceSizesQuery.data?.items ?? [];
        if (!deferredSearch) {
            return items;
        }
        const query = deferredSearch.toLowerCase();
        return items.filter((instanceSize: InstanceSize) =>
            instanceSize.name.toLowerCase().includes(query) ||
            (instanceSize.display_name ?? '').toLowerCase().includes(query) ||
            (instanceSize.description ?? '').toLowerCase().includes(query) ||
            getCapabilityLabels(instanceSize).some((label) => label.toLowerCase().includes(query))
        );
    }, [instanceSizesQuery.data?.items, deferredSearch]);

    const openCreateModal = () => {
        createForm.resetFields();
        createForm.setFieldsValue({
            enabled: true,
            sort_order: 0,
            spec_overrides_text: '{}',
        });
        setCreateOpen(true);
    };

    const openEditModal = (item: InstanceSize) => {
        const hydrated = item as InstanceSize & {
            cpu_request?: number;
            memory_request_mb?: number;
            sort_order?: number;
        };
        setEditingItem(item);
        editForm.setFieldsValue({
            name: item.name,
            display_name: item.display_name,
            description: item.description,
            cpu_cores: item.cpu_cores,
            memory_mb: item.memory_mb,
            disk_gb: item.disk_gb,
            dedicated_cpu: item.dedicated_cpu,
            cpu_request: hydrated.cpu_request,
            memory_request_mb: hydrated.memory_request_mb,
            requires_gpu: item.requires_gpu,
            requires_sriov: item.requires_sriov,
            requires_hugepages: item.requires_hugepages,
            hugepages_size: item.hugepages_size,
            sort_order: hydrated.sort_order,
            spec_overrides_text: JSON.stringify(item.spec_overrides ?? {}, null, 2),
            enabled: item.enabled,
        });
        setEditOpen(true);
    };

    const openDeleteModal = (item: InstanceSize) => {
        setDeletingItem(item);
        setDeleteOpen(true);
    };

    const submitCreate = async () => {
        const values = await createForm.validateFields();
        const specOverrides = parseJSONMap(values.spec_overrides_text ?? '', () => {
            messageApi.error(t('instanceSizes.spec_overrides_invalid'));
        });
        if (values.spec_overrides_text && !specOverrides) {
            return;
        }
        const { spec_overrides_text: _specText, ...payload } = values;
        createMutation.mutate({
            ...payload,
            spec_overrides: specOverrides,
        });
    };

    const submitEdit = async () => {
        if (!editingItem) {
            return;
        }
        const values = await editForm.validateFields();
        const specOverrides = parseJSONMap(values.spec_overrides_text ?? '', () => {
            messageApi.error(t('instanceSizes.spec_overrides_invalid'));
        });
        if (values.spec_overrides_text && !specOverrides) {
            return;
        }
        const { spec_overrides_text: _specText, ...payload } = values;
        updateMutation.mutate({
            id: editingItem.id,
            body: {
                ...payload,
                spec_overrides: specOverrides,
            },
        });
    };

    const submitDelete = () => {
        if (!deletingItem) {
            return;
        }
        deleteMutation.mutate(deletingItem.id);
    };

    return {
        messageContextHolder,
        globalSearch,
        setGlobalSearch,
        deferredSearch,
        isStale,
        searchedColumn,
        setSearchedColumn,
        searchText,
        setSearchText,
        filteredItems,
        data: instanceSizesQuery.data,
        isLoading: instanceSizesQuery.isLoading,
        refetch: instanceSizesQuery.refetch,
        createOpen,
        editOpen,
        deleteOpen,
        editingItem,
        deletingItem,
        createForm,
        editForm,
        openCreateModal,
        openEditModal,
        openDeleteModal,
        closeCreateModal: () => {
            setCreateOpen(false);
            createForm.resetFields();
        },
        closeEditModal: () => {
            setEditOpen(false);
            setEditingItem(null);
            editForm.resetFields();
        },
        closeDeleteModal: () => {
            setDeleteOpen(false);
            setDeletingItem(null);
        },
        submitCreate,
        submitEdit,
        submitDelete,
        createPending: createMutation.isPending,
        updatePending: updateMutation.isPending,
        deletePending: deleteMutation.isPending,
    };
}
