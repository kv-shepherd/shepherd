/**
 * i18n configuration (FRONTEND.md §Internationalization).
 *
 * Supported locales: en, zh-CN
 * Namespaces: common, errors, approval, vm, admin
 */
export const i18nConfig = {
    defaultLocale: 'en',
    locales: ['en', 'zh-CN'] as const,
    fallbackLng: 'en',
    namespaces: ['common', 'errors', 'approval', 'vm', 'admin'] as const,
    defaultNamespace: 'common' as const,
};

export type Locale = (typeof i18nConfig.locales)[number];
export type Namespace = (typeof i18nConfig.namespaces)[number];
