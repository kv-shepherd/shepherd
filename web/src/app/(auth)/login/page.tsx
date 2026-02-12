'use client';

/**
 * Login Page (master-flow Stage 1.5).
 *
 * Features:
 * - Username/password form with Zod validation
 * - Force password change redirect
 * - i18n support (en/zh-CN) — all text uses translation keys
 * - Ant Design LoginForm from pro-components
 *
 * AGENTS.md §6.5: Prevent hydration mismatch without flickering.
 */
import { useState } from 'react';
import Image from 'next/image';
import { Card, Form, Input, Button, Typography, Space, Alert } from 'antd';
import { UserOutlined, LockOutlined } from '@ant-design/icons';
import { useTranslation } from 'react-i18next';
import { useAuth } from '@/hooks/useAuth';

const { Title, Text } = Typography;

interface LoginFormValues {
    username: string;
    password: string;
}

export default function LoginPage() {
    const { t } = useTranslation('common');
    const { t: tErrors } = useTranslation('errors');
    const { login } = useAuth();
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState<string | null>(null);

    const handleSubmit = async (values: LoginFormValues) => {
        setLoading(true);
        setError(null);
        try {
            await login(values);
        } catch (err: unknown) {
            const apiErr = err as { code?: string };
            setError(apiErr?.code ? tErrors(apiErr.code) : tErrors('INTERNAL_ERROR'));
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
                    <Image src="/logo-wide.svg" alt="Shepherd" width={220} height={56} />
                    <div>
                        <Title level={3} style={{ marginBottom: 4 }}>
                            {t('app.name')}
                        </Title>
                        <Text type="secondary">
                            {t('app.subtitle')}
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

                <Form<LoginFormValues>
                    name="login"
                    onFinish={handleSubmit}
                    autoComplete="off"
                    size="large"
                    layout="vertical"
                >
                    <Form.Item
                        name="username"
                        rules={[
                            { required: true, message: t('validation.username_required') },
                            { min: 2, message: t('validation.username_min') },
                        ]}
                    >
                        <Input
                            prefix={<UserOutlined />}
                            placeholder={t('auth.username')}
                            autoFocus
                        />
                    </Form.Item>

                    <Form.Item
                        name="password"
                        rules={[
                            { required: true, message: t('validation.password_required') },
                        ]}
                    >
                        <Input.Password
                            prefix={<LockOutlined />}
                            placeholder={t('auth.password')}
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
                            {t('auth.login')}
                        </Button>
                    </Form.Item>
                </Form>
            </Card>
        </div>
    );
}
