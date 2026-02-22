/**
 * useApiGet — thin wrapper around @tanstack/react-query v5 useQuery.
 *
 * Adapts the openapi-fetch response shape { data, error, response }
 * into a standard loading/data/error surface for page components.
 *
 * Usage:
 *   const { data, isLoading, refetch } = useApiGet(
 *     ['key'],
 *     () => api.GET(path, {}),
 *   );
 */
import { useQuery } from '@tanstack/react-query';
import type { QueryKey } from '@tanstack/react-query';

type ApiFetchResult<T> = Promise<{
    data?: T;
    error?: unknown;
    response?: Response;
}>;

/**
 * Wraps an openapi-fetch call with react-query.
 * Returns `data` (unwrapped from the fetch envelope), plus
 * standard react-query `isLoading`, `error`, and `refetch`.
 */
export function useApiGet<T>(
    queryKey: QueryKey,
    fetcher: () => ApiFetchResult<T>,
    options?: {
        enabled?: boolean;
        staleTime?: number;
        refetchInterval?: number;
    },
) {
    const query = useQuery<T | undefined>({
        queryKey,
        queryFn: async () => {
            const result = await fetcher();
            if (result.error) {
                throw result.error;
            }
            return result.data;
        },
        enabled: options?.enabled,
        staleTime: options?.staleTime,
        refetchInterval: options?.refetchInterval,
    });

    return {
        data: query.data,
        isLoading: query.isLoading,
        isFetching: query.isFetching,
        error: query.error,
        refetch: query.refetch,
    };
}
