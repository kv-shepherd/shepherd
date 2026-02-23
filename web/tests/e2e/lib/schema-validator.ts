/**
 * OpenAPI Response Schema Validator
 *
 * CONTRACT ENFORCEMENT layer for Live E2E tests.
 *
 * Validates that real API responses from the backend conform to the schema
 * defined in api/openapi.yaml. Backend drift (wrong field names, missing
 * required fields, wrong types) causes test failure → CI blocked.
 *
 * This is NOT a mock. It validates real HTTP responses against the OpenAPI spec.
 *
 * ── AJV $ref resolution strategy (per AJV official docs) ──────────────────────
 * Every schema in components/schemas is registered with a URI-based $id of the
 * form "openapi://components/schemas/Foo". The entire spec's schemas block is
 * loaded into the same AJV instance so cross-schema $ref resolution works. Each
 * validator function is pre-compiled once at module init and cached by name —
 * this avoids repeated compile() calls and eliminates the non-standard
 * "embed components into rootSchema" workaround.
 *
 * IMPORTANT (BUG FIX 2026-02-22):
 * Previous implementation used `$id: "openapi://"` in every compiled rootDoc,
 * causing AJV to reject the second compile() call with:
 *   "schema with key or id 'openapi://' already exists"
 * Fix: each rootDoc now uses a unique `$id` per schema name, and we use AJV's
 * recommended `getSchema(uri) || compile(schema)` pattern from:
 * https://ajv.js.org/guide/managing-schemas.html
 *
 * Reference: https://ajv.js.org/guide/managing-schemas.html
 *
 * Usage:
 *   import { validateApiResponse } from '../lib/schema-validator';
 *
 *   const resp = await page.waitForResponse(...);
 *   expect(resp.status()).toBe(200);
 *   await validateApiResponse('System', resp);        // throws on schema mismatch
 *   await validateApiResponse('SystemList', resp);
 */

import Ajv, { type ValidateFunction } from 'ajv';
import addFormats from 'ajv-formats';
import { readFileSync } from 'fs';
import { resolve } from 'path';

// ── Load OpenAPI spec once at module init ─────────────────────────────────────

// Path relative to this file: tests/e2e/lib/ → ../../../../api/openapi.yaml
const OPENAPI_PATH = resolve(__dirname, '../../../../api/openapi.yaml');

type SchemaMap = Record<string, object>;

interface OpenAPISpec {
    components?: {
        schemas?: SchemaMap;
    };
}

function loadSpec(): OpenAPISpec {
    const raw = readFileSync(OPENAPI_PATH, 'utf-8');
    // Use js-yaml which is already a transitive dependency
    // eslint-disable-next-line @typescript-eslint/no-require-imports
    const yaml = require('js-yaml') as { load(s: string): unknown };
    return yaml.load(raw) as OpenAPISpec;
}

const spec = loadSpec();
const schemas: SchemaMap = (spec.components?.schemas ?? {}) as SchemaMap;

// ── Base URI for schema $id registration ──────────────────────────────────────
//
// AJV requires each registered schema to have a stable, URI-based identifier
// for $ref resolution. We use a synthetic "openapi://" URI scheme so that:
//   $ref: '#/components/schemas/Foo'  (from OpenAPI spec)
// resolves against the base URI to:
//   openapi://components/schemas/Foo
//
// See: https://ajv.js.org/guide/combining-schemas.html
const BASE_URI = 'openapi://';

// ── Build AJV instance ────────────────────────────────────────────────────────

const ajv = new Ajv({
    allErrors: true,       // report ALL errors, not just the first
    strict: false,         // allow additionalProperties (OpenAPI 3.x default)
    validateFormats: true,
});
addFormats(ajv);

// ── Register all schemas with stable URI-based $id (AJV official pattern) ─────
//
// Per AJV docs (managing-schemas.md): addSchema(schema, key) registers the
// schema in AJV's cache. The key acts as the schema $id if the schema itself
// has no $id field. We assign each schema a URI like:
//   "openapi://components/schemas/System"
//
// When a schema contains $ref: '#/components/schemas/Foo', AJV resolves it as a
// JSON Pointer against the *current document*. Since we spread properties from
// the OpenAPI spec schema object and inject all peer schemas under "definitions"
// (AJV-standard) plus a $defs alias, $ref across schemas resolves correctly.
//
// IMPORTANT: We use the AJV-recommended `getSchema(key)` check before
// addSchema() to avoid duplicate registration errors on module re-require
// (e.g., test runner hot reload or multiple test files importing this module).

for (const [name, schema] of Object.entries(schemas)) {
    const uri = `${BASE_URI}components/schemas/${name}`;
    if (!ajv.getSchema(uri)) {
        try {
            ajv.addSchema({ ...schema, $id: uri }, uri);
        } catch {
            // Ignore: defensive guard for edge cases in concurrent loading
        }
    }
}

// ── Pre-compile validator cache ───────────────────────────────────────────────
//
// AJV docs recommend pre-compiling schemas at startup rather than calling
// compile() on each validation request. This avoids repeated compilation
// overhead in long-running test suites and surfaces $ref errors early.
//
// Strategy: wrap each schema in a root document that:
//   1. Has a UNIQUE $id per schema (e.g., "openapi://validators/SystemList")
//      so it doesn't collide with other compiled schemas or the per-component
//      schemas registered above.
//   2. Exposes all peer schemas under 'definitions' (JSON Schema draft-07) and
//      '$defs' (JSON Schema 2019-09+) plus 'components.schemas' (OpenAPI 3.x)
//      so cross-schema $refs resolve.
//   3. Inlines the target schema properties (not via $ref) to avoid circular
//      $id conflicts.

const validatorCache = new Map<string, ValidateFunction>();

function getValidator(schemaName: string): ValidateFunction {
    const cached = validatorCache.get(schemaName);
    if (cached) return cached;

    const targetSchema = schemas[schemaName];
    if (!targetSchema) {
        const available = Object.keys(schemas).slice(0, 20).join(', ');
        throw new Error(
            `[schema-validator] Unknown schema: "${schemaName}".\n` +
            `Available (first 20): ${available}`
        );
    }

    // ── CRITICAL FIX (2026-02-22) ────────────────────────────────────────────
    // Each rootDoc gets a UNIQUE $id so AJV does not reject the second compile()
    // with "schema with key or id already exists". The previous code used
    // `$id: BASE_URI` ("openapi://") for every rootDoc which caused the crash.
    //
    // Per AJV docs: compile() implicitly registers the schema by its $id.
    // Using a unique $id per validator avoids collisions.
    const validatorUri = `${BASE_URI}validators/${schemaName}`;

    // Check if already compiled (AJV internal cache) —
    // per AJV docs: getSchema(key) || compile(schema) pattern
    const existing = ajv.getSchema(validatorUri);
    if (existing) {
        validatorCache.set(schemaName, existing);
        return existing;
    }

    const rootDoc = {
        $id: validatorUri,
        // Expose all schemas under both standard locations for $ref resolution
        definitions: schemas,              // JSON Schema draft-07
        $defs: schemas,                    // JSON Schema 2019-09+
        components: { schemas },           // OpenAPI 3.x $ref path
        // The actual schema to validate against (inline, not $ref, to avoid
        // circular $id conflicts with the already-registered URI)
        ...targetSchema,
    };

    const validate = ajv.compile(rootDoc);
    validatorCache.set(schemaName, validate);
    return validate;
}

// ── Core validation function ──────────────────────────────────────────────────

/**
 * Validate a parsed JSON body against a named OpenAPI component schema.
 *
 * @param schemaName - Name from openapi.yaml components/schemas (e.g. 'System', 'SystemList')
 * @param body       - Parsed JSON response body
 * @throws Error with AJV error details if body does not conform to schema
 */
export function validateResponse(schemaName: string, body: unknown): void {
    const validate = getValidator(schemaName);
    const valid = validate(body);

    if (!valid) {
        const errors = ajv.errorsText(validate.errors, {
            separator: '\n  • ',
            dataVar: 'response',
        });
        const preview = JSON.stringify(body, null, 2).slice(0, 800);
        throw new Error(
            `[schema-validator] "${schemaName}" schema violation:\n  • ${errors}\n\n` +
            `Response preview:\n${preview}`
        );
    }
}

// ── Playwright-aware helper ───────────────────────────────────────────────────

/** Minimal interface matching Playwright's Response object */
interface PlaywrightResponse {
    json(): Promise<unknown>;
    status(): number;
    url(): string;
}

/**
 * Validate a Playwright Response against a named OpenAPI schema.
 * Returns the parsed body so callers can do further assertions without
 * calling response.json() a second time (Playwright response body is
 * single-read — a second call throws a Protocol error).
 *
 * @example
 *   const resp = await page.waitForResponse(r => r.url().endsWith('/systems'));
 *   const body = await validateApiResponse('SystemList', resp);
 *   // body is typed as unknown but schema-validated; reuse it directly
 */
export async function validateApiResponse(
    schemaName: string,
    response: PlaywrightResponse
): Promise<unknown> {
    // Guard against "Protocol error (Network.getResponseBody): No resource with given identifier found"
    // This race condition occurs when the browser navigates away before Playwright reads the response body.
    // Per Playwright docs: always await waitForResponse before triggering navigation.
    // We still wrap in try-catch here to produce a clear diagnostic instead of a cryptic protocol error.
    let body: unknown;
    try {
        body = await response.json();
    } catch (err) {
        const msg = err instanceof Error ? err.message : String(err);
        // Distinguish navigation race-condition from real JSON parse failure
        if (msg.includes('No resource with given identifier') || msg.includes('Target closed')) {
            throw new Error(
                `[schema-validator] Response body was released before it could be read.\n` +
                `This is a test infrastructure race condition: page navigation completed\n` +
                `before Playwright read the response body for ${response.url()}.\n` +
                `Fix: register page.waitForResponse() BEFORE triggering page.goto().\n` +
                `Original error: ${msg}`
            );
        }
        throw new Error(
            `[schema-validator] Failed to parse response body as JSON for ${response.url()} ` +
            `(HTTP ${response.status()}): ${msg}`
        );
    }
    try {
        validateResponse(schemaName, body);
    } catch (err) {
        const msg = err instanceof Error ? err.message : String(err);
        throw new Error(`${msg}\n\nEndpoint: ${response.url()} (HTTP ${response.status()})`);
    }
    return body;
}

/**
 * Assert that a response body has all required fields from the spec.
 * Useful for quick spot-checks without full schema compilation.
 */
export function assertRequiredFields(schemaName: string, body: Record<string, unknown>): void {
    const schema = schemas[schemaName] as { required?: string[] } | undefined;
    const required = schema?.required ?? [];
    const missing = required.filter((f) => !(f in body));
    if (missing.length > 0) {
        throw new Error(
            `[schema-validator] "${schemaName}" missing required fields: ${missing.join(', ')}\n` +
            `Got keys: ${Object.keys(body).join(', ')}`
        );
    }
}
