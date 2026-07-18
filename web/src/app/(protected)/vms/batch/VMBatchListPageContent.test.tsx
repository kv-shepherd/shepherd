import { act, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';

const useApiGetMock = vi.fn();
const useApiMutationMock = vi.fn();
const authState = vi.hoisted(() => ({
    permissions: ['platform:admin'] as string[],
}));

vi.mock('next/navigation', () => ({
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
                count?: number;
                ids?: string;
                seconds?: number;
            },
        ) => {
            const labels: Record<string, string> = {
                'batch.list_title': 'Batch Operations',
                'batch.list_subtitle': 'History of batch VM operations.',
                'batch.search_placeholder': 'Search batch operations on the current page or paste a batch ID',
                'batch.search_help': 'Search applies only to rows loaded on the current page.',
                'batch.advanced_search_title': 'Filter current page',
                'batch.advanced_search_help': 'Exact filters apply only to the current page.',
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
                'batch.export_result': 'Export result',
                'batch.affected_ids_none': 'none returned',
                'common:message.error': 'Error',
            };
            if (key === 'batch.request_count') {
                return `${options?.count ?? 0} tasks`;
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
            error: vi.fn(),
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

import VMBatchListPage from './VMBatchListPageContent';
import { api } from '@/lib/api/client';

describe('VMBatchListPage', () => {
    beforeEach(() => {
        vi.clearAllMocks();
        authState.permissions = ['platform:admin'];
    });

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
        expect(screen.getByText('History of batch VM operations.')).toBeVisible();
        expect(screen.getByText('START')).toBeVisible();
        expect(screen.getByText('IN_PROGRESS')).toBeVisible();
        expect(screen.getByText('Batch ID: batch-12…7890')).toBeVisible();
        expect(screen.getByTestId('batch-action-export-batch-1234567890')).toBeDisabled();
        expect(screen.getByTestId('batch-action-detail-batch-1234567890')).toBeVisible();
    });

    it('requests controlled server pages and makes the twenty-first batch reachable', async () => {
        const user = userEvent.setup();
        useApiGetMock.mockImplementation((queryKey: readonly unknown[]) => {
            const requestedPage = queryKey[1] as number;
            return {
                data: {
                    items: [
                        {
                            id: requestedPage === 2 ? 'batch-row-21' : 'batch-row-01',
                            operation: 'START',
                            status: 'COMPLETED',
                            child_count: 1,
                            success_count: 1,
                            failed_count: 0,
                            pending_count: 0,
                            created_at: '2026-03-17T00:00:00Z',
                        },
                    ],
                    pagination: {
                        page: requestedPage,
                        per_page: 20,
                        total: 21,
                        total_pages: 2,
                    },
                },
                isLoading: false,
                refetch: vi.fn(),
            };
        });
        useApiMutationMock.mockReturnValue({ isPending: false, mutate: vi.fn() });
        vi.mocked(api.GET).mockResolvedValue({
            data: { items: [], pagination: { page: 1, per_page: 20, total: 21 } },
            response: new Response(null, { status: 200 }),
        } as never);

        const view = render(<VMBatchListPage />);

        expect(useApiGetMock.mock.calls[0]?.[0]).toEqual(['vm-batch-list', 1, 20]);
        const firstPageFetcher = useApiGetMock.mock.calls[0]?.[1] as () => Promise<unknown>;
        await firstPageFetcher();
        expect(api.GET).toHaveBeenCalledWith('/vms/batch', {
            params: { query: { page: 1, per_page: 20 } },
        });

        const pageTwoControl = view.container.querySelector('.ant-pagination-item-2');
        expect(pageTwoControl).not.toBeNull();
        await user.click(pageTwoControl as HTMLElement);

        await waitFor(() => {
            expect(screen.getByText('Batch ID: batch-row-21')).toBeVisible();
        });
        const secondPageCall = useApiGetMock.mock.calls.find(([queryKey]) => (queryKey as readonly unknown[])[1] === 2);
        expect(secondPageCall?.[0]).toEqual(['vm-batch-list', 2, 20]);
        await (secondPageCall?.[1] as () => Promise<unknown>)();
        expect(api.GET).toHaveBeenCalledWith('/vms/batch', {
            params: { query: { page: 2, per_page: 20 } },
        });
    });

    it.each([
        ['retry', true, false],
        ['cancel', false, true],
    ])('disables retry and cancel while %s is pending', (_action, retryPending, cancelPending) => {
        useApiGetMock.mockReturnValue({
            data: {
                items: [
                    {
                        id: 'batch-pending-action',
                        operation: 'START',
                        status: 'IN_PROGRESS',
                        child_count: 2,
                        success_count: 0,
                        failed_count: 1,
                        pending_count: 1,
                        created_at: '2026-03-17T00:00:00Z',
                    },
                ],
                pagination: { page: 1, per_page: 20, total: 1 },
            },
            isLoading: false,
            refetch: vi.fn(),
        });
        useApiMutationMock
            .mockReturnValueOnce({ isPending: retryPending, mutate: vi.fn() })
            .mockReturnValueOnce({ isPending: cancelPending, mutate: vi.fn() })
            .mockReturnValueOnce({ isPending: false, mutate: vi.fn() });

        render(<VMBatchListPage />);

        expect(screen.getByTestId('batch-action-retry-batch-pending-action')).toBeDisabled();
        expect(screen.getByTestId('batch-action-cancel-batch-pending-action')).toBeDisabled();
    });

    it('enables result export for completed, failed, and partial success batches only', () => {
        useApiGetMock.mockReturnValue({
            data: {
                items: [
                    {
                        id: 'batch-completed',
                        operation: 'POWER',
                        status: 'COMPLETED',
                        child_count: 1,
                        success_count: 1,
                        failed_count: 0,
                        pending_count: 0,
                        created_at: '2026-03-17T00:00:00Z',
                    },
                    {
                        id: 'batch-cancelled',
                        operation: 'DELETE',
                        status: 'CANCELLED',
                        child_count: 1,
                        success_count: 0,
                        failed_count: 0,
                        pending_count: 1,
                        created_at: '2026-03-18T00:00:00Z',
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

        expect(screen.getByTestId('batch-action-export-batch-completed')).toBeEnabled();
        expect(screen.getByTestId('batch-action-export-batch-cancelled')).toBeDisabled();
    });

    it('filters batch jobs through quick search only after submit', async () => {
        const user = userEvent.setup();
        useApiGetMock.mockReturnValue({
            data: {
                items: [
                    {
                        id: 'batch-start-1234567890',
                        operation: 'START',
                        status: 'IN_PROGRESS',
                        child_count: 3,
                        success_count: 1,
                        failed_count: 1,
                        pending_count: 1,
                        created_at: '2026-03-17T00:00:00Z',
                    },
                    {
                        id: 'batch-stop-0987654321',
                        operation: 'STOP',
                        status: 'COMPLETED',
                        child_count: 1,
                        success_count: 1,
                        failed_count: 0,
                        pending_count: 0,
                        created_at: '2026-03-18T00:00:00Z',
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

        await user.type(screen.getByTestId('batch-quick-search'), 'stop');
        expect(screen.getByText('STOP')).toBeVisible();
        expect(screen.getByText('START')).toBeVisible();

        await user.keyboard('{Enter}');
        expect(screen.getByText('STOP')).toBeVisible();
        expect(screen.queryByText('START')).not.toBeInTheDocument();
    });

    it('preflights child state and does not retry rejected or exhausted failures', async () => {
        let retryRequest:
            ((submission: { batchId: string; targetTicketIDs: string[] }) => Promise<unknown>) | undefined;
        useApiGetMock.mockReturnValue({
            data: {
                items: [
                    {
                        id: 'batch-rejected',
                        operation: 'CREATE',
                        status: 'FAILED',
                        child_count: 2,
                        success_count: 0,
                        failed_count: 2,
                        pending_count: 0,
                        created_at: '2026-03-17T00:00:00Z',
                    },
                ],
            },
            isLoading: false,
            refetch: vi.fn(),
        });
        useApiMutationMock.mockImplementation(
            (mutationFn: (submission: { batchId: string; targetTicketIDs: string[] }) => Promise<unknown>) => {
                retryRequest ??= mutationFn;
                return { isPending: false, mutate: vi.fn() };
            },
        );
        vi.mocked(api.GET).mockResolvedValue({
            data: {
                batch_id: 'batch-rejected',
                operation: 'CREATE',
                status: 'FAILED',
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
            response: new Response(null, { status: 200 }),
        } as never);
        vi.mocked(api.POST).mockClear();

        render(<VMBatchListPage />);

        expect(retryRequest).toBeDefined();
        const result = (await retryRequest?.({
            batchId: 'batch-rejected',
            targetTicketIDs: [],
        })) as {
            error?: { code?: string };
        };
        expect(result.error?.code).toBe('BATCH_NOTHING_TO_RETRY');
        expect(api.GET).toHaveBeenCalledWith('/vms/batch/{batch_id}', {
            params: { path: { batch_id: 'batch-rejected' } },
        });
        expect(api.POST).not.toHaveBeenCalled();
    });

    it('treats an omitted attempt count as the first retry attempt', async () => {
        let retryRequest:
            ((submission: { batchId: string; targetTicketIDs: string[] }) => Promise<unknown>) | undefined;
        useApiGetMock.mockReturnValue({
            data: {
                items: [
                    {
                        id: 'batch-first-retry',
                        operation: 'CREATE',
                        status: 'FAILED',
                        child_count: 1,
                        success_count: 0,
                        failed_count: 1,
                        pending_count: 0,
                        created_at: '2026-03-17T00:00:00Z',
                    },
                ],
            },
            isLoading: false,
            refetch: vi.fn(),
        });
        useApiMutationMock.mockImplementation(
            (mutationFn: (submission: { batchId: string; targetTicketIDs: string[] }) => Promise<unknown>) => {
                retryRequest ??= mutationFn;
                return { isPending: false, mutate: vi.fn() };
            },
        );
        vi.mocked(api.GET).mockResolvedValue({
            data: {
                batch_id: 'batch-first-retry',
                operation: 'CREATE',
                status: 'FAILED',
                child_count: 1,
                success_count: 0,
                failed_count: 1,
                pending_count: 0,
                created_by: 'owner-1',
                created_at: '2026-03-17T00:00:00Z',
                updated_at: '2026-03-17T00:05:00Z',
                children: [
                    {
                        ticket_id: 'ticket-first-retry',
                        event_id: 'event-first-retry',
                        resource_name: 'vm-first-retry',
                        status: 'FAILED',
                    },
                ],
            },
            response: new Response(null, { status: 200 }),
        } as never);
        vi.mocked(api.POST).mockResolvedValue({
            data: {
                batch_id: 'batch-first-retry',
                status: 'IN_PROGRESS',
                affected_count: 1,
            },
            response: new Response(null, { status: 200 }),
        } as never);

        render(<VMBatchListPage />);

        expect(retryRequest).toBeDefined();
        const result = await retryRequest?.({
            batchId: 'batch-first-retry',
            targetTicketIDs: [],
        });
        expect(result).toMatchObject({
            data: { batch_id: 'batch-first-retry', affected_count: 1 },
        });
        expect(api.POST).toHaveBeenCalledWith('/vms/batch/{batch_id}/retry', {
            params: { path: { batch_id: 'batch-first-retry' } },
        });
    });

    it('renders affected_count and authoritative affected ticket ids', () => {
        const mutationOptions: Array<{
            onSuccess?: (
                response: {
                    batch_id: string;
                    status: string;
                    affected_count: number;
                    affected_ticket_ids?: string[];
                },
                submission: {
                    batchId: string;
                    targetTicketIDs: string[];
                },
            ) => void;
        }> = [];
        useApiGetMock.mockReturnValue({
            data: {
                items: [
                    {
                        id: 'batch-feedback',
                        operation: 'CREATE',
                        status: 'FAILED',
                        child_count: 1,
                        success_count: 0,
                        failed_count: 1,
                        pending_count: 0,
                        created_at: '2026-03-17T00:00:00Z',
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

        render(<VMBatchListPage />);
        act(() => {
            mutationOptions[0]?.onSuccess?.(
                {
                    batch_id: 'batch-feedback',
                    status: 'IN_PROGRESS',
                    affected_count: 2,
                    affected_ticket_ids: ['ticket-a', 'ticket-b'],
                },
                { batchId: 'batch-feedback', targetTicketIDs: [] },
            );
        });

        expect(screen.getByTestId('batch-action-feedback')).toHaveTextContent('Retry affected 2: ticket-a, ticket-b');
    });

    it('honors BATCH_RATE_LIMITED Retry-After and refetches a conflicting state', () => {
        const refetch = vi.fn();
        const mutationOptions: Array<{
            onError?: (error: {
                code: string;
                status?: number;
                retry_after_seconds?: number;
                params?: Record<string, unknown>;
            }) => void;
        }> = [];
        useApiGetMock.mockReturnValue({
            data: {
                items: [
                    {
                        id: 'batch-cooldown',
                        operation: 'CREATE',
                        status: 'FAILED',
                        child_count: 1,
                        success_count: 0,
                        failed_count: 1,
                        pending_count: 0,
                        created_at: '2026-03-17T00:00:00Z',
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

        render(<VMBatchListPage />);
        act(() => {
            mutationOptions[0]?.onError?.({
                code: 'BATCH_RATE_LIMITED',
                retry_after_seconds: 6,
                params: { contact_admin: true },
            });
        });
        expect(screen.getByTestId('batch-action-cooldown')).toHaveTextContent(
            'Retry in 6s and contact an administrator',
        );
        expect(screen.getByTestId('batch-action-retry-batch-cooldown')).toBeDisabled();
        expect(screen.getByTestId('batch-action-cancel-batch-cooldown')).toBeDisabled();

        act(() => {
            mutationOptions[1]?.onError?.({
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
                items: [
                    {
                        id: 'batch-restart',
                        operation: 'POWER',
                        status: 'FAILED',
                        child_count: 1,
                        success_count: 0,
                        failed_count: 1,
                        pending_count: 0,
                        created_at: '2026-03-17T00:00:00Z',
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

        render(<VMBatchListPage />);
        act(() => {
            mutationOptions[0]?.onError?.({
                code: 'POWER_OPERATION_IN_PROGRESS',
                status: 409,
                params: {
                    operator_action_required: true,
                    existing_event_id: 'event-list-restart',
                    reconciliation_path: 'operator-runbook:ambiguous-vm-restart',
                },
            });
        });

        expect(screen.getByTestId('restart-reconciliation-alert')).toHaveTextContent('event-list-restart');
        expect(screen.getByTestId('restart-reconciliation-alert')).toHaveTextContent(
            'operator-runbook:ambiguous-vm-restart',
        );
        expect(screen.queryByText('restart_reconciliation.dismiss')).not.toBeInTheDocument();
        const postCalls = vi.mocked(api.POST).mock.calls as unknown as Array<[string, ...unknown[]]>;
        expect(postCalls.some(([path]) => path.includes('/admin/vm-power-events/'))).toBe(false);
    });

    it('gates actions by operation permission while allowing builtin approvers', () => {
        useApiGetMock.mockReturnValue({
            data: {
                items: [
                    {
                        id: 'batch-delete',
                        operation: 'DELETE',
                        status: 'IN_PROGRESS',
                        child_count: 1,
                        success_count: 0,
                        failed_count: 1,
                        pending_count: 0,
                        created_at: '2026-03-17T00:00:00Z',
                    },
                ],
            },
            isLoading: false,
            refetch: vi.fn(),
        });
        useApiMutationMock.mockReturnValue({ isPending: false, mutate: vi.fn() });
        authState.permissions = ['vm:operate'];
        const view = render(<VMBatchListPage />);

        expect(screen.getByTestId('batch-action-retry-batch-delete')).toBeDisabled();
        expect(screen.getByTestId('batch-action-cancel-batch-delete')).toBeDisabled();

        authState.permissions = ['builtin_approval:approve'];
        view.rerender(<VMBatchListPage />);
        expect(screen.getByTestId('batch-action-retry-batch-delete')).toBeEnabled();
        expect(screen.getByTestId('batch-action-cancel-batch-delete')).toBeEnabled();
    });
});
