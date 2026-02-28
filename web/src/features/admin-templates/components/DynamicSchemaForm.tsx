import React, { useCallback, useEffect, useImperativeHandle, useRef } from 'react';
import {
    Alert,
    Button,
    Card,
    Checkbox,
    Collapse,
    Form,
    Input,
    InputNumber,
    Select,
    Space,
    Tooltip,
    Typography,
} from 'antd';
import { MinusCircleOutlined, PlusOutlined, QuestionCircleOutlined } from '@ant-design/icons';
import { useTranslation } from 'react-i18next';

const { Text } = Typography;

// ─── Shared Types ────────────────────────────────────────────────────────────
// These types are co-located because DynamicSchemaForm owns the rendering contract.
// Consumers (hooks, parent components) import from this file.

export interface MaskField {
    path: string;
    display_name?: string;
    display_name_key?: string;
    help_text?: string;
    help_key?: string;
    placeholder?: string;
    placeholder_key?: string;
}

export interface SchemaMask {
    quick_fields: MaskField[];
    advanced_fields?: MaskField[];
}

export interface SchemaNode {
    type?: string;
    properties?: Record<string, SchemaNode>;
    items?: SchemaNode;
    enum?: (string | number)[];
    [key: string]: unknown;
}

export interface DynamicSchemaFormProps {
    /** JSON string stored in the outer Form field (spec_text). Injected by Form.Item. */
    value?: string;
    /** Callback to notify outer Form.Item of new JSON string. Injected by Form.Item. */
    onChange?: (value: string) => void;
    /** Standard OpenAPI JSON Schema format providing field types and constraints. */
    schema: SchemaNode;
    /** UI projection — defines which paths to expose and how to label them. */
    mask: SchemaMask;
    disabled?: boolean;
}

/**
 * Imperative handle exposed via ref so parent Form.onValuesChange can trigger
 * serialization without any render-phase side effects.
 *
 * Usage in parent:
 *   const formRef = useRef<DynamicSchemaFormHandle>(null);
 *   <Form onValuesChange={() => formRef.current?.sync()}>
 *     <Form.Item name="spec_text"><DynamicSchemaForm ref={formRef} ... /></Form.Item>
 *   </Form>
 */
export interface DynamicSchemaFormHandle {
    /** Serialize current dynamic field values → spec_text JSON string. */
    sync: () => void;
}

export const HUGEPAGES_PAGE_SIZE_PATH = 'spec.template.spec.domain.memory.hugepages.pageSize';
export const HUGEPAGES_PRESET_OPTIONS = ['2Mi', '1Gi'] as const;

/**
 * Normalizes hugepages pageSize user input:
 * - "512"   -> "512Mi" (custom MB input)
 * - "512Mi" -> "512Mi"
 * - "1Gi"   -> "1Gi"
 */
export function normalizeHugepagesPageSizeValue(value: unknown): string | undefined {
    if (typeof value !== 'string') return undefined;
    const trimmed = value.trim();
    if (!trimmed) return undefined;

    const compact = trimmed.replace(/\s+/g, '');
    const mbOnly = compact.match(/^([1-9]\d*)$/);
    if (mbOnly) {
        return `${mbOnly[1]}Mi`;
    }
    const mi = compact.match(/^([1-9]\d*)mi$/i);
    if (mi) {
        return `${mi[1]}Mi`;
    }
    const gi = compact.match(/^([1-9]\d*)gi$/i);
    if (gi) {
        return `${gi[1]}Gi`;
    }

    return compact;
}

export function isValidHugepagesPageSizeValue(value: unknown): boolean {
    if (value === undefined || value === null || value === '') return true;
    if (typeof value !== 'string') return false;
    if (HUGEPAGES_PRESET_OPTIONS.includes(value as (typeof HUGEPAGES_PRESET_OPTIONS)[number])) {
        return true;
    }
    // Custom value is MB-only (Mi) to keep operator input predictable.
    return /^[1-9]\d*Mi$/.test(value);
}

// ─── Spec Overrides Serialisation ─────────────────────────────────────────────

/**
 * Flattens a nested object produced by Ant Design Form into flat dot-notation keys.
 *
 * master-flow.md §Stage 3 Step 4 defines spec_overrides as flat dot-notation keys:
 *   { "spec.template.spec.domain.cpu.dedicatedCpuPlacement": true }
 *
 * Ant Design Form.Item with namePath=["spec","template","..."] stores values as nested:
 *   { spec: { template: { spec: { domain: { cpu: { dedicatedCpuPlacement: true } } } } } }
 *
 * This function converts the nested Ant Design representation back to the flat format
 * required by the backend spec_overrides validator and the database JSONB storage.
 *
 * Arrays are preserved as-is (not flattened) because spec_overrides array values
 * (e.g., GPU device lists) represent entire resource lists that must remain intact.
 */
function flattenToSpecOverrides(
    obj: Record<string, unknown>,
    prefix = '',
    result: Record<string, unknown> = {},
): Record<string, unknown> {
    for (const [key, value] of Object.entries(obj)) {
        const fullKey = prefix ? `${prefix}.${key}` : key;
        if (
            value !== null &&
            value !== undefined &&
            typeof value === 'object' &&
            !Array.isArray(value)
        ) {
            // Recurse into nested plain objects.
            flattenToSpecOverrides(value as Record<string, unknown>, fullKey, result);
        } else if (value !== null && value !== undefined) {
            // Leaf value (primitive, array, etc.) — emit as flat key.
            result[fullKey] = value;
        }
        // Skip null/undefined values — those represent unset form fields.
    }
    return result;
}

// ─── Schema Path Resolution ───────────────────────────────────────────────────

/**
 * Deeply resolves a dot-notation path against a standard JSON Object Schema.
 * Returns null for invalid paths (caller must handle gracefully — no throwing).
 * Stage 1 design: "Invalid mask paths must fail validation before deployment."
 * At runtime we degrade rather than crash.
 */
const resolveSchemaNode = (schema: SchemaNode, path: string): SchemaNode | null => {
    if (!schema) return null;
    const parts = path.split('.');
    let current: SchemaNode = schema;
    for (const part of parts) {
        if (current?.properties?.[part]) {
            current = current.properties[part];
        } else if (current?.items?.properties?.[part]) {
            // Allow one level of array traversal for UI mask paths.
            current = current.items.properties[part];
        } else {
            return null; // Path not found — caller renders a degraded placeholder.
        }
    }
    return current;
};

// ─── Field Renderers ──────────────────────────────────────────────────────────

interface DynamicFieldGroupProps {
    node: SchemaNode;
    namePath: (string | number)[];
    label: string;
    fieldPath?: string;
    helpText?: string;
    placeholder?: string;
    disabled?: boolean;
}

/**
 * Pure rendering component — renders a single schema node as the appropriate
 * Ant Design form control.  No Form instance is created here; this component
 * lives inside the outer Form provided by the parent page.
 *
 * Mapping per master-flow.md Stage 1:
 *   array   → dynamic add/remove list  (Form.List)
 *   enum    → dropdown                 (Select, options from schema)
 *   integer → numeric input            (InputNumber)
 *   boolean → toggle                   (Switch)
 *   string  → text input               (Input)   [default]
 */
const DynamicFieldGroup: React.FC<DynamicFieldGroupProps> = ({
    node,
    namePath,
    label,
    fieldPath,
    helpText,
    placeholder,
    disabled,
}) => {
    const { t } = useTranslation(['admin', 'common']);

    if (!node) return null;

    // array → dynamic add/remove table
    if (node.type === 'array' && node.items?.properties) {
        const itemKeys = Object.keys(node.items.properties);
        return (
            <Card
                size="small"
                title={(
                    <Space size={6}>
                        <Text strong>{label}</Text>
                        {helpText && (
                            <Tooltip title={helpText} trigger={['hover', 'click']}>
                                <QuestionCircleOutlined style={{ color: 'rgba(0,0,0,0.45)' }} />
                            </Tooltip>
                        )}
                    </Space>
                )}
                style={{ marginBottom: 16 }}
            >
                <Form.List name={namePath}>
                    {(fields, { add, remove }) => (
                        <div style={{ display: 'flex', flexDirection: 'column', gap: '8px' }}>
                            {fields.map((field) => (
                                <Space
                                    key={field.key}
                                    style={{ display: 'flex', marginBottom: 8, alignItems: 'flex-start' }}
                                    align="baseline"
                                >
                                    <div
                                        style={{
                                            flex: 1,
                                            padding: 8,
                                            border: '1px solid #f0f0f0',
                                            borderRadius: 4,
                                        }}
                                    >
                                        {itemKeys.map((itemKey) => {
                                            const subNode = node.items!.properties![itemKey];
                                            return (
                                                <DynamicFieldGroup
                                                    key={itemKey}
                                                    node={subNode}
                                                    namePath={[field.name, itemKey]}
                                                    label={itemKey}
                                                    fieldPath={fieldPath ? `${fieldPath}.${itemKey}` : undefined}
                                                    placeholder={placeholder}
                                                    disabled={disabled}
                                                />
                                            );
                                        })}
                                    </div>
                                    <Button
                                        type="text"
                                        danger
                                        icon={<MinusCircleOutlined />}
                                        onClick={() => remove(field.name)}
                                        disabled={disabled}
                                    />
                                </Space>
                            ))}
                            {!disabled && (
                                <Button
                                    type="dashed"
                                    onClick={() => add()}
                                    block
                                    icon={<PlusOutlined />}
                                >
                                    {t('dynamic_form.add_item', { label })}
                                </Button>
                            )}
                        </div>
                    )}
                </Form.List>
            </Card>
        );
    }

    if (fieldPath === HUGEPAGES_PAGE_SIZE_PATH) {
        return (
            <Form.Item
                label={label}
                name={namePath}
                style={{ marginBottom: 8 }}
                tooltip={helpText ? { title: helpText, trigger: ['hover', 'click'] } : undefined}
                getValueProps={(fieldValue?: string) => ({
                    value: fieldValue ? [fieldValue] : [],
                })}
                getValueFromEvent={(nextValue: unknown) => {
                    if (Array.isArray(nextValue)) {
                        const latest = nextValue[nextValue.length - 1];
                        return normalizeHugepagesPageSizeValue(latest);
                    }
                    return normalizeHugepagesPageSizeValue(nextValue);
                }}
                rules={[
                    {
                        validator: (_, fieldValue: unknown) => {
                            if (isValidHugepagesPageSizeValue(fieldValue)) {
                                return Promise.resolve();
                            }
                            return Promise.reject(
                                new Error(
                                    t(
                                        'dynamic_form.hugepages_invalid',
                                        'Hugepages must be 2Mi/1Gi, or a custom MB value (e.g. 512).'
                                    )
                                )
                            );
                        },
                    },
                ]}
            >
                <Select
                    mode="tags"
                    maxCount={1}
                    allowClear
                    disabled={disabled}
                    data-testid={`dynamic-form-${namePath.join('.')}`}
                    placeholder={placeholder ?? t('dynamic_form.hugepages_placeholder', 'Select 2Mi/1Gi or input MB')}
                    options={HUGEPAGES_PRESET_OPTIONS.map((opt) => ({ label: opt, value: opt }))}
                />
            </Form.Item>
        );
    }

    // enum → dropdown (options from schema, not developer-defined — Stage 1 constraint)
    if (node.enum && Array.isArray(node.enum)) {
        return (
            <Form.Item
                label={label}
                name={namePath}
                style={{ marginBottom: 8 }}
                tooltip={helpText ? { title: helpText, trigger: ['hover', 'click'] } : undefined}
            >
                <Select
                    disabled={disabled}
                    data-testid={`dynamic-form-${namePath.join('.')}`}
                    placeholder={placeholder}
                >
                    {node.enum.map((opt: string | number) => (
                        <Select.Option key={String(opt)} value={opt}>
                            {String(opt)}
                        </Select.Option>
                    ))}
                </Select>
            </Form.Item>
        );
    }

    // integer/number → numeric input
    // boolean → checkbox  (master-flow.md:167: "boolean → checkbox")
    // string (default) → text input
    return (
        <Form.Item
            label={label}
            name={namePath}
            valuePropName={node.type === 'boolean' ? 'checked' : 'value'}
            style={{ marginBottom: 8 }}
            tooltip={helpText ? { title: helpText, trigger: ['hover', 'click'] } : undefined}
        >
            {node.type === 'integer' || node.type === 'number' ? (
                <InputNumber
                    style={{ width: '100%' }}
                    disabled={disabled}
                    data-testid={`dynamic-form-${namePath.join('.')}`}
                    placeholder={placeholder}
                />
            ) : node.type === 'boolean' ? (
                // master-flow.md Stage 1: "boolean → checkbox" (not Switch).
                // Checkbox communicates a binary on/off choice matching spec field semantics.
                <Checkbox
                    disabled={disabled}
                    data-testid={`dynamic-form-${namePath.join('.')}`}
                >
                    {label}
                </Checkbox>
            ) : (
                <Input
                    disabled={disabled}
                    data-testid={`dynamic-form-${namePath.join('.')}`}
                    placeholder={placeholder}
                />
            )}
        </Form.Item>
    );
};

// ─── Main Component ───────────────────────────────────────────────────────────

/**
 * DynamicSchemaForm — Schema-driven form widget, designed as an Ant Design
 * custom Form control.
 *
 * ARCHITECTURE NOTE:
 * This component does NOT create its own Form instance.  It renders Form.Item /
 * Form.List elements that attach directly to the *outer* Form provided by the
 * parent page (create modal / edit modal).  This is the correct Ant Design
 * custom-control pattern: value + onChange are injected by Form.Item; all field
 * values live in the single outer FormStore and are available when the parent
 * calls form.validateFields() or form.getFieldsValue().
 *
 * Data flow:
 *   outer Form field "spec_text" (JSON string)
 *     → value prop   → parsed → form.setFieldsValue() on mount/update
 *     ← onChange     ← JSON.stringify(allFields)   ← outer Form onValuesChange
 *
 * The parent MUST wrap this component in a <Form.Item name="spec_text"> with
 * valuePropName="value" so that Ant Design's FormStore injects value/onChange.
 */
export const DynamicSchemaForm = React.forwardRef<DynamicSchemaFormHandle, DynamicSchemaFormProps>(
    function DynamicSchemaForm(
        {
            value,
            onChange,
            schema,
            mask,
            disabled,
        },
        ref
    ) {
        const { t } = useTranslation(['admin', 'common']);
        const outerForm = Form.useFormInstance();
        const isInitializedRef = useRef(false);
        const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);

        // Initialise outer form fields from the JSON string value on first render
        // or when the modal opens with a different record.
        // NOTE: hooks must be called unconditionally (React rules-of-hooks).
        // The guard check for !schema || !mask happens after all hooks.
        useEffect(() => {
            if (!outerForm || !schema || !mask) return;
            isInitializedRef.current = false; // reset on value change (modal re-open)
            if (!value) {
                // Nothing to pre-fill; leave fields at their defaults.
                isInitializedRef.current = true;
                return;
            }
            try {
                const parsed = JSON.parse(value) as Record<string, unknown>;
                // setFieldsValue merges — it does not reset unrelated fields.
                outerForm.setFieldsValue(parsed);
            } catch {
                // Malformed stored value — log and continue with empty fields.
                console.warn('DynamicSchemaForm: failed to parse stored value:', value);
            }
            isInitializedRef.current = true;
            // eslint-disable-next-line react-hooks/exhaustive-deps
        }, [value]);

        // Sync outer Form values back to the JSON string.
        // Called imperatively via ref.sync() — invoked by the parent Form's
        // onValuesChange callback, NOT during render (React best practice).
        //
        // Antd docs: shouldUpdate is for conditional rendering only.
        // Side effects (data sync) belong in event callbacks (onValuesChange).
        //
        // IMPORTANT — spec_overrides serialisation format:
        // master-flow.md §Stage 3 Step 4 specifies spec_overrides as flat dot-notation keys:
        //   { "spec.template.spec.domain.cpu.dedicatedCpuPlacement": true }
        // NOT nested objects:
        //   { "spec": { "template": { ... } } }
        //
        // Ant Design Form stores field values under nested keys when the namePath is an array
        // (e.g., name={["spec", "template", "spec", "domain", "cpu", "dedicatedCpuPlacement"]}).
        // We MUST flatten back to dot-notation before emitting onChange so the backend
        // validator (HasDedicatedCPUInSpecOverrides) receives the expected format.
        const syncToParent = useCallback(() => {
            if (!outerForm || !onChange || !schema) return;
            if (debounceRef.current) clearTimeout(debounceRef.current);
            debounceRef.current = setTimeout(() => {
                const allValues = outerForm.getFieldsValue(true) as Record<string, unknown>;
                // Strip non-spec fields (display_name, os_family, etc.) so we only
                // serialise the dynamic spec fields.  The spec fields are those
                // whose top-level keys map to schema.properties keys.
                const specKeys = Object.keys(schema.properties ?? {});
                const nestedSpecValues: Record<string, unknown> = {};
                for (const k of specKeys) {
                    if (k in allValues) nestedSpecValues[k] = allValues[k];
                }
                // Flatten nested Ant Design Form values → flat dot-notation spec_overrides.
                // This converts { spec: { template: { spec: { domain: { cpu: { dedicatedCpuPlacement: true } } } } } }
                // back to { "spec.template.spec.domain.cpu.dedicatedCpuPlacement": true }
                // as required by master-flow.md §Stage 3 Step 4 and the backend validator.
                const flatSpecOverrides = flattenToSpecOverrides(nestedSpecValues);
                onChange(JSON.stringify(flatSpecOverrides, null, 2));
            }, 300);
        }, [outerForm, onChange, schema]);

        // Expose sync() imperatively so the parent Form's onValuesChange can call it.
        // This is the Ant Design recommended pattern: side effects in event handlers,
        // not in render. ref.sync() is called from outside; no render-phase side effects.
        useImperativeHandle(ref, () => ({ sync: syncToParent }), [syncToParent]);

        // Guard: render a hard error if required props are missing.
        // This is a developer error — schema and mask must always be provided.
        // IMPORTANT: placed AFTER all hooks to comply with react-hooks/rules-of-hooks.
        if (!schema || !mask) {
            return (
                <Alert
                    type="error"
                    banner
                    message="Frontend Standard Violation: DynamicSchemaForm rendered without schema or mask."
                />
            );
        }

        // ── Mask field rendering ──────────────────────────────────────────────────

        /**
         * Render mask fields.  Invalid paths degrade to a warning Alert rather than
         * throwing — consistent with the Stage 1 principle that runtime mask path
         * errors should never crash the UI.
         */
        const renderMaskElements = (fields: MaskField[]) => {
            return fields.map((field) => {
                const node = resolveSchemaNode(schema, field.path);
                if (!node) {
                    // Stage 1 degradation: warn, do not crash.
                    return (
                        <Alert
                            key={field.path}
                            type="warning"
                            showIcon
                            message={t('dynamic_form.invalid_path', {
                                path: field.path,
                                defaultValue: `Field path not found in schema: ${field.path}`,
                            })}
                            style={{ marginBottom: 8 }}
                        />
                    );
                }
                const namePath = field.path.split('.');
                const label =
                    field.display_name_key
                        ? t(field.display_name_key, field.display_name ?? field.path)
                        : (field.display_name ?? field.path);
                const helpText =
                    field.help_key
                        ? t(field.help_key, field.help_text ?? '')
                        : field.help_text;
                const placeholder =
                    field.placeholder_key
                        ? t(field.placeholder_key, field.placeholder ?? '')
                        : field.placeholder;
                return (
                    <DynamicFieldGroup
                        key={field.path}
                        node={node}
                        namePath={namePath}
                        label={label}
                        fieldPath={field.path}
                        helpText={helpText}
                        placeholder={placeholder}
                        disabled={disabled}
                    />
                );
            });
        };

        return (
            <Card
                size="small"
                title={t('templates.spec')}
                bodyStyle={{ padding: '16px 8px' }}
            >
                {/*
                 * No shouldUpdate side-effect hack here.
                 * sync() is called by the parent Form's onValuesChange via ref.
                 * This follows Ant Design's recommended pattern:
                 *   - shouldUpdate: conditional rendering only
                 *   - onValuesChange: data synchronization (side effects)
                 */}
                {mask.quick_fields && mask.quick_fields.length > 0 && (
                    <div className="quick-fields-section">
                        {renderMaskElements(mask.quick_fields)}
                    </div>
                )}

                {mask.advanced_fields && mask.advanced_fields.length > 0 && (
                    <Collapse
                        ghost
                        items={[
                            {
                                key: 'advanced',
                                label: t('dynamic_form.advanced_settings', 'Advanced Settings'),
                                children: renderMaskElements(mask.advanced_fields),
                            },
                        ]}
                        style={{ marginTop: 16 }}
                    />
                )}
            </Card>
        );
    }
);

DynamicSchemaForm.displayName = 'DynamicSchemaForm';
