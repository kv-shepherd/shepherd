'use client';

/**
 * Users management — admin page (placeholder).
 *
 * Awaits RBAC integration (Phase 5+).
 * All text uses i18n keys from admin.json namespace.
 */
import { Typography, Card } from 'antd';
import { TeamOutlined } from '@ant-design/icons';
import { useTranslation } from 'react-i18next';

const { Title, Text } = Typography;

export default function UsersPage() {
    const { t } = useTranslation(['admin', 'common']);

    return (
        <div>
            <div style={{ marginBottom: 24 }}>
                <Title level={4} style={{ margin: 0 }}>
                    {t('users.title')}
                </Title>
                <Text type="secondary">
                    {t('users.subtitle')}
                </Text>
            </div>
            <Card style={{ borderRadius: 12, textAlign: 'center', padding: 48 }}>
                <TeamOutlined style={{ fontSize: 48, color: '#d9d9d9', marginBottom: 16 }} />
                <Title level={5} type="secondary">
                    {t('users.placeholder')}
                </Title>
                <Text type="secondary">
                    {t('users.roles')}
                </Text>
            </Card>
        </div>
    );
}
