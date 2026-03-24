import { render, screen } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

const controllerState = vi.hoisted(() => ({
    overrides: {} as Record<string, unknown>,
}));

vi.mock('react-i18next', () => ({
    useTranslation: () => ({
        t: (key: string, options?: { total?: number }) => {
            const labels: Record<string, string> = {
                'notification.title': 'Notifications',
                'notification.subtitle': 'Track approvals and VM events',
                'notification.filter_all': 'All',
                'notification.filter_unread': 'Unread',
                'button.refresh': 'Refresh',
                'notification.markAllRead': 'Mark All Read',
                'table.status': 'Status',
                'notification.read': 'Read',
                'notification.unread': 'Unread',
                'notification.type': 'Type',
                'notification.type.approval_pending': 'Approval Pending',
                'table.name': 'Title',
                'table.created_at': 'Created',
                'table.actions': 'Actions',
                'notification.markRead': 'Mark Read',
                'notification.empty': 'No notifications',
                'table.total': `Total ${options?.total ?? 0}`,
            };
            return labels[key] ?? key;
        },
    }),
}));

vi.mock('../hooks/useNotificationsController', () => ({
    useNotificationsController: () => ({
        messageContextHolder: null,
        unreadCount: 1,
        unreadOnly: false,
        isLoading: false,
        page: 1,
        pageSize: 20,
        data: {
            items: [
                {
                    id: 'notif-1',
                    read: false,
                    type: 'APPROVAL_PENDING',
                    title: 'Approval pending',
                    message: 'A request is waiting for review.',
                    created_at: '2026-03-17T00:00:00Z',
                },
            ],
            pagination: { total: 1, page: 1, per_page: 20 },
        },
        setUnreadOnly: vi.fn(),
        setPage: vi.fn(),
        setPageSize: vi.fn(),
        refetch: vi.fn(),
        markAllRead: vi.fn(),
        markAllReadPending: false,
        markRead: vi.fn(),
        markReadPending: false,
        ...controllerState.overrides,
    }),
}));

import { NotificationsContent } from './NotificationsContent';

describe('NotificationsContent', () => {
    beforeEach(() => {
        controllerState.overrides = {};
    });

    it('renders page actions and the notification table', () => {
        render(<NotificationsContent />);

        expect(screen.getByText('Notifications')).toBeVisible();
        expect(screen.getByRole('button', { name: 'Mark All Read' })).toBeVisible();
        expect(screen.getByText('Approval pending')).toBeVisible();
        expect(screen.getByText('A request is waiting for review.')).toBeVisible();
        expect(screen.getByTestId('notification-action-read-notif-1')).toBeVisible();
    });

    it('renders the empty state when there are no notifications', () => {
        controllerState.overrides = {
            data: {
                items: [],
                pagination: { total: 0, page: 1, per_page: 20 },
            },
        };

        render(<NotificationsContent />);

        expect(screen.getByText('No notifications')).toBeVisible();
    });
});
