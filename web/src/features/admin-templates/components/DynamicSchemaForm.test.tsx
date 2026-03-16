import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { Form } from 'antd';
import { beforeAll, describe, expect, it, vi } from 'vitest';
import { useRef, useState } from 'react';

import {
    HUGEPAGES_PAGE_SIZE_PATH,
    isValidHugepagesPageSizeValue,
    normalizeHugepagesPageSizeValue,
} from '@/lib/hugepages';
import {
    DynamicSchemaForm,
    type DynamicSchemaFormHandle,
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
                        metadata: {
                            type: 'object',
                            properties: {
                                annotations: {
                                    type: 'object',
                                    additionalProperties: {
                                        type: 'string',
                                    },
                                },
                            },
                        },
                        spec: {
                            type: 'object',
                            properties: {
                                nodeSelector: {
                                    type: 'object',
                                    additionalProperties: {
                                        type: 'string',
                                    },
                                },
                                domain: {
                                    type: 'object',
                                    properties: {
                                        cpu: {
                                            type: 'object',
                                            properties: {
                                                cores: { type: 'integer' },
                                                model: { type: 'string' },
                                            },
                                        },
                                        devices: {
                                            type: 'object',
                                            properties: {
                                                interfaces: {
                                                    type: 'array',
                                                    items: {
                                                        type: 'object',
                                                        properties: {
                                                            name: { type: 'string' },
                                                            model: { type: 'string' },
                                                            bridge: { type: 'object', properties: {} },
                                                        },
                                                    },
                                                },
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
                                        clock: {
                                            type: 'object',
                                            properties: {
                                                utc: {
                                                    type: 'object',
                                                    properties: {},
                                                },
                                            },
                                        },
                                    },
                                },
                                networks: {
                                    type: 'array',
                                    items: {
                                        type: 'object',
                                        properties: {
                                            name: { type: 'string' },
                                            pod: { type: 'object', properties: {} },
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

describe('DynamicSchemaForm', () => {
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
    }, 10000);

    it('normalizes custom MB hugepages input', () => {
        expect(normalizeHugepagesPageSizeValue('512')).toBe('512Mi');
        expect(normalizeHugepagesPageSizeValue(' 1024 Mi ')).toBe('1024Mi');
        expect(normalizeHugepagesPageSizeValue('1gi')).toBe('1Gi');
        expect(normalizeHugepagesPageSizeValue('')).toBeUndefined();
    }, 10000);

    it('accepts presets and custom MB values only', () => {
        expect(isValidHugepagesPageSizeValue('2Mi')).toBe(true);
        expect(isValidHugepagesPageSizeValue('1Gi')).toBe(true);
        expect(isValidHugepagesPageSizeValue('512Mi')).toBe(true);

        expect(isValidHugepagesPageSizeValue('4Gi')).toBe(false);
        expect(isValidHugepagesPageSizeValue('abc')).toBe(false);
        expect(isValidHugepagesPageSizeValue(512)).toBe(false);
    }, 10000);

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
    }, 10000);

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
    }, 10000);

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
    }, 10000);

    it('renders empty-object schema nodes as presence toggles', async () => {
        function Harness() {
            const [form] = Form.useForm();
            const formRef = useRef<DynamicSchemaFormHandle>(null);

            return (
                <Form form={form} layout="vertical">
                    <Form.Item name="spec_text" initialValue="{}">
                        <DynamicSchemaForm
                            ref={formRef}
                            schema={minimalSchema}
                            mask={{
                                quick_fields: [],
                                advanced_fields: [
                                    {
                                        path: 'spec.template.spec.domain.clock.utc',
                                        display_name: 'UTC Clock',
                                    },
                                ],
                            }}
                        />
                    </Form.Item>
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

        fireEvent.click(screen.getByRole('button', { name: /advanced features/i }));
        const checkbox = screen.getByRole('checkbox');
        fireEvent.click(checkbox);

        await act(async () => {
            screen.getByText('sync').click();
        });

        await waitFor(() => {
            expect(screen.getByTestId('spec-text')).toHaveTextContent('"utc": {}');
        });
    }, 10000);

    it('allows raw json editing for fields outside the structured mask', async () => {
        function Harness() {
            const [form] = Form.useForm();

            return (
                <Form form={form} layout="vertical">
                    <Form.Item name="spec_text" initialValue="{}">
                        <DynamicSchemaForm schema={minimalSchema} mask={minimalMask} />
                    </Form.Item>
                    <Form.Item shouldUpdate noStyle>
                        {() => <pre data-testid="spec-text">{form.getFieldValue('spec_text')}</pre>}
                    </Form.Item>
                </Form>
            );
        }

        render(<Harness />);

        fireEvent.change(screen.getByTestId('dynamic-form-raw-json'), {
            target: {
                value: '{\n  "spec": {\n    "template": {\n      "spec": {\n        "domain": {\n          "cpu": {\n            "cores": 6\n          },\n          "clock": {\n            "utc": {}\n          }\n        }\n      }\n    }\n  }\n}',
            },
        });

        await waitFor(() => {
            expect(screen.getByTestId('spec-text')).toHaveTextContent('"cores": 6');
            expect(screen.getByTestId('spec-text')).toHaveTextContent('"utc": {}');
        });
    }, 10000);

    it('clears hugepages field state when raw json removes the hugepages block', async () => {
        function Harness() {
            const [form] = Form.useForm();

            return (
                <Form form={form} layout="vertical">
                    <Form.Item
                        name="spec_text"
                        initialValue='{"spec":{"template":{"spec":{"domain":{"memory":{"hugepages":{"pageSize":"2Mi"}}}}}}}'
                    >
                        <DynamicSchemaForm schema={minimalSchema} mask={minimalMask} />
                    </Form.Item>
                    <Form.Item shouldUpdate noStyle>
                        {() => (
                            <pre data-testid="hugepages-value">
                                {String(
                                    form.getFieldValue([
                                        'spec',
                                        'template',
                                        'spec',
                                        'domain',
                                        'memory',
                                        'hugepages',
                                        'pageSize',
                                    ]) ?? ''
                                )}
                            </pre>
                        )}
                    </Form.Item>
                </Form>
            );
        }

        render(<Harness />);

        await waitFor(() => {
            expect(screen.getByTestId('hugepages-value')).toHaveTextContent('2Mi');
        });

        fireEvent.change(screen.getByTestId('dynamic-form-raw-json'), {
            target: {
                value: '{\n  "spec": {\n    "template": {\n      "spec": {\n        "domain": {\n          "cpu": {\n            "cores": 6\n          }\n        }\n      }\n    }\n  }\n}',
            },
        });

        await waitFor(() => {
            expect(screen.getByTestId('hugepages-value')).toHaveTextContent('');
            expect(screen.queryByText('2Mi')).not.toBeInTheDocument();
        });
    }, 10000);

    it('recognizes custom json fields into the recognition panel and keeps them visible after clearing', async () => {
        function Harness() {
            const [form] = Form.useForm();
            const formRef = useRef<DynamicSchemaFormHandle>(null);

            return (
                <Form form={form} layout="vertical">
                    <Form.Item name="spec_text" initialValue="{}">
                        <DynamicSchemaForm ref={formRef} schema={minimalSchema} mask={minimalMask} />
                    </Form.Item>
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

        fireEvent.click(screen.getByRole('button', { name: /json recognition/i }));

        fireEvent.change(screen.getByTestId('dynamic-form-raw-json'), {
            target: {
                value: '{\n  "spec": {\n    "template": {\n      "spec": {\n        "domain": {\n          "cpu": {\n            "model": "host-passthrough"\n          }\n        }\n      }\n    }\n  }\n}',
            },
        });

        const modelInput = await screen.findByDisplayValue('host-passthrough');
        fireEvent.change(modelInput, { target: { value: 'host-model' } });

        await act(async () => {
            screen.getByText('sync').click();
        });

        await waitFor(() => {
            expect(screen.getByTestId('spec-text')).toHaveTextContent('"model": "host-model"');
        });

        fireEvent.change(screen.getByTestId('dynamic-form-spec.template.spec.domain.cpu.model'), {
            target: { value: '' },
        });

        await waitFor(() => {
            expect(screen.getByTestId('dynamic-form-spec.template.spec.domain.cpu.model')).toBeInTheDocument();
        });
    }, 10000);

    it('does not recognize fields that are already managed by the parent form', async () => {
        render(
            <Form layout="vertical">
                <Form.Item name="spec_text" initialValue="{}">
                    <DynamicSchemaForm
                        schema={minimalSchema}
                        mask={{ quick_fields: [], advanced_fields: [] }}
                        recognizedExcludedPaths={['spec.template.spec.domain.cpu.cores']}
                    />
                </Form.Item>
            </Form>
        );

        fireEvent.change(screen.getByTestId('dynamic-form-raw-json'), {
            target: {
                value: '{\n  "spec": {\n    "template": {\n      "spec": {\n        "domain": {\n          "cpu": {\n            "cores": 4,\n            "model": "host-passthrough"\n          }\n        }\n      }\n    }\n  }\n}',
            },
        });

        fireEvent.click(screen.getByRole('button', { name: /json recognition/i }));

        await screen.findByDisplayValue('host-passthrough');
        expect(screen.queryByTestId('dynamic-form-spec.template.spec.domain.cpu.cores')).not.toBeInTheDocument();
    }, 10000);

    it('renders string map fields from mask and preserves dotted keys', async () => {
        function Harness() {
            const [form] = Form.useForm();
            const formRef = useRef<DynamicSchemaFormHandle>(null);

            return (
                <Form form={form} layout="vertical">
                    <Form.Item
                        name="spec_text"
                        initialValue='{"spec":{"template":{"spec":{"nodeSelector":{"kubevirt.io/ksm-enabled":"true"}}}}}'
                    >
                        <DynamicSchemaForm
                            ref={formRef}
                            schema={minimalSchema}
                            mask={{
                                quick_fields: [],
                                advanced_fields: [
                                    {
                                        path: 'spec.template.spec.nodeSelector',
                                        display_name: 'Node Selector',
                                    },
                                ],
                            }}
                        />
                    </Form.Item>
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

        fireEvent.click(screen.getByRole('button', { name: /advanced features/i }));

        const keyInput = await screen.findByTestId('dynamic-form-spec.template.spec.nodeSelector-key-0');
        const valueInput = screen.getByTestId('dynamic-form-spec.template.spec.nodeSelector-value-0');

        expect(keyInput).toHaveValue('kubevirt.io/ksm-enabled');
        expect(valueInput).toHaveValue('true');

        fireEvent.change(valueInput, { target: { value: 'false' } });

        await act(async () => {
            screen.getByText('sync').click();
        });

        await waitFor(() => {
            expect(screen.getByTestId('spec-text')).toHaveTextContent('"kubevirt.io/ksm-enabled": "false"');
        });
    }, 10000);

    it('preserves empty presence objects inside array items when syncing', async () => {
        function Harness() {
            const [form] = Form.useForm();
            const formRef = useRef<DynamicSchemaFormHandle>(null);

            return (
                <Form form={form} layout="vertical">
                    <Form.Item
                        name="spec_text"
                        initialValue='{"spec":{"template":{"spec":{"domain":{"devices":{"interfaces":[{"name":"default","model":"virtio","bridge":{}}]}},"networks":[{"name":"default","pod":{}}]}}}}'
                    >
                        <DynamicSchemaForm
                            ref={formRef}
                            schema={minimalSchema}
                            mask={{
                                quick_fields: [],
                                advanced_fields: [
                                    {
                                        path: 'spec.template.spec.domain.devices.interfaces',
                                        display_name: 'Interfaces',
                                    },
                                    {
                                        path: 'spec.template.spec.networks',
                                        display_name: 'Networks',
                                    },
                                ],
                            }}
                        />
                    </Form.Item>
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

        fireEvent.click(screen.getByRole('button', { name: /advanced features/i }));

        await act(async () => {
            screen.getByText('sync').click();
        });

        await waitFor(() => {
            expect(screen.getByTestId('spec-text')).toHaveTextContent('"bridge": {}');
            expect(screen.getByTestId('spec-text')).toHaveTextContent('"pod": {}');
        });
    }, 10000);
});
