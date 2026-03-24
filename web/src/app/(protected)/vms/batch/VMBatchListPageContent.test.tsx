import { render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

const useApiGetMock = vi.fn();
const useApiMutationMock = vi.fn();

vi.mock('next/navigation', () => ({
    useRouter: () => ({
        push: vi.fn(),
    }),
}));

vi.mock('react-i18next', () => ({
    useTranslation: () => ({
        t: (key: string, options?: { defaultValue?: string; count?: number }) => {
            const labels: Record<string, string> = {
                'batch.list_title': 'Batch Operations',
                'batch.list_subtitle': 'History of batch VM power operations.',
                'common:button.refresh': 'Refresh',
                'batch.summary': 'Batch',
                'batch.id': 'Batch ID',
                'batch.operation': 'Operation',
                'batch.status': 'Status',
                'batch.child_count': 'Children',
                'batch.success_count': 'Success',
                'batch.failed_count': 'Failed',
                'common:table.created_at': 'Created',
                'common:table.actions': 'Actions',
                'batch.retry_failed': 'Retry failed children',
                'batch.cancel_pending': 'Cancel pending children',
                'common:message.error': 'Error',
            };
            if (key === 'batch.request_count') {
                return `${options?.count ?? 0} tasks`;
            }
            return labels[key] ?? options?.defaultValue ?? key;
        },
    }),
}));

vi.mock('@/lib/api/useApiGet', () => ({
    useApiGet: (...args: unknown[]) => useApiGetMock(...args),
}));

vi.mock('@/lib/api/useApiMutation', () => ({
    useApiMutation: (...args: unknown[]) => useApiMutationMock(...args),
}));

vi.mock('@/lib/api/client', () => ({
    api: {
        GET: vi.fn(),
        POST: vi.fn(),
    },
}));

vi.mock('@/lib/hooks/useMessage', () => ({
    useMessage: () => ({
        messageApi: {
            success: vi.fn(),
            error: vi.fn(),
        },
        messageContextHolder: null,
    }),
}));

import VMBatchListPage from './VMBatchListPageContent';

describe('VMBatchListPage', () => {
    it('renders the batch page shell and status table', () => {
        useApiGetMock.mockReturnValue({
            data: {
                items: [
                    {
                        id: 'batch-1234567890',
                        operation: 'START',
                        status: 'IN_PROGRESS',
                        child_count: 3,
                        success_count: 1,
                        failed_count: 1,
                        pending_count: 1,
                        created_at: '2026-03-17T00:00:00Z',
                    },
                ],
            },
            isLoading: false,
            refetch: vi.fn(),
        });
        useApiMutationMock.mockReturnValue({
            isPending: false,
            mutate: vi.fn(),
        });

        render(<VMBatchListPage />);

        expect(screen.getByTestId('vm-batch-page')).toBeVisible();
        expect(screen.getByText('Batch Operations')).toBeVisible();
        expect(screen.getByText('History of batch VM power operations.')).toBeVisible();
        expect(screen.getByText('START')).toBeVisible();
        expect(screen.getByText('IN_PROGRESS')).toBeVisible();
        expect(screen.getByText('Batch ID: batch-12…7890')).toBeVisible();
        expect(screen.getByTestId('batch-action-detail-batch-1234567890')).toBeVisible();
    });
});
