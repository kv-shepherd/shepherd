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
            <ConfigProvider
                locale={antdLocaleMap[normalizedLanguage] ?? enUS}
                theme={{
                    algorithm: theme.defaultAlgorithm,
                    token: {
                        colorPrimary: '#1677ff',
                        borderRadius: 6,
                    },
                }}
            >
                <AntdApp>{children}</AntdApp>
            </ConfigProvider>
        </QueryClientProvider>
    );
}
