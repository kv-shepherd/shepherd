import { describe, expect, it } from "vitest";

import {
  buildSchemaHelpKey,
  resolveSchemaHelpText,
  type SchemaHelpScope,
} from "./schemaHelp";

function createTranslator(
  entries: Record<string, string>,
): (key: string, options?: { defaultValue?: string }) => string {
  return (key: string, options?: { defaultValue?: string }) =>
    entries[key] ?? options?.defaultValue ?? key;
}

describe("schemaHelp", () => {
  it("builds stable keys from schema paths", () => {
    expect(
      buildSchemaHelpKey(
        "instanceSize",
        "spec.template.spec.domain.cpu.threads",
      ),
    ).toBe("instanceSize.spec__template__spec__domain__cpu__threads");
  });

  it("prefers translated schema help when present", () => {
    const t = createTranslator({
      "instanceSize.spec__template__spec__domain__cpu__threads":
        "Translated help",
    });

    expect(
      resolveSchemaHelpText(
        t as unknown as Parameters<typeof resolveSchemaHelpText>[0],
        "instanceSize" as SchemaHelpScope,
        "spec.template.spec.domain.cpu.threads",
        "Fallback help",
      ),
    ).toBe("Translated help");
  });

  it("falls back to official schema description when translation is missing", () => {
    const t = createTranslator({});

    expect(
      resolveSchemaHelpText(
        t as unknown as Parameters<typeof resolveSchemaHelpText>[0],
        "instanceSize" as SchemaHelpScope,
        "spec.template.spec.domain.cpu.threads",
        "Fallback help",
      ),
    ).toBe("Fallback help");
  });
});
