'use client';

import { App, Form } from 'antd';
import type { TFunction } from 'i18next';
import { useEffect, useMemo, useState } from 'react';

import { useApiAction, useApiGet, useApiMutation } from '@/hooks/useApiQuery';
import { SETUP_GUIDE_INVALIDATION_KEYS } from '@/features/setup-guide/queryKeys';
import { api } from '@/lib/api/client';
import { translateApiError } from '@/lib/api/errorMessage';

import type { Template, TemplateCreateRequest, TemplateList, TemplateUpdateRequest } from '../types';

interface UseAdminTemplatesControllerArgs {
    t: TFunction;
    onCreateSuccess?: (template: Template, context: { isFirstTemplate: boolean }) => boolean | void;
}

interface TemplateSearchFilters {
    search: string;
    osFamily: string;
    sourceType: string;
    catalogScope: string;
    enabled: '' | 'enabled' | 'disabled';
}

const CREATE_TEMPLATE_DEFAULTS: Pick<TemplateCreateRequest, 'catalog_scope' | 'enabled' | 'source_type'> = {
    catalog_scope: 'unclassified',
    enabled: true,
    source_type: 'cdi_image_import',
};

const EMPTY_TEMPLATE_SEARCH_FILTERS: TemplateSearchFilters = {
    search: '',
    osFamily: '',
    sourceType: '',
    catalogScope: '',
    enabled: '',
};

function normalizeTemplateSearchFilters(
    nextFilters: Partial<TemplateSearchFilters>,
): TemplateSearchFilters {
    return {
        search: nextFilters.search?.trim() ?? '',
        osFamily: nextFilters.osFamily?.trim() ?? '',
        sourceType: nextFilters.sourceType?.trim() ?? '',
        catalogScope: nextFilters.catalogScope?.trim() ?? '',
        enabled:
            nextFilters.enabled === 'enabled' || nextFilters.enabled === 'disabled'
                ? nextFilters.enabled
                : '',
    };
}

function buildEditTemplateFormValues(template: Template): TemplateUpdateRequest {
    return {
        display_name: template.display_name,
        description: template.description,
        catalog_scope: template.catalog_scope,
        os_family: template.os_family,
        os_version: template.os_version,
        enabled: template.enabled,
        source_type: template.source_type,
        image_url: template.source_type === 'cdi_pvc_clone' ? undefined : template.image_url,
        pvc_name: template.source_type === 'cdi_pvc_clone' ? template.pvc_name : undefined,
        pvc_namespace: template.source_type === 'cdi_pvc_clone' ? template.pvc_namespace : undefined,
        // cloud_init is the YAML cloud-init config (plain text, not JSON).
        // master-flow Step 3: admin can freely edit this YAML text.
        cloud_init: template.cloud_init,
    };
}

/**
 * master-flow Step 3: Configure Template
 *
 * Template fields per design:
 *   - name, display_name, description, os_family, os_version, enabled
 *   - source_type: 'containerdisk' | 'cdi_image_import' | 'cdi_pvc_clone'
 *   - image_url: Boot image/import URL (when source_type is not 'cdi_pvc_clone')
 *   - pvc_name:  Source PVC name        (when source_type='cdi_pvc_clone')
 *   - cloud_init: YAML cloud-init config (admin-editable plain text, NOT JSON)
 *
 * cloud_init is a first-class Template field — it is a plain YAML string stored
 * verbatim. The admin edits it directly in a monospace textarea. There is no
 * JSON spec or DynamicSchemaForm on the Template page (that belongs to InstanceSize,
 * Step 4, via spec_overrides).
 */


export function useAdminTemplatesController({
    t,
    onCreateSuccess,
}: UseAdminTemplatesControllerArgs) {
    const { message: messageApi } = App.useApp();
    const messageContextHolder = null;
    const [createOpen, setCreateOpen] = useState(false);
    const [editOpen, setEditOpen] = useState(false);
    const [deleteOpen, setDeleteOpen] = useState(false);
    const [createExperimentalSourcesEnabled, setCreateExperimentalSourcesEnabled] = useState(false);
    const [editExperimentalSourcesEnabled, setEditExperimentalSourcesEnabled] = useState(false);
    const [editingTemplate, setEditingTemplate] = useState<Template | null>(null);
    const [deletingTemplate, setDeletingTemplate] = useState<Template | null>(null);

    const [page, setPage] = useState(1);
    const [filters, setFilters] = useState<TemplateSearchFilters>(EMPTY_TEMPLATE_SEARCH_FILTERS);

    const [searchedColumn, setSearchedColumn] = useState('');
    const [searchText, setSearchText] = useState('');

    const [createForm] = Form.useForm<TemplateCreateRequest>();
    const [editForm] = Form.useForm<TemplateUpdateRequest>();

    const templatesQuery = useApiGet<TemplateList>(
        [
            'admin-templates',
            page,
            filters.search,
            filters.osFamily,
            filters.sourceType,
            filters.catalogScope,
            filters.enabled,
        ],
        () => api.GET('/admin/templates', {
            params: {
                query: {
                    page,
                    per_page: 20,
                    ...(filters.search ? { search: filters.search } : {}),
                    ...(filters.osFamily ? { os_family: filters.osFamily } : {}),
                    ...(filters.sourceType ? { source_type: filters.sourceType } : {}),
                    ...(filters.catalogScope ? { catalog_scope: filters.catalogScope } : {}),
                    ...(filters.enabled
                        ? { enabled: filters.enabled === 'enabled' }
                        : {}),
                },
            },
        })
    );
    const existingTemplatesTotal =
        templatesQuery.data?.pagination?.total ??
        templatesQuery.data?.items?.length ??
        0;
    const shouldContinueOnboarding = existingTemplatesTotal === 0;

    const createMutation = useApiMutation<TemplateCreateRequest, Template>(
        (body) => api.POST('/admin/templates', { body }),
        {
            invalidateKeys: [['admin-templates'], ...SETUP_GUIDE_INVALIDATION_KEYS],
            onSuccess: (template) => {
                setCreateOpen(false);
                createForm.resetFields();
                const handled = onCreateSuccess?.(template, {
                    isFirstTemplate: shouldContinueOnboarding,
                }) ?? false;
                if (handled) {
                    return;
                }
                messageApi.success(t('common:message.success'));
            },
            onError: (err) => messageApi.error(translateApiError(t, err)),
        }
    );

    const updateMutation = useApiMutation<{ id: string; body: TemplateUpdateRequest }, Template>(
        ({ id, body }) => api.PATCH('/admin/templates/{template_id}', {
            params: { path: { template_id: id } },
            body,
        }),
        {
            invalidateKeys: [['admin-templates'], ...SETUP_GUIDE_INVALIDATION_KEYS],
            onSuccess: () => {
                messageApi.success(t('common:message.success'));
                setEditOpen(false);
                setEditingTemplate(null);
                editForm.resetFields();
            },
            onError: (err) => messageApi.error(translateApiError(t, err)),
        }
    );

    const deleteMutation = useApiAction<string>(
        (id) => api.DELETE('/admin/templates/{template_id}', { params: { path: { template_id: id } } }),
        {
            invalidateKeys: [['admin-templates'], ...SETUP_GUIDE_INVALIDATION_KEYS],
            onSuccess: () => {
                messageApi.success(t('common:message.success'));
                setDeleteOpen(false);
                setDeletingTemplate(null);
            },
            onError: (err) => messageApi.error(translateApiError(t, err)),
        }
    );

    const osFamilyFilters = useMemo(() => {
        const families = new Set<string>();
        (templatesQuery.data?.items ?? []).forEach((template) => {
            if (template.os_family) {
                families.add(template.os_family);
            }
        });
        return Array.from(families).sort().map((family) => ({ text: family, value: family }));
    }, [templatesQuery.data?.items]);

    const filteredItems = useMemo(
        () => templatesQuery.data?.items ?? [],
        [templatesQuery.data?.items],
    );

    const applyFilters = (nextFilters: Partial<TemplateSearchFilters>) => {
        setFilters((current) =>
            normalizeTemplateSearchFilters({
                ...current,
                ...nextFilters,
            }),
        );
        setPage(1);
    };

    const clearFilters = () => {
        setFilters(EMPTY_TEMPLATE_SEARCH_FILTERS);
        setPage(1);
    };

    const openCreateModal = () => {
        setCreateExperimentalSourcesEnabled(false);
        setCreateOpen(true);
    };

    const openEditModal = (template: Template) => {
        setEditingTemplate(template);
        setEditExperimentalSourcesEnabled(template.source_type === 'containerdisk');
        setEditOpen(true);
    };

    useEffect(() => {
        if (!createOpen) {
            return;
        }

        createForm.resetFields();
        createForm.setFieldsValue(CREATE_TEMPLATE_DEFAULTS);
    }, [createForm, createOpen]);

    // Ant Design Modal keeps its content memoized while closed. Hydrate after the
    // modal opens so the mounted form store owns the latest dependent source fields.
    useEffect(() => {
        if (!editOpen || !editingTemplate) {
            return;
        }

        editForm.resetFields();
        editForm.setFieldsValue(buildEditTemplateFormValues(editingTemplate));
    }, [editForm, editOpen, editingTemplate]);

    const openDeleteModal = (template: Template) => {
        setDeletingTemplate(template);
        setDeleteOpen(true);
    };

    /**
     * Submit create: pass form values directly to the API.
     *
     * cloud_init is submitted as-is (YAML string). The source_type toggle
     * determines which of image_url / pvc_name is relevant — clear the other
     * to avoid sending stale data.
     *
     * master-flow Step 3: no spec JSON processing here. cloud_init is YAML.
     */
    const submitCreate = async () => {
        const values = await createForm.validateFields() as TemplateCreateRequest;
        const payload: TemplateCreateRequest = { ...values };
        if (payload.source_type === 'cdi_pvc_clone') {
            payload.image_url = undefined;
        } else {
            payload.pvc_name = undefined;
            payload.pvc_namespace = undefined;
        }
        createMutation.mutate(payload);
    };

    /**
     * Submit edit: same as create, cloud_init passed verbatim.
     */
    const submitEdit = async () => {
        if (!editingTemplate) {
            return;
        }
        const values = await editForm.validateFields() as TemplateUpdateRequest;
        const payload: TemplateUpdateRequest = { ...values };
        if (payload.source_type === 'cdi_pvc_clone') {
            payload.image_url = undefined;
        } else {
            payload.pvc_name = undefined;
            payload.pvc_namespace = undefined;
        }
        updateMutation.mutate({
            id: editingTemplate.id,
            body: payload,
        });
    };

    const submitDelete = () => {
        if (!deletingTemplate) {
            return;
        }
        deleteMutation.mutate(deletingTemplate.id);
    };

    return {
        messageContextHolder,
        page,
        setPage,
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
        osFamilyFilters,
        filteredItems,
        data: templatesQuery.data,
        isLoading: templatesQuery.isLoading,
        refetch: templatesQuery.refetch,
        createOpen,
        editOpen,
        deleteOpen,
        editingTemplate,
        deletingTemplate,
        createForm,
        editForm,
        openCreateModal,
        openEditModal,
        openDeleteModal,
        closeCreateModal: () => {
            setCreateOpen(false);
            setCreateExperimentalSourcesEnabled(false);
            createForm.resetFields();
        },
        closeEditModal: () => {
            setEditOpen(false);
            setEditExperimentalSourcesEnabled(false);
            setEditingTemplate(null);
            editForm.resetFields();
        },
        closeDeleteModal: () => {
            setDeleteOpen(false);
            setDeletingTemplate(null);
        },
        submitCreate,
        submitEdit,
        submitDelete,
        createExperimentalSourcesEnabled,
        editExperimentalSourcesEnabled,
        enableCreateExperimentalSources: () => setCreateExperimentalSourcesEnabled(true),
        enableEditExperimentalSources: () => setEditExperimentalSourcesEnabled(true),
        createPending: createMutation.isPending,
        updatePending: updateMutation.isPending,
        deletePending: deleteMutation.isPending,
    };
}
