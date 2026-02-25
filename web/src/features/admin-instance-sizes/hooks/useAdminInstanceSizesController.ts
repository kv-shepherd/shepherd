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
    // legacy hugepages_setting removed: now driven by DynamicSchemaForm spec_text
    dedicated_cpu?: boolean;
    requires_sriov?: boolean;
    /**
     * spec_text: JSON string produced by DynamicSchemaForm (spec_overrides fields).
     * Contains spec.template.* keys matching the KubeVirt VirtualMachineSpec schema.
     * Replaces the previous free-form spec_overrides_text textarea (ADR-0023 Stage 1).
     */
    spec_text?: string;
    sort_order?: number;
    enabled?: boolean;
}


/**
 * Maps UI form values to API request payload.
 * spec_text (produced by DynamicSchemaForm) is parsed into spec_overrides.
 * Overcommit checkboxes → cpu_request / memory_request_mb.
 */
function formToPayload(
    values: InstanceSizeFormValues,
): Omit<InstanceSizeCreateRequest, 'name'> & { name?: string } {
    // Parse DynamicSchemaForm spec_text → spec_overrides map
    let specOverrides: Record<string, unknown> | undefined;
    if (values.spec_text && values.spec_text.trim() && values.spec_text.trim() !== '{}') {
        try {
            const parsed = JSON.parse(values.spec_text) as unknown;
            if (parsed !== null && !Array.isArray(parsed) && typeof parsed === 'object') {
                specOverrides = parsed as Record<string, unknown>;
            }
        } catch {
            // Malformed spec_text — ignore, send without spec_overrides
        }
    }

    // If overcommit not enabled, clear the request fields
    const rest = { ...values };
    if (!values.cpu_overcommit_enabled) {
        rest.cpu_request = undefined;
    }
    if (!values.memory_overcommit_enabled) {
        rest.memory_request_mb = undefined;
    }

    // Exclude form-only fields from the API payload
    const { spec_text, cpu_overcommit_enabled, memory_overcommit_enabled, ...apiFields } = rest;
    void spec_text; void cpu_overcommit_enabled; void memory_overcommit_enabled;

    return {
        ...apiFields,
        spec_overrides: specOverrides,
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
            spec_text: '{}',
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
            cpu_overcommit_enabled: !!hydrated.cpu_request,
            memory_overcommit_enabled: !!hydrated.memory_request_mb,
            requires_sriov: item.requires_sriov,
            sort_order: hydrated.sort_order,
            // spec_text: DynamicSchemaForm will parse this JSON string on mount
            spec_text: JSON.stringify(item.spec_overrides ?? {}, null, 2),
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
        const payload = formToPayload(values);
        createMutation.mutate(payload as InstanceSizeCreateRequest);
    };

    const submitEdit = async () => {
        if (!editingItem) {
            return;
        }
        const values = await editForm.validateFields();
        const payload = formToPayload(values);
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
