import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

const { fetchMock, logoutMock } = vi.hoisted(() => ({
    fetchMock: vi.fn(),
    logoutMock: vi.fn(),
}));

const originalAPIURL = process.env.NEXT_PUBLIC_API_URL;
const originalLocationPath = window.location.pathname + window.location.search + window.location.hash;

function response(status: number, body: unknown = {}) {
    return new Response(JSON.stringify(body), {
        status,
        headers: { 'Content-Type': 'application/json' },
    });
}

async function loadAPIClient() {
    const { api } = await import('./client');
    return api;
}

describe('API client transport and unauthorized response policy', () => {
    beforeEach(() => {
        vi.resetModules();
        vi.clearAllMocks();
        process.env.NEXT_PUBLIC_API_URL = 'https://api.example.test/api/v1';
        vi.stubGlobal('fetch', fetchMock);
        vi.doMock('@/stores/auth', () => ({
            useAuthStore: {
                getState: () => ({ logout: logoutMock }),
            },
        }));
        window.history.replaceState({}, '', '/login');
    });

    afterEach(() => {
        if (originalAPIURL === undefined) {
            delete process.env.NEXT_PUBLIC_API_URL;
        } else {
            process.env.NEXT_PUBLIC_API_URL = originalAPIURL;
        }
        vi.doUnmock('@/stores/auth');
        vi.unstubAllGlobals();
        window.history.replaceState({}, '', originalLocationPath);
    });

    it('sends requests through the configured base URL with session credentials', async () => {
        fetchMock.mockResolvedValue(response(200, {
            items: [],
            pagination: { page: 2, per_page: 25, total: 0, total_pages: 0 },
        }));
        const api = await loadAPIClient();

        await api.GET('/systems', {
            params: { query: { page: 2, per_page: 25 } },
        });

        expect(fetchMock).toHaveBeenCalledTimes(1);
        const request = fetchMock.mock.calls[0]?.[0] as Request;
        expect(request).toBeInstanceOf(Request);
        expect(request.url).toBe('https://api.example.test/api/v1/systems?page=2&per_page=25');
        expect(request.credentials).toBe('include');
        expect(request.headers.get('Content-Type')).toBe('application/json');
    });

    it('logs out once for a protected 401 without redirecting again from the login page', async () => {
        fetchMock.mockResolvedValue(response(401, { code: 'UNAUTHORIZED' }));
        const api = await loadAPIClient();

        const result = await api.GET('/auth/me');

        expect(result.response.status).toBe(401);
        expect(logoutMock).toHaveBeenCalledTimes(1);
        expect(window.location.pathname).toBe('/login');
    });

    it('does not clear local authentication state for a public auth endpoint 401', async () => {
        fetchMock.mockResolvedValue(response(401, { code: 'INVALID_CREDENTIALS' }));
        const api = await loadAPIClient();

        const result = await api.GET('/auth/providers');

        expect(result.response.status).toBe(401);
        expect(logoutMock).not.toHaveBeenCalled();
    });

    it('leaves authentication state untouched for non-401 responses', async () => {
        fetchMock.mockResolvedValue(response(403, { code: 'FORBIDDEN' }));
        const api = await loadAPIClient();

        const result = await api.GET('/auth/me');

        expect(result.response.status).toBe(403);
        expect(logoutMock).not.toHaveBeenCalled();
    });
});
