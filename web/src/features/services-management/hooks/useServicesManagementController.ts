'use client';

import { Form, message } from 'antd';
import type { TFunction } from 'i18next';
import { useState } from 'react';

import { useApiGet, useApiMutation } from '@/hooks/useApiQuery';
import { applyApiFieldErrors } from '@/hooks/applyApiFieldErrors';
import { api } from '@/lib/api/client';

import type {
    Service,
    ServiceCreateRequest,
    ServiceList,
    ServiceUpdateRequest,
    SystemList,
} from '../types';

interface UseServicesManagementControllerArgs {
    t: TFunction;
}

export function useServicesManagementController({ t }: UseServicesManagementControllerArgs) {
    const [messageApi, messageContextHolder] = message.useMessage();
    const [createOpen, setCreateOpen] = useState(false);
    const [editOpen, setEditOpen] = useState(false);
    const [editingService, setEditingService] = useState<Service | null>(null);
    const [selectedSystemId, setSelectedSystemId] = useState('');
    const [page, setPage] = useState(1);
    const [pageSize, setPageSize] = useState(20);
    const [form] = Form.useForm<ServiceCreateRequest & { system_id: string }>();
    const [editForm] = Form.useForm<ServiceUpdateRequest>();

    const systemsQuery = useApiGet<SystemList>(
        ['systems', 'all'],
        () => api.GET('/systems', { params: { query: { per_page: 100 } } })
    );

    const activeSystemId = selectedSystemId || systemsQuery.data?.items?.[0]?.id || '';

    const servicesQuery = useApiGet<ServiceList>(
        ['services', activeSystemId, page, pageSize],
        () => api.GET('/systems/{system_id}/services', {
            params: {
                path: { system_id: activeSystemId },
                query: { page, per_page: pageSize },
            },
        }),
        { enabled: Boolean(activeSystemId) }
    );

    const createMutation = useApiMutation<
        { system_id: string; body: ServiceCreateRequest },
        Service
    >(
        ({ system_id, body }) => api.POST('/systems/{system_id}/services', {
            params: { path: { system_id } },
            body,
        }),
        {
            invalidateKeys: [['services']],
            onSuccess: () => {
                messageApi.success(t('message.success'));
                closeCreateModal();
            },
            onError: (err) => {
                if (applyApiFieldErrors(form, err)) {
                    return;
                }
                messageApi.error(err.code === 'CONFLICT' ? t('services.error.name_exists') : t('message.error'));
            },
        }
    );

    const deleteMutation = useApiMutation<
        { systemId: string; serviceId: string },
        unknown
    >(
        ({ systemId, serviceId }) => api.DELETE('/systems/{system_id}/services/{service_id}', {
            params: {
                path: { system_id: systemId, service_id: serviceId },
                query: { confirm: true },
            },
        }),
        {
            invalidateKeys: [['services']],
            onSuccess: () => messageApi.success(t('message.success')),
            onError: (err) => messageApi.error(err.message || t('message.error')),
        }
    );

    const updateMutation = useApiMutation<
        { systemId: string; serviceId: string; body: ServiceUpdateRequest },
        Service
    >(
        ({ systemId, serviceId, body }) => api.PATCH('/systems/{system_id}/services/{service_id}', {
            params: { path: { system_id: systemId, service_id: serviceId } },
            body,
        }),
        {
            invalidateKeys: [['services']],
            onSuccess: () => {
                messageApi.success(t('message.success'));
                closeEditModal();
            },
            onError: (err) => {
                if (applyApiFieldErrors(editForm, err)) {
                    return;
                }
                messageApi.error(err.message || t('message.error'));
            },
        }
    );

    const changeSystem = (systemId: string) => {
        setSelectedSystemId(systemId);
        setPage(1);
    };

    const openCreateModal = () => {
        setCreateOpen(true);
        form.setFieldValue('system_id', activeSystemId || undefined);
    };

    const closeCreateModal = () => {
        setCreateOpen(false);
        form.resetFields();
    };

    const openEditModal = (service: Service) => {
        void api.GET('/systems/{system_id}/services/{service_id}', {
            params: {
                path: {
                    system_id: service.system_id,
                    service_id: service.id,
                },
            },
        }).then(({ data }) => {
            const resolved = data ?? service;
            setEditingService(resolved);
            editForm.setFieldsValue({ description: resolved.description || '' });
            setEditOpen(true);
        }).catch(() => {
            setEditingService(service);
            editForm.setFieldsValue({ description: service.description || '' });
            setEditOpen(true);
        });
    };

    const closeEditModal = () => {
        setEditOpen(false);
        setEditingService(null);
        editForm.resetFields();
    };

    const submitCreate = async () => {
        const values = await form.validateFields();
        const { system_id, ...body } = values;
        createMutation.mutate({ system_id, body });
    };

    const submitDelete = (systemId: string, serviceId: string) => {
        deleteMutation.mutate({ systemId, serviceId });
    };

    const submitEdit = async () => {
        if (!editingService) {
            return;
        }
        const values = await editForm.validateFields();
        updateMutation.mutate({
            systemId: editingService.system_id,
            serviceId: editingService.id,
            body: values,
        });
    };

    return {
        messageContextHolder,
        createOpen,
        editOpen,
        editingService,
        activeSystemId,
        page,
        pageSize,
        setPage,
        setPageSize,
        form,
        editForm,
        systemsData: systemsQuery.data,
        servicesData: servicesQuery.data,
        isLoading: servicesQuery.isLoading,
        refetch: servicesQuery.refetch,
        changeSystem,
        openCreateModal,
        closeCreateModal,
        openEditModal,
        closeEditModal,
        submitCreate,
        submitEdit,
        submitDelete,
        createPending: createMutation.isPending,
        updatePending: updateMutation.isPending,
        deletePending: deleteMutation.isPending,
    };
}
