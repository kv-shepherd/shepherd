import { render, screen } from '@testing-library/react';
import type { ReactNode } from 'react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

const { pushMock, useApiGetMock, useApiActionMock } = vi.hoisted(() => ({
    pushMock: vi.fn(),
    useApiGetMock: vi.fn(),
    useApiActionMock: vi.fn(),
}));

vi.mock('next/navigation', () => ({
    useRouter: () => ({ push: pushMock }),
}));

vi.mock('react-i18next', () => ({
    useTranslation: () => ({
        t: (key: string, options?: { count?: number; defaultValue?: string; text?: string }) => {
            const labels: Record<string, string> = {
                'notification.title': 'Notifications',
                'notification.markAllRead': 'Mark all read',
                'notification.viewAll': 'View all',
                'notification.empty': 'No notifications',
                'notification.type.approval_pending': 'Pending Approval',
                'notification.message.legacy.title': String(options?.text ?? ''),
                'notification.message.legacy.body': String(options?.text ?? ''),
            };
            return labels[key] ?? options?.defaultValue ?? key;
        },
    }),
}));

vi.mock('@/hooks/useApiQuery', () => ({
    useApiGet: (...args: unknown[]) => useApiGetMock(...args),
    useApiAction: (...args: unknown[]) => useApiActionMock(...args),
}));

vi.mock('antd', async () => {
    const actual = await vi.importActual<typeof import('antd')>('antd');
    return {
        ...actual,
        Popover: ({ children, content }: { children: ReactNode; content: ReactNode }) => (
            <div>
                {children}
                <div data-testid="notification-popover">{content}</div>
            </div>
        ),
        Tooltip: ({ children }: { children: ReactNode }) => <>{children}</>,
    };
});

import NotificationBell from './NotificationBell';

const buildNotification = (index: number) => ({
    id: `notification-${index}`,
    type: 'APPROVAL_PENDING' as const,
    title: `Notification ${index}`,
    title_i18n: {
        key: 'notification.message.legacy.title',
        params: { text: `Notification ${index}` },
    },
    message: `Notification message ${index}`,
    message_i18n: {
        key: 'notification.message.legacy.body',
        params: { text: `Notification message ${index}` },
    },
    resource_type: 'ticket',
    resource_id: `ticket-${index}`,
    read: false,
    created_at: '2026-04-27T00:00:00Z',
});

describe('NotificationBell', () => {
    beforeEach(() => {
        pushMock.mockReset();
        useApiGetMock.mockReset();
        useApiActionMock.mockReset();
        useApiActionMock.mockReturnValue({ mutate: vi.fn(), isPending: false });
        useApiGetMock.mockImplementation((queryKey: unknown[]) => {
            if (queryKey[1] === 'unread-count') {
                return { data: { count: 6 } };
            }
            return {
                data: {
                    items: Array.from({ length: 6 }, (_, index) => buildNotification(index + 1)),
                    pagination: { page: 1, per_page: 6, total: 6, total_pages: 1 },
                },
                isLoading: false,
            };
        });
    });

    it('limits the header popover to five preview notifications', () => {
        render(<NotificationBell />);

        expect(screen.getByText('Notification 1')).toBeVisible();
        expect(screen.getByText('Notification 5')).toBeVisible();
        expect(screen.queryByText('Notification 6')).not.toBeInTheDocument();
        expect(screen.getByText('1 more in inbox')).toBeVisible();
        expect(screen.getByTestId('notification-view-all')).toBeVisible();
    });

    it('requests only the preview limit plus one overflow item', () => {
        render(<NotificationBell />);

        expect(useApiGetMock).toHaveBeenCalledWith(
            ['notifications', 'list', 6],
            expect.any(Function),
            { enabled: false },
        );
    });
});
