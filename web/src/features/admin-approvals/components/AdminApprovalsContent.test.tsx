import { render, screen } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

const controllerState = vi.hoisted(() => ({
    overrides: {} as Record<string, unknown>,
}));

vi.mock('react-i18next', () => ({
    useTranslation: () => ({
        t: (key: string, fallback?: string | { defaultValue?: string }) => {
            if (typeof fallback === 'string') {
                return fallback;
            }
            return fallback?.defaultValue ?? key;
        },
    }),
}));

vi.mock('../hooks/useAdminApprovalsController', async () => {
    const { Form } = await import('antd');
    return {
        useAdminApprovalsController: () => {
            const [approveForm] = Form.useForm();
            const [rejectForm] = Form.useForm();
            return {
                messageContextHolder: null,
                statusFilter: 'PENDING',
                changeStatusFilter: vi.fn(),
                operationFilter: 'ALL',
                changeOperationFilter: vi.fn(),
                selectedClusterFilter: '',
                changeSelectedClusterFilter: vi.fn(),
                placementSnapshotFilter: 'ALL',
                changePlacementSnapshotFilter: vi.fn(),
                page: 1,
                pageSize: 20,
                setPage: vi.fn(),
                setPageSize: vi.fn(),
                data: {
                    items: [{
                        id: 'ticket-1',
                        event_id: 'event-1',
                        status: 'PENDING',
                        operation_type: 'CREATE',
                        requester: 'alice',
                        provisioning: {
                            phase: 'CloneInProgress',
                            progress: '45%',
                            clone_type: 'copy',
                            failure_message: 'target pod restarted once',
                        },
                    }],
                    pagination: { page: 1, per_page: 20, total: 0, total_pages: 0 },
                },
                isLoading: false,
                refetch: vi.fn(),
                approveModal: {
                    id: 'ticket-1',
                    event_id: 'event-1',
                    status: 'PENDING',
                    operation_type: 'CREATE',
                    requester: 'alice',
                    reason: 'scale up',
                    ticket_payload: {
                        namespace: 'prod-a',
                        template_id: 'tpl-1',
                        instance_size_id: 'size-1',
                        dedicated_cpu: true,
                    },
                    provisioning: {
                        phase: 'CloneInProgress',
                        progress: '45%',
                        claim_name: 'target-root-pvc',
                        pvc_phase: 'Bound',
                        clone_type: 'copy',
                        clone_phase: 'Succeeded',
                        clone_fallback_reason: 'The volume modes of source and target are incompatible',
                        failure_message: 'target pod restarted once',
                    },
                },
                rejectModal: null,
                approveForm,
                rejectForm,
                clustersData: {
                    items: [{ id: 'cluster-a', name: 'Cluster A', enabled: true }],
                },
                openApproveModal: vi.fn(),
                closeApproveModal: vi.fn(),
                openRejectModal: vi.fn(),
                closeRejectModal: vi.fn(),
                submitApprove: vi.fn(),
                submitReject: vi.fn(),
                submitCancel: vi.fn(),
                approvePending: false,
                rejectPending: false,
                cancelPending: false,
                ...controllerState.overrides,
            };
        },
    };
});

import { AdminApprovalsContent } from './AdminApprovalsContent';

describe('AdminApprovalsContent', () => {
    beforeEach(() => {
        controllerState.overrides = {};
    });

    it('shows clone fallback details for create approvals with provisioning status', async () => {
        render(<AdminApprovalsContent />);

        expect(await screen.findByTestId('approval-provisioning-card')).toBeInTheDocument();
        expect(screen.getByTestId('approval-provisioning-summary-ticket-1')).toBeInTheDocument();
        expect(screen.getByTestId('approval-provisioning-phase')).toHaveTextContent('CloneInProgress');
        expect(screen.getByTestId('approval-provisioning-clone-type')).toHaveTextContent('Host-assisted copy');
        expect(screen.getByText('The volume modes of source and target are incompatible')).toBeInTheDocument();
        expect(screen.getAllByText('target pod restarted once')).toHaveLength(2);
    });
});
