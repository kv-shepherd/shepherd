'use client';

import Image from 'next/image';
import { Alert, Button, Card, Form, Input, Space, Typography } from 'antd';
import { LockOutlined } from '@ant-design/icons';
import { useTranslation } from 'react-i18next';

import {
    type ChangePasswordFormValues,
    useChangePasswordController,
} from '../hooks/useChangePasswordController';

const { Title, Text } = Typography;

export function ChangePasswordContent() {
    const { t } = useTranslation('common');
    const controller = useChangePasswordController();

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
                    <Image
                        src="/logo-wide.svg"
                        alt="Shepherd"
                        width={180}
                        height={52}
                        style={{ width: 'auto', height: 52, maxWidth: '100%' }}
                    />
                    <div>
                        <Title level={3} style={{ marginBottom: 4 }}>
                            {t('auth.change_password')}
                        </Title>
                        <Text type="secondary">
                            {t('auth.change_password_hint')}
                        </Text>
                    </div>
                </Space>

                {controller.error && (
                    <Alert
                        message={controller.error}
                        type="error"
                        showIcon
                        closable
                        onClose={() => controller.setError(null)}
                        style={{ marginBottom: 24 }}
                    />
                )}

                <Form<ChangePasswordFormValues>
                    name="change-password"
                    onFinish={(values) => {
                        void controller.submit(values);
                    }}
                    autoComplete="off"
                    size="large"
                    layout="vertical"
                >
                    <Form.Item
                        name="current_password"
                        rules={[
                            { required: true, message: t('validation.password_required') },
                        ]}
                    >
                        <Input.Password
                            prefix={<LockOutlined />}
                            placeholder={t('auth.current_password')}
                        />
                    </Form.Item>

                    <Form.Item
                        name="new_password"
                        rules={[
                            { required: true, message: t('validation.password_required') },
                            { min: 8, message: t('validation.password_min') },
                        ]}
                    >
                        <Input.Password
                            prefix={<LockOutlined />}
                            placeholder={t('auth.new_password')}
                        />
                    </Form.Item>

                    <Form.Item
                        name="confirm_password"
                        dependencies={['new_password']}
                        rules={[
                            { required: true, message: t('validation.confirm_password_required') },
                            ({ getFieldValue }) => ({
                                validator(_, value) {
                                    if (!value || getFieldValue('new_password') === value) {
                                        return Promise.resolve();
                                    }
                                    return Promise.reject(new Error(t('validation.password_mismatch')));
                                },
                            }),
                        ]}
                    >
                        <Input.Password
                            prefix={<LockOutlined />}
                            placeholder={t('auth.confirm_password')}
                        />
                    </Form.Item>

                    <Form.Item style={{ marginBottom: 0 }}>
                        <Button
                            type="primary"
                            htmlType="submit"
                            loading={controller.loading}
                            block
                            style={{
                                height: 44,
                                borderRadius: 8,
                                fontWeight: 600,
                            }}
                        >
                            {t('auth.change_password')}
                        </Button>
                    </Form.Item>
                </Form>
            </Card>
        </div>
    );
}
