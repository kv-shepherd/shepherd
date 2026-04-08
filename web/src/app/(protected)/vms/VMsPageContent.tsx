'use client';

import {
    ExclamationCircleOutlined,
    PlusOutlined,
    ReloadOutlined,
    SettingOutlined,
} from '@ant-design/icons';
import {
    Alert,
    Button,
    Descriptions,
    Form,
    Input,
    InputNumber,
    Modal,
    Popconfirm,
    Select,
    Space,
    Tag,
    Typography,
} from 'antd';
import { useRouter, useSearchParams } from 'next/navigation';
import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';

import { PermissionGuard } from '@/components/auth/PermissionGuard';
import { PageHeader, PageSurface } from '@/components/layouts/PageSection';
import { PageSearchToolbar } from '@/components/ui/PageSearchToolbar';
import type { ServiceWorkspaceContext } from '@/features/services-management/types';
import { SetupGuideCard } from '@/features/setup-guide/components/SetupGuideCard';
import { useSetupGuide } from '@/features/setup-guide/hooks/useSetupGuide';
import { VMSavedDraftBanner } from '@/features/vm-management/components/VMSavedDraftBanner';
import { VMListTable } from '@/features/vm-management/components/VMListTable';
import { VMRequestWizard } from '@/features/vm-management/components/VMRequestWizard';
import { useVMManagementController } from '@/features/vm-management/hooks/useVMManagementController';
import { useScopedVMRequestLauncher } from '@/features/vm-management/hooks/useScopedVMRequestLauncher';
import { useApiGet } from '@/hooks/useApiQuery';
import { api } from '@/lib/api/client';

const { Paragraph, Text } = Typography;

const filterOptionByLabel = (input: string, option?: { label?: unknown }) => {
    const label = typeof option?.label === 'string' ? option.label : '';
    return label.toLowerCase().includes(input.trim().toLowerCase());
};

export default function VMsPageContent() {
    const { t } = useTranslation(['vm', 'common']);
    const searchParams = useSearchParams();
    const vm = useVMManagementController({ t });
    const lastBatchActionFeedback = vm.lastBatchActionFeedback
        ? `${vm.lastBatchActionFeedback.action} submitted for ${vm.lastBatchActionFeedback.affectedCount} item(s)`
        : '';
    const setupGuide = useSetupGuide({
        vmsTotal: vm.vmData?.pagination?.total,
    });
    const router = useRouter();
    const scopedSystemId = searchParams.get('system_id') || undefined;
    const scopedServiceId = searchParams.get('service_id') || undefined;
    const hasScopedWorkspace = Boolean(scopedSystemId && scopedServiceId);
    const scopedWorkspaceQuery = useApiGet<ServiceWorkspaceContext>(
        ['vm-workspace-context', scopedSystemId, scopedServiceId],
        () => api.GET('/systems/{system_id}/services/{service_id}/context', {
            params: {
                path: {
                    system_id: scopedSystemId!,
                    service_id: scopedServiceId!,
                },
            },
        }),
        { enabled: hasScopedWorkspace },
    );
    const scopedWorkspace = scopedWorkspaceQuery.data?.service;
    const scopedSystemLabel = scopedWorkspace?.system_name || '—';
    const scopedServiceLabel = scopedWorkspace?.name || '—';
    const [quickSearchDraft, setQuickSearchDraft] = useState(() => vm.filters.search);
    const [statusDraft, setStatusDraft] = useState(() => vm.filters.status);
    const [namespaceDraft, setNamespaceDraft] = useState(() => vm.filters.namespace);
    const [clusterDraft, setClusterDraft] = useState(() => vm.filters.clusterId);
    const [systemDraft, setSystemDraft] = useState(() => vm.filters.systemId);
    const [serviceDraft, setServiceDraft] = useState(() => vm.filters.serviceId);
    const [osDraft, setOsDraft] = useState(() => vm.filters.osName);
    const [ipAddressDraft, setIpAddressDraft] = useState(() => vm.filters.ipAddress);
    const [advancedSearchOpen, setAdvancedSearchOpen] = useState(
        () =>
            vm.filters.status !== ''
            || vm.filters.namespace !== ''
            || vm.filters.clusterId !== ''
            || vm.filters.systemId !== ''
            || vm.filters.serviceId !== ''
            || vm.filters.osName !== ''
            || vm.filters.ipAddress !== '',
    );

    const formatGroupedLabel = (label: string, group?: string) => {
        const trimmedGroup = (group ?? '').trim();
        if (trimmedGroup === '') {
            return label;
        }
        if (trimmedGroup === 'prod') {
            return `${t('field.environment')}: ${t('common:environment.prod', { defaultValue: 'Production' })} · ${label}`;
        }
        if (trimmedGroup === 'test') {
            return `${t('field.environment')}: ${t('common:environment.test', { defaultValue: 'Test' })} · ${label}`;
        }
        return `${trimmedGroup} · ${label}`;
    };

    const statusOptions = useMemo(
        () =>
            (vm.vmFilterOptions?.statuses ?? []).map((option) => ({
                value: option.value,
                label: t(`status.${option.value}`, { defaultValue: option.label }),
            })),
        [t, vm.vmFilterOptions?.statuses],
    );

    const namespaceOptions = (vm.vmFilterOptions?.namespaces ?? []).map((option) => ({
        value: option.value,
        label: formatGroupedLabel(option.label, option.group),
    }));

    const clusterOptions = (vm.vmFilterOptions?.clusters ?? []).map((option) => ({
        value: option.value,
        label: formatGroupedLabel(option.label, option.group),
    }));

    const systemOptions = useMemo(
        () =>
            (vm.vmFilterOptions?.systems ?? []).map((option) => ({
                value: option.value,
                label: option.label,
            })),
        [vm.vmFilterOptions?.systems],
    );

    const serviceOptions = useMemo(
        () =>
            (vm.vmFilterOptions?.services ?? []).map((option) => ({
                value: option.value,
                label: option.label,
            })),
        [vm.vmFilterOptions?.services],
    );

    const operatingSystemOptions = useMemo(
        () =>
            (vm.vmFilterOptions?.operating_systems ?? []).map((option) => ({
                value: option.value,
                label: option.label,
            })),
        [vm.vmFilterOptions?.operating_systems],
    );

    const ipAddressOptions = (vm.vmFilterOptions?.ip_addresses ?? []).map((option) => ({
        value: option.value,
        label: formatGroupedLabel(option.label, option.group),
    }));

    const applyQuickSearch = (value = quickSearchDraft) => {
        const nextValue = value.trim();
        setQuickSearchDraft(nextValue);
        vm.changeFilters({ search: nextValue });
    };

    const applyAdvancedFilters = () => {
        vm.changeFilters({
            status: statusDraft,
            namespace: namespaceDraft,
            clusterId: clusterDraft,
            systemId: systemDraft,
            serviceId: serviceDraft,
            osName: osDraft,
            ipAddress: ipAddressDraft,
        });
    };

    const resetFilters = () => {
        setQuickSearchDraft('');
        setStatusDraft('');
        setNamespaceDraft('');
        setClusterDraft('');
        setSystemDraft('');
        setServiceDraft('');
        setOsDraft('');
        setIpAddressDraft('');
        vm.resetFilters();
    };

    const hasActiveFilters = Object.values(vm.filters).some((value) => value !== '');

    const openCreateRequest = () => {
        if (hasScopedWorkspace) {
            vm.openWizard({ systemId: scopedSystemId, serviceId: scopedServiceId });
            return;
        }
        vm.openWizard();
    };

    useScopedVMRequestLauncher({
        canLaunchRequest: setupGuide.vmRequestReady,
        openWizard: vm.openWizard,
        openSimilarRequest: vm.openSimilarRequest,
        resumeDraft: vm.resumeDraft,
    });

    return (
        <div className="vm-workspace-page">
            {vm.messageContextHolder}
            <PageHeader
                title={t('title')}
                subtitle={t('subtitle')}
                actions={(
                    <Space className="copy-friendly-actions">
                        <Button icon={<ReloadOutlined />} onClick={() => vm.refetch()}>
                            {t('common:button.refresh')}
                        </Button>
                        <PermissionGuard permission="vm:create">
                            <Button
                                type="primary"
                                icon={<PlusOutlined />}
                                onClick={openCreateRequest}
                                disabled={!setupGuide.vmRequestReady}
                            >
                                {t('create_request')}
                            </Button>
                        </PermissionGuard>
                    </Space>
                )}
            />
            <div className="vm-page-stack">

            {vm.savedDraft && !vm.wizardOpen && (
                <div>
                    <VMSavedDraftBanner
                        t={t}
                        draft={vm.savedDraft}
                        onResume={vm.resumeDraft}
                        onDiscard={vm.discardDraft}
                    />
                </div>
            )}

            {!setupGuide.vmRequestReady ? <SetupGuideCard variant="vm" /> : null}

            {hasScopedWorkspace && (
                <PageSurface className="vm-page-surface vm-page-surface--context">
                    <Space direction="vertical" size={8} className="vm-page-block">
                        <Space className="vm-page-header-row" wrap>
                            <Space direction="vertical" size={2}>
                                <Text strong>{t('context.title')}</Text>
                                <Text type="secondary" className="vm-page-supporting-text">
                                    {t('context.description', {
                                        system: scopedSystemLabel,
                                        service: scopedServiceLabel,
                                    })}
                                </Text>
                            </Space>
                            <Space wrap className="copy-friendly-actions vm-page-actions">
                                <PermissionGuard permission="vm:create">
                                    <Button
                                        type="primary"
                                        onClick={openCreateRequest}
                                        disabled={!setupGuide.vmRequestReady}
                                    >
                                        {t('create_request')}
                                    </Button>
                                </PermissionGuard>
                                <Button
                                    onClick={() =>
                                        router.push(
                                            `/services?system_id=${scopedSystemId}&detail_service_id=${scopedServiceId}`,
                                        )
                                    }
                                >
                                    {t('context.open_service')}
                                </Button>
                                <Button onClick={() => router.push('/vms')}>
                                    {t('context.clear')}
                                </Button>
                            </Space>
                        </Space>
                        <Descriptions
                            className="vm-page-descriptions"
                            size="small"
                            column={3}
                            items={[
                                {
                                    key: 'system',
                                    label: t('context.system'),
                                    children: scopedSystemLabel,
                                },
                                {
                                    key: 'service',
                                    label: t('context.service'),
                                    children: scopedServiceLabel,
                                },
                                {
                                    key: 'summary',
                                    label: t('context.summary'),
                                    children: t('context.summary_value', {
                                        vmCount: scopedWorkspaceQuery.data?.summary.visible_vm_count ?? 0,
                                        requestCount: scopedWorkspaceQuery.data?.summary.recent_request_count ?? 0,
                                    }),
                                },
                            ]}
                        />
                    </Space>
                </PageSurface>
            )}

            <PageSurface className="vm-page-surface vm-page-surface--tight vm-page-surface--batch">
                <Space className="vm-page-header-row" wrap>
                    <Space direction="vertical" size={2}>
                        <Text strong>{t('batch.title')}</Text>
                        <Text type="secondary" className="vm-page-supporting-text">{t('batch.subtitle')}</Text>
                    </Space>
                    <Space wrap className="copy-friendly-actions vm-page-actions">
                        <Tag color="blue">{t('batch.selected', { count: vm.selectedVMIDs.length })}</Tag>
                        <PermissionGuard permission="vm:operate">
                            <Popconfirm
                                title={t('batch.start_confirm', { count: vm.selectedVMIDs.length })}
                                okText={t('common:button.confirm')}
                                cancelText={t('common:button.cancel')}
                                disabled={vm.selectedVMIDs.length === 0 || vm.batchRateLimited}
                                onConfirm={() => vm.submitBatchPowerSelected('START')}
                            >
                                <Button
                                    onClick={(event) => event.preventDefault()}
                                    loading={vm.batchSubmitPending}
                                    disabled={vm.batchRateLimited || vm.selectedVMIDs.length === 0}
                                >
                                    {t('batch.start_selected')}
                                </Button>
                            </Popconfirm>
                        </PermissionGuard>
                        <PermissionGuard permission="vm:operate">
                            <Popconfirm
                                title={t('batch.stop_confirm', { count: vm.selectedVMIDs.length })}
                                okText={t('common:button.confirm')}
                                cancelText={t('common:button.cancel')}
                                disabled={vm.selectedVMIDs.length === 0 || vm.batchRateLimited}
                                onConfirm={() => vm.submitBatchPowerSelected('STOP')}
                            >
                                <Button
                                    onClick={(event) => event.preventDefault()}
                                    loading={vm.batchSubmitPending}
                                    disabled={vm.batchRateLimited || vm.selectedVMIDs.length === 0}
                                >
                                    {t('batch.stop_selected')}
                                </Button>
                            </Popconfirm>
                        </PermissionGuard>
                        <PermissionGuard permission="vm:operate">
                            <Popconfirm
                                title={t('batch.restart_confirm', { count: vm.selectedVMIDs.length })}
                                okText={t('common:button.confirm')}
                                cancelText={t('common:button.cancel')}
                                disabled={vm.selectedVMIDs.length === 0 || vm.batchRateLimited}
                                onConfirm={() => vm.submitBatchPowerSelected('RESTART')}
                            >
                                <Button
                                    onClick={(event) => event.preventDefault()}
                                    loading={vm.batchSubmitPending}
                                    disabled={vm.batchRateLimited || vm.selectedVMIDs.length === 0}
                                >
                                    {t('batch.restart_selected')}
                                </Button>
                            </Popconfirm>
                        </PermissionGuard>
                        <PermissionGuard permission="vm:delete">
                            <Popconfirm
                                title={t('batch.delete_confirm', { count: vm.selectedVMIDs.length })}
                                okText={t('common:button.confirm')}
                                cancelText={t('common:button.cancel')}
                                disabled={vm.selectedVMIDs.length === 0 || vm.batchRateLimited}
                                onConfirm={vm.submitBatchDeleteSelected}
                            >
                                <Button
                                    danger
                                    onClick={(event) => event.preventDefault()}
                                    loading={vm.batchSubmitPending}
                                    disabled={vm.batchRateLimited || vm.selectedVMIDs.length === 0}
                                >
                                    {t('batch.delete_selected')}
                                </Button>
                            </Popconfirm>
                        </PermissionGuard>
                        <PermissionGuard permission="vm:operate">
                            <Button
                                icon={<SettingOutlined />}
                                onClick={vm.openBatchModifyModal}
                                loading={vm.modifySubmitPending}
                                disabled={vm.batchRateLimited}
                            >
                                {t('batch.modify_selected')}
                            </Button>
                        </PermissionGuard>
                    </Space>
                </Space>
                {vm.batchRateLimited && (
                    <div className="vm-page-inline-alert">
                        <Text type="warning">
                            {t('batch.rate_limited_wait', { seconds: vm.batchRetryAfterSeconds })}
                        </Text>
                    </div>
                )}
            </PageSurface>

            <PageSurface className="vm-page-surface vm-page-surface--tight vm-page-surface--search">
                <div className="vm-page-search-shell">
                    <PageSearchToolbar
                        searchValue={vm.filters.search}
                        searchDraftValue={quickSearchDraft}
                        onSearchDraftChange={setQuickSearchDraft}
                        onSearchChange={applyQuickSearch}
                        searchPlaceholder={t('search_placeholder', 'Search VM name, scope, cluster, or operating system')}
                        searchHelp={t('search_help', 'Press Enter or click Search. Quick search matches VM name, hostname, namespace, cluster, system, service, IP, and operating system.')}
                        searchTestId="vms-quick-search"
                        advancedSearch={{
                            open: advancedSearchOpen,
                            onToggle: () => setAdvancedSearchOpen((open) => !open),
                            openLabel: t('common:search.advanced', { defaultValue: 'Advanced search' }),
                            closeLabel: t('common:search.hide_advanced', { defaultValue: 'Hide advanced search' }),
                            title: t('advanced_search_title', 'Advanced search'),
                            toggleTestId: 'vms-advanced-search-toggle',
                            content: (
                                <Space direction="vertical" size={10} className="vm-page-search-advanced">
                                    <Text type="secondary" className="vm-page-supporting-text">
                                        {t('advanced_search_help', 'Select exact filters here. Options support keyword matching, but the applied filter remains an exact match.')}
                                    </Text>
                                    <Space wrap className="vm-page-filter-grid">
                                        <Select
                                            allowClear
                                            showSearch
                                            filterOption={filterOptionByLabel}
                                            optionFilterProp="label"
                                            placeholder={t('search.status', 'Status')}
                                            value={statusDraft || undefined}
                                            options={statusOptions}
                                            className="vm-page-filter vm-page-filter--sm"
                                            onChange={(value) => setStatusDraft(value ?? '')}
                                            data-testid="vms-filter-status"
                                        />
                                        <Select
                                            allowClear
                                            showSearch
                                            filterOption={filterOptionByLabel}
                                            optionFilterProp="label"
                                            placeholder={t('search.namespace', 'Namespace')}
                                            value={namespaceDraft || undefined}
                                            options={namespaceOptions}
                                            className="vm-page-filter"
                                            onChange={(value) => setNamespaceDraft(value ?? '')}
                                            data-testid="vms-filter-namespace"
                                        />
                                        <Select
                                            allowClear
                                            showSearch
                                            filterOption={filterOptionByLabel}
                                            optionFilterProp="label"
                                            placeholder={t('search.cluster', 'Cluster')}
                                            value={clusterDraft || undefined}
                                            options={clusterOptions}
                                            className="vm-page-filter"
                                            onChange={(value) => setClusterDraft(value ?? '')}
                                            data-testid="vms-filter-cluster"
                                        />
                                        <Select
                                            allowClear
                                            showSearch
                                            filterOption={filterOptionByLabel}
                                            optionFilterProp="label"
                                            placeholder={t('search.system', 'System')}
                                            value={systemDraft || undefined}
                                            options={systemOptions}
                                            className="vm-page-filter"
                                            onChange={(value) => setSystemDraft(value ?? '')}
                                            data-testid="vms-filter-system"
                                        />
                                        <Select
                                            allowClear
                                            showSearch
                                            filterOption={filterOptionByLabel}
                                            optionFilterProp="label"
                                            placeholder={t('search.service', 'Service')}
                                            value={serviceDraft || undefined}
                                            options={serviceOptions}
                                            className="vm-page-filter vm-page-filter--lg"
                                            onChange={(value) => setServiceDraft(value ?? '')}
                                            data-testid="vms-filter-service"
                                        />
                                        <Select
                                            allowClear
                                            showSearch
                                            filterOption={filterOptionByLabel}
                                            optionFilterProp="label"
                                            placeholder={t('search.operating_system', 'Operating system')}
                                            value={osDraft || undefined}
                                            options={operatingSystemOptions}
                                            className="vm-page-filter vm-page-filter--lg"
                                            onChange={(value) => setOsDraft(value ?? '')}
                                            data-testid="vms-filter-os"
                                        />
                                        <Select
                                            allowClear
                                            showSearch
                                            filterOption={filterOptionByLabel}
                                            optionFilterProp="label"
                                            placeholder={t('search.ip_address', 'IP address')}
                                            value={ipAddressDraft || undefined}
                                            options={ipAddressOptions}
                                            className="vm-page-filter"
                                            onChange={(value) => setIpAddressDraft(value ?? '')}
                                            data-testid="vms-filter-ip-address"
                                        />
                                        <Button
                                            type="primary"
                                            onClick={applyAdvancedFilters}
                                            data-testid="vms-advanced-search-submit"
                                        >
                                            {t('common:button.search')}
                                        </Button>
                                    </Space>
                                </Space>
                            ),
                        }}
                        hasActiveFilters={hasActiveFilters}
                        onClear={resetFilters}
                        clearLabel={t('common:button.clear_filters', { defaultValue: 'Clear filters' })}
                        clearTestId="vms-clear-filters"
                    />
                </div>
            </PageSurface>

            <VMListTable
                t={t}
                vmData={vm.vmData}
                isLoading={vm.isLoading}
                page={vm.page}
                pageSize={vm.pageSize}
                onPageChange={(page, pageSize) => {
                    vm.setPage(page);
                    vm.setPageSize(pageSize);
                }}
                onStart={vm.startVM}
                onStop={vm.stopVM}
                onRestart={vm.restartVM}
                onConsole={(vmRecord) => router.push(`/vms/${vmRecord.id}?focus=console`)}
                onDelete={vm.openDeleteModal}
                onModify={vm.openModifyModal}
                onRequestSimilar={(vmId) => void vm.openSimilarRequest(vmId)}
                onDetail={(vmId) => router.push(`/vms/${vmId}`)}
                onOpenSystem={(systemId) => router.push(`/systems?detail_system_id=${systemId}`)}
                onOpenService={(systemId, serviceId) =>
                    router.push(`/services?system_id=${systemId}&detail_service_id=${serviceId}`)
                }
                contextSystemId={scopedSystemId}
                contextServiceId={scopedServiceId}
                selectedRowKeys={vm.selectedVMIDs}
                onSelectionChange={vm.setSelectedVMIDs}
            />
            {vm.activeBatchID && (
                <PageSurface className="vm-page-surface vm-page-surface--status">
                    <div
                        data-testid="batch-status-live"
                        aria-live="polite"
                        style={{
                            position: 'absolute',
                            width: 1,
                            height: 1,
                            padding: 0,
                            margin: -1,
                            overflow: 'hidden',
                            clip: 'rect(0, 0, 0, 0)',
                            whiteSpace: 'nowrap',
                            border: 0,
                        }}
                    >
                        {t('batch.live_status_summary', {
                            batch_id: vm.activeBatchID,
                            status: vm.batchStatus?.status ?? '—',
                            success_count: vm.batchStatus?.success_count ?? 0,
                            failed_count: vm.batchStatus?.failed_count ?? 0,
                            pending_count: vm.batchStatus?.pending_count ?? 0,
                        })}
                    </div>
                    <Space className="vm-page-header-row" wrap>
                        <Space direction="vertical" size={2}>
                            <Text strong>{t('batch.current_batch')}</Text>
                            <Text type="secondary" className="vm-page-supporting-text">
                                {t('batch.live_status_summary', {
                                    batch_id: vm.activeBatchID,
                                    status: vm.batchStatus?.status ?? '—',
                                    success_count: vm.batchStatus?.success_count ?? 0,
                                    failed_count: vm.batchStatus?.failed_count ?? 0,
                                    pending_count: vm.batchStatus?.pending_count ?? 0,
                                })}
                            </Text>
                            {lastBatchActionFeedback !== '' && (
                                <Text type="secondary" className="vm-page-supporting-text">{lastBatchActionFeedback}</Text>
                            )}
                        </Space>
                        <Space wrap className="copy-friendly-actions vm-page-actions">
                            <Button
                                icon={<ReloadOutlined />}
                                onClick={vm.refreshBatch}
                                loading={vm.batchLoading}
                            >
                                {t('common:button.refresh')}
                            </Button>
                            <Button
                                type="primary"
                                onClick={() => router.push('/tickets?tab=batch_jobs')}
                            >
                                {t('batch.open_workbench')}
                            </Button>
                            <Button onClick={vm.clearBatchTracking}>
                                {t('batch.clear')}
                            </Button>
                        </Space>
                    </Space>
                </PageSurface>
            )}
            </div>

            <VMRequestWizard
                t={t}
                open={vm.wizardOpen}
                step={vm.wizardStep}
                setStep={vm.setWizardStep}
                requestMode={vm.requestMode}
                onRequestModeChange={vm.setRequestMode}
                form={vm.form}
                wizardSteps={vm.wizardSteps}
                selectedSystemId={vm.selectedSystemId}
                onSystemChange={vm.onSystemChange}
                systemsData={vm.systemsData}
                servicesData={vm.servicesData}
                templatesData={vm.templatesData}
                sizesData={vm.sizesData}
                selectedTemplate={vm.selectedTemplate}
                selectedSize={vm.selectedSize}
                placementHint={vm.placementHint}
                placementHintLoading={vm.placementHintLoading}
                serviceIdValue={vm.serviceIdValue}
                namespaceValue={vm.namespaceValue}
                namespaceOptions={vm.namespaceOptions}
                reasonValue={vm.reasonValue}
                batchCountValue={vm.batchCountValue}
                isSubmitting={vm.createVMRequest.isPending || vm.batchSubmitPending}
                onCancel={vm.closeWizard}
                onNext={vm.goToNextWizardStep}
                onSubmit={vm.submitWizard}
            />

            {vm.modifyOpen ? (
                <Modal
                    title={
                        vm.modifyScope === 'batch'
                            ? t('modify.batch_title', { count: vm.selectedVMIDs.length })
                            : t('modify.title', { name: vm.modifyTargetVM?.name ?? 'VM' })
                    }
                    open={vm.modifyOpen}
                    onOk={() => void vm.submitModify()}
                    onCancel={vm.closeModifyModal}
                    confirmLoading={vm.modifySubmitPending}
                    okButtonProps={{ disabled: vm.modifySubmitDisabled }}
                    okText={t('modify.submit')}
                    width={720}
                    data-testid="vm-modify-modal"
                >
                    <Form form={vm.modifyForm} layout="vertical" preserve={false}>
                        {vm.modifyScope === 'single' && vm.modifyContext && (
                            <Descriptions
                                size="small"
                                column={3}
                                items={[
                                    {
                                        key: 'cpu',
                                        label: t('modify.current_cpu'),
                                        children: vm.modifyContext.current_cpu_cores,
                                    },
                                    {
                                        key: 'memory',
                                        label: t('modify.current_memory'),
                                        children: `${vm.modifyContext.current_memory_gi} Gi`,
                                    },
                                    {
                                        key: 'disk',
                                        label: t('modify.current_disk'),
                                        children: `${vm.modifyContext.current_disk_gb} Gi`,
                                    },
                                ]}
                                style={{ marginBottom: 16 }}
                            />
                        )}
                        {vm.modifyScope === 'single' && vm.modifyContextLoading && (
                            <Alert
                                type="info"
                                showIcon={true}
                                message={t('modify.loading')}
                                style={{ marginBottom: 16 }}
                            />
                        )}
                        <Form.Item
                            name="reason"
                            label={t('modify.reason')}
                            rules={[{ required: true, message: t('modify.reason_required') }]}
                        >
                            <Input.TextArea rows={3} />
                        </Form.Item>
                        <Form.Item
                            name="target_cpu_cores"
                            label={t('modify.target_cpu')}
                            extra={
                                vm.modifyScope === 'single' && vm.modifyContext && !vm.modifyContext.cpu_supported
                                    ? vm.modifyContext.cpu_reason
                                    : t('modify.target_cpu_hint')
                            }
                        >
                            <InputNumber
                                min={1}
                                step={1}
                                precision={0}
                                style={{ width: '100%' }}
                                disabled={vm.modifyScope === 'single' && !!vm.modifyContext && !vm.modifyContext.cpu_supported}
                            />
                        </Form.Item>
                        <Form.Item
                            name="target_memory_gi"
                            label={t('modify.target_memory')}
                            extra={
                                vm.modifyScope === 'single' && vm.modifyContext && !vm.modifyContext.memory_supported
                                    ? vm.modifyContext.memory_reason
                                    : t('modify.target_memory_hint')
                            }
                        >
                            <InputNumber
                                min={0.5}
                                step={0.5}
                                precision={1}
                                style={{ width: '100%' }}
                                disabled={vm.modifyScope === 'single' && !!vm.modifyContext && !vm.modifyContext.memory_supported}
                            />
                        </Form.Item>
                        <Form.Item
                            name="target_disk_gb"
                            label={t('modify.target_disk')}
                            extra={
                                vm.modifyScope === 'single' && vm.modifyContext && !vm.modifyContext.disk_supported
                                    ? vm.modifyContext.disk_reason
                                    : t('modify.target_disk_hint')
                            }
                        >
                            <InputNumber
                                min={1}
                                step={1}
                                precision={0}
                                style={{ width: '100%' }}
                                disabled={vm.modifyScope === 'single' && !!vm.modifyContext && !vm.modifyContext.disk_supported}
                            />
                        </Form.Item>
                        {vm.modifyScope === 'batch' && (
                            <Alert
                                type="info"
                                showIcon={true}
                                message={t('modify.batch_scope', { count: vm.selectedVMIDs.length })}
                            />
                        )}
                    </Form>
                </Modal>
            ) : null}

            {vm.deleteOpen ? (
                <Modal
                    title={(
                        <Space>
                            <ExclamationCircleOutlined style={{ color: '#ff4d4f' }} />
                            {t('action.delete_confirm')}
                        </Space>
                    )}
                    open={vm.deleteOpen}
                    onOk={vm.submitDelete}
                    onCancel={vm.closeDeleteModal}
                    confirmLoading={vm.deletePending}
                    okButtonProps={{
                        danger: true,
                        disabled: vm.deletingVM?.environment !== 'test' ? vm.deleteConfirmName !== vm.deletingVM?.name : false,
                    }}
                    okText={t('common:button.delete')}
                    data-testid="vm-delete-modal"
                >
                    <Paragraph>
                        {t('action.delete_confirm_name', { name: vm.deletingVM?.name })}
                    </Paragraph>
                    {vm.deletingVM?.environment !== 'test' && (
                        <>
                            <Paragraph type="secondary">
                                {t('action.delete_type_name_hint')}
                            </Paragraph>
                            <Input
                                value={vm.deleteConfirmName}
                                onChange={(e) => vm.setDeleteConfirmName(e.target.value)}
                                placeholder={vm.deletingVM?.name}
                                status={vm.deleteConfirmName && vm.deleteConfirmName !== vm.deletingVM?.name ? 'error' : undefined}
                            />
                        </>
                    )}
                </Modal>
            ) : null}
        </div>
    );
}
