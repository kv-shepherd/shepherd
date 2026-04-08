'use client';

import { App } from 'antd';
import { useQueryClient } from '@tanstack/react-query';
import type { TFunction } from 'i18next';
import { useState } from 'react';

import { useApiAction, useApiGet, useApiMutation } from '@/hooks/useApiQuery';
import type { ApiErrorResponse } from '@/hooks/useApiQuery';
import { api } from '@/lib/api/client';
import { translateApiError } from '@/lib/api/errorMessage';

import type {
    SystemMember,
    SystemMemberList,
    SystemMemberRoleUpdateRequest,
    UserList,
} from '../types';

type CandidateUser = NonNullable<UserList['items']>[number];

interface AddSystemMembersFormValues {
    role: 'admin' | 'member' | 'viewer' | 'owner';
}

interface UseSystemMembersControllerArgs {
    t: TFunction;
    systemId: string | null;
}

export function useSystemMembersController({ t, systemId }: UseSystemMembersControllerArgs) {
    const { message } = App.useApp();
    const queryClient = useQueryClient();
    const [addMemberOpen, setAddMemberOpen] = useState(false);
    const [addMemberRole, setAddMemberRole] = useState<AddSystemMembersFormValues['role']>('member');
    const [memberCandidateSearch, setMemberCandidateSearch] = useState('');
    const [memberCandidateSearchDraft, setMemberCandidateSearchDraft] = useState('');
    const [memberCandidatePage, setMemberCandidatePage] = useState(1);
    const [memberCandidatePerPage, setMemberCandidatePerPage] = useState(20);
    const [selectedCandidateUserIds, setSelectedCandidateUserIds] = useState<string[]>([]);
    const [selectedCandidateUsersById, setSelectedCandidateUsersById] = useState<Record<string, CandidateUser>>({});
    const [addMemberPending, setAddMemberPending] = useState(false);
    const [removingMemberIds, setRemovingMemberIds] = useState<string[]>([]);
    const [removeMembersPending, setRemoveMembersPending] = useState(false);

    const membersQuery = useApiGet<SystemMemberList>(
        ['system-members', systemId],
        () => {
            if (!systemId) throw new Error('System ID is required');
            return api.GET('/systems/{system_id}/members', {
                params: { path: { system_id: systemId } },
            });
        },
        { enabled: !!systemId }
    );

    const memberCandidatesQuery = useApiGet<UserList>(
        ['system-member-candidates', systemId, memberCandidateSearch, memberCandidatePage, memberCandidatePerPage],
        () => {
            if (!systemId) throw new Error('System ID is required');
            return api.GET('/systems/{system_id}/member-candidates', {
                params: {
                    path: { system_id: systemId },
                    query: {
                        page: memberCandidatePage,
                        per_page: memberCandidatePerPage,
                        ...(memberCandidateSearch ? { search: memberCandidateSearch } : {}),
                    },
                },
            });
        },
        { enabled: addMemberOpen && !!systemId }
    );

    const removeMemberMutation = useApiAction<{ userId: string }>(
        ({ userId }) => {
            if (!systemId) throw new Error('No system selected');
            return api.DELETE('/systems/{system_id}/members/{user_id}', {
                params: {
                    path: { system_id: systemId, user_id: userId },
                },
            });
        },
        {
            invalidateKeys: [['system-members', systemId], ['system-member-candidates', systemId]],
            onSuccess: () => {
                message.success(t('message.success'));
            },
            onError: (err) => message.error(translateApiError(t, err, 'message.error')),
        }
    );

    const updateRoleMutation = useApiMutation<
        { userId: string; body: SystemMemberRoleUpdateRequest },
        SystemMember
    >(
        ({ userId, body }) => {
            if (!systemId) throw new Error('No system selected');
            return api.PATCH('/systems/{system_id}/members/{user_id}', {
                params: { path: { system_id: systemId, user_id: userId } },
                body,
            });
        },
        {
            invalidateKeys: [['system-members', systemId]],
            onSuccess: () => {
                message.success(t('message.success'));
            },
            onError: (err) => message.error(translateApiError(t, err, 'message.error')),
        }
    );

    const updateSelectedCandidateUsers = (userIds: string[], users: CandidateUser[] = []) => {
        const normalizedIds = Array.from(
            new Set(userIds.map((value) => value.trim()).filter((value) => value.length > 0)),
        );
        setSelectedCandidateUserIds(normalizedIds);
        setSelectedCandidateUsersById((current) => {
            const nextUsersById: Record<string, CandidateUser> = {};
            for (const userId of normalizedIds) {
                if (current[userId]) {
                    nextUsersById[userId] = current[userId];
                }
            }
            for (const user of users) {
                nextUsersById[user.id] = user;
            }
            return nextUsersById;
        });
    };

    const openAddMemberModal = () => {
        setAddMemberRole('member');
        setMemberCandidateSearch('');
        setMemberCandidateSearchDraft('');
        setMemberCandidatePage(1);
        setMemberCandidatePerPage(20);
        setSelectedCandidateUserIds([]);
        setSelectedCandidateUsersById({});
        setAddMemberOpen(true);
    };

    const closeAddMemberModal = () => {
        setAddMemberOpen(false);
        setMemberCandidateSearch('');
        setMemberCandidateSearchDraft('');
        setMemberCandidatePage(1);
        setMemberCandidatePerPage(20);
        setSelectedCandidateUserIds([]);
        setSelectedCandidateUsersById({});
        setAddMemberRole('member');
    };

    const applyMemberCandidateSearch = (value = memberCandidateSearchDraft) => {
        setMemberCandidateSearchDraft(value);
        setMemberCandidateSearch(value.trim());
        setMemberCandidatePage(1);
    };

    const clearMemberCandidateSearch = () => {
        setMemberCandidateSearchDraft('');
        setMemberCandidateSearch('');
        setMemberCandidatePage(1);
    };

    const setMemberCandidatePagination = (page: number, pageSize?: number) => {
        setMemberCandidatePage(page);
        if (pageSize && pageSize !== memberCandidatePerPage) {
            setMemberCandidatePerPage(pageSize);
        }
    };

    const toApiError = (
        error: ApiErrorResponse | undefined,
        response: Response,
    ): ApiErrorResponse => ({
        ...(error ?? {
            code: response.ok ? 'UNKNOWN_ERROR' : 'HTTP_ERROR',
            message: `HTTP ${response.status}`,
        }),
        status: error?.status ?? response.status,
    });

    const addSingleMember = async (userId: string, role: AddSystemMembersFormValues['role']) => {
        if (!systemId) throw new Error('No system selected');
        const { data, error, response } = await api.POST('/systems/{system_id}/members', {
            params: { path: { system_id: systemId } },
            body: {
                user_id: userId,
                role,
            },
        });
        if (error || !response.ok) {
            throw toApiError(error, response);
        }
        return data as SystemMember;
    };

    const removeSingleMember = async (userId: string) => {
        if (!systemId) throw new Error('No system selected');
        const { error, response } = await api.DELETE('/systems/{system_id}/members/{user_id}', {
            params: {
                path: { system_id: systemId, user_id: userId },
            },
        });
        if (error || !response.ok) {
            throw toApiError(error, response);
        }
    };

    const submitAddMember = async () => {
        const userIds = Array.from(
            new Set(selectedCandidateUserIds.map((value) => value.trim()).filter((value) => value.length > 0))
        );
        if (userIds.length === 0) {
            message.warning(t('validation.required'));
            return;
        }

        setAddMemberPending(true);
        let successCount = 0;
        let conflictCount = 0;
        let firstError: ApiErrorResponse | null = null;

        try {
            for (const userId of userIds) {
                try {
                    await addSingleMember(userId, addMemberRole);
                    successCount += 1;
                } catch (error) {
                    const apiError = error as ApiErrorResponse;
                    if (apiError.code === 'MEMBER_ALREADY_EXISTS' || apiError.code === 'CONFLICT') {
                        conflictCount += 1;
                        continue;
                    }
                    if (!firstError) {
                        firstError = apiError;
                    }
                }
            }

            await Promise.all([
                queryClient.invalidateQueries({ queryKey: ['system-members', systemId] }),
                queryClient.invalidateQueries({ queryKey: ['system-member-candidates', systemId] }),
            ]);

            if (successCount > 0 && !firstError && conflictCount === 0) {
                message.success(
                    userIds.length === 1
                        ? t('message.success')
                        : t('members.batch_success', {
                            defaultValue: 'Added {{count}} members.',
                            count: successCount,
                        })
                );
                closeAddMemberModal();
                return;
            }

            if (successCount > 0 || conflictCount > 0) {
                message.warning(
                    t('members.batch_partial', {
                        defaultValue:
                            'Added {{successCount}} members, skipped {{conflictCount}} existing members{{failureSuffix}}.',
                        successCount,
                        conflictCount,
                        failureSuffix: firstError
                            ? t('members.batch_partial_failure_suffix', {
                                defaultValue: ', and some requests failed',
                            })
                            : '',
                    })
                );
                closeAddMemberModal();
                return;
            }

            if (firstError) {
                message.error(firstError.code === 'CONFLICT' ? t('members.error_conflict') : translateApiError(t, firstError, 'message.error'));
            }
        } finally {
            setAddMemberPending(false);
        }
    };

    const removeMembers = async (userIds: string[]) => {
        if (!systemId || userIds.length === 0) {
            return { failedIds: [] as string[] };
        }

        setRemoveMembersPending(true);
        setRemovingMemberIds(userIds);
        let successCount = 0;
        const failedIds: string[] = [];
        let firstError: ApiErrorResponse | null = null;

        try {
            for (const userId of userIds) {
                try {
                    await removeSingleMember(userId);
                    successCount += 1;
                } catch (error) {
                    failedIds.push(userId);
                    if (!firstError) {
                        firstError = error as ApiErrorResponse;
                    }
                }
            }

            await Promise.all([
                queryClient.invalidateQueries({ queryKey: ['system-members', systemId] }),
                queryClient.invalidateQueries({ queryKey: ['system-member-candidates', systemId] }),
            ]);

            if (successCount > 0 && failedIds.length === 0) {
                message.success(
                    userIds.length === 1
                        ? t('message.success')
                        : t('members.batch_remove_success', {
                            defaultValue: 'Removed {{count}} members.',
                            count: successCount,
                        }),
                );
                return { failedIds };
            }

            if (successCount > 0) {
                message.warning(
                    t('members.batch_remove_partial', {
                        defaultValue: 'Removed {{successCount}} members, but {{failureCount}} failed.',
                        successCount,
                        failureCount: failedIds.length,
                    }),
                );
                return { failedIds };
            }

            if (firstError) {
                message.error(translateApiError(t, firstError, 'message.error'));
            }
            return { failedIds };
        } finally {
            setRemoveMembersPending(false);
            setRemovingMemberIds([]);
        }
    };

    return {
        members: membersQuery.data?.items ?? [],
        memberProfileFields: membersQuery.data?.profile_fields ?? [],
        isLoading: membersQuery.isLoading,
        membersError: membersQuery.error,
        refetch: membersQuery.refetch,
        addMemberOpen,
        openAddMemberModal,
        closeAddMemberModal,
        addMemberRole,
        setAddMemberRole,
        memberCandidates: memberCandidatesQuery.data,
        memberCandidatesLoading: memberCandidatesQuery.isLoading || memberCandidatesQuery.isFetching,
        memberCandidatesError: memberCandidatesQuery.error,
        memberCandidateSearch,
        memberCandidateSearchDraft,
        memberCandidatePage,
        memberCandidatePerPage,
        selectedCandidateUserIds,
        selectedCandidateUsers: selectedCandidateUserIds
            .map((userId) => selectedCandidateUsersById[userId])
            .filter((user): user is CandidateUser => Boolean(user)),
        setSelectedCandidateUsers: updateSelectedCandidateUsers,
        setMemberCandidateSearchDraft,
        applyMemberCandidateSearch,
        clearMemberCandidateSearch,
        setMemberCandidatePagination,
        submitAddMember,
        addMemberPending,
        removeMember: (userId: string) => removeMemberMutation.mutate({ userId }),
        removeMembers,
        removeMemberPending: removeMemberMutation.isPending || removeMembersPending,
        removingMemberIds,
        updateRole: (userId: string, role: 'admin' | 'member' | 'viewer' | 'owner') =>
            updateRoleMutation.mutate({ userId, body: { role } }),
        updateRolePending: updateRoleMutation.isPending,
    };
}
