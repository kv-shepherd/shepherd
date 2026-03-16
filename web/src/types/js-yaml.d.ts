declare module 'js-yaml' {
    export function dump(value: unknown, options?: Record<string, unknown>): string;
    export function load(value: string, options?: Record<string, unknown>): unknown;
}
