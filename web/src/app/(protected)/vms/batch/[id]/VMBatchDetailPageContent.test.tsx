import { render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

const useApiGetMock = vi.fn();
const useApiMutationMock = vi.fn();

vi.mock('next/navigation', () => ({
    useParams: () => ({ id: 'batch-1' }),
    useRouter: () => ({
        push: vi.fn(),
    }),
}));

vi.mock('react-i18next', () => ({
    useTranslation: () => ({
        t: (
            key: string,
            options?: {
                defaultValue?: string;
                batch_id?: string;
                status?: string;
                success_count?: number;
                failed_count?: number;
                pending_count?: number;
            }
        ) => {
            const labels: Record<string, string> = {
                'batch.list_title': 'Batch Operations',
                'batch.detail_title': 'Batch Operation Detail',
                'batch.detail_subtitle': 'Inspect child task status and retry or cancel the batch when needed.',
                'batch.status': 'Status',
                'batch.operation': 'Operation',
                'batch.child_count': 'Children',
                'batch.success_count': 'Success',
                'batch.failed_count': 'Failed',
                'batch.pending_count': 'Pending',
                'batch.retry_failed': 'Retry failed',
                'batch.cancel_pending': 'Cancel pending',
                'batch.export_result': 'Export result',
                'batch.child.title': 'Child Tasks',
                'batch.child.ticket': 'Ticket',
                'batch.child.resource': 'Resource',
                'batch.child.status': 'Task Status',
                'batch.child.attempt': 'Attempts',
                'batch.child.error': 'Last Error',
            };
            if (key === 'batch.live_status_summary') {
                return `Batch ${options?.batch_id ?? ''} is ${options?.status ?? ''}`;
            }
            return labels[key] ?? options?.defaultValue ?? key;
        },
    }),
}));

vi.mock('@/lib/api/useApiGet', () => ({
    useApiGet: (...args: unknown[]) => useApiGetMock(...args),
}));

vi.mock('@/hooks/useApiQuery', () => ({
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
        },
        messageContextHolder: null,
    }),
}));

import VMBatchDetailPage from './VMBatchDetailPageContent';

describe('VMBatchDetailPage', () => {
    it('renders the batch detail shell, summary, and child task table', () => {
        useApiGetMock.mockReturnValue({
            data: {
                id: 'batch-1',
                status: 'PARTIAL_SUCCESS',
                operation: 'CREATE',
                child_count: 3,
                success_count: 1,
                failed_count: 1,
                pending_count: 1,
                children: [
                    {
                        ticket_id: 'ticket-1',
                        resource_name: 'vm-a',
                        status: 'FAILED',
                        attempt_count: 2,
                        last_error: 'quota exceeded',
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

        render(<VMBatchDetailPage />);

        expect(screen.getByTestId('vm-batch-detail-page')).toBeVisible();
        expect(screen.getByText('Batch Operation Detail')).toBeVisible();
        expect(screen.getByText('Inspect child task status and retry or cancel the batch when needed.')).toBeVisible();
        expect(screen.getByTestId('batch-status-live')).toHaveTextContent('Batch batch-1 is PARTIAL_SUCCESS');
        expect(screen.getByText('Child Tasks')).toBeVisible();
        expect(screen.getByText('vm-a')).toBeVisible();
        expect(screen.getByTestId('batch-export-button')).toBeEnabled();
        expect(screen.getByTestId('batch-retry-button')).toBeVisible();
        expect(screen.getByTestId('batch-cancel-button')).toBeVisible();
    });

    it('does not export cancelled batches', () => {
        useApiGetMock.mockReturnValue({
            data: {
                id: 'batch-1',
                batch_id: 'batch-1',
                status: 'CANCELLED',
                operation: 'DELETE',
                child_count: 1,
                success_count: 0,
                failed_count: 0,
                pending_count: 1,
                created_by: 'owner-1',
                created_at: '2026-03-17T00:00:00Z',
                updated_at: '2026-03-17T00:05:00Z',
                children: [],
            },
            isLoading: false,
            refetch: vi.fn(),
        });
        useApiMutationMock.mockReturnValue({
            isPending: false,
            mutate: vi.fn(),
        });

        render(<VMBatchDetailPage />);

        expect(screen.getByTestId('batch-export-button')).toBeDisabled();
    });
});
