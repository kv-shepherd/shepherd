import { describe, expect, it } from 'vitest';

import {
    isCodespacesDemoHost,
    isLocalDemoHost,
    isLocalOrCodespacesDemoHost,
} from './demoEnvironment';

describe('demoEnvironment', () => {
    it('detects localhost-based demo hosts', () => {
        expect(isLocalDemoHost('localhost')).toBe(true);
        expect(isLocalDemoHost('127.0.0.1')).toBe(true);
        expect(isLocalDemoHost('shepherd.example.com')).toBe(false);
    });

    it('detects Codespaces forwarded hosts', () => {
        expect(isCodespacesDemoHost('fuzzy-space-3000.app.github.dev')).toBe(true);
        expect(isCodespacesDemoHost('demo-8080.github.dev')).toBe(true);
        expect(isCodespacesDemoHost('localhost')).toBe(false);
    });

    it('combines local and Codespaces detection', () => {
        expect(isLocalOrCodespacesDemoHost('localhost')).toBe(true);
        expect(isLocalOrCodespacesDemoHost('fuzzy-space-3000.app.github.dev')).toBe(true);
        expect(isLocalOrCodespacesDemoHost('shepherd.example.com')).toBe(false);
    });
});
