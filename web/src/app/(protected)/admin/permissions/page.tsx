'use client';

/**
 * /admin/permissions — Permissions management page (master-flow.md §9).
 * RBAC permissions catalog. Admin only.
 * data-testid="admin-permissions-page" required by E2E contract.
 */
import { Table, Tag, Typography } from 'antd';
import { KeyOutlined } from '@ant-design/icons';
import { useTranslation } from 'react-i18next';

import { ActionEmptyState } from '@/components/feedback/ActionEmptyState';
import { SummaryMetricCard } from '@/components/feedback/SummaryMetricCard';
import {
    AccessControlGlyph,
    RoleCatalogGlyph,
    SystemsOverviewGlyph,
    VirtualMachinesOverviewGlyph,
} from '@/components/illustrations/DashboardIllustrations';
import { PageHeader, PageSurface } from '@/components/layouts/PageSection';

const { Text } = Typography;

const STATIC_PERMISSIONS = [
    { key: 'vm:create', scope: 'vm', labelKey: 'rbac.permissions.catalog.vm_create.label', descriptionKey: 'rbac.permissions.catalog.vm_create.description' },
    { key: 'vm:operate', scope: 'vm', labelKey: 'rbac.permissions.catalog.vm_operate.label', descriptionKey: 'rbac.permissions.catalog.vm_operate.description' },
    { key: 'vm:delete', scope: 'vm', labelKey: 'rbac.permissions.catalog.vm_delete.label', descriptionKey: 'rbac.permissions.catalog.vm_delete.description' },
    { key: 'system:read', scope: 'system', labelKey: 'rbac.permissions.catalog.system_read.label', descriptionKey: 'rbac.permissions.catalog.system_read.description' },
    { key: 'system:write', scope: 'system', labelKey: 'rbac.permissions.catalog.system_write.label', descriptionKey: 'rbac.permissions.catalog.system_write.description' },
    { key: 'system:delete', scope: 'system', labelKey: 'rbac.permissions.catalog.system_delete.label', descriptionKey: 'rbac.permissions.catalog.system_delete.description' },
    { key: 'service:create', scope: 'service', labelKey: 'rbac.permissions.catalog.service_create.label', descriptionKey: 'rbac.permissions.catalog.service_create.description' },
    { key: 'service:delete', scope: 'service', labelKey: 'rbac.permissions.catalog.service_delete.label', descriptionKey: 'rbac.permissions.catalog.service_delete.description' },
    { key: 'rbac:manage', scope: 'rbac', labelKey: 'rbac.permissions.catalog.rbac_manage.label', descriptionKey: 'rbac.permissions.catalog.rbac_manage.description' },
    { key: 'admin:all', scope: 'admin', labelKey: 'rbac.permissions.catalog.admin_all.label', descriptionKey: 'rbac.permissions.catalog.admin_all.description' },
];

export default function AdminPermissionsPage() {
    const { t } = useTranslation(['admin', 'common']);
    const totalCount = STATIC_PERMISSIONS.length;
    const vmCount = STATIC_PERMISSIONS.filter((permission) => permission.scope === 'vm').length;
    const resourceCount = STATIC_PERMISSIONS.filter((permission) => permission.scope === 'system' || permission.scope === 'service').length;
    const governanceCount = STATIC_PERMISSIONS.filter((permission) => permission.scope === 'rbac' || permission.scope === 'admin').length;

    const columns = [
        {
            title: t('rbac.permissions.table.permission', { defaultValue: 'Permission' }),
            dataIndex: 'labelKey',
            key: 'key',
            render: (_: string, record: typeof STATIC_PERMISSIONS[number]) => (
                <div>
                    <Text strong>{t(record.labelKey)}</Text>
                    <div>
                        <Text type="secondary" code>{record.key}</Text>
                    </div>
                </div>
            ),
        },
        {
            title: t('rbac.bindings.scope', { defaultValue: 'Scope' }),
            dataIndex: 'scope',
            key: 'scope',
            width: 140,
            render: (scope: string) => (
                <Tag color="blue">
                    {t(`rbac.scope.${scope}`, { defaultValue: scope })}
                </Tag>
            ),
        },
        {
            title: t('rbac.permissions.table.use_case', { defaultValue: 'Typical use' }),
            dataIndex: 'descriptionKey',
            key: 'description',
            render: (_: string, record: typeof STATIC_PERMISSIONS[number]) => (
                <Text type="secondary">{t(record.descriptionKey)}</Text>
            ),
        },
    ];

    return (
        <div data-testid="admin-permissions-page">
            <PageHeader
                title={(
                    <>
                        <KeyOutlined style={{ marginRight: 8, color: '#722ed1' }} />
                        {t('rbac.permissions.title', { defaultValue: 'Permission Catalog' })}
                    </>
                )}
                subtitle={t('rbac.permissions.subtitle', { defaultValue: 'Available permission keys that roles can include' })}
            />

            <div className="summary-card-grid">
                <SummaryMetricCard
                    title={t('rbac.permissions.summary.total_title', { defaultValue: 'Permission keys' })}
                    value={totalCount}
                    description={t('rbac.permissions.summary.total_description', { defaultValue: 'All permission definitions currently available for role composition.' })}
                    visual={<RoleCatalogGlyph className="summary-metric-card__art" />}
                    accentColor="#6D4DE3"
                    surfaceColor="#F5EDFF"
                />
                <SummaryMetricCard
                    title={t('rbac.permissions.summary.vm_title', { defaultValue: 'VM operations' })}
                    value={vmCount}
                    description={t('rbac.permissions.summary.vm_description', { defaultValue: 'Permissions that govern VM requests and day-2 actions.' })}
                    visual={<VirtualMachinesOverviewGlyph className="summary-metric-card__art" />}
                    accentColor="#1D5BFF"
                    surfaceColor="#E6F4FF"
                />
                <SummaryMetricCard
                    title={t('rbac.permissions.summary.resource_title', { defaultValue: 'System & service' })}
                    value={resourceCount}
                    description={t('rbac.permissions.summary.resource_description', { defaultValue: 'Permissions that shape application resource ownership and lifecycle.' })}
                    visual={<SystemsOverviewGlyph className="summary-metric-card__art" />}
                    accentColor="#0F8F57"
                    surfaceColor="#E8FFF2"
                />
                <SummaryMetricCard
                    title={t('rbac.permissions.summary.governance_title', { defaultValue: 'Governance' })}
                    value={governanceCount}
                    description={t('rbac.permissions.summary.governance_description', { defaultValue: 'Permissions for RBAC administration and full platform control.' })}
                    visual={<AccessControlGlyph className="summary-metric-card__art" />}
                    accentColor="#D66A1F"
                    surfaceColor="#FFF4E5"
                />
            </div>

            <PageSurface flush={true}>
                <Table
                    dataSource={STATIC_PERMISSIONS}
                    columns={columns}
                    rowKey="key"
                    pagination={false}
                    size="middle"
                    locale={{
                        emptyText: (
                            <ActionEmptyState
                                compact={true}
                                title={t('rbac.permissions.empty', { defaultValue: 'No permissions available' })}
                                description={t('rbac.permissions.empty_description', { defaultValue: 'Permission definitions will appear here after the backend exposes them.' })}
                                visual={<AccessControlGlyph className="action-empty-state__art action-empty-state__art--compact" />}
                            />
                        ),
                    }}
                />
            </PageSurface>
        </div>
    );
}
