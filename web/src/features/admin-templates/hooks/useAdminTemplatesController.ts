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

interface TemplateCreateFormValues extends TemplateCreateRequest {
    spec_text?: string;
}

interface TemplateEditFormValues extends TemplateUpdateRequest {
    spec_text?: string;
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

export function useAdminTemplatesController({ t }: UseAdminTemplatesControllerArgs) {
    const [messageApi, messageContextHolder] = message.useMessage();
    const [createOpen, setCreateOpen] = useState(false);
    const [editOpen, setEditOpen] = useState(false);
    const [deleteOpen, setDeleteOpen] = useState(false);
    const [editingTemplate, setEditingTemplate] = useState<Template | null>(null);
    const [deletingTemplate, setDeletingTemplate] = useState<Template | null>(null);

    const [page, setPage] = useState(1);
    const [globalSearch, setGlobalSearch] = useState('');
    const deferredSearch = useDeferredValue(globalSearch);
    const isStale = globalSearch !== deferredSearch;

    const [searchedColumn, setSearchedColumn] = useState('');
    const [searchText, setSearchText] = useState('');

    const [createForm] = Form.useForm<TemplateCreateFormValues>();
    const [editForm] = Form.useForm<TemplateEditFormValues>();

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
            enabled: true,
            spec_text: '{}',
        });
        setCreateOpen(true);
    };

    const openEditModal = (template: Template) => {
        const hydrated = template as Template & { spec?: Record<string, unknown> };
        setEditingTemplate(template);
        editForm.setFieldsValue({
            display_name: template.display_name,
            description: template.description,
            os_family: template.os_family,
            os_version: template.os_version,
            enabled: template.enabled,
            spec_text: JSON.stringify(hydrated.spec ?? {}, null, 2),
        });
        setEditOpen(true);
    };

    const openDeleteModal = (template: Template) => {
        setDeletingTemplate(template);
        setDeleteOpen(true);
    };

    const submitCreate = async () => {
        const values = await createForm.validateFields();
        const spec = parseJSONMap(values.spec_text ?? '', () => {
            messageApi.error(t('templates.spec_invalid'));
        });
        if (values.spec_text && !spec) {
            return;
        }
        const { spec_text: _specText, ...payload } = values;
        createMutation.mutate({
            ...payload,
            spec,
        });
    };

    const submitEdit = async () => {
        if (!editingTemplate) {
            return;
        }
        const values = await editForm.validateFields();
        const spec = parseJSONMap(values.spec_text ?? '', () => {
            messageApi.error(t('templates.spec_invalid'));
        });
        if (values.spec_text && !spec) {
            return;
        }
        const { spec_text: _specText, ...payload } = values;
        updateMutation.mutate({
            id: editingTemplate.id,
            body: {
                ...payload,
                spec,
            },
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
            createForm.resetFields();
        },
        closeEditModal: () => {
            setEditOpen(false);
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
        createPending: createMutation.isPending,
        updatePending: updateMutation.isPending,
        deletePending: deleteMutation.isPending,
    };
}
