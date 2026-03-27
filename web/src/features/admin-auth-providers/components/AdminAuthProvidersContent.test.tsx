import { Form } from "antd";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

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
    mockControllerOverrides = buildMockControllerOverrides();

    render(<AdminAuthProvidersContent />);

    expect(screen.getByText("Auth Providers")).toBeVisible();
    expect(screen.getByTestId("auth-provider-create-button")).toBeVisible();
    expect(screen.getByText("GitHub OAuth")).toBeVisible();
    expect(screen.getByText("Provider ID: provider-1")).toBeVisible();
    expect(
      screen.getByTestId("auth-provider-action-test-provider-1"),
    ).toBeVisible();
  }, 30000);

  it(
    "opens the latest blocked job from the preview summary card",
    async () => {
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
      fireEvent.click(actionButton);

      await waitFor(() =>
        expect(openDirectorySyncJobDetail).toHaveBeenCalledWith(
          expect.objectContaining({ id: "job-blocked" }),
        ),
      );
    },
    20000,
  );
});
