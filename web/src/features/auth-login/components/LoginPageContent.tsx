'use client';

import { useEffect, useState } from 'react';
import Image from 'next/image';
import { Alert, Button, Card, Divider, Form, Input, Modal, Select, Space, Switch, Typography } from 'antd';
import { LockOutlined, UserOutlined } from '@ant-design/icons';
import { useTranslation } from 'react-i18next';

import { useAuth } from '@/hooks/useAuth';
import { useApiGet } from '@/hooks/useApiQuery';
import { api } from '@/lib/api/client';
import type { components } from '@/types/api.gen';

const { Title, Text } = Typography;

type LoginAuthProvider = components['schemas']['LoginAuthProvider'];
type LoginAuthProviderList = components['schemas']['LoginAuthProviderList'];
type LoginAuthMode = components['schemas']['AuthLoginMode'];
type CredentialFieldValue = string | number | boolean | null | undefined;

interface ResolvedLoginAuthMode {
    key: string;
    display_name: string;
    description?: string;
    interaction?: LoginAuthMode['interaction'];
    request_schema?: { [key: string]: unknown };
    default?: boolean;
}

interface LoginSchemaProperty {
    type?: string;
    format?: string;
    title?: string;
    description?: string;
    default?: unknown;
    enum?: string[];
}

interface LoginSchema {
    properties?: Record<string, LoginSchemaProperty>;
    required?: string[];
}

interface LoginFormValues {
    username: string;
    password: string;
}

function normalizeCredentialFieldValue(value: unknown): CredentialFieldValue {
    if (
        typeof value === 'string' ||
        typeof value === 'number' ||
        typeof value === 'boolean' ||
        value === null ||
        value === undefined
    ) {
        return value;
    }
    return undefined;
}

function resolveLoginModes(provider: LoginAuthProvider): ResolvedLoginAuthMode[] {
    if (!provider.login_modes || provider.login_modes.length === 0) {
        return [{
            key: 'default',
            display_name: provider.display_name || provider.id,
            interaction: 'redirect',
        }];
    }

    return provider.login_modes
        .filter((mode): mode is LoginAuthMode & { key: string; display_name: string } => Boolean(mode.key && mode.display_name))
        .map((mode) => ({
            key: mode.key,
            display_name: mode.display_name,
            description: mode.description,
            interaction: mode.interaction,
            request_schema: mode.request_schema,
            default: mode.default,
        }));
}

export default function LoginPageContent() {
    const { t } = useTranslation('common');
    const { t: tErrors } = useTranslation('errors');
    const { login, startExternalLogin, submitExternalCredentialLogin } = useAuth();
    const [loading, setLoading] = useState(false);
    const [externalLoadingKey, setExternalLoadingKey] = useState<string | null>(null);
    const [error, setError] = useState<string | null>(null);
    const [credentialModal, setCredentialModal] = useState<{ provider: LoginAuthProvider; mode: ResolvedLoginAuthMode } | null>(null);
    const [credentialSubmitting, setCredentialSubmitting] = useState(false);
    const [credentialForm] = Form.useForm<Record<string, CredentialFieldValue>>();
    const [mounted, setMounted] = useState(false);

    const providersQuery = useApiGet<LoginAuthProviderList>(
        ['login-auth-providers'],
        () => api.GET('/auth/providers'),
        { enabled: mounted },
    );

    useEffect(() => {
        setMounted(true);
    }, []);

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

    const handleExternalLogin = async (providerId: string, loginMode?: string) => {
        const loadingKey = `${providerId}:${loginMode ?? 'default'}`;
        setExternalLoadingKey(loadingKey);
        setError(null);
        try {
            await startExternalLogin(providerId, loginMode, '/dashboard');
        } catch (err: unknown) {
            const apiErr = err as { code?: string };
            setError(apiErr?.code ? tErrors(apiErr.code) : tErrors('INTERNAL_ERROR'));
        } finally {
            setExternalLoadingKey(null);
        }
    };

    const handleProviderLoginMode = async (provider: LoginAuthProvider, mode: ResolvedLoginAuthMode) => {
        if (mode.interaction === 'credentials') {
            credentialForm.resetFields();
            const schema = (mode.request_schema ?? {}) as LoginSchema;
            const defaults = Object.entries(schema.properties ?? {}).reduce<Record<string, CredentialFieldValue>>((acc, [key, def]) => {
                const normalized = normalizeCredentialFieldValue(def.default);
                if (normalized !== undefined) {
                    acc[key] = normalized;
                }
                return acc;
            }, {});
            credentialForm.setFieldsValue(defaults);
            setCredentialModal({ provider, mode });
            return;
        }
        await handleExternalLogin(provider.id, mode.key || undefined);
    };

    const handleCredentialSubmit = async () => {
        if (!credentialModal) {
            return;
        }
        try {
            const values = await credentialForm.validateFields();
            setCredentialSubmitting(true);
            await submitExternalCredentialLogin(
                credentialModal.provider.id,
                credentialModal.mode.key || undefined,
                values,
                '/dashboard',
            );
            setCredentialModal(null);
        } catch (err) {
            if ((err as { errorFields?: unknown[] })?.errorFields) {
                return;
            }
            const apiErr = err as { code?: string };
            setError(apiErr?.code ? tErrors(apiErr.code) : tErrors('INTERNAL_ERROR'));
        } finally {
            setCredentialSubmitting(false);
        }
    };

    const renderCredentialFields = (mode: ResolvedLoginAuthMode) => {
        const schema = (mode.request_schema ?? {}) as LoginSchema;
        const required = new Set(schema.required ?? []);
        return Object.entries(schema.properties ?? {}).map(([fieldName, field]) => {
            const rules = required.has(fieldName)
                ? [{ required: true, message: `${field.title || fieldName} is required` }]
                : [];

            if (field.enum && field.enum.length > 0) {
                return (
                    <Form.Item
                        key={fieldName}
                        name={fieldName}
                        label={field.title || fieldName}
                        tooltip={field.description}
                        rules={rules}
                    >
                        <Select options={field.enum.map((value) => ({ value, label: value }))} />
                    </Form.Item>
                );
            }

            if (field.type === 'boolean') {
                return (
                    <Form.Item
                        key={fieldName}
                        name={fieldName}
                        label={field.title || fieldName}
                        tooltip={field.description}
                        valuePropName="checked"
                        rules={rules}
                    >
                        <Switch />
                    </Form.Item>
                );
            }

            if (field.format === 'password') {
                return (
                    <Form.Item
                        key={fieldName}
                        name={fieldName}
                        label={field.title || fieldName}
                        tooltip={field.description}
                        rules={rules}
                    >
                        <Input.Password placeholder={field.description} />
                    </Form.Item>
                );
            }

            return (
                <Form.Item
                    key={fieldName}
                    name={fieldName}
                    label={field.title || fieldName}
                    tooltip={field.description}
                    rules={rules}
                >
                    <Input placeholder={field.description} />
                </Form.Item>
            );
        });
    };

    if (!mounted) {
        return null;
    }

    const externalProviders: LoginAuthProvider[] = providersQuery.data?.items ?? [];

    return (
        <div
            data-testid="login-page"
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

                {externalProviders.length > 0 && (
                    <>
                        <Divider plain style={{ margin: '24px 0' }}>
                            {t('auth.or_continue_with')}
                        </Divider>
                        <Space direction="vertical" size="middle" style={{ width: '100%' }}>
                            {externalProviders.map((provider) => {
                                const modes = resolveLoginModes(provider);
                                return modes.map((mode) => {
                                    const buttonKey = `${provider.id}:${mode.key}`;
                                    const label = modes.length > 1
                                        ? `${provider.display_name} · ${mode.display_name}`
                                        : provider.display_name;
                                    return (
                                        <Button
                                            key={buttonKey}
                                            block
                                            size="large"
                                            onClick={() => handleProviderLoginMode(provider, mode)}
                                            loading={externalLoadingKey === buttonKey}
                                        >
                                            {label}
                                        </Button>
                                    );
                                });
                            })}
                            {providersQuery.isLoading && (
                                <Text type="secondary">{t('message.loading')}</Text>
                            )}
                        </Space>
                    </>
                )}
            </Card>
            <Modal
                open={Boolean(credentialModal)}
                title={credentialModal ? `${credentialModal.provider.display_name} · ${credentialModal.mode.display_name}` : t('auth.login')}
                onCancel={() => {
                    if (!credentialSubmitting) {
                        setCredentialModal(null);
                    }
                }}
                onOk={handleCredentialSubmit}
                confirmLoading={credentialSubmitting}
                destroyOnHidden={false}
            >
                <Form form={credentialForm} layout="vertical">
                    {credentialModal ? renderCredentialFields(credentialModal.mode) : null}
                </Form>
            </Modal>
        </div>
    );
}
