'use client';

import React from 'react';
import {
    AppstoreOutlined,
    AuditOutlined,
    BellOutlined,
    CloudServerOutlined,
    ClusterOutlined,
    DashboardOutlined,
    DesktopOutlined,
    FileTextOutlined,
    GlobalOutlined,
    HddOutlined,
    KeyOutlined,
    ProfileOutlined,
    SafetyCertificateOutlined,
    SettingOutlined,
    TeamOutlined,
} from '@ant-design/icons';
import type { ProLayoutProps } from '@ant-design/pro-components';

type TranslateFn = (key: string) => string;
export type MenuRouteItem = NonNullable<NonNullable<ProLayoutProps['route']>['routes']>[number];

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
            icon: <DashboardOutlined />,
        },
        {
            path: '/notifications',
            name: t('nav.notifications'),
            icon: <BellOutlined />,
            hideInMenu: true,
        },
        {
            path: '/systems',
            name: t('nav.systems'),
            icon: <CloudServerOutlined />,
        },
        {
            path: '/services',
            name: t('nav.services'),
            icon: <AppstoreOutlined />,
        },
        {
            path: '/vms',
            name: t('nav.vms'),
            icon: <DesktopOutlined />,
        },
        {
            path: '/tickets',
            name: t('nav.my_requests'),
            icon: <AuditOutlined />,
        },
    ];

    if (includeAdmin) {
        routes.push({
            key: 'admin',
            path: '/admin',
            name: t('nav.admin'),
            icon: <SettingOutlined />,
            routes: [
                {
                    path: '/admin/approval-tasks',
                    name: t('nav.approval_tasks'),
                    icon: <AuditOutlined />,
                },
                {
                    path: '/admin/clusters',
                    name: t('nav.clusters'),
                    icon: <ClusterOutlined />,
                },
                {
                    path: '/admin/namespaces',
                    name: t('nav.namespaces'),
                    icon: <GlobalOutlined />,
                },
                {
                    path: '/admin/templates',
                    name: t('nav.templates'),
                    icon: <ProfileOutlined />,
                },
                {
                    path: '/admin/instance-sizes',
                    name: t('nav.instance_sizes'),
                    icon: <HddOutlined />,
                },
                {
                    path: '/admin/users',
                    name: t('nav.users'),
                    icon: <TeamOutlined />,
                },
                {
                    path: '/admin/rbac',
                    name: t('nav.rbac'),
                    icon: <SafetyCertificateOutlined />,
                },
                {
                    path: '/admin/auth-providers',
                    name: t('nav.auth_providers'),
                    icon: <KeyOutlined />,
                },
                {
                    path: '/admin/audit',
                    name: t('nav.audit'),
                    icon: <FileTextOutlined />,
                },
            ],
        });
    }

    return {
        path: '/',
        routes,
    };
};
