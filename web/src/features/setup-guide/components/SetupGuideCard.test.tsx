import { fireEvent, render, screen } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

const pushMock = vi.fn();
const useSetupGuideMock = vi.fn();

vi.mock('next/navigation', () => ({
    useRouter: () => ({
        push: pushMock,
    }),
}));

vi.mock('react-i18next', () => ({
    useTranslation: () => ({
        t: (key: string, values?: Record<string, string>) => {
            const labels: Record<string, string> = {
                'common:setup.card.title': 'First-time setup guide',
                'common:setup.card.description': 'Prepare the first system, service, and VM request path before handing the workflow to regular users.',
                'common:setup.tag': 'Setup Guide',
                'common:setup.step.system.title': 'Create the first system',
                'common:setup.step.system.ready': 'System ready',
                'common:setup.step.system.missing_admin': 'Create the first system now.',
                'common:setup.step.system.missing_user': 'Admin must create a system.',
                'common:setup.step.service.title': 'Create the first service',
                'common:setup.step.service.ready': 'Service ready',
                'common:setup.step.service.wait_for_system': 'Create a system first.',
                'common:setup.step.service.missing_admin': 'Create the first service now.',
                'common:setup.step.service.missing_user': 'Admin must create a service.',
                'common:setup.step.prerequisites.title': 'Prepare VM request resources',
                'common:setup.step.prerequisites.ready': 'Prerequisites ready',
                'common:setup.step.prerequisites.wait_for_service': 'Create a service first.',
                'common:setup.step.prerequisites.missing_admin': `Still required: ${values?.items ?? ''}`,
                'common:setup.step.prerequisites.missing_user': `Admin still needs to prepare: ${values?.items ?? ''}`,
                'common:setup.step.request.title': 'Submit the first VM request',
                'common:setup.step.request.complete': 'Already requested',
                'common:setup.step.request.ready': 'Request path ready',
                'common:setup.step.request.ready_readonly': 'Ready but readonly',
                'common:setup.step.request.wait': 'Finish earlier steps first',
                'common:setup.action.create_system': 'Create system',
                'common:setup.action.create_service': 'Create service',
                'common:setup.action.create_namespace': 'Create namespace',
                'common:setup.action.create_template': 'Create template',
                'common:setup.action.create_instance_size': 'Create instance size',
                'common:setup.action.open_vm_request': 'Open VM request',
                'common:setup.action.open_systems': 'Open systems',
                'common:setup.action.open_services': 'Open services',
                'common:setup.missing.namespace': 'namespace',
                'common:setup.missing.template': 'template',
                'common:setup.missing.instance_size': 'instance size',
                'common:setup.resume.eyebrow': 'Recommended next step',
                'common:setup.resume.hint': 'Use the primary action to continue onboarding. The checklist below will stay in sync as each step is completed.',
                'common:setup.resume.title': 'Continue setup',
                'common:setup.quick_actions_title': 'Quick launchers',
                'common:setup.quick_actions_description': 'Initial setup is complete. Keep these entry points nearby for repeat setup and day-one operator workflows.',
                'common:setup.quick_actions_ready': 'The first-use path is ready. Jump straight into the next workflow.',
                'common:setup.quick_actions_hint': 'Use these shortcuts to start a new system, service, resource, or VM workflow without reopening the full checklist.',
                'common:setup.resume.create-namespace': 'Service created. Next, prepare the first namespace from the dashboard guide.',
                'common:setup.resume.create-instance-size': 'Template created. Next, prepare the first instance size from the dashboard guide.',
            };
            return labels[key] ?? key;
        },
    }),
}));

vi.mock('../hooks/useSetupGuide', () => ({
    useSetupGuide: (...args: unknown[]) => useSetupGuideMock(...args),
}));

import { SetupGuideCard } from './SetupGuideCard';

describe('SetupGuideCard', () => {
    beforeEach(() => {
        pushMock.mockReset();
        useSetupGuideMock.mockReset();
    });

    it('renders admin setup actions for missing VM prerequisites', () => {
        useSetupGuideMock.mockReturnValue({
            systemsTotal: 1,
            servicesTotal: 1,
            vmsTotal: 0,
            namespacesTotal: 0,
            templatesTotal: 0,
            instanceSizesTotal: 1,
            canCreateSystem: true,
            canCreateService: true,
            canCreateVM: true,
            canManageNamespaces: true,
            canManageTemplates: true,
            canManageInstanceSizes: true,
            isPlatformAdmin: true,
            systemReady: true,
            serviceReady: true,
            prerequisitesReady: false,
            vmRequestReady: false,
            hasRequestedFirstVM: false,
            isLoading: false,
        });

        render(<SetupGuideCard variant="dashboard" />);

        expect(screen.getByText('First-time setup guide')).toBeVisible();
        expect(screen.getByText('Prepare VM request resources')).toBeVisible();
        expect(screen.getByText('Recommended next step')).toBeVisible();
        expect(screen.getByText('Still required: namespace, template')).toBeVisible();
        expect(screen.getByText('Service created. Next, prepare the first namespace from the dashboard guide.')).toBeVisible();
        expect(screen.getAllByRole('button', { name: 'Create namespace' }).length).toBeGreaterThan(0);
        expect(screen.getByRole('button', { name: 'Create template' })).toBeVisible();

        fireEvent.click(screen.getAllByRole('button', { name: 'Create namespace' })[0]);
        expect(pushMock).toHaveBeenCalledWith('/admin/namespaces?intent=create-namespace');
    });

    it('shows an inline action for the current dashboard step when the first system is missing', () => {
        useSetupGuideMock.mockReturnValue({
            systemsTotal: 0,
            servicesTotal: 0,
            vmsTotal: 0,
            namespacesTotal: 0,
            templatesTotal: 0,
            instanceSizesTotal: 0,
            canCreateSystem: true,
            canCreateService: true,
            canCreateVM: true,
            canManageNamespaces: true,
            canManageTemplates: true,
            canManageInstanceSizes: true,
            isPlatformAdmin: true,
            systemReady: false,
            serviceReady: false,
            prerequisitesReady: false,
            vmRequestReady: false,
            hasRequestedFirstVM: false,
            isLoading: false,
        });

        render(<SetupGuideCard variant="dashboard" />);

        expect(screen.getByText('Create the first system')).toBeVisible();
        expect(screen.getAllByRole('button', { name: 'Create system' }).length).toBeGreaterThan(0);

        fireEvent.click(screen.getAllByRole('button', { name: 'Create system' })[0]);
        expect(pushMock).toHaveBeenCalledWith('/systems?intent=create-system');
    });

    it('renders a focused dashboard CTA that resumes setup from the recommended page', () => {
        useSetupGuideMock.mockReturnValue({
            systemsTotal: 1,
            servicesTotal: 1,
            vmsTotal: 0,
            namespacesTotal: 1,
            templatesTotal: 1,
            instanceSizesTotal: 0,
            canCreateSystem: true,
            canCreateService: true,
            canCreateVM: true,
            canManageNamespaces: true,
            canManageTemplates: true,
            canManageInstanceSizes: true,
            isPlatformAdmin: true,
            systemReady: true,
            serviceReady: true,
            prerequisitesReady: false,
            vmRequestReady: false,
            hasRequestedFirstVM: false,
            isLoading: false,
        });

        render(<SetupGuideCard variant="dashboard" focusAction="create-instance-size" />);

        expect(screen.getByText('Recommended next step')).toBeVisible();
        expect(screen.getAllByRole('button', { name: 'Create instance size' }).length).toBeGreaterThan(0);
        expect(screen.getByText('Template created. Next, prepare the first instance size from the dashboard guide.')).toBeVisible();

        fireEvent.click(screen.getAllByRole('button', { name: 'Create instance size' })[0]);
        expect(pushMock).toHaveBeenCalledWith('/admin/instance-sizes?intent=create-instance-size');
    });

    it('keeps the dashboard guide as quick launchers after initial setup is complete', () => {
        useSetupGuideMock.mockReturnValue({
            systemsTotal: 2,
            servicesTotal: 4,
            vmsTotal: 3,
            namespacesTotal: 2,
            templatesTotal: 2,
            instanceSizesTotal: 2,
            canCreateSystem: true,
            canCreateService: true,
            canCreateVM: true,
            canManageNamespaces: true,
            canManageTemplates: true,
            canManageInstanceSizes: true,
            isPlatformAdmin: true,
            systemReady: true,
            serviceReady: true,
            prerequisitesReady: true,
            vmRequestReady: true,
            hasRequestedFirstVM: true,
            isLoading: false,
        });

        render(<SetupGuideCard variant="dashboard" />);

        expect(screen.getByText('Quick launchers')).toBeVisible();
        expect(screen.getByText('The first-use path is ready. Jump straight into the next workflow.')).toBeVisible();
        expect(screen.queryByText('Prepare VM request resources')).not.toBeInTheDocument();
        expect(screen.getByRole('button', { name: 'Create system' })).toBeVisible();
        expect(screen.getByRole('button', { name: 'Create service' })).toBeVisible();
        expect(screen.getByRole('button', { name: 'Create namespace' })).toBeVisible();
        expect(screen.getByRole('button', { name: 'Create template' })).toBeVisible();
        expect(screen.getByRole('button', { name: 'Create instance size' })).toBeVisible();
        expect(screen.getByRole('button', { name: 'Open VM request' })).toBeVisible();
    });

    it('limits quick launchers for non-platform-admin users after setup completes', () => {
        useSetupGuideMock.mockReturnValue({
            systemsTotal: 2,
            servicesTotal: 4,
            vmsTotal: 3,
            namespacesTotal: 2,
            templatesTotal: 2,
            instanceSizesTotal: 2,
            canCreateSystem: true,
            canCreateService: true,
            canCreateVM: true,
            canManageNamespaces: true,
            canManageTemplates: true,
            canManageInstanceSizes: true,
            isPlatformAdmin: false,
            systemReady: true,
            serviceReady: true,
            prerequisitesReady: true,
            vmRequestReady: true,
            hasRequestedFirstVM: true,
            isLoading: false,
        });

        render(<SetupGuideCard variant="dashboard" />);

        expect(screen.getByRole('button', { name: 'Create system' })).toBeVisible();
        expect(screen.getByRole('button', { name: 'Create service' })).toBeVisible();
        expect(screen.getByRole('button', { name: 'Open VM request' })).toBeVisible();
        expect(screen.queryByRole('button', { name: 'Create namespace' })).not.toBeInTheDocument();
        expect(screen.queryByRole('button', { name: 'Create template' })).not.toBeInTheDocument();
        expect(screen.queryByRole('button', { name: 'Create instance size' })).not.toBeInTheDocument();
    });
});
