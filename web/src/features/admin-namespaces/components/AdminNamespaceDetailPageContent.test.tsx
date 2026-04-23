import { render, screen } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

type NamespaceDetail = {
    id: string;
    name: string;
    description: string;
    environment: string;
    enabled: boolean;
    created_by: string;
    created_at: string;
};

const pushMock = vi.fn();
const namespaceQueryState = vi.hoisted(() => ({
    isLoading: false,
    data: {
        id: 'ns-1',
        name: 'team-prod',
        description: 'Production namespace',
        environment: 'prod',
        enabled: true,
        created_by: 'alice',
        created_at: '2026-03-17T00:00:00Z',
    } as NamespaceDetail | undefined,
    error: null as { status?: number; code?: string; message?: string } | null,
}));

vi.mock('next/navigation', () => ({
    useParams: () => ({ id: 'ns-1' }),
    useRouter: () => ({ push: pushMock }),
}));

vi.mock('@/hooks/useApiQuery', () => ({
    useApiGet: () => namespaceQueryState,
}));

vi.mock('react-i18next', () => ({
    useTranslation: () => ({
        t: (key: string, options?: Record<string, unknown>) => {
            const labels: Record<string, string> = {
                'namespaces.title': 'Namespaces',
                'namespaces.table.namespace': 'Namespace',
                'namespaces.environment': 'Environment',
                'namespaces.enabled': 'Enabled',
                'namespaces.enabled_yes': 'Yes',
                'namespaces.enabled_no': 'No',
                'namespaces.detail_title': `Namespace: ${String(options?.name ?? '')}`,
                'namespaces.detail_subtitle': 'Review namespace registry details.',
                'namespaces.detail_description': 'Description',
                'namespaces.detail_registry_id': 'Registry ID',
                'namespaces.detail_not_found': 'Namespace not found',
                'namespaces.detail_not_found_description': 'The namespace may have been removed or is outside your visible scope.',
                'common:button.back': 'Back',
                'common:table.created_by': 'Created By',
                'common:table.created_at': 'Created',
            };
            return labels[key] ?? key;
        },
    }),
}));

import { AdminNamespaceDetailPageContent } from './AdminNamespaceDetailPageContent';

describe('AdminNamespaceDetailPageContent', () => {
    beforeEach(() => {
        pushMock.mockReset();
        namespaceQueryState.isLoading = false;
        namespaceQueryState.error = null;
        namespaceQueryState.data = {
            id: 'ns-1',
            name: 'team-prod',
            description: 'Production namespace',
            environment: 'prod',
            enabled: true,
            created_by: 'alice',
            created_at: '2026-03-17T00:00:00Z',
        };
    });

    it('renders namespace registry details', () => {
        render(<AdminNamespaceDetailPageContent />);

        expect(screen.getByTestId('admin-namespace-detail-page')).toBeVisible();
        expect(screen.getByText('Namespace: team-prod')).toBeVisible();
        expect(screen.getByText('Production namespace')).toBeVisible();
        expect(screen.getByText('Registry ID')).toBeVisible();
        expect(screen.getByText('ns-1')).toBeVisible();
    });

    it('renders a not-found state for missing namespaces', () => {
        namespaceQueryState.data = undefined;
        namespaceQueryState.error = { status: 404, code: 'NAMESPACE_NOT_FOUND' };

        render(<AdminNamespaceDetailPageContent />);

        expect(screen.getByText('Namespace not found')).toBeVisible();
        expect(screen.getByText('The namespace may have been removed or is outside your visible scope.')).toBeVisible();
    });
});
