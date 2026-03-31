import { Form } from "antd";
import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { SchemaConfigForm } from "./SchemaConfigForm";

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string, options?: { defaultValue?: string }) =>
      options?.defaultValue ?? key,
  }),
}));

function renderSchemaConfigForm(options: {
  initialValues?: Record<string, unknown>;
  applySchemaDefaults?: boolean;
  schema?: Record<string, unknown>;
}) {
  function TestHarness() {
    const [form] = Form.useForm();
    return (
      <Form form={form} initialValues={options.initialValues}>
        <SchemaConfigForm
          schema={(options.schema as Parameters<typeof SchemaConfigForm>[0]["schema"]) ?? {
            type: "object",
            properties: {
              login_entry_url: {
                type: "string",
                title: "Login entry URL",
                default: "https://default.example.com/login",
              },
            },
          }}
          form={form}
          namePrefix="config"
          showJsonFallback={false}
          applySchemaDefaults={options.applySchemaDefaults}
        />
      </Form>
    );
  }
  render(
    <TestHarness />,
  );
}

describe("SchemaConfigForm", () => {
  it("prefers existing form values over schema defaults in edit-style usage", () => {
    renderSchemaConfigForm({
      initialValues: {
        config: {
          login_entry_url: "https://portal.example.com/login",
        },
      },
      applySchemaDefaults: false,
    });

    expect(
      screen.getByDisplayValue("https://portal.example.com/login"),
    ).toBeVisible();
  });

  it("applies schema defaults in create-style usage", () => {
    renderSchemaConfigForm({
      applySchemaDefaults: true,
    });

    expect(
      screen.getByDisplayValue("https://default.example.com/login"),
    ).toBeVisible();
  });

  it("renders generic string-array schema fields without provider-specific sections", async () => {
    renderSchemaConfigForm({
      schema: {
        type: "object",
        properties: {
          scopes: {
            type: "array",
            title: "Scopes",
            items: { type: "string" },
          },
        },
      },
    });

    fireEvent.mouseDown(screen.getByRole("combobox"));

    expect(await screen.findByText("Scopes")).toBeVisible();
    expect(screen.queryByText("User directory presentation")).not.toBeInTheDocument();
  });

  it("prefers schema-provided enum labels before falling back to raw enum values", () => {
    renderSchemaConfigForm({
      schema: {
        type: "object",
        properties: {
          incoming_token_transport: {
            type: "string",
            title: "Incoming Token Transport",
            enum: ["query", "form"],
            "x-enum-labels": {
              query: "Query parameter",
              form: "Form field",
            },
          },
        },
      },
    });

    fireEvent.mouseDown(screen.getByRole("combobox"));

    expect(screen.getAllByText("Query parameter").length).toBeGreaterThan(0);
    expect(screen.getAllByText("Form field").length).toBeGreaterThan(0);
  });
});
