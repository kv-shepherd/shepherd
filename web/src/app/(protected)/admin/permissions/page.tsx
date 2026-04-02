'use client';

/**
 * /admin/permissions — Permissions management page (master-flow.md §9).
 * RBAC permissions catalog. Admin only.
 * data-testid="admin-permissions-page" required by E2E contract.
 */
import { Button as ActionButton, Table, Tag, Typography, Select, Space } from 'antd';
import { KeyOutlined } from '@ant-design/icons';
import { useMemo, useState } from 'react';
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
import { PageSearchToolbar } from '@/components/ui/PageSearchToolbar';

const { Text } = Typography;

const filterOptionByLabel = (input: string, option?: { label?: unknown }) => {
    const label = typeof option?.label === 'string' ? option.label : '';
    return label.toLowerCase().includes(input.trim().toLowerCase());
};

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
    const [quickSearchDraft, setQuickSearchDraft] = useState('');
    const [search, setSearch] = useState('');
    const [scopeDraft, setScopeDraft] = useState('');
    const [scopeFilter, setScopeFilter] = useState('');
    const [advancedSearchOpen, setAdvancedSearchOpen] = useState(false);
    const totalCount = STATIC_PERMISSIONS.length;
    const vmCount = STATIC_PERMISSIONS.filter((permission) => permission.scope === 'vm').length;
    const resourceCount = STATIC_PERMISSIONS.filter((permission) => permission.scope === 'system' || permission.scope === 'service').length;
    const governanceCount = STATIC_PERMISSIONS.filter((permission) => permission.scope === 'rbac' || permission.scope === 'admin').length;
    const scopeOptions = useMemo(
        () =>
            Array.from(new Set(STATIC_PERMISSIONS.map((permission) => permission.scope)))
                .sort((left, right) => left.localeCompare(right))
                .map((scope) => ({
                    value: scope,
                    label: t(`rbac.scope.${scope}`, { defaultValue: scope }),
                })),
        [t],
    );
    const filteredPermissions = useMemo(() => {
        const normalizedSearch = search.trim().toLowerCase();
        return STATIC_PERMISSIONS.filter((permission) => {
            if (scopeFilter !== '' && permission.scope !== scopeFilter) {
                return false;
            }
            if (normalizedSearch === '') {
                return true;
            }
            const label = t(permission.labelKey).toLowerCase();
            const description = t(permission.descriptionKey).toLowerCase();
            return [
                permission.key,
                permission.scope,
                label,
                description,
            ].some((value) => value.toLowerCase().includes(normalizedSearch));
        });
    }, [scopeFilter, search, t]);

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
                <div style={{ padding: 16, paddingBottom: 0 }}>
                    <PageSearchToolbar
                        searchValue={search}
                        searchDraftValue={quickSearchDraft}
                        onSearchDraftChange={setQuickSearchDraft}
                        onSearchChange={(value) => {
                            const nextValue = value.trim();
                            setQuickSearchDraft(nextValue);
                            setSearch(nextValue);
                        }}
                        searchPlaceholder={t('rbac.permissions.search_placeholder', { defaultValue: 'Search permissions, keys, or use cases' })}
                        searchTestId="permissions-quick-search"
                        searchHelp={t('rbac.permissions.search_help', { defaultValue: 'Press Enter or click Search. Quick search matches permission labels, keys, scopes, and typical use descriptions.' })}
                        advancedSearch={{
                            open: advancedSearchOpen,
                            onToggle: () => setAdvancedSearchOpen((open) => !open),
                            openLabel: t('common:search.advanced', { defaultValue: 'Advanced search' }),
                            closeLabel: t('common:search.hide_advanced', { defaultValue: 'Hide advanced search' }),
                            title: t('common:search.advanced', { defaultValue: 'Advanced search' }),
                            toggleTestId: 'permissions-advanced-search-toggle',
                            content: (
                                <Space direction="vertical" size={12} style={{ width: '100%' }}>
                                    <Text type="secondary">
                                        {t('rbac.permissions.advanced_search_help', { defaultValue: 'Select exact filters here. Options support keyword matching, but the applied filter remains an exact match.' })}
                                    </Text>
                                    <Space wrap size={[12, 12]}>
                                        <Select
                                            allowClear
                                            showSearch
                                            filterOption={filterOptionByLabel}
                                            optionFilterProp="label"
                                            style={{ minWidth: 220 }}
                                            data-testid="permissions-filter-scope"
                                            placeholder={t('rbac.bindings.scope', { defaultValue: 'Scope' })}
                                            options={scopeOptions}
                                            value={scopeDraft || undefined}
                                            onChange={(value) => setScopeDraft(value ?? '')}
                                        />
                                        <ActionButton
                                            type="primary"
                                            data-testid="permissions-advanced-search-submit"
                                            onClick={() => setScopeFilter(scopeDraft)}
                                        >
                                            {t('common:button.search')}
                                        </ActionButton>
                                    </Space>
                                </Space>
                            ),
                        }}
                        hasActiveFilters={search !== '' || scopeFilter !== ''}
                        onClear={() => {
                            setQuickSearchDraft('');
                            setSearch('');
                            setScopeDraft('');
                            setScopeFilter('');
                            setAdvancedSearchOpen(false);
                        }}
                        clearLabel={t('common:button.clear_filters', { defaultValue: 'Clear filters' })}
                    />
                </div>
                <Table
                    dataSource={filteredPermissions}
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
