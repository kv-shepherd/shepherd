import { fireEvent, render, screen } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

const openAddMemberModalMock = vi.fn();
const closeAddMemberModalMock = vi.fn();
let addMemberOpenState = true;

vi.mock('react-i18next', () => ({
    useTranslation: () => ({
        t: (key: string, options?: string | { defaultValue?: string; count?: number }) =>
            typeof options === 'string' ? options : options?.defaultValue ?? key,
    }),
}));

vi.mock('@/hooks/useUserPreference', () => ({
    useUserPreference: () => ({
        exists: false,
        value: undefined,
        savePreference: vi.fn(),
        resetPreference: vi.fn(),
        savePending: false,
        resetPending: false,
    }),
}));

vi.mock('../hooks/useSystemMembersController', () => ({
    useSystemMembersController: () => {
        return {
            members: [
                {
                    user_id: 'user-1',
                    username: 'alice',
                    display_name: 'Alice Ops',
                    email: 'alice@example.com',
                    role: 'admin',
                    created_at: '2026-04-08T00:00:00Z',
                    profile_attributes: {
                        department: 'Engineering',
                        section: 'Platform',
                    },
                },
            ],
            memberProfileFields: [
                { key: 'department', label: 'Department', searchable: true },
                { key: 'section', label: 'Section', searchable: true },
            ],
            isLoading: false,
            refetch: vi.fn(),
            addMemberOpen: addMemberOpenState,
            openAddMemberModal: openAddMemberModalMock,
            closeAddMemberModal: closeAddMemberModalMock,
            addMemberRole: 'member',
            setAddMemberRole: vi.fn(),
            memberCandidates: {
                profile_fields: [
                    { key: 'department', label: 'Department', searchable: true },
                    { key: 'section', label: 'Section', searchable: true },
                ],
                pagination: {
                    page: 1,
                    per_page: 20,
                    total: 1,
                    total_pages: 1,
                },
                items: [
                    {
                        id: 'user-2',
                        username: 'bob',
                        display_name: 'Bob Platform',
                        profile_attributes: {
                            department: 'Engineering',
                            section: 'Platform',
                        },
                    },
                ],
            },
            memberCandidatesLoading: false,
            memberCandidateSearch: '',
            memberCandidateSearchDraft: '',
            memberCandidatePage: 1,
            memberCandidatePerPage: 20,
            selectedCandidateUserIds: [],
            selectedCandidateUsers: [],
            setSelectedCandidateUsers: vi.fn(),
            setMemberCandidateSearchDraft: vi.fn(),
            applyMemberCandidateSearch: vi.fn(),
            clearMemberCandidateSearch: vi.fn(),
            setMemberCandidatePagination: vi.fn(),
            submitAddMember: vi.fn(),
            addMemberPending: false,
            removeMember: vi.fn(),
            removeMembers: vi.fn(),
            removeMemberPending: false,
            removingMemberIds: [],
            updateRole: vi.fn(),
            updateRolePending: false,
        };
    },
}));

import { SystemMembersModal } from './SystemMembersModal';

describe('SystemMembersModal', () => {
    beforeEach(() => {
        addMemberOpenState = true;
        openAddMemberModalMock.mockReset();
        closeAddMemberModalMock.mockReset();
    });

    it('renders member identity summary and uses candidate select for adding members', () => {
        render(
            <SystemMembersModal
                open={true}
                onCancel={vi.fn()}
                systemId="sys-1"
                systemName="System A"
            />,
        );

        expect(screen.getByText('Alice Ops')).toBeInTheDocument();
        expect(screen.getByText('alice')).toBeInTheDocument();
        expect(screen.getByText('alice@example.com')).toBeInTheDocument();
        expect(screen.getByTestId('member-candidate-user-table')).toBeInTheDocument();
    });

    it('opens the add-member flow from the primary action', () => {
        addMemberOpenState = false;
        render(
            <SystemMembersModal
                open={true}
                onCancel={vi.fn()}
                systemId="sys-1"
                systemName="System A"
            />,
        );

        fireEvent.click(screen.getByTestId('member-add-button'));

        expect(openAddMemberModalMock).toHaveBeenCalledTimes(1);
    });

    it('shows batch remove affordance for current members', () => {
        render(
            <SystemMembersModal
                open={true}
                onCancel={vi.fn()}
                systemId="sys-1"
                systemName="System A"
            />,
        );

        expect(screen.getByTestId('member-batch-remove-button')).toBeInTheDocument();
    });
});
