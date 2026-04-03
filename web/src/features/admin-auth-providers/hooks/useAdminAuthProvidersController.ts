"use client";

import { App, Form } from "antd";
import type { TFunction } from "i18next";
import { useEffect, useMemo, useRef, useState } from "react";

import { useApiAction, useApiGet, useApiMutation } from "@/hooks/useApiQuery";
import { api } from "@/lib/api/client";
import { translateApiError } from "@/lib/api/errorMessage";
import { useScopeTargetCatalog } from "@/features/rbac-shared/useScopeTargetCatalog";

import type {
  AuthProvider,
  AuthProviderConnectionTestResult,
  AuthProviderCreateRequest,
  AuthProviderRuntimeDescriptor,
  DirectorySyncDescriptor,
  DirectorySyncPreview,
  DirectorySyncPreviewRequest,
  DirectorySyncRequest,
  DirectorySyncStartResponse,
  DirectoryEnrichmentScheduleStatus,
  DirectorySyncJob,
  DirectorySyncJobDetail,
  DirectorySyncJobList,
  AuthProviderList,
  AuthProviderType,
  AuthProviderTypeList,
  ExternalAuthPlatformSettings,
  ExternalAuthPlatformSettingsUpdateRequest,
  AuthProviderSampleResponse,
  AuthProviderUpdateRequest,
  ExternalCohort,
  ExternalCohortList,
  ExternalCohortMapping,
  ExternalCohortMappingCreateRequest,
  ExternalCohortMappingList,
  ExternalCohortMappingUpdateRequest,
  ExternalCohortSyncRequest,
  Role,
  RoleList,
} from "../types";

interface UseAdminAuthProvidersControllerArgs {
  t: TFunction;
}

interface CreateFormValues {
  name: string;
  auth_type: AuthProvider["auth_type"];
  enabled?: boolean;
  sort_order?: number;
}

interface EditFormValues {
  name?: string;
  enabled?: boolean;
  sort_order?: number;
}

interface SyncFormValues {
  cohort_kind: string;
  source_field: string;
  cohorts_text: string | string[];
}

interface MappingFormValues {
  selected_cohort_ref?: string;
  cohort_kind: string;
  cohort_key: string;
  cohort_display_name?: string;
  role_id: string;
  scope_type?: string;
  scope_id?: string;
  allowed_environments?: Array<"test" | "prod">;
}

interface MappingEditFormValues {
  role_id?: string;
  scope_type?: string;
  scope_id?: string;
  allowed_environments?: Array<"test" | "prod">;
}

interface ExternalAuthSettingsFormValues {
  public_base_url?: string;
}

interface SchemaProperty {
  type?: string;
  default?: unknown;
}

interface ConfigSchema {
  properties?: Record<string, SchemaProperty>;
}

type SampleField = NonNullable<AuthProviderSampleResponse["fields"]>[number];

interface CohortDefaults {
  cohortKind: string;
  sourceField: string;
  reason: string;
}

function localizedAuthProviderTypeLabel(
  t: TFunction,
  type: string,
  fallback: string,
) {
  return t(`authProviders.types.${type}.label`, {
    defaultValue: fallback,
  });
}

function parseCohortText(raw: string | string[]): string[] {
  const seen = new Set<string>();
  const tokens = Array.isArray(raw) ? raw : raw.split(/[\n,]/g);
  for (const token of tokens) {
    const value = token.trim();
    if (!value) continue;
    seen.add(value);
  }
  return Array.from(seen.values()).sort((a, b) => a.localeCompare(b));
}

function normalizeFieldToken(value: string): string {
  return value.trim().toLowerCase().replace(/[^a-z0-9]+/g, "_");
}

function cohortDefaultsForAuthType(
  authType: string | null | undefined,
  sampleFields: SampleField[],
  cohorts: ExternalCohort[],
): CohortDefaults {
  void authType;

  const fallback: CohortDefaults = {
    cohortKind: "group",
    sourceField: "groups",
    reason: "fallback_groups",
  };

  const discoveredCandidates = cohorts
    .filter(
      (cohort): cohort is ExternalCohort & { source_field: string } =>
        typeof cohort.source_field === "string" && cohort.source_field.trim() !== "",
    )
    .map((cohort) => ({
      cohortKind: cohort.cohort_kind,
      sourceField: cohort.source_field,
      reason: "discovered_cohort",
    }));
  if (discoveredCandidates.length > 0) {
    return discoveredCandidates.sort((left, right) => {
      return (
        left.cohortKind.localeCompare(right.cohortKind) ||
        left.sourceField.localeCompare(right.sourceField)
      );
    })[0];
  }

  const fieldIndex = new Map(
    sampleFields.map((field) => [normalizeFieldToken(field.field), field]),
  );

  const preferredFields: Array<{
    aliases: string[];
    cohortKind: string;
    sourceField: string;
    reason: string;
  }> = [
    {
      aliases: ["department", "departments", "dept"],
      cohortKind: "department",
      sourceField: "department",
      reason: "sample_department",
    },
    {
      aliases: ["section", "sections", "team", "teams"],
      cohortKind: "team",
      sourceField: "section",
      reason: "sample_section",
    },
    {
      aliases: ["groups", "group"],
      cohortKind: "group",
      sourceField: "groups",
      reason: "sample_groups",
    },
    {
      aliases: ["organization", "organisation", "org", "company"],
      cohortKind: "organization",
      sourceField: "organization",
      reason: "sample_organization",
    },
  ];

  for (const candidate of preferredFields) {
    const matchedField = candidate.aliases
      .map((alias) => fieldIndex.get(alias))
      .find((field): field is SampleField => Boolean(field));
    if (matchedField) {
      return {
        cohortKind: candidate.cohortKind,
        sourceField: matchedField.field,
        reason: candidate.reason,
      };
    }
  }

  return fallback;
}

function extractConfigObject(
  formValues: Record<string, unknown>,
  schema?: ConfigSchema | null,
  fieldName = "config",
): Record<string, unknown> | undefined {
  const raw = formValues[fieldName];
  if (!raw || typeof raw !== "object" || Array.isArray(raw)) {
    return undefined;
  }
  const config: Record<string, unknown> = {};
  for (const [key, value] of Object.entries(raw as Record<string, unknown>)) {
    if (value === undefined || value === null || value === "") {
      continue;
    }
    const propertyType = schema?.properties?.[key]?.type;
    if (propertyType === "object") {
      if (typeof value === "string") {
        const parsed = JSON.parse(value);
        if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) {
          throw new Error(`${fieldName}.${key} must be a JSON object`);
        }
        config[key] = parsed;
        continue;
      }
      if (typeof value === "object" && !Array.isArray(value)) {
        config[key] = value;
        continue;
      }
      throw new Error(`${fieldName}.${key} must be a JSON object`);
    }
    config[key] = value;
  }
  return Object.keys(config).length > 0 ? config : undefined;
}

function schemaFormValuesFromObject(
  value: Record<string, unknown> | null | undefined,
  schema?: ConfigSchema | null,
): Record<string, unknown> | undefined {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    return undefined;
  }
  return Object.fromEntries(
    Object.entries(value).map(([key, rawValue]) => {
      if (
        schema?.properties?.[key]?.type === "object" &&
        rawValue &&
        typeof rawValue === "object" &&
        !Array.isArray(rawValue)
      ) {
        return [key, JSON.stringify(rawValue, null, 2)];
      }
      return [key, rawValue];
    }),
  );
}

function schemaDefaultFormValues(
  schema?: ConfigSchema | null,
): Record<string, unknown> | undefined {
  if (!schema?.properties) {
    return undefined;
  }
  const entries = Object.entries(schema.properties)
    .filter(([, property]) => property.default !== undefined)
    .map(([key, property]) => {
      if (
        property.type === "object" &&
        property.default &&
        typeof property.default === "object" &&
        !Array.isArray(property.default)
      ) {
        return [key, JSON.stringify(property.default, null, 2)] as const;
      }
      return [key, property.default] as const;
    });
  if (entries.length === 0) {
    return undefined;
  }
  return Object.fromEntries(entries);
}

function buildProviderEditFormValues(
  provider: AuthProvider,
  providerTypes: AuthProviderType[],
): Record<string, unknown> {
  const editProviderType = providerTypes.find(
    (tp) => tp.type === provider.auth_type,
  );
  const formValues: Record<string, unknown> = {
    name: provider.name,
    enabled: provider.enabled,
    sort_order: provider.sort_order,
  };
  const configSchema = (editProviderType?.config_schema ?? null) as ConfigSchema | null;
  const defaultConfigValues = schemaDefaultFormValues(configSchema);
  const storedConfigValues =
    provider.config && typeof provider.config === "object"
      ? schemaFormValuesFromObject(
          provider.config as Record<string, unknown>,
          configSchema,
        )
      : undefined;
  if (defaultConfigValues || storedConfigValues) {
    formValues.config = {
      ...(defaultConfigValues ?? {}),
      ...(storedConfigValues ?? {}),
    };
  }
  return formValues;
}

function buildDirectoryRequestFormValues(
  schema?: ConfigSchema | null,
  providerRequest?: Record<string, unknown> | null,
): Record<string, unknown> | undefined {
  const defaultRequestValues = schemaDefaultFormValues(schema);
  const storedRequestValues = providerRequest
    ? schemaFormValuesFromObject(providerRequest, schema)
    : undefined;
  if (!defaultRequestValues && !storedRequestValues) {
    return undefined;
  }
  return {
    provider_request: {
      ...(defaultRequestValues ?? {}),
      ...(storedRequestValues ?? {}),
    },
  };
}

export function useAdminAuthProvidersController({
  t,
}: UseAdminAuthProvidersControllerArgs) {
  const { message: messageApi } = App.useApp();
  const messageContextHolder = null;

  const [createOpen, setCreateOpen] = useState(false);
  const [editOpen, setEditOpen] = useState(false);
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [mappingOpen, setMappingOpen] = useState(false);
  const [editMappingOpen, setEditMappingOpen] = useState(false);
  const [createMappingModalOpen, setCreateMappingModalOpen] = useState(false);

  const [editingProvider, setEditingProvider] = useState<AuthProvider | null>(
    null,
  );
  const [deletingProvider, setDeletingProvider] = useState<AuthProvider | null>(
    null,
  );
  const [mappingProvider, setMappingProvider] = useState<AuthProvider | null>(
    null,
  );
  const [editingMapping, setEditingMapping] =
    useState<ExternalCohortMapping | null>(null);
  const [testingProviderId, setTestingProviderId] = useState<string>("");
  const [selectedDirectorySyncJobId, setSelectedDirectorySyncJobId] =
    useState<string>("");

  const [createForm] = Form.useForm<CreateFormValues>();
  const [editForm] = Form.useForm<EditFormValues>();
  const [syncForm] = Form.useForm<SyncFormValues>();
  const [mappingForm] = Form.useForm<MappingFormValues>();
  const [mappingEditForm] = Form.useForm<MappingEditFormValues>();
  const [externalAuthSettingsForm] = Form.useForm<ExternalAuthSettingsFormValues>();
  const [directoryRequestForm] = Form.useForm<Record<string, unknown>>();
  const [directoryPreview, setDirectoryPreview] =
    useState<DirectorySyncPreview | null>(null);
  const directoryRequestHydratedForRef = useRef("");

  const providersQuery = useApiGet<AuthProviderList>(
    ["admin-auth-providers"],
    () => api.GET("/admin/auth-providers"),
  );

  const providerTypesQuery = useApiGet<AuthProviderTypeList>(
    ["admin-auth-provider-types"],
    () => api.GET("/admin/auth-provider-types"),
  );

  const rolesQuery = useApiGet<RoleList>(["admin-auth-provider-roles"], () =>
    api.GET("/admin/roles"),
  );

  const externalAuthSettingsQuery = useApiGet<ExternalAuthPlatformSettings>(
    ["admin-external-auth-platform-settings"],
    () => api.GET("/admin/platform-settings/external-auth"),
  );

  const sampleQuery = useApiGet<AuthProviderSampleResponse>(
    ["admin-auth-provider-sample", mappingProvider?.id ?? ""],
    () =>
      api.GET("/admin/auth-providers/{provider_id}/sample", {
        params: { path: { provider_id: mappingProvider?.id ?? "" } },
      }),
    { enabled: mappingOpen && !!mappingProvider?.id },
  );

  const mappingsQuery = useApiGet<ExternalCohortMappingList>(
    ["admin-auth-provider-mappings", mappingProvider?.id ?? ""],
    () =>
      api.GET("/admin/auth-providers/{provider_id}/cohort-mappings", {
        params: { path: { provider_id: mappingProvider?.id ?? "" } },
      }),
    { enabled: mappingOpen && !!mappingProvider?.id },
  );

  const cohortsQuery = useApiGet<ExternalCohortList>(
    ["admin-auth-provider-cohorts", mappingProvider?.id ?? ""],
    () =>
      api.GET("/admin/auth-providers/{provider_id}/cohorts", {
        params: { path: { provider_id: mappingProvider?.id ?? "" } },
      }),
    { enabled: mappingOpen && !!mappingProvider?.id },
  );

  const directoryScheduleQuery = useApiGet<DirectoryEnrichmentScheduleStatus>(
    ["admin-auth-provider-directory-schedule", mappingProvider?.id ?? ""],
    () =>
      api.GET("/admin/auth-providers/{provider_id}/directory/schedule", {
        params: { path: { provider_id: mappingProvider?.id ?? "" } },
      }),
    { enabled: mappingOpen && !!mappingProvider?.id },
  );

  const runtimeDescriptorQuery = useApiGet<AuthProviderRuntimeDescriptor>(
    ["admin-auth-provider-runtime-descriptor", mappingProvider?.id ?? ""],
    () =>
      api.GET("/admin/auth-providers/{provider_id}/runtime", {
        params: { path: { provider_id: mappingProvider?.id ?? "" } },
      }),
    { enabled: mappingOpen && !!mappingProvider?.id },
  );

  const directoryDescriptorQuery = useApiGet<DirectorySyncDescriptor>(
    ["admin-auth-provider-directory-descriptor", mappingProvider?.id ?? ""],
    () =>
      api.GET("/admin/auth-providers/{provider_id}/directory/descriptor", {
        params: { path: { provider_id: mappingProvider?.id ?? "" } },
      }),
    { enabled: mappingOpen && !!mappingProvider?.id, retry: false },
  );

  const directorySyncJobsQuery = useApiGet<DirectorySyncJobList>(
    ["admin-auth-provider-directory-sync-jobs", mappingProvider?.id ?? ""],
    () =>
      api.GET("/admin/auth-providers/{provider_id}/directory/sync-jobs", {
        params: {
          path: { provider_id: mappingProvider?.id ?? "" },
          query: { page: 1, per_page: 5 },
        },
      }),
    { enabled: mappingOpen && !!mappingProvider?.id },
  );

  const directorySyncJobDetailQuery = useApiGet<DirectorySyncJobDetail>(
    [
      "admin-auth-provider-directory-sync-job-detail",
      mappingProvider?.id ?? "",
      selectedDirectorySyncJobId,
    ],
    () =>
      api.GET("/admin/auth-providers/{provider_id}/directory/sync-jobs/{job_id}", {
        params: {
          path: {
            provider_id: mappingProvider?.id ?? "",
            job_id: selectedDirectorySyncJobId,
          },
        },
      }),
    {
      enabled:
        mappingOpen && !!mappingProvider?.id && !!selectedDirectorySyncJobId,
    },
  );

  const scopeCatalogEnabled =
    mappingOpen && (createMappingModalOpen || editMappingOpen);

  const { scopeTargetOptionsByType, scopeTargetLoadingByType } =
    useScopeTargetCatalog(scopeCatalogEnabled);

  const createMutation = useApiMutation<
    AuthProviderCreateRequest,
    AuthProvider
  >((body) => api.POST("/admin/auth-providers", { body }), {
    invalidateKeys: [["admin-auth-providers"]],
    onSuccess: () => {
      messageApi.success(t("common:message.success"));
      createForm.resetFields();
      setCreateOpen(false);
    },
    onError: (err) => messageApi.error(translateApiError(t, err)),
  });

  const updateMutation = useApiMutation<
    { providerId: string; body: AuthProviderUpdateRequest },
    AuthProvider
  >(
    ({ providerId, body }) =>
      api.PATCH("/admin/auth-providers/{provider_id}", {
        params: { path: { provider_id: providerId } },
        body,
      }),
    {
      invalidateKeys: [["admin-auth-providers"]],
      onSuccess: () => {
        messageApi.success(t("common:message.success"));
        editForm.resetFields();
        setEditOpen(false);
        setEditingProvider(null);
      },
      onError: (err) => messageApi.error(translateApiError(t, err)),
    },
  );

  const deleteMutation = useApiAction<string>(
    (providerId) =>
      api.DELETE("/admin/auth-providers/{provider_id}", {
        params: { path: { provider_id: providerId } },
      }),
    {
      invalidateKeys: [["admin-auth-providers"]],
      onSuccess: () => {
        messageApi.success(t("common:message.success"));
        setDeleteOpen(false);
        setDeletingProvider(null);
      },
      onError: (err) => messageApi.error(translateApiError(t, err)),
    },
  );

  const updateExternalAuthSettingsMutation = useApiMutation<
    ExternalAuthPlatformSettingsUpdateRequest,
    ExternalAuthPlatformSettings
  >(
    (body) =>
      api.PUT("/admin/platform-settings/external-auth", {
        body,
      }),
    {
      invalidateKeys: [["admin-external-auth-platform-settings"]],
      onSuccess: (resp) => {
        messageApi.success(t("common:message.success"));
        externalAuthSettingsForm.setFieldsValue({
          public_base_url: resp.public_base_url || "",
        });
      },
      onError: (err) => messageApi.error(translateApiError(t, err)),
    },
  );

  const testConnectionMutation = useApiMutation<
    { providerId: string },
    AuthProviderConnectionTestResult
  >(
    ({ providerId }) =>
      api.POST("/admin/auth-providers/{provider_id}/test-connection", {
        params: { path: { provider_id: providerId } },
      }),
    {
      onSuccess: (resp) => {
        if (resp.success) {
          messageApi.success(resp.message || t("authProviders.test_success"));
        } else {
          messageApi.error(resp.message || t("authProviders.test_failed"));
        }
        setTestingProviderId("");
      },
      onError: (err) => {
        setTestingProviderId("");
        messageApi.error(translateApiError(t, err));
      },
    },
  );

  const syncCohortsMutation = useApiMutation<
    { providerId: string; body: ExternalCohortSyncRequest },
    unknown
  >(
    ({ providerId, body }) =>
      api.POST("/admin/auth-providers/{provider_id}/cohorts/sync", {
        params: { path: { provider_id: providerId } },
        body,
      }),
    {
      invalidateKeys: [
        ["admin-auth-provider-cohorts", mappingProvider?.id ?? ""],
        ["admin-auth-provider-sample", mappingProvider?.id ?? ""],
        ["admin-auth-provider-mappings", mappingProvider?.id ?? ""],
      ],
      onSuccess: () => messageApi.success(t("common:message.success")),
      onError: (err) => messageApi.error(translateApiError(t, err)),
    },
  );

  const createMappingMutation = useApiMutation<
    { providerId: string; body: ExternalCohortMappingCreateRequest },
    ExternalCohortMapping
  >(
    ({ providerId, body }) =>
      api.POST("/admin/auth-providers/{provider_id}/cohort-mappings", {
        params: { path: { provider_id: providerId } },
        body,
      }),
    {
      invalidateKeys: [
        ["admin-auth-provider-mappings", mappingProvider?.id ?? ""],
      ],
      onSuccess: () => {
        messageApi.success(t("common:message.success"));
        mappingForm.resetFields();
        mappingForm.setFieldsValue({ scope_type: "global" });
        setCreateMappingModalOpen(false);
      },
      onError: (err) => messageApi.error(translateApiError(t, err)),
    },
  );

  const updateMappingMutation = useApiMutation<
    {
      providerId: string;
      mappingId: string;
      body: ExternalCohortMappingUpdateRequest;
    },
    ExternalCohortMapping
  >(
    ({ providerId, mappingId, body }) =>
      api.PATCH(
        "/admin/auth-providers/{provider_id}/cohort-mappings/{mapping_id}",
        {
          params: { path: { provider_id: providerId, mapping_id: mappingId } },
          body,
        },
      ),
    {
      invalidateKeys: [
        ["admin-auth-provider-mappings", mappingProvider?.id ?? ""],
      ],
      onSuccess: () => {
        messageApi.success(t("common:message.success"));
        mappingEditForm.resetFields();
        setEditMappingOpen(false);
        setEditingMapping(null);
      },
      onError: (err) => messageApi.error(translateApiError(t, err)),
    },
  );

  const deleteMappingMutation = useApiAction<{
    providerId: string;
    mappingId: string;
  }>(
    ({ providerId, mappingId }) =>
      api.DELETE(
        "/admin/auth-providers/{provider_id}/cohort-mappings/{mapping_id}",
        {
          params: { path: { provider_id: providerId, mapping_id: mappingId } },
        },
      ),
    {
      invalidateKeys: [
        ["admin-auth-provider-mappings", mappingProvider?.id ?? ""],
      ],
      onSuccess: () => messageApi.success(t("common:message.success")),
      onError: (err) => messageApi.error(translateApiError(t, err)),
    },
  );

  const previewDirectoryMutation = useApiMutation<
    { providerId: string; body: DirectorySyncPreviewRequest },
    DirectorySyncPreview
  >(
    ({ providerId, body }) =>
      api.POST("/admin/auth-providers/{provider_id}/directory/preview", {
        params: { path: { provider_id: providerId } },
        body,
      }),
    {
      onSuccess: (resp) => {
        setDirectoryPreview(resp);
      },
      onError: (err) => messageApi.error(translateApiError(t, err)),
    },
  );

  const syncDirectoryMutation = useApiMutation<
    { providerId: string; body: DirectorySyncRequest },
    DirectorySyncStartResponse
  >(
    ({ providerId, body }) =>
      api.POST("/admin/auth-providers/{provider_id}/directory/sync", {
        params: { path: { provider_id: providerId } },
        body,
      }),
    {
      invalidateKeys: [
        ["admin-auth-provider-directory-sync-jobs", mappingProvider?.id ?? ""],
      ],
      onSuccess: () => {
        messageApi.success(t("common:message.success"));
      },
      onError: (err) => messageApi.error(translateApiError(t, err)),
    },
  );

  const providers = useMemo<AuthProvider[]>(
    () => providersQuery.data?.items ?? [],
    [providersQuery.data?.items],
  );
  const providerTypes = useMemo<AuthProviderType[]>(
    () => providerTypesQuery.data?.items ?? [],
    [providerTypesQuery.data?.items],
  );
  const providerTypeOptions = useMemo(
    () =>
      providerTypes.map((item) => ({
        value: item.type,
        label: localizedAuthProviderTypeLabel(
          t,
          item.type,
          item.display_name || item.type,
        ),
      })),
    [providerTypes, t],
  );
  const providerTypeLabelByKey = useMemo<Record<string, string>>(
    () =>
      providerTypes.reduce<Record<string, string>>((acc, item) => {
        acc[item.type] = localizedAuthProviderTypeLabel(
          t,
          item.type,
          item.display_name || item.type,
        );
        return acc;
      }, {}),
    [providerTypes, t],
  );
  const roles = useMemo<Role[]>(
    () => rolesQuery.data?.items ?? [],
    [rolesQuery.data?.items],
  );
  const roleOptions = useMemo(
    () =>
      roles.map((role) => ({
        value: role.id,
        label: role.display_name || role.name,
      })),
    [roles],
  );
  const sampleFields = useMemo(
    () => sampleQuery.data?.fields ?? [],
    [sampleQuery.data?.fields],
  );
  const cohorts = useMemo<ExternalCohort[]>(
    () => cohortsQuery.data?.items ?? [],
    [cohortsQuery.data?.items],
  );
  const mappings = useMemo(
    () => mappingsQuery.data?.items ?? [],
    [mappingsQuery.data?.items],
  );
  const directorySyncJobs = useMemo<DirectorySyncJob[]>(
    () => directorySyncJobsQuery.data?.items ?? [],
    [directorySyncJobsQuery.data?.items],
  );
  const recommendedCohortDefaults = useMemo(
    () =>
      cohortDefaultsForAuthType(
        mappingProvider?.auth_type,
        sampleFields,
        cohorts,
      ),
    [cohorts, mappingProvider?.auth_type, sampleFields],
  );

  useEffect(() => {
    externalAuthSettingsForm.setFieldsValue({
      public_base_url: externalAuthSettingsQuery.data?.public_base_url || "",
    });
  }, [externalAuthSettingsForm, externalAuthSettingsQuery.data?.public_base_url]);

  useEffect(() => {
    if (!createOpen) {
      return;
    }
    createForm.resetFields();
    createForm.setFieldsValue({
      auth_type: providerTypeOptions[0]?.value,
      enabled: true,
      sort_order: 0,
    });
  }, [createForm, createOpen, providerTypeOptions]);

  useEffect(() => {
    if (!editOpen || !editingProvider) {
      return;
    }
    editForm.setFieldsValue(
      buildProviderEditFormValues(editingProvider, providerTypes),
    );
  }, [editForm, editOpen, editingProvider, providerTypes]);

  useEffect(() => {
    if (!createMappingModalOpen) {
      return;
    }
    const defaults = recommendedCohortDefaults;
    mappingForm.resetFields();
    mappingForm.setFieldsValue({
      selected_cohort_ref: undefined,
      cohort_kind: defaults.cohortKind,
      scope_type: "global",
    });
  }, [createMappingModalOpen, mappingForm, recommendedCohortDefaults]);

  useEffect(() => {
    if (!mappingOpen || !mappingProvider?.id) {
      return;
    }
    if (directoryDescriptorQuery.isLoading || directoryScheduleQuery.isLoading) {
      return;
    }
    if (directoryRequestHydratedForRef.current === mappingProvider.id) {
      return;
    }

    const scheduleStatus = directoryScheduleQuery.data as
      | (DirectoryEnrichmentScheduleStatus & {
          provider_request?: Record<string, unknown>;
        })
      | undefined;
    const scheduleProviderRequest =
      scheduleStatus?.provider_request &&
      typeof scheduleStatus.provider_request === "object" &&
      !Array.isArray(scheduleStatus.provider_request)
        ? scheduleStatus.provider_request
        : undefined;

    const initialValues = buildDirectoryRequestFormValues(
      (directoryDescriptorQuery.data?.request_schema ?? null) as ConfigSchema | null,
      scheduleProviderRequest,
    );

    directoryRequestForm.resetFields();
    if (initialValues) {
      directoryRequestForm.setFieldsValue(
        initialValues as unknown as Record<string, { [key: string]: unknown }>,
      );
    }
    directoryRequestHydratedForRef.current = mappingProvider.id;
  }, [
    directoryDescriptorQuery.data?.request_schema,
    directoryDescriptorQuery.isLoading,
    directoryRequestForm,
    directoryScheduleQuery.data,
    directoryScheduleQuery.isLoading,
    mappingOpen,
    mappingProvider?.id,
  ]);

  useEffect(() => {
    if (!editMappingOpen || !editingMapping) {
      return;
    }
    mappingEditForm.setFieldsValue({
      role_id: editingMapping.role_id,
      scope_type: editingMapping.scope_type || "global",
      scope_id: editingMapping.scope_id,
      allowed_environments: editingMapping.allowed_environments as Array<
        "test" | "prod"
      >,
    });
  }, [editMappingOpen, editingMapping, mappingEditForm]);

  const openCreateModal = () => {
    setCreateOpen(true);
  };

  const closeCreateModal = () => {
    setCreateOpen(false);
    createForm.resetFields();
  };

  const submitCreate = async () => {
    const values = await createForm.validateFields();
    const selectedProviderType = providerTypes.find(
      (item) => item.type === values.auth_type,
    );
    let config: Record<string, unknown> | undefined;
    try {
      config = extractConfigObject(
        createForm.getFieldsValue(true) as Record<string, unknown>,
        (selectedProviderType?.config_schema ?? null) as ConfigSchema | null,
      );
    } catch (error) {
      messageApi.error(
        error instanceof Error ? error.message : t("common:message.error"),
      );
      return;
    }

    createMutation.mutate({
      name: values.name,
      auth_type: values.auth_type,
      enabled: values.enabled,
      sort_order: values.sort_order,
      config,
    });
  };

  const openEditModal = (provider: AuthProvider) => {
    setEditingProvider(provider);
    setEditOpen(true);
  };

  const closeEditModal = () => {
    editForm.resetFields();
    setEditOpen(false);
    setEditingProvider(null);
  };

  const submitEdit = async () => {
    if (!editingProvider) {
      return;
    }
    const values = await editForm.validateFields();
    const editProviderType = providerTypes.find(
      (tp) => tp.type === editingProvider.auth_type,
    );
    let config: Record<string, unknown> | undefined;
    try {
      config = extractConfigObject(
        editForm.getFieldsValue(true) as Record<string, unknown>,
        (editProviderType?.config_schema ?? null) as ConfigSchema | null,
      );
    } catch (error) {
      messageApi.error(
        error instanceof Error ? error.message : t("common:message.error"),
      );
      return;
    }

    updateMutation.mutate({
      providerId: editingProvider.id,
      body: {
        name: values.name,
        enabled: values.enabled,
        sort_order: values.sort_order,
        config,
      },
    });
  };

  const openDeleteModal = (provider: AuthProvider) => {
    setDeletingProvider(provider);
    setDeleteOpen(true);
  };

  const closeDeleteModal = () => {
    setDeleteOpen(false);
    setDeletingProvider(null);
  };

  const submitDelete = () => {
    if (!deletingProvider) {
      return;
    }
    deleteMutation.mutate(deletingProvider.id);
  };

  const testConnection = (provider: AuthProvider) => {
    setTestingProviderId(provider.id);
    testConnectionMutation.mutate({ providerId: provider.id });
  };

  const previewDirectory = async () => {
    if (!mappingProvider) {
      return;
    }
    let providerRequest: Record<string, unknown> = {};
    try {
      providerRequest =
        extractConfigObject(
          directoryRequestForm.getFieldsValue(true) as Record<string, unknown>,
          (directoryDescriptorQuery.data?.request_schema ?? null) as ConfigSchema | null,
          "provider_request",
        ) ?? {};
    } catch (error) {
      messageApi.error(
        error instanceof Error ? error.message : t("common:message.error"),
      );
      return;
    }
    previewDirectoryMutation.mutate({
      providerId: mappingProvider.id,
      body: {
        provider_request: providerRequest,
        conflict_resolution: "skip",
      },
    });
  };

  const syncDirectory = async () => {
    if (!mappingProvider) {
      return;
    }
    let providerRequest: Record<string, unknown> = {};
    try {
      providerRequest =
        extractConfigObject(
          directoryRequestForm.getFieldsValue(true) as Record<string, unknown>,
          (directoryDescriptorQuery.data?.request_schema ?? null) as ConfigSchema | null,
          "provider_request",
        ) ?? {};
    } catch (error) {
      messageApi.error(
        error instanceof Error ? error.message : t("common:message.error"),
      );
      return;
    }
    syncDirectoryMutation.mutate({
      providerId: mappingProvider.id,
      body: {
        provider_request: providerRequest,
        conflict_resolution: "skip",
      },
    });
  };

  const openMappingModal = (provider: AuthProvider) => {
    setMappingProvider(provider);
    setSelectedDirectorySyncJobId("");
    setDirectoryPreview(null);
    setMappingOpen(true);
  };

  const closeMappingModal = () => {
    syncForm.resetFields();
    directoryRequestForm.resetFields();
    directoryRequestHydratedForRef.current = "";
    setMappingOpen(false);
    setMappingProvider(null);
    setDirectoryPreview(null);
    setSelectedDirectorySyncJobId("");
    if (editMappingOpen) {
      mappingEditForm.resetFields();
      setEditMappingOpen(false);
      setEditingMapping(null);
    }
    if (createMappingModalOpen) {
      mappingForm.resetFields();
      setCreateMappingModalOpen(false);
    }
  };

  const openCreateMappingModal = () => {
    setCreateMappingModalOpen(true);
  };

  const closeCreateMappingModal = () => {
    mappingForm.resetFields();
    setCreateMappingModalOpen(false);
  };

  const submitSyncCohorts = async () => {
    if (!mappingProvider) {
      return;
    }
    const values = await syncForm.validateFields();
    const cohorts = parseCohortText(values.cohorts_text);
    if (cohorts.length === 0) {
      messageApi.error(t("authProviders.cohorts_required"));
      return;
    }

    syncCohortsMutation.mutate({
      providerId: mappingProvider.id,
      body: {
        cohort_kind: values.cohort_kind.trim(),
        source_field: values.source_field.trim(),
        cohorts,
      },
    });
  };

  const submitCreateMapping = async () => {
    if (!mappingProvider) {
      return;
    }
    const values = await mappingForm.validateFields();
    createMappingMutation.mutate({
      providerId: mappingProvider.id,
      body: {
        cohort_kind: values.cohort_kind.trim(),
        cohort_key: values.cohort_key.trim(),
        cohort_display_name: values.cohort_display_name?.trim() || undefined,
        role_id: values.role_id,
        scope_type: values.scope_type?.trim() || "global",
        scope_id: values.scope_id?.trim() || undefined,
        allowed_environments: values.allowed_environments,
      },
    });
  };

  const openEditMappingModal = (mapping: ExternalCohortMapping) => {
    setEditingMapping(mapping);
    setEditMappingOpen(true);
  };

  const closeEditMappingModal = () => {
    mappingEditForm.resetFields();
    setEditMappingOpen(false);
    setEditingMapping(null);
  };

  const submitEditMapping = async () => {
    if (!mappingProvider || !editingMapping) {
      return;
    }
    const values = await mappingEditForm.validateFields();
    updateMappingMutation.mutate({
      providerId: mappingProvider.id,
      mappingId: editingMapping.id,
      body: {
        role_id: values.role_id,
        scope_type: values.scope_type?.trim() || undefined,
        scope_id: values.scope_id?.trim() || undefined,
        allowed_environments: values.allowed_environments,
      },
    });
  };

  const deleteMapping = (mapping: ExternalCohortMapping) => {
    if (!mappingProvider) {
      return;
    }
    deleteMappingMutation.mutate({
      providerId: mappingProvider.id,
      mappingId: mapping.id,
    });
  };

  const submitExternalAuthSettings = async () => {
    const values = await externalAuthSettingsForm.validateFields();
    updateExternalAuthSettingsMutation.mutate({
      public_base_url: values.public_base_url?.trim() || "",
    });
  };

  const resetExternalAuthSettingsToDeploymentDefault = () => {
    updateExternalAuthSettingsMutation.mutate({
      public_base_url: "",
    });
  };

  const openDirectorySyncJobDetail = (job: DirectorySyncJob) => {
    setSelectedDirectorySyncJobId(job.id);
  };

  const closeDirectorySyncJobDetail = () => {
    setSelectedDirectorySyncJobId("");
  };

  return {
    messageContextHolder,
    providers,
    providersLoading: providersQuery.isLoading,
    refetchProviders: providersQuery.refetch,
    providerTypes,
    providerTypesLoading: providerTypesQuery.isLoading,
    providerTypeOptions,
    providerTypeLabelByKey,

    createOpen,
    editOpen,
    deleteOpen,
    mappingOpen,
    editMappingOpen,
    createMappingModalOpen,
    editingProvider,
    deletingProvider,
    mappingProvider,
    editingMapping,
    testingProviderId,

    createForm,
    editForm,
    syncForm,
    directoryRequestForm,
    mappingForm,
    mappingEditForm,
    externalAuthSettingsForm,

    openCreateModal,
    closeCreateModal,
    submitCreate,
    openEditModal,
    closeEditModal,
    submitEdit,
    openDeleteModal,
    closeDeleteModal,
    submitDelete,
    testConnection,
    previewDirectory,
    syncDirectory,
    openDirectorySyncJobDetail,
    closeDirectorySyncJobDetail,

    openMappingModal,
    closeMappingModal,
    submitSyncCohorts,
    submitCreateMapping,
    openCreateMappingModal,
    closeCreateMappingModal,
    openEditMappingModal,
    closeEditMappingModal,
    submitEditMapping,
    deleteMapping,
    submitExternalAuthSettings,
    resetExternalAuthSettingsToDeploymentDefault,

    sampleFields,
    recommendedCohortDefaults,
    sampleLoading: sampleQuery.isLoading,
    cohorts,
    cohortsLoading: cohortsQuery.isLoading,
    runtimeDescriptor: runtimeDescriptorQuery.data,
    runtimeDescriptorLoading: runtimeDescriptorQuery.isLoading,
    directoryDescriptor: directoryDescriptorQuery.data,
    directoryDescriptorLoading: directoryDescriptorQuery.isLoading,
    directoryDescriptorUnsupported:
      directoryDescriptorQuery.error?.status === 501,
    directoryPreview,
    directorySchedule: directoryScheduleQuery.data,
    directoryScheduleLoading: directoryScheduleQuery.isLoading,
    directorySyncJobs,
    directorySyncJobsLoading: directorySyncJobsQuery.isLoading,
    directorySyncJobDetail: directorySyncJobDetailQuery.data,
    directorySyncJobDetailLoading: directorySyncJobDetailQuery.isLoading,
    selectedDirectorySyncJobId,
    mappings,
    mappingsLoading: mappingsQuery.isLoading,
    externalAuthSettings: externalAuthSettingsQuery.data,
    externalAuthSettingsLoading: externalAuthSettingsQuery.isLoading,
    scopeTargetOptionsByType,
    scopeTargetLoadingByType,
    roleOptions,

    createPending: createMutation.isPending,
    updatePending: updateMutation.isPending,
    deletePending: deleteMutation.isPending,
    testConnectionPending: testConnectionMutation.isPending,
    previewDirectoryPending: previewDirectoryMutation.isPending,
    syncDirectoryPending: syncDirectoryMutation.isPending,
    syncCohortsPending: syncCohortsMutation.isPending,
    createMappingPending: createMappingMutation.isPending,
    updateMappingPending: updateMappingMutation.isPending,
    deleteMappingPending: deleteMappingMutation.isPending,
    updateExternalAuthSettingsPending:
      updateExternalAuthSettingsMutation.isPending,
  };
}
