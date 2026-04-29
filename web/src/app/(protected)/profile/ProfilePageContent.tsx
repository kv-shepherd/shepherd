'use client';

/**
 * /profile — User profile and settings page.
 * master-flow.md §6: User Self-Service — Profile.
 *
 * API contracts:
 *   GET  /auth/me                           → UserInfo
 *   POST /auth/change-password              → 204
 *
 * E2E data-testid requirements:
 *   profile-page
 *   change-password-button
 *   change-password-modal
 *   change-password-current-input
 *   change-password-new-input
 *   change-password-confirm-input
 *   change-password-submit-button
 */
import {
    Alert,
    Button,
    Descriptions,
    Form,
    Input,
    Modal,
    Space,
    Typography,
} from 'antd';
import { LockOutlined, UserOutlined } from '@ant-design/icons';
import { useState } from 'react';
import { useRouter } from 'next/navigation';
import { useTranslation } from 'react-i18next';

import { PageHeader, PageSurface } from '@/components/layouts/PageSection';
import { useApiMutation } from '@/hooks/useApiQuery';
import { getStandardLoginPath, setNextLoginEntryOverride } from '@/lib/auth/loginEntry';
import { useApiGet } from '@/lib/api/useApiGet';
import { api } from '@/lib/api/client';
import { translateApiError } from '@/lib/api/errorMessage';
import { useMessage } from '@/lib/hooks/useMessage';
import { useAuthStore } from '@/stores/auth';

const { Text, Title } = Typography;

interface UserProfile {
    id: string;
    username: string;
    email?: string;
    role?: string;
    created_at?: string;
    last_login?: string;
}

interface ChangePasswordForm {
    current_password: string;
    new_password: string;
    confirm_password: string;
}

export default function ProfilePage() {
    const { t } = useTranslation('common');
    const router = useRouter();
    const [changePasswordOpen, setChangePasswordOpen] = useState(false);
    const [form] = Form.useForm<ChangePasswordForm>();
    const [error, setError] = useState<string | null>(null);
    const { messageContextHolder } = useMessage();

    const { data: profile } = useApiGet<UserProfile>(
        ['user-me'],
        () => api.GET('/auth/me', {}) as Promise<{ data?: UserProfile; error?: unknown; response?: Response }>
    );

    const changePasswordMutation = useApiMutation(
        (values: ChangePasswordForm) =>
            api.POST('/auth/change-password', {
                body: {
                    old_password: values.current_password,
                    new_password: values.new_password,
                },
            }),
        {
            onSuccess: () => {
                setNextLoginEntryOverride(getStandardLoginPath());
                useAuthStore.getState().logout();
                setChangePasswordOpen(false);
                form.resetFields();
                setError(null);
                router.push(getStandardLoginPath());
            },
            onError: (err) => {
                setError(translateApiError(t, err, 'auth.change_password_error'));
            },
        }
    );

    const handleSubmit = () => {
        void form.validateFields().then((values) => {
            changePasswordMutation.mutate(values);
        });
    };

    return (
        <div data-testid="profile-page">
            {messageContextHolder}
            <PageHeader
                title={(
                    <>
                        <UserOutlined style={{ marginRight: 8, color: '#1677ff' }} />
                        {t('nav.profile', { defaultValue: 'My Profile' })}
                    </>
                )}
                subtitle={t('profile.subtitle', { defaultValue: 'View your account details and manage settings.' })}
            />

            <PageSurface style={{ marginBottom: 16 }}>
                <Descriptions
                    title={t('profile.account_info', { defaultValue: 'Account Information' })}
                    bordered
                    column={2}
                >
                    <Descriptions.Item label={t('table.username', { defaultValue: 'Username' })}>
                        <Text strong>{profile?.username ?? '—'}</Text>
                    </Descriptions.Item>
                    <Descriptions.Item label={t('table.role', { defaultValue: 'Role' })}>
                        {profile?.role ?? '—'}
                    </Descriptions.Item>
                    <Descriptions.Item label={t('table.email', { defaultValue: 'Email' })}>
                        {profile?.email ?? '—'}
                    </Descriptions.Item>
                    <Descriptions.Item label={t('table.created_at', { defaultValue: 'Joined' })}>
                        {profile?.created_at ?? '—'}
                    </Descriptions.Item>
                </Descriptions>
            </PageSurface>

            <PageSurface>
                <Space direction="vertical" size="middle" style={{ width: '100%' }}>
                    <div>
                        <Title level={5} style={{ margin: 0 }}>
                            {t('auth.security', { defaultValue: 'Security' })}
                        </Title>
                        <Text type="secondary">
                            {t('auth.security_subtitle', { defaultValue: 'Manage your login credentials.' })}
                        </Text>
                    </div>
                    <Button
                        icon={<LockOutlined />}
                        data-testid="change-password-button"
                        onClick={() => setChangePasswordOpen(true)}
                    >
                        {t('auth.change_password')}
                    </Button>
                </Space>
            </PageSurface>

            {changePasswordOpen ? (
                <Modal
                    title={t('auth.change_password')}
                    open={true}
                    onOk={handleSubmit}
                    onCancel={() => {
                        setChangePasswordOpen(false);
                        form.resetFields();
                        setError(null);
                    }}
                    confirmLoading={changePasswordMutation.isPending}
                    data-testid="change-password-modal"
                >
                        {error && (
                            <Alert
                                message={error}
                                type="error"
                                showIcon
                                closable
                                onClose={() => setError(null)}
                                style={{ marginBottom: 16 }}
                            />
                        )}
                        <Form form={form} layout="vertical" name="change-password-form">
                            <Form.Item
                                name="current_password"
                                label={t('auth.current_password')}
                                rules={[{ required: true, message: t('validation.password_required') }]}
                            >
                                <Input.Password
                                    prefix={<LockOutlined />}
                                    placeholder={t('auth.current_password')}
                                    data-testid="change-password-current-input"
                                />
                            </Form.Item>
                            <Form.Item
                                name="new_password"
                                label={t('auth.new_password')}
                                rules={[
                                    { required: true, message: t('validation.password_required') },
                                    { min: 8, message: t('validation.password_min') },
                                ]}
                            >
                                <Input.Password
                                    prefix={<LockOutlined />}
                                    placeholder={t('auth.new_password')}
                                    data-testid="change-password-new-input"
                                />
                            </Form.Item>
                            <Form.Item
                                name="confirm_password"
                                label={t('auth.confirm_password')}
                                dependencies={['new_password']}
                                rules={[
                                    { required: true, message: t('validation.confirm_password_required') },
                                    ({ getFieldValue }) => ({
                                        validator(_, value) {
                                            if (!value || getFieldValue('new_password') === value) {
                                                return Promise.resolve();
                                            }
                                            return Promise.reject(
                                                new Error(t('validation.password_mismatch'))
                                            );
                                        },
                                    }),
                                ]}
                            >
                                <Input.Password
                                    prefix={<LockOutlined />}
                                    placeholder={t('auth.confirm_password')}
                                    data-testid="change-password-confirm-input"
                                />
                            </Form.Item>
                        </Form>
                </Modal>
            ) : null}
        </div>
    );
}
