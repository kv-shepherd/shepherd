/**
 * Type-safe API query hooks (TanStack Query 5 + openapi-fetch).
 *
 * Based on Context7 docs for openapi-fetch + TanStack Query integration.
 * Uses manual queryKey construction for maximum flexibility.
 *
 * AGENTS.md §4.3: Automatic deduplication via TanStack Query.
 * AGENTS.md §5.1: Calculate derived state during rendering.
 */
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import type { UseQueryOptions } from '@tanstack/react-query';

/**
 * API error shape — matches backend Error schema.
 * Frontend uses `code` as i18n key (ADR-0023).
 */
export interface ApiErrorResponse {
    code: string;
    message?: string;
    params?: Record<string, unknown>;
    status?: number;
    retry_after_seconds?: number;
    field_errors?: Array<{
        field: string;
        code: string;
        message?: string;
    }>;
}

function normalizeApiError(
    error: ApiErrorResponse | undefined,
    response: Response | undefined,
): ApiErrorResponse {
    const retryHeader = response?.headers.get('Retry-After');
    const retryAfterSeconds = retryHeader ? Number(retryHeader) : NaN;
    const base: ApiErrorResponse = error ?? {
        code: response?.ok ? 'UNKNOWN_ERROR' : 'HTTP_ERROR',
        message: response ? `HTTP ${response.status}` : undefined,
    };
    const normalized: ApiErrorResponse = {
        ...base,
        status: typeof base.status === 'number' ? base.status : response?.status,
    };
    if (Number.isFinite(retryAfterSeconds) && retryAfterSeconds > 0) {
        normalized.retry_after_seconds = retryAfterSeconds;
    }
    return normalized;
}

/**
 * Hook for typed GET requests.
 *
 * Example:
 *   const { data, isLoading } = useApiGet(
 *     ['systems'],
 *     () => api.GET('/systems', { params: { query: { page: 1 } } })
 *   );
 */
export function useApiGet<T>(
    queryKey: readonly unknown[],
    fetcher: () => Promise<{ data?: T; error?: ApiErrorResponse; response: Response }>,
    options?: Omit<UseQueryOptions<T, ApiErrorResponse>, 'queryKey' | 'queryFn'>
) {
    return useQuery<T, ApiErrorResponse>({
        queryKey,
        queryFn: async () => {
            const { data, error, response } = await fetcher();
            if (error) throw normalizeApiError(error, response);
            if (!data) throw { code: 'EMPTY_RESPONSE', message: 'No data returned' };
            return data;
        },
        ...options,
    });
}

/**
 * Hook for mutations (POST/PUT/DELETE).
 * Automatically invalidates related queries on success.
 */
export function useApiMutation<TRequest, TResponse = unknown>(
    mutationFn: (req: TRequest) => Promise<{
        data?: TResponse;
        error?: ApiErrorResponse;
        response: Response;
    }>,
    options?: {
        invalidateKeys?: readonly unknown[][];
        onSuccess?: (data: TResponse) => void;
        onError?: (error: ApiErrorResponse) => void;
    }
) {
    const queryClient = useQueryClient();

    return useMutation<TResponse, ApiErrorResponse, TRequest>({
        mutationFn: async (req: TRequest) => {
            const { data, error, response } = await mutationFn(req);
            if (error) throw normalizeApiError(error, response);
            if (!response.ok) throw normalizeApiError(undefined, response);
            return data as TResponse;
        },
        onSuccess: (data) => {
            if (options?.invalidateKeys) {
                for (const key of options.invalidateKeys) {
                    queryClient.invalidateQueries({ queryKey: key });
                }
            }
            options?.onSuccess?.(data);
        },
        onError: (error) => {
            options?.onError?.(error);
        },
    });
}

/**
 * Hook for void mutations (DELETE, POST actions like start/stop).
 * These endpoints return 202/204 with no body.
 */
export function useApiAction<TRequest = void>(
    actionFn: (req: TRequest) => Promise<{
        data?: unknown;
        error?: ApiErrorResponse;
        response: Response;
    }>,
    options?: {
        invalidateKeys?: readonly unknown[][];
        onSuccess?: () => void;
        onError?: (error: ApiErrorResponse) => void;
    }
) {
    const queryClient = useQueryClient();

    return useMutation<void, ApiErrorResponse, TRequest>({
        mutationFn: async (req: TRequest) => {
            const { error, response } = await actionFn(req);
            if (error) throw normalizeApiError(error, response);
            if (!response.ok) throw normalizeApiError({ code: 'UNEXPECTED_ERROR', message: `HTTP ${response.status}` }, response);
        },
        onSuccess: () => {
            if (options?.invalidateKeys) {
                for (const key of options.invalidateKeys) {
                    queryClient.invalidateQueries({ queryKey: key });
                }
            }
            options?.onSuccess?.();
        },
        onError: (error) => {
            options?.onError?.(error);
        },
    });
}
