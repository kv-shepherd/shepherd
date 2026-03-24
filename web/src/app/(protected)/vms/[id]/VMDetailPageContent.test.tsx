import { fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

const pushMock = vi.fn();
const refetchMock = vi.fn();
const useApiGetMock = vi.fn();
const useApiMutationMock = vi.fn();

vi.mock('next/navigation', () => ({
    useParams: () => ({ id: 'vm-1' }),
    useRouter: () => ({
        push: pushMock,
    }),
}));

vi.mock('react-i18next', () => ({
    useTranslation: () => ({
        t: (key: string, options?: { defaultValue?: string }) => {
            const labels: Record<string, string> = {
                'common:button.back': 'Back',
                'detail.subtitle': 'Review VM state, power actions, and console access.',
                'field.name': 'Name',
                'common:table.status': 'Status',
                'field.namespace': 'Namespace',
                'field.hostname': 'Hostname',
                'common:table.created_at': 'Created',
                'common:table.actions': 'Actions',
                'action.start': 'Start',
                'action.stop': 'Stop',
                'action.restart': 'Restart',
                'action.console': 'Console',
                'action.request_similar': 'Request Similar VM',
                'action.refresh_status': 'Refresh Status',
                'action.delete': 'Delete',
                'common:message.error': 'Error',
            };
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
        DELETE: vi.fn(),
    },
}));

vi.mock('@/lib/hooks/useMessage', () => ({
    useMessage: () => ({
        messageApi: {
            success: vi.fn(),
            error: vi.fn(),
            warning: vi.fn(),
        },
        messageContextHolder: null,
    }),
}));

import VMDetailPage from './VMDetailPageContent';

describe('VMDetailPage', () => {
    it('renders the unified page shell and VM action surface', () => {
        useApiGetMock.mockReturnValue({
            data: {
                id: 'vm-1',
                name: 'vm-alpha',
                status: 'RUNNING',
                namespace: 'team-prod',
                hostname: 'vm-alpha.internal',
                created_at: '2026-03-17T00:00:00Z',
                environment: 'prod',
            },
            isLoading: false,
            refetch: refetchMock,
        });
        useApiMutationMock.mockReturnValue({
            isPending: false,
            mutate: vi.fn(),
        });

        render(<VMDetailPage />);

        expect(screen.getByTestId('vm-detail-page')).toBeVisible();
        expect(screen.getAllByText('vm-alpha').length).toBeGreaterThan(0);
        expect(screen.getByText('Review VM state, power actions, and console access.')).toBeVisible();
        expect(screen.getByText('Back')).toBeVisible();
        expect(screen.getByText('Actions')).toBeVisible();
        expect(screen.getByTestId('vm-action-start-vm-1')).toBeVisible();
        expect(screen.getByTestId('vm-action-console-vm-1')).toBeVisible();
        expect(screen.getByTestId('vm-action-request-similar-vm-1')).toBeVisible();
        expect(screen.getByText('team-prod')).toBeVisible();

        fireEvent.click(screen.getByTestId('vm-action-request-similar-vm-1'));
        expect(pushMock).toHaveBeenCalledWith('/vms?request=create&source_vm_id=vm-1');
    });
});
