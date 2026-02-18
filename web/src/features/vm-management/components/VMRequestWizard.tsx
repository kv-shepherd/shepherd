'use client';

import {
    Alert,
    AutoComplete,
    Button,
    Descriptions,
    Divider,
    Form,
    Input,
    InputNumber,
    Modal,
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
} from '../types';
import { formatMemory } from '../types';

const { Text } = Typography;

interface VMRequestWizardProps {
    t: TFunction;
    open: boolean;
    step: number;
    setStep: (step: number) => void;
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

export function VMRequestWizard({
    t,
    open,
    step,
    setStep,
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
    const renderStep = () => {
        switch (step) {
            case 0:
                return (
                    <>
                        <Form.Item label={t('wizard.select_system')} style={{ marginBottom: 16 }}>
                            <Select
                                placeholder={t('wizard.select_system')}
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
            case 1:
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
            case 2:
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
                                    .map((size: InstanceSize) => ({
                                        label: (
                                            <Space direction="vertical" size={0}>
                                                <Space size={6}>
                                                    <Text strong>{size.display_name || size.name}</Text>
                                                    <Text type="secondary">{size.cpu_cores} vCPU · {formatMemory(size.memory_mb)}</Text>
                                                    {size.disk_gb && <Text type="secondary">· {size.disk_gb} GB</Text>}
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
            case 3:
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
            case 4:
                return (
                    <div>
                        <Alert
                            type="info"
                            message={t('wizard.confirm_note')}
                            style={{ marginBottom: 16 }}
                            showIcon
                        />
                        <Descriptions bordered column={1} size="small">
                            <Descriptions.Item label={t('wizard.confirm_service')}>
                                {servicesData?.items?.find((service) => service.id === serviceIdValue)?.name ?? '—'}
                            </Descriptions.Item>
                            <Descriptions.Item label={t('wizard.confirm_template')}>
                                {selectedTemplate?.display_name || selectedTemplate?.name || '—'}
                            </Descriptions.Item>
                            <Descriptions.Item label={t('wizard.confirm_size')}>
                                {selectedSize ? `${selectedSize.display_name || selectedSize.name} (${selectedSize.cpu_cores} vCPU · ${formatMemory(selectedSize.memory_mb)})` : '—'}
                                {selectedSize && capabilityTags(selectedSize, t).length > 0 && (
                                    <div style={{ marginTop: 8 }}>
                                        <Space wrap>{capabilityTags(selectedSize, t)}</Space>
                                    </div>
                                )}
                            </Descriptions.Item>
                            <Descriptions.Item label={t('wizard.confirm_namespace')}>
                                <Tag>{namespaceValue}</Tag>
                            </Descriptions.Item>
                            <Descriptions.Item label={t('wizard.confirm_reason')}>
                                {reasonValue}
                            </Descriptions.Item>
                            <Descriptions.Item label={t('wizard.confirm_batch_count')}>
                                {batchCountValue}
                            </Descriptions.Item>
                        </Descriptions>
                    </div>
                );
            default:
                return null;
        }
    };

    return (
        <Modal
            title={t('wizard.title')}
            open={open}
            onCancel={onCancel}
            width={720}
            footer={(
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
            )}
            forceRender
        >
            <Steps current={step} items={wizardSteps} size="small" style={{ marginBottom: 24 }} />
            <Divider />
            <Form form={form} layout="vertical" name="vm-request-wizard">
                {renderStep()}
            </Form>
        </Modal>
    );
}
