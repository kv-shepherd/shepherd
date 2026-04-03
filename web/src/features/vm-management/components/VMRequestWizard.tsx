'use client';

import {
    Alert,
    Button,
    Form,
    Modal,
    Segmented,
    Space,
    Steps,
    Typography,
    type FormInstance,
} from 'antd';
import type { TFunction } from 'i18next';

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
import {
    VMRequestConfigurationFields,
    VMRequestConfirmStep,
    VMRequestSectionCard,
    VMRequestServiceFields,
    VMRequestSizeFields,
    VMRequestTemplateFields,
} from './VMRequestWizardSections';

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
    const renderFullForm = () => (
        <Space direction="vertical" size={24} style={{ width: '100%' }}>
            <Alert
                type="info"
                showIcon
                message={t('wizard.mode.full_description')}
            />
            <VMRequestSectionCard title={t('wizard.section.service')}>
                <VMRequestServiceFields
                    t={t}
                    selectedSystemId={selectedSystemId}
                    onSystemChange={onSystemChange}
                    systemsData={systemsData}
                    servicesData={servicesData}
                />
            </VMRequestSectionCard>
            <VMRequestSectionCard title={t('wizard.section.template')}>
                <VMRequestTemplateFields
                    t={t}
                    templatesData={templatesData}
                />
            </VMRequestSectionCard>
            <VMRequestSectionCard title={t('wizard.section.size')}>
                <VMRequestSizeFields
                    t={t}
                    sizesData={sizesData}
                    selectedSize={selectedSize}
                />
            </VMRequestSectionCard>
            <VMRequestSectionCard title={t('wizard.section.configuration')}>
                <VMRequestConfigurationFields
                    t={t}
                    selectedTemplate={selectedTemplate}
                    selectedSize={selectedSize}
                    placementHint={placementHint}
                    placementHintLoading={placementHintLoading}
                    namespaceValue={namespaceValue}
                    namespaceOptions={namespaceOptions}
                />
            </VMRequestSectionCard>
            <VMRequestSectionCard title={t('wizard.section.review')}>
                <VMRequestConfirmStep
                    t={t}
                    servicesData={servicesData}
                    selectedTemplate={selectedTemplate}
                    selectedSize={selectedSize}
                    serviceIdValue={serviceIdValue}
                    namespaceValue={namespaceValue}
                    reasonValue={reasonValue}
                    batchCountValue={batchCountValue}
                />
            </VMRequestSectionCard>
        </Space>
    );

    const renderStep = () => {
        switch (step) {
            case 0:
                return (
                    <VMRequestServiceFields
                        t={t}
                        selectedSystemId={selectedSystemId}
                        onSystemChange={onSystemChange}
                        systemsData={systemsData}
                        servicesData={servicesData}
                    />
                );
            case 1:
                return <VMRequestTemplateFields t={t} templatesData={templatesData} />;
            case 2:
                return (
                    <VMRequestSizeFields
                        t={t}
                        sizesData={sizesData}
                        selectedSize={selectedSize}
                    />
                );
            case 3:
                return (
                    <VMRequestConfigurationFields
                        t={t}
                        selectedTemplate={selectedTemplate}
                        selectedSize={selectedSize}
                        placementHint={placementHint}
                        placementHintLoading={placementHintLoading}
                        namespaceValue={namespaceValue}
                        namespaceOptions={namespaceOptions}
                    />
                );
            case 4:
                return (
                    <VMRequestConfirmStep
                        t={t}
                        servicesData={servicesData}
                        selectedTemplate={selectedTemplate}
                        selectedSize={selectedSize}
                        serviceIdValue={serviceIdValue}
                        namespaceValue={namespaceValue}
                        reasonValue={reasonValue}
                        batchCountValue={batchCountValue}
                    />
                );
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

    return open ? (
        <Modal
            title={(
                <Space direction="vertical" size={2}>
                    <span>{t('wizard.title')}</span>
                    <Text type="secondary">
                        {requestMode === 'guided' ? t('wizard.mode.guided_description') : t('wizard.mode.full_description')}
                    </Text>
                </Space>
            )}
            open={true}
            onCancel={onCancel}
            width={720}
            footer={footer}
            styles={{ body: { paddingTop: 8 } }}
            data-testid="vm-request-wizard-modal"
        >
            <Space direction="vertical" size={16} style={{ width: '100%' }}>
                <VMRequestSectionCard
                    title={t('wizard.mode.title')}
                    extra={(
                        <Text type="secondary">
                            {requestMode === 'guided' ? t('wizard.mode.guided_description') : t('wizard.mode.full_description')}
                        </Text>
                    )}
                >
                    <Segmented
                        block
                        value={requestMode}
                        onChange={(value) => onRequestModeChange(value as VMRequestMode)}
                        options={[
                            { label: t('wizard.mode.guided'), value: 'guided' },
                            { label: t('wizard.mode.full'), value: 'full' },
                        ]}
                    />
                </VMRequestSectionCard>
                {requestMode === 'guided' && (
                    <VMRequestSectionCard title={t('wizard.progress_title')}>
                        <Steps current={step} items={wizardSteps} size="small" />
                    </VMRequestSectionCard>
                )}
                <Form form={form} layout="vertical" name="vm-request-wizard">
                    {requestMode === 'guided'
                        ? (
                            <VMRequestSectionCard title={wizardSteps[step]?.title ?? t('wizard.title')}>
                                {renderStep()}
                            </VMRequestSectionCard>
                        )
                        : renderFullForm()}
                </Form>
            </Space>
        </Modal>
    ) : null;
}
