import { render, screen } from '@testing-library/react';
import { Form } from 'antd';
import { beforeAll, describe, expect, it, vi } from 'vitest';

import {
    DynamicSchemaForm,
    HUGEPAGES_PAGE_SIZE_PATH,
    isValidHugepagesPageSizeValue,
    normalizeHugepagesPageSizeValue,
    type SchemaMask,
    type SchemaNode,
} from './DynamicSchemaForm';

vi.mock('react-i18next', () => ({
    useTranslation: () => ({
        t: (_key: string, defaultValue?: string) => defaultValue ?? _key,
    }),
}));

beforeAll(() => {
    if (!window.matchMedia) {
        Object.defineProperty(window, 'matchMedia', {
            writable: true,
            value: vi.fn().mockImplementation((query: string) => ({
                matches: false,
                media: query,
                onchange: null,
                addListener: vi.fn(),
                removeListener: vi.fn(),
                addEventListener: vi.fn(),
                removeEventListener: vi.fn(),
                dispatchEvent: vi.fn(),
            })),
        });
    }
});

const minimalSchema: SchemaNode = {
    type: 'object',
    properties: {
        spec: {
            type: 'object',
            properties: {
                template: {
                    type: 'object',
                    properties: {
                        spec: {
                            type: 'object',
                            properties: {
                                domain: {
                                    type: 'object',
                                    properties: {
                                        cpu: {
                                            type: 'object',
                                            properties: {
                                                cores: { type: 'integer' },
                                            },
                                        },
                                        memory: {
                                            type: 'object',
                                            properties: {
                                                hugepages: {
                                                    type: 'object',
                                                    properties: {
                                                        pageSize: { type: 'string' },
                                                    },
                                                },
                                            },
                                        },
                                    },
                                },
                            },
                        },
                    },
                },
            },
        },
    },
};

const minimalMask: SchemaMask = {
    quick_fields: [
        {
            path: 'spec.template.spec.domain.cpu.cores',
            display_name: 'CPU Cores',
        },
        {
            path: HUGEPAGES_PAGE_SIZE_PATH,
            display_name: 'Hugepages',
        },
    ],
};

describe('DynamicSchemaForm hugepages behavior', () => {
    it('renders fields from schema + mask dynamically', () => {
        render(
            <Form layout="vertical">
                <Form.Item name="spec_text" initialValue="{}">
                    <DynamicSchemaForm schema={minimalSchema} mask={minimalMask} />
                </Form.Item>
            </Form>
        );

        expect(screen.getByTestId('dynamic-form-spec.template.spec.domain.cpu.cores')).toBeInTheDocument();
        expect(screen.getByTestId(`dynamic-form-${HUGEPAGES_PAGE_SIZE_PATH}`)).toBeInTheDocument();
    });

    it('normalizes custom MB hugepages input', () => {
        expect(normalizeHugepagesPageSizeValue('512')).toBe('512Mi');
        expect(normalizeHugepagesPageSizeValue(' 1024 Mi ')).toBe('1024Mi');
        expect(normalizeHugepagesPageSizeValue('1gi')).toBe('1Gi');
        expect(normalizeHugepagesPageSizeValue('')).toBeUndefined();
    });

    it('accepts presets and custom MB values only', () => {
        expect(isValidHugepagesPageSizeValue('2Mi')).toBe(true);
        expect(isValidHugepagesPageSizeValue('1Gi')).toBe(true);
        expect(isValidHugepagesPageSizeValue('512Mi')).toBe(true);

        expect(isValidHugepagesPageSizeValue('4Gi')).toBe(false);
        expect(isValidHugepagesPageSizeValue('abc')).toBe(false);
        expect(isValidHugepagesPageSizeValue(512)).toBe(false);
    });
});
