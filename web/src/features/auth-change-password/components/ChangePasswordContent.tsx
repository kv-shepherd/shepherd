'use client';

import Image from 'next/image';
import { Alert, Button, Form, Input, Space, Tag, Typography } from 'antd';
import { LockOutlined } from '@ant-design/icons';
import { useTranslation } from 'react-i18next';

import {
    type ChangePasswordFormValues,
    useChangePasswordController,
} from '../hooks/useChangePasswordController';
import { PageSurface } from '@/components/layouts/PageSection';

const { Title, Text } = Typography;

export function ChangePasswordContent() {
    const { t } = useTranslation('common');
    const controller = useChangePasswordController();

    return (
        <div className="auth-shell" data-testid="change-password-page">
            <PageSurface className="auth-shell__card">
                <Space
                    direction="vertical"
                    size="large"
                    className="auth-shell__hero"
                >
                    <Tag color="blue" className="auth-shell__eyebrow">
                        {t('auth.security')}
                    </Tag>
                    <Image
                        src="/logo-wide.svg"
                        alt="Shepherd"
                        width={180}
                        height={52}
                        className="auth-shell__logo"
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
                            className="auth-shell__submit"
                            data-testid="change-password-submit"
                        >
                            {t('auth.change_password')}
                        </Button>
                    </Form.Item>
                </Form>
            </PageSurface>
        </div>
    );
}
