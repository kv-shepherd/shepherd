'use client';

import { Form, message } from 'antd';
import type { TFunction } from 'i18next';
import { useMemo, useState } from 'react';

import { useApiAction, useApiGet, useApiMutation } from '@/hooks/useApiQuery';
import { api } from '@/lib/api/client';

import type {
    AuthProvider,
    AuthProviderConnectionTestResult,
    AuthProviderCreateRequest,
    AuthProviderGroupSyncRequest,
    AuthProviderList,
    AuthProviderType,
    AuthProviderTypeList,
    AuthProviderSampleResponse,
    AuthProviderUpdateRequest,
    IdPGroupMapping,
    IdPGroupMappingCreateRequest,
    IdPGroupMappingList,
    IdPGroupMappingUpdateRequest,
    Role,
    RoleList,
} from '../types';

interface UseAdminAuthProvidersControllerArgs {
    t: TFunction;
}

interface CreateFormValues {
    name: string;
    auth_type: AuthProvider['auth_type'];
    enabled?: boolean;
    sort_order?: number;
    config_text?: string;
}

interface EditFormValues {
    name?: string;
    enabled?: boolean;
    sort_order?: number;
    config_text?: string;
}

interface SyncFormValues {
    source_field: string;
    groups_text: string;
}

interface MappingFormValues {
    external_group_id: string;
    group_name?: string;
    role_id: string;
    scope_type?: string;
    scope_id?: string;
    allowed_environments?: Array<'test' | 'prod'>;
}

interface MappingEditFormValues {
    role_id?: string;
    scope_type?: string;
    scope_id?: string;
    allowed_environments?: Array<'test' | 'prod'>;
}

function parseConfigJSON(raw: string, onError: () => void): Record<string, unknown> | undefined {
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

function parseGroupsText(raw: string): string[] {
    const seen = new Set<string>();
    for (const token of raw.split(/[\n,]/g)) {
        const value = token.trim();
        if (!value) continue;
        seen.add(value);
    }
    return Array.from(seen.values()).sort((a, b) => a.localeCompare(b));
}

export function useAdminAuthProvidersController({ t }: UseAdminAuthProvidersControllerArgs) {
    const [messageApi, messageContextHolder] = message.useMessage();

    const [createOpen, setCreateOpen] = useState(false);
    const [editOpen, setEditOpen] = useState(false);
    const [deleteOpen, setDeleteOpen] = useState(false);
    const [mappingOpen, setMappingOpen] = useState(false);
    const [editMappingOpen, setEditMappingOpen] = useState(false);

    const [editingProvider, setEditingProvider] = useState<AuthProvider | null>(null);
    const [deletingProvider, setDeletingProvider] = useState<AuthProvider | null>(null);
    const [mappingProvider, setMappingProvider] = useState<AuthProvider | null>(null);
    const [editingMapping, setEditingMapping] = useState<IdPGroupMapping | null>(null);
    const [testingProviderId, setTestingProviderId] = useState<string>('');

    const [createForm] = Form.useForm<CreateFormValues>();
    const [editForm] = Form.useForm<EditFormValues>();
    const [syncForm] = Form.useForm<SyncFormValues>();
    const [mappingForm] = Form.useForm<MappingFormValues>();
    const [mappingEditForm] = Form.useForm<MappingEditFormValues>();

    const providersQuery = useApiGet<AuthProviderList>(
        ['admin-auth-providers'],
        () => api.GET('/admin/auth-providers')
    );

    const providerTypesQuery = useApiGet<AuthProviderTypeList>(
        ['admin-auth-provider-types'],
        () => api.GET('/admin/auth-provider-types')
    );

    const rolesQuery = useApiGet<RoleList>(
        ['admin-auth-provider-roles'],
        () => api.GET('/admin/roles')
    );

    const sampleQuery = useApiGet<AuthProviderSampleResponse>(
        ['admin-auth-provider-sample', mappingProvider?.id ?? ''],
        () => api.GET('/admin/auth-providers/{provider_id}/sample', {
            params: { path: { provider_id: mappingProvider?.id ?? '' } },
        }),
        { enabled: mappingOpen && !!mappingProvider?.id }
    );

    const mappingsQuery = useApiGet<IdPGroupMappingList>(
        ['admin-auth-provider-mappings', mappingProvider?.id ?? ''],
        () => api.GET('/admin/auth-providers/{provider_id}/group-mappings', {
            params: { path: { provider_id: mappingProvider?.id ?? '' } },
        }),
        { enabled: mappingOpen && !!mappingProvider?.id }
    );

    const createMutation = useApiMutation<AuthProviderCreateRequest, AuthProvider>(
        (body) => api.POST('/admin/auth-providers', { body }),
        {
            invalidateKeys: [['admin-auth-providers']],
            onSuccess: () => {
                messageApi.success(t('common:message.success'));
                setCreateOpen(false);
                createForm.resetFields();
            },
            onError: (err) => messageApi.error(err.message || t('common:message.error')),
        }
    );

    const updateMutation = useApiMutation<{ providerId: string; body: AuthProviderUpdateRequest }, AuthProvider>(
        ({ providerId, body }) => api.PATCH('/admin/auth-providers/{provider_id}', {
            params: { path: { provider_id: providerId } },
            body,
        }),
        {
            invalidateKeys: [['admin-auth-providers']],
            onSuccess: () => {
                messageApi.success(t('common:message.success'));
                setEditOpen(false);
                setEditingProvider(null);
                editForm.resetFields();
            },
            onError: (err) => messageApi.error(err.message || t('common:message.error')),
        }
    );

    const deleteMutation = useApiAction<string>(
        (providerId) => api.DELETE('/admin/auth-providers/{provider_id}', {
            params: { path: { provider_id: providerId } },
        }),
        {
            invalidateKeys: [['admin-auth-providers']],
            onSuccess: () => {
                messageApi.success(t('common:message.success'));
                setDeleteOpen(false);
                setDeletingProvider(null);
            },
            onError: (err) => messageApi.error(err.message || t('common:message.error')),
        }
    );

    const testConnectionMutation = useApiMutation<{ providerId: string }, AuthProviderConnectionTestResult>(
        ({ providerId }) => api.POST('/admin/auth-providers/{provider_id}/test-connection', {
            params: { path: { provider_id: providerId } },
        }),
        {
            onSuccess: (resp) => {
                if (resp.success) {
                    messageApi.success(resp.message || t('authProviders.test_success'));
                } else {
                    messageApi.error(resp.message || t('authProviders.test_failed'));
                }
                setTestingProviderId('');
            },
            onError: (err) => {
                setTestingProviderId('');
                messageApi.error(err.message || t('common:message.error'));
            },
        }
    );

    const syncGroupsMutation = useApiMutation<{ providerId: string; body: AuthProviderGroupSyncRequest }, unknown>(
        ({ providerId, body }) => api.POST('/admin/auth-providers/{provider_id}/sync', {
            params: { path: { provider_id: providerId } },
            body,
        }),
        {
            invalidateKeys: [
                ['admin-auth-provider-sample', mappingProvider?.id ?? ''],
                ['admin-auth-provider-mappings', mappingProvider?.id ?? ''],
            ],
            onSuccess: () => messageApi.success(t('common:message.success')),
            onError: (err) => messageApi.error(err.message || t('common:message.error')),
        }
    );

    const createMappingMutation = useApiMutation<{ providerId: string; body: IdPGroupMappingCreateRequest }, IdPGroupMapping>(
        ({ providerId, body }) => api.POST('/admin/auth-providers/{provider_id}/group-mappings', {
            params: { path: { provider_id: providerId } },
            body,
        }),
        {
            invalidateKeys: [['admin-auth-provider-mappings', mappingProvider?.id ?? '']],
            onSuccess: () => {
                messageApi.success(t('common:message.success'));
                mappingForm.resetFields();
                mappingForm.setFieldsValue({ scope_type: 'global' });
            },
            onError: (err) => messageApi.error(err.message || t('common:message.error')),
        }
    );

    const updateMappingMutation = useApiMutation<{
        providerId: string;
        mappingId: string;
        body: IdPGroupMappingUpdateRequest;
    }, IdPGroupMapping>(
        ({ providerId, mappingId, body }) => api.PATCH('/admin/auth-providers/{provider_id}/group-mappings/{mapping_id}', {
            params: { path: { provider_id: providerId, mapping_id: mappingId } },
            body,
        }),
        {
            invalidateKeys: [['admin-auth-provider-mappings', mappingProvider?.id ?? '']],
            onSuccess: () => {
                messageApi.success(t('common:message.success'));
                setEditMappingOpen(false);
                setEditingMapping(null);
                mappingEditForm.resetFields();
            },
            onError: (err) => messageApi.error(err.message || t('common:message.error')),
        }
    );

    const deleteMappingMutation = useApiAction<{ providerId: string; mappingId: string }>(
        ({ providerId, mappingId }) => api.DELETE('/admin/auth-providers/{provider_id}/group-mappings/{mapping_id}', {
            params: { path: { provider_id: providerId, mapping_id: mappingId } },
        }),
        {
            invalidateKeys: [['admin-auth-provider-mappings', mappingProvider?.id ?? '']],
            onSuccess: () => messageApi.success(t('common:message.success')),
            onError: (err) => messageApi.error(err.message || t('common:message.error')),
        }
    );

    const providers = useMemo<AuthProvider[]>(() => providersQuery.data?.items ?? [], [providersQuery.data?.items]);
    const providerTypes = useMemo<AuthProviderType[]>(
        () => providerTypesQuery.data?.items ?? [],
        [providerTypesQuery.data?.items]
    );
    const providerTypeOptions = useMemo(
        () =>
            providerTypes.map((item) => ({
                value: item.type,
                label: item.display_name || item.type,
            })),
        [providerTypes]
    );
    const providerTypeLabelByKey = useMemo<Record<string, string>>(
        () =>
            providerTypes.reduce<Record<string, string>>((acc, item) => {
                acc[item.type] = item.display_name || item.type;
                return acc;
            }, {}),
        [providerTypes]
    );
    const roles = useMemo<Role[]>(() => rolesQuery.data?.items ?? [], [rolesQuery.data?.items]);
    const roleOptions = useMemo(
        () => roles.map((role) => ({ value: role.id, label: role.display_name || role.name })),
        [roles]
    );
    const sampleFields = useMemo(() => sampleQuery.data?.fields ?? [], [sampleQuery.data?.fields]);
    const mappings = useMemo(() => mappingsQuery.data?.items ?? [], [mappingsQuery.data?.items]);

    const openCreateModal = () => {
        createForm.resetFields();
        const defaultAuthType = providerTypeOptions[0]?.value;
        createForm.setFieldsValue({
            auth_type: defaultAuthType,
            enabled: true,
            sort_order: 0,
            config_text: '{}',
        });
        setCreateOpen(true);
    };

    const closeCreateModal = () => {
        setCreateOpen(false);
        createForm.resetFields();
    };

    const submitCreate = async () => {
        const values = await createForm.validateFields();
        const config = parseConfigJSON(values.config_text ?? '', () => {
            messageApi.error(t('authProviders.config_invalid'));
        });
        if (values.config_text && !config) {
            return;
        }

        createMutation.mutate({
            name: values.name,
            auth_type: values.auth_type,
            enabled: values.enabled,
            sort_order: values.sort_order,
            config,
        });
    };

    const openEditModal = (provider: AuthProvider) => {
        setEditingProvider(provider);
        editForm.setFieldsValue({
            name: provider.name,
            enabled: provider.enabled,
            sort_order: provider.sort_order,
            config_text: JSON.stringify(provider.config ?? {}, null, 2),
        });
        setEditOpen(true);
    };

    const closeEditModal = () => {
        setEditOpen(false);
        setEditingProvider(null);
        editForm.resetFields();
    };

    const submitEdit = async () => {
        if (!editingProvider) {
            return;
        }
        const values = await editForm.validateFields();
        const config = parseConfigJSON(values.config_text ?? '', () => {
            messageApi.error(t('authProviders.config_invalid'));
        });
        if (values.config_text && !config) {
            return;
        }

        updateMutation.mutate({
            providerId: editingProvider.id,
            body: {
                name: values.name,
                enabled: values.enabled,
                sort_order: values.sort_order,
                config,
            },
        });
    };

    const openDeleteModal = (provider: AuthProvider) => {
        setDeletingProvider(provider);
        setDeleteOpen(true);
    };

    const closeDeleteModal = () => {
        setDeleteOpen(false);
        setDeletingProvider(null);
    };

    const submitDelete = () => {
        if (!deletingProvider) {
            return;
        }
        deleteMutation.mutate(deletingProvider.id);
    };

    const testConnection = (provider: AuthProvider) => {
        setTestingProviderId(provider.id);
        testConnectionMutation.mutate({ providerId: provider.id });
    };

    const openMappingModal = (provider: AuthProvider) => {
        setMappingProvider(provider);
        mappingForm.resetFields();
        mappingEditForm.resetFields();
        syncForm.resetFields();
        syncForm.setFieldsValue({ source_field: 'groups' });
        mappingForm.setFieldsValue({ scope_type: 'global' });
        setMappingOpen(true);
    };

    const closeMappingModal = () => {
        setMappingOpen(false);
        setMappingProvider(null);
        setEditMappingOpen(false);
        setEditingMapping(null);
        mappingForm.resetFields();
        mappingEditForm.resetFields();
        syncForm.resetFields();
    };

    const submitSyncGroups = async () => {
        if (!mappingProvider) {
            return;
        }
        const values = await syncForm.validateFields();
        const groups = parseGroupsText(values.groups_text);
        if (groups.length === 0) {
            messageApi.error(t('authProviders.groups_required'));
            return;
        }

        syncGroupsMutation.mutate({
            providerId: mappingProvider.id,
            body: {
                source_field: values.source_field.trim(),
                groups,
            },
        });
    };

    const submitCreateMapping = async () => {
        if (!mappingProvider) {
            return;
        }
        const values = await mappingForm.validateFields();
        createMappingMutation.mutate({
            providerId: mappingProvider.id,
            body: {
                external_group_id: values.external_group_id.trim(),
                group_name: values.group_name?.trim() || undefined,
                role_id: values.role_id,
                scope_type: values.scope_type?.trim() || 'global',
                scope_id: values.scope_id?.trim() || undefined,
                allowed_environments: values.allowed_environments,
            },
        });
    };

    const openEditMappingModal = (mapping: IdPGroupMapping) => {
        setEditingMapping(mapping);
        mappingEditForm.setFieldsValue({
            role_id: mapping.role_id,
            scope_type: mapping.scope_type || 'global',
            scope_id: mapping.scope_id,
            allowed_environments: mapping.allowed_environments as Array<'test' | 'prod'>,
        });
        setEditMappingOpen(true);
    };

    const closeEditMappingModal = () => {
        setEditMappingOpen(false);
        setEditingMapping(null);
        mappingEditForm.resetFields();
    };

    const submitEditMapping = async () => {
        if (!mappingProvider || !editingMapping) {
            return;
        }
        const values = await mappingEditForm.validateFields();
        updateMappingMutation.mutate({
            providerId: mappingProvider.id,
            mappingId: editingMapping.id,
            body: {
                role_id: values.role_id,
                scope_type: values.scope_type?.trim() || undefined,
                scope_id: values.scope_id?.trim() || undefined,
                allowed_environments: values.allowed_environments,
            },
        });
    };

    const deleteMapping = (mapping: IdPGroupMapping) => {
        if (!mappingProvider) {
            return;
        }
        deleteMappingMutation.mutate({
            providerId: mappingProvider.id,
            mappingId: mapping.id,
        });
    };

    return {
        messageContextHolder,
        providers,
        providersLoading: providersQuery.isLoading,
        refetchProviders: providersQuery.refetch,
        providerTypes,
        providerTypesLoading: providerTypesQuery.isLoading,
        providerTypeOptions,
        providerTypeLabelByKey,

        createOpen,
        editOpen,
        deleteOpen,
        mappingOpen,
        editMappingOpen,
        editingProvider,
        deletingProvider,
        mappingProvider,
        editingMapping,
        testingProviderId,

        createForm,
        editForm,
        syncForm,
        mappingForm,
        mappingEditForm,

        openCreateModal,
        closeCreateModal,
        submitCreate,
        openEditModal,
        closeEditModal,
        submitEdit,
        openDeleteModal,
        closeDeleteModal,
        submitDelete,
        testConnection,

        openMappingModal,
        closeMappingModal,
        submitSyncGroups,
        submitCreateMapping,
        openEditMappingModal,
        closeEditMappingModal,
        submitEditMapping,
        deleteMapping,

        sampleFields,
        sampleLoading: sampleQuery.isLoading,
        mappings,
        mappingsLoading: mappingsQuery.isLoading,
        roleOptions,

        createPending: createMutation.isPending,
        updatePending: updateMutation.isPending,
        deletePending: deleteMutation.isPending,
        testConnectionPending: testConnectionMutation.isPending,
        syncGroupsPending: syncGroupsMutation.isPending,
        createMappingPending: createMappingMutation.isPending,
        updateMappingPending: updateMappingMutation.isPending,
        deleteMappingPending: deleteMappingMutation.isPending,
    };
}
