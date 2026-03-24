'use client';

import { Form, message } from 'antd';
import type { TFunction } from 'i18next';
import { useCallback, useMemo, useState } from 'react';

import { useApiGet, useApiMutation } from '@/hooks/useApiQuery';
import { applyApiFieldErrors } from '@/hooks/applyApiFieldErrors';
import { SETUP_GUIDE_INVALIDATION_KEYS } from '@/features/setup-guide/queryKeys';
import { api } from '@/lib/api/client';
import { translateApiError } from '@/lib/api/errorMessage';

import type {
    Service,
    ServiceCreateRequest,
    ServiceList,
    ServiceUpdateRequest,
    SystemList,
} from '../types';

interface UseServicesManagementControllerArgs {
    t: TFunction;
    onCreateSuccess?: (service: Service, context: { isFirstService: boolean }) => boolean | void;
}

const ALL_SYSTEMS_FILTER = '__all__';

export function useServicesManagementController({
    t,
    onCreateSuccess,
}: UseServicesManagementControllerArgs) {
    const [messageApi, messageContextHolder] = message.useMessage();
    const [createOpen, setCreateOpen] = useState(false);
    const [editOpen, setEditOpen] = useState(false);
    const [editingService, setEditingService] = useState<Service | null>(null);
    const [selectedSystemId, setSelectedSystemId] = useState(ALL_SYSTEMS_FILTER);
    const [page, setPage] = useState(1);
    const [pageSize, setPageSize] = useState(20);
    const [form] = Form.useForm<ServiceCreateRequest & { system_id: string }>();
    const [editForm] = Form.useForm<ServiceUpdateRequest>();

    const systemsQuery = useApiGet<SystemList>(
        ['systems', 'all'],
        () => api.GET('/systems', { params: { query: { per_page: 100 } } })
    );

    const allSystems = useMemo(
        () => systemsQuery.data?.items ?? [],
        [systemsQuery.data?.items],
    );
    const activeSystemId = selectedSystemId;

    const servicesQuery = useApiGet<ServiceList>(
        ['services', activeSystemId, page, pageSize],
        () => api.GET('/services', {
            params: {
                query: {
                    page,
                    per_page: pageSize,
                    ...(activeSystemId !== ALL_SYSTEMS_FILTER ? { system_id: activeSystemId } : {}),
                },
            },
        }),
    );
    const existingServicesTotal =
        servicesQuery.data?.pagination?.total ??
        servicesQuery.data?.items?.length ??
        0;
    const shouldContinueOnboarding = existingServicesTotal === 0;

    const createMutation = useApiMutation<
        { system_id: string; body: ServiceCreateRequest },
        Service
    >(
        ({ system_id, body }) => api.POST('/systems/{system_id}/services', {
            params: { path: { system_id } },
            body,
        }),
        {
            invalidateKeys: [['services'], ...SETUP_GUIDE_INVALIDATION_KEYS],
            onSuccess: (service) => {
                closeCreateModal();
                const handled = onCreateSuccess?.(service, {
                    isFirstService: shouldContinueOnboarding,
                }) ?? false;
                if (handled) {
                    return;
                }
                messageApi.success(t('message.success'));
            },
            onError: (err) => {
                if (applyApiFieldErrors(form, err)) {
                    return;
                }
                messageApi.error(
                    err.code === 'CONFLICT' ? t('services.error.name_exists') : translateApiError(t, err, 'message.error'),
                );
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
            invalidateKeys: [['services'], ...SETUP_GUIDE_INVALIDATION_KEYS],
            onSuccess: () => messageApi.success(t('message.success')),
            onError: (err) => messageApi.error(translateApiError(t, err, 'message.error')),
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
            invalidateKeys: [['services'], ...SETUP_GUIDE_INVALIDATION_KEYS],
            onSuccess: () => {
                messageApi.success(t('message.success'));
                closeEditModal();
            },
            onError: (err) => {
                if (applyApiFieldErrors(editForm, err)) {
                    return;
                }
                messageApi.error(translateApiError(t, err, 'message.error'));
            },
        }
    );

    const changeSystem = (systemId: string) => {
        setSelectedSystemId(systemId);
        setPage(1);
    };

    const openCreateModal = useCallback((systemId?: string) => {
        setCreateOpen(true);
        const targetSystemId = systemId ?? (
            activeSystemId !== ALL_SYSTEMS_FILTER
                ? activeSystemId
                : allSystems[0]?.id
        );
        if (targetSystemId) {
            setSelectedSystemId(targetSystemId);
            setPage(1);
        }
        form.setFieldValue('system_id', targetSystemId || undefined);
    }, [activeSystemId, allSystems, form]);

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
