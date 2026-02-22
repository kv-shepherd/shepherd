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

interface GpuDevice {
    name: string;
    deviceName: string;
}

interface InstanceSizeFormValues {
    name: string;
    display_name?: string;
    description?: string;
    cpu_cores: number;
    memory_mb: number;
    disk_gb?: number;
    cpu_request?: number;
    memory_request_mb?: number;
    cpu_overcommit_enabled?: boolean;
    memory_overcommit_enabled?: boolean;
    hugepages_setting?: string;
    dedicated_cpu?: boolean;
    requires_sriov?: boolean;
    gpu_devices?: GpuDevice[];
    spec_overrides_text?: string;
    sort_order?: number;
    enabled?: boolean;
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

/**
 * Maps UI form values to API request payload.
 * Handles: hugepages_setting → requires_hugepages + hugepages_size,
 *          gpu_devices → requires_gpu + merged into spec_overrides,
 *          overcommit checkboxes → cpu_request / memory_request_mb.
 */
function formToPayload(
    values: InstanceSizeFormValues,
    specOverrides: Record<string, unknown> | undefined,
): Omit<InstanceSizeCreateRequest, 'name'> & { name?: string } {
    const {
        hugepages_setting,
        gpu_devices,
        ...rest
    } = values;

    // Hugepages mapping
    const requires_hugepages = !!hugepages_setting && hugepages_setting !== 'none';
    const hugepages_size = requires_hugepages ? hugepages_setting : undefined;

    // GPU devices mapping
    const requires_gpu = (gpu_devices?.length ?? 0) > 0;
    let mergedOverrides = specOverrides;
    if (requires_gpu && gpu_devices && gpu_devices.length > 0) {
        mergedOverrides = {
            ...(mergedOverrides ?? {}),
            gpus: gpu_devices,
        };
    }

    // If overcommit not enabled, clear the request fields
    if (!values.cpu_overcommit_enabled) {
        rest.cpu_request = undefined;
    }
    if (!values.memory_overcommit_enabled) {
        rest.memory_request_mb = undefined;
    }

    return {
        ...rest,
        requires_hugepages,
        hugepages_size,
        requires_gpu,
        spec_overrides: mergedOverrides,
    };
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

    const [createForm] = Form.useForm<InstanceSizeFormValues>();
    const [editForm] = Form.useForm<InstanceSizeFormValues>();

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
            hugepages_setting: 'none',
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

        // Derive hugepages_setting from API fields
        let hugepages_setting = 'none';
        if (item.requires_hugepages && item.hugepages_size) {
            hugepages_setting = item.hugepages_size;
        }

        // Extract gpu_devices from spec_overrides if present
        const specOverrides = item.spec_overrides ?? {};
        const gpuDevices = (specOverrides.gpus as GpuDevice[] | undefined) ?? [];
        const cleanOverrides = { ...specOverrides };
        delete cleanOverrides.gpus;

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
            cpu_overcommit_enabled: !!hydrated.cpu_request,
            memory_overcommit_enabled: !!hydrated.memory_request_mb,
            hugepages_setting,
            requires_sriov: item.requires_sriov,
            gpu_devices: gpuDevices,
            sort_order: hydrated.sort_order,
            spec_overrides_text: JSON.stringify(cleanOverrides, null, 2),
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
        if (values.spec_overrides_text && values.spec_overrides_text.trim() && !specOverrides) {
            return;
        }
        const payload = formToPayload(values, specOverrides);
        createMutation.mutate(payload as InstanceSizeCreateRequest);
    };

    const submitEdit = async () => {
        if (!editingItem) {
            return;
        }
        const values = await editForm.validateFields();
        const specOverrides = parseJSONMap(values.spec_overrides_text ?? '', () => {
            messageApi.error(t('instanceSizes.spec_overrides_invalid'));
        });
        if (values.spec_overrides_text && values.spec_overrides_text.trim() && !specOverrides) {
            return;
        }
        const payload = formToPayload(values, specOverrides);
        updateMutation.mutate({
            id: editingItem.id,
            body: payload as InstanceSizeUpdateRequest,
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
