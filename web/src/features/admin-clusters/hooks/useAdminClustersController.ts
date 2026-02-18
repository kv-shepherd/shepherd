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
    const [form] = Form.useForm<ClusterCreateRequest>();

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
    };
}
