"use client";

import { Form, Input, Select, Switch, Divider, Alert } from "antd";
import type { FormInstance } from "antd";
import { useMemo } from "react";
import { useTranslation } from "react-i18next";

/**
 * JSON Schema property definition from config_schema.
 */
interface SchemaProperty {
  type?: string;
  format?: string;
  title?: string;
  description?: string;
  default?: unknown;
  items?: { type?: string };
  additionalProperties?: unknown;
  enum?: string[];
  ["x-enum-labels"]?: Record<string, string>;
}

/**
 * JSON Schema root definition from config_schema.
 */
interface ConfigSchema {
  type?: string;
  properties?: Record<string, SchemaProperty>;
  required?: string[];
  additionalProperties?: unknown;
}

function objectFieldInitialValue(value: unknown): string | undefined {
  if (value && typeof value === "object" && !Array.isArray(value)) {
    return JSON.stringify(value, null, 2);
  }
  if (typeof value === "string") {
    return value;
  }
  return undefined;
}

interface SchemaConfigFormProps {
  /** JSON Schema from AuthProviderType.config_schema */
  schema: ConfigSchema | null | undefined;
  /** Ant Design form instance to bind fields to */
  form: FormInstance;
  /** Field name prefix for nested form values (default: 'config') */
  namePrefix?: string;
  /** Whether to show a fallback JSON textarea when schema is empty */
  showJsonFallback?: boolean;
  /** Optional translation namespace prefix for provider-specific schema labels */
  schemaNamespace?: string;
  /** Whether to apply schema defaults when the form does not yet have a value */
  applySchemaDefaults?: boolean;
}

function schemaTranslationCandidates(
  schemaNamespace: string | undefined,
  fieldKey: string,
  suffix: "label" | "description" | "placeholder",
) {
  const candidates: string[] = [];
  if (schemaNamespace) {
    candidates.push(`${schemaNamespace}.${fieldKey}.${suffix}`);
  }
  candidates.push(`authProviders.schema.shared.${fieldKey}.${suffix}`);
  return candidates;
}

function resolveSchemaFieldText(
  t: (key: string, options?: Record<string, unknown>) => string,
  schemaNamespace: string | undefined,
  fieldKey: string,
  suffix: "label" | "description" | "placeholder",
  fallback: string,
) {
  for (const key of schemaTranslationCandidates(schemaNamespace, fieldKey, suffix)) {
    const translated = t(key, { defaultValue: key });
    if (translated !== key) {
      return translated;
    }
  }
  return fallback;
}

function resolveSchemaEnumLabel(
  t: (key: string, options?: Record<string, unknown>) => string,
  schemaNamespace: string | undefined,
  fieldKey: string,
  value: string,
  property?: SchemaProperty,
) {
  const explicitLabel = property?.["x-enum-labels"]?.[value];
  if (explicitLabel && explicitLabel.trim() !== "") {
    return explicitLabel;
  }
  const candidates: string[] = [];
  if (schemaNamespace) {
    candidates.push(`${schemaNamespace}.${fieldKey}.options.${value}`);
  }
  candidates.push(`authProviders.schema.shared.${fieldKey}.options.${value}`);
  for (const key of candidates) {
    const translated = t(key, { defaultValue: key });
    if (translated !== key) {
      return translated;
    }
  }
  return value;
}

/**
 * SchemaConfigForm renders form fields dynamically based on a JSON Schema
 * returned by the auth provider type discovery API.
 *
 * This replaces the raw JSON TextArea approach with guided, schema-driven
 * form fields per ADR-0035 best practices.
 */
export function SchemaConfigForm({
  schema,
  namePrefix = "config",
  showJsonFallback = false,
  schemaNamespace,
  applySchemaDefaults = false,
}: SchemaConfigFormProps) {
  const { t } = useTranslation(["admin", "common"]);

  const fields = useMemo(() => {
    if (!schema?.properties) return [];

    const required = new Set(schema.required ?? []);
    // Filter out internal fields (sample_users is for backend use).
    const skipFields = new Set(["sample_users"]);

    return Object.entries(schema.properties)
      .filter(([key]) => !skipFields.has(key))
      .map(([key, def]) => ({
        key,
        ...def,
        isRequired: required.has(key),
      }));
  }, [schema]);
  // No schema or empty properties — show JSON fallback
  if (fields.length === 0) {
    if (!showJsonFallback) return null;
    return (
      <Form.Item name={`${namePrefix}_text`} label={t("authProviders.config")}>
        <Input.TextArea
          rows={8}
          style={{ fontFamily: "monospace", fontSize: 13 }}
          placeholder="{}"
        />
      </Form.Item>
    );
  }

  const renderField = (field: (typeof fields)[number]) => {
    const fieldName = [namePrefix, field.key];
    const label = resolveSchemaFieldText(
      t,
      schemaNamespace,
      field.key,
      "label",
      field.title || field.key,
    );
    const description = resolveSchemaFieldText(
      t,
      schemaNamespace,
      field.key,
      "description",
      field.description || "",
    );
    const placeholder = resolveSchemaFieldText(
      t,
      schemaNamespace,
      field.key,
      "placeholder",
      description,
    );
    const rules = field.isRequired
      ? [
          {
            required: true,
            message: t("authProviders.validation.required", {
              field: label,
              defaultValue: `${label} is required`,
            }),
          },
        ]
      : [];

    if (field.format === "password") {
      return (
        <Form.Item
          key={field.key}
          name={fieldName}
          label={label}
          tooltip={description || undefined}
          rules={rules}
        >
          <Input.Password
            placeholder={placeholder}
            autoComplete="new-password"
          />
        </Form.Item>
      );
    }

    if (field.type === "boolean") {
      return (
        <Form.Item
          key={field.key}
          name={fieldName}
          label={label}
          tooltip={description || undefined}
          valuePropName="checked"
          initialValue={applySchemaDefaults ? field.default : undefined}
        >
          <Switch />
        </Form.Item>
      );
    }

    if (field.type === "array" && field.items?.type === "string") {
      return (
        <Form.Item
          key={field.key}
          name={fieldName}
          label={label}
          tooltip={description || undefined}
          rules={rules}
          initialValue={applySchemaDefaults ? field.default : undefined}
        >
          <Select
            mode="tags"
            style={{ width: "100%" }}
            placeholder={placeholder}
            tokenSeparators={[","]}
          />
        </Form.Item>
      );
    }

    if (field.enum && field.enum.length > 0) {
      return (
        <Form.Item
          key={field.key}
          name={fieldName}
          label={label}
          tooltip={description || undefined}
          rules={rules}
          initialValue={applySchemaDefaults ? field.default : undefined}
        >
          <Select
            options={field.enum.map((v) => ({
              value: v,
              label: resolveSchemaEnumLabel(t, schemaNamespace, field.key, v, field),
            }))}
          />
        </Form.Item>
      );
    }

    if (field.type === "object") {
      return (
        <Form.Item
          key={field.key}
          name={fieldName}
          label={label}
          tooltip={description || undefined}
          rules={rules}
          initialValue={
            applySchemaDefaults
              ? objectFieldInitialValue(field.default)
              : undefined
          }
        >
          <Input.TextArea
            rows={4}
            style={{ fontFamily: "monospace", fontSize: 13 }}
            placeholder={placeholder || "{}"}
          />
        </Form.Item>
      );
    }

    return (
      <Form.Item
        key={field.key}
        name={fieldName}
        label={label}
        tooltip={description || undefined}
        rules={rules}
        initialValue={applySchemaDefaults ? field.default : undefined}
      >
        <Input
          placeholder={
            field.format === "uri" ? placeholder || "https://..." : placeholder || ""
          }
        />
      </Form.Item>
    );
  };

  return (
    <div>
      {fields.map(renderField)}

      <Divider dashed style={{ margin: "12px 0" }} />
      <Alert
        type="info"
        showIcon
        message={t("authProviders.schema_driven_hint")}
        style={{ marginBottom: 12 }}
      />
    </div>
  );
}
