import fs from "node:fs";
import path from "node:path";

const rootDir = path.resolve(path.dirname(new URL(import.meta.url).pathname), "..");

const manifestPath = path.join(rootDir, "..", "internal", "pkg", "schema", "manifest.json");
const enSchemaLocalePath = path.join(rootDir, "src", "i18n", "locales", "en", "schema.json");
const zhSchemaLocalePath = path.join(rootDir, "src", "i18n", "locales", "zh-CN", "schema.json");

function readJson(filePath) {
  return JSON.parse(fs.readFileSync(filePath, "utf8"));
}

function resolveCurrentInstancesizeAssets() {
  const manifest = readJson(manifestPath);
  const entity = manifest?.entities?.instancesize;
  const currentVersion = entity?.current_version;
  const versionConfig = currentVersion ? entity?.versions?.[currentVersion] : null;
  if (!versionConfig?.schema_path || !versionConfig?.mask_path) {
    throw new Error("instancesize schema manifest is missing schema_path/mask_path");
  }
  return {
    schemaPath: path.join(rootDir, "..", "internal", "pkg", "schema", versionConfig.schema_path),
    maskPath: path.join(rootDir, "..", "internal", "pkg", "schema", versionConfig.mask_path),
  };
}

function resolveSchemaNode(root, schemaPathValue) {
  return schemaPathValue.split(".").reduce((current, part) => {
    if (!current) return null;
    if (current.properties?.[part]) return current.properties[part];
    if (current.items?.properties?.[part]) return current.items.properties[part];
    return null;
  }, root);
}

function buildSchemaHelpKey(pathValue) {
  return pathValue.replace(/[^A-Za-z0-9]+/g, "__");
}

function getNestedValue(resource, segments) {
  return segments.reduce(
    (current, segment) =>
      current && typeof current === "object" ? current[segment] : undefined,
    resource,
  );
}

const { schemaPath, maskPath } = resolveCurrentInstancesizeAssets();
const schema = readJson(schemaPath);
const mask = readJson(maskPath);
const enSchemaLocale = readJson(enSchemaLocalePath);
const zhSchemaLocale = readJson(zhSchemaLocalePath);
const ciMode = process.argv.includes("--ci");

const maskFields = [
  ...mask.quick_fields,
  ...(mask.advanced_fields ?? []),
  ...(mask.professional_fields ?? []),
];

const noDescriptionFields = maskFields
  .map((field) => {
    const node = resolveSchemaNode(schema, field.path);
    const description =
      node && typeof node.description === "string" ? node.description.trim() : "";
    const key = buildSchemaHelpKey(field.path);
    return {
      path: field.path,
      key: `instanceSize.${key}`,
      rawKey: key,
      description,
      hasDescription: Boolean(description),
    };
  })
  .filter((entry) => !entry.hasDescription);

const missingSchemaTranslationsForNoDescription = noDescriptionFields.filter(
  (entry) => {
    const enTranslation = getNestedValue(enSchemaLocale, ["instanceSize", entry.rawKey]);
    const zhTranslation = getNestedValue(zhSchemaLocale, ["instanceSize", entry.rawKey]);
    return !(
      typeof enTranslation === "string" &&
      enTranslation.trim().length > 0 &&
      typeof zhTranslation === "string" &&
      zhTranslation.trim().length > 0
    );
  },
);

const missingZhForSchemaFallback = maskFields
  .filter((field) => !field.help_key && !field.help_text)
  .map((field) => {
    const node = resolveSchemaNode(schema, field.path);
    const description =
      node && typeof node.description === "string" ? node.description.trim() : "";
    const key = buildSchemaHelpKey(field.path);
    const translated = getNestedValue(zhSchemaLocale, ["instanceSize", key]);
    return {
      path: field.path,
      key: `instanceSize.${key}`,
      description,
      translated: typeof translated === "string" && translated.trim().length > 0,
    };
  })
  .filter((entry) => entry.description && !entry.translated);

if (
  noDescriptionFields.length === 0 &&
  missingSchemaTranslationsForNoDescription.length === 0 &&
  missingZhForSchemaFallback.length === 0
) {
  console.log("schema-help audit: no missing schema-help translations.");
  process.exit(0);
}

if (noDescriptionFields.length > 0) {
  console.log(`schema-help audit: ${noDescriptionFields.length} fields without official schema description`);
  for (const entry of noDescriptionFields) {
    console.log(`- ${entry.key}`);
    console.log(`  path: ${entry.path}`);
  }
}

if (missingSchemaTranslationsForNoDescription.length > 0) {
  console.log(
    `schema-help audit: ${missingSchemaTranslationsForNoDescription.length} no-description fields missing en/zh schema translations`,
  );
  for (const entry of missingSchemaTranslationsForNoDescription) {
    console.log(`- ${entry.key}`);
    console.log(`  path: ${entry.path}`);
  }
}

if (missingZhForSchemaFallback.length > 0) {
  console.log(
    `schema-help audit: ${missingZhForSchemaFallback.length} described fields missing zh-CN schema translations`,
  );
  for (const entry of missingZhForSchemaFallback) {
    console.log(`- ${entry.key}`);
    console.log(`  path: ${entry.path}`);
    console.log(`  description: ${entry.description}`);
  }
}

if (
  ciMode &&
  (
    missingSchemaTranslationsForNoDescription.length > 0 ||
    missingZhForSchemaFallback.length > 0
  )
) {
  process.exit(1);
}
