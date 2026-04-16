'use client';

import * as React from 'react';
import { GlobalOutlined } from '@ant-design/icons';
import { Dropdown } from 'antd';
import { useTranslation } from 'react-i18next';

function normalizeLanguageKey(language: string | null | undefined): 'en' | 'zh-CN' {
    const normalized = (language ?? 'en').toLowerCase();
    return normalized.startsWith('zh') ? 'zh-CN' : 'en';
}

interface LanguageSwitcherProps {
    buttonClassName?: string;
    dataTestId?: string;
    placement?: 'bottomLeft' | 'bottomRight' | 'topLeft' | 'topRight';
}

export default function LanguageSwitcher({
    buttonClassName,
    dataTestId,
    placement = 'bottomRight',
}: LanguageSwitcherProps) {
    const { t, i18n } = useTranslation('common');
    const languageKey = React.useMemo(
        () => normalizeLanguageKey(i18n.resolvedLanguage ?? i18n.language),
        [i18n.language, i18n.resolvedLanguage],
    );

    const handleLanguageChange = React.useCallback((language: 'en' | 'zh-CN') => {
        void i18n.changeLanguage(language);
    }, [i18n]);

    return (
        <Dropdown
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
            placement={placement}
        >
            <button
                type="button"
                className={['action-icon', 'app-shell-icon-action', 'app-shell-lang-trigger', buttonClassName]
                    .filter(Boolean)
                    .join(' ')}
                aria-label={t('language.label')}
                data-testid={dataTestId}
            >
                <span className="app-shell-lang-trigger__label">
                    {languageKey === 'zh-CN' ? t('language.short_chinese') : t('language.short_english')}
                </span>
                <GlobalOutlined style={{ fontSize: 18 }} />
            </button>
        </Dropdown>
    );
}
