import type { ReactNode } from 'react';
import { act, renderHook, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { describe, expect, it, vi } from 'vitest';

import type { ApiErrorResponse } from './useApiQuery';
import { useApiAction, useApiGet, useApiMutation } from './useApiQuery';

function createTestHarness() {
	const queryClient = new QueryClient({
		defaultOptions: {
			queries: { retry: false },
			mutations: { retry: false },
		},
	});

	const wrapper = ({ children }: { children: ReactNode }) => (
		<QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
	);

	return { queryClient, wrapper };
}

describe('useApiGet', () => {
	it('returns data when fetcher succeeds', async () => {
		const { wrapper } = createTestHarness();

		const { result } = renderHook(
			() =>
				useApiGet(['systems'], async () => ({
					data: { items: ['a'] },
					response: new Response(),
				})),
			{ wrapper }
		);

		await waitFor(() => expect(result.current.isSuccess).toBe(true));
		expect(result.current.data).toEqual({ items: ['a'] });
	});

	it('surfaces backend error payload', async () => {
		const { wrapper } = createTestHarness();
		const apiError: ApiErrorResponse = { code: 'NOT_FOUND', message: 'not found' };

		const { result } = renderHook(
			() =>
				useApiGet(['systems'], async () => ({
					error: apiError,
					response: new Response(null, { status: 404 }),
				})),
			{ wrapper }
		);

		await waitFor(() => expect(result.current.isError).toBe(true));
		expect(result.current.error).toMatchObject({
			code: 'NOT_FOUND',
			message: 'not found',
			status: 404,
		});
	});

	it('returns EMPTY_RESPONSE when no data is provided', async () => {
		const { wrapper } = createTestHarness();

		const { result } = renderHook(
			() =>
				useApiGet(['systems'], async () => ({
					response: new Response(null, { status: 204 }),
				})),
			{ wrapper }
		);

		await waitFor(() => expect(result.current.isError).toBe(true));
		expect(result.current.error).toMatchObject({ code: 'EMPTY_RESPONSE' });
	});
});

describe('useApiMutation', () => {
	it('invalidates configured keys on success', async () => {
		const { wrapper, queryClient } = createTestHarness();
		const onSuccess = vi.fn();
		const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries');

		const { result } = renderHook(
			() =>
				useApiMutation(
					async (name: string) => ({
						data: { id: 'sys-1', name },
						response: new Response(),
					}),
					{
						invalidateKeys: [['systems'], ['dashboard']],
						onSuccess,
					}
				),
			{ wrapper }
		);

		await act(async () => {
			await result.current.mutateAsync('demo');
		});

		expect(invalidateSpy).toHaveBeenCalledTimes(2);
		expect(onSuccess).toHaveBeenCalledWith({ id: 'sys-1', name: 'demo' });
	});

	it('calls onError when mutation fails', async () => {
		const { wrapper } = createTestHarness();
		const onError = vi.fn();
		const apiError: ApiErrorResponse = { code: 'CONFLICT', message: 'duplicate' };

		const { result } = renderHook(
			() =>
				useApiMutation(
					async () => ({
						error: apiError,
						response: new Response(null, { status: 409 }),
					}),
					{ onError }
				),
			{ wrapper }
		);

		await expect(result.current.mutateAsync('ignored')).rejects.toMatchObject({
			code: 'CONFLICT',
			message: 'duplicate',
			status: 409,
		});
		expect(onError).toHaveBeenCalledWith(expect.objectContaining({
			code: 'CONFLICT',
			status: 409,
		}));
	});

	it('extracts retry_after_seconds from Retry-After header', async () => {
		const { wrapper } = createTestHarness();

		const { result } = renderHook(
			() =>
				useApiMutation(async () => ({
					error: { code: 'BATCH_RATE_LIMITED', message: 'limited' },
					response: new Response(null, {
						status: 429,
						headers: { 'Retry-After': '8' },
					}),
				})),
			{ wrapper }
		);

		await expect(result.current.mutateAsync('ignored')).rejects.toMatchObject({
			code: 'BATCH_RATE_LIMITED',
			status: 429,
			retry_after_seconds: 8,
		});
	});
});

describe('useApiAction', () => {
	it('invalidates queries for successful void actions', async () => {
		const { wrapper, queryClient } = createTestHarness();
		const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries');
		const onSuccess = vi.fn();

		const { result } = renderHook(
			() =>
				useApiAction(
					async () => ({
						response: new Response(null, { status: 204 }),
					}),
					{
						invalidateKeys: [['vms']],
						onSuccess,
					}
				),
			{ wrapper }
		);

		await act(async () => {
			await result.current.mutateAsync();
		});

		expect(invalidateSpy).toHaveBeenCalledTimes(1);
		expect(onSuccess).toHaveBeenCalledTimes(1);
	});

	it('maps non-2xx responses to UNEXPECTED_ERROR', async () => {
		const { wrapper } = createTestHarness();

		const { result } = renderHook(
			() =>
				useApiAction(async () => ({
					response: new Response(null, { status: 503 }),
				})),
			{ wrapper }
		);

		await expect(result.current.mutateAsync()).rejects.toMatchObject({
			code: 'UNEXPECTED_ERROR',
			message: 'HTTP 503',
		});
	});
});
