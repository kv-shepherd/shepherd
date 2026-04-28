import type { TFunction } from 'i18next';

import { translateI18nMessage } from '@/lib/i18n/messages';
import type { components } from '@/types/api.gen';

type Notification = components['schemas']['Notification'];

interface NotificationDisplay {
    title: string;
    message: string;
}

export function getNotificationDisplay(notification: Notification, t: TFunction<'common'>): NotificationDisplay {
    return {
        title: translateI18nMessage(t, notification.title_i18n, notification.title),
        message: translateI18nMessage(t, notification.message_i18n, notification.message),
    };
}
