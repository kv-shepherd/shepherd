import React, {
  useCallback,
  useEffect,
  useImperativeHandle,
  useMemo,
  useRef,
  useState,
} from "react";
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
  Tag,
  Tooltip,
  Typography,
  type CollapseProps,
} from "antd";
import {
  DeleteOutlined,
  MinusCircleOutlined,
  PlusOutlined,
  QuestionCircleOutlined,
} from "@ant-design/icons";
import { useTranslation } from "react-i18next";
import { resolveSchemaHelpText } from "@/i18n/schemaHelp";

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
  introduced_in?: string;
}

export interface SchemaMask {
  quick_fields: MaskField[];
  advanced_fields?: MaskField[];
  professional_fields?: MaskField[];
}

export interface SchemaNode {
  type?: string;
  properties?: Record<string, SchemaNode>;
  items?: SchemaNode;
  enum?: (string | number)[];
  additionalProperties?: SchemaNode | boolean;
  [key: string]: unknown;
}

interface DynamicSchemaFormProps {
  /** JSON string stored in the outer Form field (spec_text). Injected by Form.Item. */
  value?: string;
  /** Callback to notify outer Form.Item of new JSON string. Injected by Form.Item. */
  onChange?: (value: string) => void;
  /** Standard OpenAPI JSON Schema format providing field types and constraints. */
  schema: SchemaNode;
  /** UI projection — defines which paths to expose and how to label them. */
  mask: SchemaMask;
  disabled?: boolean;
  /** Spec paths already managed by parent-level form controls and should not reappear in JSON recognition. */
  recognizedExcludedPaths?: string[];
  /** Optional i18n scope for schema-backed help translations. */
  schemaHelpScope?: "instanceSize";
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

// ─── Spec Overrides Serialisation ─────────────────────────────────────────────

export function pruneSpecTree(
  value: unknown,
  preserveEmptyObjectPaths?: Set<string>,
  currentPath = "",
  schemaNode?: SchemaNode,
): unknown {
  if (Array.isArray(value)) {
    const items = value
      .map((item) =>
        pruneSpecTree(item, preserveEmptyObjectPaths, currentPath, schemaNode?.items),
      )
      .filter((item) => item !== undefined);
    return items.length > 0 ? items : undefined;
  }

  if (value !== null && value !== undefined && typeof value === "object") {
    const result: Record<string, unknown> = {};
    for (const [key, child] of Object.entries(value as Record<string, unknown>)) {
      const childPath = currentPath ? `${currentPath}.${key}` : key;
      const childSchemaNode =
        schemaNode?.properties?.[key] &&
        typeof schemaNode.properties[key] === "object"
          ? schemaNode.properties[key]
          : schemaNode?.additionalProperties &&
              typeof schemaNode.additionalProperties === "object"
            ? schemaNode.additionalProperties
            : undefined;
      const pruned = pruneSpecTree(
        child,
        preserveEmptyObjectPaths,
        childPath,
        childSchemaNode,
      );
      if (pruned !== undefined) {
        result[key] = pruned;
      }
    }
    if (Object.keys(result).length > 0) {
      return result;
    }
    return preserveEmptyObjectPaths?.has(currentPath) || (schemaNode ? isPresenceObjectNode(schemaNode) : false)
      ? {}
      : undefined;
  }

  if (typeof value === "string" && value.trim() === "") {
    return undefined;
  }

  return value === null || value === undefined ? undefined : value;
}

// ─── Schema Path Resolution ───────────────────────────────────────────────────

/**
 * Deeply resolves a dot-notation path against a standard JSON Object Schema.
 * Returns null for invalid paths (caller must handle gracefully — no throwing).
 * Stage 1 design: "Invalid mask paths must fail validation before deployment."
 * At runtime we degrade rather than crash.
 */
const resolveSchemaNode = (
  schema: SchemaNode,
  path: string,
): SchemaNode | null => {
  if (!schema) return null;
  const parts = path.split(".");
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

function parseCommittedValue(value?: string): Record<string, unknown> {
  if (!value || !value.trim()) {
    return {};
  }
  try {
    const parsed = JSON.parse(value) as unknown;
    if (parsed && typeof parsed === "object" && !Array.isArray(parsed)) {
      return parsed as Record<string, unknown>;
    }
  } catch {
    // Invalid JSON is handled by the raw editor validator.
  }
  return {};
}

function dedupeMaskFields(fields: MaskField[]): MaskField[] {
  const seen = new Set<string>();
  return fields.filter((field) => {
    if (seen.has(field.path)) {
      return false;
    }
    seen.add(field.path);
    return true;
  });
}

function clonePlainRecord(value: Record<string, unknown>): Record<string, unknown> {
  if (typeof structuredClone === "function") {
    return structuredClone(value) as Record<string, unknown>;
  }
  return JSON.parse(JSON.stringify(value)) as Record<string, unknown>;
}

function collectPresentSchemaPaths(
  value: unknown,
  node: SchemaNode,
  currentPath = "",
): string[] {
  if (value === undefined || value === null || !node) {
    return [];
  }

  if (isPresenceObjectNode(node)) {
    return currentPath ? [currentPath] : [];
  }

  if (node.type === "array") {
    return Array.isArray(value) && currentPath ? [currentPath] : [];
  }

  if (
    node.type === "object" &&
    node.additionalProperties &&
    typeof node.additionalProperties === "object" &&
    value &&
    typeof value === "object" &&
    !Array.isArray(value)
  ) {
    return currentPath ? [currentPath] : [];
  }

  if (
    node.enum ||
    node.type === "string" ||
    node.type === "integer" ||
    node.type === "number" ||
    node.type === "boolean"
  ) {
    return currentPath ? [currentPath] : [];
  }

  if (
    (node.type === "object" || node.properties) &&
    value &&
    typeof value === "object" &&
    !Array.isArray(value)
  ) {
    return Object.entries(value as Record<string, unknown>).flatMap(
      ([key, child]) => {
        const childNode = node.properties?.[key];
        if (!childNode) {
          return [];
        }
        const childPath = currentPath ? `${currentPath}.${key}` : key;
        return collectPresentSchemaPaths(child, childNode, childPath);
      },
    );
  }

  return [];
}

function humanizeFieldSegment(segment: string): string {
  const normalized = segment.replace(/_/g, " ");
  const specialLabels: Record<string, string> = {
    acpi: "ACPI",
    apic: "APIC",
    cpu: "CPU",
    gpu: "GPU",
    gpus: "GPUs",
    hpet: "HPET",
    io: "I/O",
    numa: "NUMA",
    pit: "PIT",
    rng: "RNG",
    rtc: "RTC",
    sriov: "SR-IOV",
    utc: "UTC",
    vendorid: "Vendor ID",
    vpindex: "VPIndex",
    vsock: "VSOCK",
  };
  const key = normalized.toLowerCase();
  if (specialLabels[key]) {
    return specialLabels[key];
  }
  return normalized
    .replace(/([a-z0-9])([A-Z])/g, "$1 $2")
    .replace(/\b\w/g, (char) => char.toUpperCase());
}

function buildDetectedFieldLabel(path: string): string {
  const segments = path.split(".").slice(-2);
  const compactSegments =
    segments.length > 1 && segments[0] === segments[1]
      ? [segments[1]]
      : segments;
  return compactSegments.map(humanizeFieldSegment).join(" ");
}

export function deriveEnumOptionsFromDescription(description?: string): string[] {
  if (typeof description !== "string") {
    return [];
  }

  const normalized = description.trim();
  const lower = normalized.toLowerCase();
  if (
    !lower.includes("one of ") &&
    !lower.includes("possible options are")
  ) {
    return [];
  }

  const matches = Array.from(normalized.matchAll(/"([^"]+)"/g)).map(
    (match) => match[1],
  );
  const unique = Array.from(new Set(matches.filter(Boolean)));
  return unique.length >= 2 ? unique : [];
}

export function buildRecognizedMaskFields(
  schema: SchemaNode,
  mask: SchemaMask,
  committedValue: Record<string, unknown>,
  recognizedExcludedPaths?: string[],
): MaskField[] {
  const maskedPaths = new Set(
    [
      ...(mask.quick_fields ?? []),
      ...(mask.advanced_fields ?? []),
      ...(mask.professional_fields ?? []),
    ].map(
      (field) => field.path,
    ),
  );
  const excludedPaths = new Set(recognizedExcludedPaths ?? []);

  return dedupeMaskFields(
    collectPresentSchemaPaths(committedValue, schema)
      .filter((path) => !maskedPaths.has(path) && !excludedPaths.has(path))
      .map((path) => ({
        path,
        display_name: buildDetectedFieldLabel(path),
      })),
  );
}

// ─── Field Renderers ──────────────────────────────────────────────────────────

interface DynamicFieldGroupProps {
  node: SchemaNode;
  namePath: (string | number)[];
  label: React.ReactNode;
  labelText: string;
  helpText?: string;
  placeholder?: string;
  disabled?: boolean;
}

const isPresenceObjectNode = (node: SchemaNode): boolean =>
  node.type === "object" &&
  !node.items &&
  !node.enum &&
  !node.additionalProperties &&
  Object.keys(node.properties ?? {}).length === 0;

const getScalarMapValueNode = (node: SchemaNode): SchemaNode | null => {
  if (node.type !== "object") {
    return null;
  }
  if (node.properties && Object.keys(node.properties).length > 0) {
    return null;
  }
  if (!node.additionalProperties || typeof node.additionalProperties !== "object") {
    return null;
  }
  const valueNode = node.additionalProperties as SchemaNode;
  if (valueNode.enum && Array.isArray(valueNode.enum)) {
    return valueNode;
  }
  if (
    valueNode.type === "string" ||
    valueNode.type === "integer" ||
    valueNode.type === "number" ||
    valueNode.type === "boolean"
  ) {
    return valueNode;
  }
  return null;
};

interface ScalarMapEditorProps {
  value?: Record<string, unknown>;
  onChange?: (value?: Record<string, unknown>) => void;
  valueNode: SchemaNode;
  disabled?: boolean;
  testIdBase: string;
  valuePlaceholder?: string;
}

interface ScalarMapRow {
  id: number;
  keyText: string;
  value: string | number | boolean | undefined;
}

interface ScalarMapState {
  rows: ScalarMapRow[];
  nextID: number;
}

interface RawEditorDraft {
  text: string;
  sourceValue: string;
}

function coerceMapValue(
  rawValue: unknown,
  valueNode: SchemaNode,
): string | number | boolean | undefined {
  if (valueNode.enum && Array.isArray(valueNode.enum)) {
    return rawValue === undefined || rawValue === null ? undefined : String(rawValue);
  }
  if (valueNode.type === "integer" || valueNode.type === "number") {
    return typeof rawValue === "number"
      ? rawValue
      : rawValue === undefined || rawValue === null || rawValue === ""
        ? undefined
        : Number(rawValue);
  }
  if (valueNode.type === "boolean") {
    return typeof rawValue === "boolean" ? rawValue : Boolean(rawValue);
  }
  return rawValue === undefined || rawValue === null ? "" : String(rawValue);
}

export function buildScalarMapState(
  value: Record<string, unknown> | undefined,
  valueNode: SchemaNode,
  startID = 0,
): ScalarMapState {
  let nextID = startID;
  const rows = Object.entries(value ?? {}).map(([key, rawValue]) => ({
    id: nextID++,
    keyText: key,
    value: coerceMapValue(rawValue, valueNode),
  }));
  return { rows, nextID };
}

export function normalizeMapRows(
  rows: ScalarMapRow[],
  valueNode: SchemaNode,
): Record<string, unknown> | undefined {
  const result: Record<string, unknown> = {};

  for (const row of rows) {
    const key = row.keyText.trim();
    if (!key) {
      continue;
    }

    if (valueNode.type === "integer" || valueNode.type === "number") {
      if (typeof row.value !== "number" || Number.isNaN(row.value)) {
        continue;
      }
      result[key] = row.value;
      continue;
    }

    if (valueNode.type === "boolean") {
      if (typeof row.value !== "boolean") {
        continue;
      }
      result[key] = row.value;
      continue;
    }

    const valueText = String(row.value ?? "").trim();
    if (!valueText) {
      continue;
    }
    result[key] = valueText;
  }

  return Object.keys(result).length > 0 ? result : undefined;
}

export function validateRawEditorText(
  rawText: string,
  t: ReturnType<typeof useTranslation>["t"],
): string | null {
  if (!rawText.trim()) {
    return null;
  }

  try {
    const parsed = JSON.parse(rawText) as unknown;
    if (parsed && typeof parsed === "object" && !Array.isArray(parsed)) {
      return null;
    }
    return t(
      "dynamic_form.raw_json_object_only",
      'Spec JSON must be an object rooted at {"spec": ...}.',
    );
  } catch {
    return t(
      "dynamic_form.raw_json_invalid",
      "JSON is invalid. Fix the syntax before applying advanced changes.",
    );
  }
}

const ScalarMapEditor: React.FC<ScalarMapEditorProps> = ({
  value,
  onChange,
  valueNode,
  disabled,
  testIdBase,
  valuePlaceholder,
}) => {
  const { t } = useTranslation(["admin", "common"]);
  const committedState = useMemo(
    () => buildScalarMapState(value, valueNode),
    [value, valueNode],
  );
  const committedKey = useMemo(
    () => JSON.stringify(value ?? {}),
    [value],
  );
  const [mapDraft, setMapDraft] = useState<{
    sourceKey: string;
    rows: ScalarMapRow[];
    nextID: number;
  } | null>(null);
  const activeDraft =
    mapDraft?.sourceKey === committedKey ? mapDraft : null;
  const rows = activeDraft?.rows ?? committedState.rows;
  const nextID = activeDraft?.nextID ?? committedState.nextID;

  const commitRows = useCallback(
    (nextRows: ScalarMapRow[], nextDraftID = nextID) => {
      setMapDraft({
        sourceKey: committedKey,
        rows: nextRows,
        nextID: nextDraftID,
      });
      onChange?.(normalizeMapRows(nextRows, valueNode));
    },
    [committedKey, nextID, onChange, valueNode],
  );

  const updateRow = useCallback(
    (rowID: number, patch: Partial<ScalarMapRow>) => {
      commitRows(
        rows.map((row) => (row.id === rowID ? { ...row, ...patch } : row)),
      );
    },
    [commitRows, rows],
  );

  return (
    <Space direction="vertical" size={8} style={{ width: "100%" }}>
      {rows.map((row, index) => (
        <Space
          key={row.id}
          align="start"
          style={{ display: "flex", width: "100%" }}
        >
          <Input
            value={row.keyText}
            disabled={disabled}
            placeholder={t("dynamic_form.map_key_placeholder", "Key")}
            data-testid={`${testIdBase}-key-${index}`}
            onChange={(event) =>
              updateRow(row.id, { keyText: event.target.value })
            }
          />
          {valueNode.enum && Array.isArray(valueNode.enum) ? (
            <Select
              allowClear
              value={typeof row.value === "string" ? row.value : undefined}
              disabled={disabled}
              placeholder={valuePlaceholder ?? t("dynamic_form.map_value_placeholder", "Value")}
              data-testid={`${testIdBase}-value-${index}`}
              style={{ minWidth: 180 }}
              onChange={(nextValue) => updateRow(row.id, { value: nextValue })}
              options={valueNode.enum.map((option) => ({
                label: String(option),
                value: String(option),
              }))}
            />
          ) : valueNode.type === "integer" || valueNode.type === "number" ? (
            <InputNumber
              value={typeof row.value === "number" ? row.value : undefined}
              disabled={disabled}
              placeholder={valuePlaceholder}
              data-testid={`${testIdBase}-value-${index}`}
              style={{ width: 180 }}
              onChange={(nextValue) =>
                updateRow(row.id, {
                  value: typeof nextValue === "number" ? nextValue : undefined,
                })
              }
            />
          ) : valueNode.type === "boolean" ? (
            <Checkbox
              checked={Boolean(row.value)}
              disabled={disabled}
              data-testid={`${testIdBase}-value-${index}`}
              onChange={(event) =>
                updateRow(row.id, { value: Boolean(event.target.checked) })
              }
            >
              {t("dynamic_form.map_boolean_value", "Enabled")}
            </Checkbox>
          ) : (
            <Input
              value={typeof row.value === "string" ? row.value : ""}
              disabled={disabled}
              placeholder={valuePlaceholder ?? t("dynamic_form.map_value_placeholder", "Value")}
              data-testid={`${testIdBase}-value-${index}`}
              onChange={(event) =>
                updateRow(row.id, { value: event.target.value })
              }
            />
          )}
          <Button
            type="text"
            danger
            icon={<DeleteOutlined />}
            disabled={disabled}
            onClick={() => commitRows(rows.filter((item) => item.id !== row.id))}
          />
        </Space>
      ))}
      <Button
        type="dashed"
        onClick={() => {
          const nextRow = {
            id: nextID,
            keyText: "",
            value: "" as const,
          };
          commitRows([...rows, nextRow], nextID + 1);
        }}
        block
        icon={<PlusOutlined />}
        disabled={disabled}
      >
        {t("dynamic_form.add_map_item", "Add entry")}
      </Button>
    </Space>
  );
};

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
  labelText,
  helpText,
  placeholder,
  disabled,
}) => {
  const { t } = useTranslation(["admin", "common"]);

  if (!node) return null;

  const fieldTestId = `dynamic-form-${namePath.join(".")}`;

  const renderFieldHeader = () => (
    <div className="dynamic-form-field-header">
      <div className="dynamic-form-field-title">
        {typeof label === "string" ? <Text strong>{label}</Text> : label}
      </div>
      {helpText ? (
        <Text type="secondary" className="dynamic-form-field-help">
          {helpText}
        </Text>
      ) : null}
    </div>
  );

  const renderFieldShell = (
    formItemProps: Omit<React.ComponentProps<typeof Form.Item>, "children">,
    control: React.ReactNode,
    className?: string,
  ) => (
    <div className={`dynamic-form-field-shell${className ? ` ${className}` : ""}`}>
      {renderFieldHeader()}
      <Form.Item {...formItemProps} style={{ marginBottom: 0 }}>
        {control}
      </Form.Item>
    </div>
  );

  const resolvedSelectPlaceholder = placeholder ?? "Select an option";

  // array → dynamic add/remove table
  if (node.type === "array" && node.items?.properties) {
    const itemKeys = Object.keys(node.items.properties);
    return (
      <Card
        size="small"
        title={
          <Space size={6}>
            <Text strong>{label}</Text>
            {helpText && (
              <Tooltip title={helpText} trigger={["hover", "click"]}>
                <QuestionCircleOutlined style={{ color: "rgba(0,0,0,0.45)" }} />
              </Tooltip>
            )}
          </Space>
        }
        style={{ marginBottom: 16 }}
      >
        <Form.List name={namePath}>
          {(fields, { add, remove }) => (
            <div
              style={{ display: "flex", flexDirection: "column", gap: "8px" }}
            >
              {fields.map((field) => (
                <Space
                  key={field.key}
                  style={{
                    display: "flex",
                    marginBottom: 8,
                    alignItems: "flex-start",
                  }}
                  align="baseline"
                >
                  <div
                    style={{
                      flex: 1,
                      padding: 8,
                      border: "1px solid #f0f0f0",
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
                          labelText={itemKey}
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
                  {t("dynamic_form.add_item", { label: labelText })}
                </Button>
              )}
            </div>
          )}
        </Form.List>
      </Card>
    );
  }

  const scalarMapValueNode = getScalarMapValueNode(node);
  if (scalarMapValueNode) {
    return renderFieldShell(
      {
        name: namePath,
      },
      (
        <ScalarMapEditor
          valueNode={scalarMapValueNode}
          disabled={disabled}
          testIdBase={fieldTestId}
          valuePlaceholder={placeholder}
        />
      ),
    );
  }

  if (isPresenceObjectNode(node)) {
    return renderFieldShell(
      {
        name: namePath,
        getValueProps: (fieldValue?: Record<string, unknown>) => ({
          checked: fieldValue !== undefined,
        }),
        getValueFromEvent: (event: { target?: { checked?: boolean } }) =>
          event?.target?.checked ? {} : undefined
        ,
        valuePropName: "checked",
      },
      (
        <Checkbox
          disabled={disabled}
          data-testid={fieldTestId}
          aria-label={labelText}
        />
      ),
      "dynamic-form-field-shell-boolean",
    );
  }

  // enum → dropdown (options from schema, not developer-defined — Stage 1 constraint)
  const descriptionEnumOptions =
    !node.enum || !Array.isArray(node.enum)
      ? deriveEnumOptionsFromDescription(
          typeof node.description === "string" ? node.description : undefined,
        )
      : [];
  const selectOptions =
    node.enum && Array.isArray(node.enum)
      ? node.enum.map((option) => String(option))
      : descriptionEnumOptions;

  if (selectOptions.length > 0) {
    return renderFieldShell(
      {
        name: namePath,
      },
      (
        <Select
          disabled={disabled}
          data-testid={fieldTestId}
          placeholder={resolvedSelectPlaceholder}
          style={{ width: "100%" }}
        >
          {selectOptions.map((opt) => (
            <Select.Option key={opt} value={opt}>
              {opt}
            </Select.Option>
          ))}
        </Select>
      ),
    );
  }

  // integer/number → numeric input
  // boolean → checkbox  (master-flow.md:167: "boolean → checkbox")
  // string (default) → text input
  return renderFieldShell(
    {
      name: namePath,
      valuePropName: node.type === "boolean" ? "checked" : "value",
    },
    node.type === "integer" || node.type === "number" ? (
        <InputNumber
          style={{ width: "100%" }}
          disabled={disabled}
          data-testid={fieldTestId}
          placeholder={placeholder}
        />
      ) : node.type === "boolean" ? (
        // master-flow.md Stage 1: "boolean → checkbox" (not Switch).
        // Checkbox communicates a binary on/off choice matching spec field semantics.
        <Checkbox
          disabled={disabled}
          data-testid={fieldTestId}
          aria-label={labelText}
        />
      ) : (
        <Input
          disabled={disabled}
          data-testid={fieldTestId}
          placeholder={placeholder}
        />
      ),
    node.type === "boolean" ? "dynamic-form-field-shell-boolean" : undefined,
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
export const DynamicSchemaForm = React.forwardRef<
  DynamicSchemaFormHandle,
  DynamicSchemaFormProps
>(function DynamicSchemaForm({
  value,
  onChange,
  schema,
  mask,
  disabled,
  recognizedExcludedPaths,
  schemaHelpScope = "instanceSize",
}, ref) {
  const { t } = useTranslation(["admin", "schema", "common"]);
  const outerForm = Form.useFormInstance();
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => {
    return () => {
      if (debounceRef.current) {
        clearTimeout(debounceRef.current);
      }
    };
  }, []);
  const appliedFieldPathsRef = useRef<string[]>([]);
  const [rawEditorDraft, setRawEditorDraft] = useState<RawEditorDraft | null>(
    null,
  );
  const [recognizedFields, setRecognizedFields] = useState<MaskField[]>([]);
  const committedValue = useMemo(() => parseCommittedValue(value), [value]);
  const committedRawEditorText = value ?? "{}";
  const activeRawEditorDraft =
    rawEditorDraft?.sourceValue === committedRawEditorText
      ? rawEditorDraft
      : null;
  const rawEditorText = activeRawEditorDraft?.text ?? committedRawEditorText;
  const rawEditorError = useMemo(
    () => validateRawEditorText(rawEditorText, t),
    [rawEditorText, t],
  );
  const quickFields = useMemo(
    () => dedupeMaskFields(mask?.quick_fields ?? []),
    [mask?.quick_fields],
  );
  const advancedFields = useMemo(
    () => dedupeMaskFields(mask?.advanced_fields ?? []),
    [mask?.advanced_fields],
  );
  const professionalFields = useMemo(
    () => dedupeMaskFields(mask?.professional_fields ?? []),
    [mask?.professional_fields],
  );

  const managedFields = useMemo(
    () =>
      dedupeMaskFields([
        ...quickFields,
        ...advancedFields,
        ...professionalFields,
        ...recognizedFields,
      ]),
    [advancedFields, professionalFields, quickFields, recognizedFields],
  );

  const applyParsedValueToForm = useCallback(
    (parsed: Record<string, unknown>, nextManagedFields: MaskField[]) => {
      if (!outerForm) {
        return;
      }

      const clearRoots = Array.from(
        new Set([
          ...appliedFieldPathsRef.current,
          ...nextManagedFields.map((field) => field.path),
        ].map((path) => path.split(".")[0]).filter(Boolean)),
      );

      if (clearRoots.length > 0) {
        outerForm.setFieldsValue(
          Object.fromEntries(clearRoots.map((root) => [root, undefined])),
        );
      }

      outerForm.setFieldsValue(clonePlainRecord(parsed));
      appliedFieldPathsRef.current = nextManagedFields.map((field) => field.path);
    },
    [outerForm],
  );

  const serializeManagedSpecOverrides = useCallback(() => {
    if (!outerForm || !schema) {
      return "{}";
    }

    const allValues = outerForm.getFieldsValue(true) as Record<string, unknown>;
    const specKeys = Object.keys(schema.properties ?? {});
    const nestedSpecValues: Record<string, unknown> = {};
    for (const k of specKeys) {
      if (k in allValues) {
        nestedSpecValues[k] = allValues[k];
      }
    }
    const presenceObjectPaths = new Set(
      managedFields
        .filter((field) => {
          const node = resolveSchemaNode(schema, field.path);
          return node ? isPresenceObjectNode(node) : false;
        })
        .map((field) => field.path),
    );
    const prunedSpecOverrides = pruneSpecTree(
      nestedSpecValues,
      presenceObjectPaths,
      "",
      schema,
    );
    return JSON.stringify(prunedSpecOverrides ?? {}, null, 2);
  }, [managedFields, outerForm, schema]);

  // Initialise outer form fields from the JSON string value on first render
  // or when the modal opens with a different record.
  // NOTE: hooks must be called unconditionally (React rules-of-hooks).
  // The guard check for !schema || !mask happens after all hooks.
  useEffect(() => {
    if (!outerForm || !schema || !mask) return;
    applyParsedValueToForm(committedValue, managedFields);
    if (!onChange) {
      return;
    }
    const nextSerializedValue = serializeManagedSpecOverrides();
    if (nextSerializedValue !== value) {
      onChange(nextSerializedValue);
    }
  }, [
    applyParsedValueToForm,
    committedValue,
    managedFields,
    mask,
    onChange,
    outerForm,
    schema,
    serializeManagedSpecOverrides,
    value,
  ]);

  // Sync outer Form values back to the JSON string.
  // Called imperatively via ref.sync() — invoked by the parent Form's
  // onValuesChange callback, NOT during render (React best practice).
  //
  // Antd docs: shouldUpdate is for conditional rendering only.
  // Side effects (data sync) belong in event callbacks (onValuesChange).
  //
  const syncToParent = useCallback(() => {
    if (!outerForm || !onChange || !schema) return;
    if (debounceRef.current) clearTimeout(debounceRef.current);
    debounceRef.current = setTimeout(() => {
      const nextSerializedValue = serializeManagedSpecOverrides();
      if (nextSerializedValue === value) {
        return;
      }
      onChange(nextSerializedValue);
    }, 300);
  }, [onChange, outerForm, schema, serializeManagedSpecOverrides, value]);

  // Expose sync() imperatively so the parent Form's onValuesChange can call it.
  // This is the Ant Design recommended pattern: side effects in event handlers,
  // not in render. ref.sync() is called from outside; no render-phase side effects.
  useImperativeHandle(ref, () => ({ sync: syncToParent }), [syncToParent]);

  const handleRawEditorChange = (nextText: string) => {
    setRawEditorDraft({
      text: nextText,
      sourceValue: committedRawEditorText,
    });

    if (!outerForm || !schema || !mask || !onChange) {
      return;
    }

    if (!nextText.trim()) {
      setRecognizedFields([]);
      applyParsedValueToForm({}, managedFields);
      onChange("{}");
      return;
    }

    try {
      const parsed = JSON.parse(nextText) as unknown;
      if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) {
        return;
      }

      const nextRecognizedFields = buildRecognizedMaskFields(
        schema,
        mask,
        parsed as Record<string, unknown>,
        recognizedExcludedPaths,
      );
      const nextManagedFields = dedupeMaskFields([
        ...quickFields,
        ...advancedFields,
        ...professionalFields,
        ...nextRecognizedFields,
      ]);

      setRecognizedFields(nextRecognizedFields);
      applyParsedValueToForm(
        parsed as Record<string, unknown>,
        nextManagedFields,
      );
      onChange(nextText);
    } catch {
      return;
    }
  };

  // Guard: render a hard error if required props are missing.
  // This is a developer error — schema and mask must always be provided.
  // IMPORTANT: placed AFTER all hooks to comply with react-hooks/rules-of-hooks.
  if (!schema || !mask) {
    return (
      <Alert
        type="error"
        banner
        message={t(
          "dynamic_form.schema_unavailable",
          "Dynamic form configuration is unavailable.",
        )}
        description={t(
          "dynamic_form.schema_unavailable_help",
          "The schema or field mask for this form is missing. Refresh the page or contact an administrator if the problem persists.",
        )}
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
            message={t("dynamic_form.invalid_path", {
              path: field.path,
              defaultValue: `Field path not found in schema: ${field.path}`,
            })}
            style={{ marginBottom: 8 }}
          />
        );
      }
      const namePath = field.path.split(".");
      const label = field.display_name ?? field.path;
      const versionBadge = field.introduced_in ? `v${field.introduced_in}+` : undefined;
      const schemaDescription =
        typeof node.description === "string" ? node.description : undefined;
      const helpText = field.help_key
        ? t(field.help_key, field.help_text ?? "")
        : (field.help_text ??
          resolveSchemaHelpText(
            t,
            schemaHelpScope,
            field.path,
            schemaDescription,
          ));
      const placeholder = field.placeholder_key
        ? t(field.placeholder_key, field.placeholder ?? "")
        : field.placeholder;
      return (
        <DynamicFieldGroup
          key={field.path}
          node={node}
          namePath={namePath}
          label={
            <Space size={6} wrap>
              <Text strong>{label}</Text>
              {versionBadge ? (
                <Tag color="blue" className="dynamic-form-version-tag">
                  {versionBadge}
                </Tag>
              ) : null}
            </Space>
          }
          labelText={label}
          helpText={helpText}
          placeholder={placeholder}
          disabled={disabled}
        />
      );
    });
  };

  const collapseItems: NonNullable<CollapseProps["items"]> = [];
  if (advancedFields.length > 0) {
    collapseItems.push({
      key: "advanced",
      label: t("dynamic_form.advanced_settings", "Advanced Features"),
      children: renderMaskElements(advancedFields),
    });
  }
  if (professionalFields.length > 0) {
    collapseItems.push({
      key: "professional",
      label: t("dynamic_form.professional_features", "Professional Features"),
      children: renderMaskElements(professionalFields),
    });
  }
  collapseItems.push({
    key: "json-recognition",
    label: t("dynamic_form.supplemental_fields", "JSON Recognition"),
    children: (
      <Space direction="vertical" size={8} style={{ width: "100%" }}>
        <Text type="secondary">
          {t(
            "dynamic_form.supplemental_fields_help",
            "Fields outside the mask can be recognized here after you provide custom JSON in the raw editor.",
          )}
        </Text>
        {recognizedFields.length > 0 ? (
          renderMaskElements(recognizedFields)
        ) : (
          <Alert
            type="info"
            showIcon
            message={t(
              "dynamic_form.supplemental_fields_empty",
              "No recognized fields yet. This section is used only for custom JSON recognition.",
            )}
          />
        )}
      </Space>
    ),
  });
  collapseItems.push({
    key: "raw-json",
    label: t("dynamic_form.raw_json", "Raw KubeVirt JSON"),
    children: (
      <Space direction="vertical" size={8} style={{ width: "100%" }}>
        <Text type="secondary">
          {t(
            "dynamic_form.raw_json_help",
            "This editor covers raw spec overrides only. When you provide custom JSON here, fields outside the mask can be recognized into the JSON Recognition section.",
          )}
        </Text>
        {rawEditorError ? (
          <Alert type="warning" showIcon message={rawEditorError} />
        ) : null}
        <Input.TextArea
          value={rawEditorText}
          onChange={(event) => handleRawEditorChange(event.target.value)}
          autoSize={{ minRows: 8, maxRows: 18 }}
          disabled={disabled}
          data-testid="dynamic-form-raw-json"
          style={{ fontFamily: "monospace", fontSize: 14 }}
          placeholder={t(
            "dynamic_form.raw_json_placeholder",
            '{\n  "spec": {\n    "template": {\n      "spec": {}\n    }\n  }\n}',
          )}
        />
      </Space>
    ),
  });

  return (
    <Card
      size="small"
      title={t("templates.spec")}
      styles={{ body: { padding: "16px 8px" } }}
    >
      {/*
       * No shouldUpdate side-effect hack here.
       * sync() is called by the parent Form's onValuesChange via ref.
       * This follows Ant Design's recommended pattern:
       *   - shouldUpdate: conditional rendering only
       *   - onValuesChange: data synchronization (side effects)
       */}
      {quickFields.length > 0 && (
        <div className="quick-fields-section">
          {renderMaskElements(quickFields)}
        </div>
      )}

      {collapseItems.length > 0 && (
        <Collapse
          ghost
          items={collapseItems}
          style={{ marginTop: 16 }}
        />
      )}
    </Card>
  );
});

DynamicSchemaForm.displayName = "DynamicSchemaForm";
