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
import Link from 'next/link';
import {
    DashboardOutlined,
    CloudServerOutlined,
    AppstoreOutlined,
    DesktopOutlined,
    AuditOutlined,
    BellOutlined,
    ClusterOutlined,
    TeamOutlined,
    LogoutOutlined,
    SettingOutlined,
    FileTextOutlined,
    GlobalOutlined,
    HddOutlined,
    ProfileOutlined,
    SafetyCertificateOutlined,
    KeyOutlined,
} from '@ant-design/icons';
import { ProLayout } from '@ant-design/pro-components';
import type { ProLayoutProps } from '@ant-design/pro-components';
import { Dropdown, Typography } from 'antd';
import { useTranslation } from 'react-i18next';
import { useAuthStore } from '@/stores/auth';
import NotificationBell from '@/components/ui/NotificationBell';
import { hasPermission, PLATFORM_ADMIN_PERMISSION } from '@/lib/auth/permissions';

const { Text } = Typography;

/**
 * Navigation route configuration.
 * Maps to FRONTEND.md directory structure.
 */
const getMenuRoutes = (t: (key: string) => string, includeAdmin: boolean): ProLayoutProps['route'] => {
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
    ];

    if (includeAdmin) {
        routes.push({
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

export default function AppLayout({
    children,
}: {
    children: React.ReactNode;
}) {
    const router = useRouter();
    const pathname = usePathname();
    const { t, i18n } = useTranslation('common');
    const { user, logout } = useAuthStore();
    const canAccessAdmin = hasPermission(user, PLATFORM_ADMIN_PERMISSION);
    const route = React.useMemo(() => getMenuRoutes(t, canAccessAdmin), [t, canAccessAdmin]);
    const languageKey = React.useMemo(() => {
        const lang = (i18n.resolvedLanguage ?? i18n.language ?? 'en').toLowerCase();
        return lang.startsWith('zh') ? 'zh-CN' : 'en';
    }, [i18n.language, i18n.resolvedLanguage]);

    const handleLanguageChange = (lang: string) => {
        void i18n.changeLanguage(lang);
    };

    return (
        <ProLayout
            style={{ minHeight: '100vh' }}
            title="Shepherd"
            logo={<Image src="/logo-icon.svg" alt="Shepherd" width={32} height={32} />}
            route={route}
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
                <Dropdown
                    key="language"
                    menu={{
                        items: [
                            {
                                key: 'en',
                                label: 'English',
                                onClick: () => handleLanguageChange('en'),
                            },
                            {
                                key: 'zh-CN',
                                label: 'Simplified Chinese',
                                onClick: () => handleLanguageChange('zh-CN'),
                            },
                        ],
                        selectedKeys: [languageKey],
                    }}
                    placement="bottomRight"
                >
                    <div
                        style={{
                            display: 'flex',
                            alignItems: 'center',
                            justifyContent: 'center',
                            width: 32,
                            height: 32,
                            cursor: 'pointer',
                            borderRadius: '50%',
                            transition: 'background-color 0.3s',
                        }}
                        className="action-icon"
                    >
                        <GlobalOutlined style={{ fontSize: 18 }} />
                    </div>
                </Dropdown>,
                <NotificationBell key="notification" />,
            ]}
            menuItemRender={(item, dom) => (
                <Link href={item.path || '#'} legacyBehavior={false} style={{ width: '100%', display: 'block' }}>
                    {dom}
                </Link>
            )}
            avatarProps={{
                src: undefined,
                title: user?.display_name ?? user?.username ?? 'User',
                size: 'small',
                render: (_props, dom) => (
                    <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
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
                    </div>
                ),
            }}
        >
            {children}
        </ProLayout>
    );
}
