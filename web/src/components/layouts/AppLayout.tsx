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
    DownOutlined,
    GithubOutlined,
} from '@ant-design/icons';
import { ProLayout } from '@ant-design/pro-components';
import { AutoComplete, Button, Dropdown, Input, Typography, Avatar } from 'antd';
import { useTranslation } from 'react-i18next';
import { useAuthStore } from '@/stores/auth';
import {
    getStandardLoginPath,
    setNextLoginEntryOverride,
} from '@/lib/auth/loginEntry';
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
    const displayName = user?.display_name ?? user?.username ?? 'User';
    const sidebarFooterTitle = 'KubeVirt Shepherd';
    const sidebarFooterMeta = 'Cloud-native VM governance';

    const filteredMenuEntries = React.useMemo(
        () => filterMenuSearchEntries(searchableMenuEntries, menuSearch).slice(0, 8),
        [menuSearch, searchableMenuEntries],
    );

    const menuSearchOptions = React.useMemo(
        () => filteredMenuEntries.map((entry) => ({
            value: entry.path,
            label: (
                <div className="app-shell-search-option">
                    <span>{entry.label}</span>
                    {entry.groupLabel ? (
                        <Text type="secondary" className="app-shell-search-option__group">
                            {entry.groupLabel}
                        </Text>
                    ) : null}
                </div>
            ),
        })),
        [filteredMenuEntries],
    );

    const avatarSvg = encodeURIComponent(`
<svg viewBox="0 0 32 32" fill="none" xmlns="http://www.w3.org/2000/svg">
  <rect width="32" height="32" rx="16" fill="#EFF6FF"/>
  <path d="M16 16.5C18.4853 16.5 20.5 14.4853 20.5 12C20.5 9.51472 18.4853 7.5 16 7.5C13.5147 7.5 11.5 9.51472 11.5 12C11.5 14.4853 13.5147 16.5 16 16.5Z" fill="#3B82F6"/>
  <path d="M16 19C11.5817 19 8 21.6863 8 25V26.5C8 27.3284 8.67157 28 9.5 28H22.5C23.3284 28 24 27.3284 24 26.5V25C24 21.6863 20.4183 19 16 19Z" fill="#93C5FD"/>
</svg>`.trim());
    const avatarUrl = `data:image/svg+xml;utf8,${avatarSvg}`;

    const renderSidebarIdentity = (collapsed?: boolean) => (
        collapsed ? (
            <div className="app-shell-sidebar-footer app-shell-sidebar-footer--collapsed">
                <span className="app-shell-sidebar-footer__pulse" />
            </div>
        ) : (
            <div className="app-shell-sidebar-footer">
                <span className="app-shell-sidebar-footer__label">{sidebarFooterTitle}</span>
                <span className="app-shell-sidebar-footer__meta">{sidebarFooterMeta}</span>
            </div>
        )
    );

    return (
        <ProLayout
            className="app-shell-layout"
            style={{ height: '100vh' }}
            title="KubeVirt Shepherd"
            logo={<Image src="/logo-icon.svg" alt="Shepherd" width={32} height={32} style={{ width: 'auto', height: 32 }} />}
            route={route}
            location={{ pathname }}
            fixSiderbar
            fixedHeader
            layout="mix"
            splitMenus={false}
            headerTitleRender={(logo) => (
                <Link href="/dashboard" className="app-shell-brand">
                    <span className="app-shell-brand__mark">{logo}</span>
                    <span className="app-shell-brand__copy">
                        <span className="app-shell-brand__title">{t('app.name')}</span>
                    </span>
                </Link>
            )}
            menuFooterRender={(props) => renderSidebarIdentity(props?.collapsed)}
            token={{
                sider: {
                    colorMenuBackground: '#091423',
                    colorTextMenu: 'rgba(226, 232, 240, 0.8)',
                    colorTextMenuSelected: '#ffffff',
                    colorBgMenuItemSelected: 'rgba(37, 99, 235, 0.22)',
                },
                header: {
                    colorBgHeader: 'rgba(247, 250, 252, 0.72)',
                    heightLayoutHeader: 64,
                },
            }}
            actionsRender={() => [
                <div
                    key="local-timezone"
                    className="app-shell-toolbar-badge"
                >
                    <LocalTimezoneBadge />
                </div>,
                <div key="nav-search" style={{ display: 'flex', alignItems: 'center' }}>
                    <AutoComplete
                        className="app-shell-nav-search"
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
                    </AutoComplete>
                </div>,
                <Dropdown
                    key="quick-actions"
                    menu={{ items: quickActionItems }}
                    placement="bottomRight"
                >
                    <Button
                        type="primary"
                        icon={<ThunderboltOutlined />}
                        className="app-shell-action-button app-shell-action-button--primary"
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
                    <button
                        type="button"
                        className="action-icon app-shell-icon-action app-shell-lang-trigger"
                        aria-label={t('language.label')}
                    >
                        <span className="app-shell-lang-trigger__label">
                            {languageKey === 'zh-CN' ? t('language.short_chinese') : t('language.short_english')}
                        </span>
                        <GlobalOutlined style={{ fontSize: 18 }} />
                    </button>
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
            footerRender={() => (
                <div style={{ textAlign: 'center', padding: '24px 0 24px', color: '#8c98a4', fontSize: 13 }}>
                    <div style={{ display: 'flex', justifyContent: 'center', alignItems: 'center', gap: '16px', marginBottom: '8px' }}>
                        <span style={{ fontFamily: 'monospace', fontWeight: 600, padding: '4px 10px', background: 'rgba(15, 23, 42, 0.03)', borderRadius: '6px', border: '1px solid rgba(15, 23, 42, 0.05)', color: '#64748b' }}>Shepherd alpha</span>
                        <a href="https://github.com/kv-shepherd" target="_blank" rel="noopener noreferrer" style={{ color: '#64748b', fontSize: 18, display: 'flex', alignItems: 'center', opacity: 0.8 }} title="GitHub">
                            <GithubOutlined />
                        </a>
                    </div>
                    <div style={{ letterSpacing: '0.01em' }}>
                        Copyright &copy; 2026 The KubeVirt Shepherd Authors.
                    </div>
                </div>
            )}
            avatarProps={{
                src: undefined, // Let our custom render handle the image completely
                title: undefined, // Suppress default ProLayout text injection!
                size: 28,
                render: () => (
                    <Dropdown
                        menu={{
                            items: [
                                {
                                    key: 'username',
                                    label: (
                                        <Text strong>
                                            {displayName}
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
                                        setNextLoginEntryOverride(getStandardLoginPath());
                                        logout();
                                        router.push(getStandardLoginPath());
                                    },
                                },
                            ],
                        }}
                    >
                        <button
                            type="button"
                            className="app-shell-profile-trigger"
                            title={displayName}
                        >
                            <span className="app-shell-profile-trigger__avatar">
                                <Avatar src={avatarUrl} size={28} style={{ border: '1px solid #bfdbfe', background: '#eff6ff' }} />
                            </span>
                            <span className="app-shell-profile-trigger__name">{displayName}</span>
                            <DownOutlined className="app-shell-profile-trigger__chevron" />
                        </button>
                    </Dropdown>
                ),
            }}
        >
            <div className="app-shell-content">
                <div className="app-shell-content__inner">{children}</div>
            </div>
        </ProLayout>
    );
}
