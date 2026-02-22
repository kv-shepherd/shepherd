'use client';

/**
 * /admin/permissions — Permissions management page (master-flow.md §9).
 * RBAC permissions catalog. Admin only.
 * data-testid="admin-permissions-page" required by E2E contract.
 */
import { Card, Table, Tag, Typography } from 'antd';
import { KeyOutlined } from '@ant-design/icons';
import { useTranslation } from 'react-i18next';

const { Title, Text } = Typography;

const STATIC_PERMISSIONS = [
    { key: 'vm:create', scope: 'VM', description: 'Request VM provisioning' },
    { key: 'vm:operate', scope: 'VM', description: 'Start/Stop/Restart VMs' },
    { key: 'vm:delete', scope: 'VM', description: 'Request VM deletion (approval required)' },
    { key: 'system:read', scope: 'System', description: 'List/view systems' },
    { key: 'system:write', scope: 'System', description: 'Create/edit systems' },
    { key: 'system:delete', scope: 'System', description: 'Delete systems' },
    { key: 'service:create', scope: 'Service', description: 'Create services' },
    { key: 'service:delete', scope: 'Service', description: 'Delete services' },
    { key: 'rbac:manage', scope: 'RBAC', description: 'Manage role bindings and system members' },
    { key: 'admin:all', scope: 'Admin', description: 'Full platform administration' },
];

export default function AdminPermissionsPage() {
    const { t } = useTranslation(['admin', 'common']);

    const columns = [
        {
            title: 'Permission Key',
            dataIndex: 'key',
            key: 'key',
            render: (key: string) => <Text code>{key}</Text>,
        },
        {
            title: 'Scope',
            dataIndex: 'scope',
            key: 'scope',
            width: 100,
            render: (scope: string) => <Tag color="blue">{scope}</Tag>,
        },
        {
            title: 'Description',
            dataIndex: 'description',
            key: 'description',
            render: (desc: string) => <Text type="secondary">{desc}</Text>,
        },
    ];

    return (
        <div data-testid="admin-permissions-page">
            <div style={{ marginBottom: 24 }}>
                <Title level={4} style={{ margin: 0 }}>
                    <KeyOutlined style={{ marginRight: 8, color: '#722ed1' }} />
                    {t('nav.permissions', { defaultValue: 'Permissions' })}
                </Title>
                <Text type="secondary">
                    {t('permissions.subtitle', { defaultValue: 'Platform permission catalog. Assign permissions via role bindings.' })}
                </Text>
            </div>

            <Card style={{ borderRadius: 12 }} styles={{ body: { padding: 0 } }}>
                <Table
                    dataSource={STATIC_PERMISSIONS}
                    columns={columns}
                    rowKey="key"
                    pagination={false}
                    size="middle"
                />
            </Card>
        </div>
    );
}
