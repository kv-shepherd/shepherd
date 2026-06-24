'use client';

import React from 'react';
import {
    DashboardIcon,
    ObservabilityIcon,
    NotificationsIcon,
    SystemsIcon,
    ServicesIcon,
    VMsIcon,
    RequestsIcon,
    AdminIcon,
    ApprovalTasksIcon,
    ClustersIcon,
    NamespacesIcon,
    TemplatesIcon,
    InstanceSizesIcon,
    UsersIcon,
    RbacIcon,
    RateLimitsIcon,
    AuthProvidersIcon,
    AuditIcon,
} from './MenuIcons';
import type { ProLayoutProps } from '@ant-design/pro-components';
import {
    ADMIN_MENU_ROUTE_PERMISSIONS,
    type AdminMenuRouteKey,
} from '@/lib/auth/permissions';

type TranslateFn = (key: string) => string;
export type MenuRouteItem = NonNullable<NonNullable<ProLayoutProps['route']>['routes']>[number];
type MenuPermissionChecker = boolean | ((permissions: readonly string[]) => boolean);

interface AdminMenuRouteDefinition extends MenuRouteItem {
    routeKey: AdminMenuRouteKey;
    requiredPermissions: readonly string[];
}

interface MenuRouteOptions {
    flattenAdmin?: boolean;
}

export const resolveMenuHref = (item: {
    path?: string;
    routes?: Array<{ path?: string }>;
}): string | undefined => {
    if (item.routes?.length) {
        return item.routes.find((child) => typeof child.path === 'string' && child.path.trim() !== '')?.path;
    }
    return item.path;
};

export const getMenuRoutes = (
    t: TranslateFn,
    includeAdmin: MenuPermissionChecker,
    options: MenuRouteOptions = {},
): NonNullable<ProLayoutProps['route']> => {
    const canAccessMenuItem =
        typeof includeAdmin === 'function'
            ? includeAdmin
            : () => includeAdmin;
    const routes: NonNullable<ProLayoutProps['route']>['routes'] = [
        {
            path: '/dashboard',
            name: t('nav.dashboard'),
            icon: <DashboardIcon />,
        },
        {
            path: '/notifications',
            name: t('nav.notifications'),
            icon: <NotificationsIcon />,
        },
        {
            path: '/systems',
            name: t('nav.systems'),
            icon: <SystemsIcon />,
        },
        {
            path: '/services',
            name: t('nav.services'),
            icon: <ServicesIcon />,
        },
        {
            path: '/vms',
            name: t('nav.vms'),
            icon: <VMsIcon />,
        },
        {
            path: '/tickets',
            name: t('nav.my_requests'),
            icon: <RequestsIcon />,
        },
    ];

    const adminRouteDefinitions: AdminMenuRouteDefinition[] = [
        {
            routeKey: 'approvalTasks',
            requiredPermissions: ADMIN_MENU_ROUTE_PERMISSIONS.approvalTasks,
            path: '/admin/approval-tasks',
            name: t('nav.approval_tasks'),
            icon: <ApprovalTasksIcon />,
        },
        {
            routeKey: 'externalApprovalSystems',
            requiredPermissions: ADMIN_MENU_ROUTE_PERMISSIONS.externalApprovalSystems,
            path: '/admin/external-approval-systems',
            name: t('nav.external_approval_systems'),
            icon: <ApprovalTasksIcon />,
        },
        {
            routeKey: 'clusters',
            requiredPermissions: ADMIN_MENU_ROUTE_PERMISSIONS.clusters,
            path: '/admin/clusters',
            name: t('nav.clusters'),
            icon: <ClustersIcon />,
        },
        {
            routeKey: 'pendingAdoptions',
            requiredPermissions: ADMIN_MENU_ROUTE_PERMISSIONS.pendingAdoptions,
            path: '/admin/pending-adoptions',
            name: t('nav.pending_adoptions'),
            icon: <ApprovalTasksIcon />,
        },
        {
            routeKey: 'namespaces',
            requiredPermissions: ADMIN_MENU_ROUTE_PERMISSIONS.namespaces,
            path: '/admin/namespaces',
            name: t('nav.namespaces'),
            icon: <NamespacesIcon />,
        },
        {
            routeKey: 'templates',
            requiredPermissions: ADMIN_MENU_ROUTE_PERMISSIONS.templates,
            path: '/admin/templates',
            name: t('nav.templates'),
            icon: <TemplatesIcon />,
        },
        {
            routeKey: 'instanceSizes',
            requiredPermissions: ADMIN_MENU_ROUTE_PERMISSIONS.instanceSizes,
            path: '/admin/instance-sizes',
            name: t('nav.instance_sizes'),
            icon: <InstanceSizesIcon />,
        },
        {
            routeKey: 'users',
            requiredPermissions: ADMIN_MENU_ROUTE_PERMISSIONS.users,
            path: '/admin/users',
            name: t('nav.users'),
            icon: <UsersIcon />,
        },
        {
            routeKey: 'rbac',
            requiredPermissions: ADMIN_MENU_ROUTE_PERMISSIONS.rbac,
            path: '/admin/rbac',
            name: t('nav.rbac'),
            icon: <RbacIcon />,
        },
        {
            routeKey: 'rateLimits',
            requiredPermissions: ADMIN_MENU_ROUTE_PERMISSIONS.rateLimits,
            path: '/admin/rate-limits',
            name: t('nav.rate_limits'),
            icon: <RateLimitsIcon />,
        },
        {
            routeKey: 'authProviders',
            requiredPermissions: ADMIN_MENU_ROUTE_PERMISSIONS.authProviders,
            path: '/admin/auth-providers',
            name: t('nav.auth_providers'),
            icon: <AuthProvidersIcon />,
        },
        {
            routeKey: 'observability',
            requiredPermissions: ADMIN_MENU_ROUTE_PERMISSIONS.observability,
            path: '/admin/observability',
            name: t('nav.observability'),
            icon: <ObservabilityIcon />,
        },
        {
            routeKey: 'audit',
            requiredPermissions: ADMIN_MENU_ROUTE_PERMISSIONS.audit,
            path: '/admin/audit',
            name: t('nav.audit'),
            icon: <AuditIcon />,
        },
    ];

    const adminRoutes: MenuRouteItem[] = adminRouteDefinitions.filter((route) =>
        canAccessMenuItem(route.requiredPermissions),
    ).map((route) => ({
        path: route.path,
        name: route.name,
        icon: route.icon,
    }));

    if (adminRoutes.length > 0 && options.flattenAdmin) {
        routes.push(...adminRoutes);
    } else if (adminRoutes.length > 0) {
        routes.push({
            key: 'admin',
            path: '/admin',
            name: t('nav.admin'),
            icon: <AdminIcon />,
            routes: adminRoutes,
        });
    }

    return {
        path: '/',
        routes,
    };
};
