'use client';

import { Form } from 'antd';
import { useQueryClient } from '@tanstack/react-query';
import type { MessageInstance } from 'antd/es/message/interface';
import type { TFunction } from 'i18next';
import { useState } from 'react';

import { useApiAction, useApiGet } from '@/hooks/useApiQuery';
import type { ApiErrorResponse } from '@/hooks/useApiQuery';
import { api } from '@/lib/api/client';
import { translateApiError } from '@/lib/api/errorMessage';
import type { components } from '@/types/api.gen';
import { useScopeTargetCatalog } from './useScopeTargetCatalog';

type GlobalRoleBinding = components['schemas']['GlobalRoleBinding'];
type GlobalRoleBindingCreateRequest = components['schemas']['GlobalRoleBindingCreateRequest'];
type GlobalRoleBindingList = components['schemas']['GlobalRoleBindingList'];
type UserList = components['schemas']['UserList'];
type User = components['schemas']['User'];

interface BindingFormValues {
    role_id: string;
    scope_type: string;
    scope_id?: string;
    allowed_environments?: Array<'test' | 'prod'>;
}

interface UseUserRoleBindingsManagerArgs {
    t: TFunction;
    selectedUserId: string;
    messageApi: MessageInstance;
    enabled?: boolean;
}

export function useUserRoleBindingsManager({
    t,
    selectedUserId,
    messageApi,
    enabled = true,
}: UseUserRoleBindingsManagerArgs) {
    const queryClient = useQueryClient();
    const [addBindingOpen, setAddBindingOpen] = useState(false);
    const [deletingBindingId, setDeletingBindingId] = useState('');
    const [deletingBindingIds, setDeletingBindingIds] = useState<string[]>([]);
    const [bindingUserSearch, setBindingUserSearch] = useState('');
    const [bindingUserSearchDraft, setBindingUserSearchDraft] = useState('');
    const [bindingUserPage, setBindingUserPage] = useState(1);
    const [bindingUserPerPage, setBindingUserPerPage] = useState(20);
    const [seedBindingUserIds, setSeedBindingUserIds] = useState<string[]>([]);
    const [selectedBindingUserIds, setSelectedBindingUserIds] = useState<string[]>([]);
    const [selectedBindingUsersById, setSelectedBindingUsersById] = useState<Record<string, User>>({});
    const [createBindingPending, setCreateBindingPending] = useState(false);
    const [deleteBindingsPending, setDeleteBindingsPending] = useState(false);
    const [bindingForm] = Form.useForm<BindingFormValues>();
    const bindingEnabled = enabled && selectedUserId.length > 0;
    const { scopeTargetOptionsByType, scopeTargetLoadingByType } = useScopeTargetCatalog(addBindingOpen);

    const roleBindingsQuery = useApiGet<GlobalRoleBindingList>(
        ['admin-user-role-bindings', selectedUserId],
        () => api.GET('/admin/users/{user_id}/role-bindings', {
            params: { path: { user_id: selectedUserId } },
        }),
        { enabled: bindingEnabled }
    );

    const bindingUserCandidatesQuery = useApiGet<UserList>(
        ['admin-user-binding-candidates', bindingUserSearch, bindingUserPage, bindingUserPerPage],
        () => api.GET('/admin/users', {
            params: {
                query: {
                    page: bindingUserPage,
                    per_page: bindingUserPerPage,
                    ...(bindingUserSearch ? { search: bindingUserSearch } : {}),
                },
            },
        }),
        { enabled: addBindingOpen }
    );

    const deleteBindingMutation = useApiAction<{ userId: string; bindingId: string }>(
        ({ userId, bindingId }) => api.DELETE('/admin/users/{user_id}/role-bindings/{binding_id}', {
            params: { path: { user_id: userId, binding_id: bindingId } },
        }),
        {
            invalidateKeys: [
                ['admin-user-role-bindings', selectedUserId],
                ['admin-users'],
                ['admin-rbac-users'],
            ],
            onSuccess: () => {
                setDeletingBindingId('');
                messageApi.success(t('common:message.success'));
            },
            onError: (err) => {
                setDeletingBindingId('');
                messageApi.error(translateApiError(t, err));
            },
        }
    );

    const setSelectedBindingUsers = (userIds: string[], users: User[] = []) => {
        const normalizedIds = Array.from(
            new Set(userIds.map((value) => value.trim()).filter((value) => value.length > 0)),
        );
        setSelectedBindingUserIds(normalizedIds);
        setSelectedBindingUsersById((current) => {
            const nextUsersById: Record<string, User> = {};
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

    const openAddBindingModal = (presetUsers?: User[], presetSearch = '') => {
        bindingForm.resetFields();
        const normalizedPresetUsers = Array.from(
            new Map((presetUsers ?? []).map((user) => [user.id, user] as const)).values(),
        );
        const normalizedPresetUserIds = normalizedPresetUsers.map((user) => user.id);
        bindingForm.setFieldsValue({
            scope_type: 'global',
        });
        const initialUserIds =
            normalizedPresetUserIds.length > 0
                ? normalizedPresetUserIds
                : selectedUserId
                    ? [selectedUserId]
                    : [];
        setSeedBindingUserIds(initialUserIds);
        setSelectedBindingUserIds(initialUserIds);
        setBindingUserSearch(presetSearch.trim());
        setBindingUserSearchDraft(presetSearch);
        setBindingUserPage(1);
        setBindingUserPerPage(20);
        setSelectedBindingUsersById(
            Object.fromEntries(normalizedPresetUsers.map((user) => [user.id, user] as const)),
        );
        setAddBindingOpen(true);
    };

    const closeAddBindingModal = () => {
        setAddBindingOpen(false);
        setBindingUserSearch('');
        setBindingUserSearchDraft('');
        setBindingUserPage(1);
        setBindingUserPerPage(20);
        setSeedBindingUserIds([]);
        setSelectedBindingUserIds([]);
        setSelectedBindingUsersById({});
        bindingForm.resetFields();
    };

    const applyBindingUserSearch = (value = bindingUserSearchDraft) => {
        setBindingUserSearchDraft(value);
        setBindingUserSearch(value.trim());
        setBindingUserPage(1);
    };

    const clearBindingUserSearch = () => {
        setBindingUserSearchDraft('');
        setBindingUserSearch('');
        setBindingUserPage(1);
    };

    const clearSelectedBindingUsers = () => {
        setSeedBindingUserIds([]);
        setSelectedBindingUserIds([]);
        setSelectedBindingUsersById({});
    };

    const setBindingUserPagination = (page: number, pageSize?: number) => {
        setBindingUserPage(page);
        if (pageSize && pageSize !== bindingUserPerPage) {
            setBindingUserPerPage(pageSize);
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

    const createRoleBindingForUser = async (
        userId: string,
        body: GlobalRoleBindingCreateRequest,
    ) => {
        const { data, error, response } = await api.POST('/admin/users/{user_id}/role-bindings', {
            params: { path: { user_id: userId } },
            body,
        });
        if (error || !response.ok) {
            throw toApiError(error, response);
        }
        return data as GlobalRoleBinding;
    };

    const deleteSingleRoleBinding = async (userId: string, bindingId: string) => {
        const { error, response } = await api.DELETE('/admin/users/{user_id}/role-bindings/{binding_id}', {
            params: { path: { user_id: userId, binding_id: bindingId } },
        });
        if (error || !response.ok) {
            throw toApiError(error, response);
        }
    };

    const submitAddBinding = async () => {
        const values = await bindingForm.validateFields([
            'role_id',
            'scope_type',
            'scope_id',
            'allowed_environments',
        ]);
        const effectiveSelectedUserIds =
            selectedBindingUserIds.length > 0 ? selectedBindingUserIds : seedBindingUserIds;
        const userIds = Array.from(
            new Set(
                effectiveSelectedUserIds
                    .map((value) => value.trim())
                    .filter((value) => value.length > 0)
            )
        );
        if (userIds.length === 0) {
            messageApi.warning(t('rbac.bindings.select_user_first'));
            return;
        }

        const payload: GlobalRoleBindingCreateRequest = {
            role_id: values.role_id,
            scope_type: values.scope_type || 'global',
            scope_id: values.scope_id?.trim() || undefined,
            allowed_environments:
                values.allowed_environments && values.allowed_environments.length > 0
                    ? values.allowed_environments
                    : undefined,
        };

        setCreateBindingPending(true);
        let successCount = 0;
        let conflictCount = 0;
        let firstError: ApiErrorResponse | null = null;

        try {
            for (const userId of userIds) {
                try {
                    await createRoleBindingForUser(userId, payload);
                    successCount += 1;
                } catch (error) {
                    const apiError = error as ApiErrorResponse;
                    if (apiError.code === 'ROLE_BINDING_EXISTS') {
                        conflictCount += 1;
                        continue;
                    }
                    if (!firstError) {
                        firstError = apiError;
                    }
                }
            }

            await Promise.all([
                queryClient.invalidateQueries({ queryKey: ['admin-user-role-bindings', selectedUserId] }),
                queryClient.invalidateQueries({ queryKey: ['admin-users'] }),
                queryClient.invalidateQueries({ queryKey: ['admin-rbac-users'] }),
                queryClient.invalidateQueries({ queryKey: ['admin-user-binding-candidates'] }),
            ]);

            if (successCount > 0 && !firstError && conflictCount === 0) {
                messageApi.success(
                    userIds.length === 1
                        ? t('common:message.success')
                        : t('rbac.bindings.batch_success', {
                            defaultValue: 'Added this access binding to {{count}} users.',
                            count: successCount,
                        })
                );
                closeAddBindingModal();
                return;
            }

            if (successCount > 0 || conflictCount > 0) {
                messageApi.warning(
                    t('rbac.bindings.batch_partial', {
                        defaultValue:
                            'Added access bindings for {{successCount}} users, skipped {{conflictCount}} existing bindings{{failureSuffix}}.',
                        successCount,
                        conflictCount,
                        failureSuffix: firstError ? t('rbac.bindings.batch_partial_failure_suffix', {
                            defaultValue: ', and some requests failed'
                        }) : '',
                    })
                );
                closeAddBindingModal();
                return;
            }

            if (firstError) {
                messageApi.error(translateApiError(t, firstError));
            }
        } finally {
            setCreateBindingPending(false);
        }
    };

    const deleteRoleBinding = (bindingId: string) => {
        if (!selectedUserId) {
            return;
        }
        setDeletingBindingId(bindingId);
        deleteBindingMutation.mutate({ userId: selectedUserId, bindingId });
    };

    const deleteRoleBindings = async (bindingIds: string[]) => {
        if (!selectedUserId || bindingIds.length === 0) {
            return { failedIds: [] as string[] };
        }

        setDeleteBindingsPending(true);
        setDeletingBindingIds(bindingIds);
        let successCount = 0;
        const failedIds: string[] = [];
        let firstError: ApiErrorResponse | null = null;

        try {
            for (const bindingId of bindingIds) {
                try {
                    await deleteSingleRoleBinding(selectedUserId, bindingId);
                    successCount += 1;
                } catch (error) {
                    failedIds.push(bindingId);
                    if (!firstError) {
                        firstError = error as ApiErrorResponse;
                    }
                }
            }

            await Promise.all([
                queryClient.invalidateQueries({ queryKey: ['admin-user-role-bindings', selectedUserId] }),
                queryClient.invalidateQueries({ queryKey: ['admin-users'] }),
                queryClient.invalidateQueries({ queryKey: ['admin-rbac-users'] }),
            ]);

            if (successCount > 0 && failedIds.length === 0) {
                messageApi.success(
                    bindingIds.length === 1
                        ? t('common:message.success')
                        : t('users.directory.batch_delete_bindings_success', {
                            defaultValue: 'Removed {{count}} bindings.',
                            count: successCount,
                        }),
                );
                return { failedIds };
            }

            if (successCount > 0) {
                messageApi.warning(
                    t('users.directory.batch_delete_bindings_partial', {
                        defaultValue: 'Removed {{successCount}} bindings, but {{failureCount}} failed.',
                        successCount,
                        failureCount: failedIds.length,
                    }),
                );
                return { failedIds };
            }

            if (firstError) {
                messageApi.error(translateApiError(t, firstError));
            }
            return { failedIds };
        } finally {
            setDeleteBindingsPending(false);
            setDeletingBindingIds([]);
        }
    };

    const listRoleBindingsForUser = async (userId: string) => {
        const { data, error, response } = await api.GET('/admin/users/{user_id}/role-bindings', {
            params: { path: { user_id: userId } },
        });
        if (error || !response.ok) {
            throw toApiError(error, response);
        }
        return (data?.items ?? []) as GlobalRoleBinding[];
    };

    const resetRoleBindingsForUsers = async (userIds: string[]) => {
        const normalizedUserIds = Array.from(
            new Set(userIds.map((value) => value.trim()).filter((value) => value.length > 0)),
        );
        if (normalizedUserIds.length === 0) {
            return { failedUserIds: [] as string[] };
        }

        setDeleteBindingsPending(true);
        let successCount = 0;
        let skippedCount = 0;
        const failedUserIds: string[] = [];
        let firstError: ApiErrorResponse | null = null;

        try {
            for (const userId of normalizedUserIds) {
                try {
                    const bindings = await listRoleBindingsForUser(userId);
                    if (bindings.length === 0) {
                        skippedCount += 1;
                        continue;
                    }
                    for (const binding of bindings) {
                        await deleteSingleRoleBinding(userId, binding.id);
                    }
                    successCount += 1;
                } catch (error) {
                    failedUserIds.push(userId);
                    if (!firstError) {
                        firstError = error as ApiErrorResponse;
                    }
                }
            }

            await Promise.all([
                queryClient.invalidateQueries({ queryKey: ['admin-user-role-bindings'] }),
                queryClient.invalidateQueries({ queryKey: ['admin-users'] }),
                queryClient.invalidateQueries({ queryKey: ['admin-rbac-users'] }),
            ]);

            if (successCount > 0 && failedUserIds.length === 0) {
                messageApi.success(
                    t('users.directory.batch_reset_access_success', {
                        defaultValue:
                            skippedCount > 0
                                ? 'Reset explicit access for {{count}} users and skipped {{skippedCount}} with no bindings.'
                                : 'Reset explicit access for {{count}} users.',
                        count: successCount,
                        skippedCount,
                    }),
                );
                return { failedUserIds };
            }

            if (successCount > 0 || skippedCount > 0) {
                messageApi.warning(
                    t('users.directory.batch_reset_access_partial', {
                        defaultValue:
                            'Reset {{successCount}} users, skipped {{skippedCount}} with no bindings, and {{failureCount}} failed.',
                        successCount,
                        skippedCount,
                        failureCount: failedUserIds.length,
                    }),
                );
                return { failedUserIds };
            }

            if (firstError) {
                messageApi.error(translateApiError(t, firstError));
            }
            return { failedUserIds };
        } finally {
            setDeleteBindingsPending(false);
        }
    };

    return {
        roleBindings: roleBindingsQuery.data?.items ?? [],
        roleBindingsLoading: roleBindingsQuery.isLoading,
        refetchRoleBindings: roleBindingsQuery.refetch,
        addBindingOpen,
        deletingBindingId,
        deletingBindingIds,
        bindingForm,
        bindingUserCandidates: bindingUserCandidatesQuery.data?.items ?? [],
        bindingUserCandidateProfileFields: bindingUserCandidatesQuery.data?.profile_fields ?? [],
        bindingUserCandidatesPagination: bindingUserCandidatesQuery.data?.pagination,
        bindingUserCandidatesLoading: bindingUserCandidatesQuery.isLoading || bindingUserCandidatesQuery.isFetching,
        bindingUserSearch,
        bindingUserSearchDraft,
        bindingUserPage,
        bindingUserPerPage,
        seedBindingUserIds,
        selectedBindingUserIds,
        selectedBindingUsers: (
            selectedBindingUserIds.length > 0 ? selectedBindingUserIds : seedBindingUserIds
        )
            .map((userId) => selectedBindingUsersById[userId])
            .filter((user): user is User => Boolean(user)),
        effectiveSelectedBindingUserIds:
            selectedBindingUserIds.length > 0 ? selectedBindingUserIds : seedBindingUserIds,
        setSelectedBindingUsers,
        clearSelectedBindingUsers,
        setBindingUserSearchDraft,
        applyBindingUserSearch,
        clearBindingUserSearch,
        setBindingUserPagination,
        openAddBindingModal,
        closeAddBindingModal,
        submitAddBinding,
        deleteRoleBinding,
        deleteRoleBindings,
        resetRoleBindingsForUsers,
        createBindingPending,
        deleteBindingPending: deleteBindingMutation.isPending || deleteBindingsPending,
        scopeTargetOptionsByType,
        scopeTargetLoadingByType,
    };
}
