import { beforeEach, describe, expect, it } from 'vitest';

import type { components } from '@/types/api.gen';
import { AUTH_STORAGE_KEY, useAuthStore } from './auth';

type UserInfo = components['schemas']['UserInfo'];

function resetAuthStore() {
	useAuthStore.setState({
		user: null,
		isAuthenticated: false,
		forcePasswordChange: false,
		hasHydrated: true,
		hasValidatedSession: false,
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

		useAuthStore.getState().login(user, true);

		const state = useAuthStore.getState();
		expect(state.user).toEqual(user);
		expect(state.isAuthenticated).toBe(true);
		expect(state.forcePasswordChange).toBe(true);
		expect(state.hasValidatedSession).toBe(true);
	});

	it('clears auth state on logout', () => {
		useAuthStore.getState().login({ id: 'u-alice', username: 'alice' }, true);
		useAuthStore.getState().logout();

		const state = useAuthStore.getState();
		expect(state.user).toBeNull();
		expect(state.isAuthenticated).toBe(false);
		expect(state.forcePasswordChange).toBe(false);
		expect(state.hasValidatedSession).toBe(true);
	});

	it('updates user profile without changing auth state', () => {
		useAuthStore.getState().login({ id: 'u-alice', username: 'alice' }, false);

		useAuthStore.getState().updateUser({ id: 'u-alice', username: 'alice-updated' });

		const state = useAuthStore.getState();
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
		useAuthStore.getState().login(
			{ id: 'u-carl', username: 'carl', force_password_change: true },
			true,
		);
		useAuthStore.getState().clearForcePasswordChange();

		const state = useAuthStore.getState();
		expect(state.forcePasswordChange).toBe(false);
		expect(state.user?.force_password_change).toBe(false);
		expect(state.isAuthenticated).toBe(true);
	});

	it('persists an empty schema to clear legacy localStorage auth', () => {
		useAuthStore.getState().login({ id: 'u-bob', username: 'bob' }, true);

		const raw = localStorage.getItem(AUTH_STORAGE_KEY);
		expect(raw).toBeTruthy();

		const parsed = JSON.parse(raw as string);
		expect(parsed.version).toBe(1);
		expect(parsed.state).toEqual({});
	});

	it('restores session state from server user info', () => {
		useAuthStore.getState().restoreSession({
			id: 'u-restore',
			username: 'restore-user',
			force_password_change: true,
		});

		const state = useAuthStore.getState();
		expect(state.user?.username).toBe('restore-user');
		expect(state.isAuthenticated).toBe(true);
		expect(state.forcePasswordChange).toBe(true);
		expect(state.hasValidatedSession).toBe(true);
	});
});
