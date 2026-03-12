import { act, render, screen, waitFor } from '@testing-library/react';
import { Form } from 'antd';
import { beforeAll, describe, expect, it, vi } from 'vitest';
import { useRef, useState } from 'react';

import {
    DynamicSchemaForm,
    type DynamicSchemaFormHandle,
    HUGEPAGES_PAGE_SIZE_PATH,
    isValidHugepagesPageSizeValue,
    normalizeHugepagesPageSizeValue,
    type SchemaMask,
    type SchemaNode,
} from './DynamicSchemaForm';

vi.mock('react-i18next', () => ({
    useTranslation: () => ({
        t: (_key: string, fallback?: string | { defaultValue?: string; label?: string }) => {
            if (typeof fallback === 'string') {
                return fallback;
            }
            if (fallback?.defaultValue) {
                return fallback.defaultValue;
            }
            if (fallback?.label) {
                return `Add ${fallback.label}`;
            }
            return _key;
        },
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
                                        devices: {
                                            type: 'object',
                                            properties: {
                                                gpus: {
                                                    type: 'array',
                                                    items: {
                                                        type: 'object',
                                                        properties: {
                                                            name: { type: 'string' },
                                                            deviceName: { type: 'string' },
                                                        },
                                                    },
                                                },
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

    it('serializes spec_text as nested JSON instead of flat dot-notation keys', async () => {
        function Harness() {
            const [form] = Form.useForm();
            const formRef = useRef<DynamicSchemaFormHandle>(null);

            return (
                <Form form={form} layout="vertical">
                    <Form.Item name="spec_text" initialValue="{}">
                        <DynamicSchemaForm ref={formRef} schema={minimalSchema} mask={minimalMask} />
                    </Form.Item>
                    <button
                        type="button"
                        onClick={() => {
                            form.setFieldsValue({
                                spec: {
                                    template: {
                                        spec: {
                                            domain: {
                                                cpu: { cores: 4 },
                                            },
                                        },
                                    },
                                },
                            });
                        }}
                    >
                        set-cpu
                    </button>
                    <button type="button" onClick={() => formRef.current?.sync()}>
                        sync
                    </button>
                    <Form.Item shouldUpdate noStyle>
                        {() => <pre data-testid="spec-text">{form.getFieldValue('spec_text')}</pre>}
                    </Form.Item>
                </Form>
            );
        }

        render(<Harness />);

        await act(async () => {
            screen.getByText('set-cpu').click();
            screen.getByText('sync').click();
        });

        await waitFor(() => {
            expect(screen.getByTestId('spec-text')).toHaveTextContent('"spec"');
            expect(screen.getByTestId('spec-text')).toHaveTextContent('"cores": 4');
        });
        expect(screen.getByTestId('spec-text')).not.toHaveTextContent(
            'spec.template.spec.domain.cpu.cores'
        );
    });

    it('hydrates existing nested values after schema becomes available', async () => {
        function DelayedSchemaHarness() {
            const [ready, setReady] = useState(false);

            return (
                <>
                    <button type="button" onClick={() => setReady(true)}>
                        enable-schema
                    </button>
                    <Form layout="vertical">
                        <Form.Item
                            name="spec_text"
                            initialValue='{"spec":{"template":{"spec":{"domain":{"cpu":{"cores":2}}}}}}'
                        >
                            <DynamicSchemaForm
                                schema={ready ? minimalSchema : (undefined as unknown as SchemaNode)}
                                mask={ready ? minimalMask : (undefined as unknown as SchemaMask)}
                            />
                        </Form.Item>
                    </Form>
                </>
            );
        }

        render(<DelayedSchemaHarness />);

        await act(async () => {
            screen.getByText('enable-schema').click();
        });

        await waitFor(() => {
            expect(screen.getByDisplayValue('2')).toBeInTheDocument();
        });
    });

    it('hydrates advanced gpu array fields from nested spec_text', async () => {
        render(
            <Form layout="vertical">
                <Form.Item
                    name="spec_text"
                    initialValue='{"spec":{"template":{"spec":{"domain":{"devices":{"gpus":[{"name":"gpu0","deviceName":"nvidia.com/A10"}]}}}}}}'
                >
                    <DynamicSchemaForm
                        schema={minimalSchema}
                        mask={{
                            quick_fields: [],
                            advanced_fields: [
                                {
                                    path: 'spec.template.spec.domain.devices.gpus',
                                    display_name: 'GPU Devices',
                                },
                            ],
                        }}
                    />
                </Form.Item>
            </Form>
        );

        await waitFor(() => {
            expect(screen.getByDisplayValue('gpu0')).toBeInTheDocument();
            expect(screen.getByDisplayValue('nvidia.com/A10')).toBeInTheDocument();
        });
    });
});
