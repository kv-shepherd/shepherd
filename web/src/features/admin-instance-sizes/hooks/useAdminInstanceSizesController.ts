'use client';

import { Form, message } from 'antd';
import type { TFunction } from 'i18next';
import { useDeferredValue, useEffect, useMemo, useState } from 'react';

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
    catalog_scope?: InstanceSize['catalog_scope'];
    cpu_cores: number;
    memory_gi: number;
    disk_gb?: number;
    cpu_request?: number;
    memory_request_gi?: number;
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
    root_volume_mode_intent?: 'auto' | 'explicit';
    dv_access_modes?: string[];
    dv_volume_mode?: InstanceSize['dv_volume_mode'];
    sort_order?: number;
    enabled?: boolean;
}

type InstanceSizePayload = Record<string, unknown> & {
    name?: string;
};


/**
 * Maps UI form values to API request payload.
 * spec_text (produced by DynamicSchemaForm) is parsed into spec_overrides.
 * Overcommit checkboxes → cpu_request / memory_request_gi.
 */
function formToPayload(
    values: InstanceSizeFormValues,
    mode: 'create' | 'update',
): InstanceSizePayload {
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

    const cpuOvercommitEnabled = values.dedicated_cpu ? false : values.cpu_overcommit_enabled;
    // Overcommit clear semantics:
    // - create: omit request fields when disabled
    // - update: send 0 as clear sentinel so backend can clear persisted values
    const cpuRequest =
        cpuOvercommitEnabled ? values.cpu_request : (mode === 'update' ? 0 : undefined);
    const memoryRequestGi =
        values.memory_overcommit_enabled ? values.memory_request_gi : (mode === 'update' ? 0 : undefined);
    const explicitRootVolumeMode = values.root_volume_mode_intent === 'explicit';
    const dvAccessModes = normalizeStringList(values.dv_access_modes);
    const dvVolumeMode = typeof values.dv_volume_mode === 'string' ? values.dv_volume_mode.trim() : '';

    // Explicit whitelist to avoid leaking dynamic-form internals (for example "spec")
    // into API payload and violating OpenAPI additionalProperties=false checks.
    const payload: InstanceSizePayload = {
        name: values.name,
        display_name: values.display_name,
        description: values.description,
        catalog_scope: values.catalog_scope,
        cpu_cores: values.cpu_cores,
        memory_gi: values.memory_gi,
        disk_gb: values.disk_gb,
        cpu_request: cpuRequest,
        memory_request_gi: memoryRequestGi,
        dedicated_cpu: values.dedicated_cpu,
        requires_sriov: values.requires_sriov,
        sort_order: values.sort_order,
        enabled: values.enabled,
        spec_overrides: specOverrides,
        ...(explicitRootVolumeMode && dvAccessModes.length > 0 && dvVolumeMode
            ? {
                dv_access_modes: dvAccessModes,
                dv_volume_mode: dvVolumeMode as InstanceSize['dv_volume_mode'],
            }
            : mode === 'update'
                ? {
                    dv_access_modes: [],
                }
                : {}),
    };

    return Object.fromEntries(
        Object.entries(payload).filter(([, value]) => value !== undefined)
    ) as Omit<InstanceSizeCreateRequest, 'name'> & { name?: string };
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
            getCapabilityLabels(instanceSize).some((label) => label.toLowerCase().includes(query)) ||
            getRootVolumeModeLabels(instanceSize).some((label) => label.toLowerCase().includes(query))
        );
    }, [instanceSizesQuery.data?.items, deferredSearch]);

    const openCreateModal = () => {
        createForm.resetFields();
        createForm.setFieldsValue({
            catalog_scope: 'unclassified',
            enabled: true,
            sort_order: 0,
            dedicated_cpu: false,
            spec_text: '{}',
            root_volume_mode_intent: 'auto',
            dv_access_modes: undefined,
            dv_volume_mode: undefined,
        });
        setCreateOpen(true);
    };

    const openEditModal = (item: InstanceSize) => {
        setEditingItem(item);
        setEditOpen(true);
    };

    // Edit modal uses destroyOnHidden, so form fields are unmounted while closed.
    // Hydrate fields after modal opens to avoid empty required values.
    useEffect(() => {
        if (!editOpen || !editingItem) {
            return;
        }

        const hydrated = editingItem as InstanceSize & {
            cpu_request?: number;
            memory_request_gi?: number;
            sort_order?: number;
        };

        let phaseTwoTimer: ReturnType<typeof setTimeout> | null = null;

        // Defer one tick so Modal/Form fields are mounted before hydration.
        const timer = setTimeout(() => {
            editForm.resetFields();
            editForm.setFieldsValue({
                name: editingItem.name,
                display_name: editingItem.display_name,
                description: editingItem.description,
                catalog_scope: editingItem.catalog_scope,
                cpu_cores: editingItem.cpu_cores,
                memory_gi: editingItem.memory_gi,
                disk_gb: editingItem.disk_gb,
                dedicated_cpu: editingItem.dedicated_cpu,
                cpu_overcommit_enabled: !!hydrated.cpu_request,
                memory_overcommit_enabled: !!hydrated.memory_request_gi,
                requires_sriov: editingItem.requires_sriov,
                root_volume_mode_intent:
                    (editingItem.dv_access_modes?.length ?? 0) > 0 || editingItem.dv_volume_mode
                        ? 'explicit'
                        : 'auto',
                dv_access_modes: editingItem.dv_access_modes,
                dv_volume_mode: editingItem.dv_volume_mode,
                sort_order: hydrated.sort_order,
                // spec_text: DynamicSchemaForm will parse this JSON string on mount
                spec_text: JSON.stringify(editingItem.spec_overrides ?? {}, null, 2),
                enabled: editingItem.enabled,
            });

            // Conditional overcommit fields mount one render later when their
            // toggle booleans become true. Hydrate request values after mount.
            phaseTwoTimer = setTimeout(() => {
                editForm.setFieldsValue({
                    cpu_request: hydrated.cpu_request,
                    memory_request_gi: hydrated.memory_request_gi,
                });
            }, 0);
        }, 0);
        return () => {
            clearTimeout(timer);
            if (phaseTwoTimer) {
                clearTimeout(phaseTwoTimer);
            }
        };
    }, [editForm, editOpen, editingItem]);

    const openDeleteModal = (item: InstanceSize) => {
        setDeletingItem(item);
        setDeleteOpen(true);
    };

    const submitCreate = async () => {
        const values = await createForm.validateFields();
        const payload = formToPayload(values, 'create');
        createMutation.mutate(payload as unknown as InstanceSizeCreateRequest);
    };

    const submitEdit = async () => {
        if (!editingItem) {
            return;
        }
        const values = await editForm.validateFields();
        const payload = formToPayload(values, 'update');
        updateMutation.mutate({
            id: editingItem.id,
            body: payload as unknown as InstanceSizeUpdateRequest,
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
        listError: instanceSizesQuery.error,
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

function normalizeStringList(values: string[] | undefined): string[] {
    if (!Array.isArray(values) || values.length === 0) {
        return [];
    }
    const seen = new Set<string>();
    const normalized: string[] = [];
    for (const rawValue of values) {
        const value = typeof rawValue === 'string' ? rawValue.trim() : '';
        if (!value || seen.has(value)) {
            continue;
        }
        seen.add(value);
        normalized.push(value);
    }
    return normalized;
}

function getRootVolumeModeLabels(record: InstanceSize): string[] {
    if (!record.dv_volume_mode || !record.dv_access_modes?.length) {
        return ['Root Volume Auto'];
    }
    return [`Root Volume ${record.dv_volume_mode} ${record.dv_access_modes.join('/')}`];
}
