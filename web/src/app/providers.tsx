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
import { ConfigProvider, App as AntdApp, theme } from 'antd';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import zhCN from 'antd/locale/zh_CN';
import enUS from 'antd/locale/en_US';

// Initialize i18n (side-effect import)
import '@/i18n';

const antdLocaleMap: Record<string, typeof enUS> = {
    en: enUS,
    'zh-CN': zhCN,
};

export default function Providers({
    children,
    locale = 'en',
}: {
    children: React.ReactNode;
    locale?: string;
}) {
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
                locale={antdLocaleMap[locale] ?? enUS}
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
