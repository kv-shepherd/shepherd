import { Form } from "antd";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { describe, expect, it, vi } from "vitest";

const authState = {
  user: {
    id: "admin-1",
    username: "admin",
    permissions: [
      "auth_provider:configure",
      "auth_provider:update",
      "auth_provider:delete",
      "auth_provider:sync",
      "auth_provider:mapping_create",
      "auth_provider:mapping_update",
      "auth_provider:mapping_delete",
    ],
  },
};

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string, options?: { defaultValue?: string; count?: number }) => {
      const labels: Record<string, string> = {
        "authProviders.title": "Auth Providers",
        "authProviders.subtitle": "Manage authentication integrations",
        "authProviders.config_help":
          "Configure external providers and cohort mappings",
        "common:button.refresh": "Refresh",
        "common:button.details": "Details",
        "authProviders.add": "Add Provider",
        "authProviders.alpha_badge": "Alpha",
        "authProviders.alpha_title": "Alpha integrations",
        "authProviders.alpha_description":
          "Authentication provider integrations are currently in alpha.",
        "authProviders.table.provider": "Provider",
        "authProviders.table.provider_id": "Provider ID",
        "authProviders.table.integration": "Integration",
        "common:table.status": "Status",
        "users.status.enabled": "Enabled",
        "users.status.disabled": "Disabled",
        "authProviders.table.priority": "Priority",
        "authProviders.table.updated": "Updated",
        "common:table.actions": "Actions",
        "authProviders.test_connection": "Test Connection",
        "authProviders.cohort_mappings": "Cohort Mappings",
        "common:button.edit": "Edit",
        "common:button.delete": "Delete",
      };
      return labels[key] ?? options?.defaultValue ?? key;
    },
  }),
}));

vi.mock("@/stores/auth", () => ({
  useAuthStore: (selector: (state: typeof authState) => unknown) => selector(authState),
}));

vi.mock("antd", async (importOriginal) => {
  const actual = await importOriginal<typeof import("antd")>();

  const DescriptionsItem = ({
    label,
    children,
  }: {
    label?: ReactNode;
    children?: ReactNode;
  }) => (
    <div data-testid="antd-descriptions-item">
      {label ? <dt>{label}</dt> : null}
      <dd>{children}</dd>
    </div>
  );

  const Descriptions = (({
    items,
    children,
  }: {
    items?: Array<{ key?: string; label?: ReactNode; children?: ReactNode }>;
    children?: ReactNode;
  }) => (
    <div data-testid="antd-descriptions">
      {items
        ? items.map((item, index) => (
            <DescriptionsItem
              key={String(item.key ?? index)}
              label={item.label}
            >
              {item.children}
            </DescriptionsItem>
          ))
        : children}
    </div>
  )) as typeof actual.Descriptions;
  Descriptions.Item = DescriptionsItem as typeof actual.Descriptions.Item;

  return {
    ...actual,
    Card: ({
      title,
      extra,
      children,
    }: {
      title?: ReactNode;
      extra?: ReactNode;
      children?: ReactNode;
    }) => (
      <section data-testid="antd-card">
        {title ? <header>{title}</header> : null}
        {extra ? <div>{extra}</div> : null}
        <div>{children}</div>
      </section>
    ),
    Collapse: ({
      items,
    }: {
      items?: Array<{ key?: string; label?: ReactNode; children?: ReactNode }>;
    }) => (
      <section data-testid="antd-collapse">
        {items?.map((item, index) => (
          <div key={String(item.key ?? index)}>
            {item.label ? <div>{item.label}</div> : null}
            <div>{item.children}</div>
          </div>
        ))}
      </section>
    ),
    Descriptions,
    Drawer: ({
      open,
      title,
      children,
      footer,
    }: {
      open?: boolean;
      title?: ReactNode;
      children?: ReactNode;
      footer?: ReactNode;
    }) =>
      open ? (
        <section data-testid="antd-drawer">
          {title ? <header>{title}</header> : null}
          <div>{children}</div>
          {footer ? <footer>{footer}</footer> : null}
        </section>
      ) : null,
    Modal: ({
      open,
      title,
      children,
      footer,
    }: {
      open?: boolean;
      title?: ReactNode;
      children?: ReactNode;
      footer?: ReactNode;
    }) =>
      open ? (
        <section className="ant-modal">
          {title ? <header>{title}</header> : null}
          <div>{children}</div>
          {footer ? <footer>{footer}</footer> : null}
        </section>
      ) : null,
    Steps: ({
      items = [],
      current = 0,
    }: {
      items?: Array<{ title?: ReactNode; description?: ReactNode }>;
      current?: number;
    }) => (
      <ol data-testid="antd-steps">
        {items.map((item, index) => (
          <li key={index} data-current={index === current}>
            {item.title}
            {item.description}
          </li>
        ))}
      </ol>
    ),
    Table: ({
      columns = [],
      dataSource = [],
    }: {
      columns?: Array<{
        key?: string;
        title?: ReactNode;
        dataIndex?: string | string[];
        render?: (value: unknown, record: Record<string, unknown>, index: number) => ReactNode;
      }>;
      dataSource?: Array<Record<string, unknown>>;
    }) => (
      <table data-testid="antd-table">
        <thead>
          <tr>
            {columns.map((column, index) => (
              <th key={String(column.key ?? column.dataIndex ?? index)}>
                {column.title}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {dataSource.map((record, rowIndex) => (
            <tr key={String(record.id ?? rowIndex)}>
              {columns.map((column, columnIndex) => {
                const value = Array.isArray(column.dataIndex)
                  ? column.dataIndex.reduce<unknown>(
                      (current, key) =>
                        current && typeof current === "object"
                          ? (current as Record<string, unknown>)[key]
                          : undefined,
                      record,
                    )
                  : typeof column.dataIndex === "string"
                    ? record[column.dataIndex]
                    : undefined;
                const content = column.render
                  ? column.render(value, record, rowIndex)
                  : (value as ReactNode);
                return (
                  <td
                    key={String(column.key ?? column.dataIndex ?? columnIndex)}
                  >
                    {content}
                  </td>
                );
              })}
            </tr>
          ))}
        </tbody>
      </table>
    ),
  };
});

vi.mock("@/components/feedback/ActionEmptyState", () => ({
  ActionEmptyState: ({
    title,
    description,
    actions,
  }: {
    title: ReactNode;
    description?: ReactNode;
    actions?: ReactNode;
  }) => (
    <section data-testid="action-empty-state">
      <h2>{title}</h2>
      {description ? <p>{description}</p> : null}
      {actions}
    </section>
  ),
}));

vi.mock("@/components/feedback/SummaryMetricCard", () => ({
  SummaryMetricCard: ({
    title,
    value,
    description,
    action,
  }: {
    title: ReactNode;
    value?: ReactNode;
    description?: ReactNode;
    action?: ReactNode;
  }) => (
    <section data-testid="summary-metric-card">
      <h2>{title}</h2>
      {value ? <div>{value}</div> : null}
      {description ? <div>{description}</div> : null}
      {action}
    </section>
  ),
}));

vi.mock("@/components/layouts/PageSection", () => ({
  PageHeader: ({
    title,
    subtitle,
    actions,
  }: {
    title: ReactNode;
    subtitle?: ReactNode;
    actions?: ReactNode;
  }) => (
    <header data-testid="page-header">
      <h1>{title}</h1>
      {subtitle ? <p>{subtitle}</p> : null}
      {actions}
    </header>
  ),
  PageSurface: ({ children }: { children: ReactNode }) => (
    <section data-testid="page-surface">{children}</section>
  ),
}));

vi.mock("@/components/ui/LocalDateTimeText", () => ({
  LocalDateTimeText: ({ value }: { value?: string | null }) =>
    value ? <time dateTime={value}>{value}</time> : null,
}));

vi.mock("@/components/illustrations/DashboardIllustrations", () => ({
  AccessControlGlyph: (props: Record<string, unknown>) => (
    <span {...props}>access-glyph</span>
  ),
  NotificationInboxGlyph: (props: Record<string, unknown>) => (
    <span {...props}>notification-glyph</span>
  ),
  QueueReviewGlyph: (props: Record<string, unknown>) => (
    <span {...props}>queue-glyph</span>
  ),
  RateLimitGaugeGlyph: (props: Record<string, unknown>) => (
    <span {...props}>rate-limit-glyph</span>
  ),
  RequestsOverviewGlyph: (props: Record<string, unknown>) => (
    <span {...props}>requests-glyph</span>
  ),
  ServiceWorkspaceGlyph: (props: Record<string, unknown>) => (
    <span {...props}>service-glyph</span>
  ),
}));

vi.mock("./SchemaConfigForm", () => ({
  SchemaConfigForm: () => <div data-testid="schema-config-form" />,
}));

function buildMockControllerOverrides(): Record<string, unknown> {
  return {
    messageContextHolder: null,
    providers: [
      {
        id: "provider-1",
        name: "GitHub OAuth",
        auth_type: "oidc",
        enabled: true,
        sort_order: 1,
        updated_at: "2026-03-17T00:00:00Z",
      },
    ],
    providersLoading: false,
    refetchProviders: vi.fn(),
    openCreateModal: vi.fn(),
    openEditModal: vi.fn(),
    openDeleteModal: vi.fn(),
    openMappingModal: vi.fn(),
    testConnection: vi.fn(),
    previewDirectory: vi.fn(),
    syncDirectory: vi.fn(),
    testingProviderId: "",
    testConnectionPending: false,
    previewDirectoryPending: false,
    syncDirectoryPending: false,
    providerTypeLabelByKey: { oidc: "OIDC" },
    roleOptions: [],
    createOpen: false,
    editOpen: false,
    createPending: false,
    updatePending: false,
    submitCreate: vi.fn(),
    submitEdit: vi.fn(),
    closeCreateModal: vi.fn(),
    closeEditModal: vi.fn(),
    editingProvider: null,
    providerTypeOptions: [],
    providerTypesLoading: false,
    mappingOpen: false,
    mappingProvider: null,
    closeMappingModal: vi.fn(),
    sampleFields: [],
    sampleLoading: false,
    cohorts: [],
    cohortsLoading: false,
    runtimeDescriptor: {
      supported: false,
      supports_redirect: false,
      supports_credentials: false,
      requires_public_base_url: false,
      login_modes: [],
    },
    runtimeDescriptorLoading: false,
    directoryDescriptor: undefined,
    directoryDescriptorLoading: false,
    directoryDescriptorUnsupported: true,
    directoryPreview: null,
    directorySchedule: undefined,
    directoryScheduleLoading: false,
    directorySyncJobs: [],
    directorySyncJobsLoading: false,
    directorySyncJobDetail: null,
    directorySyncJobDetailLoading: false,
    selectedDirectorySyncJobId: null,
    openDirectorySyncJobDetail: vi.fn(),
    closeDirectorySyncJobDetail: vi.fn(),
    syncCohortsPending: false,
    submitSyncCohorts: vi.fn(),
    mappingsLoading: false,
    mappings: [],
    openCreateMappingModal: vi.fn(),
    createMappingModalOpen: false,
    mappingEditOpen: false,
    submitCreateMapping: vi.fn(),
    submitEditMapping: vi.fn(),
    deleteMapping: vi.fn(),
    closeCreateMappingModal: vi.fn(),
    closeEditMappingModal: vi.fn(),
    createMappingPending: false,
    updateMappingPending: false,
    deleteMappingPending: false,
    editingMapping: null,
    providerTypes: [],
    externalAuthSettings: {
      public_base_url: "",
      effective_public_base_url: "https://console.example.com",
      runtime_login_ready: true,
      source: "server_config",
    },
    externalAuthSettingsLoading: false,
    submitExternalAuthSettings: vi.fn(),
    resetExternalAuthSettingsToDeploymentDefault: vi.fn(),
    updateExternalAuthSettingsPending: false,
    scopeTargetOptionsByType: {},
    scopeTargetLoadingByType: {},
  };
}

let mockControllerOverrides: Record<string, unknown> = {};

vi.mock("../hooks/useAdminAuthProvidersController", () => ({
  useAdminAuthProvidersController: () => {
    const [createForm] = Form.useForm();
    const [editForm] = Form.useForm();
    const [syncForm] = Form.useForm();
    const [directoryRequestForm] = Form.useForm();
    const [mappingForm] = Form.useForm();
    const [mappingEditForm] = Form.useForm();
    const [externalAuthSettingsForm] = Form.useForm();

    return {
      ...buildMockControllerOverrides(),
      ...mockControllerOverrides,
      createForm,
      editForm,
      syncForm,
      directoryRequestForm,
      mappingForm,
      mappingEditForm,
      externalAuthSettingsForm,
    };
  },
}));

import { AdminAuthProvidersContent } from "./AdminAuthProvidersContent";

describe("AdminAuthProvidersContent", () => {
  it("renders the page shell and primary provider actions", () => {
    authState.user.permissions = [
      "auth_provider:configure",
      "auth_provider:update",
      "auth_provider:delete",
      "auth_provider:sync",
      "auth_provider:mapping_create",
      "auth_provider:mapping_update",
      "auth_provider:mapping_delete",
    ];
    mockControllerOverrides = buildMockControllerOverrides();

    render(<AdminAuthProvidersContent />);

    expect(screen.getByText("Auth Providers")).toBeVisible();
    expect(screen.getByTestId("auth-provider-create-button")).toBeVisible();
    expect(screen.getByText("GitHub OAuth")).toBeVisible();
    expect(screen.getByText("Provider ID: provider-1")).toBeVisible();
    expect(screen.getByText("Alpha integrations")).toBeVisible();
    expect(screen.getAllByText("Alpha").length).toBeGreaterThan(0);
    expect(
      screen.getByTestId("auth-provider-action-test-provider-1"),
    ).toBeVisible();
  });

  it("keeps auth provider inventory readable while hiding mutation controls for read-only access", () => {
    authState.user.permissions = ["auth_provider:read"];
    mockControllerOverrides = buildMockControllerOverrides();

    render(<AdminAuthProvidersContent />);

    expect(screen.queryByTestId("auth-provider-create-button")).not.toBeInTheDocument();
    expect(screen.getByTestId("auth-provider-action-mappings-provider-1")).toBeVisible();
    expect(screen.queryByTestId("auth-provider-action-test-provider-1")).not.toBeInTheDocument();
    expect(screen.queryByTestId("auth-provider-action-edit-provider-1")).not.toBeInTheDocument();
    expect(screen.queryByTestId("auth-provider-action-delete-provider-1")).not.toBeInTheDocument();
  });

  it(
    "opens the latest blocked job from the preview summary card",
    async () => {
      const user = userEvent.setup();
      const openDirectorySyncJobDetail = vi.fn();
      mockControllerOverrides = {
        ...buildMockControllerOverrides(),
        directoryDescriptor: {
          description: "Directory preview",
          request_schema: {
            type: "object",
            properties: {},
          },
        },
        directoryDescriptorUnsupported: false,
        mappingOpen: true,
        mappingProvider: {
          id: "provider-1",
          name: "corp-directory-enrichment",
          auth_type: "generic",
          enabled: true,
        },
        directoryPreview: {
          items: [
            {
              external_id: "u-1",
              record: {
                external_id: "u-1",
                username: "alice",
                display_name: "Alice",
                email: "alice@example.com",
                cohorts: [],
                attributes: {},
              },
              match: {
                action: "blocked",
              },
              warnings: [],
              conflicts: ["username_conflict"],
            },
          ],
        },
        directorySyncJobs: [
          {
            id: "job-blocked",
            provider_id: "provider-1",
            sync_mode: "manual_import",
            status: "completed",
            join_key_type: "username",
            total_entries: 1,
            result_summary: {
              create_count: 0,
              update_count: 0,
              blocked_count: 1,
            },
            error_count: 0,
            errors: [],
            triggered_by: "admin",
            created_at: "2026-03-23T01:00:00Z",
            started_at: "2026-03-23T01:00:01Z",
            completed_at: "2026-03-23T01:00:02Z",
          },
        ],
        openDirectorySyncJobDetail,
      };

      render(<AdminAuthProvidersContent />);

      const actionButton = await screen.findByTestId(
        "directory-preview-latest-blocked-job",
      );
      await user.click(actionButton);

      await waitFor(() =>
        expect(openDirectorySyncJobDetail).toHaveBeenCalledWith(
          expect.objectContaining({ id: "job-blocked" }),
        ),
      );
    },
  );

});
