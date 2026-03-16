import type { TFunction } from "i18next";

export const SCHEMA_HELP_NAMESPACE = "schema";

export type SchemaHelpScope = "instanceSize";

const MISSING_TRANSLATION_SENTINEL = "__schema_help_missing__";

function sanitizeSchemaPath(path: string): string {
  return path.replace(/[^A-Za-z0-9]+/g, "__");
}

export function buildSchemaHelpKey(
  scope: SchemaHelpScope,
  path: string,
): string {
  return `${scope}.${sanitizeSchemaPath(path)}`;
}

export function resolveSchemaHelpText(
  t: TFunction,
  scope: SchemaHelpScope,
  path: string,
  fallbackText?: string,
): string | undefined {
  const translated = t(buildSchemaHelpKey(scope, path), {
    ns: SCHEMA_HELP_NAMESPACE,
    defaultValue: MISSING_TRANSLATION_SENTINEL,
  });

  if (
    typeof translated === "string" &&
    translated !== MISSING_TRANSLATION_SENTINEL
  ) {
    return translated;
  }

  return fallbackText;
}
