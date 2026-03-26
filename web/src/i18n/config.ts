/**
 * i18n configuration (FRONTEND.md §Internationalization).
 *
 * Supported locales: en, zh-CN
 * Namespaces: common, errors, approval, vm, admin, schema
 */
export const i18nConfig = {
    defaultLocale: 'en',
    locales: ['en', 'zh-CN'] as const,
    fallbackLng: 'en',
    namespaces: ['common', 'errors', 'approval', 'vm', 'admin', 'schema'] as const,
    defaultNamespace: 'common' as const,
};
