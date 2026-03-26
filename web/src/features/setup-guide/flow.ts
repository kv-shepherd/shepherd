import type { SetupGuideState } from './hooks/useSetupGuide';

type SetupCompletionStep =
    | 'system'
    | 'service'
    | 'namespace'
    | 'template'
    | 'instance-size';

export type SetupResumeAction =
    | 'create-system'
    | 'create-service'
    | 'create-namespace'
    | 'create-template'
    | 'create-instance-size'
    | 'open-vm-request';

function incrementIfCompleted(
    current: number,
    completedStep: SetupCompletionStep,
    expectedStep: SetupCompletionStep,
): number {
    return current + (completedStep === expectedStep ? 1 : 0);
}

function resolveNextSetupHref(
    setup: SetupGuideState,
    completedStep: SetupCompletionStep,
): string | null {
    const systemsTotal = incrementIfCompleted(setup.systemsTotal, completedStep, 'system');
    const servicesTotal = incrementIfCompleted(setup.servicesTotal, completedStep, 'service');
    const namespacesTotal = incrementIfCompleted(setup.namespacesTotal, completedStep, 'namespace');
    const templatesTotal = incrementIfCompleted(setup.templatesTotal, completedStep, 'template');
    const instanceSizesTotal = incrementIfCompleted(
        setup.instanceSizesTotal,
        completedStep,
        'instance-size',
    );

    if (systemsTotal === 0) {
        return setup.canCreateSystem
            ? '/systems?intent=create-system'
            : '/systems';
    }

    if (servicesTotal === 0) {
        return setup.canCreateService
            ? '/services?intent=create-service'
            : '/services';
    }

    if (namespacesTotal === 0) {
        return setup.canManageNamespaces
            ? '/admin/namespaces?intent=create-namespace'
            : '/admin/namespaces';
    }

    if (templatesTotal === 0) {
        return setup.canManageTemplates
            ? '/admin/templates?intent=create-template'
            : '/admin/templates';
    }

    if (instanceSizesTotal === 0) {
        return setup.canManageInstanceSizes
            ? '/admin/instance-sizes?intent=create-instance-size'
            : '/admin/instance-sizes';
    }

    if (setup.canCreateVM) {
        return '/vms?request=create';
    }

    return null;
}

export function resolveNextSetupAction(
    setup: SetupGuideState,
    completedStep: SetupCompletionStep,
): SetupResumeAction | null {
    const nextHref = resolveNextSetupHref(setup, completedStep);
    if (!nextHref) {
        return null;
    }

    if (nextHref.startsWith('/systems')) {
        return 'create-system';
    }
    if (nextHref.startsWith('/services')) {
        return 'create-service';
    }
    if (nextHref.startsWith('/admin/namespaces')) {
        return 'create-namespace';
    }
    if (nextHref.startsWith('/admin/templates')) {
        return 'create-template';
    }
    if (nextHref.startsWith('/admin/instance-sizes')) {
        return 'create-instance-size';
    }
    if (nextHref.startsWith('/vms')) {
        return 'open-vm-request';
    }

    return null;
}

export function buildDashboardSetupResumeHref(
    action: SetupResumeAction,
): string {
    const params = new URLSearchParams({ setup_action: action });
    return `/dashboard?${params.toString()}`;
}

export function buildSetupActionHref(
    action: SetupResumeAction,
): string {
    switch (action) {
        case 'create-system':
            return '/systems?intent=create-system';
        case 'create-service':
            return '/services?intent=create-service';
        case 'create-namespace':
            return '/admin/namespaces?intent=create-namespace';
        case 'create-template':
            return '/admin/templates?intent=create-template';
        case 'create-instance-size':
            return '/admin/instance-sizes?intent=create-instance-size';
        case 'open-vm-request':
            return '/vms?request=create';
        default:
            return '/dashboard';
    }
}
