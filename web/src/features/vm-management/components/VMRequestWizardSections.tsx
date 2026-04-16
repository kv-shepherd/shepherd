'use client';

import {
    Alert,
    AutoComplete,
    Card,
    Descriptions,
    Form,
    Input,
    InputNumber,
    Select,
    Space,
    Tag,
    Typography,
} from 'antd';
import type { TFunction } from 'i18next';
import type { ReactNode } from 'react';

import type {
    InstanceSize,
    InstanceSizeList,
    ServiceList,
    SystemList,
    Template,
    TemplateList,
    VMPlacementHint,
} from '../types';
import { formatMemory } from '../types';
import {
    getPlacementAdvisoryLabelKey,
    getPlacementReasonActionKey,
    shouldShowPlacementHintToUser,
    sortPlacementAdvisoryCounts,
    sortPlacementReasonCounts,
} from '../placementHint';

const { Text } = Typography;

function capabilityTags(size: InstanceSize, t: TFunction) {
    const tags: ReactNode[] = [];
    if (size.requires_gpu) {
        tags.push(<Tag key="gpu" color="volcano">{t('capability.gpu')}</Tag>);
    }
    if (size.requires_sriov) {
        tags.push(<Tag key="sriov" color="purple">{t('capability.sriov')}</Tag>);
    }
    if (size.requires_hugepages) {
        const label = size.hugepages_size ? `${t('capability.hugepages')}: ${size.hugepages_size}` : t('capability.hugepages');
        tags.push(<Tag key="hugepages" color="gold">{label}</Tag>);
    }
    if (size.dedicated_cpu) {
        tags.push(<Tag key="dedicated" color="blue">{t('capability.dedicated_cpu')}</Tag>);
    }
    return tags;
}

function formatInstanceSizeSummary(size: InstanceSize, t: TFunction) {
    return t('wizard.size_summary', {
        cpu: size.cpu_cores,
        memory: formatMemory(size.memory_gi),
    });
}

function formatInstanceSizeDisk(size: InstanceSize, t: TFunction) {
    return t('wizard.size_disk_suffix', { disk: size.disk_gb });
}

function formatCPUValue(value: number) {
    if (!Number.isFinite(value) || value <= 0) {
        return '0';
    }
    return Number.isInteger(value) ? String(value) : value.toFixed(1);
}

export function VMRequestSectionCard({
    title,
    children,
    extra,
}: {
    title: ReactNode;
    children: ReactNode;
    extra?: ReactNode;
}) {
    return (
        <Card
            size="small"
            title={title}
            extra={extra}
            styles={{ body: { paddingTop: 16 } }}
        >
            {children}
        </Card>
    );
}

export function VMRequestServiceFields({
    t,
    selectedSystemId,
    onSystemChange,
    systemsData,
    servicesData,
}: {
    t: TFunction;
    selectedSystemId: string;
    onSystemChange: (systemId: string) => void;
    systemsData: SystemList | undefined;
    servicesData: ServiceList | undefined;
}) {
    return (
        <>
            <Form.Item label={t('wizard.select_system')} style={{ marginBottom: 16 }}>
                <Select
                    placeholder={t('wizard.select_system')}
                    showSearch
                    optionFilterProp="label"
                    value={selectedSystemId || undefined}
                    onChange={onSystemChange}
                    options={systemsData?.items?.map((system) => ({
                        label: system.name,
                        value: system.id,
                    }))}
                    style={{ width: '100%' }}
                />
            </Form.Item>
            <Form.Item
                name="service_id"
                label={t('wizard.select_service')}
                rules={[{ required: true, message: t('wizard.validation.service_required') }]}
            >
                <Select
                    placeholder={t('wizard.select_service')}
                    showSearch
                    optionFilterProp="label"
                    disabled={!selectedSystemId}
                    options={servicesData?.items?.map((service) => ({
                        label: service.name,
                        value: service.id,
                    }))}
                    style={{ width: '100%' }}
                />
            </Form.Item>
        </>
    );
}

export function VMRequestTemplateFields({
    t,
    templatesData,
}: {
    t: TFunction;
    templatesData: TemplateList | undefined;
}) {
    return (
        <Form.Item
            name="template_id"
            label={t('wizard.select_template')}
            rules={[{ required: true, message: t('wizard.validation.template_required') }]}
        >
            <Select
                placeholder={t('wizard.select_template')}
                options={templatesData?.items
                    ?.filter((template: Template) => template.enabled !== false)
                    .map((template: Template) => ({
                        label: (
                            <Space>
                                <Text strong>{template.display_name || template.name}</Text>
                                {template.os_family && <Tag color="blue">{template.os_family} {template.os_version}</Tag>}
                            </Space>
                        ),
                        value: template.id,
                    }))}
                style={{ width: '100%' }}
            />
        </Form.Item>
    );
}

export function VMRequestSizeFields({
    t,
    sizesData,
    selectedSize,
    targetCpuValue,
    targetMemoryValue,
    targetDiskValue,
}: {
    t: TFunction;
    sizesData: InstanceSizeList | undefined;
    selectedSize: InstanceSize | undefined;
    targetCpuValue?: number;
    targetMemoryValue?: number;
    targetDiskValue?: number;
}) {
    const selectedSizeTags = selectedSize ? capabilityTags(selectedSize, t) : [];

    return (
        <>
            <Form.Item
                name="instance_size_id"
                label={t('wizard.select_size')}
                rules={[{ required: true, message: t('wizard.validation.size_required') }]}
            >
                <Select
                    placeholder={t('wizard.select_size')}
                    options={sizesData?.items
                        ?.filter((size: InstanceSize) => size.enabled !== false)
                        .map((size: InstanceSize) => {
                            const sizeTags = capabilityTags(size, t);
                            return {
                                label: `${size.display_name || size.name}  ${formatInstanceSizeSummary(size, t)}${size.disk_gb ? ` · ${formatInstanceSizeDisk(size, t)}` : ''}`,
                                value: size.id,
                                sizeName: size.display_name || size.name,
                                sizeSummary: formatInstanceSizeSummary(size, t),
                                sizeDisk: size.disk_gb ? formatInstanceSizeDisk(size, t) : null,
                                sizeTags,
                            };
                        })}
                    optionRender={(option) => (
                        <div style={{ display: 'flex', flexDirection: 'column', gap: 2, padding: '4px 0' }}>
                            <div>
                                <Text strong>{option.data.sizeName}</Text>{' '}
                                <Text type="secondary">{option.data.sizeSummary}</Text>
                                {option.data.sizeDisk && <Text type="secondary"> · {option.data.sizeDisk}</Text>}
                            </div>
                            {option.data.sizeTags?.length > 0 ? (
                                <Space size={4} wrap>
                                    {option.data.sizeTags}
                                </Space>
                            ) : null}
                        </div>
                    )}
                    popupMatchSelectWidth={false}
                    style={{ width: '100%' }}
                />
            </Form.Item>
            {selectedSize && selectedSizeTags.length > 0 ? (
                <Alert
                    type={selectedSize.requires_gpu ? 'warning' : 'info'}
                    showIcon
                    message={t('wizard.size_capability_notice')}
                    description={<Space wrap>{selectedSizeTags}</Space>}
                    style={{ marginBottom: 24 }}
                />
            ) : null}
            {selectedSize ? (
                <>
                    <Alert
                        type="info"
                        showIcon
                        message={t('wizard.custom_resources_title')}
                        description={t('wizard.custom_resources_hint')}
                        style={{ marginBottom: 24 }}
                    />
                    <Form.Item
                        name="target_cpu_cores"
                        label={t('modify.target_cpu')}
                        extra={t('wizard.custom_resource_default', {
                            value: `${formatCPUValue(selectedSize.cpu_cores)} vCPU`,
                        })}
                    >
                        <InputNumber
                            min={0.5}
                            step={0.5}
                            precision={1}
                            placeholder={formatCPUValue(selectedSize.cpu_cores)}
                            style={{ width: '100%' }}
                        />
                    </Form.Item>
                    <Form.Item
                        name="target_memory_gi"
                        label={t('modify.target_memory')}
                        extra={t('wizard.custom_resource_default', {
                            value: formatMemory(selectedSize.memory_gi),
                        })}
                    >
                        <InputNumber
                            min={0.5}
                            step={0.5}
                            precision={1}
                            placeholder={String(selectedSize.memory_gi)}
                            style={{ width: '100%' }}
                        />
                    </Form.Item>
                    <Form.Item
                        name="target_disk_gb"
                        label={t('modify.target_disk')}
                        extra={t('wizard.custom_resource_default', {
                            value: `${selectedSize.disk_gb} Gi`,
                        })}
                    >
                        <InputNumber
                            min={1}
                            step={1}
                            precision={0}
                            placeholder={String(selectedSize.disk_gb)}
                            style={{ width: '100%' }}
                        />
                    </Form.Item>
                    {targetCpuValue || targetMemoryValue || targetDiskValue ? (
                        <Alert
                            type="success"
                            showIcon
                            message={t('wizard.custom_resources_active')}
                        />
                    ) : null}
                </>
            ) : null}
        </>
    );
}

export function VMRequestConfigurationFields({
    t,
    selectedTemplate,
    selectedSize,
    placementHint,
    placementHintLoading,
    namespaceValue,
    namespaceOptions,
}: {
    t: TFunction;
    selectedTemplate: Template | undefined;
    selectedSize: InstanceSize | undefined;
    placementHint: VMPlacementHint | undefined;
    placementHintLoading: boolean;
    namespaceValue: string | undefined;
    namespaceOptions: string[];
}) {
    return (
        <>
            <Form.Item
                name="namespace"
                label={t('wizard.namespace')}
                rules={[{ required: true, message: t('wizard.validation.namespace_required') }]}
                extra={t('wizard.namespace_hint')}
            >
                <AutoComplete
                    placeholder={t('wizard.namespace_placeholder')}
                    options={namespaceOptions.map((ns) => ({ value: ns }))}
                    filterOption={(inputValue, option) => (
                        (option?.value ?? '').toLowerCase().includes(inputValue.toLowerCase())
                    )}
                />
            </Form.Item>
            <Form.Item
                name="reason"
                label={t('wizard.reason')}
                rules={[{ required: true, message: t('wizard.validation.reason_required') }]}
            >
                <Input.TextArea rows={4} placeholder={t('wizard.reason_placeholder')} />
            </Form.Item>
            {selectedTemplate &&
            selectedSize &&
            namespaceValue &&
            shouldShowPlacementHintToUser(placementHint) ? (
                placementHintLoading ? (
                    <Alert
                        type="info"
                        showIcon
                        style={{ marginBottom: 16 }}
                        message={t('wizard.placement_hint_loading')}
                    />
                ) : placementHint ? (
                    <Alert
                        type={placementHint.status === 'AVAILABLE' ? 'success' : 'warning'}
                        showIcon
                        style={{ marginBottom: 16 }}
                        message={placementHint.status === 'AVAILABLE'
                            ? t('wizard.placement_hint_available', { count: placementHint.compatible_cluster_count })
                            : t('wizard.placement_hint_unavailable')}
                        description={renderPlacementHintDescription(placementHint, t)}
                    />
                ) : null
            ) : null}
            <Form.Item
                name="batch_count"
                label={t('wizard.batch_count')}
                rules={[{ required: true, message: t('wizard.batch_count_required') }]}
                initialValue={1}
                extra={t('wizard.batch_count_hint')}
            >
                <InputNumber
                    min={1}
                    max={50}
                    style={{ width: '100%' }}
                />
            </Form.Item>
        </>
    );
}

export function VMRequestConfirmStep({
    t,
    servicesData,
    selectedTemplate,
    selectedSize,
    serviceIdValue,
    namespaceValue,
    reasonValue,
    batchCountValue,
    targetCpuValue,
    targetMemoryValue,
    targetDiskValue,
}: {
    t: TFunction;
    servicesData: ServiceList | undefined;
    selectedTemplate: Template | undefined;
    selectedSize: InstanceSize | undefined;
    serviceIdValue: string | undefined;
    namespaceValue: string | undefined;
    reasonValue: string | undefined;
    batchCountValue: number;
    targetCpuValue?: number;
    targetMemoryValue?: number;
    targetDiskValue?: number;
}) {
    const selectedSizeTags = selectedSize ? capabilityTags(selectedSize, t) : [];

    return (
        <Space direction="vertical" size={16} style={{ width: '100%' }}>
            <Alert
                type="info"
                message={t('wizard.confirm_note')}
                showIcon
            />
            <Descriptions
                bordered
                column={1}
                size="small"
                items={[
                    {
                        key: 'service',
                        label: t('wizard.confirm_service'),
                        children: servicesData?.items?.find((service) => service.id === serviceIdValue)?.name ?? '—',
                    },
                    {
                        key: 'template',
                        label: t('wizard.confirm_template'),
                        children: selectedTemplate?.display_name || selectedTemplate?.name || '—',
                    },
                    {
                        key: 'size',
                        label: t('wizard.confirm_size'),
                        children: (
                            <Space direction="vertical" size={8} style={{ width: '100%' }}>
                                <span>
                                    {selectedSize
                                        ? `${selectedSize.display_name || selectedSize.name} (${formatInstanceSizeSummary(selectedSize, t)})`
                                        : '—'}
                                </span>
                                {selectedSizeTags.length > 0 ? (
                                    <Space wrap>{selectedSizeTags}</Space>
                                ) : null}
                            </Space>
                        ),
                    },
                    {
                        key: 'namespace',
                        label: t('wizard.confirm_namespace'),
                        children: <Tag>{namespaceValue}</Tag>,
                    },
                    {
                        key: 'reason',
                        label: t('wizard.confirm_reason'),
                        children: reasonValue,
                    },
                    {
                        key: 'batchCount',
                        label: t('wizard.confirm_batch_count'),
                        children: batchCountValue,
                    },
                    ...(targetCpuValue
                        ? [{
                            key: 'targetCpu',
                            label: t('modify.target_cpu'),
                            children: `${formatCPUValue(targetCpuValue)} vCPU`,
                        }]
                        : []),
                    ...(targetMemoryValue
                        ? [{
                            key: 'targetMemory',
                            label: t('modify.target_memory'),
                            children: formatMemory(targetMemoryValue),
                        }]
                        : []),
                    ...(targetDiskValue
                        ? [{
                            key: 'targetDisk',
                            label: t('modify.target_disk'),
                            children: `${targetDiskValue} Gi`,
                        }]
                        : []),
                ]}
            />
        </Space>
    );
}

function renderPlacementHintDescription(hint: VMPlacementHint, t: TFunction): ReactNode {
    if (hint.status === 'AVAILABLE') {
        const advisoryCounts = sortPlacementAdvisoryCounts(hint.advisory_counts);
        return (
            <Space direction="vertical" size={4}>
                <Text>
                    {t('wizard.placement_hint_available_detail', {
                        count: hint.compatible_cluster_count,
                        total: hint.evaluated_cluster_count,
                    })}
                </Text>
                {hint.primary_advisory_code ? (
                    <Text type="warning">
                        {t('wizard.placement_hint_advisory', {
                            note: t(getPlacementAdvisoryLabelKey(hint.primary_advisory_code)),
                            count: advisoryCounts[0]?.count ?? 1,
                        })}
                    </Text>
                ) : null}
            </Space>
        );
    }

    const primaryReason = t(`wizard.placement_reason.${hint.primary_reason_code ?? 'Other'}`);
    const suggestedAction = t(getPlacementReasonActionKey(hint.primary_reason_code));
    const reasonCounts = sortPlacementReasonCounts(hint.reason_counts);
    return (
        <Space direction="vertical" size={6}>
            <Text>{t('wizard.placement_hint_unavailable_detail', { reason: primaryReason })}</Text>
            <Text type="secondary">{t('wizard.placement_hint_next_step', { action: suggestedAction })}</Text>
            {reasonCounts.length > 0 ? (
                <Space size={[0, 4]} wrap>
                    {reasonCounts.slice(0, 3).map((reason) => (
                        <Tag key={reason.code} color="orange">
                            {t(`wizard.placement_reason.${reason.code}`)} ({reason.count})
                        </Tag>
                    ))}
                </Space>
            ) : null}
        </Space>
    );
}
