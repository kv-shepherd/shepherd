'use client';

import {
    Alert,
    AutoComplete,
    Button,
    Card,
    Descriptions,
    Form,
    Input,
    InputNumber,
    Modal,
    Segmented,
    Select,
    Space,
    Steps,
    Tag,
    Typography,
    type FormInstance,
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
    VMCreateRequest,
    VMPlacementHint,
    VMRequestMode,
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

interface VMRequestWizardProps {
    t: TFunction;
    open: boolean;
    step: number;
    setStep: (step: number) => void;
    requestMode: VMRequestMode;
    onRequestModeChange: (mode: VMRequestMode) => void;
    form: FormInstance<VMCreateRequest>;
    wizardSteps: Array<{ title: string }>;
    selectedSystemId: string;
    onSystemChange: (systemId: string) => void;
    systemsData: SystemList | undefined;
    servicesData: ServiceList | undefined;
    templatesData: TemplateList | undefined;
    sizesData: InstanceSizeList | undefined;
    selectedTemplate: Template | undefined;
    selectedSize: InstanceSize | undefined;
    placementHint: VMPlacementHint | undefined;
    placementHintLoading: boolean;
    serviceIdValue: string | undefined;
    namespaceValue: string | undefined;
    namespaceOptions: string[];
    reasonValue: string | undefined;
    batchCountValue: number;
    isSubmitting: boolean;
    onCancel: () => void;
    onNext: () => void;
    onSubmit: () => void;
}

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

function renderSectionCard(title: ReactNode, children: ReactNode, extra?: ReactNode) {
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

function formatInstanceSizeSummary(size: InstanceSize, t: TFunction) {
    return t('wizard.size_summary', {
        cpu: size.cpu_cores,
        memory: formatMemory(size.memory_gi),
    });
}

function formatInstanceSizeDisk(size: InstanceSize, t: TFunction) {
    return t('wizard.size_disk_suffix', { disk: size.disk_gb });
}

export function VMRequestWizard({
    t,
    open,
    step,
    setStep,
    requestMode,
    onRequestModeChange,
    form,
    wizardSteps,
    selectedSystemId,
    onSystemChange,
    systemsData,
    servicesData,
    templatesData,
    sizesData,
    selectedTemplate,
    selectedSize,
    placementHint,
    placementHintLoading,
    serviceIdValue,
    namespaceValue,
    namespaceOptions,
    reasonValue,
    batchCountValue,
    isSubmitting,
    onCancel,
    onNext,
    onSubmit,
}: VMRequestWizardProps) {
    const renderServiceFields = () => (
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

    const renderTemplateFields = () => (
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

    const renderSizeFields = () => (
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
                        .map((size: InstanceSize) => ({
                            label: (
                                <Space direction="vertical" size={0}>
                                    <Space size={6}>
                                        <Text strong>{size.display_name || size.name}</Text>
                                        <Text type="secondary">{formatInstanceSizeSummary(size, t)}</Text>
                                        {size.disk_gb && <Text type="secondary">· {formatInstanceSizeDisk(size, t)}</Text>}
                                    </Space>
                                    {capabilityTags(size, t).length > 0 && (
                                        <Space size={4} wrap>
                                            {capabilityTags(size, t)}
                                        </Space>
                                    )}
                                </Space>
                            ),
                            value: size.id,
                        }))}
                    style={{ width: '100%' }}
                />
            </Form.Item>
            {selectedSize && capabilityTags(selectedSize, t).length > 0 && (
                <Alert
                    type={selectedSize.requires_gpu ? 'warning' : 'info'}
                    showIcon
                    message={t('wizard.size_capability_notice')}
                    description={<Space wrap>{capabilityTags(selectedSize, t)}</Space>}
                />
            )}
        </>
    );

    const renderConfigFields = () => (
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

    const renderConfirmStep = () => (
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
                                {selectedSize && capabilityTags(selectedSize, t).length > 0 ? (
                                    <Space wrap>{capabilityTags(selectedSize, t)}</Space>
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
                ]}
            />
        </Space>
    );

    const renderFullForm = () => (
        <Space direction="vertical" size={24} style={{ width: '100%' }}>
            <Alert
                type="info"
                showIcon
                message={t('wizard.mode.full_description')}
            />
            {renderSectionCard(t('wizard.section.service'), renderServiceFields())}
            {renderSectionCard(t('wizard.section.template'), renderTemplateFields())}
            {renderSectionCard(t('wizard.section.size'), renderSizeFields())}
            {renderSectionCard(t('wizard.section.configuration'), renderConfigFields())}
            {renderSectionCard(t('wizard.section.review'), renderConfirmStep())}
        </Space>
    );

    const renderStep = () => {
        switch (step) {
            case 0:
                return renderServiceFields();
            case 1:
                return renderTemplateFields();
            case 2:
                return renderSizeFields();
            case 3:
                return renderConfigFields();
            case 4:
                return renderConfirmStep();
            default:
                return null;
        }
    };

    const footer = requestMode === 'full'
        ? (
            <Space>
                <Button onClick={onCancel}>
                    {t('common:button.cancel')}
                </Button>
                <Button
                    type="primary"
                    onClick={onSubmit}
                    loading={isSubmitting}
                >
                    {t('common:button.submit')}
                </Button>
            </Space>
        )
        : (
            <Space>
                {step > 0 && (
                    <Button onClick={() => setStep(step - 1)}>
                        {t('common:button.prev')}
                    </Button>
                )}
                {step < wizardSteps.length - 1 ? (
                    <Button type="primary" onClick={onNext}>
                        {t('common:button.next')}
                    </Button>
                ) : (
                    <Button
                        type="primary"
                        onClick={onSubmit}
                        loading={isSubmitting}
                    >
                        {t('common:button.submit')}
                    </Button>
                )}
            </Space>
        );

    return (
        <Modal
            title={(
                <Space direction="vertical" size={2}>
                    <span>{t('wizard.title')}</span>
                    <Text type="secondary">
                        {requestMode === 'guided' ? t('wizard.mode.guided_description') : t('wizard.mode.full_description')}
                    </Text>
                </Space>
            )}
            open={open}
            onCancel={onCancel}
            width={720}
            footer={footer}
            styles={{ body: { paddingTop: 8 } }}
            forceRender={true}
            data-testid="vm-request-wizard-modal"
        >
            <Space direction="vertical" size={16} style={{ width: '100%' }}>
                {renderSectionCard(
                    t('wizard.mode.title'),
                    <Segmented
                        block
                        value={requestMode}
                        onChange={(value) => onRequestModeChange(value as VMRequestMode)}
                        options={[
                            { label: t('wizard.mode.guided'), value: 'guided' },
                            { label: t('wizard.mode.full'), value: 'full' },
                        ]}
                    />,
                    <Text type="secondary">
                        {requestMode === 'guided' ? t('wizard.mode.guided_description') : t('wizard.mode.full_description')}
                    </Text>
                )}
                {requestMode === 'guided' && (
                    renderSectionCard(
                        t('wizard.progress_title'),
                        <Steps current={step} items={wizardSteps} size="small" />
                    )
                )}
                <Form form={form} layout="vertical" name="vm-request-wizard">
                    {requestMode === 'guided'
                        ? renderSectionCard(wizardSteps[step]?.title ?? t('wizard.title'), renderStep())
                        : renderFullForm()}
                </Form>
            </Space>
        </Modal>
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
