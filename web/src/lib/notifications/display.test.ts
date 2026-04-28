import { describe, expect, it } from 'vitest';
import type { TFunction } from 'i18next';

import type { components } from '@/types/api.gen';
import { getNotificationDisplay } from './display';

type Notification = components['schemas']['Notification'];

const t: TFunction<'common'> = ((key: string, options?: Record<string, unknown>) => {
    if (key === 'notification.message.approval_pending.title') {
        return 'Pending approval';
    }
    if (key === 'notification.message.approval_pending.body') {
        return `${options?.requester} requested a VM in ${options?.namespace}`;
    }
    return String(options?.defaultValue ?? key);
}) as TFunction<'common'>;

const baseNotification: Notification = {
    id: 'notification-1',
    type: 'APPROVAL_PENDING',
    title: 'New VM request pending approval',
    title_i18n: {
        key: 'notification.message.approval_pending.title',
    },
    message: 'User alice submitted a VM request in namespace prod',
    message_i18n: {
        key: 'notification.message.approval_pending.body',
        params: {
            requester: 'alice',
            namespace: 'prod',
        },
    },
    read: false,
    created_at: '2026-04-27T00:00:00Z',
};

describe('getNotificationDisplay', () => {
    it('renders notification text from the i18n contract', () => {
        expect(getNotificationDisplay(baseNotification, t)).toEqual({
            title: 'Pending approval',
            message: 'alice requested a VM in prod',
        });
    });

    it('falls back to API fallback text when the key is not translated', () => {
        expect(getNotificationDisplay({
            ...baseNotification,
            title: 'Fallback title',
            title_i18n: {
                key: 'notification.message.unknown.title',
            },
            message: 'Fallback message',
            message_i18n: {
                key: 'notification.message.unknown.body',
            },
        }, t)).toEqual({
            title: 'Fallback title',
            message: 'Fallback message',
        });
    });
});
