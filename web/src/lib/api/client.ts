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
import { AUTH_STORAGE_KEY } from '@/stores/auth';

const baseUrl = process.env.NEXT_PUBLIC_API_URL ?? 'http://localhost:8080/api/v1';

export const api = createClient<paths>({
  baseUrl,
  headers: {
    'Content-Type': 'application/json',
  },
});

/**
 * Middleware: attach JWT token from localStorage to all requests.
 * Reads from the Zustand persisted store key.
 */
api.use({
  async onRequest({ request }) {
    if (typeof window !== 'undefined') {
      try {
        const stored = localStorage.getItem(AUTH_STORAGE_KEY);
        if (stored) {
          const parsed = JSON.parse(stored);
          const token = parsed?.state?.token;
          if (token) {
            request.headers.set('Authorization', `Bearer ${token}`);
          }
        }
      } catch {
        // ignore parse errors
      }
    }
    return request;
  },
});

/**
 * Middleware: handle 401 responses globally (redirect to login).
 */
api.use({
  async onResponse({ response }) {
    if (response.status === 401 && typeof window !== 'undefined') {
      const { useAuthStore } = await import('@/stores/auth');
      useAuthStore.getState().logout();
      window.location.href = '/login';
    }
    return response;
  },
});

/**
 * Type helpers for components schemas.
 */
export type { paths };
