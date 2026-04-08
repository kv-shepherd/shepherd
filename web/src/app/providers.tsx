'use client';

/**
 * Client-side providers for the application.
 *
 * Wraps the app with:
 * - Ant Design ConfigProvider (theming)
 * - TanStack Query QueryClientProvider (data fetching)
 * - i18n (internationalization)
 */
import React, { useState } from 'react';
import '@ant-design/v5-patch-for-react-19';
import { ConfigProvider, App as AntdApp, theme } from 'antd';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import zhCN from 'antd/locale/zh_CN';
import enUS from 'antd/locale/en_US';
import { useTranslation } from 'react-i18next';

// Initialize i18n (side-effect import)
import '@/i18n';
import { DevBrowserLogBridge } from '@/components/dev/DevBrowserLogBridge';

const antdLocaleMap: Record<string, typeof enUS> = {
    en: enUS,
    'zh-CN': zhCN,
};

export default function Providers({
    children,
}: {
    children: React.ReactNode;
}) {
    const { i18n } = useTranslation();
    const normalizedLanguage = React.useMemo(() => {
        const lang = (i18n.resolvedLanguage ?? i18n.language ?? 'en').toLowerCase();
        return lang.startsWith('zh') ? 'zh-CN' : 'en';
    }, [i18n.language, i18n.resolvedLanguage]);
    const [queryClient] = useState(
        () =>
            new QueryClient({
                defaultOptions: {
                    queries: {
                        staleTime: 60 * 1000, // 1 minute
                        retry: 1,
                        refetchOnWindowFocus: false,
                    },
                },
            })
    );

    return (
        <QueryClientProvider client={queryClient}>
            {process.env.NEXT_PUBLIC_DEV_BROWSER_LOG_BRIDGE === '1' && <DevBrowserLogBridge />}
            <ConfigProvider
                locale={antdLocaleMap[normalizedLanguage] ?? enUS}
                theme={{
                    algorithm: theme.defaultAlgorithm,
                    token: {
                        colorPrimary: '#5e6ad2',
                        colorInfo: '#5e6ad2',
                        colorLink: '#5e6ad2',
                        colorSuccess: '#27a644',
                        colorWarning: '#d97706',
                        colorError: '#d14343',
                        colorBgLayout: '#f3f5fb',
                        colorBgContainer: '#ffffff',
                        colorText: '#171b26',
                        colorTextSecondary: '#676f83',
                        colorTextTertiary: '#878ea0',
                        colorFillSecondary: 'rgba(94, 106, 210, 0.08)',
                        colorFillTertiary: 'rgba(94, 106, 210, 0.12)',
                        colorBorder: '#d7ddec',
                        colorBorderSecondary: '#e7ebf5',
                        borderRadius: 14,
                        borderRadiusLG: 20,
                        borderRadiusSM: 10,
                        fontFamily: 'var(--font-geist-sans), ui-sans-serif, system-ui, sans-serif',
                        boxShadowSecondary: '0 22px 56px rgba(16, 24, 40, 0.1), 0 6px 18px rgba(16, 24, 40, 0.05)',
                    },
                }}
            >
                <AntdApp>{children}</AntdApp>
            </ConfigProvider>
        </QueryClientProvider>
    );
}
