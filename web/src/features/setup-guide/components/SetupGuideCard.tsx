'use client';

import { Button, Empty, Space, Steps, Tag, Typography } from 'antd';
import type { ReactNode } from 'react';
import { useMemo } from 'react';
import { useRouter } from 'next/navigation';
import { useTranslation } from 'react-i18next';

import { PageSurface } from '@/components/layouts/PageSection';
import {
    buildSetupActionHref,
    type SetupResumeAction,
} from '../flow';
import { useSetupGuide } from '../hooks/useSetupGuide';

const { Text } = Typography;

type SetupGuideVariant = 'dashboard' | 'systems' | 'services' | 'vm';

interface SetupGuideCardProps {
    variant: SetupGuideVariant;
    focusAction?: SetupResumeAction | null;
    surface?: boolean;
    snapshot?: {
        systemsTotal?: number;
        servicesTotal?: number;
        vmsTotal?: number;
    };
}

interface SetupAction {
    key: string;
    href: string;
    label: string;
}

interface StepViewModel {
    key: string;
    title: string;
    status: 'wait' | 'process' | 'finish';
    description: ReactNode;
}

function joinMissingItems(items: string[]): string {
    return items.join(', ');
}

export function SetupGuideCard({
    focusAction = null,
    variant,
    surface = true,
    snapshot,
}: SetupGuideCardProps) {
    const { t } = useTranslation(['common', 'vm']);
    const router = useRouter();
    const setup = useSetupGuide(snapshot);

    const missingPrerequisites = useMemo(() => {
        const items: string[] = [];
        if (setup.namespacesTotal === 0) {
            items.push(t('common:setup.missing.namespace'));
        }
        if (setup.templatesTotal === 0) {
            items.push(t('common:setup.missing.template'));
        }
        if (setup.instanceSizesTotal === 0) {
            items.push(t('common:setup.missing.instance_size'));
        }
        return items;
    }, [setup.instanceSizesTotal, setup.namespacesTotal, setup.templatesTotal, t]);

    const missingPrerequisitesLabel = joinMissingItems(missingPrerequisites);

    const shouldShowStepActions = (status: StepViewModel['status']) => (
        variant === 'dashboard' ? status === 'process' : true
    );

    const systemActions = useMemo<SetupAction[]>(() => (
        setup.canCreateSystem
            ? [{ key: 'create-system', href: '/systems?intent=create-system', label: t('common:setup.action.create_system') }]
            : [{ key: 'open-systems', href: '/systems', label: t('common:setup.action.open_systems') }]
    ), [setup.canCreateSystem, t]);

    const serviceActions = useMemo<SetupAction[]>(() => (
        !setup.systemReady
            ? systemActions
            : setup.canCreateService
                ? [{ key: 'create-service', href: '/services?intent=create-service', label: t('common:setup.action.create_service') }]
                : [{ key: 'open-services', href: '/services', label: t('common:setup.action.open_services') }]
    ), [setup.canCreateService, setup.systemReady, systemActions, t]);

    const prerequisiteActions = useMemo<SetupAction[]>(() => {
        const actions: SetupAction[] = [];
        if (setup.namespacesTotal === 0 && setup.canManageNamespaces) {
            actions.push({
                key: 'create-namespace',
                href: '/admin/namespaces?intent=create-namespace',
                label: t('common:setup.action.create_namespace'),
            });
        }
        if (setup.templatesTotal === 0 && setup.canManageTemplates) {
            actions.push({
                key: 'create-template',
                href: '/admin/templates?intent=create-template',
                label: t('common:setup.action.create_template'),
            });
        }
        if (setup.instanceSizesTotal === 0 && setup.canManageInstanceSizes) {
            actions.push({
                key: 'create-instance-size',
                href: '/admin/instance-sizes?intent=create-instance-size',
                label: t('common:setup.action.create_instance_size'),
            });
        }
        return actions;
    }, [
        setup.canManageInstanceSizes,
        setup.canManageNamespaces,
        setup.canManageTemplates,
        setup.instanceSizesTotal,
        setup.namespacesTotal,
        setup.templatesTotal,
        t,
    ]);

    const requestActions = useMemo<SetupAction[]>(() => (
        setup.vmRequestReady && setup.canCreateVM
            ? [{ key: 'open-vm-request', href: '/vms?request=create', label: t('common:setup.action.open_vm_request') }]
            : []
    ), [setup.canCreateVM, setup.vmRequestReady, t]);
    const isDashboardChecklistActive =
        !setup.systemReady ||
        !setup.serviceReady ||
        !setup.prerequisitesReady ||
        (setup.canCreateVM && !setup.hasRequestedFirstVM);
    const quickActions = useMemo<SetupAction[]>(() => {
        if (variant !== 'dashboard') {
            return [];
        }
        const actions: SetupAction[] = [
            {
                key: setup.canCreateSystem ? 'create-system' : 'open-systems',
                href: setup.canCreateSystem ? '/systems?intent=create-system' : '/systems',
                label: setup.canCreateSystem ? t('common:setup.action.create_system') : t('common:setup.action.open_systems'),
            },
            {
                key: setup.canCreateService ? 'create-service' : 'open-services',
                href: setup.canCreateService ? '/services?intent=create-service' : '/services',
                label: setup.canCreateService ? t('common:setup.action.create_service') : t('common:setup.action.open_services'),
            },
        ];
        if (setup.isPlatformAdmin && setup.canManageNamespaces) {
            actions.push({
                key: 'create-namespace',
                href: '/admin/namespaces?intent=create-namespace',
                label: t('common:setup.action.create_namespace'),
            });
        }
        if (setup.isPlatformAdmin && setup.canManageTemplates) {
            actions.push({
                key: 'create-template',
                href: '/admin/templates?intent=create-template',
                label: t('common:setup.action.create_template'),
            });
        }
        if (setup.isPlatformAdmin && setup.canManageInstanceSizes) {
            actions.push({
                key: 'create-instance-size',
                href: '/admin/instance-sizes?intent=create-instance-size',
                label: t('common:setup.action.create_instance_size'),
            });
        }
        if (setup.canCreateVM) {
            actions.push({
                key: 'open-vm-request',
                href: '/vms?request=create',
                label: t('common:setup.action.open_vm_request'),
            });
        }
        return actions;
    }, [
        setup.canCreateService,
        setup.canCreateSystem,
        setup.canCreateVM,
        setup.canManageInstanceSizes,
        setup.canManageNamespaces,
        setup.canManageTemplates,
        setup.isPlatformAdmin,
        t,
        variant,
    ]);
    const recommendedActionKey = focusAction ?? (
        !setup.systemReady
            ? 'create-system'
            : !setup.serviceReady
                ? 'create-service'
                : setup.namespacesTotal === 0
                    ? 'create-namespace'
                    : setup.templatesTotal === 0
                        ? 'create-template'
                        : setup.instanceSizesTotal === 0
                            ? 'create-instance-size'
                            : !setup.hasRequestedFirstVM && setup.vmRequestReady && setup.canCreateVM
                                ? 'open-vm-request'
                                : null
    );
    const focusMessage = recommendedActionKey
        ? t(`common:setup.resume.${recommendedActionKey}`)
        : null;
    const recommendedAction = useMemo(() => {
        if (!recommendedActionKey) {
            return null;
        }

        const allActions = [
            ...systemActions,
            ...serviceActions,
            ...prerequisiteActions,
            ...requestActions,
        ];
        return allActions.find((action) => action.key === recommendedActionKey) ?? {
            key: recommendedActionKey,
            href: buildSetupActionHref(recommendedActionKey),
            label: t(`common:setup.action.${recommendedActionKey.replaceAll('-', '_')}`),
        };
    }, [
        prerequisiteActions,
        recommendedActionKey,
        requestActions,
        serviceActions,
        systemActions,
        t,
    ]);

    const steps: StepViewModel[] = [
        {
            key: 'system',
            title: t('common:setup.step.system.title'),
            status: setup.systemReady ? 'finish' : 'process',
            description: (
                <Space direction="vertical" size={8}>
                    <Text type="secondary">
                        {setup.systemReady
                            ? t('common:setup.step.system.ready')
                            : setup.canCreateSystem
                                ? t('common:setup.step.system.missing_admin')
                                : t('common:setup.step.system.missing_user')}
                    </Text>
                    {!setup.systemReady && shouldShowStepActions(setup.systemReady ? 'finish' : 'process') ? (
                        <Space wrap>
                            {systemActions.map((action) => (
                                <Button key={action.key} size="small" type="primary" onClick={() => router.push(action.href)}>
                                    {action.label}
                                </Button>
                            ))}
                        </Space>
                    ) : null}
                </Space>
            ),
        },
        {
            key: 'service',
            title: t('common:setup.step.service.title'),
            status: setup.serviceReady ? 'finish' : setup.systemReady ? 'process' : 'wait',
            description: (
                <Space direction="vertical" size={8}>
                    <Text type="secondary">
                        {setup.serviceReady
                            ? t('common:setup.step.service.ready')
                            : setup.systemReady
                                ? setup.canCreateService
                                    ? t('common:setup.step.service.missing_admin')
                                    : t('common:setup.step.service.missing_user')
                                : t('common:setup.step.service.wait_for_system')}
                    </Text>
                    {!setup.serviceReady && shouldShowStepActions(setup.serviceReady ? 'finish' : setup.systemReady ? 'process' : 'wait') ? (
                        <Space wrap>
                            {serviceActions.map((action) => (
                                <Button
                                    key={action.key}
                                    size="small"
                                    type={setup.systemReady ? 'primary' : 'default'}
                                    onClick={() => router.push(action.href)}
                                >
                                    {action.label}
                                </Button>
                            ))}
                        </Space>
                    ) : null}
                </Space>
            ),
        },
        {
            key: 'prerequisites',
            title: t('common:setup.step.prerequisites.title'),
            status: setup.prerequisitesReady ? 'finish' : setup.serviceReady ? 'process' : 'wait',
            description: (
                <Space direction="vertical" size={8}>
                    <Text type="secondary">
                        {setup.prerequisitesReady
                            ? t('common:setup.step.prerequisites.ready')
                            : setup.serviceReady
                                ? prerequisiteActions.length > 0
                                    ? t('common:setup.step.prerequisites.missing_admin', { items: missingPrerequisitesLabel })
                                    : t('common:setup.step.prerequisites.missing_user', { items: missingPrerequisitesLabel })
                                : t('common:setup.step.prerequisites.wait_for_service')}
                    </Text>
                    {!setup.prerequisitesReady && prerequisiteActions.length > 0 && shouldShowStepActions(setup.prerequisitesReady ? 'finish' : setup.serviceReady ? 'process' : 'wait') ? (
                        <Space wrap>
                            {prerequisiteActions.map((action) => (
                                <Button key={action.key} size="small" type="primary" onClick={() => router.push(action.href)}>
                                    {action.label}
                                </Button>
                            ))}
                        </Space>
                    ) : null}
                </Space>
            ),
        },
        {
            key: 'request',
            title: t('common:setup.step.request.title'),
            status: setup.hasRequestedFirstVM
                ? 'finish'
                : setup.vmRequestReady
                    ? setup.canCreateVM
                        ? 'process'
                        : 'finish'
                    : 'wait',
            description: (
                <Space direction="vertical" size={8}>
                    <Text type="secondary">
                        {setup.hasRequestedFirstVM
                            ? t('common:setup.step.request.complete')
                            : setup.vmRequestReady
                                ? setup.canCreateVM
                                    ? t('common:setup.step.request.ready')
                                    : t('common:setup.step.request.ready_readonly')
                                : t('common:setup.step.request.wait')}
                    </Text>
                    {!setup.hasRequestedFirstVM && requestActions.length > 0 && shouldShowStepActions(
                        setup.hasRequestedFirstVM
                            ? 'finish'
                            : setup.vmRequestReady && setup.canCreateVM
                                ? 'process'
                                : setup.vmRequestReady
                                    ? 'finish'
                                    : 'wait'
                    ) ? (
                        <Space wrap>
                            {requestActions.map((action) => (
                                <Button key={action.key} size="small" type="primary" onClick={() => router.push(action.href)}>
                                    {action.label}
                                </Button>
                            ))}
                        </Space>
                    ) : null}
                </Space>
            ),
        },
    ];

    const currentStepIndex = Math.max(0, steps.findIndex((step) => step.status === 'process'));
    const shouldRender =
        variant === 'dashboard'
            ? isDashboardChecklistActive || quickActions.length > 0
            : variant === 'systems'
                ? !setup.systemReady
                : variant === 'services'
                    ? !setup.systemReady || !setup.serviceReady
                    : !setup.vmRequestReady;

    if (setup.isLoading || !shouldRender) {
        return null;
    }

    const dashboardQuickActionsMode = variant === 'dashboard' && !isDashboardChecklistActive;

    const title =
        dashboardQuickActionsMode
            ? t('common:setup.quick_actions_title')
            : variant === 'dashboard'
            ? t('common:setup.card.title')
            : variant === 'systems'
                ? t('common:setup.systems.title')
                : variant === 'services'
                    ? setup.systemReady
                        ? t('common:setup.services.title')
                        : t('common:setup.services.no_system_title')
                    : t('common:setup.vm.title');

    const description =
        dashboardQuickActionsMode
            ? t('common:setup.quick_actions_description')
            : variant === 'dashboard'
            ? t('common:setup.card.description')
            : variant === 'systems'
                ? t('common:setup.systems.description')
                : variant === 'services'
                    ? setup.systemReady
                        ? t('common:setup.services.description')
                        : t('common:setup.services.no_system_description')
                    : t('common:setup.vm.description');

    const setupHeader = (
        <Space direction="vertical" size={6} style={{ width: '100%' }}>
            <Space wrap>
                <Text strong>{title}</Text>
                <Tag color="blue">{t('common:setup.tag')}</Tag>
            </Space>
            <Text type="secondary">{description}</Text>
            {focusMessage && recommendedAction ? (
                <div className="setup-guide-next">
                    <div className="setup-guide-next__meta">
                        <Tag color="blue" className="setup-guide-next__eyebrow">
                            {t('common:setup.resume.eyebrow')}
                        </Tag>
                        <Text strong className="setup-guide-next__title">
                            {recommendedAction.label}
                        </Text>
                        <Text type="secondary">{focusMessage}</Text>
                        <Text type="secondary" className="setup-guide-next__hint">
                            {t('common:setup.resume.hint')}
                        </Text>
                    </div>
                    <div className="setup-guide-next__actions">
                        <Button
                            type="primary"
                            onClick={() => router.push(recommendedAction.href)}
                        >
                            {recommendedAction.label}
                        </Button>
                    </div>
                </div>
            ) : focusMessage ? (
                <Text strong style={{ color: '#1677ff' }}>
                    {focusMessage}
                </Text>
            ) : null}
            {variant === 'vm' && prerequisiteActions.length > 0 ? (
                <Text type="secondary">{t('common:setup.vm.admin_hint')}</Text>
            ) : null}
        </Space>
    );

    const stepsBlock = (
        <Steps
            className="setup-guide-steps"
            direction="vertical"
            size="small"
            current={currentStepIndex}
            items={steps.map((step) => ({
                key: step.key,
                title: step.title,
                status: step.status,
                description: step.description,
            }))}
        />
    );

    const quickActionsBlock = (
        <div className="setup-guide-quick">
            <div className="setup-guide-quick__header">
                <Text strong>{t('common:setup.quick_actions_ready')}</Text>
                <Text type="secondary">{t('common:setup.quick_actions_hint')}</Text>
            </div>
            <div className="setup-guide-quick__actions">
                {quickActions.map((action) => (
                    <Button
                        key={action.key}
                        className="app-shell-action-button"
                        onClick={() => router.push(action.href)}
                    >
                        {action.label}
                    </Button>
                ))}
            </div>
        </div>
    );

    const content = (
        <PageSurface style={{ marginBottom: 24 }}>
            <Space direction="vertical" size={16} style={{ width: '100%' }}>
                {variant !== 'dashboard' ? <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={false} /> : null}
                {setupHeader}
                {dashboardQuickActionsMode ? quickActionsBlock : stepsBlock}
            </Space>
        </PageSurface>
    );

    if (surface) {
        return content;
    }

    return (
        <Space direction="vertical" size={16} style={{ width: '100%' }}>
            {variant !== 'dashboard' ? <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={false} /> : null}
            {setupHeader}
            {dashboardQuickActionsMode ? quickActionsBlock : stepsBlock}
        </Space>
    );
}
