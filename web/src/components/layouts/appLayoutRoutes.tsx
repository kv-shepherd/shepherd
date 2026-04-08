'use client';

import React from 'react';
import {
    DashboardIcon,
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

type TranslateFn = (key: string) => string;
export type MenuRouteItem = NonNullable<NonNullable<ProLayoutProps['route']>['routes']>[number];
interface MenuSearchEntry {
    key: string;
    path: string;
    label: string;
    groupLabel?: string;
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
    includeAdmin: boolean
): NonNullable<ProLayoutProps['route']> => {
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
            hideInMenu: true,
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

    if (includeAdmin) {
        routes.push({
            key: 'admin',
            path: '/admin',
            name: t('nav.admin'),
            icon: <AdminIcon />,
            routes: [
                {
                    path: '/admin/approval-tasks',
                    name: t('nav.approval_tasks'),
                    icon: <ApprovalTasksIcon />,
                },
                {
                    path: '/admin/clusters',
                    name: t('nav.clusters'),
                    icon: <ClustersIcon />,
                },
                {
                    path: '/admin/namespaces',
                    name: t('nav.namespaces'),
                    icon: <NamespacesIcon />,
                },
                {
                    path: '/admin/templates',
                    name: t('nav.templates'),
                    icon: <TemplatesIcon />,
                },
                {
                    path: '/admin/instance-sizes',
                    name: t('nav.instance_sizes'),
                    icon: <InstanceSizesIcon />,
                },
                {
                    path: '/admin/users',
                    name: t('nav.users'),
                    icon: <UsersIcon />,
                },
                {
                    path: '/admin/rbac',
                    name: t('nav.rbac'),
                    icon: <RbacIcon />,
                },
                {
                    path: '/admin/rate-limits',
                    name: t('nav.rate_limits'),
                    icon: <RateLimitsIcon />,
                },
                {
                    path: '/admin/auth-providers',
                    name: t('nav.auth_providers'),
                    icon: <AuthProvidersIcon />,
                },
                {
                    path: '/admin/audit',
                    name: t('nav.audit'),
                    icon: <AuditIcon />,
                },
            ],
        });
    }

    return {
        path: '/',
        routes,
    };
};

const normalizeSearchValue = (value: string): string =>
    value.toLowerCase().replace(/[^a-z0-9\u4e00-\u9fa5]+/gi, ' ').trim();

export const flattenMenuRoutes = (routes: MenuRouteItem[] | undefined, parentLabel?: string): MenuSearchEntry[] => {
    if (!routes || routes.length === 0) {
        return [];
    }
    return routes.flatMap((route) => {
        const currentLabel = typeof route.name === 'string' ? route.name : '';
        const currentParentLabel = parentLabel && parentLabel !== currentLabel ? parentLabel : undefined;
        const current: MenuSearchEntry[] = [];

        if (!route.hideInMenu && typeof route.path === 'string' && route.path.trim() !== '' && !route.routes?.length) {
            current.push({
                key: String(route.key || route.path),
                path: route.path,
                label: currentLabel,
                groupLabel: currentParentLabel,
            });
        }

        return current.concat(flattenMenuRoutes(route.routes as MenuRouteItem[] | undefined, currentLabel || currentParentLabel));
    });
};

export const filterMenuSearchEntries = (entries: MenuSearchEntry[], query: string): MenuSearchEntry[] => {
    const normalizedQuery = normalizeSearchValue(query);
    if (normalizedQuery === '') {
        return entries;
    }
    const queryTerms = normalizedQuery.split(/\s+/).filter(Boolean);
    return entries.filter((entry) => {
        const haystack = normalizeSearchValue(
            [entry.label, entry.groupLabel, entry.path]
                .filter(Boolean)
                .join(' '),
        );
        return queryTerms.every((term) => haystack.includes(term));
    });
};
