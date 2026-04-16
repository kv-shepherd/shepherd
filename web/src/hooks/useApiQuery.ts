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
import type { UseMutationResult, UseQueryOptions } from '@tanstack/react-query';

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

type ApiClientResult<T> = {
    data?: T;
    error?: unknown;
    response?: Response;
};

function normalizeApiError(error: unknown, response: Response | undefined): ApiErrorResponse {
    const retryHeader = response?.headers.get('Retry-After');
    const retryAfterSeconds = retryHeader ? Number(retryHeader) : NaN;
    const base = toApiErrorResponse(error, response) ?? {
        code: response?.ok ? 'UNKNOWN_ERROR' : 'HTTP_ERROR',
        message: error instanceof Error ? error.message : response ? `HTTP ${response.status}` : undefined,
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

function toApiErrorResponse(
    error: unknown,
    response: Response | undefined,
): ApiErrorResponse | undefined {
    if (!error || typeof error !== 'object') {
        return undefined;
    }

    const candidate = error as Partial<ApiErrorResponse>;
    if (typeof candidate.code === 'string') {
        return candidate as ApiErrorResponse;
    }
    if (response) {
        return {
            code: 'HTTP_ERROR',
            message: typeof candidate.message === 'string' ? candidate.message : `HTTP ${response.status}`,
            status: response.status,
        };
    }
    if (typeof candidate.message === 'string') {
        return {
            code: 'UNKNOWN_ERROR',
            message: candidate.message,
        };
    }
    return undefined;
}

async function invalidateQueryKeys(
    queryClient: ReturnType<typeof useQueryClient>,
    queryKeys: readonly unknown[][] | undefined,
): Promise<void> {
    if (!queryKeys || queryKeys.length === 0) {
        return;
    }
    await Promise.all(
        queryKeys.map((key) => queryClient.invalidateQueries({ queryKey: key })),
    );
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
    fetcher: () => Promise<ApiClientResult<T>>,
    options?: Omit<UseQueryOptions<T, ApiErrorResponse>, 'queryKey' | 'queryFn'>
) {
    // queryKey is the cache identity chosen by the caller; this wrapper keeps
    // fetcher out of the key on purpose so callers don't accidentally key by
    // function identity.
    // eslint-disable-next-line @tanstack/query/exhaustive-deps
    return useQuery<T, ApiErrorResponse>({
        queryKey,
        queryFn: async () => {
            const { data, error, response } = await fetcher();
            if (error) throw normalizeApiError(error, response);
            if (typeof data === 'undefined') {
                throw { code: 'EMPTY_RESPONSE', message: 'No data returned' } satisfies ApiErrorResponse;
            }
            return data;
        },
        ...options,
    });
}

/**
 * Hook for mutations (POST/PUT/DELETE).
 * Automatically invalidates related queries on success.
 */
export function useApiMutation<TResponse = unknown>(
    mutationFn: () => Promise<ApiClientResult<TResponse>>,
    options?: {
        invalidateKeys?: readonly unknown[][];
        onSuccess?: (data: TResponse) => void;
        onError?: (error: ApiErrorResponse) => void;
    }
): UseMutationResult<TResponse, ApiErrorResponse, void>;
export function useApiMutation<TRequest, TResponse = unknown>(
    mutationFn: (req: TRequest) => Promise<ApiClientResult<TResponse>>,
    options?: {
        invalidateKeys?: readonly unknown[][];
        onSuccess?: (data: TResponse) => void;
        onError?: (error: ApiErrorResponse) => void;
    }
): UseMutationResult<TResponse, ApiErrorResponse, TRequest>;
export function useApiMutation<TRequest = void, TResponse = unknown>(
    mutationFn: ((req: TRequest) => Promise<ApiClientResult<TResponse>>) | (() => Promise<ApiClientResult<TResponse>>),
    options?: {
        invalidateKeys?: readonly unknown[][];
        onSuccess?: (data: TResponse) => void;
        onError?: (error: ApiErrorResponse) => void;
    }
) {
    const queryClient = useQueryClient();

    return useMutation<TResponse, ApiErrorResponse, TRequest>({
        mutationFn: async (req: TRequest) => {
            const { data, error, response } = await (mutationFn as (req: TRequest) => Promise<ApiClientResult<TResponse>>)(req);
            if (error) throw normalizeApiError(error, response);
            if (!response?.ok) throw normalizeApiError(undefined, response);
            return data as TResponse;
        },
        onSuccess: async (data) => {
            await invalidateQueryKeys(queryClient, options?.invalidateKeys);
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
    actionFn: (req: TRequest) => Promise<ApiClientResult<unknown>>,
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
            if (!response?.ok) {
                throw normalizeApiError({ code: 'UNEXPECTED_ERROR', message: response ? `HTTP ${response.status}` : 'No response returned' }, response);
            }
        },
        onSuccess: async () => {
            await invalidateQueryKeys(queryClient, options?.invalidateKeys);
            options?.onSuccess?.();
        },
        onError: (error) => {
            options?.onError?.(error);
        },
    });
}
