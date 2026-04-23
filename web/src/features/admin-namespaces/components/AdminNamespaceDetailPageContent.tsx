'use client';

import { ArrowLeftOutlined } from '@ant-design/icons';
import { Alert, Button, Descriptions, Result, Space, Spin, Tag, Typography } from 'antd';
import type { DescriptionsProps } from 'antd';
import { useParams, useRouter } from 'next/navigation';
import { useTranslation } from 'react-i18next';

import { PageHeader, PageSurface } from '@/components/layouts/PageSection';
import { LocalDateTimeText } from '@/components/ui/LocalDateTimeText';
import { useApiGet } from '@/hooks/useApiQuery';
import { api } from '@/lib/api/client';
import { translateApiError } from '@/lib/api/errorMessage';
import { ENV_MAP, type NamespaceRegistry } from '../types';

const { Text } = Typography;

export function AdminNamespaceDetailPageContent() {
    const { t } = useTranslation(['admin', 'common']);
    const params = useParams<{ id: string }>();
    const router = useRouter();
    const namespaceID = params.id;
    const namespaceQuery = useApiGet<NamespaceRegistry>(
        ['admin-namespace-detail', namespaceID],
        () => api.GET('/admin/namespaces/{namespace_id}', {
            params: { path: { namespace_id: namespaceID } },
        }),
        { enabled: Boolean(namespaceID) },
    );
    const namespace = namespaceQuery.data;
    const environmentLabel = namespace ? (ENV_MAP[namespace.environment]?.label ?? namespace.environment) : '—';
    const descriptionItems: DescriptionsProps['items'] = namespace ? [
        {
            key: 'name',
            label: t('namespaces.table.namespace'),
            children: <Text strong>{namespace.name}</Text>,
        },
        {
            key: 'environment',
            label: t('namespaces.environment'),
            children: (
                <Tag color={ENV_MAP[namespace.environment]?.color ?? 'default'}>
                    {environmentLabel}
                </Tag>
            ),
        },
        {
            key: 'enabled',
            label: t('namespaces.enabled'),
            children: (
                <Tag color={namespace.enabled ? 'green' : 'default'}>
                    {namespace.enabled ? t('namespaces.enabled_yes') : t('namespaces.enabled_no')}
                </Tag>
            ),
        },
        {
            key: 'description',
            label: t('namespaces.detail_description'),
            children: namespace.description || '—',
        },
        {
            key: 'created_by',
            label: t('common:table.created_by'),
            children: namespace.created_by || '—',
        },
        {
            key: 'created_at',
            label: t('common:table.created_at'),
            children: <LocalDateTimeText value={namespace.created_at} />,
        },
        {
            key: 'id',
            label: t('namespaces.detail_registry_id'),
            children: <Text code>{namespace.id}</Text>,
        },
    ] : [];

    return (
        <div data-testid="admin-namespace-detail-page">
            <PageHeader
                title={namespace ? t('namespaces.detail_title', { name: namespace.name }) : t('namespaces.title')}
                subtitle={t('namespaces.detail_subtitle')}
                actions={(
                    <Button
                        icon={<ArrowLeftOutlined />}
                        onClick={() => router.push('/admin/namespaces')}
                    >
                        {t('common:button.back')}
                    </Button>
                )}
            />

            {namespaceQuery.isLoading ? (
                <PageSurface flush={true}>
                    <div style={{ display: 'flex', justifyContent: 'center', padding: '56px 24px' }}>
                        <Spin size="large" />
                    </div>
                </PageSurface>
            ) : null}

            {!namespaceQuery.isLoading && namespaceQuery.error?.status === 404 ? (
                <PageSurface flush={true}>
                    <Result
                        status="404"
                        title={t('namespaces.detail_not_found')}
                        subTitle={t('namespaces.detail_not_found_description')}
                        extra={(
                            <Button type="primary" onClick={() => router.push('/admin/namespaces')}>
                                {t('common:button.back')}
                            </Button>
                        )}
                    />
                </PageSurface>
            ) : null}

            {!namespaceQuery.isLoading && namespaceQuery.error && namespaceQuery.error.status !== 404 ? (
                <PageSurface flush={true}>
                    <Alert
                        data-testid="admin-namespace-detail-error"
                        type="error"
                        showIcon={true}
                        message={translateApiError(t, namespaceQuery.error)}
                    />
                </PageSurface>
            ) : null}

            {!namespaceQuery.isLoading && namespace ? (
                <Space direction="vertical" size={24} style={{ display: 'flex' }}>
                    <PageSurface flush={true}>
                        <Descriptions
                            bordered={true}
                            column={1}
                            size="middle"
                            items={descriptionItems}
                        />
                    </PageSurface>
                </Space>
            ) : null}
        </div>
    );
}
