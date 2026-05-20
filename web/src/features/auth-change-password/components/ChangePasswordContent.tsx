'use client';

import { useState } from 'react';
import Image from 'next/image';
import { Alert, Button, Card, ConfigProvider, Form, Input, Progress, Typography } from 'antd';
import { CheckCircleFilled, CloseCircleFilled, LockOutlined } from '@ant-design/icons';
import { useTranslation } from 'react-i18next';

import {
    type ChangePasswordFormValues,
    useChangePasswordController,
} from '../hooks/useChangePasswordController';
import LanguageSwitcher from '@/components/ui/LanguageSwitcher';
import { useAuthStore } from '@/stores/auth';

const { Title, Text } = Typography;

// Mirrors backend NIST-mode common password blocklist
// (see internal/config/password_policy.go).
const COMMON_PASSWORDS = new Set([
    '123456',
    '12345678',
    '123456789',
    'admin',
    'admin123',
    'changeme',
    'letmein',
    'password',
    'password1',
    'password123',
    'qwerty',
    'secret',
    'welcome',
]);

function normalizeIdentityHint(raw: string | null | undefined): string {
    if (!raw) return '';
    const lower = raw.trim().toLowerCase();
    if (!lower) return '';
    const atIdx = lower.indexOf('@');
    if (atIdx > 0) {
        const local = lower.slice(0, atIdx).trim();
        if (local) return local;
    }
    return lower;
}

interface PasswordChecks {
    minLength: boolean;
    notCommon: boolean;
    notIdentifier: boolean;
}

function evaluatePassword(pw: string, hints: string[]): PasswordChecks {
    const trimmed = pw.trim();
    const lower = trimmed.toLowerCase();
    const hasValue = trimmed !== '';
    const minLength = pw.length >= 8;
    const notCommon = hasValue && !COMMON_PASSWORDS.has(lower);
    const notIdentifier = hasValue && !hints.some((h) => h !== '' && h === lower);
    return { minLength, notCommon, notIdentifier };
}

function calcPasswordStrength(pw: string): { percent: number; color: string; labelKey: string } {
    if (!pw) return { percent: 0, color: '#e2e8f0', labelKey: '' };
    let score = 0;
    if (pw.length >= 8) score++;
    if (pw.length >= 12) score++;
    if (/[A-Z]/.test(pw)) score++;
    if (/[0-9]/.test(pw)) score++;
    if (/[^A-Za-z0-9]/.test(pw)) score++;
    if (score <= 1) return { percent: 25, color: '#ef4444', labelKey: 'auth.password_strength_weak' };
    if (score <= 2) return { percent: 50, color: '#f97316', labelKey: 'auth.password_strength_fair' };
    if (score <= 3) return { percent: 75, color: '#3b82f6', labelKey: 'auth.password_strength_strong' };
    return { percent: 100, color: '#10b981', labelKey: 'auth.password_strength_very_strong' };
}

export function ChangePasswordContent() {
    const { t } = useTranslation('common');
    const controller = useChangePasswordController();
    const user = useAuthStore((state) => state.user);
    const identityHints = [
        normalizeIdentityHint(user?.username),
        normalizeIdentityHint(user?.email),
        normalizeIdentityHint(user?.display_name),
    ];
    const [newPassword, setNewPassword] = useState('');
    const strength = calcPasswordStrength(newPassword);
    const checks = evaluatePassword(newPassword, identityHints);

    return (
        <ConfigProvider theme={{
            token: {
                colorPrimary: '#1677ff',
                colorBgContainer: '#ffffff',
                colorBgElevated: '#ffffff',
                colorBorder: '#e2e8f0',
                borderRadius: 12,
                fontFamily: 'var(--font-ui-sans)',
            },
        }}>
            <div className="auth-shell" data-testid="change-password-page">
                <div className="auth-shell__single">
                    <div className="auth-shell__single-logo">
                        <Image
                            src="/logo-wide.svg"
                            alt="Shepherd"
                            width={160}
                            height={46}
                        />
                    </div>
                    <Card className="auth-shell__card">
                        <div className="auth-shell__card-toolbar">
                            <LanguageSwitcher dataTestId="change-password-language-switcher" />
                        </div>
                        <div className="auth-shell__card-header">
                            <Text className="auth-shell__card-eyebrow">
                                {t('auth.security')}
                            </Text>
                            <Title level={3} className="auth-shell__card-title">
                                {t('auth.change_password')}
                            </Title>
                            <Text className="auth-shell__card-subtitle">
                                {t('auth.change_password_hint')}
                            </Text>
                        </div>

                        {controller.error && (
                            <Alert
                                message={controller.error}
                                type="error"
                                showIcon
                                closable
                                onClose={() => controller.setError(null)}
                                className="auth-shell__error"
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
                                    {
                                        validator: (_, value: string) => {
                                            if (!value) return Promise.resolve();
                                            const lower = value.trim().toLowerCase();
                                            if (COMMON_PASSWORDS.has(lower)) {
                                                return Promise.reject(new Error(t('validation.password_too_common')));
                                            }
                                            if (lower && identityHints.some((h) => h !== '' && h === lower)) {
                                                return Promise.reject(new Error(t('validation.password_matches_identifier')));
                                            }
                                            return Promise.resolve();
                                        },
                                    },
                                ]}
                            >
                                <Input.Password
                                    prefix={<LockOutlined />}
                                    placeholder={t('auth.new_password')}
                                    onChange={(e) => setNewPassword(e.target.value)}
                                />
                            </Form.Item>

                            {newPassword && (
                                <div className="change-password__strength">
                                    <div className="change-password__strength-row">
                                        <Text style={{ fontSize: 12, color: '#64748b' }}>
                                            {t('auth.password_strength')}:
                                        </Text>
                                        <Text style={{ fontSize: 12, color: strength.color, fontWeight: 600 }}>
                                            {t(strength.labelKey)}
                                        </Text>
                                    </div>
                                    <Progress
                                        percent={strength.percent}
                                        strokeColor={strength.color}
                                        showInfo={false}
                                        size="small"
                                        trailColor="#e2e8f0"
                                    />
                                </div>
                            )}

                            <div className="change-password__requirements">
                                <Text className="change-password__requirements-title">
                                    {t('auth.password_requirements')}
                                </Text>
                                <ul className="change-password__requirements-list">
                                    <RequirementItem
                                        ok={checks.minLength}
                                        label={t('auth.password_req_min_length')}
                                    />
                                    <RequirementItem
                                        ok={checks.notCommon}
                                        label={t('auth.password_req_not_common')}
                                    />
                                    <RequirementItem
                                        ok={checks.notIdentifier}
                                        label={t('auth.password_req_not_identifier')}
                                    />
                                </ul>
                            </div>

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
                    </Card>
                </div>
            </div>
        </ConfigProvider>
    );
}

function RequirementItem({ ok, label }: { ok: boolean; label: string }) {
    return (
        <li className={`change-password__requirement${ok ? ' is-ok' : ''}`}>
            {ok
                ? <CheckCircleFilled style={{ color: '#10b981' }} />
                : <CloseCircleFilled style={{ color: '#cbd5e1' }} />}
            <span>{label}</span>
        </li>
    );
}
