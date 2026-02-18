'use client';

import { App, Form } from 'antd';
import type { TFunction } from 'i18next';
import { useState } from 'react';

import { useApiAction, useApiGet, useApiMutation } from '@/hooks/useApiQuery';
import { api } from '@/lib/api/client';

import type {
    SystemMember,
    SystemMemberCreateRequest,
    SystemMemberList,
    SystemMemberRoleUpdateRequest,
} from '../types';

interface UseSystemMembersControllerArgs {
    t: TFunction;
    systemId: string | null;
}

export function useSystemMembersController({ t, systemId }: UseSystemMembersControllerArgs) {
    const { message } = App.useApp();
    const [addMemberOpen, setAddMemberOpen] = useState(false);
    const [addMemberForm] = Form.useForm<SystemMemberCreateRequest>();

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

    const addMemberMutation = useApiMutation<SystemMemberCreateRequest, SystemMember>(
        (req) => {
            if (!systemId) throw new Error('No system selected');
            return api.POST('/systems/{system_id}/members', {
                params: { path: { system_id: systemId } },
                body: req,
            });
        },
        {
            invalidateKeys: [['system-members', systemId]],
            onSuccess: () => {
                message.success(t('message.success'));
                closeAddMemberModal();
            },
            onError: (err) => {
                message.error(err.code === 'CONFLICT' ? t('members.error_conflict') : t('message.error'));
            },
        }
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
            invalidateKeys: [['system-members', systemId]],
            onSuccess: () => {
                message.success(t('message.success'));
            },
            onError: (err) => message.error(err.message || t('message.error')),
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
            onError: (err) => message.error(err.message || t('message.error')),
        }
    );

    const openAddMemberModal = () => {
        setAddMemberOpen(true);
    };

    const closeAddMemberModal = () => {
        setAddMemberOpen(false);
        addMemberForm.resetFields();
    };

    const submitAddMember = async () => {
        const values = await addMemberForm.validateFields();
        addMemberMutation.mutate(values);
    };

    return {
        members: membersQuery.data?.items ?? [],
        isLoading: membersQuery.isLoading,
        refetch: membersQuery.refetch,
        addMemberOpen,
        openAddMemberModal,
        closeAddMemberModal,
        addMemberForm,
        submitAddMember,
        addMemberPending: addMemberMutation.isPending,
        removeMember: (userId: string) => removeMemberMutation.mutate({ userId }),
        removeMemberPending: removeMemberMutation.isPending,
        updateRole: (userId: string, role: 'admin' | 'member' | 'viewer' | 'owner') =>
            updateRoleMutation.mutate({ userId, body: { role } }),
        updateRolePending: updateRoleMutation.isPending,
    };
}
