"use client";

import type { FormInstance } from "antd";
import type { TFunction } from "i18next";
import {
  Alert,
  AutoComplete,
  Button,
  Card,
  Col,
  Collapse,
  Descriptions,
  Drawer,
  Form,
  Input,
  InputNumber,
  Modal,
  Popconfirm,
  Row,
  Select,
  Segmented,
  Space,
  Steps,
  Switch,
  Table,
  Tag,
  Typography,
} from "antd";
import {
  CloudServerOutlined,
  DeleteOutlined,
  EditOutlined,
  KeyOutlined,
  LinkOutlined,
  PlusOutlined,
  ReloadOutlined,
  SafetyOutlined,
  SettingOutlined,
  SyncOutlined,
} from "@ant-design/icons";
import type { ColumnsType } from "antd/es/table";
import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";

import { ActionEmptyState } from "@/components/feedback/ActionEmptyState";
import { SummaryMetricCard } from "@/components/feedback/SummaryMetricCard";
import type { ScopeTargetOption } from "@/features/rbac-shared/useScopeTargetCatalog";
import {
  AccessControlGlyph,
  NotificationInboxGlyph,
  QueueReviewGlyph,
  RateLimitGaugeGlyph,
  RequestsOverviewGlyph,
  ServiceWorkspaceGlyph,
} from "@/components/illustrations/DashboardIllustrations";
import { PageHeader, PageSurface } from "@/components/layouts/PageSection";
import { LocalDateTimeText } from "@/components/ui/LocalDateTimeText";
import { PageSearchToolbar, filterOptionByLabel } from "@/components/ui/PageSearchToolbar";
import { hasPermission } from "@/lib/auth/permissions";
import { useAuthStore } from "@/stores/auth";
import { useAdminAuthProvidersController } from "../hooks/useAdminAuthProvidersController";
import {
  type AuthProvider,
  type AuthProviderRuntimeDescriptor,
  type AuthProviderType,
  type DirectorySyncPreview,
  type DirectorySyncJob,
  type ExternalCohort,
  type ExternalCohortMapping,
} from "../types";
import { SchemaConfigForm } from "./SchemaConfigForm";

const { Text } = Typography;

interface DiscoveredCohortOption {
  value: string;
  label: string;
  cohortKind: string;
  cohortKey: string;
  cohortDisplayName: string;
}

interface LabeledOption {
  value: string;
  label: string;
}

interface ManualCohortSeedCandidate {
  cohortKind: string;
  sourceField: string;
  values: string[];
  uniqueCount: number;
}

function dedupeOptions(options: LabeledOption[]): LabeledOption[] {
  const seen = new Set<string>();
  const unique: LabeledOption[] = [];
  for (const option of options) {
    const value = option.value.trim();
    if (!value || seen.has(value)) {
      continue;
    }
    seen.add(value);
    unique.push({ value, label: option.label });
  }
  return unique;
}

function normalizeFieldToken(value: string): string {
  return value.trim().toLowerCase().replace(/[^a-z0-9]+/g, "_");
}

function inferManualCohortKind(fieldName: string): string | null {
  const normalized = normalizeFieldToken(fieldName);
  if (["department", "departments", "dept"].includes(normalized)) {
    return "department";
  }
  if (["section", "sections", "team", "teams"].includes(normalized)) {
    return "team";
  }
  if (["organization", "organisation", "org", "company"].includes(normalized)) {
    return "organization";
  }
  if (["groups", "group"].includes(normalized)) {
    return "group";
  }
  return null;
}

function localizeProviderProfileFieldLabel(
  t: TFunction<["admin", "common"]>,
  fieldKey: string,
) {
  return t(`users.profile_fields.${fieldKey}`, {
    defaultValue: fieldKey,
  });
}

interface ProviderMappingWorkflowCopy {
  discoveredEmptyTitle: string;
  discoveredEmptyDescription: string;
  sampleEmptyTitle: string;
  sampleEmptyDescription: string;
  manualTitle: string;
  manualDescription: string;
  cohortKindHint: string;
  sourceFieldHint: string;
  cohortsPlaceholder: string;
  guideSteps: string[];
}

type DirectoryPreviewFilter = "all" | "ready" | "warning" | "conflict";
type DirectoryJobFilter = "all" | "create" | "update" | "blocked";

function authProviderTypeLabel(
  authType: string | undefined,
  fallback: string,
  t: (key: string, opts?: Record<string, unknown>) => string,
) {
  if (!authType) {
    return fallback;
  }
  return t(`authProviders.types.${authType}.label`, {
    defaultValue: fallback,
  });
}

function authProviderTypeDescription(
  authType: string | undefined,
  fallback: string | undefined,
  t: (key: string, opts?: Record<string, unknown>) => string,
) {
  if (!authType) {
    return fallback || "";
  }
  return t(`authProviders.types.${authType}.description`, {
    defaultValue: fallback || "",
  });
}

function renderAuthProviderAlphaTag(
  t: (key: string, opts?: Record<string, unknown>) => string,
) {
  return (
    <Tag color="gold" style={{ marginInlineEnd: 0 }}>
      {t("authProviders.alpha_badge", {
        defaultValue: "Alpha",
      })}
    </Tag>
  );
}

function authProviderRuntimeModeLabel(
  authType: string | undefined,
  modeKey: string | undefined,
  fallback: string,
  t: (key: string, opts?: Record<string, unknown>) => string,
) {
  if (!authType || !modeKey) {
    return fallback;
  }
  return t(`authProviders.runtime.modes.${authType}.${modeKey}.label`, {
    defaultValue: fallback,
  });
}

function authProviderRuntimeModeDescription(
  authType: string | undefined,
  modeKey: string | undefined,
  fallback: string | undefined,
  t: (key: string, opts?: Record<string, unknown>) => string,
) {
  if (!authType || !modeKey) {
    return fallback || "";
  }
  return t(`authProviders.runtime.modes.${authType}.${modeKey}.description`, {
    defaultValue: fallback || "",
  });
}

function authProviderDirectoryDescription(
  authType: string | undefined,
  fallback: string | undefined,
  t: (key: string, opts?: Record<string, unknown>) => string,
) {
  if (!authType) {
    return fallback || "";
  }
  return t(`authProviders.directory.providers.${authType}.description`, {
    defaultValue: fallback || "",
  });
}

function getProviderMappingWorkflowCopy(
  authType: string | undefined,
  t: (key: string, opts?: Record<string, unknown>) => string,
): ProviderMappingWorkflowCopy {
  void authType;
  return {
    discoveredEmptyTitle: t("authProviders.discovered.empty_title", {
      defaultValue: "No discovered cohorts yet",
    }),
    discoveredEmptyDescription: t(
      "authProviders.discovered.empty_description",
      {
        defaultValue:
          "Let one external user complete a real login first. The platform will automatically discover external cohorts from that login.",
      },
    ),
    sampleEmptyTitle: t("authProviders.sample.empty", {
      defaultValue: "No sample attributes yet",
    }),
    sampleEmptyDescription: t("authProviders.sample.empty_description", {
      defaultValue:
        "Run a connection test after signing in with this provider to inspect the incoming claims.",
    }),
    manualTitle: t("authProviders.sync.manual_title", {
      defaultValue: "Manually register known cohorts",
    }),
    manualDescription: t("authProviders.sync.manual_description", {
      defaultValue:
        "Normal path: let one user log in and use discovered cohorts. Only fill this section when you need to pre-create mappings before the first login.",
    }),
    cohortKindHint: t("authProviders.sync.cohort_kind_hint", {
      defaultValue:
        "Choose the canonical cohort kind that your provider projects into the platform.",
    }),
    sourceFieldHint: t("authProviders.sync.source_field_hint", {
      defaultValue:
        "Use the raw provider field name that produces this cohort set.",
    }),
    cohortsPlaceholder: t("authProviders.sync.cohorts_placeholder", {
      defaultValue: "ops-team\nplatform-admin",
    }),
    guideSteps: [
      t("authProviders.guide.default.step_1", {
        defaultValue: "Validate provider connectivity.",
      }),
      t("authProviders.guide.default.step_2", {
        defaultValue: "Have one real external user complete login once.",
      }),
      t("authProviders.guide.default.step_3", {
        defaultValue:
          "Review discovered cohorts and choose one when creating the mapping.",
      }),
      t("authProviders.guide.default.step_4", {
        defaultValue:
          "Use manual cohort registration only when you need a mapping before the first real login.",
      }),
    ],
  };
}

function directorySyncModeLabel(
  value: DirectorySyncJob["sync_mode"],
  t: (key: string, opts?: Record<string, unknown>) => string,
) {
  switch (value) {
    case "scheduled_enrichment":
      return t("authProviders.jobs.mode.scheduled_enrichment", {
        defaultValue: "Scheduled enrichment",
      });
    case "manual_import":
      return t("authProviders.jobs.mode.manual_import", {
        defaultValue: "Manual import",
      });
    default:
      return "—";
  }
}

function directorySyncStatusColor(value: DirectorySyncJob["status"]) {
  switch (value) {
    case "completed":
      return "green";
    case "failed":
      return "red";
    case "running":
      return "processing";
    case "pending":
    default:
      return "default";
  }
}

function directoryJobMatchesFilter(
  job: DirectorySyncJob,
  filter: DirectoryJobFilter,
) {
  switch (filter) {
    case "create":
      return job.result_summary.create_count > 0;
    case "update":
      return job.result_summary.update_count > 0;
    case "blocked":
      return job.result_summary.blocked_count > 0;
    case "all":
    default:
      return true;
  }
}

function formatJsonObject(value: Record<string, unknown> | undefined) {
  if (!value || Object.keys(value).length === 0) {
    return "{}";
  }
  return JSON.stringify(value, null, 2);
}

function directoryPreviewOutcome(
  item: NonNullable<DirectorySyncPreview["items"]>[number],
) {
  if (item.match.action === "blocked") {
    return "conflict" as const;
  }
  if (item.warnings && item.warnings.length > 0) {
    return "warning" as const;
  }
  return "ready" as const;
}

function directoryPreviewOutcomeColor(
  value: ReturnType<typeof directoryPreviewOutcome>,
) {
  switch (value) {
    case "conflict":
      return "orange";
    case "warning":
      return "gold";
    case "ready":
    default:
      return "green";
  }
}

function directoryPreviewOutcomeLabel(
  value: ReturnType<typeof directoryPreviewOutcome>,
  t: (key: string, opts?: Record<string, unknown>) => string,
) {
  switch (value) {
    case "conflict":
      return t("authProviders.directory.outcome.conflict", {
        defaultValue: "Conflict",
      });
    case "warning":
      return t("authProviders.directory.outcome.warning", {
        defaultValue: "Warning",
      });
    case "ready":
    default:
      return t("authProviders.directory.outcome.ready", {
        defaultValue: "Ready",
      });
  }
}

function directoryConflictCodeLabel(
  code: string,
  t: (key: string, opts?: Record<string, unknown>) => string,
) {
  switch (code) {
    case "same_external_identity":
      return t("authProviders.directory.conflict_code.same_external_identity", {
        defaultValue: "Same external identity",
      });
    case "same_canonical_identity":
      return t("authProviders.directory.conflict_code.same_canonical_identity", {
        defaultValue: "Same canonical identity",
      });
    case "username_conflict":
      return t("authProviders.directory.conflict_code.username_conflict", {
        defaultValue: "Username conflict",
      });
    case "email_conflict":
      return t("authProviders.directory.conflict_code.email_conflict", {
        defaultValue: "Email conflict",
      });
    case "ambiguous_existing_user":
      return t(
        "authProviders.directory.conflict_code.ambiguous_existing_user",
        {
          defaultValue: "Ambiguous existing user",
        },
      );
    default:
      return code;
  }
}

function directoryPreviewActionLabel(
  value: NonNullable<DirectorySyncPreview["items"]>[number]["match"]["action"],
  t: (key: string, opts?: Record<string, unknown>) => string,
) {
  switch (value) {
    case "update":
      return t("authProviders.directory.action.update", {
        defaultValue: "Update existing user",
      });
    case "blocked":
      return t("authProviders.directory.action.blocked", {
        defaultValue: "Blocked by conflicts",
      });
    case "create":
    default:
      return t("authProviders.directory.action.create", {
        defaultValue: "Create new user",
      });
  }
}

export function AdminAuthProvidersContent() {
  const { t } = useTranslation(["admin", "common"]);
  const currentUser = useAuthStore((state) => state.user);
  const canCreateAuthProviders = hasPermission(currentUser, "auth_provider:configure");
  const canUpdateAuthProviders = hasPermission(currentUser, "auth_provider:update");
  const canDeleteAuthProviders = hasPermission(currentUser, "auth_provider:delete");
  const canTestAuthProviders = hasPermission(currentUser, "auth_provider:configure");
  const canSyncAuthProviders = hasPermission(currentUser, "auth_provider:sync");
  const canCreateAuthProviderMappings = hasPermission(currentUser, "auth_provider:mapping_create");
  const canUpdateAuthProviderMappings = hasPermission(currentUser, "auth_provider:mapping_update");
  const canDeleteAuthProviderMappings = hasPermission(currentUser, "auth_provider:mapping_delete");
  const providers = useAdminAuthProvidersController({ t });
  const [directoryPreviewFilter, setDirectoryPreviewFilter] =
    useState<DirectoryPreviewFilter>("all");
  const [directoryJobFilter, setDirectoryJobFilter] =
    useState<DirectoryJobFilter>("all");
  const [quickSearch, setQuickSearch] = useState("");
  const [quickSearchDraft, setQuickSearchDraft] = useState("");
  const [filtersOpen, setFiltersOpen] = useState(false);
  const [providerNameFilter, setProviderNameFilter] = useState("");
  const [providerNameFilterDraft, setProviderNameFilterDraft] = useState("");
  const [authTypeFilter, setAuthTypeFilter] = useState("");
  const [authTypeFilterDraft, setAuthTypeFilterDraft] = useState("");
  const [statusFilter, setStatusFilter] =
    useState<"all" | "enabled" | "disabled">("all");
  const [statusFilterDraft, setStatusFilterDraft] =
    useState<"all" | "enabled" | "disabled">("all");
  const providerTypeMetadataByType = useMemo(
    () =>
      Object.fromEntries(
        (providers.providerTypes ?? []).map((item) => [item.type, item] as const),
      ),
    [providers.providerTypes],
  );
  const allProviderItems = useMemo(
    () => providers.providers ?? [],
    [providers.providers],
  );
  const providerItems = useMemo(() => {
    const query = quickSearch.trim().toLowerCase();
    return allProviderItems.filter((provider) => {
      if (providerNameFilter && provider.name !== providerNameFilter) {
        return false;
      }
      if (authTypeFilter && provider.auth_type !== authTypeFilter) {
        return false;
      }
      if (statusFilter === "enabled" && !provider.enabled) {
        return false;
      }
      if (statusFilter === "disabled" && provider.enabled) {
        return false;
      }
      if (!query) {
        return true;
      }
      const providerTypeMetadata = providerTypeMetadataByType[provider.auth_type];
      const displayName =
        providers.providerTypeLabelByKey[provider.auth_type] ??
        providerTypeMetadata?.display_name ??
        provider.auth_type;
      const description = providerTypeMetadata?.description ?? "";
      return [provider.id, provider.name, provider.auth_type, displayName, description]
        .filter(Boolean)
        .some((value) => String(value).toLowerCase().includes(query));
    });
  }, [
    allProviderItems,
    authTypeFilter,
    providerNameFilter,
    providerTypeMetadataByType,
    providers.providerTypeLabelByKey,
    quickSearch,
    statusFilter,
  ]);
  const providerNameOptions = useMemo(
    () =>
      Array.from(new Set(allProviderItems.map((provider) => provider.name).filter(Boolean)))
        .sort((left, right) => left.localeCompare(right))
        .map((name) => ({
          value: name,
          label: name,
        })),
    [allProviderItems],
  );
  const createForm = providers.createForm;
  const editForm = providers.editForm;
  const selectedManualSourceField = Form.useWatch(
    "source_field",
    providers.syncForm,
  ) as string | undefined;
  const filteredDirectorySyncJobs = useMemo(
    () =>
      (providers.directorySyncJobs ?? []).filter((job) =>
        directoryJobMatchesFilter(job, directoryJobFilter),
      ),
    [directoryJobFilter, providers.directorySyncJobs],
  );
  const latestDirectorySyncJobByAction = useMemo(
    () => ({
      create:
        (providers.directorySyncJobs ?? []).find(
          (job) => job.result_summary.create_count > 0,
        ) ?? null,
      update:
        (providers.directorySyncJobs ?? []).find(
          (job) => job.result_summary.update_count > 0,
        ) ?? null,
      blocked:
        (providers.directorySyncJobs ?? []).find(
          (job) => job.result_summary.blocked_count > 0,
        ) ?? null,
    }),
    [providers.directorySyncJobs],
  );
  const latestDirectorySyncJob = providers.directorySyncJobs?.[0] ?? null;
  const renderDirectorySummaryJobActionButton = (
    job: DirectorySyncJob | null,
    label: string,
    testId: string,
  ) =>
    job ? (
      <Button
        key={testId}
        type="link"
        size="small"
        icon={<LinkOutlined />}
        onClick={() => providers.openDirectorySyncJobDetail(job)}
        data-testid={testId}
      >
        {label}
      </Button>
    ) : null;
  const readyDirectorySummaryActions = [
    renderDirectorySummaryJobActionButton(
      latestDirectorySyncJobByAction.create,
      t("authProviders.directory.summary.ready_latest_create", {
        defaultValue: "Latest create job",
      }),
      "directory-preview-ready-latest-create-job",
    ),
    renderDirectorySummaryJobActionButton(
      latestDirectorySyncJobByAction.update,
      t("authProviders.directory.summary.ready_latest_update", {
        defaultValue: "Latest update job",
      }),
      "directory-preview-ready-latest-update-job",
    ),
  ].filter(Boolean);
  const scopeOptions = useMemo(
    () => [
      { value: "global", label: t("rbac.scope.global") },
      { value: "system", label: t("rbac.scope.system") },
      { value: "service", label: t("rbac.scope.service") },
      { value: "vm", label: t("rbac.scope.vm") },
    ],
    [t],
  );
  const providerSummary = useMemo(() => {
    const enabledCount = providerItems.filter(
      (provider) => provider.enabled,
    ).length;
    const distinctTypes = new Set(
      providerItems.map((provider) => provider.auth_type).filter(Boolean),
    ).size;
    return {
      totalCount: providerItems.length,
      enabledCount,
      disabledCount: providerItems.length - enabledCount,
      distinctTypes,
    };
  }, [providerItems]);
  const environmentOptions = useMemo(
    () => [
      { value: "test", label: t("authProviders.env.test") },
      { value: "prod", label: t("authProviders.env.prod") },
    ],
    [t],
  );
  const roleLabelById = useMemo(
    () =>
      Object.fromEntries(
        (providers.roleOptions ?? []).map((option) => [
          String(option.value),
          String(option.label),
        ]),
      ),
    [providers.roleOptions],
  );
  const externalAuthSettings = providers.externalAuthSettings;
  const externalAuthRuntimeReady =
    externalAuthSettings?.runtime_login_ready ?? false;
  const externalAuthSettingSource = externalAuthSettings?.source ?? "unset";
  const externalAuthEffectiveBaseURL =
    externalAuthSettings?.effective_public_base_url || "";
  const externalAuthConfiguredBaseURL =
    externalAuthSettings?.public_base_url || "";
  const runtimeDescriptor = providers.runtimeDescriptor;
  const runtimeSupported = runtimeDescriptor?.supported ?? false;
  const runtimeReady = runtimeSupported
    ? !runtimeDescriptor?.requires_public_base_url || externalAuthRuntimeReady
    : false;
  const directoryDescriptor = providers.directoryDescriptor;
  const directoryPreview = providers.directoryPreview;
  const directorySupported =
    !providers.directoryDescriptorUnsupported && !!directoryDescriptor;
  const directoryPreviewSummary = useMemo(() => {
    const items = directoryPreview?.items ?? [];
    return items.reduce(
      (acc, item) => {
        const outcome = directoryPreviewOutcome(item);
        acc.total += 1;
        acc[outcome] += 1;
        return acc;
      },
      { total: 0, ready: 0, warning: 0, conflict: 0 },
    );
  }, [directoryPreview]);
  const directoryPreviewConflictCodes = useMemo(() => {
    const counts = new Map<string, number>();
    for (const item of directoryPreview?.items ?? []) {
      if (item.match.action !== "blocked") {
        continue;
      }
      for (const conflict of item.conflicts ?? []) {
        counts.set(conflict.code, (counts.get(conflict.code) ?? 0) + 1);
      }
    }
    return Array.from(counts.entries())
      .map(([code, count]) => ({ code, count }))
      .sort((a, b) => b.count - a.count || a.code.localeCompare(b.code));
  }, [directoryPreview]);
  const directoryPreviewConflictDescriptionItems = useMemo(
    () =>
      directoryPreviewConflictCodes.map((item) => ({
        key: item.code,
        label: directoryConflictCodeLabel(item.code, t),
        children: (
          <Space size={6} wrap={true}>
            <Tag color="orange">{item.count}</Tag>
            <Text type="secondary" style={{ fontSize: 13 }}>
              {item.code}
            </Text>
          </Space>
        ),
      })),
    [directoryPreviewConflictCodes, t],
  );
  const filteredDirectoryPreviewItems = useMemo(() => {
    if (!directoryPreview) {
      return [];
    }
    if (directoryPreviewFilter === "all") {
      return directoryPreview.items;
    }
    return directoryPreview.items.filter(
      (item) => directoryPreviewOutcome(item) === directoryPreviewFilter,
    );
  }, [directoryPreview, directoryPreviewFilter]);
  const filteredDirectoryPreviewConflictGroups = useMemo(() => {
    const grouped = new Map<
      string,
      Array<NonNullable<DirectorySyncPreview["items"]>[number]>
    >();
    for (const item of filteredDirectoryPreviewItems) {
      for (const conflict of item.conflicts ?? []) {
        const existing = grouped.get(conflict.code) ?? [];
        existing.push(item);
        grouped.set(conflict.code, existing);
      }
    }
    return Array.from(grouped.entries())
      .map(([code, items]) => ({ code, items }))
      .sort((a, b) => b.items.length - a.items.length || a.code.localeCompare(b.code));
  }, [filteredDirectoryPreviewItems]);
  const filteredDirectoryPreviewWarningGroups = useMemo(() => {
    const grouped = new Map<
      string,
      Array<NonNullable<DirectorySyncPreview["items"]>[number]>
    >();
    for (const item of filteredDirectoryPreviewItems) {
      for (const warning of item.warnings ?? []) {
        const existing = grouped.get(warning) ?? [];
        existing.push(item);
        grouped.set(warning, existing);
      }
    }
    return Array.from(grouped.entries())
      .map(([warning, items]) => ({ warning, items }))
      .sort(
        (a, b) => b.items.length - a.items.length || a.warning.localeCompare(b.warning),
      );
  }, [filteredDirectoryPreviewItems]);
  const discoveredCohortOptions = useMemo<DiscoveredCohortOption[]>(
    () =>
      (providers.cohorts ?? []).map((cohort: ExternalCohort) => ({
        value: `${cohort.cohort_kind}|${cohort.cohort_key}`,
        label: `${cohort.display_name || cohort.cohort_key} (${cohort.cohort_kind}:${cohort.cohort_key})`,
        cohortKind: cohort.cohort_kind,
        cohortKey: cohort.cohort_key,
        cohortDisplayName: cohort.display_name || cohort.cohort_key,
      })),
    [providers.cohorts],
  );
  const manualCohortKindOptions = useMemo<LabeledOption[]>(
    () =>
      dedupeOptions([
        {
          value: "group",
          label: t("authProviders.sync.cohort_kind_option.group", {
            defaultValue: "Group",
          }),
        },
        {
          value: "department",
          label: t("authProviders.sync.cohort_kind_option.department", {
            defaultValue: "Department",
          }),
        },
        {
          value: "team",
          label: t("authProviders.sync.cohort_kind_option.team", {
            defaultValue: "Team",
          }),
        },
        {
          value: "organization",
          label: t("authProviders.sync.cohort_kind_option.organization", {
            defaultValue: "Organization",
          }),
        },
        ...((providers.cohorts ?? []).map((cohort: ExternalCohort) => ({
          value: cohort.cohort_kind,
          label: cohort.cohort_kind,
        })) ?? []),
      ]),
    [providers.cohorts, t],
  );
  const manualSourceFieldOptions = useMemo<LabeledOption[]>(
    () =>
      dedupeOptions([
        ...((providers.sampleFields ?? []).map((field) => ({
          value: field.field,
          label:
            field.sample && field.sample.length > 0
              ? `${localizeProviderProfileFieldLabel(t, field.field)} (${field.value_type}; ${field.unique_count} distinct; ${field.sample
                  .slice(0, 3)
                  .join(", ")})`
              : `${localizeProviderProfileFieldLabel(t, field.field)} (${field.value_type}; ${field.unique_count} distinct)`,
        })) ?? []),
        ...((providers.cohorts ?? [])
          .filter((cohort: ExternalCohort) => Boolean(cohort.source_field))
          .map((cohort: ExternalCohort) => ({
            value: cohort.source_field as string,
            label: `${localizeProviderProfileFieldLabel(t, cohort.source_field as string)} (${cohort.cohort_kind})`,
          })) ?? []),
      ]),
    [providers.cohorts, providers.sampleFields, t],
  );
  const manualCohortSeedCandidates = useMemo<ManualCohortSeedCandidate[]>(
    () =>
      (providers.sampleFields ?? [])
        .map((field): ManualCohortSeedCandidate | null => {
          if (field.value_type !== "string" || !field.sample || field.sample.length === 0) {
            return null;
          }
          const cohortKind = inferManualCohortKind(field.field);
          if (!cohortKind) {
            return null;
          }
          return {
            cohortKind,
            sourceField: field.field,
            values: Array.from(
              new Set(
                field.sample
                  .map((value) => value.trim())
                  .filter((value) => value.length > 0),
              ),
            ),
            uniqueCount: field.unique_count,
          };
        })
        .filter((candidate): candidate is ManualCohortSeedCandidate => {
          return candidate !== null && candidate.values.length > 0;
        }),
    [providers.sampleFields],
  );
  const activeManualCohortSeedCandidate = useMemo(() => {
    if (selectedManualSourceField) {
      const matched = manualCohortSeedCandidates.find(
        (candidate) => candidate.sourceField === selectedManualSourceField,
      );
      if (matched) {
        return matched;
      }
    }
    if (!providers.recommendedCohortDefaults) {
      return null;
    }
    return (
      manualCohortSeedCandidates.find(
        (candidate) =>
          candidate.sourceField === providers.recommendedCohortDefaults?.sourceField,
      ) ?? null
    );
  }, [
    manualCohortSeedCandidates,
    providers.recommendedCohortDefaults,
    selectedManualSourceField,
  ]);
  const manualKnownCohortOptions = useMemo<LabeledOption[]>(
    () =>
      dedupeOptions([
        ...(providers.cohorts ?? []).map((cohort: ExternalCohort) => ({
          value: cohort.cohort_key,
          label: `${cohort.display_name || cohort.cohort_key} (${cohort.cohort_kind})`,
        })),
        ...((activeManualCohortSeedCandidate?.values ?? []).map((value) => ({
          value,
          label: `${value} (${activeManualCohortSeedCandidate?.cohortKind}; ${t(
            "authProviders.sync.sample_value_label",
            { defaultValue: "sample value" },
          )})`,
        })) ?? []),
      ]),
    [activeManualCohortSeedCandidate, providers.cohorts, t],
  );
  const mappingWorkflow = useMemo(
    () =>
      getProviderMappingWorkflowCopy(providers.mappingProvider?.auth_type, t),
    [providers.mappingProvider?.auth_type, t],
  );
  const recommendedCohortCopy = useMemo(() => {
    const recommended = providers.recommendedCohortDefaults;
    if (!recommended) {
      return null;
    }

    const reasonText =
      recommended.reason === "sample_department"
        ? t("authProviders.sync.recommended_reason.sample_department", {
            defaultValue:
              'Observed sample field "department", so department → department is the best starting point.',
          })
        : recommended.reason === "sample_section"
          ? t("authProviders.sync.recommended_reason.sample_section", {
              defaultValue:
                'Observed sample field "section", so team → section is the best starting point.',
            })
          : recommended.reason === "sample_groups"
            ? t("authProviders.sync.recommended_reason.sample_groups", {
                defaultValue:
                  'Observed sample field "groups", so group → groups is the best starting point.',
              })
            : recommended.reason === "sample_organization"
              ? t("authProviders.sync.recommended_reason.sample_organization", {
                  defaultValue:
                    'Observed sample field "organization", so organization → organization is the best starting point.',
                })
              : recommended.reason === "discovered_cohort"
                ? t("authProviders.sync.recommended_reason.discovered_cohort", {
                    defaultValue:
                      "Existing discovered cohorts already indicate the right organization type and source field.",
                  })
                : t("authProviders.sync.recommended_reason.fallback_groups", {
                    defaultValue:
                      "No stronger signal is available yet, so the generic group → groups fallback is selected.",
                  });

    return {
      title: t("authProviders.sync.recommended_title", {
        defaultValue: "Recommended starting point",
      }),
      description: t("authProviders.sync.recommended_description", {
        defaultValue:
          'Start with external organization type "{{cohortKind}}" and source field "{{sourceField}}". {{reason}}',
        cohortKind: recommended.cohortKind,
        sourceField: recommended.sourceField,
        reason: reasonText,
      }),
    };
  }, [providers.recommendedCohortDefaults, t]);
  const runtimeModeColumns: ColumnsType<
    NonNullable<AuthProviderRuntimeDescriptor["login_modes"]>[number]
  > = [
    {
      title: t("authProviders.runtime.mode", {
        defaultValue: "Login mode",
      }),
      key: "display_name",
      render: (_, mode) => (
        <Space direction="vertical" size={0}>
          <Text strong>
            {authProviderRuntimeModeLabel(
              providers.mappingProvider?.auth_type,
              mode.key,
              mode.display_name,
              t,
            )}
          </Text>
          {authProviderRuntimeModeDescription(
            providers.mappingProvider?.auth_type,
            mode.key,
            mode.description,
            t,
          ) ? (
            <Text type="secondary" style={{ fontSize: 13 }}>
              {authProviderRuntimeModeDescription(
                providers.mappingProvider?.auth_type,
                mode.key,
                mode.description,
                t,
              )}
            </Text>
          ) : null}
        </Space>
      ),
    },
    {
      title: t("authProviders.runtime.interaction", {
        defaultValue: "Interaction",
      }),
      dataIndex: "interaction",
      key: "interaction",
      width: 160,
      render: (value?: "redirect" | "credentials") => (
        <Tag color={value === "credentials" ? "purple" : "blue"}>
          {value === "credentials"
            ? t("authProviders.runtime.interaction_credentials", {
                defaultValue: "Credentials",
              })
            : t("authProviders.runtime.interaction_redirect", {
                defaultValue: "Redirect",
              })}
        </Tag>
      ),
    },
    {
      title: t("common:table.default", {
        defaultValue: "Default",
      }),
      dataIndex: "default",
      key: "default",
      width: 120,
      render: (value?: boolean) =>
        value ? (
          <Tag color="green">
            {t("common:status.enabled", { defaultValue: "Default" })}
          </Tag>
        ) : (
          <Text type="secondary">—</Text>
        ),
    },
  ];
  const discoveredCohortColumns: ColumnsType<ExternalCohort> = [
    {
      title: t("authProviders.discovered.cohort", {
        defaultValue: "Discovered cohort",
      }),
      key: "cohort",
      render: (_, cohort) => (
        <Space direction="vertical" size={0}>
          <Text strong>{cohort.display_name || cohort.cohort_key}</Text>
          <Text type="secondary" style={{ fontSize: 13 }}>
            {cohort.cohort_kind}:{cohort.cohort_key}
          </Text>
        </Space>
      ),
    },
    {
      title: t("authProviders.discovered.source_field", {
        defaultValue: "Source field",
      }),
      dataIndex: "source_field",
      key: "source_field",
      width: 160,
      render: (value?: string) => value || "—",
    },
    {
      title: t("common:table.updated_at", { defaultValue: "Updated" }),
      key: "updated_at",
      width: 180,
      render: (_, cohort) => (
        <LocalDateTimeText value={cohort.last_synced_at || cohort.created_at} />
      ),
    },
  ];
  const directoryPreviewColumns: ColumnsType<
    NonNullable<DirectorySyncPreview["items"]>[number]
  > = [
    {
      title: t("authProviders.directory.result", {
        defaultValue: "Result",
      }),
      key: "result",
      width: 120,
      render: (_, item) => {
        const outcome = directoryPreviewOutcome(item);
        return (
          <Space direction="vertical" size={0}>
            <Tag color={directoryPreviewOutcomeColor(outcome)}>
              {directoryPreviewOutcomeLabel(outcome, t)}
            </Tag>
            <Text type="secondary" style={{ fontSize: 13 }}>
              {directoryPreviewActionLabel(item.match.action, t)}
            </Text>
          </Space>
        );
      },
    },
    {
      title: t("authProviders.directory.record", {
        defaultValue: "Canonical record",
      }),
      key: "record",
      render: (_, item) => (
        <Space direction="vertical" size={0}>
          <Text strong>{item.record.display_name}</Text>
          <Text type="secondary" style={{ fontSize: 13 }}>
            {item.record.username}
            {item.record.email ? ` · ${item.record.email}` : ""}
          </Text>
          <Text type="secondary" style={{ fontSize: 13 }}>
            external_id: {item.record.external_id}
          </Text>
          {item.match.action === "update" && item.match.existing_user_id ? (
            <Text type="secondary" style={{ fontSize: 13 }}>
              {t("authProviders.directory.matched_existing_user", {
                defaultValue: "Matched existing user: {{id}} via {{matchedBy}}",
                id: item.match.existing_user_id,
                matchedBy: item.match.matched_by || "external_id",
              })}
            </Text>
          ) : null}
        </Space>
      ),
    },
    {
      title: t("authProviders.directory.cohorts", {
        defaultValue: "Cohorts",
      }),
      key: "cohorts",
      render: (_, item) =>
        item.record.cohorts && item.record.cohorts.length > 0 ? (
          <Space size={4} wrap={true}>
            {item.record.cohorts.map((cohort) => (
              <Tag key={`${cohort.kind}:${cohort.key}`}>
                {cohort.display_name || cohort.key}
              </Tag>
            ))}
          </Space>
        ) : (
          <Text type="secondary">—</Text>
        ),
    },
    {
      title: t("authProviders.directory.conflicts", {
        defaultValue: "Conflicts",
      }),
      key: "conflicts",
      width: 220,
      render: (_, item) =>
        item.conflicts && item.conflicts.length > 0 ? (
          <Space size={4} wrap={true}>
            {item.conflicts.map((conflict, index) => (
              <Tag color="orange" key={`${conflict.code}-${index}`}>
                {conflict.code}
              </Tag>
            ))}
          </Space>
        ) : (
          <Tag color="green">
            {t("authProviders.directory.no_conflicts", {
              defaultValue: "No conflicts",
            })}
          </Tag>
        ),
    },
    {
      title: t("authProviders.directory.warnings", {
        defaultValue: "Warnings",
      }),
      key: "warnings",
      render: (_, item) =>
        item.warnings && item.warnings.length > 0 ? (
          <Space direction="vertical" size={0}>
            {item.warnings.map((warning, index) => (
              <Text key={`${warning}-${index}`} type="secondary">
                {warning}
              </Text>
            ))}
          </Space>
        ) : (
          <Text type="secondary">—</Text>
        ),
    },
  ];
  const directorySyncJobColumns: ColumnsType<DirectorySyncJob> = [
    {
      title: t("authProviders.jobs.mode", {
        defaultValue: "Mode",
      }),
      dataIndex: "sync_mode",
      key: "sync_mode",
      width: 160,
      render: (value: DirectorySyncJob["sync_mode"]) => (
        <Tag color={value === "scheduled_enrichment" ? "purple" : "blue"}>
          {directorySyncModeLabel(value, t)}
        </Tag>
      ),
    },
    {
      title: t("common:table.status"),
      dataIndex: "status",
      key: "status",
      width: 120,
      render: (value: DirectorySyncJob["status"]) => (
        <Tag color={directorySyncStatusColor(value)}>{value}</Tag>
      ),
    },
    {
      title: t("authProviders.jobs.join_key", {
        defaultValue: "Join key",
      }),
      dataIndex: "join_key_type",
      key: "join_key_type",
      width: 120,
      render: (value: DirectorySyncJob["join_key_type"]) =>
        value ? <Tag>{value}</Tag> : <Text type="secondary">—</Text>,
    },
    {
      title: t("authProviders.jobs.summary", {
        defaultValue: "Summary",
      }),
      key: "summary",
      render: (_, record) => (
        <Space size={4} wrap={true}>
          <Text type="secondary">
            {t("authProviders.jobs.total_entries", {
              defaultValue: "{{count}} total",
              count: record.total_entries,
            })}
          </Text>
          <Tag color="green">
            {record.result_summary.create_count}{" "}
            {directoryPreviewActionLabel("create", t)}
          </Tag>
          <Tag color="blue">
            {record.result_summary.update_count}{" "}
            {directoryPreviewActionLabel("update", t)}
          </Tag>
          <Tag color="orange">
            {record.result_summary.blocked_count}{" "}
            {directoryPreviewActionLabel("blocked", t)}
          </Tag>
          {record.error_count > 0 ? (
            <Tag color="red">
              {t("authProviders.jobs.error_count", {
                defaultValue: "{{count}} errors",
                count: record.error_count,
              })}
            </Tag>
          ) : null}
        </Space>
      ),
    },
    {
      title: t("authProviders.jobs.created_at", {
        defaultValue: "Created",
      }),
      dataIndex: "created_at",
      key: "created_at",
      width: 180,
      render: (value: string) =>
        value ? (
          <LocalDateTimeText value={value} />
        ) : (
          <Text type="secondary">—</Text>
        ),
    },
    {
      title: t("authProviders.jobs.completed_at", {
        defaultValue: "Completed",
      }),
      dataIndex: "completed_at",
      key: "completed_at",
      width: 180,
      render: (value?: string) =>
        value ? (
          <LocalDateTimeText value={value} />
        ) : (
          <Text type="secondary">—</Text>
        ),
    },
    {
      title: t("common:table.actions"),
      key: "actions",
      width: 120,
      render: (_, record) => (
        <Space size={4} wrap className="copy-friendly-actions">
          <Button
            type="link"
            size="small"
            onClick={() => providers.openDirectorySyncJobDetail(record)}
          >
            {t("common:button.detail", {
              defaultValue: "Details",
            })}
          </Button>
        </Space>
      ),
    },
  ];
  const columns: ColumnsType<AuthProvider> = [
    {
      title: t("authProviders.table.provider", { defaultValue: "Provider" }),
      dataIndex: "name",
      key: "name",
      render: (name: string, record: AuthProvider) => (
        <Space direction="vertical" size={2}>
          <Text strong>{name}</Text>
          <Space size={6} wrap>
            <Tag color="processing">
              {providers.providerTypeLabelByKey[record.auth_type] ??
                record.auth_type}
            </Tag>
            {renderAuthProviderAlphaTag(t)}
            <Tag color={record.enabled ? "green" : "default"}>
              {record.enabled
                ? t("users.status.enabled")
                : t("users.status.disabled")}
            </Tag>
          </Space>
          <Text type="secondary" style={{ fontSize: 13 }}>
            {t("authProviders.table.provider_id", {
              defaultValue: "Provider ID",
            })}
            : {record.id}
          </Text>
        </Space>
      ),
    },
    {
      title: t("authProviders.table.integration", {
        defaultValue: "Integration",
      }),
      dataIndex: "auth_type",
      key: "auth_type",
      width: 200,
      render: (authType: string) => (
        <Space direction="vertical" size={0}>
          <Space size={6} wrap>
            <Text strong>
              {providers.providerTypeLabelByKey[authType] ?? authType}
            </Text>
            {renderAuthProviderAlphaTag(t)}
          </Space>
          <Text type="secondary" style={{ fontSize: 13 }}>
            {authType}
          </Text>
        </Space>
      ),
    },
    {
      title: t("common:table.status"),
      dataIndex: "enabled",
      key: "enabled",
      width: 120,
      render: (enabled: boolean) => (
        <Tag color={enabled ? "green" : "default"}>
          {enabled ? t("users.status.enabled") : t("users.status.disabled")}
        </Tag>
      ),
    },
    {
      title: t("authProviders.table.priority", { defaultValue: "Priority" }),
      dataIndex: "sort_order",
      key: "sort_order",
      width: 120,
      render: (sortOrder?: number) => sortOrder ?? 0,
    },
    {
      title: t("authProviders.table.updated", { defaultValue: "Updated" }),
      dataIndex: "updated_at",
      key: "updated_at",
      width: 180,
      render: (updatedAt?: string) => <LocalDateTimeText value={updatedAt} />,
    },
    {
      title: t("common:table.actions"),
      key: "actions",
      width: 260,
      render: (_, record: AuthProvider) => (
        <Space size={4} wrap className="copy-friendly-actions">
          <Button
            type="link"
            size="small"
            data-testid={`auth-provider-action-mappings-${record.id}`}
            icon={<SafetyOutlined />}
            onClick={() => providers.openMappingModal(record)}
          >
            {t("authProviders.cohort_mappings")}
          </Button>
          {canTestAuthProviders ? (
            <Button
              type="link"
              size="small"
              data-testid={`auth-provider-action-test-${record.id}`}
              icon={<LinkOutlined />}
              loading={
                providers.testingProviderId === record.id &&
                providers.testConnectionPending
              }
              onClick={() => providers.testConnection(record)}
            >
              {t("authProviders.test_connection")}
            </Button>
          ) : null}
          {canUpdateAuthProviders ? (
            <Button
              type="link"
              size="small"
              data-testid={`auth-provider-action-edit-${record.id}`}
              icon={<EditOutlined />}
              onClick={() => providers.openEditModal(record)}
            >
              {t("common:button.edit")}
            </Button>
          ) : null}
          {canDeleteAuthProviders ? (
            <Button
              type="link"
              size="small"
              data-testid={`auth-provider-action-delete-${record.id}`}
              danger
              icon={<DeleteOutlined />}
              onClick={() => providers.openDeleteModal(record)}
            >
              {t("common:button.delete")}
            </Button>
          ) : null}
        </Space>
      ),
    },
  ];

  const mappingColumns: ColumnsType<ExternalCohortMapping> = [
    {
      title: t("authProviders.mapping.cohort"),
      key: "cohort",
      render: (_, record) => (
        <Space direction="vertical" size={0}>
          <Text strong>{record.cohort_display_name || record.cohort_key}</Text>
          <Text type="secondary" style={{ fontSize: 13 }}>
            {record.cohort_kind}:{record.cohort_key}
          </Text>
        </Space>
      ),
    },
    {
      title: t("authProviders.mapping.access", { defaultValue: "Access" }),
      key: "access",
      render: (_, record) => (
        <Space direction="vertical" size={0}>
          <Text strong>
            {record.role_name ||
              roleLabelById[record.role_id] ||
              record.role_id}
          </Text>
          <Text type="secondary" style={{ fontSize: 13 }}>
            {record.scope_type && record.scope_type !== "global"
              ? t("authProviders.mapping.scope_summary_named", {
                  defaultValue: "{{scope}}: {{target}}",
                  scope: t(`rbac.scope.${record.scope_type}`, {
                    defaultValue: record.scope_type,
                  }),
                  target:
                    record.scope_id || t("common:empty", { defaultValue: "—" }),
                })
              : t("authProviders.mapping.scope_summary_global", {
                  defaultValue: "All platform resources",
                })}
          </Text>
        </Space>
      ),
    },
    {
      title: t("authProviders.mapping.envs"),
      key: "allowed_environments",
      render: (_, record) => (
        <Space size={4} wrap>
          {(record.allowed_environments?.length
            ? record.allowed_environments
            : ["all"]
          ).map((env) => (
            <Tag
              key={`${record.id}-${env}`}
              color={env === "prod" ? "red" : "blue"}
            >
              {env === "all"
                ? t("rbac.bindings.all_environments", {
                    defaultValue: "All environments",
                  })
                : t(`authProviders.env.${env}`, { defaultValue: env })}
            </Tag>
          ))}
        </Space>
      ),
    },
    {
      title: t("common:table.updated_at", { defaultValue: "Updated" }),
      key: "updated_at",
      width: 180,
      render: (_, record) => (
        <LocalDateTimeText value={record.updated_at ?? record.created_at} />
      ),
    },
    {
      title: t("common:table.actions"),
      key: "actions",
      width: 180,
      render: (_, record) => (
        <Space size={4} wrap>
          {canUpdateAuthProviderMappings ? (
            <Button
              type="link"
              size="small"
              data-testid={`cohort-mapping-action-edit-${record.id}`}
              icon={<EditOutlined />}
              onClick={() => providers.openEditMappingModal(record)}
            >
              {t("common:button.edit")}
            </Button>
          ) : null}
          {canDeleteAuthProviderMappings ? (
            <Popconfirm
              title={t("authProviders.mapping.delete_confirm")}
              onConfirm={() => providers.deleteMapping(record)}
            >
              <Button
                type="link"
                size="small"
                danger
                data-testid={`cohort-mapping-action-delete-${record.id}`}
                icon={<DeleteOutlined />}
              >
                {t("common:button.delete")}
              </Button>
            </Popconfirm>
          ) : null}
        </Space>
      ),
    },
  ];

  return (
    <div className="auth-providers-page copy-friendly-actions">
      {providers.messageContextHolder}
      <PageHeader
        title={t("authProviders.title")}
        subtitle={t("authProviders.subtitle")}
      />
      <div className="summary-card-grid">
        <SummaryMetricCard
          title={t("authProviders.summary.total_title", {
            defaultValue: "Providers",
          })}
          value={providerSummary.totalCount}
          description={t("authProviders.summary.total_description", {
            defaultValue: "Authentication integrations currently configured.",
          })}
          visual={
            <ServiceWorkspaceGlyph className="summary-metric-card__art" />
          }
          accentColor="#1D5BFF"
          surfaceColor="#E6F4FF"
        />
        <SummaryMetricCard
          title={t("authProviders.summary.enabled_title", {
            defaultValue: "Enabled",
          })}
          value={providerSummary.enabledCount}
          description={t("authProviders.summary.enabled_description", {
            defaultValue: "Providers currently available for sign-in flows.",
          })}
          visual={
            <RequestsOverviewGlyph className="summary-metric-card__art" />
          }
          accentColor="#0F8F57"
          surfaceColor="#E8FFF2"
        />
        <SummaryMetricCard
          title={t("authProviders.summary.types_title", {
            defaultValue: "Provider types",
          })}
          value={providerSummary.distinctTypes}
          description={t("authProviders.summary.types_description", {
            defaultValue: "Distinct provider types represented in this list.",
          })}
          visual={<QueueReviewGlyph className="summary-metric-card__art" />}
          accentColor="#D66A1F"
          surfaceColor="#FFF4E5"
        />
        <SummaryMetricCard
          title={t("authProviders.summary.disabled_title", {
            defaultValue: "Disabled",
          })}
          value={providerSummary.disabledCount}
          description={t("authProviders.summary.disabled_description", {
            defaultValue:
              "Configured providers currently kept out of active login flows.",
          })}
          visual={
            <NotificationInboxGlyph className="summary-metric-card__art" />
          }
          accentColor="#6D4DE3"
          surfaceColor="#F5EDFF"
        />
      </div>

      <PageSurface className="auth-providers-page__workspace-surface">
        <Card
          className="auth-providers-page__runtime-card"
          size="small"
          title={t("authProviders.externalAuthSettings.title", {
            defaultValue: "External login public address",
          })}
          extra={
            <Tag color={externalAuthRuntimeReady ? "green" : "orange"}>
              {externalAuthSettingSource === "platform_setting"
                ? t("authProviders.externalAuthSettings.source_platform", {
                    defaultValue: "Platform override",
                  })
                : externalAuthSettingSource === "server_config"
                  ? t("authProviders.externalAuthSettings.source_deployment", {
                      defaultValue: "Deployment default",
                    })
                  : t("authProviders.externalAuthSettings.source_unset", {
                      defaultValue: "Not configured",
                    })}
            </Tag>
          }
          style={{ marginBottom: 16 }}
        >
          <Space direction="vertical" size={16} style={{ width: "100%" }}>
            <Alert
              showIcon
              type={externalAuthRuntimeReady ? "info" : "warning"}
              message={
                externalAuthRuntimeReady
                  ? t("authProviders.externalAuthSettings.ready_title", {
                      defaultValue: "Runtime external login is configured",
                    })
                  : t("authProviders.externalAuthSettings.missing_title", {
                      defaultValue: "Runtime external login is not ready",
                    })
              }
              description={
                externalAuthRuntimeReady
                  ? t("authProviders.externalAuthSettings.ready_description", {
                      defaultValue:
                        "WeCom, OIDC, and other external login providers reuse the same public callback base URL.",
                    })
                  : t("authProviders.externalAuthSettings.missing_description", {
                      defaultValue:
                        "Set a platform-wide public base URL before relying on runtime external login flows. Provider credential tests can still run without it.",
                    })
              }
            />
            <Space direction="vertical" size={4}>
              <Text type="secondary">
                {t("authProviders.externalAuthSettings.effective_label", {
                  defaultValue: "Effective public base URL",
                })}
              </Text>
              <Text code={true}>
                {externalAuthEffectiveBaseURL ||
                  t("authProviders.externalAuthSettings.unset_value", {
                    defaultValue: "Not configured",
                  })}
              </Text>
            </Space>
            <Form
              form={providers.externalAuthSettingsForm}
              layout="vertical"
              initialValues={{ public_base_url: externalAuthConfiguredBaseURL }}
            >
              <Form.Item
                name="public_base_url"
                label={t("authProviders.externalAuthSettings.input_label", {
                  defaultValue: "Platform override URL",
                })}
                tooltip={t("authProviders.externalAuthSettings.input_tooltip", {
                  defaultValue:
                    "Optional. Leave empty to keep using the deployment-level default.",
                })}
                rules={[
                  {
                    validator: async (_, value) => {
                      const trimmed = String(value || "").trim();
                      if (!trimmed) {
                        return;
                      }
                      try {
                        const parsed = new URL(trimmed);
                        if (
                          (parsed.protocol !== "http:" &&
                            parsed.protocol !== "https:") ||
                          parsed.pathname !== "/" && parsed.pathname !== "" ||
                          parsed.search ||
                          parsed.hash
                        ) {
                          throw new Error("invalid");
                        }
                      } catch {
                        throw new Error(
                          t("authProviders.externalAuthSettings.invalid_url", {
                            defaultValue:
                              "Enter an absolute http or https URL without any path.",
                          }),
                        );
                      }
                    },
                  },
                ]}
              >
                <Input
                  prefix={<LinkOutlined />}
                  placeholder="https://auth.example.com"
                  autoComplete="off"
                  disabled={!canUpdateAuthProviders}
                />
              </Form.Item>
              {canUpdateAuthProviders ? (
                <Space wrap={true}>
                  <Button
                    type="primary"
                    onClick={() => {
                      void providers.submitExternalAuthSettings();
                    }}
                    loading={providers.updateExternalAuthSettingsPending}
                  >
                    {t("common:button.save")}
                  </Button>
                  <Button
                    onClick={providers.resetExternalAuthSettingsToDeploymentDefault}
                    loading={providers.updateExternalAuthSettingsPending}
                  >
                    {t("authProviders.externalAuthSettings.use_deployment_default", {
                      defaultValue: "Use deployment default",
                    })}
                  </Button>
                </Space>
              ) : null}
            </Form>
          </Space>
        </Card>

        <Space className="auth-providers-page__toolbar" style={{ width: "100%", justifyContent: "space-between" }} wrap>
          <Text>{t("authProviders.config_help")}</Text>
          <Space>
            <Button
              icon={<ReloadOutlined />}
              onClick={() => providers.refetchProviders()}
            >
              {t("common:button.refresh")}
            </Button>
            {canCreateAuthProviders ? (
              <Button
                type="primary"
                icon={<PlusOutlined />}
                data-testid="auth-provider-create-button"
                onClick={providers.openCreateModal}
              >
                {t("authProviders.add")}
              </Button>
            ) : null}
          </Space>
        </Space>
        <div className="auth-providers-page__search-stack" style={{ marginTop: 16 }}>
          <PageSearchToolbar
            searchValue={quickSearch}
            searchDraftValue={quickSearchDraft}
            onSearchDraftChange={setQuickSearchDraft}
            onSearchChange={(value) => {
              setQuickSearchDraft(value);
              setQuickSearch(value);
            }}
            searchPlaceholder={t("authProviders.search_placeholder", {
              defaultValue: "Search providers by name, type, or description",
            })}
            searchHelp={t("authProviders.search_help", {
              defaultValue:
                "Press Enter or click Search. Quick search matches provider names, types, display names, descriptions, and pasted IDs.",
            })}
            advancedSearch={{
              open: filtersOpen,
              onToggle: () => setFiltersOpen((open) => !open),
              openLabel: t("common:search.advanced", { defaultValue: "Advanced search" }),
              closeLabel: t("common:search.hide_advanced", {
                defaultValue: "Hide advanced search",
              }),
              title: t("common:search.advanced", { defaultValue: "Advanced search" }),
              content: (
                <Space direction="vertical" size={12} style={{ width: "100%" }}>
                  <Text type="secondary">
                    {t("authProviders.advanced_search_help", {
                      defaultValue:
                        "Select exact provider filters here. Options support keyword matching, but the applied filter remains an exact value.",
                    })}
                  </Text>
                  <Space wrap align="end">
                  <Select
                    allowClear
                    showSearch
                    filterOption={filterOptionByLabel}
                    optionFilterProp="label"
                    style={{ width: 220 }}
                    placeholder={t("authProviders.filter_name", {
                      defaultValue: "Provider name",
                    })}
                    value={providerNameFilterDraft || undefined}
                    onChange={(value) => setProviderNameFilterDraft(String(value ?? ""))}
                    options={providerNameOptions}
                  />
                  <Select
                    allowClear
                    showSearch
                    filterOption={filterOptionByLabel}
                    optionFilterProp="label"
                    style={{ width: 220 }}
                    placeholder={t("authProviders.filter_type", {
                      defaultValue: "Provider type",
                    })}
                    value={authTypeFilterDraft || undefined}
                    onChange={(value) => setAuthTypeFilterDraft(String(value ?? ""))}
                    options={(providers.providerTypes ?? []).map((type) => ({
                      value: type.type,
                      label:
                        providers.providerTypeLabelByKey[type.type] ??
                        type.display_name,
                    }))}
                  />
                  <Select
                    showSearch
                    filterOption={filterOptionByLabel}
                    optionFilterProp="label"
                    style={{ width: 180 }}
                    value={statusFilterDraft}
                    onChange={(value) =>
                      setStatusFilterDraft(
                        value as "all" | "enabled" | "disabled",
                      )
                    }
                    options={[
                      {
                        value: "all",
                        label: t("common:filter.all", {
                          defaultValue: "All",
                        }),
                      },
                      {
                        value: "enabled",
                        label: t("authProviders.status_enabled", {
                          defaultValue: "Enabled",
                        }),
                      },
                      {
                        value: "disabled",
                        label: t("authProviders.status_disabled", {
                          defaultValue: "Disabled",
                        }),
                      },
                    ]}
                  />
                  <Button
                    type="primary"
                    data-testid="auth-providers-advanced-search-submit"
                    onClick={() => {
                      setQuickSearch(quickSearchDraft);
                      setProviderNameFilter(providerNameFilterDraft);
                      setAuthTypeFilter(authTypeFilterDraft);
                      setStatusFilter(statusFilterDraft);
                    }}
                  >
                    {t("common:button.search")}
                  </Button>
                  </Space>
                </Space>
              ),
            }}
            hasActiveFilters={Boolean(
              quickSearch.trim() ||
                providerNameFilter ||
                authTypeFilter ||
                statusFilter !== "all",
            )}
            onClear={() => {
              setQuickSearch("");
              setQuickSearchDraft("");
              setProviderNameFilter("");
              setProviderNameFilterDraft("");
              setAuthTypeFilter("");
              setAuthTypeFilterDraft("");
              setStatusFilter("all");
              setStatusFilterDraft("all");
            }}
            clearLabel={t("common:button.clear_filters", {
              defaultValue: "Clear filters",
            })}
          />
        </div>
        <Alert
          showIcon={true}
          type="info"
          style={{ marginTop: 16 }}
          message={t("authProviders.page_help_title", {
            defaultValue: "Recommended setup order",
          })}
          description={t("authProviders.page_help_description", {
            defaultValue:
              "Set the platform-wide external login address first, then create and test the provider, let one real user log in, and finally use discovered cohorts to grant access.",
          })}
        />
        <Alert
          showIcon={true}
          type="warning"
          style={{ marginTop: 16 }}
          message={t("authProviders.alpha_title", {
            defaultValue: "Alpha integrations",
          })}
          description={t("authProviders.alpha_description", {
            defaultValue:
              "Authentication provider integrations are currently in alpha. They are not yet fully validated and may not work reliably in every environment.",
          })}
        />

        <Table<AuthProvider>
          style={{ marginTop: 16 }}
          rowKey="id"
          columns={columns}
          dataSource={providerItems}
          loading={providers.providersLoading}
          pagination={false}
          locale={{
            emptyText: (
              <ActionEmptyState
                compact={true}
                title={t("authProviders.empty", {
                  defaultValue: "No authentication providers",
                })}
                description={t("authProviders.empty_description", {
                  defaultValue:
                    "Add the first provider before handing identity login to external users or admins.",
                })}
                visual={
                  <ServiceWorkspaceGlyph className="action-empty-state__art action-empty-state__art--compact" />
                }
              />
            ),
          }}
        />
      </PageSurface>

      <CreateProviderWizard
        open={providers.createOpen}
        form={createForm}
        providerTypes={providers.providerTypes ?? []}
        providerTypeOptions={providers.providerTypeOptions ?? []}
        providerTypesLoading={providers.providerTypesLoading ?? false}
        onSubmit={() => {
          void providers.submitCreate();
        }}
        onCancel={providers.closeCreateModal}
        confirmLoading={providers.createPending}
        t={t}
      />

      <EditProviderModal
        open={providers.editOpen}
        form={editForm}
        editingProvider={providers.editingProvider}
        providerTypes={providers.providerTypes ?? []}
        onSubmit={() => {
          void providers.submitEdit();
        }}
        onCancel={providers.closeEditModal}
        confirmLoading={providers.updatePending}
        t={t}
      />

      <Modal
        title={t("common:button.delete")}
        open={providers.deleteOpen}
        onOk={providers.submitDelete}
        onCancel={providers.closeDeleteModal}
        confirmLoading={providers.deletePending}
        okButtonProps={{ danger: true }}
        rootClassName="copy-friendly-actions"
        data-testid="auth-provider-delete-modal"
      >
        <Text>
          {t("authProviders.delete_confirm", {
            name: providers.deletingProvider?.name || "",
          })}
        </Text>
      </Modal>

      <Modal
        title={t("authProviders.mapping.modal_title", {
          name: providers.mappingProvider?.name || "",
        })}
        open={providers.mappingOpen}
        onCancel={providers.closeMappingModal}
        afterOpenChange={(open) => {
          if (!open) {
            return;
          }
          providers.syncForm.resetFields();
          providers.directoryRequestForm.resetFields();
          providers.syncForm.setFieldsValue({
            cohort_kind: providers.recommendedCohortDefaults.cohortKind,
            source_field: providers.recommendedCohortDefaults.sourceField,
          });
        }}
        footer={null}
        width={980}
        maskClosable={false}
        keyboard={false}
        rootClassName="copy-friendly-actions"
        data-testid="auth-provider-mappings-page"
      >
        <Space direction="vertical" size={20} style={{ width: "100%" }}>
          <Card size="small">
            <Space direction="vertical" size={4} style={{ width: "100%" }}>
              <Text strong>{providers.mappingProvider?.name}</Text>
              <Space size={6} wrap>
                <Tag color="processing">
                  {providers.mappingProvider
                    ? (providers.providerTypeLabelByKey[
                        providers.mappingProvider.auth_type
                      ] ?? providers.mappingProvider.auth_type)
                    : "—"}
                </Tag>
                <Tag
                  color={
                    providers.mappingProvider?.enabled ? "green" : "default"
                  }
                >
                  {providers.mappingProvider?.enabled
                    ? t("users.status.enabled")
                    : t("users.status.disabled")}
                </Tag>
              </Space>
              <Text type="secondary">
                {t("authProviders.table.provider_id", {
                  defaultValue: "Provider ID",
                })}
                : {providers.mappingProvider?.id ?? "—"}
              </Text>
            </Space>
          </Card>

          <Card
            size="small"
            title={t("authProviders.runtime.title", {
              defaultValue: "Runtime login",
            })}
            loading={providers.runtimeDescriptorLoading}
          >
            {!runtimeSupported ? (
              <Alert
                type="info"
                showIcon={true}
                message={t("authProviders.runtime.unsupported_title", {
                  defaultValue: "No runtime login capability",
                })}
                description={t("authProviders.runtime.unsupported_description", {
                  defaultValue:
                    "This provider can be managed in Shepherd, but it does not expose a public runtime login flow.",
                })}
              />
            ) : (
              <Space direction="vertical" size={12} style={{ width: "100%" }}>
                <Space size={8} wrap={true}>
                  {runtimeDescriptor?.supports_redirect ? (
                    <Tag color="blue">
                      {t("authProviders.runtime.redirect_supported", {
                        defaultValue: "Redirect login",
                      })}
                    </Tag>
                  ) : null}
                  {runtimeDescriptor?.supports_credentials ? (
                    <Tag color="purple">
                      {t("authProviders.runtime.credentials_supported", {
                        defaultValue: "Credential login",
                      })}
                    </Tag>
                  ) : null}
                  {runtimeDescriptor?.requires_public_base_url ? (
                    <Tag color={runtimeReady ? "green" : "orange"}>
                      {runtimeReady
                        ? t("authProviders.runtime.public_base_ready", {
                            defaultValue: "Public callback ready",
                          })
                        : t("authProviders.runtime.public_base_required", {
                            defaultValue: "Public callback required",
                          })}
                    </Tag>
                  ) : (
                    <Tag color="green">
                      {t("authProviders.runtime.no_public_base_required", {
                        defaultValue: "No public callback required",
                      })}
                    </Tag>
                  )}
                </Space>
                <Alert
                  type={runtimeReady ? "success" : "warning"}
                  showIcon={true}
                  message={
                    runtimeReady
                      ? t("authProviders.runtime.ready_title", {
                          defaultValue: "Runtime login is ready",
                        })
                      : t("authProviders.runtime.missing_title", {
                          defaultValue: "Runtime login needs platform setup",
                        })
                  }
                  description={
                    runtimeReady
                      ? t("authProviders.runtime.ready_description", {
                          defaultValue:
                            "This provider can be used for runtime login with the current platform settings.",
                        })
                      : t("authProviders.runtime.missing_description", {
                          defaultValue:
                            "At least one login mode requires the platform-wide external auth public base URL before browser-based runtime login can work reliably.",
                        })
                  }
                />
                <Table
                  rowKey={(mode) => mode.key}
                  size="small"
                  pagination={false}
                  columns={runtimeModeColumns}
                  dataSource={runtimeDescriptor?.login_modes || []}
                />
              </Space>
            )}
          </Card>

          <Card
            size="small"
            title={t("authProviders.directory.title", {
              defaultValue: "Directory preview",
            })}
            loading={providers.directoryDescriptorLoading}
          >
            {!directorySupported ? (
              <Alert
                type="info"
                showIcon={true}
                message={t("authProviders.directory.unsupported_title", {
                  defaultValue: "No directory capability",
                })}
                description={t("authProviders.directory.unsupported_description", {
                  defaultValue:
                    "This provider does not expose the optional directory sync capability.",
                })}
              />
            ) : (
              <Space direction="vertical" size={12} style={{ width: "100%" }}>
                <Text type="secondary">
                  {authProviderDirectoryDescription(
                    providers.mappingProvider?.auth_type,
                    directoryDescriptor?.description,
                    t,
                  ) ||
                    t("authProviders.directory.default_description", {
                      defaultValue:
                        "Build a provider-owned request, preview the canonical result, then trigger an asynchronous sync job if needed.",
                    })}
                </Text>
                <Form
                  form={providers.directoryRequestForm}
                  layout="vertical"
                  preserve={false}
                >
                  <SchemaConfigForm
                    schema={
                      (directoryDescriptor?.request_schema ??
                        null) as Parameters<typeof SchemaConfigForm>[0]["schema"]
                    }
                    form={providers.directoryRequestForm}
                    namePrefix="provider_request"
                    showJsonFallback={false}
                    applySchemaDefaults={true}
                    schemaNamespace={
                      providers.mappingProvider?.auth_type
                        ? `authProviders.directoryRequest.${providers.mappingProvider.auth_type}`
                        : undefined
                    }
                  />
                </Form>
                <Space size={8} wrap={true}>
                  {canSyncAuthProviders ? (
                    <>
                      <Button
                        icon={<ReloadOutlined />}
                        onClick={() => void providers.previewDirectory()}
                        loading={providers.previewDirectoryPending}
                      >
                        {t("authProviders.directory.preview", {
                          defaultValue: "Preview canonical result",
                        })}
                      </Button>
                      <Button
                        type="primary"
                        icon={<SyncOutlined />}
                        onClick={() => void providers.syncDirectory()}
                        loading={providers.syncDirectoryPending}
                      >
                        {t("authProviders.directory.sync", {
                          defaultValue: "Run sync job",
                        })}
                      </Button>
                    </>
                  ) : null}
                </Space>
                {directoryPreview ? (
                  <Space direction="vertical" size={12} style={{ width: "100%" }}>
                    <Row gutter={[12, 12]}>
                      <Col xs={24} md={6}>
                        <SummaryMetricCard
                          title={t("authProviders.directory.summary.total", {
                            defaultValue: "Previewed records",
                          })}
                          value={directoryPreviewSummary.total}
                          description={t(
                            "authProviders.directory.summary.total_description",
                            {
                              defaultValue:
                                "Canonical records returned by the provider-owned preview request.",
                            },
                          )}
                          action={renderDirectorySummaryJobActionButton(
                            latestDirectorySyncJob,
                            t("authProviders.directory.summary.latest_job", {
                              defaultValue: "Latest job details",
                            }),
                            "directory-preview-latest-job",
                          )}
                          accentColor="#2563eb"
                          surfaceColor="rgba(37, 99, 235, 0.10)"
                        />
                      </Col>
                      <Col xs={24} md={6}>
                        <SummaryMetricCard
                          title={t("authProviders.directory.summary.ready", {
                            defaultValue: "Ready to apply",
                          })}
                          value={directoryPreviewSummary.ready}
                          description={t(
                            "authProviders.directory.summary.ready_description",
                            {
                              defaultValue:
                                "Records with no canonical conflicts or warnings.",
                            },
                          )}
                          action={
                            readyDirectorySummaryActions.length > 0 ? (
                              <Space size={4} wrap={true}>
                                {readyDirectorySummaryActions}
                              </Space>
                            ) : null
                          }
                          accentColor="#059669"
                          surfaceColor="rgba(5, 150, 105, 0.10)"
                        />
                      </Col>
                      <Col xs={24} md={6}>
                        <SummaryMetricCard
                          title={t("authProviders.directory.summary.warning", {
                            defaultValue: "Needs review",
                          })}
                          value={directoryPreviewSummary.warning}
                          description={t(
                            "authProviders.directory.summary.warning_description",
                            {
                              defaultValue:
                                "Records with warnings but no blocking canonical conflicts.",
                            },
                          )}
                          accentColor="#d97706"
                          surfaceColor="rgba(217, 119, 6, 0.10)"
                        />
                      </Col>
                      <Col xs={24} md={6}>
                        <SummaryMetricCard
                          title={t("authProviders.directory.summary.conflict", {
                            defaultValue: "Blocked by conflicts",
                          })}
                          value={directoryPreviewSummary.conflict}
                          description={t(
                            "authProviders.directory.summary.conflict_description",
                            {
                              defaultValue:
                                "Records that would not apply cleanly without explicit resolution.",
                            },
                          )}
                          action={renderDirectorySummaryJobActionButton(
                            latestDirectorySyncJobByAction.blocked,
                            t("authProviders.directory.summary.latest_blocked_job", {
                              defaultValue: "Latest blocked job",
                            }),
                            "directory-preview-latest-blocked-job",
                          )}
                          accentColor="#ea580c"
                          surfaceColor="rgba(234, 88, 12, 0.10)"
                        />
                      </Col>
                    </Row>
                    <Space direction="vertical" size={8} style={{ width: "100%" }}>
                      <Space
                        align="center"
                        size={[8, 8]}
                        wrap={true}
                        style={{ width: "100%", justifyContent: "space-between" }}
                      >
                        <Segmented<DirectoryPreviewFilter>
                          value={directoryPreviewFilter}
                          onChange={(value) => setDirectoryPreviewFilter(value)}
                          options={[
                            {
                              label: t("authProviders.directory.filter.all", {
                                defaultValue: "All",
                              }),
                              value: "all",
                            },
                            {
                              label: t("authProviders.directory.filter.ready", {
                                defaultValue: "Ready",
                              }),
                              value: "ready",
                            },
                            {
                              label: t("authProviders.directory.filter.warning", {
                                defaultValue: "Warning",
                              }),
                              value: "warning",
                            },
                            {
                              label: t("authProviders.directory.filter.conflict", {
                                defaultValue: "Conflict",
                              }),
                              value: "conflict",
                            },
                          ]}
                        />
                        <Text type="secondary">
                          {t("authProviders.directory.filter_count", {
                            defaultValue: "{{count}} records in current view",
                            count: filteredDirectoryPreviewItems.length,
                          })}
                        </Text>
                      </Space>
                      {directoryPreviewConflictCodes.length > 0 ? (
                        <Space direction="vertical" size={8} style={{ width: "100%" }}>
                          <Descriptions
                            size="small"
                            column={1}
                            items={directoryPreviewConflictDescriptionItems}
                          />
                          {filteredDirectoryPreviewConflictGroups.length > 0 ? (
                            <Collapse
                              size="small"
                              items={filteredDirectoryPreviewConflictGroups.map(
                                (group) => ({
                                  key: group.code,
                                  label: (
                                    <Space size={8} wrap={true}>
                                      <Text strong>
                                        {directoryConflictCodeLabel(group.code, t)}
                                      </Text>
                                      <Tag color="orange">{group.items.length}</Tag>
                                      <Text type="secondary" style={{ fontSize: 13 }}>
                                        {group.code}
                                      </Text>
                                    </Space>
                                  ),
                                  children: (
                                    <Table<
                                      NonNullable<DirectorySyncPreview["items"]>[number]
                                    >
                                      rowKey={(item) =>
                                        `${group.code}:${item.record.external_id}:${item.record.username}`
                                      }
                                      size="small"
                                      pagination={false}
                                      columns={directoryPreviewColumns}
                                      dataSource={group.items}
                                    />
                                  ),
                                }),
                              )}
                            />
                          ) : null}
                        </Space>
                      ) : null}
                      {filteredDirectoryPreviewWarningGroups.length > 0 ? (
                        <Space direction="vertical" size={8} style={{ width: "100%" }}>
                          <Text strong>
                            {t("authProviders.directory.warning_groups", {
                              defaultValue: "Grouped warnings",
                            })}
                          </Text>
                          <Collapse
                            size="small"
                            items={filteredDirectoryPreviewWarningGroups.map(
                              (group) => ({
                                key: group.warning,
                                label: (
                                  <Space size={8} wrap={true}>
                                    <Tag color="gold">{group.items.length}</Tag>
                                    <Text>{group.warning}</Text>
                                  </Space>
                                ),
                                children: (
                                  <Table<
                                    NonNullable<DirectorySyncPreview["items"]>[number]
                                  >
                                    rowKey={(item) =>
                                      `${group.warning}:${item.record.external_id}:${item.record.username}`
                                    }
                                    size="small"
                                    pagination={false}
                                    columns={directoryPreviewColumns}
                                    dataSource={group.items}
                                  />
                                ),
                              }),
                            )}
                          />
                        </Space>
                      ) : null}
                    </Space>
                    <Table<
                      NonNullable<DirectorySyncPreview["items"]>[number]
                    >
                      rowKey={(item) =>
                        `${item.record.external_id}:${item.record.username}`
                      }
                      size="small"
                      pagination={false}
                      columns={directoryPreviewColumns}
                      dataSource={filteredDirectoryPreviewItems}
                    />
                  </Space>
                ) : (
                  <Alert
                    type="info"
                    showIcon={true}
                    message={t("authProviders.directory.preview_empty_title", {
                      defaultValue: "No preview result yet",
                    })}
                    description={t(
                      "authProviders.directory.preview_empty_description",
                      {
                        defaultValue:
                          "Use the provider request form above to preview the canonical directory result before launching a sync job.",
                      },
                    )}
                  />
                )}
              </Space>
            )}
          </Card>

          <Card
            size="small"
            title={t("authProviders.schedule.title", {
              defaultValue: "Scheduled enrichment",
            })}
            loading={providers.directoryScheduleLoading}
          >
            {providers.directorySchedule?.supported === false ? (
              <Alert
                type="info"
                showIcon={true}
                message={t("authProviders.schedule.unsupported_title", {
                  defaultValue: "Manual-only directory workflow",
                })}
                description={t("authProviders.schedule.unsupported_description", {
                  defaultValue:
                    "This provider supports manual directory sync but does not publish a scheduled enrichment plan.",
                })}
              />
            ) : providers.directorySchedule?.enabled ? (
              <Space direction="vertical" size={8} style={{ width: "100%" }}>
                <Row gutter={[16, 8]}>
                  <Col span={12}>
                    <Text type="secondary">
                      {t("authProviders.schedule.mode", {
                        defaultValue: "Mode",
                      })}
                    </Text>
                    <br />
                    <Tag color="processing">
                      {providers.directorySchedule.mode || "—"}
                    </Tag>
                  </Col>
                  <Col span={12}>
                    <Text type="secondary">
                      {t("authProviders.schedule.join_key", {
                        defaultValue: "Join key",
                      })}
                    </Text>
                    <br />
                    <Tag>{providers.directorySchedule.join_key_type || "—"}</Tag>
                  </Col>
                  <Col span={12}>
                    <Text type="secondary">
                      {t("authProviders.schedule.cron", {
                        defaultValue: "Cron",
                      })}
                    </Text>
                    <br />
                    <Text code={true}>
                      {providers.directorySchedule.schedule_cron || "—"}
                    </Text>
                  </Col>
                  <Col span={12}>
                    <Text type="secondary">
                      {t("authProviders.schedule.timezone", {
                        defaultValue: "Timezone",
                      })}
                    </Text>
                    <br />
                    <Text>{providers.directorySchedule.schedule_timezone || "—"}</Text>
                  </Col>
                  <Col span={12}>
                    <Text type="secondary">
                      {t("authProviders.schedule.last_run", {
                        defaultValue: "Last run",
                      })}
                    </Text>
                    <br />
                    {providers.directorySchedule.last_job_created_at ? (
                      <LocalDateTimeText
                        value={providers.directorySchedule.last_job_created_at}
                      />
                    ) : (
                      <Text type="secondary">—</Text>
                    )}
                  </Col>
                  <Col span={12}>
                    <Text type="secondary">
                      {t("authProviders.schedule.next_run", {
                        defaultValue: "Next run",
                      })}
                    </Text>
                    <br />
                    {providers.directorySchedule.next_run_at ? (
                      <LocalDateTimeText
                        value={providers.directorySchedule.next_run_at}
                      />
                    ) : (
                      <Text type="secondary">—</Text>
                    )}
                  </Col>
                </Row>
                <Space size={8} wrap={true}>
                  {providers.directorySchedule.last_job_status ? (
                    <Tag color="blue">
                      {t("authProviders.schedule.last_job_status", {
                        defaultValue: "Last job",
                      })}
                      : {providers.directorySchedule.last_job_status}
                    </Tag>
                  ) : null}
                  {providers.directorySchedule.pending_job_id ? (
                    <Tag color="orange">
                      {t("authProviders.schedule.pending_job", {
                        defaultValue: "Pending job",
                      })}
                      : {providers.directorySchedule.pending_job_status || "pending"}
                    </Tag>
                  ) : null}
                </Space>
              </Space>
            ) : (
              <Alert
                type="warning"
                showIcon={true}
                message={t("authProviders.schedule.disabled_title", {
                  defaultValue: "Scheduled enrichment is disabled",
                })}
                description={t("authProviders.schedule.disabled_description", {
                  defaultValue:
                    "This provider can publish a scheduled enrichment plan, but the plan is currently disabled in provider configuration.",
                })}
              />
            )}
          </Card>

          <Alert
            type="info"
            showIcon={true}
            message={t("authProviders.guide.title", {
              defaultValue: "Recommended flow",
            })}
            description={
              <ol style={{ margin: 0, paddingInlineStart: 18 }}>
                {mappingWorkflow.guideSteps.map((step) => (
                  <li key={step}>{step}</li>
                ))}
              </ol>
            }
          />

          <Card
            size="small"
            title={t("authProviders.sample.title")}
            extra={
              canTestAuthProviders ? (
                <Button
                  data-testid={
                    providers.mappingProvider
                      ? `auth-provider-action-sample-${providers.mappingProvider.id}`
                      : undefined
                  }
                  icon={<SyncOutlined />}
                  onClick={() =>
                    providers.testConnection(
                      providers.mappingProvider as AuthProvider,
                    )
                  }
                  loading={providers.testConnectionPending}
                  disabled={!providers.mappingProvider}
                >
                  {t("authProviders.test_connection")}
                </Button>
              ) : null
            }
          >
            <Table
              rowKey="field"
              size="small"
              pagination={false}
              loading={providers.sampleLoading}
              dataSource={providers.sampleFields}
              columns={[
                {
                  title: t("authProviders.sample.field"),
                  dataIndex: "field",
                  key: "field",
                  render: (value: string) => localizeProviderProfileFieldLabel(t, value),
                },
                {
                  title: t("authProviders.sample.value_type"),
                  dataIndex: "value_type",
                  key: "value_type",
                  width: 120,
                },
                {
                  title: t("authProviders.sample.unique_count"),
                  dataIndex: "unique_count",
                  key: "unique_count",
                  width: 120,
                },
                {
                  title: t("authProviders.sample.sample"),
                  key: "sample",
                  render: (_, record) => (record.sample ?? []).join(", "),
                },
              ]}
              locale={{
                emptyText: (
                  <ActionEmptyState
                    compact={true}
                    title={mappingWorkflow.sampleEmptyTitle}
                    description={mappingWorkflow.sampleEmptyDescription}
                    visual={
                      <RateLimitGaugeGlyph className="action-empty-state__art action-empty-state__art--compact" />
                    }
                  />
                ),
              }}
            />
          </Card>

          <Card
            size="small"
            title={t("authProviders.discovered.title", {
              defaultValue: "Discovered cohorts",
            })}
          >
            <Table<ExternalCohort>
              rowKey="id"
              size="small"
              pagination={false}
              loading={providers.cohortsLoading}
              dataSource={providers.cohorts}
              columns={discoveredCohortColumns}
              locale={{
                emptyText: (
                  <ActionEmptyState
                    compact={true}
                    title={mappingWorkflow.discoveredEmptyTitle}
                    description={mappingWorkflow.discoveredEmptyDescription}
                    visual={
                      <AccessControlGlyph className="action-empty-state__art action-empty-state__art--compact" />
                    }
                  />
                ),
              }}
            />
          </Card>

          <Card
            size="small"
            title={t("authProviders.jobs.title", {
              defaultValue: "Recent sync jobs",
            })}
            extra={
              <Segmented<DirectoryJobFilter>
                size="small"
                value={directoryJobFilter}
                onChange={(value) => setDirectoryJobFilter(value)}
                options={[
                  {
                    label: t("authProviders.jobs.filter.all", {
                      defaultValue: "All",
                    }),
                    value: "all",
                  },
                  {
                    label: `${directoryPreviewActionLabel("create", t)} (${(providers.directorySyncJobs ?? []).filter((job) => job.result_summary.create_count > 0).length})`,
                    value: "create",
                  },
                  {
                    label: `${directoryPreviewActionLabel("update", t)} (${(providers.directorySyncJobs ?? []).filter((job) => job.result_summary.update_count > 0).length})`,
                    value: "update",
                  },
                  {
                    label: `${directoryPreviewActionLabel("blocked", t)} (${(providers.directorySyncJobs ?? []).filter((job) => job.result_summary.blocked_count > 0).length})`,
                    value: "blocked",
                  },
                ]}
              />
            }
          >
            <Table<DirectorySyncJob>
              rowKey="id"
              size="small"
              pagination={false}
              loading={providers.directorySyncJobsLoading}
              dataSource={filteredDirectorySyncJobs}
              columns={directorySyncJobColumns}
              locale={{
                emptyText: (
                  <ActionEmptyState
                    compact={true}
                    title={t("authProviders.jobs.empty", {
                      defaultValue: "No sync jobs yet",
                    })}
                    description={t("authProviders.jobs.empty_description", {
                      defaultValue:
                        "Run a manual directory action or wait for the first scheduled enrichment run to see job history here.",
                    })}
                    visual={
                      <QueueReviewGlyph className="action-empty-state__art action-empty-state__art--compact" />
                    }
                  />
                ),
              }}
            />
          </Card>

          <Drawer
            title={t("authProviders.jobs.detail_title", {
              defaultValue: "Sync job details",
            })}
            placement="right"
            width={560}
            open={!!providers.selectedDirectorySyncJobId}
            onClose={providers.closeDirectorySyncJobDetail}
            destroyOnClose={true}
          >
            {providers.directorySyncJobDetailLoading ? (
              <Text type="secondary">
                {t("common:status.loading", {
                  defaultValue: "Loading…",
                })}
              </Text>
            ) : providers.directorySyncJobDetail ? (
              <Space direction="vertical" size={16} style={{ width: "100%" }}>
                <Descriptions
                  size="small"
                  bordered={true}
                  column={1}
                  items={[
                    {
                      key: "id",
                      label: t("common:table.id", {
                        defaultValue: "ID",
                      }),
                      children: providers.directorySyncJobDetail.id,
                    },
                    {
                      key: "mode",
                      label: t("authProviders.jobs.mode", {
                        defaultValue: "Mode",
                      }),
                      children: (
                        <Tag
                          color={
                            providers.directorySyncJobDetail.sync_mode ===
                            "scheduled_enrichment"
                              ? "purple"
                              : "blue"
                          }
                        >
                          {directorySyncModeLabel(
                            providers.directorySyncJobDetail.sync_mode,
                            t,
                          )}
                        </Tag>
                      ),
                    },
                    {
                      key: "status",
                      label: t("common:table.status"),
                      children: (
                        <Tag
                          color={directorySyncStatusColor(
                            providers.directorySyncJobDetail.status,
                          )}
                        >
                          {providers.directorySyncJobDetail.status}
                        </Tag>
                      ),
                    },
                    {
                      key: "join-key",
                      label: t("authProviders.jobs.join_key", {
                        defaultValue: "Join key",
                      }),
                      children:
                        providers.directorySyncJobDetail.join_key_type || "—",
                    },
                    {
                      key: "summary",
                      label: t("authProviders.jobs.summary", {
                        defaultValue: "Summary",
                      }),
                      children: (
                        <Space size={4} wrap={true}>
                          <Tag color="green">
                            {
                              providers.directorySyncJobDetail.result_summary
                                .create_count
                            }{" "}
                            {directoryPreviewActionLabel("create", t)}
                          </Tag>
                          <Tag color="blue">
                            {
                              providers.directorySyncJobDetail.result_summary
                                .update_count
                            }{" "}
                            {directoryPreviewActionLabel("update", t)}
                          </Tag>
                          <Tag color="orange">
                            {
                              providers.directorySyncJobDetail.result_summary
                                .blocked_count
                            }{" "}
                            {directoryPreviewActionLabel("blocked", t)}
                          </Tag>
                          {providers.directorySyncJobDetail.error_count > 0 ? (
                            <Tag color="red">
                              {t("authProviders.jobs.error_count", {
                                defaultValue: "{{count}} errors",
                                count:
                                  providers.directorySyncJobDetail.error_count,
                              })}
                            </Tag>
                          ) : null}
                        </Space>
                      ),
                    },
                    {
                      key: "total",
                      label: t("authProviders.jobs.total_entries", {
                        defaultValue: "Total entries",
                      }),
                      children:
                        providers.directorySyncJobDetail.total_entries.toString(),
                    },
                    {
                      key: "triggered-by",
                      label: t("authProviders.jobs.triggered_by", {
                        defaultValue: "Triggered by",
                      }),
                      children: providers.directorySyncJobDetail.triggered_by,
                    },
                    {
                      key: "created-at",
                      label: t("authProviders.jobs.created_at", {
                        defaultValue: "Created",
                      }),
                      children: (
                        <LocalDateTimeText
                          value={providers.directorySyncJobDetail.created_at}
                        />
                      ),
                    },
                    {
                      key: "started-at",
                      label: t("authProviders.jobs.started_at", {
                        defaultValue: "Started",
                      }),
                      children: providers.directorySyncJobDetail.started_at ? (
                        <LocalDateTimeText
                          value={providers.directorySyncJobDetail.started_at}
                        />
                      ) : (
                        "—"
                      ),
                    },
                    {
                      key: "completed-at",
                      label: t("authProviders.jobs.completed_at", {
                        defaultValue: "Completed",
                      }),
                      children: providers.directorySyncJobDetail.completed_at ? (
                        <LocalDateTimeText
                          value={providers.directorySyncJobDetail.completed_at}
                        />
                      ) : (
                        "—"
                      ),
                    },
                  ]}
                />

                {providers.directorySyncJobDetail.errors.length > 0 ? (
                  <Alert
                    type="error"
                    showIcon={true}
                    message={t("authProviders.jobs.errors_title", {
                      defaultValue: "Execution errors",
                    })}
                    description={
                      <Space direction="vertical" size={4}>
                        {providers.directorySyncJobDetail.errors.map((error: string) => (
                          <Text key={error} type="secondary">
                            {error}
                          </Text>
                        ))}
                      </Space>
                    }
                  />
                ) : null}

                <Card
                  size="small"
                  title={t("authProviders.jobs.request_snapshot", {
                    defaultValue: "Request snapshot",
                  })}
                >
                  <pre
                    style={{
                      margin: 0,
                      whiteSpace: "pre-wrap",
                      wordBreak: "break-word",
                      fontSize: 13,
                    }}
                  >
                    {formatJsonObject(
                      providers.directorySyncJobDetail.request_snapshot as
                        | Record<string, unknown>
                        | undefined,
                    )}
                  </pre>
                </Card>
              </Space>
            ) : null}
          </Drawer>

          {canSyncAuthProviders ? (
            <Card size="small" title={mappingWorkflow.manualTitle}>
              <Form form={providers.syncForm} layout="vertical">
                <Space direction="vertical" size={12} style={{ width: "100%" }}>
                  <Text type="secondary">
                    {mappingWorkflow.manualDescription}
                  </Text>
                  <Alert
                    type="warning"
                    showIcon={true}
                    message={t("authProviders.sync.manual_warning_title", {
                      defaultValue:
                        "Use this only when pre-registration is necessary",
                    })}
                    description={t(
                      "authProviders.sync.manual_warning_description",
                      {
                        defaultValue:
                          "If a real external user can log in first, prefer discovered cohorts above. Manual registration is a fallback for pre-binding access.",
                      },
                    )}
                  />
                  {recommendedCohortCopy ? (
                    <Alert
                      type="info"
                      showIcon={true}
                      message={recommendedCohortCopy.title}
                      description={recommendedCohortCopy.description}
                    />
                  ) : null}
                  {manualCohortSeedCandidates.length > 0 ? (
                    <Card
                      size="small"
                      title={t("authProviders.sync.quick_start_title", {
                        defaultValue: "Quick start from observed values",
                      })}
                    >
                      <Space direction="vertical" size={8} style={{ width: "100%" }}>
                        <Text type="secondary">
                          {t("authProviders.sync.quick_start_description", {
                            defaultValue:
                              "Choose a discovered provider field and let Shepherd pre-fill the organization type and known sample values for you.",
                          })}
                        </Text>
                        <Space wrap={true} className="copy-friendly-actions">
                          {manualCohortSeedCandidates.map((candidate) => (
                            <Button
                              key={`${candidate.cohortKind}:${candidate.sourceField}`}
                              size="small"
                              onClick={() => {
                                providers.syncForm.setFieldsValue({
                                  cohort_kind: candidate.cohortKind,
                                  source_field: candidate.sourceField,
                                  cohorts_text: candidate.values,
                                });
                              }}
                            >
                              {t("authProviders.sync.quick_start_button", {
                                defaultValue:
                                  '{{field}} -> {{cohortKind}} ({{count}} values)',
                                field: candidate.sourceField,
                                cohortKind: candidate.cohortKind,
                                count: candidate.values.length,
                              })}
                            </Button>
                          ))}
                        </Space>
                      </Space>
                    </Card>
                  ) : null}
                  {activeManualCohortSeedCandidate ? (
                    <Alert
                      type="success"
                      showIcon={true}
                      message={t("authProviders.sync.selected_source_hint_title", {
                        defaultValue: "Selected source field has known values",
                      })}
                      description={t(
                        "authProviders.sync.selected_source_hint_description",
                        {
                          defaultValue:
                            'Field "{{field}}" currently exposes {{count}} sample values. The cohort list below has been preloaded from those values and still allows manual additions.',
                          field: activeManualCohortSeedCandidate.sourceField,
                          count: activeManualCohortSeedCandidate.values.length,
                        },
                      )}
                    />
                  ) : null}
                </Space>
                <Form.Item
                  name="cohort_kind"
                  label={t("authProviders.sync.cohort_kind")}
                  rules={[{ required: true }]}
                  extra={mappingWorkflow.cohortKindHint}
                >
                  <AutoComplete
                    options={manualCohortKindOptions}
                    allowClear={true}
                    placeholder={t("authProviders.sync.cohort_kind_placeholder", {
                      defaultValue: "Select or type a canonical cohort kind",
                    })}
                    filterOption={(inputValue, option) => {
                      const search = inputValue.trim().toLowerCase();
                      const label = String(option?.label ?? "").toLowerCase();
                      const value = String(option?.value ?? "").toLowerCase();
                      return label.includes(search) || value.includes(search);
                    }}
                  />
                </Form.Item>
                <Form.Item
                  name="source_field"
                  label={t("authProviders.sync.source_field")}
                  rules={[{ required: true }]}
                  extra={mappingWorkflow.sourceFieldHint}
                >
                  <AutoComplete
                    options={manualSourceFieldOptions}
                    allowClear={true}
                    placeholder={t("authProviders.sync.source_field_placeholder", {
                      defaultValue: "Select a discovered field or type a provider field",
                    })}
                    filterOption={(inputValue, option) => {
                      const search = inputValue.trim().toLowerCase();
                      const label = String(option?.label ?? "").toLowerCase();
                      const value = String(option?.value ?? "").toLowerCase();
                      return label.includes(search) || value.includes(search);
                    }}
                  />
                </Form.Item>
                <Form.Item
                  name="cohorts_text"
                  label={t("authProviders.sync.cohorts")}
                  rules={[{ required: true }]}
                  extra={t("authProviders.sync.cohorts_help", {
                    defaultValue:
                      "Prefer selecting discovered cohorts below. You can still type a new value if the provider has not exposed it yet.",
                  })}
                >
                  <Select
                    mode="tags"
                    allowClear={true}
                    showSearch={true}
                    options={manualKnownCohortOptions}
                    placeholder={mappingWorkflow.cohortsPlaceholder}
                    tokenSeparators={[",", "\n"]}
                  />
                </Form.Item>
                <Button
                  type="primary"
                  icon={<SyncOutlined />}
                  loading={providers.syncCohortsPending}
                  data-testid={
                    providers.mappingProvider
                      ? `auth-provider-action-sync-${providers.mappingProvider.id}`
                      : undefined
                  }
                  onClick={() => {
                    void providers.submitSyncCohorts();
                  }}
                >
                  {t("authProviders.sync.submit_manual", {
                    defaultValue: "Save known cohorts",
                  })}
                </Button>
              </Form>
            </Card>
          ) : null}

          <Card
            size="small"
            title={t("authProviders.mapping.title")}
            extra={
              canCreateAuthProviderMappings ? (
                <Button
                  type="primary"
                  icon={<PlusOutlined />}
                  data-testid="cohort-mapping-create-button"
                  onClick={providers.openCreateMappingModal}
                >
                  {t("authProviders.mapping.add")}
                </Button>
              ) : null
            }
          >
            <Table<ExternalCohortMapping>
              rowKey="id"
              size="small"
              columns={mappingColumns}
              dataSource={providers.mappings}
              loading={providers.mappingsLoading}
              pagination={false}
              locale={{
                emptyText: (
                  <ActionEmptyState
                    compact={true}
                    title={t("authProviders.mapping.empty", {
                      defaultValue: "No cohort mappings yet",
                    })}
                    description={t("authProviders.mapping.empty_description", {
                      defaultValue:
                        "Create the first mapping before delegating access through this provider.",
                    })}
                    visual={
                      <AccessControlGlyph className="action-empty-state__art action-empty-state__art--compact" />
                    }
                  />
                ),
              }}
            />
          </Card>
        </Space>
      </Modal>

      <Modal
        title={t("authProviders.mapping.edit_title")}
        open={providers.editMappingOpen}
        onCancel={providers.closeEditMappingModal}
        onOk={() => {
          void providers.submitEditMapping();
        }}
        confirmLoading={providers.updateMappingPending}
        maskClosable={false}
        keyboard={false}
        rootClassName="copy-friendly-actions"
        data-testid="cohort-mapping-edit-modal"
      >
        <Form
          form={providers.mappingEditForm}
          layout="vertical"
          preserve={false}
        >
          <MappingFormFields
            form={providers.mappingEditForm}
            scopeTargetOptionsByType={providers.scopeTargetOptionsByType}
            scopeTargetLoadingByType={providers.scopeTargetLoadingByType}
            environmentOptions={environmentOptions}
            roleOptions={providers.roleOptions}
            scopeOptions={scopeOptions}
            t={t}
          />
        </Form>
      </Modal>

      {/* ── Cohort Mapping Create Modal ─────────────────────────────── */}
      <Modal
        title={t("authProviders.mapping.add")}
        open={providers.createMappingModalOpen}
        onOk={() => {
          void providers.submitCreateMapping();
        }}
        onCancel={providers.closeCreateMappingModal}
        confirmLoading={providers.createMappingPending}
        width={720}
        maskClosable={false}
        keyboard={false}
        rootClassName="copy-friendly-actions"
        data-testid="cohort-mapping-create-modal"
      >
        <Form form={providers.mappingForm} layout="vertical" preserve={false}>
          <MappingFormFields
            form={providers.mappingForm}
            discoveredCohortOptions={discoveredCohortOptions}
            discoveredCohortsLoading={providers.cohortsLoading}
            scopeTargetOptionsByType={providers.scopeTargetOptionsByType}
            scopeTargetLoadingByType={providers.scopeTargetLoadingByType}
            environmentOptions={environmentOptions}
            roleOptions={providers.roleOptions}
            scopeOptions={scopeOptions}
            showCohortFields={true}
            t={t}
          />
        </Form>
      </Modal>
    </div>
  );
}

// ── Provider Type Icons ───────────────────────────────────────────────────
const providerTypeIcons: Record<string, React.ReactNode> = {
  oidc: <KeyOutlined />,
  ldap: <CloudServerOutlined />,
};

// ── CreateProviderWizard ──────────────────────────────────────────────────

interface CreateProviderWizardProps {
  open: boolean;
  form: FormInstance;
  providerTypes: AuthProviderType[];
  providerTypeOptions: Array<{ value: string; label: string }>;
  providerTypesLoading: boolean;
  onSubmit: () => void;
  onCancel: () => void;
  confirmLoading: boolean;
  t: (key: string, opts?: Record<string, unknown>) => string;
}

function CreateProviderWizard({
  open,
  form,
  providerTypes,
  providerTypeOptions,
  providerTypesLoading,
  onSubmit,
  onCancel,
  confirmLoading,
  t,
}: CreateProviderWizardProps) {
  const [step, setStep] = useState(0);

  const handleNext = async () => {
    try {
      if (step === 0) {
        await form.validateFields(["name", "auth_type"]);
      }
      setStep((s) => Math.min(s + 1, 2));
    } catch {
      // validation failed — stay on current step
    }
  };

  const handlePrev = () => setStep((s) => Math.max(s - 1, 0));

  const handleSubmit = () => {
    onSubmit();
    setStep(0);
  };

  const handleCancel = () => {
    setStep(0);
    onCancel();
  };

  const footerButtons = () => {
    const buttons: React.ReactNode[] = [
      <Button key="cancel" onClick={handleCancel}>
        {t("common:button.cancel")}
      </Button>,
    ];
    if (step > 0) {
      buttons.push(
        <Button key="prev" onClick={handlePrev}>
          {t("authProviders.wizard.prev")}
        </Button>,
      );
    }
    if (step < 2) {
      buttons.push(
        <Button
          key="next"
          type="primary"
          onClick={() => {
            void handleNext();
          }}
        >
          {t("authProviders.wizard.next")}
        </Button>,
      );
    }
    if (step === 2) {
      buttons.push(
        <Button
          key="submit"
          type="primary"
          loading={confirmLoading}
          onClick={handleSubmit}
        >
          {t("common:button.submit")}
        </Button>,
      );
    }
    return buttons;
  };

  return (
    <Modal
      title={t("authProviders.add_title")}
      open={open}
      onCancel={handleCancel}
      width={680}
      maskClosable={false}
      keyboard={false}
      rootClassName="copy-friendly-actions"
      footer={footerButtons()}
      data-testid="auth-provider-create-modal"
    >
      <Steps
        current={step}
        size="small"
        style={{ marginBottom: 24 }}
        items={[
          { title: t("authProviders.wizard.step_basic") },
          { title: t("authProviders.wizard.step_config") },
          { title: t("authProviders.wizard.step_confirm") },
        ]}
      />

      <Form
        form={form}
        layout="vertical"
        preserve={false}
        initialValues={{ enabled: true, sort_order: 0 }}
      >
        <Form.Item noStyle={true} shouldUpdate={true}>
          {(formInstance) => {
            const authTypeValue = formInstance.getFieldValue("auth_type");
            const selectedAuthType =
              typeof authTypeValue === "string" ? authTypeValue : undefined;
            const selectedProviderType = selectedAuthType
              ? (providerTypes.find((tp) => tp.type === selectedAuthType) ??
                null)
              : null;
            const selectedSchema = (selectedProviderType?.config_schema ??
              null) as Record<string, unknown> | null;

            return (
              <>
                {/* Step 0: Type selection + basic info */}
                <div style={{ display: step === 0 ? "block" : "none" }}>
                  <Form.Item
                    name="auth_type"
                    label={t("authProviders.type")}
                    rules={[{ required: true }]}
                  >
                    <Select
                      options={providerTypeOptions}
                      loading={providerTypesLoading}
                      showSearch={true}
                      optionFilterProp="label"
                      size="large"
                    />
                  </Form.Item>

                  {/* Type description cards */}
                  {selectedAuthType ? (
                    <Card
                      size="small"
                      style={{
                        marginBottom: 16,
                        background:
                          "var(--ant-color-bg-container-disabled, #fafafa)",
                      }}
                    >
                      <Space>
                        <span style={{ fontSize: 20 }}>
                          {providerTypeIcons[selectedAuthType] || (
                            <SettingOutlined />
                          )}
                        </span>
                        <div>
                          <Space size={6} wrap>
                            <Text strong={true}>
                              {authProviderTypeLabel(
                                selectedAuthType,
                                selectedProviderType?.display_name ||
                                  selectedAuthType ||
                                  "",
                                t,
                              )}
                            </Text>
                            {renderAuthProviderAlphaTag(t)}
                          </Space>
                          <br />
                          <Text type="secondary" style={{ fontSize: 13 }}>
                            {authProviderTypeDescription(
                              selectedAuthType,
                              selectedProviderType?.description,
                              t,
                            )}
                          </Text>
                          <br />
                          <Text type="warning" style={{ fontSize: 13 }}>
                            {t("authProviders.alpha_description", {
                              defaultValue:
                                "Authentication provider integrations are currently in alpha. They are not yet fully validated and may not work reliably in every environment.",
                            })}
                          </Text>
                        </div>
                      </Space>
                    </Card>
                  ) : null}

                  <Form.Item
                    name="name"
                    label={t("common:table.name")}
                    rules={[{ required: true }]}
                  >
                    <Input placeholder={t("authProviders.name_placeholder")} />
                  </Form.Item>
                  <Row gutter={16}>
                    <Col span={12}>
                      <Form.Item
                        name="sort_order"
                        label={t("authProviders.sort_order")}
                      >
                        <InputNumber min={0} style={{ width: "100%" }} />
                      </Form.Item>
                    </Col>
                    <Col span={12}>
                      <Form.Item
                        name="enabled"
                        label={t("common:table.status")}
                        valuePropName="checked"
                      >
                        <Switch />
                      </Form.Item>
                    </Col>
                  </Row>
                </div>

                {/* Step 1: Schema-driven configuration */}
                <div style={{ display: step === 1 ? "block" : "none" }}>
                  <SchemaConfigForm
                    schema={
                      selectedSchema as Parameters<
                        typeof SchemaConfigForm
                      >[0]["schema"]
                    }
                    form={form}
                    namePrefix="config"
                    showJsonFallback={false}
                    applySchemaDefaults={true}
                    schemaNamespace={
                      selectedAuthType
                        ? `authProviders.schema.${selectedAuthType}`
                        : undefined
                    }
                  />
                </div>

                {/* Step 2: Confirmation summary */}
                <div style={{ display: step === 2 ? "block" : "none" }}>
                  <Card size="small" title={t("authProviders.wizard.summary")}>
                    <Space
                      direction="vertical"
                      size={4}
                      style={{ width: "100%" }}
                    >
                      <Text>
                        <Text strong={true}>{t("authProviders.type")}:</Text>{" "}
                        {authProviderTypeLabel(
                          selectedAuthType,
                          selectedProviderType?.display_name || selectedAuthType || "",
                          t,
                        )}
                      </Text>
                      <Text>
                        <Text strong={true}>{t("common:table.name")}:</Text>{" "}
                        {formInstance.getFieldValue("name") as string}
                      </Text>
                      <Text>
                        <Text strong={true}>{t("common:table.status")}:</Text>{" "}
                        <Tag
                          color={
                            formInstance.getFieldValue("enabled")
                              ? "green"
                              : "default"
                          }
                        >
                          {formInstance.getFieldValue("enabled")
                            ? t("users.status.enabled")
                            : t("users.status.disabled")}
                        </Tag>
                      </Text>
                    </Space>
                  </Card>
                </div>
              </>
            );
          }}
        </Form.Item>
      </Form>
    </Modal>
  );
}

interface MappingFormFieldsProps {
  form: FormInstance;
  discoveredCohortOptions?: DiscoveredCohortOption[];
  discoveredCohortsLoading?: boolean;
  scopeTargetOptionsByType?: Record<string, ScopeTargetOption[]>;
  scopeTargetLoadingByType?: Record<string, boolean>;
  roleOptions: Array<{ value: string; label: string }>;
  scopeOptions: Array<{ value: string; label: string }>;
  environmentOptions: Array<{ value: string; label: string }>;
  t: (key: string, opts?: Record<string, unknown>) => string;
  showCohortFields?: boolean;
}

function MappingFormFields({
  discoveredCohortOptions = [],
  discoveredCohortsLoading = false,
  environmentOptions,
  form,
  roleOptions,
  scopeTargetLoadingByType = {},
  scopeTargetOptionsByType = {},
  scopeOptions,
  showCohortFields = false,
  t,
}: MappingFormFieldsProps) {
  const handleDiscoveredCohortChange = (value?: string) => {
    if (!value) {
      return;
    }
    const selected = discoveredCohortOptions.find(
      (option) => option.value === value,
    );
    if (!selected) {
      return;
    }
    form.setFieldsValue({
      selected_cohort_ref: value,
      cohort_kind: selected.cohortKind,
      cohort_key: selected.cohortKey,
      cohort_display_name: selected.cohortDisplayName,
    });
  };
  const handleScopeTypeChange = (value: string) => {
    form.setFieldsValue({
      scope_type: value,
      scope_id: undefined,
    });
  };

  return (
    <>
      {showCohortFields ? (
        <>
          <Form.Item
            name="selected_cohort_ref"
            label={t("authProviders.mapping.discovered")}
            extra={t("authProviders.mapping.discovered_help", {
              defaultValue:
                "Choose a discovered cohort to prefill the canonical mapping fields, or keep typing them manually.",
            })}
          >
            <Select
              allowClear={true}
              showSearch={true}
              optionFilterProp="label"
              loading={discoveredCohortsLoading}
              options={discoveredCohortOptions}
              placeholder={t("authProviders.mapping.discovered_placeholder", {
                defaultValue: "Select an observed cohort",
              })}
              notFoundContent={t("authProviders.mapping.discovered_empty", {
                defaultValue: "No discovered cohorts yet",
              })}
              onChange={(value) => handleDiscoveredCohortChange(value)}
            />
          </Form.Item>
          <Form.Item
            name="cohort_kind"
            label={t("authProviders.mapping.cohort_kind")}
            rules={[{ required: true }]}
          >
            <Input />
          </Form.Item>
          <Form.Item
            name="cohort_key"
            label={t("authProviders.mapping.cohort_key")}
            rules={[{ required: true }]}
          >
            <Input />
          </Form.Item>
          <Form.Item
            name="cohort_display_name"
            label={t("authProviders.mapping.cohort_display_name")}
          >
            <Input />
          </Form.Item>
        </>
      ) : null}
      <Form.Item
        name="role_id"
        label={t("authProviders.mapping.role")}
        rules={[{ required: true }]}
      >
        <Select options={roleOptions} />
      </Form.Item>
      <Form.Item
        name="scope_type"
        label={t("authProviders.mapping.scope_type")}
      >
        <Select
          options={scopeOptions}
          onChange={(value) => handleScopeTypeChange(value)}
        />
      </Form.Item>
      <Form.Item noStyle={true} shouldUpdate={true}>
        {(formInstance) => {
          const scopeTypeValue = formInstance.getFieldValue("scope_type");
          const scopeType =
            typeof scopeTypeValue === "string" && scopeTypeValue.trim()
              ? scopeTypeValue
              : "global";
          if (scopeType === "global") {
            return null;
          }

          const scopeTargetOptions = scopeTargetOptionsByType[scopeType] ?? [];
          const scopeTargetLoading =
            scopeTargetLoadingByType[scopeType] ?? false;

          return (
            <Form.Item
              name="scope_id"
              label={t("authProviders.mapping.scope_id")}
              extra={t("authProviders.mapping.scope_id_help", {
                defaultValue:
                  "Choose a known {{scope}} target or type the exact ID if it is not listed yet.",
                scope: t(`rbac.scope.${scopeType}`, {
                  defaultValue: scopeType,
                }),
              })}
            >
              <AutoComplete
                options={scopeTargetOptions}
                allowClear={true}
                placeholder={t("authProviders.mapping.scope_id_placeholder", {
                  defaultValue: "Paste the target resource ID",
                })}
                filterOption={(inputValue, option) => {
                  const label = String(option?.label ?? "").toLowerCase();
                  const value = String(option?.value ?? "").toLowerCase();
                  const search = inputValue.trim().toLowerCase();
                  return label.includes(search) || value.includes(search);
                }}
                notFoundContent={
                  scopeTargetLoading
                    ? t("common:status.loading", { defaultValue: "Loading…" })
                    : t("authProviders.mapping.scope_target_empty", {
                        defaultValue: "No suggested targets yet",
                      })
                }
              />
            </Form.Item>
          );
        }}
      </Form.Item>
      <Form.Item
        name="allowed_environments"
        label={t("authProviders.mapping.envs")}
      >
        <Select mode="multiple" options={environmentOptions} />
      </Form.Item>
    </>
  );
}

// ── EditProviderModal ────────────────────────────────────────────────────

interface EditProviderModalProps {
  open: boolean;
  form: FormInstance;
  editingProvider: AuthProvider | null;
  providerTypes: AuthProviderType[];
  onSubmit: () => void;
  onCancel: () => void;
  confirmLoading: boolean;
  t: (key: string, opts?: Record<string, unknown>) => string;
}

function EditProviderModal({
  open,
  form,
  editingProvider,
  providerTypes,
  onSubmit,
  onCancel,
  confirmLoading,
  t,
}: EditProviderModalProps) {
  const editProviderType = editingProvider?.auth_type
    ? (providerTypes.find((tp) => tp.type === editingProvider.auth_type) ??
      null)
    : null;
  const editSchema = (editProviderType?.config_schema ?? null) as Record<
    string,
    unknown
  > | null;

  return (
    <Modal
      title={t("authProviders.edit_title", {
        name: editingProvider?.name || "",
      })}
      open={open}
      onOk={onSubmit}
      onCancel={onCancel}
      confirmLoading={confirmLoading}
      width={640}
      maskClosable={false}
      keyboard={false}
      rootClassName="copy-friendly-actions"
      data-testid="auth-provider-edit-modal"
    >
      <Form form={form} layout="vertical">
        <Form.Item
          name="name"
          label={t("common:table.name")}
          rules={[{ required: true }]}
        >
          <Input />
        </Form.Item>
        <Row gutter={16}>
          <Col span={12}>
            <Form.Item name="sort_order" label={t("authProviders.sort_order")}>
              <InputNumber min={0} style={{ width: "100%" }} />
            </Form.Item>
          </Col>
          <Col span={12}>
            <Form.Item
              name="enabled"
              label={t("common:table.status")}
              valuePropName="checked"
            >
              <Switch />
            </Form.Item>
          </Col>
        </Row>
        <SchemaConfigForm
          schema={
            editSchema as Parameters<typeof SchemaConfigForm>[0]["schema"]
          }
          form={form}
          namePrefix="config"
          showJsonFallback={false}
          schemaNamespace={
            editingProvider?.auth_type
              ? `authProviders.schema.${editingProvider.auth_type}`
              : undefined
          }
        />
      </Form>
    </Modal>
  );
}
