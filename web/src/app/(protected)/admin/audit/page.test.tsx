import { render, screen } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

const { useApiGetMock } = vi.hoisted(() => ({
    useApiGetMock: vi.fn(),
}));

vi.mock('react-i18next', () => ({
    useTranslation: () => ({
        t: (key: string, options?: { defaultValue?: string }) => options?.defaultValue ?? key,
    }),
}));

vi.mock('@/hooks/useApiQuery', () => ({
    useApiGet: (...args: unknown[]) => useApiGetMock(...args),
}));

vi.mock('@/lib/api/client', () => ({
    api: {
        GET: vi.fn(),
    },
}));

import { buildAuditLogQuery } from '@/features/admin-audit/query';

import AuditLogPage from './page';

describe('AuditLogPage', () => {
    beforeEach(() => {
        vi.clearAllMocks();
        useApiGetMock.mockReturnValue({
            data: {
                items: [],
                pagination: {
                    page: 1,
                    per_page: 20,
                    total: 0,
                    total_pages: 0,
                },
            },
            isLoading: false,
            refetch: vi.fn(),
        });
    });

    it('renders without jsdom browser capability warnings', () => {
        render(<AuditLogPage />);

        expect(screen.getByText('audit.title')).toBeInTheDocument();
        expect(screen.getByRole('searchbox')).toBeInTheDocument();
        expect(screen.getByRole('button', { name: 'common:search.advanced' })).toBeInTheDocument();
        expect(screen.getByText('Visible events')).toBeInTheDocument();
    }, 10000);

    it('builds query params for approval decision and placement advisory/reason filters', () => {
        expect(buildAuditLogQuery(1, 20, {
            search: '',
            category: '',
            action: '',
            approval_decision: 'validation_failed',
            actor: '',
            placement_advisory_code: 'PVC_CLONE_HOST_ASSISTED_FALLBACK_LIKELY',
            placement_reason_code: 'CLUSTER_POLICY_DENIED',
            resource_type: 'ticket',
            resource_id: '',
        })).toEqual({
            page: 1,
            per_page: 20,
            approval_decision: 'validation_failed',
            placement_advisory_code: 'PVC_CLONE_HOST_ASSISTED_FALLBACK_LIKELY',
            placement_reason_code: 'CLUSTER_POLICY_DENIED',
            resource_type: 'ticket',
        });
    });
});
