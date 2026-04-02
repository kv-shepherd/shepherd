/**
 * Vitest test setup (FRONTEND.md §Testing).
 *
 * Configures jsdom and testing-library matchers.
 *
 * Keep this file lightweight: it runs before every test file. Tests that
 * need HTTP interception should set it up explicitly inside the test file
 * instead of paying the setup cost globally.
 */
import '@testing-library/jest-dom/vitest';
import { cleanup } from '@testing-library/react';
import { afterEach, beforeAll } from 'vitest';

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
});

afterEach(() => {
    cleanup();
});
