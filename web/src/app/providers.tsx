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
import { SessionBootstrap } from '@/components/auth/SessionBootstrap';
import { DevBrowserLogBridge } from '@/components/dev/DevBrowserLogBridge';

const antdLocaleMap: Record<string, typeof enUS> = {
    en: enUS,
    'zh-CN': zhCN,
};

export default function Providers({
    children,
    nonce,
}: {
    children: React.ReactNode;
    nonce?: string;
}) {
    const { i18n } = useTranslation();
    const normalizedLanguage = React.useMemo(() => {
        const lang = (i18n.resolvedLanguage ?? i18n.language ?? 'en').toLowerCase();
        return lang.startsWith('zh') ? 'zh-CN' : 'en';
    }, [i18n.language, i18n.resolvedLanguage]);
    React.useEffect(() => {
        if (typeof document === 'undefined') {
            return;
        }

        document.documentElement.lang = normalizedLanguage;
    }, [normalizedLanguage]);
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
    const enableDevBrowserLogBridge =
        process.env.NODE_ENV === 'development'
        && process.env.NEXT_PUBLIC_DEV_BROWSER_LOG_BRIDGE === '1';

    return (
        <QueryClientProvider client={queryClient}>
            <SessionBootstrap />
            {enableDevBrowserLogBridge && <DevBrowserLogBridge />}
            <ConfigProvider
                csp={nonce ? { nonce } : undefined}
                locale={antdLocaleMap[normalizedLanguage] ?? enUS}
                theme={{
                    algorithm: theme.defaultAlgorithm,
                    token: {
                        colorPrimary: '#2563eb',
                        colorInfo: '#2563eb',
                        colorLink: '#1d4ed8',
                        colorSuccess: '#1f9d61',
                        colorWarning: '#d97706',
                        colorError: '#dc2626',
                        colorBgLayout: '#f5f7fb',
                        colorBgContainer: '#ffffff',
                        colorText: '#0f172a',
                        colorTextSecondary: '#5b677d',
                        colorTextTertiary: '#7d879b',
                        colorFillSecondary: 'rgba(37, 99, 235, 0.08)',
                        colorFillTertiary: 'rgba(14, 165, 233, 0.12)',
                        colorBorder: '#d6e1ee',
                        colorBorderSecondary: '#e8eef6',
                        borderRadius: 8,
                        borderRadiusLG: 10,
                        borderRadiusSM: 6,
                        fontSize: 15,
                        fontSizeSM: 13,
                        fontSizeLG: 17,
                        lineHeight: 1.6,
                        fontFamily: 'var(--font-ui-sans)',
                        fontFamilyCode: 'var(--font-ui-mono)',
                        boxShadowSecondary: '0 10px 24px rgba(15, 23, 42, 0.08)',
                    },
                }}
            >
                <AntdApp>{children}</AntdApp>
            </ConfigProvider>
        </QueryClientProvider>
    );
}
