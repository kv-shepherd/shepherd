import { Form } from 'antd';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import type { FormInstance } from 'antd';
import type { TFunction } from 'i18next';
import { describe, expect, it, vi } from 'vitest';

import type {
    InstanceSize,
    InstanceSizeList,
    ServiceList,
    SystemList,
    Template,
    TemplateList,
    VMRequestMode,
} from '../types';
import { VMRequestWizard } from './VMRequestWizard';

vi.mock('react-i18next', () => ({
    useTranslation: () => ({
        t: (key: string, options?: { defaultValue?: string }) => {
            const labels: Record<string, string> = {
                'wizard.title': 'Create VM Request',
                'wizard.mode.title': 'Request Mode',
                'wizard.mode.guided': 'Guided',
                'wizard.mode.full': 'Full Form',
                'wizard.mode.guided_description': 'Step through the request one decision at a time.',
                'wizard.mode.full_description': 'Review and edit the full request in one form.',
                'wizard.progress_title': 'Progress',
                'wizard.section.service': 'Service Context',
                'wizard.section.template': 'Template',
                'wizard.section.size': 'Instance Size',
                'wizard.section.configuration': 'Configuration',
                'wizard.section.review': 'Review',
                'wizard.select_system': 'Select system',
                'wizard.select_service': 'Select service',
                'wizard.select_template': 'Select template',
                'wizard.select_size': 'Select size',
                'wizard.namespace': 'Namespace',
                'wizard.namespace_hint': 'Namespace hint',
                'wizard.namespace_placeholder': 'Namespace placeholder',
                'wizard.reason': 'Reason',
                'wizard.reason_placeholder': 'Explain why',
                'wizard.batch_count': 'Batch Count',
                'wizard.batch_count_hint': 'Batch hint',
                'wizard.batch_count_required': 'Batch count required',
                'wizard.confirm_note': 'Please review the request before submitting.',
                'wizard.confirm_service': 'Service',
                'wizard.confirm_template': 'Template',
                'wizard.confirm_size': 'Size',
                'wizard.confirm_namespace': 'Namespace',
                'wizard.confirm_reason': 'Reason',
                'wizard.confirm_batch_count': 'Batch Count',
                'wizard.validation.service_required': 'Service is required',
                'wizard.validation.template_required': 'Template is required',
                'wizard.validation.size_required': 'Size is required',
                'wizard.validation.namespace_required': 'Namespace is required',
                'wizard.validation.reason_required': 'Reason is required',
                'common:button.cancel': 'Cancel',
                'common:button.submit': 'Submit',
                'common:button.prev': 'Previous',
                'common:button.next': 'Next',
            };
            return labels[key] ?? options?.defaultValue ?? key;
        },
    }),
}));

function WizardHarness({
    requestMode,
    step = 0,
    targetCpuValue,
    targetMemoryValue,
    targetDiskValue,
    form: providedForm,
}: {
    requestMode: VMRequestMode;
    step?: number;
    targetCpuValue?: number;
    targetMemoryValue?: number;
    targetDiskValue?: number;
    form?: FormInstance;
}) {
    const [internalForm] = Form.useForm();
    const form = providedForm ?? internalForm;
    const t = ((key: string, options?: { defaultValue?: string }) => {
        const labels: Record<string, string> = {
            'wizard.title': 'Create VM Request',
            'wizard.mode.title': 'Request Mode',
            'wizard.mode.guided': 'Guided',
            'wizard.mode.full': 'Full Form',
            'wizard.mode.guided_description': 'Step through the request one decision at a time.',
            'wizard.mode.full_description': 'Review and edit the full request in one form.',
            'wizard.progress_title': 'Progress',
            'wizard.section.service': 'Service Context',
            'wizard.section.template': 'Template',
            'wizard.section.size': 'Instance Size',
            'wizard.section.configuration': 'Configuration',
            'wizard.section.review': 'Review',
            'wizard.select_system': 'Select system',
            'wizard.select_service': 'Select service',
            'wizard.select_template': 'Select template',
                'wizard.select_size': 'Select size',
                'wizard.custom_resources_title': 'Optional Resource Adjustment',
                'wizard.custom_resources_hint': 'Enable this to customize CPU, memory, or disk before submitting.',
                'wizard.custom_resources_active': 'Custom resource overrides are active.',
                'wizard.custom_resource_default': 'Default: {{value}}',
                'modify.target_cpu': 'Target CPU',
                'modify.target_memory': 'Target Memory',
                'modify.target_disk': 'Target Disk',
                'wizard.size_summary': '{{cpu}} vCPU · {{memory}}',
                'wizard.size_disk_suffix': '{{disk}} Gi disk',
                'wizard.namespace': 'Namespace',
            'wizard.namespace_hint': 'Namespace hint',
            'wizard.namespace_placeholder': 'Namespace placeholder',
            'wizard.reason': 'Reason',
            'wizard.reason_placeholder': 'Explain why',
            'wizard.batch_count': 'Batch Count',
            'wizard.batch_count_hint': 'Batch hint',
            'wizard.batch_count_required': 'Batch count required',
            'wizard.confirm_note': 'Please review the request before submitting.',
            'wizard.confirm_service': 'Service',
            'wizard.confirm_template': 'Template',
            'wizard.confirm_size': 'Size',
            'wizard.confirm_namespace': 'Namespace',
            'wizard.confirm_reason': 'Reason',
            'wizard.confirm_batch_count': 'Batch Count',
            'wizard.validation.service_required': 'Service is required',
            'wizard.validation.template_required': 'Template is required',
            'wizard.validation.size_required': 'Size is required',
            'wizard.validation.namespace_required': 'Namespace is required',
            'wizard.validation.reason_required': 'Reason is required',
            'common:button.cancel': 'Cancel',
            'common:button.submit': 'Submit',
            'common:button.prev': 'Previous',
            'common:button.next': 'Next',
        };
        return labels[key] ?? options?.defaultValue ?? key;
    }) as unknown as TFunction;
    const systemsData = { items: [{ id: 'sys-1', name: 'System A' }] } as unknown as SystemList;
    const servicesData = { items: [{ id: 'svc-1', name: 'Service A' }] } as unknown as ServiceList;
    const templatesData = { items: [{ id: 'tpl-1', name: 'ubuntu', display_name: 'Ubuntu' }] } as unknown as TemplateList;
    const sizesData = {
        items: [{ id: 'size-1', name: 'small', display_name: 'Small', cpu_cores: 2, memory_gi: 4, enabled: true }],
    } as unknown as InstanceSizeList;
    const selectedTemplate = {
        id: 'tpl-1',
        name: 'ubuntu',
        display_name: 'Ubuntu',
    } as unknown as Template;
    const selectedSize = {
        id: 'size-1',
        name: 'small',
        display_name: 'Small',
        cpu_cores: 2,
        memory_gi: 4,
        enabled: true,
    } as unknown as InstanceSize;

    return (
        <VMRequestWizard
            t={t}
            open={true}
            step={step}
            setStep={vi.fn()}
            requestMode={requestMode}
            onRequestModeChange={vi.fn()}
            form={form}
            wizardSteps={[
                { title: 'Service Context' },
                { title: 'Template' },
                { title: 'Instance Size' },
                { title: 'Configuration' },
                { title: 'Review' },
            ]}
            selectedSystemId="sys-1"
            onSystemChange={vi.fn()}
            systemsData={systemsData}
            servicesData={servicesData}
            templatesData={templatesData}
            sizesData={sizesData}
            selectedTemplate={selectedTemplate}
            selectedSize={selectedSize}
            placementHint={undefined}
            placementHintLoading={false}
            serviceIdValue="svc-1"
            namespaceValue="team-prod"
            namespaceOptions={['team-prod']}
            reasonValue="Need capacity"
            batchCountValue={1}
            targetCpuValue={targetCpuValue}
            targetMemoryValue={targetMemoryValue}
            targetDiskValue={targetDiskValue}
            isSubmitting={false}
            onCancel={vi.fn()}
            onNext={vi.fn()}
            onSubmit={vi.fn()}
        />
    );
}

describe('VMRequestWizard', () => {
    it('renders full-form mode as a stack of section cards', () => {
        render(<WizardHarness requestMode="full" />);

        expect(screen.getByTestId('vm-request-wizard-modal')).toBeVisible();
        expect(screen.getAllByText('Request Mode').length).toBeGreaterThan(0);
        expect(screen.getAllByText('Service Context').length).toBeGreaterThan(0);
        expect(screen.getAllByText('Template').length).toBeGreaterThan(0);
        expect(screen.getAllByText('Instance Size').length).toBeGreaterThan(0);
        expect(screen.getAllByText('Configuration').length).toBeGreaterThan(0);
        expect(screen.getAllByText('Review').length).toBeGreaterThan(0);
    });

    it('renders guided mode with progress and the current step card', () => {
        render(<WizardHarness requestMode="guided" step={2} />);

        expect(screen.getAllByText('Progress').length).toBeGreaterThan(0);
        expect(screen.getAllByText('Instance Size').length).toBeGreaterThan(0);
        expect(screen.getAllByText('Next').length).toBeGreaterThan(0);
    });

    it('keeps optional resource adjustment expanded when target overrides already exist', () => {
        render(
            <WizardHarness
                requestMode="guided"
                step={2}
                targetCpuValue={3}
                targetMemoryValue={6}
                targetDiskValue={80}
            />,
        );

        expect(screen.getByRole('checkbox', { name: 'Optional Resource Adjustment' })).toBeChecked();
        expect(screen.getByTitle('Target CPU')).toBeInTheDocument();
        expect(screen.getByTitle('Target Memory')).toBeInTheDocument();
        expect(screen.getByTitle('Target Disk')).toBeInTheDocument();
    });

    it('persists optional resource adjustment across guided-step remounts', async () => {
        const user = userEvent.setup();

        function PersistentWizardHarness({ step }: { step: number }) {
            const [form] = Form.useForm();
            return <WizardHarness requestMode="guided" step={step} form={form} />;
        }

        const { rerender } = render(<PersistentWizardHarness step={2} />);

        const checkbox = screen.getByRole('checkbox', { name: 'Optional Resource Adjustment' });
        await user.click(checkbox);
        expect(checkbox).toBeChecked();

        rerender(<PersistentWizardHarness step={3} />);
        rerender(<PersistentWizardHarness step={2} />);

        expect(
            screen.getByRole('checkbox', { name: 'Optional Resource Adjustment' }),
        ).toBeChecked();
    });
});
