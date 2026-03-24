import type { TFunction } from 'i18next';

import type { ApiErrorResponse } from '@/hooks/useApiQuery';

type ApiErrorLike = Partial<ApiErrorResponse> & {
    message?: string;
};

export function translateApiError(
    t: TFunction,
    error: ApiErrorLike | Error | undefined,
    fallbackKey = 'common:message.error',
): string {
    const fallback = t(fallbackKey, { defaultValue: fallbackKey });
    if (!error) {
        return fallback;
    }

    const apiError = error as ApiErrorLike;
    const params = apiError.params ?? {};
    if (typeof apiError.code === 'string' && apiError.code !== '') {
        const key = `errors:${apiError.code}`;
        const translated = t(key, { ...params, defaultValue: '' });
        if (translated && translated !== key) {
            return translated;
        }
    }

    if (typeof apiError.message === 'string' && apiError.message !== '') {
        return apiError.message;
    }

    if ('message' in error && typeof error.message === 'string' && error.message !== '') {
        return error.message;
    }

    return fallback;
}
