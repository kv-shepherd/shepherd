import { describe, expect, it } from 'vitest';

import {
    getRequestPath,
    isConsoleSessionRequestPath,
    isPublicAuthRequestPath,
    shouldAttachAuthHeader,
    shouldLogoutOnUnauthorized,
    shouldRedirectToLoginOnUnauthorized,
} from './authPolicy';

describe('authPolicy', () => {
    it('treats public auth endpoints as unauthenticated request paths', () => {
        expect(isPublicAuthRequestPath('/api/v1/auth/login')).toBe(true);
        expect(isPublicAuthRequestPath('/api/v1/auth/providers')).toBe(true);
        expect(isPublicAuthRequestPath('/api/v1/auth/providers/provider-external/login/start')).toBe(true);
        expect(isPublicAuthRequestPath('/api/v1/auth/providers/provider-external/callback')).toBe(true);
        expect(isPublicAuthRequestPath('/api/v1/auth/change-password')).toBe(false);
        expect(isPublicAuthRequestPath('/api/v1/vms')).toBe(false);
    });

    it('does not attach auth header for public auth requests', () => {
        expect(shouldAttachAuthHeader('/api/v1/auth/providers')).toBe(false);
        expect(shouldAttachAuthHeader('/api/v1/auth/providers/provider-external/login/start')).toBe(false);
        expect(shouldAttachAuthHeader('/api/v1/auth/login')).toBe(false);
        expect(shouldAttachAuthHeader('/api/v1/tickets')).toBe(true);
    });

    it('treats console session endpoints as special-case authenticated requests', () => {
        expect(isConsoleSessionRequestPath('/api/v1/vms/vm-1/serial')).toBe(true);
        expect(isConsoleSessionRequestPath('/api/v1/vms/vm-1/vnc')).toBe(true);
        expect(isConsoleSessionRequestPath('/api/v1/vms/vm-1/console/request')).toBe(false);
        expect(shouldAttachAuthHeader('/api/v1/vms/vm-1/serial')).toBe(true);
        expect(shouldLogoutOnUnauthorized('/api/v1/vms/vm-1/serial')).toBe(false);
        expect(shouldRedirectToLoginOnUnauthorized('/api/v1/vms/vm-1/vnc', '/vms/vm-1')).toBe(false);
    });

    it('avoids redirect loops on login page and public auth requests', () => {
        expect(shouldRedirectToLoginOnUnauthorized('/api/v1/auth/providers', '/login')).toBe(false);
        expect(shouldRedirectToLoginOnUnauthorized('/api/v1/auth/login', '/login')).toBe(false);
        expect(shouldRedirectToLoginOnUnauthorized('/api/v1/vms', '/login')).toBe(false);
        expect(shouldRedirectToLoginOnUnauthorized('/api/v1/vms', '/dashboard')).toBe(true);
    });

    it('extracts request path safely', () => {
        const request = new Request('https://shepherd.example.com/api/v1/auth/providers');
        expect(getRequestPath(request)).toBe('/api/v1/auth/providers');
    });
});
