import { act, renderHook } from '@testing-library/react';
import type { TFunction } from 'i18next';
import { beforeEach, describe, expect, it, vi } from 'vitest';

const {
    useApiGetMock,
    useApiActionMock,
    markReadMutate,
    markAllMutate,
    messageSuccessMock,
    messageErrorMock,
} = vi.hoisted(() => ({
    useApiGetMock: vi.fn(),
    useApiActionMock: vi.fn(),
    markReadMutate: vi.fn(),
    markAllMutate: vi.fn(),
    messageSuccessMock: vi.fn(),
    messageErrorMock: vi.fn(),
}));

vi.mock('antd', () => ({
    message: {
        useMessage: () => [
            {
                success: messageSuccessMock,
                error: messageErrorMock,
            },
            null,
        ],
    },
}));

vi.mock('@/hooks/useApiQuery', () => ({
    useApiGet: (...args: unknown[]) => useApiGetMock(...args),
    useApiAction: (...args: unknown[]) => useApiActionMock(...args),
}));

import { useNotificationsController } from './useNotificationsController';

describe('useNotificationsController', () => {
    const t = ((key: string) => key) as unknown as TFunction;

    beforeEach(() => {
        vi.clearAllMocks();

        let getCall = 0;
        useApiGetMock.mockImplementation(() => {
            getCall += 1;
            if (getCall % 2 === 1) {
                return {
                    data: {
                        items: [{ id: 'n-1', read: false, type: 'APPROVAL_PENDING' }],
                        pagination: { page: 1, per_page: 20, total: 1, total_pages: 1 },
                    },
                    isLoading: false,
                    refetch: vi.fn(),
                };
            }
            return {
                data: { count: 3 },
                isLoading: false,
                refetch: vi.fn(),
            };
        });

        let actionCall = 0;
        useApiActionMock.mockImplementation(() => {
            actionCall += 1;
            if (actionCall % 2 === 1) {
                return { mutate: markReadMutate, isPending: false };
            }
            return { mutate: markAllMutate, isPending: false };
        });
    });

    it('toggles unread filter and updates pagination state', () => {
        const { result } = renderHook(() => useNotificationsController({ t }));

        expect(result.current.unreadOnly).toBe(false);
        expect(result.current.unreadCount).toBe(3);

        act(() => {
            result.current.setUnreadOnly(true);
            result.current.setPage(2);
            result.current.setPageSize(50);
        });

        expect(result.current.unreadOnly).toBe(true);
        expect(result.current.page).toBe(2);
        expect(result.current.pageSize).toBe(50);
    });

    it('dispatches mark read and mark all actions', () => {
        const { result } = renderHook(() => useNotificationsController({ t }));

        act(() => {
            result.current.markRead('n-1');
            result.current.markAllRead();
        });

        expect(markReadMutate).toHaveBeenCalledWith('n-1');
        expect(markAllMutate).toHaveBeenCalled();
    });
});
