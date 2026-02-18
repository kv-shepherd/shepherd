'use client';

import { useState } from 'react';
import { useRouter } from 'next/navigation';
import { useTranslation } from 'react-i18next';

import { api } from '@/lib/api/client';
import { useAuthStore } from '@/stores/auth';

export interface ChangePasswordFormValues {
    current_password: string;
    new_password: string;
    confirm_password: string;
}

export function useChangePasswordController() {
    const { t: tErrors } = useTranslation('errors');
    const router = useRouter();
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState<string | null>(null);

    const submit = async (values: ChangePasswordFormValues) => {
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

            useAuthStore.getState().clearForcePasswordChange();
            router.push('/dashboard');
        } catch {
            setError(tErrors('INTERNAL_ERROR'));
        } finally {
            setLoading(false);
        }
    };

    return {
        loading,
        error,
        setError,
        submit,
    };
}
