import { getLoginEntryPath } from "@/lib/auth/loginEntry";

export function getRequestPath(request: Request, origin = 'http://localhost'): string {
    try {
        return new URL(request.url, origin).pathname;
    } catch {
        return '';
    }
}

export function isPublicAuthRequestPath(requestPath: string): boolean {
    const normalized = requestPath.replace(/\/+$/, '');
    if (normalized.endsWith('/auth/login')) {
        return true;
    }
    return normalized.includes('/auth/providers');
}

export function isConsoleSessionRequestPath(requestPath: string): boolean {
    const normalized = requestPath.replace(/\/+$/, '');
    return /\/api\/v1\/vms\/[^/]+\/(serial|vnc)$/.test(normalized);
}

export function shouldAttachAuthHeader(requestPath: string): boolean {
    return !isPublicAuthRequestPath(requestPath);
}

export function shouldLogoutOnUnauthorized(requestPath: string): boolean {
    if (isPublicAuthRequestPath(requestPath)) {
        return false;
    }
    if (isConsoleSessionRequestPath(requestPath)) {
        return false;
    }
    return true;
}

export function shouldRedirectToLoginOnUnauthorized(
    requestPath: string,
    currentPathname: string
): boolean {
    if (!shouldLogoutOnUnauthorized(requestPath)) {
        return false;
    }
    return currentPathname !== getLoginEntryPath();
}
