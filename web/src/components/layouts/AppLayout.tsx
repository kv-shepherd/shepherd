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
    ThunderboltOutlined,
    LogoutOutlined,
    GlobalOutlined,
    SearchOutlined,
} from '@ant-design/icons';
import { ProLayout } from '@ant-design/pro-components';
import { AutoComplete, Button, Dropdown, Input, Typography } from 'antd';
import { useTranslation } from 'react-i18next';
import { useAuthStore } from '@/stores/auth';
import NotificationBell from '@/components/ui/NotificationBell';
import LocalTimezoneBadge from '@/components/ui/LocalTimezoneBadge';
import { hasPermission, PLATFORM_ADMIN_PERMISSION } from '@/lib/auth/permissions';
import {
    filterMenuSearchEntries,
    flattenMenuRoutes,
    getMenuRoutes,
    resolveMenuHref,
} from './appLayoutRoutes';

const { Text } = Typography;

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
    const [menuSearch, setMenuSearch] = React.useState('');
    const languageKey = React.useMemo(() => {
        const lang = (i18n.resolvedLanguage ?? i18n.language ?? 'en').toLowerCase();
        return lang.startsWith('zh') ? 'zh-CN' : 'en';
    }, [i18n.language, i18n.resolvedLanguage]);
    const secureDevOrigin = process.env.NEXT_PUBLIC_DEV_SECURE_ORIGIN?.trim() ?? '';
    const devIngressPort = process.env.NEXT_PUBLIC_DEV_HTTP_INGRESS_PORT?.trim() ?? '';

    React.useEffect(() => {
        if (process.env.NODE_ENV !== 'development') {
            return;
        }
        if (!secureDevOrigin || typeof window === 'undefined') {
            return;
        }
        if (window.location.protocol !== 'http:') {
            return;
        }
        if (devIngressPort !== '' && window.location.port !== devIngressPort) {
            return;
        }

        try {
            const target = new URL(secureDevOrigin);
            if (window.location.hostname !== target.hostname) {
                return;
            }
            const nextURL = `${target.origin}${window.location.pathname}${window.location.search}${window.location.hash}`;
            if (nextURL !== window.location.href) {
                window.location.replace(nextURL);
            }
        } catch {
            // Ignore malformed dev origin overrides; the HTTP page still works with a manual refresh.
        }
    }, [devIngressPort, secureDevOrigin]);

    const handleLanguageChange = (lang: string) => {
        void i18n.changeLanguage(lang);
    };

    const quickActionItems = React.useMemo(() => {
        const items = [
            {
                key: 'new-vm-request',
                label: t('quick_actions.new_vm_request'),
                onClick: () => router.push('/vms?request=create'),
            },
            {
                key: 'my-requests',
                label: t('quick_actions.open_my_requests'),
                onClick: () => router.push('/tickets'),
            },
        ];
        if (canAccessAdmin) {
            items.push({
                key: 'approval-center',
                label: t('quick_actions.open_approval_tasks'),
                onClick: () => router.push('/admin/approval-tasks'),
            });
        }
        return items;
    }, [canAccessAdmin, router, t]);

    const searchableMenuEntries = React.useMemo(
        () => flattenMenuRoutes(route.routes),
        [route.routes],
    );

    const filteredMenuEntries = React.useMemo(
        () => filterMenuSearchEntries(searchableMenuEntries, menuSearch).slice(0, 8),
        [menuSearch, searchableMenuEntries],
    );

    const menuSearchOptions = React.useMemo(
        () => filteredMenuEntries.map((entry) => ({
            value: entry.path,
            label: (
                <div style={{ display: 'flex', flexDirection: 'column' }}>
                    <span>{entry.label}</span>
                    {entry.groupLabel ? (
                        <Text type="secondary" style={{ fontSize: 12 }}>
                            {entry.groupLabel}
                        </Text>
                    ) : null}
                </div>
            ),
        })),
        [filteredMenuEntries],
    );

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
                    colorMenuBackground: '#0f1c2f',
                    colorTextMenu: '#dbe7ffcc',
                    colorTextMenuSelected: '#fff',
                    colorBgMenuItemSelected: '#155eef2b',
                },
                header: {
                    colorBgHeader: 'rgba(255, 255, 255, 0.78)',
                    heightLayoutHeader: 56,
                },
            }}
            actionsRender={() => [
                <div
                    key="local-timezone"
                    style={{
                        display: 'flex',
                        alignItems: 'center',
                        paddingInline: 8,
                    }}
                >
                    <LocalTimezoneBadge />
                </div>,
                <AutoComplete
                    key="nav-search"
                    style={{ width: 280 }}
                    value={menuSearch}
                    options={menuSearchOptions}
                    onSearch={setMenuSearch}
                    onChange={(value) => setMenuSearch(String(value))}
                    onSelect={(value) => {
                        setMenuSearch('');
                        router.push(String(value));
                    }}
                    notFoundContent={t('nav.search_empty')}
                >
                    <Input
                        allowClear={true}
                        prefix={<SearchOutlined />}
                        placeholder={t('nav.search_placeholder')}
                    />
                </AutoComplete>,
                <Dropdown
                    key="quick-actions"
                    menu={{ items: quickActionItems }}
                    placement="bottomRight"
                >
                    <Button
                        type="text"
                        icon={<ThunderboltOutlined />}
                        data-testid="quick-actions-trigger"
                    >
                        {t('quick_actions.label')}
                    </Button>
                </Dropdown>,
                <Dropdown
                    key="language"
                    menu={{
                        items: [
                            {
                                key: 'en',
                                label: t('language.english'),
                                onClick: () => handleLanguageChange('en'),
                            },
                            {
                                key: 'zh-CN',
                                label: t('language.chinese'),
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
            menuItemRender={(item, dom) => {
                const href = resolveMenuHref(item);
                return href ? (
                    <Link
                        href={href}
                        legacyBehavior={false}
                        style={{ width: '100%', display: 'block' }}
                        data-testid={`nav-item-${String(item.key || href)
                            .replace(/^\//, '')
                            .replace(/\//g, '-')
                            .replace(/\s+/g, '-')
                            .toLowerCase()}`}
                    >
                        {dom}
                    </Link>
                ) : (
                    <span
                        style={{ width: '100%', display: 'block' }}
                        data-testid={`nav-item-${String(item.key || item.name || 'group')
                            .replace(/^\//, '')
                            .replace(/\//g, '-')
                            .replace(/\s+/g, '-')
                            .toLowerCase()}`}
                    >
                        {dom}
                    </span>
                );
            }}
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
            <div className="app-shell-content">{children}</div>
        </ProLayout>
    );
}
