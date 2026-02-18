'use client';

import { message } from 'antd';
import type { TFunction } from 'i18next';
import { useState } from 'react';

import { useApiAction, useApiGet } from '@/hooks/useApiQuery';
import { api } from '@/lib/api/client';

import type { NotificationList, UnreadCount } from '../types';

interface UseNotificationsControllerArgs {
    t: TFunction;
}

export function useNotificationsController({ t }: UseNotificationsControllerArgs) {
    const [messageApi, messageContextHolder] = message.useMessage();
    const [page, setPage] = useState(1);
    const [pageSize, setPageSize] = useState(20);
    const [unreadOnly, setUnreadOnly] = useState(false);

    const notificationsQuery = useApiGet<NotificationList>(
        ['notifications', 'page', page, pageSize, unreadOnly],
        () => api.GET('/notifications', {
            params: {
                query: {
                    page,
                    per_page: pageSize,
                    unread_only: unreadOnly,
                },
            },
        }),
    );

    const unreadCountQuery = useApiGet<UnreadCount>(
        ['notifications', 'unread-count', 'page'],
        () => api.GET('/notifications/unread-count'),
        { refetchInterval: 30000 }
    );

    const markReadAction = useApiAction<string>(
        (notificationID) => api.PATCH('/notifications/{notification_id}/read', {
            params: { path: { notification_id: notificationID } },
        }),
        {
            invalidateKeys: [['notifications', 'page'], ['notifications', 'unread-count'], ['notifications', 'list']],
            onError: (err) => messageApi.error(err.message || t('common:message.error')),
        }
    );

    const markAllReadAction = useApiAction<void>(
        () => api.POST('/notifications/mark-all-read'),
        {
            invalidateKeys: [['notifications', 'page'], ['notifications', 'unread-count'], ['notifications', 'list']],
            onSuccess: () => messageApi.success(t('common:message.success')),
            onError: (err) => messageApi.error(err.message || t('common:message.error')),
        }
    );

    const markRead = (notificationID: string) => {
        markReadAction.mutate(notificationID);
    };

    const markAllRead = () => {
        markAllReadAction.mutate();
    };

    return {
        messageContextHolder,
        page,
        pageSize,
        setPage,
        setPageSize,
        unreadOnly,
        setUnreadOnly,
        data: notificationsQuery.data,
        unreadCount: unreadCountQuery.data?.count ?? 0,
        isLoading: notificationsQuery.isLoading,
        refetch: notificationsQuery.refetch,
        markRead,
        markAllRead,
        markReadPending: markReadAction.isPending,
        markAllReadPending: markAllReadAction.isPending,
    };
}
