import { act, render, screen } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

const useApiGetMock = vi.fn();
const useApiMutationMock = vi.fn();
const messageErrorMock = vi.fn();
const authState = vi.hoisted(() => ({
    permissions: ['platform:admin'] as string[],
}));

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
                count?: number;
                ids?: string;
                seconds?: number;
            },
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
                'batch.affected_ids_none': 'none returned',
            };
            if (key === 'batch.live_status_summary') {
                return `Batch ${options?.batch_id ?? ''} is ${options?.status ?? ''}`;
            }
            if (key === 'batch.retry_feedback') {
                return `Retry affected ${options?.count ?? 0}: ${options?.ids ?? ''}`;
            }
            if (key === 'batch.cancel_feedback') {
                return `Cancel affected ${options?.count ?? 0}: ${options?.ids ?? ''}`;
            }
            if (key === 'batch.rate_limited_wait') {
                return `Retry in ${options?.seconds ?? 0}s`;
            }
            if (key === 'batch.rate_limited_contact_admin') {
                return `Retry in ${options?.seconds ?? 0}s and contact an administrator`;
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
            error: messageErrorMock,
        },
        messageContextHolder: null,
    }),
}));

vi.mock('@/stores/auth', () => ({
    useAuthStore: (
        selector: (state: {
            user: {
                id: string;
                username: string;
                permissions: string[];
            };
        }) => unknown,
    ) =>
        selector({
            user: {
                id: 'user-1',
                username: 'operator',
                permissions: authState.permissions,
            },
        }),
}));

import VMBatchDetailPage from './VMBatchDetailPageContent';

describe('VMBatchDetailPage', () => {
    beforeEach(() => {
        vi.clearAllMocks();
        authState.permissions = ['platform:admin'];
    });

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

    it('disables retry when failures are approval rejections or exhausted', () => {
        useApiGetMock.mockReturnValue({
            data: {
                id: 'batch-1',
                batch_id: 'batch-1',
                status: 'FAILED',
                operation: 'CREATE',
                child_count: 2,
                success_count: 0,
                failed_count: 2,
                pending_count: 0,
                created_by: 'owner-1',
                created_at: '2026-03-17T00:00:00Z',
                updated_at: '2026-03-17T00:05:00Z',
                children: [
                    {
                        ticket_id: 'ticket-rejected',
                        event_id: 'event-rejected',
                        resource_name: 'vm-rejected',
                        status: 'REJECTED',
                        attempt_count: 0,
                    },
                    {
                        ticket_id: 'ticket-exhausted',
                        event_id: 'event-exhausted',
                        resource_name: 'vm-exhausted',
                        status: 'FAILED',
                        attempt_count: 3,
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

        expect(screen.getByTestId('batch-retry-button')).toBeDisabled();
    });

    it('allows retry while a batch is in progress when a failed child remains eligible', () => {
        useApiGetMock.mockReturnValue({
            data: {
                id: 'batch-1',
                batch_id: 'batch-1',
                status: 'IN_PROGRESS',
                operation: 'POWER',
                child_count: 2,
                success_count: 0,
                failed_count: 1,
                pending_count: 1,
                created_by: 'owner-1',
                created_at: '2026-03-17T00:00:00Z',
                updated_at: '2026-03-17T00:05:00Z',
                children: [
                    {
                        ticket_id: 'ticket-failed',
                        event_id: 'event-failed',
                        resource_name: 'vm-failed',
                        status: 'FAILED',
                    },
                    {
                        ticket_id: 'ticket-pending',
                        event_id: 'event-pending',
                        resource_name: 'vm-pending',
                        status: 'PENDING',
                        attempt_count: 0,
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

        expect(screen.getByTestId('batch-retry-button')).toBeEnabled();
    });

    it.each([
        ['retry', true, false],
        ['cancel', false, true],
    ])('disables retry and cancel while %s is pending', (_action, retryPending, cancelPending) => {
        useApiGetMock.mockReturnValue({
            data: {
                batch_id: 'batch-1',
                status: 'IN_PROGRESS',
                operation: 'CREATE',
                child_count: 2,
                success_count: 0,
                failed_count: 1,
                pending_count: 1,
                created_by: 'owner-1',
                created_at: '2026-03-17T00:00:00Z',
                updated_at: '2026-03-17T00:05:00Z',
                children: [
                    {
                        ticket_id: 'ticket-failed',
                        event_id: 'event-1',
                        status: 'FAILED',
                    },
                    {
                        ticket_id: 'ticket-pending',
                        event_id: 'event-2',
                        status: 'PENDING',
                    },
                ],
            },
            isLoading: false,
            refetch: vi.fn(),
        });
        useApiMutationMock
            .mockReturnValueOnce({ isPending: retryPending, mutate: vi.fn() })
            .mockReturnValueOnce({ isPending: cancelPending, mutate: vi.fn() });

        render(<VMBatchDetailPage />);

        expect(screen.getByTestId('batch-retry-button')).toBeDisabled();
        expect(screen.getByTestId('batch-cancel-button')).toBeDisabled();
    });

    it('reports retry and cancel failures instead of failing silently', () => {
        messageErrorMock.mockClear();
        useApiGetMock.mockReturnValue({
            data: {
                id: 'batch-1',
                batch_id: 'batch-1',
                status: 'IN_PROGRESS',
                operation: 'CREATE',
                child_count: 1,
                success_count: 0,
                failed_count: 1,
                pending_count: 0,
                created_by: 'owner-1',
                created_at: '2026-03-17T00:00:00Z',
                updated_at: '2026-03-17T00:05:00Z',
                children: [
                    {
                        ticket_id: 'ticket-failed',
                        event_id: 'event-failed',
                        resource_name: 'vm-failed',
                        status: 'FAILED',
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

        const calls = useApiMutationMock.mock.calls.slice(-2) as Array<
            [unknown, { onError?: (error: { code: string; message: string }) => void }]
        >;
        calls[0]?.[1].onError?.({
            code: 'BATCH_RETRY_REVIEW_REQUIRED',
            message: 'review is required',
        });
        calls[1]?.[1].onError?.({
            code: 'BATCH_ACTION_NOT_APPLICABLE',
            message: 'batch state changed',
        });

        expect(messageErrorMock).toHaveBeenNthCalledWith(1, 'review is required');
        expect(messageErrorMock).toHaveBeenNthCalledWith(2, 'batch state changed');
    });

    it('polls non-terminal batches and stops polling terminal batches', () => {
        useApiGetMock.mockReturnValue({
            data: undefined,
            isLoading: true,
            refetch: vi.fn(),
        });
        useApiMutationMock.mockReturnValue({ isPending: false, mutate: vi.fn() });

        render(<VMBatchDetailPage />);
        const options = useApiGetMock.mock.calls[0]?.[2] as {
            refetchInterval: (query: { state: { data?: { status?: string } } }) => number | false;
        };
        expect(options.refetchInterval({ state: { data: { status: 'IN_PROGRESS' } } })).toBe(2_000);
        expect(options.refetchInterval({ state: { data: { status: 'COMPLETED' } } })).toBe(false);
        expect(options.refetchInterval({ state: { data: { status: 'FAILED' } } })).toBe(false);
    });

    it('shows affected ids, applies cooldown, and refetches after a 409 conflict', () => {
        const refetch = vi.fn();
        const mutationOptions: Array<{
            onSuccess?: (
                response: {
                    batch_id: string;
                    status: string;
                    affected_count: number;
                    affected_ticket_ids?: string[];
                },
                submission: { targetTicketIDs: string[] },
            ) => void;
            onError?: (error: {
                code: string;
                status?: number;
                retry_after_seconds?: number;
                params?: Record<string, unknown>;
            }) => void;
        }> = [];
        useApiGetMock.mockReturnValue({
            data: {
                batch_id: 'batch-1',
                status: 'IN_PROGRESS',
                operation: 'CREATE',
                child_count: 2,
                success_count: 0,
                failed_count: 1,
                pending_count: 1,
                created_by: 'owner-1',
                created_at: '2026-03-17T00:00:00Z',
                updated_at: '2026-03-17T00:05:00Z',
                children: [
                    { ticket_id: 'ticket-failed', event_id: 'event-1', status: 'FAILED' },
                    {
                        ticket_id: 'ticket-pending',
                        event_id: 'event-2',
                        status: 'PENDING',
                    },
                ],
            },
            isLoading: false,
            refetch,
        });
        useApiMutationMock.mockImplementation((_fn, options) => {
            mutationOptions.push(options);
            return { isPending: false, mutate: vi.fn() };
        });

        render(<VMBatchDetailPage />);
        act(() => {
            mutationOptions[0]?.onSuccess?.(
                {
                    batch_id: 'batch-1',
                    status: 'IN_PROGRESS',
                    affected_count: 1,
                    affected_ticket_ids: ['ticket-failed'],
                },
                { targetTicketIDs: ['ticket-failed'] },
            );
        });
        expect(screen.getByTestId('batch-action-feedback')).toHaveTextContent('Retry affected 1: ticket-failed');

        act(() => {
            mutationOptions[1]?.onError?.({
                code: 'BATCH_RATE_LIMITED',
                retry_after_seconds: 8,
                params: { contact_admin: true },
            });
        });
        expect(screen.getByTestId('batch-action-cooldown')).toHaveTextContent(
            'Retry in 8s and contact an administrator',
        );
        expect(screen.getByTestId('batch-retry-button')).toBeDisabled();
        expect(screen.getByTestId('batch-cancel-button')).toBeDisabled();

        act(() => {
            mutationOptions[0]?.onError?.({
                code: 'BATCH_ACTION_NOT_APPLICABLE',
                status: 409,
            });
        });
        expect(refetch).toHaveBeenCalled();
    });

    it('shows ambiguous restart retry metadata as read-only runbook guidance', () => {
        const mutationOptions: Array<{
            onError?: (error: { code: string; status?: number; params?: Record<string, unknown> }) => void;
        }> = [];
        useApiGetMock.mockReturnValue({
            data: {
                batch_id: 'batch-1',
                status: 'FAILED',
                operation: 'POWER',
                child_count: 1,
                success_count: 0,
                failed_count: 1,
                pending_count: 0,
                created_by: 'owner-1',
                created_at: '2026-03-17T00:00:00Z',
                updated_at: '2026-03-17T00:05:00Z',
                children: [
                    {
                        ticket_id: 'ticket-failed',
                        event_id: 'event-detail-restart',
                        status: 'FAILED',
                        attempt_count: 1,
                    },
                ],
            },
            isLoading: false,
            refetch: vi.fn(),
        });
        useApiMutationMock.mockImplementation((_fn, options) => {
            mutationOptions.push(options);
            return { isPending: false, mutate: vi.fn() };
        });

        render(<VMBatchDetailPage />);
        act(() => {
            mutationOptions[0]?.onError?.({
                code: 'DUPLICATE_PENDING_REQUEST',
                status: 409,
                params: {
                    operator_action_required: true,
                    existing_event_id: 'event-detail-restart',
                    reconciliation_path: 'operator-runbook:ambiguous-vm-restart',
                },
            });
        });

        expect(screen.getByTestId('restart-reconciliation-alert')).toHaveTextContent('event-detail-restart');
        expect(screen.getByTestId('restart-reconciliation-alert')).toHaveTextContent(
            'operator-runbook:ambiguous-vm-restart',
        );
        expect(screen.queryByText('restart_reconciliation.dismiss')).not.toBeInTheDocument();
    });

    it('enforces operation permissions and allows builtin approvers', () => {
        useApiGetMock.mockReturnValue({
            data: {
                batch_id: 'batch-1',
                status: 'IN_PROGRESS',
                operation: 'DELETE',
                child_count: 2,
                success_count: 0,
                failed_count: 1,
                pending_count: 1,
                created_by: 'owner-1',
                created_at: '2026-03-17T00:00:00Z',
                updated_at: '2026-03-17T00:05:00Z',
                children: [
                    { ticket_id: 'ticket-failed', event_id: 'event-1', status: 'FAILED' },
                    {
                        ticket_id: 'ticket-pending',
                        event_id: 'event-2',
                        status: 'PENDING',
                    },
                ],
            },
            isLoading: false,
            refetch: vi.fn(),
        });
        useApiMutationMock.mockReturnValue({ isPending: false, mutate: vi.fn() });
        authState.permissions = ['vm:operate'];
        const view = render(<VMBatchDetailPage />);

        expect(screen.getByTestId('batch-retry-button')).toBeDisabled();
        expect(screen.getByTestId('batch-cancel-button')).toBeDisabled();

        authState.permissions = ['builtin_approval:approve'];
        view.rerender(<VMBatchDetailPage />);
        expect(screen.getByTestId('batch-retry-button')).toBeEnabled();
        expect(screen.getByTestId('batch-cancel-button')).toBeEnabled();
    });
});
