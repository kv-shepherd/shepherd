import { Form } from 'antd';
import { render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

const useSetupGuideMock = vi.fn();
const useScopedVMRequestLauncherMock = vi.fn();

vi.mock('react-i18next', () => ({
    useTranslation: () => ({
        t: (
            key: string,
            options?: string | { defaultValue?: string; [key: string]: unknown },
        ) => {
            const labels: Record<string, string> = {
                title: 'Virtual Machines',
                subtitle: 'Manage virtual machine lifecycle',
                create_request: 'Create Request',
                'common:button.refresh': 'Refresh',
            };
            const fallback =
                typeof options === 'string' ? options : options?.defaultValue;
            let message = labels[key] ?? fallback ?? key;
            if (options && typeof options === 'object') {
                for (const [name, value] of Object.entries(options)) {
                    if (name === 'defaultValue') {
                        continue;
                    }
                    message = message.replaceAll(`{{${name}}}`, String(value));
                }
            }
            return message;
        },
    }),
}));

vi.mock('next/navigation', () => ({
    useRouter: () => ({
        push: vi.fn(),
    }),
}));

vi.mock('@/components/auth/PermissionGuard', () => ({
    PermissionGuard: ({ children }: { children: React.ReactNode }) => children,
}));

vi.mock('@/features/setup-guide/components/SetupGuideCard', () => ({
    SetupGuideCard: ({ variant }: { variant: string }) => <div>{`setup-guide-${variant}`}</div>,
}));

vi.mock('@/features/setup-guide/hooks/useSetupGuide', () => ({
    useSetupGuide: (...args: unknown[]) => useSetupGuideMock(...args),
}));

vi.mock('@/features/vm-management/components/VMSavedDraftBanner', () => ({
    VMSavedDraftBanner: () => <div>saved-draft-banner</div>,
}));

vi.mock('@/features/vm-management/components/VMListTable', () => ({
    VMListTable: () => <div>vm-list-table</div>,
}));

vi.mock('@/features/vm-management/components/VMRequestWizard', () => ({
    VMRequestWizard: () => null,
}));

vi.mock('@/features/vm-management/hooks/useScopedVMRequestLauncher', () => ({
    useScopedVMRequestLauncher: (...args: unknown[]) => useScopedVMRequestLauncherMock(...args),
}));

vi.mock('@/features/vm-management/hooks/useVMManagementController', () => ({
    useVMManagementController: () => {
        const [form] = Form.useForm();
        const [modifyForm] = Form.useForm();

        return {
            messageContextHolder: null,
            savedDraft: null,
            wizardOpen: false,
            refetch: vi.fn(),
            openWizard: vi.fn(),
            openSimilarRequest: vi.fn(),
            resumeDraft: vi.fn(),
            discardDraft: vi.fn(),
            selectedVMIDs: [],
            batchSubmitPending: false,
            batchRateLimited: false,
            modifySubmitPending: false,
            openBatchModifyModal: vi.fn(),
            submitBatchPowerSelected: vi.fn(),
            submitBatchDeleteSelected: vi.fn(),
            batchRetryAfterSeconds: 0,
            vmData: { items: [], pagination: { total: 0 } },
            isLoading: false,
            page: 1,
            pageSize: 20,
            setPage: vi.fn(),
            setPageSize: vi.fn(),
            startVM: vi.fn(),
            stopVM: vi.fn(),
            restartVM: vi.fn(),
            requestConsole: vi.fn(),
            openDeleteModal: vi.fn(),
            openModifyModal: vi.fn(),
            setSelectedVMIDs: vi.fn(),
            activeBatchID: '',
            batchStatus: null,
            batchLoading: false,
            refreshBatch: vi.fn(),
            clearBatchTracking: vi.fn(),
            lastBatchActionFeedback: null,
            wizardStep: 0,
            setWizardStep: vi.fn(),
            requestMode: 'guided',
            setRequestMode: vi.fn(),
            form,
            wizardSteps: [],
            selectedSystemId: '',
            onSystemChange: vi.fn(),
            systemsData: undefined,
            servicesData: undefined,
            templatesData: undefined,
            sizesData: undefined,
            selectedTemplate: undefined,
            selectedSize: undefined,
            placementHint: undefined,
            placementHintLoading: false,
            serviceIdValue: undefined,
            namespaceValue: undefined,
            namespaceOptions: [],
            reasonValue: undefined,
            batchCountValue: 1,
            createVMRequest: { isPending: false },
            goToNextWizardStep: vi.fn(),
            submitWizard: vi.fn(),
            closeWizard: vi.fn(),
            modifyScope: 'single',
            modifyTargetVM: null,
            modifyOpen: false,
            submitModify: vi.fn(),
            closeModifyModal: vi.fn(),
            modifySubmitDisabled: false,
            modifyForm,
            modifyContext: null,
            modifyContextLoading: false,
            deleteOpen: false,
            submitDelete: vi.fn(),
            closeDeleteModal: vi.fn(),
            deletePending: false,
            deletingVM: null,
            deleteConfirmName: '',
            setDeleteConfirmName: vi.fn(),
        };
    },
}));

import VMsPage from './page';

describe('VMsPage', () => {
    it('shows setup guidance and disables create when request prerequisites are missing', () => {
        useSetupGuideMock.mockReturnValue({
            vmRequestReady: false,
        });
        useScopedVMRequestLauncherMock.mockReset();

        render(<VMsPage />);

        expect(screen.getByText('setup-guide-vm')).toBeVisible();
        expect(screen.getByRole('button', { name: /Create Request/ })).toBeDisabled();
        expect(useScopedVMRequestLauncherMock).toHaveBeenCalledWith(
            expect.objectContaining({ canLaunchRequest: false }),
        );
    });
});
