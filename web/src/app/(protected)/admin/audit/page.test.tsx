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

import AuditLogPage, { buildAuditLogQuery } from './page';

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
        expect(screen.getAllByText('Decision').length).toBeGreaterThan(0);
        expect(screen.getAllByText('Placement').length).toBeGreaterThan(0);
        expect(screen.getByPlaceholderText('audit.filter.placement_advisory_code')).toBeInTheDocument();
        expect(screen.getByPlaceholderText('audit.filter.placement_reason_code')).toBeInTheDocument();
    });

    it('builds query params for approval decision and placement advisory/reason filters', () => {
        expect(buildAuditLogQuery(1, 20, {
            action: '',
            approval_decision: 'validation_failed',
            actor: '',
            placement_advisory_code: 'PVC_CLONE_HOST_ASSISTED_FALLBACK_LIKELY',
            placement_reason_code: 'CLUSTER_POLICY_DENIED',
            resource_type: 'approval_ticket',
            resource_id: '',
        })).toEqual({
            page: 1,
            per_page: 20,
            approval_decision: 'validation_failed',
            placement_advisory_code: 'PVC_CLONE_HOST_ASSISTED_FALLBACK_LIKELY',
            placement_reason_code: 'CLUSTER_POLICY_DENIED',
            resource_type: 'approval_ticket',
        });
    });
});
