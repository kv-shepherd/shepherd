import type { components } from '@/types/api.gen';

type I18nMessage = components['schemas']['I18nMessage'];
type TranslationFn = (key: string, options?: Record<string, unknown>) => string;

export function translateI18nMessage(
    t: TranslationFn,
    message: I18nMessage | undefined,
    fallback: string,
): string {
    if (!message?.key) {
        return fallback;
    }
    const translated = t(message.key, {
        ...(message.params ?? {}),
        defaultValue: '',
    });
    return translated && translated !== message.key ? translated : fallback;
}
