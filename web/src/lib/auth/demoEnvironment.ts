export function isCodespacesDemoHost(hostname?: string): boolean {
    const normalized = (hostname ?? '').trim().toLowerCase();
    if (normalized === '') {
        return false;
    }
    return normalized.endsWith('.github.dev') || normalized.endsWith('.app.github.dev');
}

export function isLocalDemoHost(hostname?: string): boolean {
    const normalized = (hostname ?? '').trim().toLowerCase();
    return normalized === 'localhost' || normalized === '127.0.0.1';
}

export function isLocalOrCodespacesDemoHost(hostname?: string): boolean {
    return isLocalDemoHost(hostname) || isCodespacesDemoHost(hostname);
}
