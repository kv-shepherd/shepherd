'use client';

import { Form, message } from 'antd';
import type { TFunction } from 'i18next';
import { useDeferredValue, useMemo, useState } from 'react';

import { useApiAction, useApiGet, useApiMutation } from '@/hooks/useApiQuery';
import { api } from '@/lib/api/client';

import type { Template, TemplateCreateRequest, TemplateList, TemplateUpdateRequest } from '../types';

interface UseAdminTemplatesControllerArgs {
    t: TFunction;
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


export function useAdminTemplatesController({ t }: UseAdminTemplatesControllerArgs) {
    const [messageApi, messageContextHolder] = message.useMessage();
    const [createOpen, setCreateOpen] = useState(false);
    const [editOpen, setEditOpen] = useState(false);
    const [deleteOpen, setDeleteOpen] = useState(false);
    const [createExperimentalSourcesEnabled, setCreateExperimentalSourcesEnabled] = useState(false);
    const [editExperimentalSourcesEnabled, setEditExperimentalSourcesEnabled] = useState(false);
    const [editingTemplate, setEditingTemplate] = useState<Template | null>(null);
    const [deletingTemplate, setDeletingTemplate] = useState<Template | null>(null);

    const [page, setPage] = useState(1);
    const [globalSearch, setGlobalSearch] = useState('');
    const deferredSearch = useDeferredValue(globalSearch);
    const isStale = globalSearch !== deferredSearch;

    const [searchedColumn, setSearchedColumn] = useState('');
    const [searchText, setSearchText] = useState('');

    const [createForm] = Form.useForm<TemplateCreateRequest>();
    const [editForm] = Form.useForm<TemplateUpdateRequest>();

    const templatesQuery = useApiGet<TemplateList>(
        ['admin-templates', page],
        () => api.GET('/admin/templates', {
            params: { query: { page, per_page: 20 } },
        })
    );

    const createMutation = useApiMutation<TemplateCreateRequest, Template>(
        (body) => api.POST('/admin/templates', { body }),
        {
            invalidateKeys: [['admin-templates']],
            onSuccess: () => {
                messageApi.success(t('common:message.success'));
                setCreateOpen(false);
                createForm.resetFields();
            },
            onError: (err) => messageApi.error(err.message || t('common:message.error')),
        }
    );

    const updateMutation = useApiMutation<{ id: string; body: TemplateUpdateRequest }, Template>(
        ({ id, body }) => api.PATCH('/admin/templates/{template_id}', {
            params: { path: { template_id: id } },
            body,
        }),
        {
            invalidateKeys: [['admin-templates']],
            onSuccess: () => {
                messageApi.success(t('common:message.success'));
                setEditOpen(false);
                setEditingTemplate(null);
                editForm.resetFields();
            },
            onError: (err) => messageApi.error(err.message || t('common:message.error')),
        }
    );

    const deleteMutation = useApiAction<string>(
        (id) => api.DELETE('/admin/templates/{template_id}', { params: { path: { template_id: id } } }),
        {
            invalidateKeys: [['admin-templates']],
            onSuccess: () => {
                messageApi.success(t('common:message.success'));
                setDeleteOpen(false);
                setDeletingTemplate(null);
            },
            onError: (err) => messageApi.error(err.message || t('common:message.error')),
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

    const filteredItems = useMemo(() => {
        const items = templatesQuery.data?.items ?? [];
        if (!deferredSearch) {
            return items;
        }
        const query = deferredSearch.toLowerCase();
        return items.filter((template: Template) =>
            template.name.toLowerCase().includes(query) ||
            (template.display_name ?? '').toLowerCase().includes(query) ||
            (template.description ?? '').toLowerCase().includes(query) ||
            (template.os_family ?? '').toLowerCase().includes(query)
        );
    }, [templatesQuery.data?.items, deferredSearch]);

    const openCreateModal = () => {
        createForm.resetFields();
        createForm.setFieldsValue({
            catalog_scope: 'unclassified',
            enabled: true,
            source_type: 'cdi_image_import',
        });
        setCreateExperimentalSourcesEnabled(false);
        setCreateOpen(true);
    };

    const openEditModal = (template: Template) => {
        setEditingTemplate(template);
        setEditExperimentalSourcesEnabled(template.source_type === 'containerdisk');
        editForm.setFieldsValue({
            display_name: template.display_name,
            description: template.description,
            catalog_scope: template.catalog_scope,
            os_family: template.os_family,
            os_version: template.os_version,
            enabled: template.enabled,
            source_type: template.source_type,
            image_url: template.image_url,
            pvc_name: template.pvc_name,
            // pvc_namespace: must be populated so the required validation passes
            // when editing an existing PVC-type template (master-flow Step 3).
            pvc_namespace: template.pvc_namespace,
            // cloud_init is the YAML cloud-init config (plain text, not JSON).
            // master-flow Step 3: admin can freely edit this YAML text.
            cloud_init: template.cloud_init,
        });
        setEditOpen(true);
    };

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
        globalSearch,
        setGlobalSearch,
        deferredSearch,
        isStale,
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
