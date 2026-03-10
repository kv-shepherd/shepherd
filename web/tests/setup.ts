/**
 * Vitest test setup (FRONTEND.md §Testing).
 *
 * Configures jsdom, testing-library matchers, and MSW.
 */
import '@testing-library/jest-dom/vitest';
import { cleanup } from '@testing-library/react';
import { afterAll, afterEach, beforeAll } from 'vitest';
import { setupServer } from 'msw/node';

import { handlers } from './mocks/handlers';

export const server = setupServer(...handlers);

function installBrowserTestShims() {
    if (typeof window === 'undefined') {
        return;
    }

    if (!window.matchMedia) {
        Object.defineProperty(window, 'matchMedia', {
            writable: true,
            value: (query: string) => ({
                matches: false,
                media: query,
                onchange: null,
                addListener: () => {},
                removeListener: () => {},
                addEventListener: () => {},
                removeEventListener: () => {},
                dispatchEvent: () => false,
            }),
        });
    }

    const patchedFlag = '__shepherd_patched_get_computed_style__';
    const maybePatchedWindow = window as Window & {
        [patchedFlag]?: boolean;
    };
    if (maybePatchedWindow[patchedFlag]) {
        return;
    }

    const originalGetComputedStyle = window.getComputedStyle.bind(window);
    const patchedGetComputedStyle: typeof window.getComputedStyle = ((elt: Element, pseudoElt?: string | null) => {
        // jsdom does not implement getComputedStyle(..., pseudoElt). Ant Design's
        // rc-util scrollbar measurement probes ::-webkit-scrollbar, which would
        // otherwise emit a noisy "not implemented" warning in every test run.
        // Returning the base computed style is the closest deterministic fallback
        // available in a non-layout jsdom environment.
        if (typeof pseudoElt === 'string' && pseudoElt.trim() !== '') {
            return originalGetComputedStyle(elt);
        }
        return originalGetComputedStyle(elt, pseudoElt ?? undefined);
    }) as typeof window.getComputedStyle;

    Object.defineProperty(window, 'getComputedStyle', {
        configurable: true,
        writable: true,
        value: patchedGetComputedStyle,
    });
    Object.defineProperty(globalThis, 'getComputedStyle', {
        configurable: true,
        writable: true,
        value: patchedGetComputedStyle,
    });
    maybePatchedWindow[patchedFlag] = true;
}

beforeAll(() => {
    installBrowserTestShims();
    server.listen({ onUnhandledRequest: 'error' });
});

afterEach(() => {
    cleanup();
    server.resetHandlers();
});

afterAll(() => {
    server.close();
});
