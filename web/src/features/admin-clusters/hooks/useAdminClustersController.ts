'use client';

import { Form, message } from 'antd';
import type { TFunction } from 'i18next';
import { useState } from 'react';

import { useApiGet, useApiMutation } from '@/hooks/useApiQuery';
import { api } from '@/lib/api/client';

import type { Cluster, ClusterCreateRequest, ClusterList } from '../types';

interface UseAdminClustersControllerArgs {
    t: TFunction;
}

export function useAdminClustersController({ t }: UseAdminClustersControllerArgs) {
    const [messageApi, messageContextHolder] = message.useMessage();
    const [createOpen, setCreateOpen] = useState(false);
    const [envModalOpen, setEnvModalOpen] = useState(false);
    const [selectedClusterId, setSelectedClusterId] = useState<string>('');
    const [selectedClusterEnv, setSelectedClusterEnv] = useState<'test' | 'prod'>('test');
    const [form] = Form.useForm<ClusterCreateRequest>();
    const [envForm] = Form.useForm<{ environment: 'test' | 'prod' }>();

    const clusterListQuery = useApiGet<ClusterList>(
        ['admin-clusters'],
        () => api.GET('/admin/clusters')
    );

    const createMutation = useApiMutation<ClusterCreateRequest, Cluster>(
        (req) => api.POST('/admin/clusters', { body: req }),
        {
            invalidateKeys: [['admin-clusters']],
            onSuccess: () => {
                messageApi.success(t('common:message.success'));
                closeCreateModal();
            },
            onError: (err) => messageApi.error(err.message || t('common:message.error')),
        }
    );

    const updateEnvironmentMutation = useApiMutation<
        { clusterId: string; environment: 'test' | 'prod' },
        Cluster
    >(
        ({ clusterId, environment }) => api.PUT('/admin/clusters/{cluster_id}/environment', {
            params: { path: { cluster_id: clusterId } },
            body: { environment },
        }),
        {
            invalidateKeys: [['admin-clusters']],
            onSuccess: () => messageApi.success(t('common:message.success')),
            onError: (err) => messageApi.error(err.message || t('common:message.error')),
        }
    );

    const openCreateModal = () => {
        setCreateOpen(true);
    };

    const closeCreateModal = () => {
        setCreateOpen(false);
        form.resetFields();
    };

    const submitCreate = async () => {
        const values = await form.validateFields();
        createMutation.mutate(values);
    };

    const updateEnvironment = (clusterId: string, environment: 'test' | 'prod') => {
        updateEnvironmentMutation.mutate({ clusterId, environment });
    };

    const openEnvModal = (clusterId: string, currentEnv: 'test' | 'prod') => {
        setSelectedClusterId(clusterId);
        setSelectedClusterEnv(currentEnv);
        envForm.setFieldsValue({ environment: currentEnv });
        setEnvModalOpen(true);
    };

    const closeEnvModal = () => {
        setEnvModalOpen(false);
        setSelectedClusterId('');
        envForm.resetFields();
    };

    const submitEnvUpdate = async () => {
        const values = await envForm.validateFields();
        await updateEnvironmentMutation.mutateAsync({ clusterId: selectedClusterId, environment: values.environment });
        closeEnvModal();
    };

    return {
        messageContextHolder,
        createOpen,
        form,
        data: clusterListQuery.data,
        isLoading: clusterListQuery.isLoading,
        refetch: clusterListQuery.refetch,
        openCreateModal,
        closeCreateModal,
        submitCreate,
        updateEnvironment,
        createPending: createMutation.isPending,
        updateEnvironmentPending: updateEnvironmentMutation.isPending,
        envModalOpen,
        selectedClusterId,
        selectedClusterEnv,
        envForm,
        openEnvModal,
        closeEnvModal,
        submitEnvUpdate,
    };
}
