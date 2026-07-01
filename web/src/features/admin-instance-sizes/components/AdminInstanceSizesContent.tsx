'use client';

import { useEffect, useMemo, useRef, useState, type RefObject } from 'react';
import {
    Alert,
    Button,
    Card,
    Checkbox,
    Divider,
    Form,
    Input,
    InputNumber,
    Modal,
    Select,
    Space,
    Spin,
    Switch,
    Table,
    Tag,
    Tooltip,
    Typography,
    type FormInstance,
} from 'antd';
import type { InputRef } from 'antd';
import type { ColumnsType, FilterDropdownProps } from 'antd/es/table/interface';
import {
    DeleteOutlined,
    EditOutlined,
    HddOutlined,
    QuestionCircleOutlined,
    ReloadOutlined,
    SearchOutlined,
} from '@ant-design/icons';
import { useRouter } from 'next/navigation';
import { useTranslation } from 'react-i18next';

import { ActionEmptyState } from '@/components/feedback/ActionEmptyState';
import { SummaryMetricCard } from '@/components/feedback/SummaryMetricCard';
import {
    InstanceSizeBlueprintGlyph,
    QueueReviewGlyph,
    RequestsOverviewGlyph,
    ServiceWorkspaceGlyph,
} from '@/components/illustrations/DashboardIllustrations';
import { PageHeader, PageSurface } from '@/components/layouts/PageSection';
import { PageSearchToolbar } from '@/components/ui/PageSearchToolbar';
import { UnitInputNumber } from '@/components/form/UnitInputNumber';
import {
    buildDashboardSetupResumeHref,
    resolveNextSetupAction,
} from '@/features/setup-guide/flow';
import {
    SYSTEM_LABEL_OPTIONS,
    normalizeSizeSystemLabelSelection,
    normalizeSystemLabels,
    systemLabelColor,
    systemLabelText,
} from '@/features/catalog/systemLabels';
import { translateApiError } from '@/lib/api/errorMessage';
import {
    HUGEPAGES_PAGE_SIZE_PATH,
    normalizeHugepagesPageSizeValue,
} from '@/lib/hugepages';
import { useAutoOpenIntent } from '@/features/setup-guide/hooks/useAutoOpenIntent';
import { useSetupGuide } from '@/features/setup-guide/hooks/useSetupGuide';
import { useAdminInstanceSizesController } from '../hooks/useAdminInstanceSizesController';
import {
    buildInstanceSizePresetValues,
    getInstanceSizePresetGroups,
    type InstanceSizePresetKey,
} from '../instanceSizePresets';
import { buildResolvedInstanceSizePreview } from '../resolvedPreview';
import {
    getSpecOverrideValue,
    INDEXED_SPEC_OVERRIDE_PATHS,
    normalizeInstanceSizeSpecOverrides,
} from '../specOverrides';
import {
    formatCores,
    formatMemory,
    getGPUDeviceLabels,
    hasDedicatedCPURequirement,
    hasCPUOvercommit,
    hasMemoryOvercommit,
    type InstanceSize,
} from '../types';
import {
    DynamicSchemaForm,
    type DynamicSchemaFormHandle,
    type SchemaNode,
    type SchemaMask,
} from '../../admin-templates/components/DynamicSchemaForm';
import { useDynamicSchema } from '../../admin-templates/hooks/useDynamicSchema';

const { Text } = Typography;

// DynamicSchemaForm hides these spec paths because they are owned by the
// InstanceSize indexed columns (cpu_cores / memory_gi / dedicated_cpu / ...).
// The same list is used at every data boundary by stripIndexedSpecOverridePaths
// to keep spec_text clean — see specOverrides.ts for the full rationale.
const INSTANCE_SIZE_RECOGNIZED_EXCLUDED_PATHS: string[] = [...INDEXED_SPEC_OVERRIDE_PATHS];

const ROOT_VOLUME_ACCESS_MODE_OPTIONS = [
    'ReadWriteOnce',
    'ReadOnlyMany',
    'ReadWriteMany',
    'ReadWriteOncePod',
];

function catalogScopeLabel(scope: InstanceSize['catalog_scope'], t: ReturnType<typeof useTranslation>['t']): string {
    switch (scope) {
        case 'test':
            return t('templates.scope_test');
        case 'prod':
            return t('templates.scope_prod');
        case 'all':
            return t('templates.scope_all');
        default:
            return t('templates.scope_unclassified');
    }
}

function catalogScopeColor(scope: InstanceSize['catalog_scope']): string {
    switch (scope) {
        case 'test':
            return 'blue';
        case 'prod':
            return 'red';
        case 'all':
            return 'green';
        default:
            return 'default';
    }
}

function getInstanceSizePublicationMeta(
    record: Pick<InstanceSize, 'enabled' | 'catalog_scope'>,
    t: ReturnType<typeof useTranslation>['t'],
): { statusLabel: string; statusColor: string; description: string } {
    if (record.enabled === false) {
        return {
            statusLabel: t('common:status.disabled'),
            statusColor: 'default',
            description: t(
                'instanceSizes.catalog_status_disabled_description',
                'Enable this profile before exposing it in the VM request catalog.',
            ),
        };
    }
    if ((record.catalog_scope ?? 'unclassified') === 'unclassified') {
        return {
            statusLabel: t('instanceSizes.catalog_status_hidden', 'Hidden from requests'),
            statusColor: 'gold',
            description: t(
                'instanceSizes.catalog_status_hidden_description',
                'Move the catalog scope to test, prod, or all before regular users can pick it.',
            ),
        };
    }
    return {
        statusLabel: t('instanceSizes.catalog_status_ready', 'Visible in requests'),
        statusColor: 'green',
        description: t(
            'instanceSizes.catalog_status_ready_description',
            'This profile is already visible in the VM request flow.',
        ),
    };
}

function getInstanceSizeCapabilityTags(
    record: InstanceSize,
    t: ReturnType<typeof useTranslation>['t'],
): { label: string; color: string }[] {
    const tags: { label: string; color: string }[] = [];
    const gpuDevices = getGPUDeviceLabels(record);
    if (gpuDevices.length > 0) {
        tags.push(...gpuDevices.map((device) => ({
            label: t('instanceSizes.capability_gpu_device', {
                defaultValue: `GPU ${device}`,
                device,
            }),
            color: 'volcano',
        })));
    } else if (record.requires_gpu) {
        tags.push({ label: t('instanceSizes.capability_gpu', 'GPU'), color: 'volcano' });
    }
    if (record.requires_sriov) {
        tags.push({ label: t('instanceSizes.capability_sriov', 'SR-IOV'), color: 'purple' });
    }
    if (record.requires_hugepages) {
        tags.push({
            label: record.hugepages_size
                ? t('instanceSizes.capability_hugepages_size', {
                    defaultValue: `Hugepages ${record.hugepages_size}`,
                    size: record.hugepages_size,
                })
                : t('instanceSizes.capability_hugepages', 'Hugepages'),
            color: 'geekblue',
        });
    }
    if (hasDedicatedCPURequirement(record)) {
        tags.push({ label: t('instanceSizes.capability_dedicated_cpu', 'Dedicated CPU'), color: 'orange' });
    }
    if (hasCPUOvercommit(record)) {
        tags.push({
            label: t('instanceSizes.capability_cpu_overcommit', {
                defaultValue: 'CPU request {{value}}',
                value: `${formatCores(record.cpu_request!)} ${t('instanceSizes.cores')}`,
            }),
            color: 'cyan',
        });
    }
    if (hasMemoryOvercommit(record)) {
        tags.push({
            label: t('instanceSizes.capability_memory_overcommit', {
                defaultValue: 'Memory request {{value}}',
                value: formatMemory(record.memory_request_gi!),
            }),
            color: 'blue',
        });
    }
    return tags;
}

function renderSystemLabelTags(labels: string[] | undefined, t: ReturnType<typeof useTranslation>['t']) {
    return normalizeSystemLabels(labels).map((label) => (
        <Tag key={label} color={systemLabelColor(label)}>
            {systemLabelText(label, t)}
        </Tag>
    ));
}

function handleInstanceSizeFormValuesChange(
    form: FormInstance,
    formRef: RefObject<DynamicSchemaFormHandle | null>,
    changedValues: Record<string, unknown>,
) {
    const updates: Record<string, unknown> = {};

    if (changedValues.dedicated_cpu === true) {
        updates.cpu_overcommit_enabled = false;
        updates.cpu_request = undefined;
    }
    if (changedValues.cpu_overcommit_enabled === true) {
        updates.dedicated_cpu = false;
    }
    if (changedValues.cpu_overcommit_enabled === false) {
        updates.cpu_request = undefined;
    }
    const hugepagesSize = resolveHugepagesSizeFromSpecText(
        changedValues.spec_text ?? form.getFieldValue('spec_text'),
    );
    if (hugepagesSize) {
        updates.memory_overcommit_enabled = false;
        updates.memory_request_gi = undefined;
    }
    if (changedValues.memory_overcommit_enabled === false) {
        updates.memory_request_gi = undefined;
    }
    if (changedValues.root_volume_mode_intent === 'auto') {
        updates.dv_access_modes = undefined;
        updates.dv_volume_mode = undefined;
    }

    if (Object.keys(updates).length > 0) {
        form.setFieldsValue(updates);
    }
    formRef.current?.sync();
}

function scheduleInterlockResume(flagRef: RefObject<boolean>) {
    const resume = () => {
        flagRef.current = false;
    };
    if (typeof queueMicrotask === 'function') {
        queueMicrotask(resume);
        return;
    }
    setTimeout(resume, 0);
}

function hydrateFormWithoutInterlocks(
    form: FormInstance,
    values: Record<string, unknown>,
    suspendRef: RefObject<boolean>,
) {
    suspendRef.current = true;
    form.resetFields();
    const {
        root_volume_mode_intent: rootVolumeModeIntent,
        dv_access_modes: dvAccessModes,
        dv_volume_mode: dvVolumeMode,
        ...rest
    } = values;

    form.setFieldsValue({
        ...rest,
        root_volume_mode_intent: rootVolumeModeIntent,
    });

    if (rootVolumeModeIntent === 'explicit') {
        setTimeout(() => {
            form.setFieldsValue({
                dv_access_modes: dvAccessModes,
                dv_volume_mode: dvVolumeMode,
            });
            scheduleInterlockResume(suspendRef);
        }, 0);
        return;
    }
    scheduleInterlockResume(suspendRef);
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

function renderInlineHelpLabel(label: string, helpText?: string): React.ReactNode {
    if (!helpText) {
        return label;
    }
    return (
        <Space size={6}>
            <span>{label}</span>
            <Tooltip title={helpText} trigger={['hover', 'click']}>
                <QuestionCircleOutlined style={{ color: 'rgba(0,0,0,0.45)' }} />
            </Tooltip>
        </Space>
    );
}

function resolveHugepagesSizeFromSpecText(specText: unknown): string | undefined {
    if (typeof specText !== 'string' || !specText.trim()) {
        return undefined;
    }
    try {
        const parsed = JSON.parse(specText) as unknown;
        if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
            return undefined;
        }
        const normalized = normalizeInstanceSizeSpecOverrides(parsed as Record<string, unknown>);
        return normalizeHugepagesPageSizeValue(
            getSpecOverrideValue(normalized, HUGEPAGES_PAGE_SIZE_PATH),
        );
    } catch {
        return undefined;
    }
}

/**
 * Schema-driven spec_overrides section for InstanceSize modals.
 *
 * Replaces hardcoded Hugepages Select + GPU Form.List + JSON textarea.
 * Loads schema+mask from GET /schemas/instancesize (ADR-0023 Stage 1).
 *
 * Degradation chain (ADR-0023):
 *   success → DynamicSchemaForm renders mask fields
 *   isError → collapsed warning, form still submittable without dynamic fields
 *   isPending → loading spinner
 */
function InstanceSizeSpecSection({
    formRef,
    disabled,
}: {
    formRef: React.RefObject<DynamicSchemaFormHandle | null>;
    disabled?: boolean;
}) {
    const { t } = useTranslation(['admin', 'common']);
    const { data, isError, isPending } = useDynamicSchema('instancesize');

    if (isPending) {
        return (
            <Card size="small" style={{ textAlign: 'center', padding: 24 }}>
                <Spin size="small" />
                <Text type="secondary" style={{ marginLeft: 8 }}>
                    {t('instanceSizes.schema_loading', 'Loading spec schema…')}
                </Text>
            </Card>
        );
    }

    if (isError || !data) {
        return (
            <Alert
                type="warning"
                showIcon
                message={t(
                    'instanceSizes.schema_unavailable',
                    'Spec schema unavailable — advanced KubeVirt settings cannot be configured right now.'
                )}
                style={{ marginBottom: 8 }}
            />
        );
    }

    return (
        <Form.Item
            name="spec_text"
            // valuePropName="value" is injected by Form.Item automatically
            noStyle
        >
            <DynamicSchemaForm
                ref={formRef}
                schema={data.schema as SchemaNode}
                mask={data.mask as SchemaMask}
                disabled={disabled}
                recognizedExcludedPaths={INSTANCE_SIZE_RECOGNIZED_EXCLUDED_PATHS}
                schemaHelpScope="instanceSize"
            />
        </Form.Item>
    );
}

/**
 * Shared form fields for InstanceSize create/edit modals.
 *
 * Architecture:
 * - Metadata fields (cpu_cores, memory_gi, overcommit, etc.) remain as
 *   explicit Ant Design Form.Items — these are InstanceSize-specific API fields.
 * - Spec overrides (hugepages, GPU devices, CPU model, etc.) are rendered
 *   schema-driven via DynamicSchemaForm (ADR-0023 Stage 1).
 *
 * The `formRef` is passed down so the parent Form's onValuesChange can
 * call formRef.current?.sync() to keep spec_text in sync (antd best practice:
 * side effects in event handlers, not in render).
 */
function InstanceSizeFormFields({
    isCreate,
    formRef,
}: {
    isCreate: boolean;
    formRef: React.RefObject<DynamicSchemaFormHandle | null>;
}) {
    const { t } = useTranslation(['admin', 'common']);
    const form = Form.useFormInstance();
    const dedicatedCPU = Form.useWatch('dedicated_cpu', form);
    const cpuOvercommitEnabled = Form.useWatch('cpu_overcommit_enabled', form);
    const memoryOvercommitEnabled = Form.useWatch('memory_overcommit_enabled', form);
    const specText = Form.useWatch('spec_text', form);
    const rootVolumeModeIntent = Form.useWatch('root_volume_mode_intent', form);
    const hugepagesSize = resolveHugepagesSizeFromSpecText(specText);
    const memoryOvercommitDisabledByHugepages = Boolean(hugepagesSize);
    const showMemoryOvercommitFields = !!memoryOvercommitEnabled && !memoryOvercommitDisabledByHugepages;

    return (
        <>
            {/* ── Basic Info ── */}
            <Divider orientation="left" plain style={{ marginTop: 0 }}>
                {t('instanceSizes.section_basic')}
            </Divider>

            <Form.Item name="name" label={t('common:table.name')} rules={[{ required: true }]}>
                <Input placeholder="gpu-workstation" disabled={!isCreate} />
            </Form.Item>
            <Form.Item name="display_name" label={t('common:table.display_name')}>
                <Input placeholder="GPU Workstation (8 cores 32GB)" />
            </Form.Item>
            <Form.Item name="description" label={t('common:table.description')}>
                <Input.TextArea rows={2} />
            </Form.Item>
            <Form.Item
                name="catalog_scope"
                label={t('instanceSizes.catalog_scope')}
                extra={t('instanceSizes.catalog_scope_help')}
                rules={[
                    { required: true, message: t('instanceSizes.catalog_scope_required') },
                ]}
            >
                <Select
                    options={[
                        { label: t('templates.scope_unclassified'), value: 'unclassified' },
                        { label: t('templates.scope_test'), value: 'test' },
                        { label: t('templates.scope_prod'), value: 'prod' },
                        { label: t('templates.scope_all'), value: 'all' },
                    ]}
                />
            </Form.Item>
            <Form.Item
                name="sort_order"
                label={t('instanceSizes.sort_order')}
                tooltip={{ title: t('instanceSizes.sort_order_help'), trigger: ['hover', 'click'] }}
            >
                <InputNumber style={{ width: '100%' }} />
            </Form.Item>
            <Form.Item
                name="system_labels"
                label={t('systemLabels.instance_size_field')}
                extra={t('systemLabels.instance_size_help')}
            >
                <Select
                    mode="multiple"
                    allowClear
                    options={SYSTEM_LABEL_OPTIONS.map((option) => ({
                        label: systemLabelText(option.value, t),
                        value: option.value,
                    }))}
                    onChange={(value) => {
                        form.setFieldValue('system_labels', normalizeSizeSystemLabelSelection(value));
                    }}
                />
            </Form.Item>

            {/* ── Resource Configuration ── */}
            <Divider orientation="left" plain>
                {t('instanceSizes.section_resources')}
            </Divider>

            <Form.Item
                name="cpu_cores"
                label={t('instanceSizes.cpu')}
                tooltip={{ title: t('instanceSizes.cpu_help'), trigger: ['hover', 'click'] }}
                rules={[{ required: true }]}
            >
                <UnitInputNumber min={0.5} step={0.5} precision={1} unit={t('instanceSizes.cores')} />
            </Form.Item>

            <Form.Item name="dedicated_cpu" valuePropName="checked">
                <Checkbox disabled={!!cpuOvercommitEnabled}>
                    {renderInlineHelpLabel(
                        t('instanceSizes.dedicated'),
                        t('instanceSizes.dedicated_help')
                    )}
                </Checkbox>
            </Form.Item>

            <Form.Item name="cpu_overcommit_enabled" valuePropName="checked">
                <Checkbox disabled={!!dedicatedCPU}>
                    {renderInlineHelpLabel(
                        t('instanceSizes.enable_cpu_overcommit'),
                        t('instanceSizes.enable_cpu_overcommit_help')
                    )}
                </Checkbox>
            </Form.Item>
            {cpuOvercommitEnabled && !dedicatedCPU ? (
                <Card size="small" style={{ marginBottom: 16, background: '#fafafa' }}>
                    <Space style={{ width: '100%' }} direction="vertical">
                        <Form.Item
                            name="cpu_request"
                            label={t('instanceSizes.cpu_request')}
                            tooltip={{ title: t('instanceSizes.cpu_request_help'), trigger: ['hover', 'click'] }}
                            dependencies={['cpu_cores']}
                            rules={[
                                ({ getFieldValue }) => ({
                                    validator(_, value) {
                                        if (typeof value !== 'number') {
                                            return Promise.resolve();
                                        }
                                        const cpuCores = getFieldValue('cpu_cores');
                                        if (typeof cpuCores === 'number' && value > cpuCores) {
                                            return Promise.reject(
                                                new Error(
                                                    t(
                                                        'instanceSizes.cpu_request_exceeds_limit',
                                                        'CPU request cannot exceed CPU limit.',
                                                    ),
                                                ),
                                            );
                                        }
                                        return Promise.resolve();
                                    },
                                }),
                            ]}
                            style={{ margin: 0 }}
                        >
                            <UnitInputNumber min={0.5} step={0.5} precision={1} unit={t('instanceSizes.cores')} />
                        </Form.Item>
                        <Text type="secondary" style={{ fontSize: 13 }}>
                            {t('instanceSizes.overcommit_ratio_hint')}
                        </Text>
                    </Space>
                </Card>
            ) : null}

            <Form.Item
                name="memory_gi"
                label={t('instanceSizes.memory')}
                tooltip={{ title: t('instanceSizes.memory_help'), trigger: ['hover', 'click'] }}
                rules={[{ required: true }]}
            >
                <UnitInputNumber min={0.5} step={0.5} precision={1} unit="Gi" />
            </Form.Item>

            {/* Memory Overcommit: conditional reveal */}
            <Form.Item
                name="memory_overcommit_enabled"
                valuePropName="checked"
                extra={
                    memoryOvercommitDisabledByHugepages
                        ? t('instanceSizes.memory_overcommit_disabled_by_hugepages', {
                            size: hugepagesSize,
                            defaultValue: 'Memory Overcommit is unavailable when Hugepages is enabled because memory request must equal the memory limit.',
                        })
                        : undefined
                }
            >
                <Checkbox disabled={memoryOvercommitDisabledByHugepages}>
                    {renderInlineHelpLabel(
                        t('instanceSizes.enable_memory_overcommit'),
                        t('instanceSizes.enable_memory_overcommit_help')
                    )}
                </Checkbox>
            </Form.Item>
            {showMemoryOvercommitFields ? (
                <Card size="small" style={{ marginBottom: 16, background: '#fafafa' }}>
                    <Form.Item
                        name="memory_request_gi"
                        label={t('instanceSizes.memory_request')}
                        tooltip={{ title: t('instanceSizes.memory_request_help'), trigger: ['hover', 'click'] }}
                        dependencies={['memory_gi']}
                        rules={[
                            ({ getFieldValue }) => ({
                                validator(_, value) {
                                    if (typeof value !== 'number') {
                                        return Promise.resolve();
                                    }
                                    const memoryGi = getFieldValue('memory_gi');
                                    if (typeof memoryGi === 'number' && value > memoryGi) {
                                        return Promise.reject(
                                            new Error(
                                                t(
                                                    'instanceSizes.memory_request_exceeds_limit',
                                                    'Memory request cannot exceed memory limit.',
                                                ),
                                            ),
                                        );
                                    }
                                    return Promise.resolve();
                                },
                            }),
                        ]}
                        style={{ margin: 0 }}
                    >
                        <UnitInputNumber min={0.5} step={0.5} precision={1} unit="Gi" />
                    </Form.Item>
                </Card>
            ) : null}

            <Form.Item
                name="disk_gb"
                label={t('instanceSizes.disk')}
                tooltip={{ title: t('instanceSizes.disk_help'), trigger: ['hover', 'click'] }}
            >
                <UnitInputNumber min={1} step={1} precision={0} unit="GB" />
            </Form.Item>

            <Form.Item
                name="root_volume_mode_intent"
                label={t('instanceSizes.root_volume_mode')}
                tooltip={{ title: t('instanceSizes.root_volume_mode_help'), trigger: ['hover', 'click'] }}
            >
                <Select
                    options={[
                        { label: t('instanceSizes.root_volume_mode_auto'), value: 'auto' },
                        { label: t('instanceSizes.root_volume_mode_explicit'), value: 'explicit' },
                    ]}
                />
            </Form.Item>

            {rootVolumeModeIntent === 'explicit' ? (
                <Card size="small" style={{ marginBottom: 16, background: '#fafafa' }}>
                    <Space direction="vertical" size={12} style={{ width: '100%' }}>
                        <Text type="secondary">
                            {t(
                                'instanceSizes.root_volume_mode_explicit_help',
                                'Approval validates whether the target StorageClass supports this accessModes + volumeMode combination. If the cluster does not support it, the approval cannot pass.'
                            )}
                        </Text>
                        <Form.Item
                            name="dv_volume_mode"
                            label={t('instanceSizes.dv_volume_mode')}
                            tooltip={{ title: t('instanceSizes.dv_volume_mode_help'), trigger: ['hover', 'click'] }}
                            rules={[{ required: true, message: t('instanceSizes.dv_volume_mode_required') }]}
                            style={{ margin: 0 }}
                        >
                            <Select
                                options={[
                                    { label: 'Block', value: 'Block' },
                                    { label: 'Filesystem', value: 'Filesystem' },
                                ]}
                            />
                        </Form.Item>
                        <Form.Item
                            name="dv_access_modes"
                            label={t('instanceSizes.dv_access_modes')}
                            tooltip={{ title: t('instanceSizes.dv_access_modes_help'), trigger: ['hover', 'click'] }}
                            rules={[{ required: true, message: t('instanceSizes.dv_access_modes_required') }]}
                            style={{ margin: 0 }}
                        >
                            <Select
                                mode="multiple"
                                allowClear
                                options={ROOT_VOLUME_ACCESS_MODE_OPTIONS.map((value) => ({
                                    label: value,
                                    value,
                                }))}
                            />
                        </Form.Item>
                    </Space>
                </Card>
            ) : (
                <Alert
                    type="info"
                    showIcon
                    style={{ marginBottom: 16 }}
                    message={t('instanceSizes.root_volume_mode_auto')}
                    description={t(
                        'instanceSizes.root_volume_mode_auto_help',
                        'The spec keeps only the Auto intent. During approval, the target cluster and StorageProfile resolve the real root volume mode; if the result is not unique, approval must choose an explicit mode.'
                    )}
                />
            )}

            <Form.Item
                name="requires_sriov"
                label={t('instanceSizes.sriov')}
                tooltip={{ title: t('instanceSizes.sriov_help'), trigger: ['hover', 'click'] }}
                valuePropName="checked"
            >
                <Switch />
            </Form.Item>

            {/* ── Spec Overrides (Schema-driven, ADR-0023 Stage 1) ── */}
            <Divider orientation="left" plain>
                {t('instanceSizes.section_advanced')}
            </Divider>

            <Alert
                type="info"
                showIcon
                style={{ marginBottom: 16 }}
                message={t('instanceSizes.kubevirt_hotplug_defaults_title')}
                description={(
                    <Space direction="vertical" size={6} style={{ width: '100%' }}>
                        <Text type="secondary">
                            {t('instanceSizes.kubevirt_hotplug_defaults_description')}
                        </Text>
                        <Space wrap size={[8, 8]}>
                            <Tag color="default">cpu.maxSockets</Tag>
                            <Tag color="default">memory.maxGuest</Tag>
                        </Space>
                    </Space>
                )}
            />

            {/*
             * DynamicSchemaForm renders KubeVirt spec fields driven by the
             * mask from GET /schemas/instancesize.  This replaces the previous
             * hardcoded Hugepages Select + GPU Form.List + JSON textarea.
             *
             * Data flow:
             *   spec_text (JSON string in Form) ←→ DynamicSchemaForm
             *   onValuesChange → formRef.current?.sync() → spec_text updated
             */}
            <InstanceSizeSpecSection formRef={formRef} disabled={false} />

            <Form.Item noStyle shouldUpdate>
                {({ getFieldsValue }) => {
                    const resolvedPreview = buildResolvedInstanceSizePreview(
                        getFieldsValue(true) as Record<string, unknown>,
                    );

                    return (
                        <Card size="small" style={{ marginTop: 16, background: '#fafafa' }}>
                            <Space direction="vertical" size={8} style={{ width: '100%' }}>
                                <Text strong>{t('instanceSizes.section_resolved_preview')}</Text>
                                <Text type="secondary">
                                    {t(
                                        'instanceSizes.resolved_preview_help',
                                        'This preview merges the indexed fields above with the raw spec overrides below in real time. The _platform block is platform metadata, not raw KubeVirt spec.'
                                    )}
                                </Text>
                                <Input.TextArea
                                    readOnly
                                    value={resolvedPreview}
                                    autoSize={{ minRows: 10, maxRows: 24 }}
                                    data-testid="instance-size-resolved-preview"
                                    style={{ fontFamily: 'monospace', fontSize: 14 }}
                                />
                            </Space>
                        </Card>
                    );
                }}
            </Form.Item>

            <Form.Item
                name="enabled"
                label={t('instanceSizes.enabled')}
                tooltip={{ title: t('instanceSizes.enabled_help'), trigger: ['hover', 'click'] }}
                valuePropName="checked"
                style={{ marginTop: 16 }}
            >
                <Switch />
            </Form.Item>
        </>
    );
}

export function applyInstanceSizePreset(
    form: FormInstance,
    _formRef: RefObject<DynamicSchemaFormHandle | null>,
    presetKey: InstanceSizePresetKey,
) {
    const preserved = form.getFieldsValue([
        'name',
        'display_name',
        'description',
        'sort_order',
    ]) as Record<string, unknown>;
    const presetValues = buildInstanceSizePresetValues(presetKey);
    const basePresetValues = {
        catalog_scope: 'unclassified',
        cpu_cores: undefined,
        memory_gi: undefined,
        disk_gb: undefined,
        cpu_request: undefined,
        memory_request_gi: undefined,
        cpu_overcommit_enabled: false,
        memory_overcommit_enabled: false,
        dedicated_cpu: false,
        root_volume_mode_intent: 'auto',
        dv_access_modes: undefined,
        dv_volume_mode: undefined,
        system_labels: ['os:any'],
        requires_sriov: false,
        enabled: true,
        spec_text: '{}',
    } satisfies Record<string, unknown>;

    form.setFieldsValue(Object.assign({}, basePresetValues, presetValues, preserved));
}

function InstanceSizePresetPicker({
    t,
    form,
    formRef,
}: {
    t: ReturnType<typeof useTranslation>['t'];
    form: FormInstance;
    formRef: RefObject<DynamicSchemaFormHandle | null>;
}) {
    return (
        <Alert
            type="info"
            showIcon
            style={{ marginBottom: 16 }}
            message={t('instanceSizes.preset_title', 'Preset Sizes')}
            description={(
                <Space direction="vertical" size={12} style={{ width: '100%' }}>
                    <Text type="secondary">
                        {t(
                            'instanceSizes.preset_description',
                            'These presets group the parameter combinations that commonly appear together in KubeVirt specs. Start from the closest size, then tune only the differences in the advanced sections.'
                        )}
                    </Text>
                    {getInstanceSizePresetGroups().map((group) => (
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
                                                onClick={() => applyInstanceSizePreset(
                                                    form,
                                                    formRef,
                                                    preset.key as InstanceSizePresetKey,
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

export function AdminInstanceSizesContent() {
    const { t } = useTranslation(['admin', 'common']);
    const router = useRouter();
    const setupGuide = useSetupGuide();
    const canManageInstanceSizes = setupGuide.canManageInstanceSizes;
    const sizes = useAdminInstanceSizesController({
        t,
        onCreateSuccess: (_instanceSize, context) => {
            if (!context.isFirstInstanceSize) {
                return false;
            }
            const nextAction = resolveNextSetupAction(setupGuide, 'instance-size');
            if (!nextAction) {
                return false;
            }
            router.push(buildDashboardSetupResumeHref(nextAction));
            return true;
        },
    });
    const searchInputRef = useRef<InputRef>(null);
    const [quickSearchDraft, setQuickSearchDraft] = useState(() => sizes.filters.search);
    const [filtersOpen, setFiltersOpen] = useState(() => sizes.hasActiveFilters);
    const [catalogScopeDraft, setCatalogScopeDraft] = useState(() => sizes.filters.catalogScope);
    const [enabledDraft, setEnabledDraft] = useState(() => sizes.filters.enabled);
    const [publicationDraft, setPublicationDraft] = useState(() => sizes.filters.publication);
    const [capabilityDraft, setCapabilityDraft] = useState(() => sizes.filters.capability);

    useAutoOpenIntent('create-instance-size', () => {
        if (!canManageInstanceSizes) {
            return;
        }
        sizes.openCreateModal();
    });
    const catalogScopeOptions = [
        { label: t('templates.scope_unclassified'), value: 'unclassified' },
        { label: t('templates.scope_test'), value: 'test' },
        { label: t('templates.scope_prod'), value: 'prod' },
        { label: t('templates.scope_all'), value: 'all' },
    ];
    const catalogScopeFilters = catalogScopeOptions.map((option) => ({
        text: option.label,
        value: option.value,
    }));
    const enabledOptions = [
        { label: t('common:status.active', { defaultValue: 'Enabled' }), value: 'enabled' },
        { label: t('common:status.disabled', { defaultValue: 'Disabled' }), value: 'disabled' },
    ];
    const publicationOptions = [
        { label: t('instanceSizes.catalog_status_ready', { defaultValue: 'Visible in requests' }), value: 'ready' },
        { label: t('instanceSizes.catalog_status_hidden', { defaultValue: 'Hidden from requests' }), value: 'hidden' },
        { label: t('common:status.disabled', { defaultValue: 'Disabled' }), value: 'disabled' },
    ];
    const capabilityOptions = [
        { label: t('instanceSizes.capability_gpu', { defaultValue: 'GPU' }), value: 'gpu' },
        { label: t('instanceSizes.capability_sriov', { defaultValue: 'SR-IOV' }), value: 'sriov' },
        { label: t('instanceSizes.capability_hugepages', { defaultValue: 'Hugepages' }), value: 'hugepages' },
        { label: t('instanceSizes.capability_dedicated_cpu', { defaultValue: 'Dedicated CPU' }), value: 'dedicated_cpu' },
        { label: t('instanceSizes.enable_cpu_overcommit', { defaultValue: 'CPU Overcommit' }), value: 'cpu_overcommit' },
        { label: t('instanceSizes.enable_memory_overcommit', { defaultValue: 'Memory Overcommit' }), value: 'memory_overcommit' },
    ];

    // Refs for DynamicSchemaForm imperative sync (antd best practice).
    // formRef.current?.sync() is called in onValuesChange to update spec_text.
    const createFormRef = useRef<DynamicSchemaFormHandle>(null);
    const editFormRef = useRef<DynamicSchemaFormHandle>(null);
    const suspendEditInterlocksRef = useRef<boolean>(false);

    useEffect(() => {
        if (!sizes.editOpen || !sizes.editingItem || !sizes.editInitialValues) {
            return;
        }
        hydrateFormWithoutInterlocks(
            sizes.editForm,
            sizes.editInitialValues as unknown as Record<string, unknown>,
            suspendEditInterlocksRef,
        );
    }, [sizes.editForm, sizes.editInitialValues, sizes.editOpen, sizes.editingItem]);

    const getColumnSearchProps = (dataIndex: keyof InstanceSize): Partial<ColumnsType<InstanceSize>[number]> => ({
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
                                sizes.setSearchText(currentValue);
                                sizes.setSearchedColumn(String(dataIndex));
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
                            sizes.setSearchText(currentValue);
                            sizes.setSearchedColumn(String(dataIndex));
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
                            sizes.setSearchText('');
                            sizes.setSearchedColumn('');
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

    const columns: ColumnsType<InstanceSize> = [
        {
            title: t('common:table.name'),
            dataIndex: 'name',
            key: 'name',
            ...getColumnSearchProps('name'),
            sorter: (a, b) => a.name.localeCompare(b.name),
            render: (name: string, record: InstanceSize) => (
                <Space>
                    <HddOutlined style={{ color: '#1677ff' }} />
                    <div>
                        {/*
                         * Empty-string display_name should fall back to name.
                         * The API returns "" for unset optional strings, so ?? would
                         * render a blank primary label and make the row look missing.
                         */}
                        <Text strong>
                            {(() => {
                                const displayName = record.display_name || name;
                                return sizes.searchedColumn === 'name'
                                    ? highlightText(displayName, sizes.searchText)
                                    : displayName;
                            })()}
                        </Text>
                        <br />
                        <Text type="secondary" style={{ fontSize: 13 }}>
                            {sizes.searchedColumn === 'name' ? highlightText(name, sizes.searchText) : name}
                        </Text>
                    </div>
                </Space>
            ),
        },
        {
            title: t('instanceSizes.resources_summary', 'Resources'),
            key: 'resources_summary',
            width: 220,
            sorter: (a, b) => (a.cpu_cores + a.memory_gi) - (b.cpu_cores + b.memory_gi),
            render: (_: unknown, record: InstanceSize) => (
                <Space direction="vertical" size={4} style={{ width: '100%' }}>
                    <Text strong>
                        {t('instanceSizes.resources_primary', {
                            defaultValue: `${formatCores(record.cpu_cores)} ${t('instanceSizes.cores')} · ${formatMemory(record.memory_gi)}`,
                            cpu: formatCores(record.cpu_cores),
                            cores: t('instanceSizes.cores'),
                            memory: formatMemory(record.memory_gi),
                        })}
                    </Text>
                    <Text type="secondary" style={{ fontSize: 13 }}>
                        {record.disk_gb
                            ? t('instanceSizes.resources_disk_summary', {
                                defaultValue: `Default root disk ${record.disk_gb} GB`,
                                disk: record.disk_gb,
                            })
                            : t('instanceSizes.resources_disk_unset', 'Root disk size follows the selected template.')}
                    </Text>
                    {(hasCPUOvercommit(record) || hasMemoryOvercommit(record)) ? (
                        <Text type="secondary" style={{ fontSize: 13 }}>
                            {[
                                hasCPUOvercommit(record)
                                    ? t('instanceSizes.request_compact', {
                                        value: `${formatCores(record.cpu_request!)} ${t('instanceSizes.cores')}`,
                                    })
                                    : null,
                                hasMemoryOvercommit(record)
                                    ? t('instanceSizes.request_compact', {
                                        value: formatMemory(record.memory_request_gi!),
                                    })
                                    : null,
                            ].filter(Boolean).join(' · ')}
                        </Text>
                    ) : null}
                </Space>
            ),
        },
        {
            title: t('instanceSizes.catalog_publication', 'Catalog Publication'),
            key: 'catalog_publication',
            width: 240,
            render: (_: unknown, record: InstanceSize) => {
                const publication = getInstanceSizePublicationMeta(record, t);
                return (
                    <Space direction="vertical" size={4}>
                        <Space size={[4, 4]} wrap>
                            <Tag color={publication.statusColor}>{publication.statusLabel}</Tag>
                            <Tag color={catalogScopeColor(record.catalog_scope)}>
                                {catalogScopeLabel(record.catalog_scope, t)}
                            </Tag>
                            <Tag color={record.enabled !== false ? 'green' : 'default'}>
                                {record.enabled !== false ? t('common:status.active') : t('common:status.disabled')}
                            </Tag>
                        </Space>
                        <Text type="secondary" style={{ fontSize: 13 }}>
                            {publication.description}
                        </Text>
                    </Space>
                );
            },
            filters: catalogScopeFilters,
            onFilter: (value, record) => record.catalog_scope === value,
        },
        {
            title: t('instanceSizes.capabilities'),
            key: 'capabilities',
            width: 200,
            render: (_: unknown, record: InstanceSize) => {
                const tags = getInstanceSizeCapabilityTags(record, t);
                if (tags.length === 0) return <Text type="secondary">—</Text>;
                return (
                    <Space size={[0, 4]} wrap>
                        {tags.map((tag) => (
                            <Tag key={tag.label} color={tag.color}>{tag.label}</Tag>
                        ))}
                    </Space>
                );
            },
        },
        {
            title: t('systemLabels.compatibility_column', 'Compatibility'),
            key: 'system_labels',
            width: 180,
            render: (_: unknown, record: InstanceSize) => (
                <Space size={[0, 4]} wrap>
                    {renderSystemLabelTags(record.system_labels, t)}
                </Space>
            ),
        },
        {
            title: t('common:table.actions'),
            key: 'actions',
            width: 150,
            render: (_: unknown, record: InstanceSize) =>
                canManageInstanceSizes ? (
                    <Space size="small">
                        <Button
                            type="link"
                            size="small"
                            data-testid={`instance-size-action-edit-${record.id}`}
                            icon={<EditOutlined />}
                            onClick={() => sizes.openEditModal(record)}
                        >
                            {t('common:button.edit')}
                        </Button>
                        <Button
                            type="link"
                            size="small"
                            danger
                            data-testid={`instance-size-action-delete-${record.id}`}
                            icon={<DeleteOutlined />}
                            onClick={() => sizes.openDeleteModal(record)}
                        >
                            {t('common:button.delete')}
                        </Button>
                    </Space>
                ) : (
                    <Text type="secondary">—</Text>
                ),
        },
    ];

    const sizeSummary = useMemo(() => {
        const items = sizes.filteredItems;
        const enabledCount = items.filter((item) => item.enabled !== false).length;
        const requestReadyCount = items.filter(
            (item) => item.enabled !== false && (item.catalog_scope ?? 'unclassified') !== 'unclassified',
        ).length;
        const specializedCount = items.filter(
            (item) =>
                item.requires_gpu ||
                item.requires_hugepages ||
                item.requires_sriov ||
                hasCPUOvercommit(item) ||
                hasMemoryOvercommit(item),
        ).length;
        return {
            totalCount: items.length,
            enabledCount,
            requestReadyCount,
            specializedCount,
        };
    }, [sizes.filteredItems]);

    return (
        <div>
            {sizes.messageContextHolder}
            <PageHeader
                title={t('instanceSizes.title')}
                subtitle={t('instanceSizes.subtitle')}
                actions={(
                    <Space>
                    <Button icon={<ReloadOutlined />} onClick={() => sizes.refetch()}>
                        {t('common:button.refresh')}
                    </Button>
                    {canManageInstanceSizes ? (
                        <Button
                            type="primary"
                            icon={<HddOutlined />}
                            data-testid="instance-size-create-button"
                            onClick={sizes.openCreateModal}
                        >
                            {t('common:button.add')}
                        </Button>
                    ) : null}
                    </Space>
                )}
            />
            <div className="summary-card-grid">
                <SummaryMetricCard
                    title={t('instanceSizes.summary.total_title', 'Catalog sizes')}
                    value={sizeSummary.totalCount}
                    description={t('instanceSizes.summary.total_description', 'Instance-size profiles visible after the current filters.')}
                    visual={<InstanceSizeBlueprintGlyph className="summary-metric-card__art" />}
                    accentColor="#1D5BFF"
                    surfaceColor="#E6F4FF"
                />
                <SummaryMetricCard
                    title={t('instanceSizes.summary.enabled_title', 'Enabled')}
                    value={sizeSummary.enabledCount}
                    description={t('instanceSizes.summary.enabled_description', 'Profiles currently available for catalog publication.')}
                    visual={<ServiceWorkspaceGlyph className="summary-metric-card__art" />}
                    accentColor="#0F8F57"
                    surfaceColor="#E8FFF2"
                />
                <SummaryMetricCard
                    title={t('instanceSizes.summary.request_ready_title', 'Request-ready')}
                    value={sizeSummary.requestReadyCount}
                    description={t('instanceSizes.summary.request_ready_description', 'Profiles already visible to the VM request flow.')}
                    visual={<RequestsOverviewGlyph className="summary-metric-card__art" />}
                    accentColor="#D66A1F"
                    surfaceColor="#FFF4E5"
                />
                <SummaryMetricCard
                    title={t('instanceSizes.summary.specialized_title', 'Specialized')}
                    value={sizeSummary.specializedCount}
                    description={t('instanceSizes.summary.specialized_description', 'Profiles with GPU, hugepages, SR-IOV, or overcommit tuning.')}
                    visual={<QueueReviewGlyph className="summary-metric-card__art" />}
                    accentColor="#6D4DE3"
                    surfaceColor="#F5EDFF"
                />
            </div>

            <PageSurface flush={true}>
                <PageSearchToolbar
                    searchValue={sizes.globalSearch}
                    searchDraftValue={quickSearchDraft}
                    onSearchDraftChange={setQuickSearchDraft}
                    onSearchChange={(value) => {
                        setQuickSearchDraft(value);
                        sizes.applyFilters({ search: value });
                    }}
                    searchPlaceholder={t('instanceSizes.search_placeholder', 'Search profiles by name, display name, resource settings, or ID')}
                    searchHelp={t('instanceSizes.search_help', 'Press Enter or click Search. Quick search matches profile names, display names, CPU, memory, disk, capabilities, key catalog flags, and IDs.')}
                    advancedSearch={{
                        open: filtersOpen,
                        onToggle: () => setFiltersOpen((open) => !open),
                        openLabel: t('common:search.advanced', { defaultValue: 'Advanced search' }),
                        closeLabel: t('common:search.hide_advanced', { defaultValue: 'Hide advanced search' }),
                        title: t('instanceSizes.advanced_search_title', 'Advanced search'),
                        content: (
                            <Space direction="vertical" size={12} style={{ width: '100%' }}>
                                <Text type="secondary">
                                    {t('instanceSizes.advanced_search_help', 'Choose exact profile filters here. Options can be searched by keyword, but the applied filter remains an exact value.')}
                                </Text>
                                <Space wrap>
                                    <Select
                                        allowClear
                                        showSearch
                                        optionFilterProp="label"
                                        placeholder={t('instanceSizes.catalog_scope', 'Catalog scope')}
                                        value={catalogScopeDraft || undefined}
                                        options={catalogScopeOptions}
                                        style={{ minWidth: 200 }}
                                        onChange={(value) => setCatalogScopeDraft((value as string | undefined) ?? '')}
                                    />
                                    <Select
                                        allowClear
                                        showSearch
                                        optionFilterProp="label"
                                        placeholder={t('common:table.status', 'Status')}
                                        value={enabledDraft || undefined}
                                        options={enabledOptions}
                                        style={{ minWidth: 180 }}
                                        onChange={(value) =>
                                            setEnabledDraft((value as 'enabled' | 'disabled' | undefined) ?? '')
                                        }
                                    />
                                    <Select
                                        allowClear
                                        showSearch
                                        optionFilterProp="label"
                                        placeholder={t('instanceSizes.catalog_publication', 'Catalog publication')}
                                        value={publicationDraft || undefined}
                                        options={publicationOptions}
                                        style={{ minWidth: 220 }}
                                        onChange={(value) =>
                                            setPublicationDraft((value as 'ready' | 'hidden' | 'disabled' | undefined) ?? '')
                                        }
                                    />
                                    <Select
                                        allowClear
                                        showSearch
                                        optionFilterProp="label"
                                        placeholder={t('instanceSizes.capabilities', 'Capabilities')}
                                        value={capabilityDraft || undefined}
                                        options={capabilityOptions}
                                        style={{ minWidth: 220 }}
                                        onChange={(value) =>
                                            setCapabilityDraft(
                                                (
                                                    value as
                                                        | 'gpu'
                                                        | 'sriov'
                                                        | 'hugepages'
                                                        | 'dedicated_cpu'
                                                        | 'cpu_overcommit'
                                                        | 'memory_overcommit'
                                                        | undefined
                                                ) ?? '',
                                            )
                                        }
                                    />
                                    <Button
                                        type="primary"
                                        onClick={() =>
                                            sizes.applyFilters({
                                                search: quickSearchDraft,
                                                catalogScope: catalogScopeDraft,
                                                enabled: enabledDraft,
                                                publication: publicationDraft,
                                                capability: capabilityDraft,
                                            })
                                        }
                                    >
                                        {t('common:button.search')}
                                    </Button>
                                </Space>
                            </Space>
                        ),
                    }}
                    hasActiveFilters={sizes.hasActiveFilters}
                    onClear={() => {
                        setQuickSearchDraft('');
                        setCatalogScopeDraft('');
                        setEnabledDraft('');
                        setPublicationDraft('');
                        setCapabilityDraft('');
                        sizes.clearFilters();
                    }}
                    clearLabel={t('common:button.clear_filters', { defaultValue: 'Clear filters' })}
                />
                {sizes.listError && (
                    <Alert
                        type="error"
                        showIcon
                        style={{ margin: 16, marginTop: 16, marginBottom: 0 }}
                        message={t('instanceSizes.load_error', 'Failed to load instance sizes')}
                        description={translateApiError(t, sizes.listError)}
                        action={
                            <Button size="small" onClick={() => sizes.refetch()}>
                                {t('common:button.refresh')}
                            </Button>
                        }
                    />
                )}
                <div style={{
                    marginTop: sizes.listError ? 0 : 16,
                    opacity: sizes.isStale ? 0.6 : 1,
                    transition: sizes.isStale ? 'opacity 0.2s 0.1s linear' : 'opacity 0s 0s linear',
                }}>
                    <Table<InstanceSize>
                        columns={columns}
                        dataSource={sizes.filteredItems}
                        rowKey="id"
                        loading={sizes.isLoading}
                        size="middle"
                        pagination={{
                            total: sizes.filteredItems.length,
                            pageSize: 20,
                            showTotal: (total) => t('common:table.total', { total }),
                            showSizeChanger: false,
                        }}
                        locale={{
                            emptyText: (
                                <ActionEmptyState
                                    compact={true}
                                    title={
                                        sizes.deferredSearch
                                            ? t('common:message.no_data')
                                            : t('instanceSizes.empty')
                                    }
                                    description={
                                        sizes.deferredSearch
                                            ? t('instanceSizes.empty_filtered_description', 'Try a broader search or reset the current filters.')
                                            : t('instanceSizes.empty_description', 'Create at least one instance-size profile before opening the VM request catalog.')
                                    }
                                    visual={<InstanceSizeBlueprintGlyph className="action-empty-state__art action-empty-state__art--compact" />}
                                />
                            ),
                        }}
                    />
                </div>
            </PageSurface>

            {/* Create Modal */}
            <Modal
                title={t('instanceSizes.create_title')}
                open={sizes.createOpen}
                onOk={() => { void sizes.submitCreate(); }}
                onCancel={sizes.closeCreateModal}
                confirmLoading={sizes.createPending}
                width={720}
                data-testid="instance-size-create-modal"
            >
                <Form
                    key="instance-size-create-form"
                    form={sizes.createForm}
                    layout="vertical"
                    preserve={false}
                    onValuesChange={(changedValues) => {
                        handleInstanceSizeFormValuesChange(sizes.createForm, createFormRef, changedValues);
                    }}
                >
                    <InstanceSizePresetPicker t={t} form={sizes.createForm} formRef={createFormRef} />
                    <InstanceSizeFormFields isCreate={true} formRef={createFormRef} />
                </Form>
            </Modal>

            {/* Edit Modal */}
            <Modal
                title={t('instanceSizes.edit_title')}
                open={sizes.editOpen}
                forceRender={true}
                onOk={() => { void sizes.submitEdit(); }}
                onCancel={sizes.closeEditModal}
                confirmLoading={sizes.updatePending}
                width={720}
                data-testid="instance-size-edit-modal"
            >
                <Form
                    form={sizes.editForm}
                    layout="vertical"
                    preserve={false}
                    onValuesChange={(changedValues) => {
                        if (suspendEditInterlocksRef.current) {
                            return;
                        }
                        handleInstanceSizeFormValuesChange(sizes.editForm, editFormRef, changedValues);
                    }}
                >
                    <InstanceSizePresetPicker t={t} form={sizes.editForm} formRef={editFormRef} />
                    <InstanceSizeFormFields isCreate={false} formRef={editFormRef} />
                </Form>
            </Modal>

            {/* Delete Modal */}
            <Modal
                title={t('common:button.delete')}
                open={sizes.deleteOpen}
                onOk={sizes.submitDelete}
                onCancel={sizes.closeDeleteModal}
                confirmLoading={sizes.deletePending}
                okButtonProps={{ danger: true }}
            >
                <Text>
                    {t('common:message.delete_confirm', {
                        name: sizes.deletingItem?.display_name || sizes.deletingItem?.name || '-',
                    })}
                </Text>
            </Modal>
        </div>
    );
}
