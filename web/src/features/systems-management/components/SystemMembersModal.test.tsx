import { fireEvent, render, screen } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

const openAddMemberModalMock = vi.fn();

vi.mock('react-i18next', () => ({
    useTranslation: () => ({
        t: (key: string, fallback?: string) => fallback ?? key,
    }),
}));

vi.mock('../hooks/useSystemMembersController', () => ({
    useSystemMembersController: () => ({
        members: [
            {
                user_id: 'user-1',
                username: 'alice',
                display_name: 'Alice Ops',
                email: 'alice@example.com',
                role: 'admin',
            },
        ],
        isLoading: false,
        refetch: vi.fn(),
        addMemberOpen: true,
        openAddMemberModal: openAddMemberModalMock,
        closeAddMemberModal: vi.fn(),
        addMemberForm: undefined,
        memberCandidates: {
            items: [
                {
                    id: 'user-2',
                    username: 'bob',
                    display_name: 'Bob Platform',
                },
            ],
        },
        memberCandidatesLoading: false,
        memberCandidateSearch: '',
        setMemberCandidateSearch: vi.fn(),
        submitAddMember: vi.fn(),
        addMemberPending: false,
        removeMember: vi.fn(),
        removeMemberPending: false,
        updateRole: vi.fn(),
        updateRolePending: false,
    }),
}));

import { SystemMembersModal } from './SystemMembersModal';

describe('SystemMembersModal', () => {
    beforeEach(() => {
        openAddMemberModalMock.mockReset();
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
        expect(screen.getByTestId('member-candidate-user-select')).toBeInTheDocument();
    });

    it('opens the add-member flow from the primary action', () => {
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
});
