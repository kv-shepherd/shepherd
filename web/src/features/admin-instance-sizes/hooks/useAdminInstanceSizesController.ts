'use client';

import { App, Form } from 'antd';
import type { TFunction } from 'i18next';
import { useEffect, useMemo, useState } from 'react';

import { useApiAction, useApiGet, useApiMutation } from '@/hooks/useApiQuery';
import { SETUP_GUIDE_INVALIDATION_KEYS } from '@/features/setup-guide/queryKeys';
import { api } from '@/lib/api/client';
import { translateApiError } from '@/lib/api/errorMessage';
import {
    HUGEPAGES_PAGE_SIZE_PATH,
    normalizeHugepagesPageSizeValue,
} from '@/lib/hugepages';

import {
    hasDedicatedCPURequirement,
    getCapabilityLabels,
    hasCPUOvercommit,
    hasMemoryOvercommit,
    type InstanceSize,
    type InstanceSizeCreateRequest,
    type InstanceSizeList,
    type InstanceSizeUpdateRequest,
} from '../types';
import {
    getSpecOverrideValue,
    normalizeInstanceSizeSpecOverrides,
    setNestedValue,
} from '../specOverrides';

interface UseAdminInstanceSizesControllerArgs {
    t: TFunction;
    onCreateSuccess?: (instanceSize: InstanceSize, context: { isFirstInstanceSize: boolean }) => boolean | void;
}

interface InstanceSizeSearchFilters {
    search: string;
    catalogScope: string;
    enabled: '' | 'enabled' | 'disabled';
    publication: '' | 'ready' | 'hidden' | 'disabled';
    capability:
        | ''
        | 'gpu'
        | 'sriov'
        | 'hugepages'
        | 'dedicated_cpu'
        | 'cpu_overcommit'
        | 'memory_overcommit';
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

const INSTANCE_SIZE_CREATE_INITIAL_VALUES: Partial<InstanceSizeFormValues> = {
    catalog_scope: 'unclassified',
    enabled: true,
    sort_order: 0,
    dedicated_cpu: false,
    spec_text: '{}',
    root_volume_mode_intent: 'auto',
    dv_access_modes: undefined,
    dv_volume_mode: undefined,
};

const EMPTY_INSTANCE_SIZE_SEARCH_FILTERS: InstanceSizeSearchFilters = {
    search: '',
    catalogScope: '',
    enabled: '',
    publication: '',
    capability: '',
};

function normalizeInstanceSizeSearchFilters(
    nextFilters: Partial<InstanceSizeSearchFilters>,
): InstanceSizeSearchFilters {
    return {
        search: nextFilters.search?.trim() ?? '',
        catalogScope: nextFilters.catalogScope?.trim() ?? '',
        enabled:
            nextFilters.enabled === 'enabled' || nextFilters.enabled === 'disabled'
                ? nextFilters.enabled
                : '',
        publication:
            nextFilters.publication === 'ready' ||
            nextFilters.publication === 'hidden' ||
            nextFilters.publication === 'disabled'
                ? nextFilters.publication
                : '',
        capability:
            nextFilters.capability === 'gpu' ||
            nextFilters.capability === 'sriov' ||
            nextFilters.capability === 'hugepages' ||
            nextFilters.capability === 'dedicated_cpu' ||
            nextFilters.capability === 'cpu_overcommit' ||
            nextFilters.capability === 'memory_overcommit'
                ? nextFilters.capability
                : '',
    };
}

function resolveHugepagesRequirement(record: Pick<InstanceSize, 'requires_hugepages' | 'hugepages_size' | 'spec_overrides'>) {
    const normalizedSpecOverrides = normalizeInstanceSizeSpecOverrides(
        record.spec_overrides as Record<string, unknown> | undefined,
    );
    const indexedSize = normalizeHugepagesPageSizeValue(record.hugepages_size);
    const specSize = normalizeHugepagesPageSizeValue(
        getSpecOverrideValue(normalizedSpecOverrides, HUGEPAGES_PAGE_SIZE_PATH),
    );
    const hugepagesSize = indexedSize ?? specSize;
    return {
        requiresHugepages: record.requires_hugepages === true || Boolean(hugepagesSize),
        hugepagesSize,
    };
}

function hydrateSpecOverridesForEditing(instanceSize: InstanceSize): Record<string, unknown> {
    const specOverrides = normalizeInstanceSizeSpecOverrides(
        instanceSize.spec_overrides as Record<string, unknown> | undefined,
    );
    const { hugepagesSize } = resolveHugepagesRequirement(instanceSize);
    if (hugepagesSize && getSpecOverrideValue(specOverrides, HUGEPAGES_PAGE_SIZE_PATH) === undefined) {
        setNestedValue(specOverrides, HUGEPAGES_PAGE_SIZE_PATH, hugepagesSize);
    }
    return specOverrides;
}

export function resolveInstanceSizePublicationState(record: Pick<InstanceSize, 'enabled' | 'catalog_scope'>) {
    if (record.enabled === false) {
        return 'disabled';
    }
    if ((record.catalog_scope ?? 'unclassified') === 'unclassified') {
        return 'hidden';
    }
    return 'ready';
}

export function matchesInstanceSizeCapabilityFilter(
    record: InstanceSize,
    capability: InstanceSizeSearchFilters['capability'],
) {
    const hugepages = resolveHugepagesRequirement(record);
    switch (capability) {
        case '':
            return true;
        case 'gpu':
            return record.requires_gpu || getCapabilityLabels(record).some((label) => label.startsWith('GPU '));
        case 'sriov':
            return record.requires_sriov === true;
        case 'hugepages':
            return hugepages.requiresHugepages;
        case 'dedicated_cpu':
            return hasDedicatedCPURequirement(record);
        case 'cpu_overcommit':
            return hasCPUOvercommit(record);
        case 'memory_overcommit':
            return hasMemoryOvercommit(record);
        default:
            return true;
    }
}

export function filterAdminInstanceSizes(
    items: InstanceSize[],
    filters: InstanceSizeSearchFilters,
) {
    const search = filters.search.toLowerCase();
    return items.filter((instanceSize) => {
        if (search) {
            const searchTokens = [
                instanceSize.id,
                instanceSize.name,
                instanceSize.display_name ?? '',
                instanceSize.description ?? '',
                instanceSize.catalog_scope ?? '',
                instanceSize.enabled === false ? 'disabled' : 'enabled',
                resolveInstanceSizePublicationState(instanceSize),
                String(instanceSize.cpu_cores),
                String(instanceSize.memory_gi),
                String(instanceSize.disk_gb ?? ''),
                ...getCapabilityLabels(instanceSize),
                ...(instanceSize.dv_access_modes ?? []),
                instanceSize.dv_volume_mode ?? '',
            ]
                .join(' ')
                .toLowerCase();
            if (!searchTokens.includes(search)) {
                return false;
            }
        }
        if (filters.catalogScope && instanceSize.catalog_scope !== filters.catalogScope) {
            return false;
        }
        if (filters.enabled) {
            const isEnabled = instanceSize.enabled !== false;
            if (filters.enabled === 'enabled' && !isEnabled) {
                return false;
            }
            if (filters.enabled === 'disabled' && isEnabled) {
                return false;
            }
        }
        if (filters.publication && resolveInstanceSizePublicationState(instanceSize) !== filters.publication) {
            return false;
        }
        if (!matchesInstanceSizeCapabilityFilter(instanceSize, filters.capability)) {
            return false;
        }
        return true;
    });
}


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
                specOverrides = normalizeInstanceSizeSpecOverrides(parsed as Record<string, unknown>);
            }
        } catch {
            // Malformed spec_text — ignore, send without spec_overrides
        }
    }
    const hugepagesSize = normalizeHugepagesPageSizeValue(
        getSpecOverrideValue(specOverrides, HUGEPAGES_PAGE_SIZE_PATH),
    );

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
        requires_hugepages: Boolean(hugepagesSize),
        hugepages_size: hugepagesSize,
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

function instanceSizeToFormValues(instanceSize: InstanceSize): InstanceSizeFormValues {
    const hydrated = instanceSize as InstanceSize & {
        cpu_request?: number;
        memory_request_gi?: number;
        sort_order?: number;
    };
    const dedicatedCPU = hasDedicatedCPURequirement(instanceSize);

    return {
        name: instanceSize.name,
        display_name: instanceSize.display_name,
        description: instanceSize.description,
        catalog_scope: instanceSize.catalog_scope,
        cpu_cores: instanceSize.cpu_cores,
        memory_gi: instanceSize.memory_gi,
        disk_gb: instanceSize.disk_gb,
        dedicated_cpu: dedicatedCPU,
        cpu_overcommit_enabled: !dedicatedCPU && typeof hydrated.cpu_request === 'number' && hydrated.cpu_request > 0,
        memory_overcommit_enabled: typeof hydrated.memory_request_gi === 'number' && hydrated.memory_request_gi > 0,
        cpu_request: hydrated.cpu_request,
        memory_request_gi: hydrated.memory_request_gi,
        requires_sriov: instanceSize.requires_sriov,
        root_volume_mode_intent:
            (instanceSize.dv_access_modes?.length ?? 0) > 0 || instanceSize.dv_volume_mode
                ? 'explicit'
                : 'auto',
        dv_access_modes: instanceSize.dv_access_modes,
        dv_volume_mode: instanceSize.dv_volume_mode,
        sort_order: hydrated.sort_order,
        spec_text: JSON.stringify(hydrateSpecOverridesForEditing(instanceSize), null, 2),
        enabled: instanceSize.enabled,
    };
}

export function useAdminInstanceSizesController({
    t,
    onCreateSuccess,
}: UseAdminInstanceSizesControllerArgs) {
    const { message: messageApi } = App.useApp();
    const messageContextHolder = null;
    const [filters, setFilters] = useState<InstanceSizeSearchFilters>(EMPTY_INSTANCE_SIZE_SEARCH_FILTERS);

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
    const existingInstanceSizesTotal =
        instanceSizesQuery.data?.items?.length ??
        0;
    const shouldContinueOnboarding = existingInstanceSizesTotal === 0;

    const createMutation = useApiMutation<InstanceSizeCreateRequest, InstanceSize>(
        (body) => api.POST('/admin/instance-sizes', { body }),
        {
            invalidateKeys: [['admin-instance-sizes'], ...SETUP_GUIDE_INVALIDATION_KEYS],
            onSuccess: (instanceSize) => {
                setCreateOpen(false);
                createForm.resetFields();
                const handled = onCreateSuccess?.(instanceSize, {
                    isFirstInstanceSize: shouldContinueOnboarding,
                }) ?? false;
                if (handled) {
                    return;
                }
                messageApi.success(t('common:message.success'));
            },
            onError: (err) => messageApi.error(translateApiError(t, err)),
        }
    );

    const updateMutation = useApiMutation<{ id: string; body: InstanceSizeUpdateRequest }, InstanceSize>(
        ({ id, body }) => api.PATCH('/admin/instance-sizes/{instance_size_id}', {
            params: { path: { instance_size_id: id } },
            body,
        }),
        {
            invalidateKeys: [['admin-instance-sizes'], ...SETUP_GUIDE_INVALIDATION_KEYS],
            onSuccess: () => {
                messageApi.success(t('common:message.success'));
                setEditOpen(false);
                setEditingItem(null);
                editForm.resetFields();
            },
            onError: (err) => messageApi.error(translateApiError(t, err)),
        }
    );

    const deleteMutation = useApiAction<string>(
        (id) => api.DELETE('/admin/instance-sizes/{instance_size_id}', { params: { path: { instance_size_id: id } } }),
        {
            invalidateKeys: [['admin-instance-sizes'], ...SETUP_GUIDE_INVALIDATION_KEYS],
            onSuccess: () => {
                messageApi.success(t('common:message.success'));
                setDeleteOpen(false);
                setDeletingItem(null);
            },
            onError: (err) => messageApi.error(translateApiError(t, err)),
        }
    );

    const filteredItems = useMemo(() => {
        const items = instanceSizesQuery.data?.items ?? [];
        return filterAdminInstanceSizes(items, filters);
    }, [instanceSizesQuery.data?.items, filters]);
    const editInitialValues = useMemo(
        () => (editingItem ? instanceSizeToFormValues(editingItem) : undefined),
        [editingItem]
    );

    const applyFilters = (nextFilters: Partial<InstanceSizeSearchFilters>) => {
        setFilters((current) =>
            normalizeInstanceSizeSearchFilters({
                ...current,
                ...nextFilters,
            }),
        );
    };

    const clearFilters = () => {
        setFilters(EMPTY_INSTANCE_SIZE_SEARCH_FILTERS);
    };

    const openCreateModal = () => {
        setCreateOpen(true);
    };

    const openEditModal = (item: InstanceSize) => {
        setEditingItem(item);
        setEditOpen(true);
    };

    const openDeleteModal = (item: InstanceSize) => {
        setDeletingItem(item);
        setDeleteOpen(true);
    };

    useEffect(() => {
        if (!createOpen) {
            return;
        }
        createForm.resetFields();
        createForm.setFieldsValue(INSTANCE_SIZE_CREATE_INITIAL_VALUES);
    }, [createForm, createOpen]);

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
        filters,
        hasActiveFilters: Object.values(filters).some((value) => value !== ''),
        applyFilters,
        clearFilters,
        globalSearch: filters.search,
        setGlobalSearch: (value: string) => applyFilters({ search: value }),
        deferredSearch: filters.search,
        isStale: false,
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
        createInitialValues: INSTANCE_SIZE_CREATE_INITIAL_VALUES,
        editInitialValues,
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
