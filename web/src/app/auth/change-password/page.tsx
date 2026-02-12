'use client';

/**
 * Change Password Page.
 *
 * Enforced by the backend when force_password_change=true
 * (e.g., first login with default admin credentials).
 *
 * Uses the same visual style as the login page.
 */
import { useState } from 'react';
import { Card, Form, Input, Button, Typography, Space, Alert } from 'antd';
import { LockOutlined } from '@ant-design/icons';
import { useTranslation } from 'react-i18next';
import { useRouter } from 'next/navigation';
import { api } from '@/lib/api/client';
import { useAuthStore } from '@/stores/auth';

const { Title, Text } = Typography;

interface ChangePasswordFormValues {
    current_password: string;
    new_password: string;
    confirm_password: string;
}

export default function ChangePasswordPage() {
    const { t } = useTranslation('common');
    const { t: tErrors } = useTranslation('errors');
    const router = useRouter();
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState<string | null>(null);

    const handleSubmit = async (values: ChangePasswordFormValues) => {
        setLoading(true);
        setError(null);
        try {
            const { error: apiError } = await api.POST('/auth/change-password', {
                body: {
                    old_password: values.current_password,
                    new_password: values.new_password,
                },
            });

            if (apiError) {
                const err = apiError as unknown as { code?: string };
                setError(err?.code ? tErrors(err.code) : tErrors('INTERNAL_ERROR'));
                return;
            }

            // Password changed successfully — clear forcePasswordChange flag and go to dashboard
            useAuthStore.getState().clearForcePasswordChange();
            router.push('/dashboard');
        } catch {
            setError(tErrors('INTERNAL_ERROR'));
        } finally {
            setLoading(false);
        }
    };

    return (
        <div
            style={{
                display: 'flex',
                justifyContent: 'center',
                alignItems: 'center',
                minHeight: '100vh',
                background: 'linear-gradient(135deg, #0f0c29 0%, #302b63 50%, #24243e 100%)',
                padding: 24,
            }}
        >
            <Card
                style={{
                    width: 420,
                    borderRadius: 16,
                    boxShadow: '0 20px 60px rgba(0, 0, 0, 0.3)',
                    border: 'none',
                }}
            >
                <Space
                    direction="vertical"
                    size="large"
                    style={{ width: '100%', textAlign: 'center', marginBottom: 32 }}
                >
                    <img
                        src="/logo-wide.svg"
                        alt="Shepherd"
                        style={{ width: 220, height: 'auto' }}
                    />
                    <div>
                        <Title level={3} style={{ marginBottom: 4 }}>
                            {t('auth.change_password', '修改密码')}
                        </Title>
                        <Text type="secondary">
                            {t('auth.change_password_hint', '首次登录请修改默认密码')}
                        </Text>
                    </div>
                </Space>

                {error && (
                    <Alert
                        message={error}
                        type="error"
                        showIcon
                        closable
                        onClose={() => setError(null)}
                        style={{ marginBottom: 24 }}
                    />
                )}

                <Form<ChangePasswordFormValues>
                    name="change-password"
                    onFinish={handleSubmit}
                    autoComplete="off"
                    size="large"
                    layout="vertical"
                >
                    <Form.Item
                        name="current_password"
                        rules={[
                            { required: true, message: t('validation.password_required', '请输入当前密码') },
                        ]}
                    >
                        <Input.Password
                            prefix={<LockOutlined />}
                            placeholder={t('auth.current_password', '当前密码')}
                        />
                    </Form.Item>

                    <Form.Item
                        name="new_password"
                        rules={[
                            { required: true, message: t('validation.password_required', '请输入新密码') },
                            { min: 8, message: t('validation.password_min', '密码至少 8 个字符') },
                        ]}
                    >
                        <Input.Password
                            prefix={<LockOutlined />}
                            placeholder={t('auth.new_password', '新密码')}
                        />
                    </Form.Item>

                    <Form.Item
                        name="confirm_password"
                        dependencies={['new_password']}
                        rules={[
                            { required: true, message: t('validation.confirm_password_required', '请确认新密码') },
                            ({ getFieldValue }) => ({
                                validator(_, value) {
                                    if (!value || getFieldValue('new_password') === value) {
                                        return Promise.resolve();
                                    }
                                    return Promise.reject(new Error(t('validation.password_mismatch', '两次密码不一致')));
                                },
                            }),
                        ]}
                    >
                        <Input.Password
                            prefix={<LockOutlined />}
                            placeholder={t('auth.confirm_password', '确认新密码')}
                        />
                    </Form.Item>

                    <Form.Item style={{ marginBottom: 0 }}>
                        <Button
                            type="primary"
                            htmlType="submit"
                            loading={loading}
                            block
                            style={{
                                height: 44,
                                borderRadius: 8,
                                fontWeight: 600,
                            }}
                        >
                            {t('auth.change_password', '修改密码')}
                        </Button>
                    </Form.Item>
                </Form>
            </Card>
        </div>
    );
}
