'use client';

import { Alert, Button, Space, Tag, Typography } from 'antd';
import type { TFunction } from 'i18next';

import { LocalDateTimeText } from '@/components/ui/LocalDateTimeText';
import type { VMRequestDraft } from '../types';

const { Text } = Typography;

interface VMSavedDraftBannerProps {
    t: TFunction;
    draft: VMRequestDraft;
    onResume: () => void;
    onDiscard: () => void;
}

export function VMSavedDraftBanner({
    t,
    draft,
    onResume,
    onDiscard,
}: VMSavedDraftBannerProps) {
    return (
        <Alert
            showIcon
            type="info"
            message={t('draft.banner_title')}
            description={(
                <Space direction="vertical" size={8}>
                    <Text type="secondary">{t('draft.banner_description')}</Text>
                    <Space wrap size={8}>
                        {draft.serviceLabel && <Tag>{draft.serviceLabel}</Tag>}
                        {draft.templateLabel && <Tag color="blue">{draft.templateLabel}</Tag>}
                        {draft.instanceSizeLabel && <Tag color="purple">{draft.instanceSizeLabel}</Tag>}
                        {draft.namespace && <Tag color="gold">{draft.namespace}</Tag>}
                        <Text type="secondary">
                            {t('draft.banner_updated_at')} <LocalDateTimeText value={draft.updatedAt} />
                        </Text>
                    </Space>
                </Space>
            )}
            action={(
                <Space wrap>
                    <Button type="primary" onClick={onResume}>
                        {t('draft.resume')}
                    </Button>
                    <Button onClick={onDiscard}>
                        {t('draft.discard')}
                    </Button>
                </Space>
            )}
        />
    );
}
