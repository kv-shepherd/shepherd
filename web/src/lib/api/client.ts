/**
 * Type-safe API client generated from OpenAPI contract (ADR-0021).
 *
 * Uses openapi-fetch for type-safe requests.
 * All paths are typed from generated api.gen.ts.
 *
 * Usage:
 *   import { api } from '@/lib/api/client';
 *   const { data, error } = await api.GET('/systems', {
 *     params: { query: { page: 1, per_page: 20 } },
 *   });
 */
import type { paths } from '@/types/api.gen';
import createClient from 'openapi-fetch';
import { getLoginEntryPath } from '@/lib/auth/loginEntry';
import {
  getRequestPath,
  shouldLogoutOnUnauthorized,
  shouldRedirectToLoginOnUnauthorized,
} from './authPolicy';

// Use relative path to leverage Next.js rewrites (see next.config.ts)
// This fixes access from remote IPs (e.g. 10.x.x.x) by proxying through the Next.js server
const baseUrl = process.env.NEXT_PUBLIC_API_URL ?? '/api/v1';

export const api = createClient<paths>({
  baseUrl,
  fetch: (request: Request) => fetch(new Request(request, { credentials: 'include' })),
  headers: {
    'Content-Type': 'application/json',
  },
});

/**
 * Middleware: handle 401 responses globally (redirect to login).
 */
api.use({
  async onResponse({ request, response }) {
    if (response.status === 401 && typeof window !== 'undefined') {
      const requestPath = getRequestPath(request, window.location.origin);

      if (!shouldLogoutOnUnauthorized(requestPath)) {
        return response;
      }

      const { useAuthStore } = await import('@/stores/auth');
      useAuthStore.getState().logout();

      if (!shouldRedirectToLoginOnUnauthorized(requestPath, window.location.pathname)) {
        return response;
      }

      window.location.replace(getLoginEntryPath());
    }
    return response;
  },
});
