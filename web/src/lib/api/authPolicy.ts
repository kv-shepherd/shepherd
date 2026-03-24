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

export function shouldAttachAuthHeader(requestPath: string): boolean {
    return !isPublicAuthRequestPath(requestPath);
}

export function shouldRedirectToLoginOnUnauthorized(
    requestPath: string,
    currentPathname: string
): boolean {
    if (isPublicAuthRequestPath(requestPath)) {
        return false;
    }
    return currentPathname !== '/login';
}
