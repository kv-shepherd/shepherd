/**
 * useApiMutation — thin wrapper around @tanstack/react-query v5 useMutation.
 *
 * Adapts openapi-fetch calls (which return { data, error, response })
 * into a standard mutation surface with onSuccess / onError callbacks.
 *
 * Usage:
 *   const mutation = useApiMutation(
 *     (vars) => api.POST(path, { body: vars }),
 *     { onSuccess: () => { ... } }
 *   );
 *   mutation.mutate(payload);
 */
import { useMutation } from '@tanstack/react-query';

type ApiFetchResult<T> = Promise<{
    data?: T;
    error?: unknown;
    response?: Response;
}>;

interface UseMutationOptions<TData, TVariables> {
    onSuccess?: (data: TData | undefined, variables: TVariables) => void;
    onError?: (error: Error, variables: TVariables) => void;
    onSettled?: (data: TData | undefined, error: Error | null, variables: TVariables) => void;
}

/**
 * Wraps an openapi-fetch mutation call with react-query useMutation.
 * Unwraps the { data, error } envelope and surfaces errors as thrown Error objects.
 */
export function useApiMutation<TData = unknown, TVariables = void>(
    mutationFn: (variables: TVariables) => ApiFetchResult<TData>,
    options?: UseMutationOptions<TData, TVariables>,
) {
    const mutation = useMutation<TData | undefined, Error, TVariables>({
        mutationFn: async (variables) => {
            const result = await mutationFn(variables);
            if (result.error) {
                const err = result.error;
                if (err instanceof Error) throw err;
                if (typeof err === 'object' && err !== null && 'message' in err) {
                    throw new Error(String((err as { message: unknown }).message));
                }
                throw new Error('API request failed');
            }
            return result.data;
        },
        onSuccess: options?.onSuccess,
        onError: options?.onError,
        onSettled: options?.onSettled,
    });

    return {
        mutate: mutation.mutate,
        mutateAsync: mutation.mutateAsync,
        isPending: mutation.isPending,
        isSuccess: mutation.isSuccess,
        isError: mutation.isError,
        error: mutation.error,
        data: mutation.data,
        reset: mutation.reset,
    };
}
