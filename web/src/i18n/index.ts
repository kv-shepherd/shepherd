/**
 * i18n initialization (FRONTEND.md §Initialization).
 *
 * Uses react-i18next with browser language detection.
 * Namespaces: common, errors, vm, approval, admin, schema
 */
import i18n from 'i18next';
import { initReactI18next } from 'react-i18next';
import LanguageDetector from 'i18next-browser-languagedetector';
import { i18nConfig } from './config';

// Import locale resources
import enCommon from './locales/en/common.json';
import enErrors from './locales/en/errors.json';
import enVm from './locales/en/vm.json';
import enApproval from './locales/en/approval.json';
import enAdmin from './locales/en/admin.json';
import enSchema from './locales/en/schema.json';
import zhCNCommon from './locales/zh-CN/common.json';
import zhCNErrors from './locales/zh-CN/errors.json';
import zhCNVm from './locales/zh-CN/vm.json';
import zhCNApproval from './locales/zh-CN/approval.json';
import zhCNAdmin from './locales/zh-CN/admin.json';
import zhCNSchema from './locales/zh-CN/schema.json';

i18n
    .use(LanguageDetector)
    .use(initReactI18next)
    .init({
        resources: {
            en: {
                common: enCommon,
                errors: enErrors,
                vm: enVm,
                approval: enApproval,
                admin: enAdmin,
                schema: enSchema,
            },
            'zh-CN': {
                common: zhCNCommon,
                errors: zhCNErrors,
                vm: zhCNVm,
                approval: zhCNApproval,
                admin: zhCNAdmin,
                schema: zhCNSchema,
            },
        },
        ...i18nConfig,
        interpolation: {
            escapeValue: false,
        },
    });

export default i18n;
