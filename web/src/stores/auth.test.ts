import { beforeEach, describe, expect, it } from 'vitest';

import type { components } from '@/types/api.gen';
import { AUTH_STORAGE_KEY, useAuthStore } from './auth';

type UserInfo = components['schemas']['UserInfo'];

function resetAuthStore() {
	useAuthStore.setState({
		token: null,
		user: null,
		isAuthenticated: false,
		forcePasswordChange: false,
	});
}

describe('auth store', () => {
	beforeEach(() => {
		localStorage.clear();
		resetAuthStore();
	});

	it('keeps storage key stable for persisted auth schema', () => {
		expect(AUTH_STORAGE_KEY).toBe('shepherd-auth');
	});

	it('sets authenticated state on login', () => {
		const user: UserInfo = { id: 'u-alice', username: 'alice' };

		useAuthStore.getState().login('token-1', user, true);

		const state = useAuthStore.getState();
		expect(state.token).toBe('token-1');
		expect(state.user).toEqual(user);
		expect(state.isAuthenticated).toBe(true);
		expect(state.forcePasswordChange).toBe(true);
	});

	it('clears auth state on logout', () => {
		useAuthStore.getState().login('token-1', { id: 'u-alice', username: 'alice' }, true);
		useAuthStore.getState().logout();

		const state = useAuthStore.getState();
		expect(state.token).toBeNull();
		expect(state.user).toBeNull();
		expect(state.isAuthenticated).toBe(false);
		expect(state.forcePasswordChange).toBe(false);
	});

	it('updates user profile without changing token', () => {
		useAuthStore.getState().login('token-1', { id: 'u-alice', username: 'alice' }, false);

		useAuthStore.getState().updateUser({ id: 'u-alice', username: 'alice-updated' });

		const state = useAuthStore.getState();
		expect(state.token).toBe('token-1');
		expect(state.user).toEqual({ id: 'u-alice', username: 'alice-updated' });
		expect(state.isAuthenticated).toBe(true);
	});

	it('allows updating user snapshot without changing auth flag', () => {
		useAuthStore.getState().updateUser({ id: 'u-ghost', username: 'ghost' });

		const state = useAuthStore.getState();
		expect(state.user).toEqual({ id: 'u-ghost', username: 'ghost' });
		expect(state.isAuthenticated).toBe(false);
	});

	it('clears forcePasswordChange flag independently', () => {
		useAuthStore.getState().login('token-3', { id: 'u-carl', username: 'carl' }, true);
		useAuthStore.getState().clearForcePasswordChange();

		const state = useAuthStore.getState();
		expect(state.forcePasswordChange).toBe(false);
		expect(state.token).toBe('token-3');
		expect(state.isAuthenticated).toBe(true);
	});

	it('persists only the configured subset of fields', () => {
		useAuthStore.getState().login('token-2', { id: 'u-bob', username: 'bob' }, true);

		const raw = localStorage.getItem(AUTH_STORAGE_KEY);
		expect(raw).toBeTruthy();

		const parsed = JSON.parse(raw as string);
		expect(parsed.state).toMatchObject({
			token: 'token-2',
			user: { id: 'u-bob', username: 'bob' },
			isAuthenticated: true,
		});
		expect(parsed.state.forcePasswordChange).toBeUndefined();
	});
});
