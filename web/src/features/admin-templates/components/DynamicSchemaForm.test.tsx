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
    buildRecognizedMaskFields,
    buildScalarMapState,
    deriveEnumOptionsFromDescription,
    DynamicSchemaForm,
    type DynamicSchemaFormHandle,
    normalizeMapRows,
    pruneSpecTree,
    type SchemaMask,
    type SchemaNode,
    validateRawEditorText,
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

function openCollapsePanel(label: string) {
    fireEvent.click(screen.getByText(label));
}

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
                                                autoattachGraphicsDevice: {
                                                    type: 'boolean',
                                                    description: 'Whether to attach the default graphics device or not.',
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
    });

    it('renders boolean mask fields with english heading, persistent help, and a single toggle control', () => {
        const booleanMask: SchemaMask = {
            quick_fields: [
                {
                    path: 'spec.template.spec.domain.devices.autoattachGraphicsDevice',
                    display_name: 'Auto-attach Graphics',
                    help_text:
                        'Keep the minimal graphics device attached so the noVNC console remains available.',
                },
            ],
        };

        render(
            <Form layout="vertical">
                <Form.Item name="spec_text" initialValue="{}">
                    <DynamicSchemaForm schema={minimalSchema} mask={booleanMask} />
                </Form.Item>
            </Form>
        );

        expect(screen.getByText('Auto-attach Graphics')).toBeVisible();
        expect(
            screen.getByText(
                'Keep the minimal graphics device attached so the noVNC console remains available.',
            ),
        ).toBeVisible();
        expect(
            screen
                .getByTestId('dynamic-form-spec.template.spec.domain.devices.autoattachGraphicsDevice')
                .closest('.ant-checkbox-wrapper'),
        ).toBeVisible();
    });

    it('renders introduced-in badges for fields added in newer KubeVirt baselines', () => {
        const versionedMask: SchemaMask = {
            quick_fields: [
                {
                    path: 'spec.template.spec.domain.devices.autoattachGraphicsDevice',
                    display_name: 'Auto-attach Graphics',
                    introduced_in: '1.8.0',
                },
            ],
        };

        render(
            <Form layout="vertical">
                <Form.Item name="spec_text" initialValue="{}">
                    <DynamicSchemaForm schema={minimalSchema} mask={versionedMask} />
                </Form.Item>
            </Form>,
        );

        expect(screen.getByText('v1.8.0+')).toBeVisible();
        expect(screen.getByText('Auto-attach Graphics')).toBeVisible();
    });

    it('derives enum-like options from description text', () => {
        expect(
            deriveEnumOptionsFromDescription(
                'The possible options are: - "None": no action. - "LiveMigrate": migrate. - "LiveMigrateIfPossible": migrate if possible.',
            ),
        ).toEqual(['None', 'LiveMigrate', 'LiveMigrateIfPossible']);
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

    it('does not emit a redundant parent update when sync keeps spec_text unchanged', async () => {
        vi.useFakeTimers();
        const onValuesChange = vi.fn();

        function Harness() {
            const [form] = Form.useForm();
            const formRef = useRef<DynamicSchemaFormHandle>(null);

            return (
                <Form form={form} layout="vertical" onValuesChange={onValuesChange}>
                    <Form.Item name="spec_text" initialValue="{}">
                        <DynamicSchemaForm ref={formRef} schema={minimalSchema} mask={minimalMask} />
                    </Form.Item>
                    <button type="button" onClick={() => formRef.current?.sync()}>
                        sync
                    </button>
                </Form>
            );
        }

        render(<Harness />);

        await act(async () => {
            screen.getByText('sync').click();
            vi.advanceTimersByTime(350);
        });

        expect(onValuesChange).not.toHaveBeenCalled();
        vi.useRealTimers();
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

        openCollapsePanel('Advanced Features');

        await waitFor(() => {
            expect(screen.getByDisplayValue('gpu0')).toBeInTheDocument();
            expect(screen.getByDisplayValue('nvidia.com/A10')).toBeInTheDocument();
        });
    });

    it('pre-renders advanced and professional fields so collapsed sections keep hydrated values', async () => {
        render(
            <Form layout="vertical">
                <Form.Item
                    name="spec_text"
                    initialValue='{"spec":{"template":{"spec":{"domain":{"cpu":{"model":"host-passthrough"},"clock":{"utc":{}}}}}}}'
                >
                    <DynamicSchemaForm
                        schema={minimalSchema}
                        mask={{
                            quick_fields: [],
                            advanced_fields: [
                                {
                                    path: 'spec.template.spec.domain.cpu.model',
                                    display_name: 'CPU Model',
                                },
                            ],
                            professional_fields: [
                                {
                                    path: 'spec.template.spec.domain.clock.utc',
                                    display_name: 'UTC Clock',
                                },
                            ],
                        }}
                    />
                </Form.Item>
            </Form>,
        );

        await waitFor(() => {
            expect(
                screen.getByTestId('dynamic-form-spec.template.spec.domain.cpu.model'),
            ).toHaveValue('host-passthrough');
            expect(
                screen.getByTestId('dynamic-form-spec.template.spec.domain.clock.utc'),
            ).toBeChecked();
        });
    });

    it('preserves empty-object presence nodes when pruning', () => {
        expect(
            pruneSpecTree(
                {
                    spec: {
                        template: {
                            spec: {
                                domain: {
                                    clock: {
                                        utc: {},
                                    },
                                },
                            },
                        },
                    },
                },
                undefined,
                '',
                minimalSchema,
            ),
        ).toEqual({
            spec: {
                template: {
                    spec: {
                        domain: {
                            clock: {
                                utc: {},
                            },
                        },
                    },
                },
            },
        });
    });

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

        openCollapsePanel('Raw KubeVirt JSON');

        fireEvent.change(screen.getByTestId('dynamic-form-raw-json'), {
            target: {
                value: '{\n  "spec": {\n    "template": {\n      "spec": {\n        "domain": {\n          "cpu": {\n            "cores": 6\n          },\n          "clock": {\n            "utc": {}\n          }\n        }\n      }\n    }\n  }\n}',
            },
        });

        await waitFor(() => {
            expect(screen.getByTestId('spec-text')).toHaveTextContent('"cores": 6');
            expect(screen.getByTestId('spec-text')).toHaveTextContent('"utc": {}');
        });
    });

    it('clears hugepages field state when raw json removes the hugepages block', async () => {
        const consoleErrorSpy = vi.spyOn(console, 'error').mockImplementation(() => {});

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

        openCollapsePanel('Raw KubeVirt JSON');

        fireEvent.change(screen.getByTestId('dynamic-form-raw-json'), {
            target: {
                value: '{\n  "spec": {\n    "template": {\n      "spec": {\n        "domain": {\n          "cpu": {\n            "cores": 6\n          }\n        }\n      }\n    }\n  }\n}',
            },
        });

        await waitFor(() => {
            expect(screen.getByTestId('hugepages-value')).toHaveTextContent('');
            expect(screen.queryByText('2Mi')).not.toBeInTheDocument();
        });
        expect(
            consoleErrorSpy.mock.calls.some((call) =>
                call.some((value) => String(value).includes('circular references')),
            ),
        ).toBe(false);
        consoleErrorSpy.mockRestore();
    });

    it('recognizes custom json fields outside the mask', () => {
        expect(
            buildRecognizedMaskFields(
                minimalSchema,
                minimalMask,
                {
                    spec: {
                        template: {
                            spec: {
                                domain: {
                                    cpu: {
                                        model: 'host-passthrough',
                                    },
                                },
                            },
                        },
                    },
                },
            ),
        ).toEqual([
            {
                path: 'spec.template.spec.domain.cpu.model',
                display_name: 'CPU Model',
            },
        ]);
    });

    it('does not recognize fields excluded by parent-managed paths', () => {
        expect(
            buildRecognizedMaskFields(
                minimalSchema,
                { quick_fields: [], advanced_fields: [] },
                {
                    spec: {
                        template: {
                            spec: {
                                domain: {
                                    cpu: {
                                        cores: 4,
                                        model: 'host-passthrough',
                                    },
                                },
                            },
                        },
                    },
                },
                ['spec.template.spec.domain.cpu.cores'],
            ),
        ).toEqual([
            {
                path: 'spec.template.spec.domain.cpu.model',
                display_name: 'CPU Model',
            },
        ]);
    });

    it('preserves dotted keys when normalizing string map rows', () => {
        const valueNode = { type: 'string' };
        const { rows } = buildScalarMapState(
            { 'kubevirt.io/ksm-enabled': 'true' },
            valueNode,
        );
        expect(
            normalizeMapRows(
                rows.map((row) => ({
                    ...row,
                    value: row.keyText === 'kubevirt.io/ksm-enabled' ? 'false' : row.value,
                })),
                valueNode,
            ),
        ).toEqual({ 'kubevirt.io/ksm-enabled': 'false' });
    });

    it('preserves empty presence objects inside array items when pruning', () => {
        expect(
            pruneSpecTree(
                {
                    spec: {
                        template: {
                            spec: {
                                domain: {
                                    devices: {
                                        interfaces: [{ name: 'default', model: 'virtio', bridge: {} }],
                                    },
                                },
                                networks: [{ name: 'default', pod: {} }],
                            },
                        },
                    },
                },
                undefined,
                '',
                minimalSchema,
            ),
        ).toEqual({
            spec: {
                template: {
                    spec: {
                        domain: {
                            devices: {
                                interfaces: [{ name: 'default', model: 'virtio', bridge: {} }],
                            },
                        },
                        networks: [{ name: 'default', pod: {} }],
                    },
                },
            },
        });
    });

    it('validates raw editor json as object-only input', () => {
        const t = (_key: string, fallback?: string) => fallback ?? _key;
        expect(validateRawEditorText('', t as never)).toBeNull();
        expect(validateRawEditorText('[]', t as never)).toContain('Spec JSON must be an object');
        expect(validateRawEditorText('{', t as never)).toContain('JSON is invalid');
    });
});
