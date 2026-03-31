'use client';

import { useMemo } from 'react';
import { useQueryClient } from '@tanstack/react-query';

import { useApiAction, useApiGet, useApiMutation } from '@/hooks/useApiQuery';
import { api } from '@/lib/api/client';
import type { components } from '@/types/api.gen';

type UserPreference = components['schemas']['UserPreference'];
type UserPreferenceUpdateRequest = components['schemas']['UserPreferenceUpdateRequest'];

interface UserPreferenceQueryResult {
    exists: boolean;
    preference?: UserPreference;
}

export function useUserPreference<TValue extends object>(key: string) {
    const queryClient = useQueryClient();
    const query = useApiGet<UserPreferenceQueryResult>(
        ['user-preference', key],
        async () => {
            const result = await api.GET('/auth/preferences/{key}', {
                params: {
                    path: { key },
                },
            });
            if (result.response.status === 404) {
                return {
                    data: { exists: false },
                    response: result.response,
                };
            }
            return {
                data: result.data ? { exists: true, preference: result.data } : undefined,
                error: result.error,
                response: result.response,
            };
        }
    );

    const updateMutation = useApiMutation<UserPreferenceUpdateRequest, UserPreference>(
        (body) =>
            api.PUT('/auth/preferences/{key}', {
                params: {
                    path: { key },
                },
                body,
            }),
        {
            invalidateKeys: [['user-preference', key]],
        }
    );

    const deleteMutation = useApiAction<void>(
        () =>
            api.DELETE('/auth/preferences/{key}', {
                params: {
                    path: { key },
                },
            }),
        {
            invalidateKeys: [['user-preference', key]],
        }
    );

    const preferenceValue = useMemo(
        () => (query.data?.exists ? (query.data.preference?.value as TValue | undefined) : undefined),
        [query.data]
    );

    const savePreference = async (body: UserPreferenceUpdateRequest) => {
        const preference = await updateMutation.mutateAsync(body);
        queryClient.setQueryData<UserPreferenceQueryResult>(['user-preference', key], {
            exists: true,
            preference,
        });
        return preference;
    };

    const resetPreference = async () => {
        await deleteMutation.mutateAsync();
        queryClient.setQueryData<UserPreferenceQueryResult>(['user-preference', key], {
            exists: false,
        });
    };

    return {
        ...query,
        exists: query.data?.exists ?? false,
        preference: query.data?.preference,
        value: preferenceValue,
        savePreference,
        resetPreference,
        savePending: updateMutation.isPending,
        resetPending: deleteMutation.isPending,
    };
}
