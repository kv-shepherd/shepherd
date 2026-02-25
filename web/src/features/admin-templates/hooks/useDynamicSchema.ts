import { useQuery } from '@tanstack/react-query';
import { api } from '@/lib/api/client';
import type { SchemaMask } from '../components/DynamicSchemaForm';

export interface DynamicSchemaResponse {
    /** Raw OpenAPI JSON Schema returned from backend cache or embedded fallback. */
    schema: Record<string, unknown>;
    /** Admin/developer-configured UI mask dictating which paths and labels to expose. */
    mask: SchemaMask;
    /**
     * Schema version string for drift detection and cache audit.
     * Populated when backend implements ADR-0023 response headers.
     */
    schema_version?: string;
    /**
     * Data source of the schema: cache | embedded | remote.
     * ADR-0023: enables frontend to display appropriate degradation indicator.
     */
    source?: 'cache' | 'embedded' | 'remote';
    /**
     * Whether the response is using a degraded/fallback schema.
     * ADR-0023: frontend MUST show a warning when this is true.
     */
    degraded?: boolean;
    /** ISO timestamp when the schema was fetched or last cached. */
    fetched_at?: string;
}

/**
 * Fetches schema-driven configurations per master-flow.md Stage 1.
 *
 * Error handling contract (TanStack Query v5 best practice):
 *   - On fetch failure: the queryFn throws, query enters `isError` state.
 *   - Callers check `isError` to render the ADR-0023 degraded UI.
 *   - This hook does NOT swallow errors — silent degradation hides problems.
 *
 * Degradation strategy (FRONTEND.md ADR-0023):
 *   - Normal:   schema from server cache              → full dynamic form
 *   - Fallback: embedded schema, degraded=true        → ⚠️ warning banner
 *   - Error:    isError=true                          → caller shows text fallback
 *
 * Backend note (for future implementation):
 *   The response should include schema_version, source, degraded, fetched_at
 *   fields so the frontend can render the correct degradation level indicator.
 */
/**
 * Supported entity types for the dynamic schema endpoint.
 *
 * Only entity types with a real KubeVirt spec_overrides schema are listed:
 *   - instancesize: KubeVirt v1.7.0 VirtualMachineSpec sub-schema (cpu/memory/gpu/hugepages).
 *
 * Intentionally excluded:
 *   - template: cloud_init is a static YAML textarea only (master-flow Step 3, no spec_overrides).
 *   - cluster:  schema not yet designed (ADR-0023 phase 2).
 */
export type DynamicSchemaEntityType = 'instancesize';

export const useDynamicSchema = (entityType: DynamicSchemaEntityType) => {
    return useQuery({
        queryKey: ['dynamic-schema', entityType],
        queryFn: async (): Promise<DynamicSchemaResponse> => {
            const response = await api.GET('/schemas/{entity_type}', {
                params: {
                    path: { entity_type: entityType },
                },
            });

            // TanStack Query v5: queryFn MUST throw for the query to enter
            // the `isError` state.  Returning a fallback value here would
            // silently set status='success' and hide the error from callers.
            if (response.error) {
                const msg =
                    response.error &&
                        typeof response.error === 'object' &&
                        'message' in response.error
                        ? (response.error as { message: string }).message
                        : `Failed to fetch schema for entity type: ${entityType}`;
                throw new Error(msg);
            }

            return {
                schema: response.data?.schema as Record<string, unknown>,
                mask: response.data?.mask as SchemaMask,
                // ADR-0023 optional metadata — present only when backend implements them.
                schema_version: (response.data as DynamicSchemaResponse | undefined)?.schema_version,
                source: (response.data as DynamicSchemaResponse | undefined)?.source,
                degraded: (response.data as DynamicSchemaResponse | undefined)?.degraded ?? false,
                fetched_at: (response.data as DynamicSchemaResponse | undefined)?.fetched_at,
            };
        },
        staleTime: 5 * 60 * 1000, // 5 min — schema changes are infrequent
        retry: 0, // Fail fast: backend endpoint is not yet released; no retries needed.
    });
};
