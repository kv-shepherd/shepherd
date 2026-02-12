'use client';

/**
 * Application Shell Layout with ProLayout sidebar navigation.
 *
 * AGENTS.md §2.1: Direct imports (antd is in optimizePackageImports).
 * AGENTS.md §3.5: Parallel data fetching with component composition.
 *
 * This layout wraps all authenticated pages (dashboard, systems, services, vms, admin).
 * Auth route group (auth) uses its own layout without sidebar.
 */
import React from 'react';
import Image from 'next/image';
import { useRouter, usePathname } from 'next/navigation';
import {
    DashboardOutlined,
    CloudServerOutlined,
    AppstoreOutlined,
    DesktopOutlined,
    AuditOutlined,
    ClusterOutlined,
    TeamOutlined,
    LogoutOutlined,
    SettingOutlined,
    FileTextOutlined,
    GlobalOutlined,
    HddOutlined,
    ProfileOutlined,
} from '@ant-design/icons';
import { ProLayout } from '@ant-design/pro-components';
import type { ProLayoutProps } from '@ant-design/pro-components';
import { Dropdown, Typography } from 'antd';
import { useTranslation } from 'react-i18next';
import { useAuthStore } from '@/stores/auth';
import NotificationBell from '@/components/ui/NotificationBell';

const { Text } = Typography;

/**
 * Navigation route configuration.
 * Maps to FRONTEND.md directory structure.
 */
const getMenuRoutes = (t: (key: string) => string): ProLayoutProps['route'] => ({
    path: '/',
    routes: [
        {
            path: '/dashboard',
            name: t('nav.dashboard'),
            icon: <DashboardOutlined />,
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
            name: 'Admin',
            icon: <SettingOutlined />,
            path: '/admin',
            routes: [
                {
                    path: '/admin/approvals',
                    name: t('nav.approvals'),
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
                    path: '/admin/audit',
                    name: t('nav.audit'),
                    icon: <FileTextOutlined />,
                },
            ],
        },
    ],
});

export default function AppLayout({
    children,
}: {
    children: React.ReactNode;
}) {
    const router = useRouter();
    const pathname = usePathname();
    const { t } = useTranslation('common');
    const { user, logout } = useAuthStore();

    return (
        <ProLayout
            title="Shepherd"
            logo={<Image src="/logo-icon.svg" alt="Shepherd" width={32} height={32} />}
            route={getMenuRoutes(t)}
            location={{ pathname }}
            fixSiderbar
            fixedHeader
            layout="mix"
            splitMenus={false}
            token={{
                sider: {
                    colorMenuBackground: '#001529',
                    colorTextMenu: '#ffffffa6',
                    colorTextMenuSelected: '#fff',
                    colorBgMenuItemSelected: '#1677ff22',
                },
                header: {
                    colorBgHeader: '#fff',
                    heightLayoutHeader: 56,
                },
            }}
            actionsRender={() => [
                <NotificationBell key="notifications" />,
            ]}
            menuItemRender={(item, dom) => (
                <div
                    onClick={() => {
                        if (item.path) {
                            router.push(item.path);
                        }
                    }}
                >
                    {dom}
                </div>
            )}
            avatarProps={{
                src: undefined,
                title: user?.display_name ?? user?.username ?? 'User',
                size: 'small',
                render: (_props, dom) => (
                    <Dropdown
                        menu={{
                            items: [
                                {
                                    key: 'username',
                                    label: (
                                        <Text strong>
                                            {user?.display_name ?? user?.username}
                                        </Text>
                                    ),
                                    disabled: true,
                                },
                                { type: 'divider' },
                                {
                                    key: 'logout',
                                    icon: <LogoutOutlined />,
                                    label: t('auth.logout'),
                                    danger: true,
                                    onClick: () => {
                                        logout();
                                        router.push('/login');
                                    },
                                },
                            ],
                        }}
                    >
                        {dom}
                    </Dropdown>
                ),
            }}
        >
            {children}
        </ProLayout>
    );
}
