'use client';

import { useMemo, useRef, useState } from 'react';
import {
    Alert,
    Button,
    Divider,
    Form,
    Input,
    Modal,
    Radio,
    Select,
    Space,
    Switch,
    Table,
    Tag,
    Tooltip,
    Typography,
} from 'antd';
import type { FormInstance, InputRef } from 'antd';
import type { ColumnsType, FilterDropdownProps } from 'antd/es/table/interface';
import {
    DeleteOutlined,
    EditOutlined,
    FileTextOutlined,
    PlusOutlined,
    ReloadOutlined,
    SearchOutlined,
} from '@ant-design/icons';
import { useTranslation } from 'react-i18next';
import { useRouter } from 'next/navigation';

import { ActionEmptyState } from '@/components/feedback/ActionEmptyState';
import { SummaryMetricCard } from '@/components/feedback/SummaryMetricCard';
import {
    QueueReviewGlyph,
    RequestsOverviewGlyph,
    ServiceWorkspaceGlyph,
    TemplateCatalogGlyph,
} from '@/components/illustrations/DashboardIllustrations';
import { PageHeader, PageSurface } from '@/components/layouts/PageSection';
import {
    buildDashboardSetupResumeHref,
    resolveNextSetupAction,
} from '@/features/setup-guide/flow';
import { useAutoOpenIntent } from '@/features/setup-guide/hooks/useAutoOpenIntent';
import { useSetupGuide } from '@/features/setup-guide/hooks/useSetupGuide';
import { useAdminTemplatesController } from '../hooks/useAdminTemplatesController';
import {
    buildTemplatePresetValues,
    getTemplateCloudInitExample,
    getTemplateImageURLSuggestions,
    getTemplateOSVersionSuggestions,
    getTemplatePresetGroups,
    getTemplatePVCNameSuggestions,
    getTemplatePVCNamespaceSuggestions,
    TEMPLATE_OS_FAMILY_OPTIONS,
    type TemplatePresetKey,
} from '../templatePresets';
import { OS_COLOR_MAP, type Template } from '../types';
import { getTemplateRequestFlowStatus } from '../requestFlow';

const { Text } = Typography;

function getTemplateSourceType(template: Pick<Template, 'source_type' | 'image_url' | 'pvc_name'>): string {
    if (template.source_type) {
        return template.source_type;
    }
    if (template.pvc_name) {
        return 'cdi_pvc_clone';
    }
    if (template.image_url) {
        return 'containerdisk';
    }
    return '';
}

function getTemplateSourceLabel(
    template: Pick<Template, 'source_type' | 'image_url' | 'pvc_name'>,
    t: ReturnType<typeof useTranslation>['t'],
): string {
    const sourceType = getTemplateSourceType(template);
    switch (sourceType) {
        case 'containerdisk':
            return t('templates.source_containerdisk');
        case 'cdi_image_import':
            return t('templates.source_cdi_import');
        case 'cdi_pvc_clone':
            return t('templates.source_cdi_clone');
        default:
            return t('templates.source_unconfigured', 'Source not configured');
    }
}

function getTemplateRequestFlowMeta(
    template: Pick<Template, 'enabled' | 'catalog_scope' | 'source_type' | 'image_url' | 'pvc_name'>,
    t: ReturnType<typeof useTranslation>['t'],
): { color: string; label: string; description: string } {
    const status = getTemplateRequestFlowStatus(template);
    switch (status) {
        case 'self_service':
            return {
                color: 'green',
                label: t('templates.request_flow_self_service'),
                description: t(
                    'templates.request_flow_reason_self_service',
                    'This template is already visible in the VM request wizard.',
                ),
            };
        case 'admin_only_source':
            return {
                color: 'orange',
                label: t('templates.request_flow_admin_only'),
                description: t('templates.request_flow_reason_admin_only'),
            };
        case 'hidden_unclassified':
            return {
                color: 'gold',
                label: t('templates.request_flow_hidden'),
                description: t('templates.request_flow_reason_hidden'),
            };
        case 'disabled':
            return {
                color: 'default',
                label: t('templates.request_flow_disabled'),
                description: t('templates.request_flow_reason_disabled'),
            };
        default:
            return {
                color: 'red',
                label: t('templates.request_flow_unavailable'),
                description: t('templates.request_flow_reason_unsupported'),
            };
    }
}

function getTemplateSourceSummary(record: Template, t: ReturnType<typeof useTranslation>['t']): string {
    if (record.source_type === 'cdi_pvc_clone' || record.pvc_name) {
        const namespace = record.pvc_namespace?.trim() || t('common:status.unknown', { defaultValue: 'Unknown' });
        const pvcName = record.pvc_name?.trim() || t('templates.source_unconfigured', 'Source not configured');
        return `${namespace} / ${pvcName}`;
    }
    if (record.image_url?.trim()) {
        return record.image_url.trim();
    }
    return t('templates.source_unconfigured', 'Source not configured');
}

function getTemplateOsFamilyLabel(osFamily: string | undefined, t: ReturnType<typeof useTranslation>['t']): string {
    if (!osFamily) {
        return t('common:status.unknown', { defaultValue: 'Unknown' });
    }
    const normalized = osFamily.toLowerCase();
    return t(`templates.os_family_${normalized}`, { defaultValue: osFamily });
}

function SuggestedValueInput({
    value,
    onChange,
    options,
    selectPlaceholder,
    inputPlaceholder,
    customToggleLabel,
    suggestedToggleLabel,
    emptyHint,
    noSuggestionsHint,
}: {
    value?: string;
    onChange?: (nextValue?: string) => void;
    options: string[];
    selectPlaceholder: string;
    inputPlaceholder: string;
    customToggleLabel: string;
    suggestedToggleLabel: string;
    emptyHint: string;
    noSuggestionsHint: string;
}) {
    const [forceCustom, setForceCustom] = useState(false);
    const hasSuggestions = options.length > 0;
    const [firstOption] = options;
    const matchesSuggestion = typeof value === 'string' && options.includes(value);
    const useCustomInput =
        forceCustom || !hasSuggestions || (typeof value === 'string' && value.trim() !== '' && !matchesSuggestion);

    return (
        <Space direction="vertical" size={8} style={{ width: '100%' }}>
            <Space.Compact style={{ width: '100%' }}>
                {useCustomInput ? (
                    <Input
                        value={value}
                        placeholder={inputPlaceholder}
                        onChange={(event) => onChange?.(event.target.value)}
                    />
                ) : (
                    <Select
                        showSearch
                        allowClear
                        value={value}
                        placeholder={selectPlaceholder}
                        options={options.map((option) => ({ label: option, value: option }))}
                        optionFilterProp="label"
                        onChange={(nextValue) => onChange?.(nextValue)}
                    />
                )}
                {hasSuggestions ? (
                    <Button
                        onClick={() => {
                            if (useCustomInput) {
                                setForceCustom(false);
                                onChange?.(matchesSuggestion ? value : firstOption);
                                return;
                            }
                            setForceCustom(true);
                        }}
                    >
                        {useCustomInput ? suggestedToggleLabel : customToggleLabel}
                    </Button>
                ) : null}
            </Space.Compact>
            <Text type="secondary">
                {hasSuggestions ? emptyHint : noSuggestionsHint}
            </Text>
        </Space>
    );
}

function ExperimentalSourceGate({
    title,
    description,
    buttonLabel,
    onEnable,
}: {
    title: string;
    description: string;
    buttonLabel: string;
    onEnable: () => void;
}) {
    return (
        <Alert
            type="info"
            showIcon
            style={{ marginBottom: 16 }}
            message={title}
            description={description}
            action={
                <Button size="small" onClick={onEnable}>
                    {buttonLabel}
                </Button>
            }
        />
    );
}

function applyTemplatePreset(
    form: FormInstance,
    presetKey: TemplatePresetKey,
    enableExperimentalSources?: () => void,
) {
    const values = buildTemplatePresetValues(presetKey);
    if (values.source_type === 'containerdisk') {
        enableExperimentalSources?.();
    }
    form.setFieldsValue({
        image_url: undefined,
        pvc_name: undefined,
        pvc_namespace: undefined,
        ...values,
    });
}

function TemplatePresetPicker({
    t,
    form,
    enableExperimentalSources,
}: {
    t: ReturnType<typeof useTranslation>['t'];
    form: FormInstance;
    enableExperimentalSources?: () => void;
}) {
    const groups = getTemplatePresetGroups();

    return (
        <Alert
            type="info"
            showIcon
            style={{ marginBottom: 16 }}
            message={t('templates.preset_title', 'Preset Catalog')}
            description={(
                <Space direction="vertical" size={12} style={{ width: '100%' }}>
                    <Text type="secondary">
                        {t(
                            'templates.preset_description',
                            'Start with the closest preset, then fine-tune only the differences. Presets fill the image source, catalog scope, operating system, and cloud-init example together.'
                        )}
                    </Text>
                    {groups.map((group) => (
                        <Space key={group.sourceType} direction="vertical" size={6} style={{ width: '100%' }}>
                            <Space size={8} wrap>
                                <Text strong>{t(group.titleKey)}</Text>
                                <Text type="secondary">{t(group.descriptionKey)}</Text>
                            </Space>
                            {group.scopeGroups.map((scopeGroup) => (
                                <Space
                                    key={`${group.sourceType}-${scopeGroup.scope}`}
                                    direction="vertical"
                                    size={4}
                                    style={{ width: '100%' }}
                                >
                                    <Text type="secondary">{t(scopeGroup.titleKey)}</Text>
                                    <Space wrap>
                                        {scopeGroup.items.map((preset) => (
                                            <Button
                                                key={preset.key}
                                                size="small"
                                                onClick={() => applyTemplatePreset(
                                                    form,
                                                    preset.key as TemplatePresetKey,
                                                    enableExperimentalSources,
                                                )}
                                            >
                                                {t(preset.labelKey)}
                                            </Button>
                                        ))}
                                    </Space>
                                </Space>
                            ))}
                        </Space>
                    ))}
                </Space>
            )}
        />
    );
}

function highlightText(text: string, highlight: string): React.ReactNode {
    if (!highlight) {
        return text;
    }
    const index = text.toLowerCase().indexOf(highlight.toLowerCase());
    if (index === -1) {
        return text;
    }
    const before = text.slice(0, index);
    const match = text.slice(index, index + highlight.length);
    const after = text.slice(index + highlight.length);
    return (
        <>
            {before}
            <span style={{ backgroundColor: '#ffc069', fontWeight: 600, padding: '0 2px' }}>
                {match}
            </span>
            {after}
        </>
    );
}

export function AdminTemplatesContent() {
    const { t } = useTranslation(['admin', 'common', 'error']);
    const searchInputRef = useRef<InputRef>(null);
    const router = useRouter();
    const setupGuide = useSetupGuide();
    const templates = useAdminTemplatesController({
        t,
        onCreateSuccess: (_template, context) => {
            if (!context.isFirstTemplate) {
                return false;
            }
            const nextAction = resolveNextSetupAction(setupGuide, 'template');
            if (!nextAction) {
                return false;
            }
            router.push(buildDashboardSetupResumeHref(nextAction));
            return true;
        },
    });

    useAutoOpenIntent('create-template', () => {
        templates.openCreateModal();
    });
    const catalogScopeOptions = [
        { label: t('templates.scope_unclassified'), value: 'unclassified' },
        { label: t('templates.scope_test'), value: 'test' },
        { label: t('templates.scope_prod'), value: 'prod' },
        { label: t('templates.scope_all'), value: 'all' },
    ];

    const getColumnSearchProps = (dataIndex: keyof Template): Partial<ColumnsType<Template>[number]> => ({
        filterDropdown: ({ setSelectedKeys, selectedKeys, confirm, clearFilters }: FilterDropdownProps) => (
            <div style={{ padding: 8 }} onKeyDown={(e) => e.stopPropagation()}>
                {(() => {
                    const currentValue = typeof selectedKeys[0] === 'symbol'
                        ? ''
                        : typeof selectedKeys[0] === 'undefined'
                            ? ''
                            : String(selectedKeys[0]);
                    return (
                        <Input
                            ref={searchInputRef}
                            placeholder={`${t('common:button.search')} ${String(dataIndex)}`}
                            value={currentValue}
                            onChange={(e) => setSelectedKeys(e.target.value ? [e.target.value] : [])}
                            onPressEnter={() => {
                                confirm();
                                templates.setSearchText(currentValue);
                                templates.setSearchedColumn(String(dataIndex));
                            }}
                            style={{ marginBottom: 8, display: 'block' }}
                        />
                    );
                })()}
                <Space>
                    <Button
                        type="primary"
                        onClick={() => {
                            const currentValue = typeof selectedKeys[0] === 'symbol'
                                ? ''
                                : typeof selectedKeys[0] === 'undefined'
                                    ? ''
                                    : String(selectedKeys[0]);
                            confirm();
                            templates.setSearchText(currentValue);
                            templates.setSearchedColumn(String(dataIndex));
                        }}
                        icon={<SearchOutlined />}
                        size="small"
                        style={{ width: 90 }}
                    >
                        {t('common:button.search')}
                    </Button>
                    <Button
                        onClick={() => {
                            clearFilters?.();
                            templates.setSearchText('');
                            templates.setSearchedColumn('');
                            confirm();
                        }}
                        size="small"
                        style={{ width: 90 }}
                    >
                        {t('common:button.reset')}
                    </Button>
                </Space>
            </div>
        ),
        filterIcon: (filtered: boolean) => (
            <SearchOutlined style={{ color: filtered ? '#1677ff' : undefined }} />
        ),
        onFilter: (value, record) =>
            (record[dataIndex] ?? '').toString().toLowerCase().includes((value as string).toLowerCase()),
        filterDropdownProps: {
            onOpenChange: (visible) => {
                if (visible) {
                    setTimeout(() => searchInputRef.current?.select(), 100);
                }
            },
        },
    });

    const columns: ColumnsType<Template> = [
        {
            title: t('common:table.name'),
            dataIndex: 'name',
            key: 'name',
            ...getColumnSearchProps('name'),
            sorter: (a, b) => a.name.localeCompare(b.name),
            render: (name: string, record: Template) => (
                <Space>
                    <FileTextOutlined style={{ color: '#1677ff' }} />
                    <div>
                        <Text strong>
                            {templates.searchedColumn === 'name'
                                ? highlightText(record.display_name ?? name, templates.searchText)
                                : (record.display_name ?? name)}
                        </Text>
                        <br />
                        <Text type="secondary" style={{ fontSize: 12 }}>
                            {templates.searchedColumn === 'name' ? highlightText(name, templates.searchText) : name}
                        </Text>
                    </div>
                </Space>
            ),
        },
        {
            title: t('templates.os_profile', 'OS Profile'),
            key: 'os_profile',
            width: 160,
            filters: templates.osFamilyFilters,
            onFilter: (value, record) => record.os_family === value,
            render: (_: unknown, record: Template) => {
                const family = record.os_family;
                const familyColor = family ? (OS_COLOR_MAP[family.toLowerCase()] ?? 'default') : 'default';
                return (
                    <Space direction="vertical" size={4}>
                        <Space size={[4, 4]} wrap>
                            <Tag color={familyColor}>
                                {getTemplateOsFamilyLabel(family, t)}
                            </Tag>
                            {record.os_version ? <Tag>{record.os_version}</Tag> : null}
                        </Space>
                        <Text type="secondary" style={{ fontSize: 12 }}>
                            {record.os_version
                                ? t(
                                    'templates.os_profile_summary',
                                    {
                                        defaultValue: `${getTemplateOsFamilyLabel(family, t)} · ${record.os_version}`,
                                        family: getTemplateOsFamilyLabel(family, t),
                                        version: record.os_version,
                                    },
                                )
                                : getTemplateOsFamilyLabel(family, t)}
                        </Text>
                    </Space>
                );
            },
        },
        {
            title: t('templates.catalog_publication', 'Catalog Publication'),
            key: 'request_flow',
            width: 260,
            render: (_: unknown, record: Template) => {
                const flow = getTemplateRequestFlowMeta(record, t);
                const scope = (record.catalog_scope ?? 'unclassified').toLowerCase();
                const scopeColor = scope === 'prod' ? 'red' : scope === 'all' ? 'blue' : scope === 'test' ? 'gold' : 'default';
                return (
                    <Space direction="vertical" size={4}>
                        <Space size={[4, 4]} wrap>
                            <Tag color={flow.color}>{flow.label}</Tag>
                            <Tag color={scopeColor}>{t(`templates.scope_${scope}`, { defaultValue: scope })}</Tag>
                            <Tag color={record.enabled !== false ? 'green' : 'default'}>
                                {record.enabled !== false ? t('common:status.active') : t('common:status.disabled')}
                            </Tag>
                        </Space>
                        <Text type="secondary" style={{ fontSize: 12 }}>
                            {flow.description}
                        </Text>
                    </Space>
                );
            },
        },
        {
            title: t('templates.image_source'),
            key: 'image_source',
            width: 260,
            render: (_: unknown, record: Template) => {
                const sourceSummary = getTemplateSourceSummary(record, t);
                return (
                    <Space direction="vertical" size={4} style={{ width: '100%' }}>
                        <Space size={[4, 4]} wrap>
                            <Tag>{getTemplateSourceLabel(record, t)}</Tag>
                        </Space>
                        <Tooltip title={sourceSummary}>
                            <Text
                                style={{
                                    display: 'inline-block',
                                    maxWidth: 220,
                                }}
                                ellipsis={true}
                            >
                                {sourceSummary}
                            </Text>
                        </Tooltip>
                    </Space>
                );
            },
        },
        {
            title: t('common:table.description'),
            dataIndex: 'description',
            key: 'description',
            ellipsis: true,
            render: (desc: string | undefined) => (
                <Text type="secondary">{desc || '—'}</Text>
            ),
        },
        {
            title: t('common:table.actions'),
            key: 'actions',
            width: 150,
            render: (_: unknown, record: Template) => (
                <Space size="small">
                    <Button
                        type="link"
                        size="small"
                        data-testid={`template-action-edit-${record.id}`}
                        icon={<EditOutlined />}
                        onClick={() => templates.openEditModal(record)}
                    >
                        {t('common:button.edit')}
                    </Button>
                    <Button
                        type="link"
                        size="small"
                        danger
                        data-testid={`template-action-delete-${record.id}`}
                        icon={<DeleteOutlined />}
                        onClick={() => templates.openDeleteModal(record)}
                    >
                        {t('common:button.delete')}
                    </Button>
                </Space>
            ),
        },
    ];

    const templateSummary = useMemo(() => {
        const items = templates.filteredItems;
        const enabledCount = items.filter((item) => item.enabled !== false).length;
        const selfServiceCount = items.filter((item) => getTemplateRequestFlowStatus(item) === 'self_service').length;
        const attentionCount = items.filter((item) => getTemplateRequestFlowStatus(item) !== 'self_service').length;
        return {
            totalCount: items.length,
            enabledCount,
            selfServiceCount,
            attentionCount,
        };
    }, [templates.filteredItems]);

    return (
        <div>
            {templates.messageContextHolder}
            <PageHeader
                title={t('templates.title')}
                subtitle={t('templates.subtitle')}
                actions={(
                    <Space>
                    <Input
                        placeholder={t('common:button.search')}
                        prefix={<SearchOutlined />}
                        value={templates.globalSearch}
                        onChange={(e) => templates.setGlobalSearch(e.target.value)}
                        allowClear
                        style={{ width: 220 }}
                    />
                    <Button icon={<ReloadOutlined />} onClick={() => templates.refetch()}>
                        {t('common:button.refresh')}
                    </Button>
                    <Button
                        type="primary"
                        icon={<PlusOutlined />}
                        data-testid="template-create-button"
                        onClick={templates.openCreateModal}
                    >
                        {t('common:button.add')}
                    </Button>
                    </Space>
                )}
            />
            <div className="summary-card-grid">
                <SummaryMetricCard
                    title={t('templates.summary.total_title', 'Catalog templates')}
                    value={templateSummary.totalCount}
                    description={t('templates.summary.total_description', 'Templates visible after the current filters.')}
                    visual={<TemplateCatalogGlyph className="summary-metric-card__art" />}
                    accentColor="#D66A1F"
                    surfaceColor="#FFF4E5"
                />
                <SummaryMetricCard
                    title={t('templates.summary.enabled_title', 'Enabled')}
                    value={templateSummary.enabledCount}
                    description={t('templates.summary.enabled_description', 'Templates currently published in the catalog.')}
                    visual={<ServiceWorkspaceGlyph className="summary-metric-card__art" />}
                    accentColor="#0F8F57"
                    surfaceColor="#E8FFF2"
                />
                <SummaryMetricCard
                    title={t('templates.summary.self_service_title', 'Request-ready')}
                    value={templateSummary.selfServiceCount}
                    description={t('templates.summary.self_service_description', 'Templates already available in the request wizard.')}
                    visual={<RequestsOverviewGlyph className="summary-metric-card__art" />}
                    accentColor="#1D5BFF"
                    surfaceColor="#E6F4FF"
                />
                <SummaryMetricCard
                    title={t('templates.summary.attention_title', 'Needs review')}
                    value={templateSummary.attentionCount}
                    description={t('templates.summary.attention_description', 'Templates still hidden, disabled, or admin-only.')}
                    visual={<QueueReviewGlyph className="summary-metric-card__art" />}
                    accentColor="#6D4DE3"
                    surfaceColor="#F5EDFF"
                />
            </div>

            <PageSurface flush={true}>
                <div style={{
                    opacity: templates.isStale ? 0.6 : 1,
                    transition: templates.isStale ? 'opacity 0.2s 0.1s linear' : 'opacity 0s 0s linear',
                }}>
                    <Table<Template>
                        columns={columns}
                        dataSource={templates.filteredItems}
                        rowKey="id"
                        loading={templates.isLoading}
                        pagination={{
                            current: templates.page,
                            total: templates.data?.pagination?.total ?? templates.filteredItems.length,
                            pageSize: 20,
                            onChange: templates.setPage,
                            showTotal: (total) => t('common:table.total', { total }),
                            showSizeChanger: false,
                        }}
                        size="middle"
                        locale={{
                            emptyText: (
                                <ActionEmptyState
                                    compact={true}
                                    title={
                                        templates.deferredSearch
                                            ? t('common:message.no_data')
                                            : t('templates.empty')
                                    }
                                    description={
                                        templates.deferredSearch
                                            ? t('templates.empty_filtered_description', 'Try a broader search or reset the current filters.')
                                            : t('templates.empty_description', 'Import or create a template before opening VM requests to regular users.')
                                    }
                                    visual={<TemplateCatalogGlyph className="action-empty-state__art action-empty-state__art--compact" />}
                                />
                            ),
                        }}
                    />
                </div>
            </PageSurface>

            {/* ── Create Modal (master-flow Step 3) ── */}
            <Modal
                title={t('common:button.add')}
                open={templates.createOpen}
                onOk={() => { void templates.submitCreate(); }}
                onCancel={templates.closeCreateModal}
                confirmLoading={templates.createPending}
                forceRender={true}
                width={640}
                data-testid="template-create-modal"
            >
                <Form form={templates.createForm} layout="vertical" preserve={false}>
                    <TemplatePresetPicker
                        t={t}
                        form={templates.createForm}
                        enableExperimentalSources={templates.enableCreateExperimentalSources}
                    />
                    <Form.Item name="name" label={t('common:table.name')} rules={[{ required: true }]}>
                        <Input placeholder="centos7-standard" />
                    </Form.Item>
                    <Form.Item name="display_name" label={t('common:table.display_name')}>
                        <Input />
                    </Form.Item>
                    <Form.Item name="os_family" label={t('templates.os_family')}>
                        <Select
                            placeholder={t('templates.os_family_placeholder')}
                            options={TEMPLATE_OS_FAMILY_OPTIONS.map((option) => ({
                                label: t(option.labelKey),
                                value: option.value,
                            }))}
                        />
                    </Form.Item>
                    <Form.Item noStyle shouldUpdate={(prev, cur) => prev.os_family !== cur.os_family}>
                        {({ getFieldValue }) => (
                            <Form.Item
                                name="os_version"
                                label={t('templates.os_version')}
                                extra={t('templates.os_version_help', 'Prefer a suggested version first. Switch to custom input only when you need a version outside the preset list.')}
                            >
                                <SuggestedValueInput
                                    options={getTemplateOSVersionSuggestions(getFieldValue('os_family'))}
                                    selectPlaceholder={t('templates.os_version_placeholder')}
                                    inputPlaceholder={t('templates.os_version_placeholder')}
                                    customToggleLabel={t('templates.use_custom_value', 'Custom')}
                                    suggestedToggleLabel={t('templates.use_suggested_value', 'Use suggested value')}
                                    emptyHint={t(
                                        'templates.select_or_customize_hint',
                                        'Prefer a suggested value. Switch to custom input only when the preset list does not cover your case.'
                                    )}
                                    noSuggestionsHint={t(
                                        'templates.no_suggestions_hint',
                                        'No suggested values are available for this field yet. Enter a custom value directly.'
                                    )}
                                />
                            </Form.Item>
                        )}
                    </Form.Item>
                    <Form.Item name="description" label={t('common:table.description')}>
                        <Input.TextArea rows={3} />
                    </Form.Item>
                    <Form.Item
                        name="catalog_scope"
                        label={t('templates.catalog_scope')}
                        initialValue="unclassified"
                        dependencies={['source_type']}
                        extra={t('templates.catalog_scope_help')}
                        rules={[
                            { required: true, message: t('templates.catalog_scope_required') },
                            ({ getFieldValue }) => ({
                                validator: async (_, value: string | undefined) => {
                                    if (getFieldValue('source_type') === 'containerdisk' && (value === 'prod' || value === 'all')) {
                                        throw new Error(t('templates.containerdisk_scope_invalid'));
                                    }
                                },
                            }),
                        ]}
                    >
                        <Select options={catalogScopeOptions} />
                    </Form.Item>

                    {/* Boot Source — three-mode taxonomy from the current backend canonical model */}
                    <Divider orientation="left" plain>{t('templates.image_source')}</Divider>
                    {!templates.createExperimentalSourcesEnabled ? (
                        <ExperimentalSourceGate
                            title={t('templates.experimental_source_title')}
                            description={t('templates.experimental_source_description')}
                            buttonLabel={t('templates.experimental_source_enable')}
                            onEnable={templates.enableCreateExperimentalSources}
                        />
                    ) : null}
                    <Form.Item name="source_type" label={t('templates.source_type')} initialValue="cdi_image_import">
                        <Radio.Group>
                            <Radio value="cdi_image_import">{t('templates.source_cdi_import')}</Radio>
                            <Radio value="cdi_pvc_clone">{t('templates.source_cdi_clone')}</Radio>
                            {templates.createExperimentalSourcesEnabled ? (
                                <Radio value="containerdisk">{t('templates.source_containerdisk')}</Radio>
                            ) : null}
                        </Radio.Group>
                    </Form.Item>
                    <Form.Item
                        noStyle
                        shouldUpdate={(prev, cur) =>
                            prev.source_type !== cur.source_type || prev.catalog_scope !== cur.catalog_scope
                        }
                    >
                        {({ getFieldValue }) => {
                            const sourceType = getFieldValue('source_type');
                            const scope = getFieldValue('catalog_scope');
                            if (sourceType !== 'containerdisk') {
                                return null;
                            }
                            const invalidScope = scope === 'prod' || scope === 'all';
                            return (
                                <Alert
                                    type={invalidScope ? 'error' : 'warning'}
                                    showIcon
                                    style={{ marginBottom: 16 }}
                                    message={invalidScope ? t('templates.containerdisk_scope_invalid') : t('templates.containerdisk_scope_warning')}
                                    description={t('templates.request_flow_reason_admin_only')}
                                />
                            );
                        }}
                    </Form.Item>
                    <Form.Item noStyle shouldUpdate={(prev, cur) => prev.source_type !== cur.source_type}>
                        {({ getFieldValue }) =>
                            getFieldValue('source_type') === 'cdi_pvc_clone' ? (
                                <>
                                    <Form.Item
                                        name="pvc_namespace"
                                        label={t('templates.pvc_namespace')}
                                        rules={[{ required: true, message: t('templates.pvc_namespace_required') }]}
                                        extra={t('templates.pvc_namespace_help')}
                                    >
                                        <SuggestedValueInput
                                            options={getTemplatePVCNamespaceSuggestions()}
                                            selectPlaceholder="vm-muban"
                                            inputPlaceholder="vm-muban"
                                            customToggleLabel={t('templates.use_custom_value', 'Custom')}
                                            suggestedToggleLabel={t('templates.use_suggested_value', 'Use suggested value')}
                                            emptyHint={t(
                                                'templates.select_or_customize_hint',
                                                'Prefer a suggested value. Switch to custom input only when the preset list does not cover your case.'
                                            )}
                                            noSuggestionsHint={t(
                                                'templates.no_suggestions_hint',
                                                'No suggested values are available for this field yet. Enter a custom value directly.'
                                            )}
                                        />
                                    </Form.Item>
                                    <Form.Item noStyle shouldUpdate={(prev, cur) => prev.os_family !== cur.os_family}>
                                        {({ getFieldValue }) => (
                                            <Form.Item
                                                name="pvc_name"
                                                label={t('templates.pvc_name')}
                                                rules={[{ required: true, message: t('templates.pvc_name_required') }]}
                                                extra={t('templates.pvc_name_help')}
                                            >
                                                <SuggestedValueInput
                                                    options={getTemplatePVCNameSuggestions(getFieldValue('os_family'))}
                                                    selectPlaceholder="openeuler2203-image"
                                                    inputPlaceholder="openeuler2203-image"
                                                    customToggleLabel={t('templates.use_custom_value', 'Custom')}
                                                    suggestedToggleLabel={t('templates.use_suggested_value', 'Use suggested value')}
                                                    emptyHint={t(
                                                        'templates.select_or_customize_hint',
                                                        'Prefer a suggested value. Switch to custom input only when the preset list does not cover your case.'
                                                    )}
                                                    noSuggestionsHint={t(
                                                        'templates.no_suggestions_hint',
                                                        'No suggested values are available for this field yet. Enter a custom value directly.'
                                                    )}
                                                />
                                            </Form.Item>
                                        )}
                                    </Form.Item>
                                </>
                            ) : (
                                <Form.Item name="image_url" label={t('templates.image_url')} rules={[{ required: true, message: t('templates.image_url_required') }]}>
                                    <SuggestedValueInput
                                        options={getTemplateImageURLSuggestions(getFieldValue('os_family'))}
                                        selectPlaceholder="docker://quay.io/containerdisks/ubuntu:22.04"
                                        inputPlaceholder="docker://quay.io/containerdisks/ubuntu:22.04"
                                        customToggleLabel={t('templates.use_custom_value', 'Custom')}
                                        suggestedToggleLabel={t('templates.use_suggested_value', 'Use suggested value')}
                                        emptyHint={t(
                                            'templates.select_or_customize_hint',
                                            'Prefer a suggested value. Switch to custom input only when the preset list does not cover your case.'
                                        )}
                                        noSuggestionsHint={t(
                                            'templates.no_suggestions_hint',
                                            'No suggested values are available for this field yet. Enter a custom value directly.'
                                        )}
                                    />
                                </Form.Item>
                            )
                        }
                    </Form.Item>

                    {/* cloud-init config — master-flow Step 3: YAML text, NOT JSON spec */}
                    <Divider orientation="left" plain>{t('templates.cloud_init')}</Divider>
                    <Space wrap style={{ marginBottom: 8 }}>
                        <Text type="secondary">
                            {t('templates.cloud_init_example_hint', 'Insert an example first, then edit it to fit the target system.')}
                        </Text>
                        <Button
                            size="small"
                            onClick={() => templates.createForm.setFieldValue('cloud_init', getTemplateCloudInitExample('linux'))}
                        >
                            {t('templates.cloud_init_example_linux', 'Linux example')}
                        </Button>
                        <Button
                            size="small"
                            onClick={() => templates.createForm.setFieldValue('cloud_init', getTemplateCloudInitExample('windows'))}
                        >
                            {t('templates.cloud_init_example_windows', 'Windows example')}
                        </Button>
                    </Space>
                    <Form.Item
                        name="cloud_init"
                        extra={t('templates.cloud_init_help', 'YAML cloud-init configuration. Provides one-time password and initial OS setup.')}
                    >
                        <Input.TextArea
                            rows={8}
                            style={{ fontFamily: 'monospace', fontSize: 13 }}
                            placeholder={'#cloud-config\nusers:\n  - name: admin\n    sudo: ALL=(ALL) NOPASSWD:ALL\nchpasswd:\n  expire: true\n  users:\n    - name: admin\n      password: changeme123'}
                        />
                    </Form.Item>

                    <Form.Item name="enabled" label={t('templates.enabled')} valuePropName="checked" initialValue={true}>
                        <Switch />
                    </Form.Item>
                </Form>
            </Modal>

            {/* ── Edit Modal (master-flow Step 3) ── */}
            <Modal
                title={t('common:button.edit')}
                open={templates.editOpen}
                onOk={() => { void templates.submitEdit(); }}
                onCancel={templates.closeEditModal}
                confirmLoading={templates.updatePending}
                forceRender={true}
                width={640}
                data-testid="template-edit-modal"
            >
                <Form form={templates.editForm} layout="vertical" preserve={false}>
                    <TemplatePresetPicker
                        t={t}
                        form={templates.editForm}
                        enableExperimentalSources={templates.enableEditExperimentalSources}
                    />
                    <Form.Item name="display_name" label={t('common:table.display_name')}>
                        <Input />
                    </Form.Item>
                    <Form.Item name="os_family" label={t('templates.os_family')}>
                        <Select
                            placeholder={t('templates.os_family_placeholder')}
                            options={TEMPLATE_OS_FAMILY_OPTIONS.map((option) => ({
                                label: t(option.labelKey),
                                value: option.value,
                            }))}
                        />
                    </Form.Item>
                    <Form.Item noStyle shouldUpdate={(prev, cur) => prev.os_family !== cur.os_family}>
                        {({ getFieldValue }) => (
                            <Form.Item
                                name="os_version"
                                label={t('templates.os_version')}
                                extra={t('templates.os_version_help', 'Prefer a suggested version first. Switch to custom input only when you need a version outside the preset list.')}
                            >
                                <SuggestedValueInput
                                    options={getTemplateOSVersionSuggestions(getFieldValue('os_family'))}
                                    selectPlaceholder={t('templates.os_version_placeholder')}
                                    inputPlaceholder={t('templates.os_version_placeholder')}
                                    customToggleLabel={t('templates.use_custom_value', 'Custom')}
                                    suggestedToggleLabel={t('templates.use_suggested_value', 'Use suggested value')}
                                    emptyHint={t(
                                        'templates.select_or_customize_hint',
                                        'Prefer a suggested value. Switch to custom input only when the preset list does not cover your case.'
                                    )}
                                    noSuggestionsHint={t(
                                        'templates.no_suggestions_hint',
                                        'No suggested values are available for this field yet. Enter a custom value directly.'
                                    )}
                                />
                            </Form.Item>
                        )}
                    </Form.Item>
                    <Form.Item name="description" label={t('common:table.description')}>
                        <Input.TextArea rows={3} />
                    </Form.Item>
                    <Form.Item
                        name="catalog_scope"
                        label={t('templates.catalog_scope')}
                        dependencies={['source_type']}
                        extra={t('templates.catalog_scope_help')}
                        rules={[
                            { required: true, message: t('templates.catalog_scope_required') },
                            ({ getFieldValue }) => ({
                                validator: async (_, value: string | undefined) => {
                                    if (getFieldValue('source_type') === 'containerdisk' && (value === 'prod' || value === 'all')) {
                                        throw new Error(t('templates.containerdisk_scope_invalid'));
                                    }
                                },
                            }),
                        ]}
                    >
                        <Select options={catalogScopeOptions} />
                    </Form.Item>

                    {/* Image Source toggle */}
                    <Divider orientation="left" plain>{t('templates.image_source')}</Divider>
                    {!templates.editExperimentalSourcesEnabled ? (
                        <ExperimentalSourceGate
                            title={t('templates.experimental_source_title')}
                            description={t('templates.experimental_source_description')}
                            buttonLabel={t('templates.experimental_source_enable')}
                            onEnable={templates.enableEditExperimentalSources}
                        />
                    ) : null}
                    <Form.Item name="source_type" label={t('templates.source_type')}>
                        <Radio.Group>
                            <Radio value="cdi_image_import">{t('templates.source_cdi_import')}</Radio>
                            <Radio value="cdi_pvc_clone">{t('templates.source_cdi_clone')}</Radio>
                            {templates.editExperimentalSourcesEnabled ? (
                                <Radio value="containerdisk">{t('templates.source_containerdisk')}</Radio>
                            ) : null}
                        </Radio.Group>
                    </Form.Item>
                    <Form.Item
                        noStyle
                        shouldUpdate={(prev, cur) =>
                            prev.source_type !== cur.source_type || prev.catalog_scope !== cur.catalog_scope
                        }
                    >
                        {({ getFieldValue }) => {
                            const sourceType = getFieldValue('source_type');
                            const scope = getFieldValue('catalog_scope');
                            if (sourceType !== 'containerdisk') {
                                return null;
                            }
                            const invalidScope = scope === 'prod' || scope === 'all';
                            return (
                                <Alert
                                    type={invalidScope ? 'error' : 'warning'}
                                    showIcon
                                    style={{ marginBottom: 16 }}
                                    message={invalidScope ? t('templates.containerdisk_scope_invalid') : t('templates.containerdisk_scope_warning')}
                                    description={t('templates.request_flow_reason_admin_only')}
                                />
                            );
                        }}
                    </Form.Item>
                    <Form.Item noStyle shouldUpdate={(prev, cur) => prev.source_type !== cur.source_type}>
                        {({ getFieldValue }) =>
                            getFieldValue('source_type') === 'cdi_pvc_clone' ? (
                                <>
                                    <Form.Item
                                        name="pvc_namespace"
                                        label={t('templates.pvc_namespace')}
                                        rules={[{ required: true, message: t('templates.pvc_namespace_required') }]}
                                        extra={t('templates.pvc_namespace_help')}
                                    >
                                        <SuggestedValueInput
                                            options={getTemplatePVCNamespaceSuggestions()}
                                            selectPlaceholder="vm-muban"
                                            inputPlaceholder="vm-muban"
                                            customToggleLabel={t('templates.use_custom_value', 'Custom')}
                                            suggestedToggleLabel={t('templates.use_suggested_value', 'Use suggested value')}
                                            emptyHint={t(
                                                'templates.select_or_customize_hint',
                                                'Prefer a suggested value. Switch to custom input only when the preset list does not cover your case.'
                                            )}
                                            noSuggestionsHint={t(
                                                'templates.no_suggestions_hint',
                                                'No suggested values are available for this field yet. Enter a custom value directly.'
                                            )}
                                        />
                                    </Form.Item>
                                    <Form.Item noStyle shouldUpdate={(prev, cur) => prev.os_family !== cur.os_family}>
                                        {({ getFieldValue }) => (
                                            <Form.Item
                                                name="pvc_name"
                                                label={t('templates.pvc_name')}
                                                rules={[{ required: true, message: t('templates.pvc_name_required') }]}
                                                extra={t('templates.pvc_name_help')}
                                            >
                                                <SuggestedValueInput
                                                    options={getTemplatePVCNameSuggestions(getFieldValue('os_family'))}
                                                    selectPlaceholder="win2022-image"
                                                    inputPlaceholder="win2022-image"
                                                    customToggleLabel={t('templates.use_custom_value', 'Custom')}
                                                    suggestedToggleLabel={t('templates.use_suggested_value', 'Use suggested value')}
                                                    emptyHint={t(
                                                        'templates.select_or_customize_hint',
                                                        'Prefer a suggested value. Switch to custom input only when the preset list does not cover your case.'
                                                    )}
                                                    noSuggestionsHint={t(
                                                        'templates.no_suggestions_hint',
                                                        'No suggested values are available for this field yet. Enter a custom value directly.'
                                                    )}
                                                />
                                            </Form.Item>
                                        )}
                                    </Form.Item>
                                </>
                            ) : (
                                <Form.Item
                                    name="image_url"
                                    label={t('templates.image_url')}
                                    rules={[{ required: true, message: t('templates.image_url_required') }]}
                                >
                                    <SuggestedValueInput
                                        options={getTemplateImageURLSuggestions(getFieldValue('os_family'))}
                                        selectPlaceholder="docker://quay.io/containerdisks/ubuntu:22.04"
                                        inputPlaceholder="docker://quay.io/containerdisks/ubuntu:22.04"
                                        customToggleLabel={t('templates.use_custom_value', 'Custom')}
                                        suggestedToggleLabel={t('templates.use_suggested_value', 'Use suggested value')}
                                        emptyHint={t(
                                            'templates.select_or_customize_hint',
                                            'Prefer a suggested value. Switch to custom input only when the preset list does not cover your case.'
                                        )}
                                        noSuggestionsHint={t(
                                            'templates.no_suggestions_hint',
                                            'No suggested values are available for this field yet. Enter a custom value directly.'
                                        )}
                                    />
                                </Form.Item>
                            )
                        }
                    </Form.Item>

                    {/* cloud-init YAML editor */}
                    <Divider orientation="left" plain>{t('templates.cloud_init')}</Divider>
                    <Space wrap style={{ marginBottom: 8 }}>
                        <Text type="secondary">
                            {t('templates.cloud_init_example_hint', 'Insert an example first, then edit it to fit the target system.')}
                        </Text>
                        <Button
                            size="small"
                            onClick={() => templates.editForm.setFieldValue('cloud_init', getTemplateCloudInitExample('linux'))}
                        >
                            {t('templates.cloud_init_example_linux', 'Linux example')}
                        </Button>
                        <Button
                            size="small"
                            onClick={() => templates.editForm.setFieldValue('cloud_init', getTemplateCloudInitExample('windows'))}
                        >
                            {t('templates.cloud_init_example_windows', 'Windows example')}
                        </Button>
                    </Space>
                    <Form.Item
                        name="cloud_init"
                        extra={t('templates.cloud_init_help', 'YAML cloud-init configuration. Provides one-time password and initial OS setup.')}
                    >
                        <Input.TextArea
                            rows={8}
                            style={{ fontFamily: 'monospace', fontSize: 13 }}
                            placeholder={'#cloud-config\nusers:\n  - name: admin'}
                        />
                    </Form.Item>

                    <Form.Item name="enabled" label={t('templates.enabled')} valuePropName="checked">
                        <Switch />
                    </Form.Item>
                </Form>
            </Modal>

            <Modal
                title={t('common:button.delete')}
                open={templates.deleteOpen}
                onOk={templates.submitDelete}
                onCancel={templates.closeDeleteModal}
                confirmLoading={templates.deletePending}
                okButtonProps={{ danger: true }}
            >
                <Text>
                    {t('common:message.delete_confirm', {
                        name: templates.deletingTemplate?.display_name ?? templates.deletingTemplate?.name ?? '-',
                    })}
                </Text>
            </Modal>
        </div>
    );
}
