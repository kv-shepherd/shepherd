# Design Note: ADR-0048 — Directory Sync Capability for Auth Providers

> **Status**: Accepted (ADR-0048 accepted on 2026-03-23)
> **Related ADR**: [ADR-0048](../../adr/ADR-0048-directory-sync-capability.md)
> **Owner**: @jindyzhao
> **Created**: 2026-03-19
> **Last Updated**: 2026-03-20

## Summary

ADR-0048 no longer standardizes a universal vendor workflow such as "department
tree + attribute picker + preview wizard".

Instead:

* the provider owns the sync request shape and acquisition workflow
* the core owns canonical preview/import result shape, conflict classification,
  async job semantics, and persistence invariants

This note captures the implementation shape that follows that boundary.

It also aligns the preview/import record wording with the normalized
`external cohort` standard introduced in the ADR-0049 draft.

Admin-only read models for this workspace may live under
`internal/edge/authworkspace/...`, split into narrower subpackages such as
`runtimeview` and `directoryview`, so that HTTP handlers stay thin while core
and provider code remain isolated. `runtimeview` owns runtime/login-mode DTO
construction; `directoryview` owns preview/job/schedule DTO construction,
including list pagination and unsupported schedule-state DTOs.

## Scope

- In scope: optional `DirectorySyncCapability` contract on auth-provider adapters
- In scope: provider-defined `request_schema`
- In scope: canonical preview/sync result contract
- In scope: conflict classification before persistence
- In scope: async sync jobs and auditability
- In scope: dedicated storage for provider-specific raw profile attributes
- Out of scope: hardcoded department/attribute workflow as a core assumption
- Out of scope: provider-specific frontend flows in core
- Out of scope: using `User.metadata` for raw directory attributes

---

## 1. Capability Contract

### 1.1 Go Interface

Primary contract file: `internal/provider/directorycontract/contract.go`

`internal/provider/directory_sync.go` may remain as a thin re-export layer while
the broader provider package is reduced.

```go
package provider

import "context"

// DirectorySyncDescriptor tells core how to collect provider-owned request input.
type DirectorySyncDescriptor struct {
    DisplayName     string                 `json:"display_name"`
    Description     string                 `json:"description,omitempty"`
    RequestSchema   map[string]interface{} `json:"request_schema,omitempty"`
    SupportsPreview bool                   `json:"supports_preview"`
}

type DirectoryCohortRef struct {
    Kind        string `json:"kind"`
    Key         string `json:"key"`
    DisplayName string `json:"display_name,omitempty"`
}

// DirectoryUserRecord is the canonical user-import contract consumed by core.
type DirectoryUserRecord struct {
    ExternalID  string                 `json:"external_id"`
    Username    string                 `json:"username"`
    DisplayName string                 `json:"display_name"`
    Email       string                 `json:"email,omitempty"`
    Cohorts     []DirectoryCohortRef   `json:"cohorts,omitempty"`
    Attributes  map[string]interface{} `json:"attributes,omitempty"` // raw provider attributes, non-authoritative
}

// DirectoryConflict represents a canonical classification result.
type DirectoryConflict struct {
    Code           string `json:"code"` // same_external_identity | username_conflict | email_conflict | ambiguous_existing_user
    Field          string `json:"field,omitempty"`
    ExistingUserID string `json:"existing_user_id,omitempty"`
    Message        string `json:"message,omitempty"`
}

// DirectoryPreviewItem is what core shows in preview responses.
type DirectoryPreviewMatch struct {
    Action         string `json:"action"` // create | update | blocked
    ExistingUserID string `json:"existing_user_id,omitempty"`
    MatchedBy      string `json:"matched_by,omitempty"` // external_id
}

type DirectoryPreviewItem struct {
    Record    DirectoryUserRecord  `json:"record"`
    Match     DirectoryPreviewMatch `json:"match"`
    Conflicts []DirectoryConflict  `json:"conflicts,omitempty"`
    Warnings  []string             `json:"warnings,omitempty"`
}

type DirectorySyncPreview struct {
    TotalCount int                    `json:"total_count"`
    Items      []DirectoryPreviewItem `json:"items"`
}

type DirectorySyncCapability interface {
    DescribeDirectorySync() DirectorySyncDescriptor
    PreviewDirectorySync(ctx context.Context, config map[string]interface{}, providerRequest map[string]interface{}) (*DirectorySyncPreview, error)
    ListDirectoryUsers(ctx context.Context, config map[string]interface{}, providerRequest map[string]interface{}) ([]DirectoryUserRecord, error)
}
```

### 1.2 Boundary Rules

Core is allowed to know:

* `request_schema`
* canonical user record fields
* canonical conflict types
* canonical job states

Core is not allowed to know:

* whether the provider selected departments, groups, OUs, tags, filters, or cursors
* provider-specific request keys
* provider-specific attribute names outside the raw `attributes` blob

Normalized cohorts are non-authoritative organizational references. They may be
used later by explicit mapping/batch-management services, but they are not
runtime permissions.

### 1.3 Why this shape

This keeps the interface small and stable while avoiding the earlier mistake of
turning vendor workflow vocabulary into core API shape.

### 1.4 Public plugin SDK exposure

Implementation must expose the directory-sync capability through the public
plugin SDK, not only through `internal/provider`.

Required follow-up at implementation time:

* re-export `DirectorySyncCapability` from `pkg/authproviderplugin`
* re-export the canonical directory-sync DTOs used by plugin authors
* keep third-party plugins from importing `internal/provider` directly

This follows the shared-SDK pattern already used for admin auth-provider
plugins and keeps the host/plugin contract small, stable, and importable by
external plugin code.

---

## 2. Persistence Model

### 2.1 `User` remains canonical

Canonical imported identity fields continue to live on `User`:

* `auth_provider_id`
* `external_id`
* `username`
* `display_name`
* `email`

Do **not** add `User.metadata` for provider-specific directory attributes.

### 2.2 New `UserDirectoryProfile` projection

File: `ent/schema/user_directory_profile.go`

```go
package schema

import (
    "entgo.io/ent"
    "entgo.io/ent/schema/edge"
    "entgo.io/ent/schema/field"
)

// UserDirectoryProfile stores non-authoritative raw directory attributes.
type UserDirectoryProfile struct {
    ent.Schema
}

func (UserDirectoryProfile) Mixin() []ent.Mixin {
    return []ent.Mixin{TimeMixin{}}
}

func (UserDirectoryProfile) Fields() []ent.Field {
    return []ent.Field{
        field.String("id").Unique().Immutable(),
        field.JSON("attributes", map[string]interface{}{}).
            Comment("Raw provider-specific directory attributes; informational only"),
        field.Time("last_synced_at"),
    }
}

func (UserDirectoryProfile) Edges() []ent.Edge {
    return []ent.Edge{
        edge.From("user", User.Type).Ref("directory_profile").Unique().Required(),
    }
}
```

Rule:

* `UserDirectoryProfile.attributes` is display/audit data only.
* Auth/RBAC/approval/policy/runtime code must not read it for authoritative
  behavior.

### 2.3 `DirectorySyncJob`

File: `ent/schema/directory_sync_job.go`

```go
type DirectorySyncJob struct {
    ent.Schema
}

func (DirectorySyncJob) Fields() []ent.Field {
    return []ent.Field{
        field.String("id").Unique().Immutable(),
        field.String("auth_provider_id").NotEmpty(),
        field.Enum("status").
            Values("pending", "running", "completed", "failed").
            Default("pending"),
        field.JSON("request_snapshot", map[string]interface{}{}).
            Comment("Opaque provider_request payload frozen at trigger time"),
        field.String("conflict_resolution").Default("skip"),
        field.Int("total_entries").Default(0),
        field.Int("create_count").Default(0),
        field.Int("update_count").Default(0),
        field.Int("blocked_count").Default(0),
        field.Int("error_count").Default(0),
        field.JSON("errors", []string{}).Optional(),
        field.String("triggered_by").NotEmpty(),
        field.Time("started_at").Optional().Nillable(),
        field.Time("completed_at").Optional().Nillable(),
    }
}
```

---

## 3. Conflict Classification

Conflict classification happens before writes.

### 3.1 Required classes

| Code | Meaning | Typical action |
|------|---------|----------------|
| `same_external_identity` | Same `(auth_provider_id, external_id)` | update / skip |
| `username_conflict` | `username` already belongs to another user | error or manual resolve |
| `email_conflict` | `email` already belongs to another user | error or manual resolve |
| `ambiguous_existing_user` | Existing local/imported user cannot be safely linked | error |

### 3.2 Worker rule

The worker must not treat a database unique violation as normal conflict
classification. It must resolve conflicts first, then write.

### 3.3 Preview rule

Preview responses include canonical `conflicts` so administrators see how a sync
will behave before enqueue.

### 3.4 Execution summary rule

Async job results must reuse the same canonical action vocabulary as preview:

- `create`
- `update`
- `blocked`

`manual_import` and `scheduled_enrichment` may differ in source flow, but their
persisted job summary must expose the same action buckets so admin UI and
automation do not reintroduce provider-specific reasoning.

---

## 4. API Shape

### 4.1 `GET /admin/auth-providers/{id}/directory/descriptor`

Returns capability metadata:

```json
{
  "display_name": "WeCom Directory Sync",
  "description": "Import users from enterprise contacts",
  "supports_preview": true,
  "request_schema": {
    "type": "object",
    "properties": {
      "departments": { "type": "array", "items": { "type": "string" } },
      "include_nested": { "type": "boolean", "default": true },
      "selected_fields": { "type": "array", "items": { "type": "string" } }
    }
  }
}
```

The same endpoint for another provider might expose a completely different
schema. Core must not care.

### 4.2 `POST /admin/auth-providers/{id}/directory/preview`

Request:

```json
{
  "provider_request": {
    "departments": ["2", "3"],
    "include_nested": true
  },
  "conflict_resolution": "skip"
}
```

Response:

```json
{
  "total_count": 2,
  "items": [
    {
      "record": {
        "external_id": "zhangsan",
        "username": "zhangsan",
        "display_name": "Zhang San",
        "email": "zhangsan@example.com",
        "cohorts": [
          { "kind": "department", "key": "wecom:department:2", "display_name": "Engineering" },
          { "kind": "tag", "key": "wecom:tag:tech", "display_name": "Tech" }
        ],
        "attributes": { "department_name": "Engineering" }
      },
      "conflicts": []
    }
  ]
}
```

This endpoint is synchronous and read-only. It validates the opaque
`provider_request`, asks the adapter for a canonical preview, and does not
enqueue a River job.

### 4.3 `POST /admin/auth-providers/{id}/directory/sync`

Request:

```json
{
  "provider_request": {
    "departments": ["2", "3"],
    "include_nested": true
  },
  "conflict_resolution": "skip"
}
```

Response:

```json
{
  "job_id": "dsj_abc123",
  "status": "pending"
}
```

This endpoint is asynchronous. It freezes `provider_request` into
`DirectorySyncJob.request_snapshot` and enqueues the River worker.

### 4.4 `GET /admin/auth-providers/{id}/directory/sync-jobs`

Standard paginated list response.

### 4.5 `GET /admin/auth-providers/{id}/directory/sync-jobs/{jobId}`

Returns canonical job detail:

* standard execution metadata
* standard `result_summary` buckets using the same `create / update / blocked`
  vocabulary as preview
* opaque `request_snapshot` for debugging/replay

It still does not expose provider workflow internals as first-class fields.

### 4.6 Current admin workbench baseline

The current host implementation already exposes a minimal provider-agnostic
directory workbench around these APIs:

* runtime/schedule/jobs state remains visible in the same admin surface
* the provider-owned `request_schema` is rendered as a schema-driven request
  form
* preview shows canonical rows, normalized conflicts, and warnings
* preview also exposes a result-first summary split into `ready`,
  `warning`, and `conflict` buckets so administrators can understand match
  quality before reading the detailed table
* preview now carries a canonical `match` object so the host can distinguish
  `create`, safe `update`, and `blocked` rows without guessing from raw
  conflict arrays
* preview can be filtered by those same standard result buckets and surfaces
  aggregated conflict codes without introducing provider-specific navigation
* preview summary cards may link to the latest relevant sync job detail using
  the same canonical action vocabulary, for example jumping from a blocked
  summary card to the latest job whose `result_summary.blocked_count > 0`
* conflict-heavy previews may also render grouped summaries for canonical
  conflict codes such as `username_conflict` or `ambiguous_existing_user`,
  but that grouping remains derived from the existing canonical result shape
* grouped conflict summaries may expand into per-code detail sections, but
  those sections still reuse the same canonical preview rows instead of
  introducing provider-owned sub-workflows
* warning-heavy previews may apply the same pattern, grouping repeated warning
  messages into expandable detail sections derived from the canonical preview
  rows
* manual sync launches an async job and relies on the standard jobs list for
  follow-up
* jobs list can be filtered by the same canonical action vocabulary
  (`create / update / blocked`) instead of provider-specific outcome labels
* a job detail surface may show result-summary tags, execution metadata,
  errors, and opaque request snapshot, but should continue to avoid
  provider-specific workflow widgets
* handler-local view-model shaping for this admin workbench may be extracted
  into an `internal/edge/...` package so HTTP handlers keep orchestration
  concerns while edge workspace code owns provider-neutral admin DTO mapping
* the same pattern can cover schedule status shaping as well: handlers load the
  latest/pending job rows, while `internal/edge/...` code computes the standard
  `supported/enabled/next_run` view model from the normalized plan
* runtime descriptor construction and directory-capability resolution may also
  move into `internal/edge/...` helpers so admin handlers stay focused on HTTP
  flow, permission gates, and error mapping

This is intentionally a result-first workbench. It must not grow
provider-specific navigation models such as department trees, OU explorers, or
vendor wizards into the shared UI/API.

### 4.7 Schema-driven object field handling

Provider-owned request payloads may legitimately contain nested objects. The
schema-driven form layer should therefore treat `type: object` fields as local
JSON editing surfaces:

* edit-time defaults should be rendered as formatted JSON text
* submit-time values should be parsed back into JSON objects before request
  transmission
* invalid JSON must be surfaced to administrators as a request-form error

This keeps the shared workbench generic while still supporting provider-owned
nested request structures.

---

## 5. Worker Flow

File: `internal/jobs/directory_sync_worker.go`

### 5.1 Job args

```go
type DirectorySyncArgs struct {
    AuthProviderID string `json:"auth_provider_id"`
    JobID          string `json:"job_id"`
}
```

### 5.2 Execution steps

1. Load auth provider and job row
2. Resolve adapter from auth-provider registry
3. Type-assert `DirectorySyncCapability`
4. Load `request_snapshot` and `conflict_resolution`
5. Call `ListDirectoryUsers(config, request_snapshot)`
6. For each canonical `DirectoryUserRecord`:
   - classify conflicts
   - create/update canonical `User`
   - upsert `UserDirectoryProfile.attributes`
7. Update `DirectorySyncJob` result summary using canonical action buckets
   (`create / update / blocked`)

### 5.3 Explicit non-goals

The worker must not:

* parse provider-specific request keys
* branch on provider type
* treat raw attributes as authoritative core data

---

## 6. Frontend Integration

### 6.1 Reuse schema-driven form machinery

The existing auth-provider admin UI already renders `config_schema` dynamically.
Directory sync should follow the same pattern:

* fetch `descriptor`
* render `descriptor.request_schema`
* send opaque `provider_request`
* display canonical preview table and job list

### 6.2 Default UI is generic, not prescriptive

Core may ship a default request form and preview table, but that UI must be
generic:

* no hardcoded department tree assumption
* no hardcoded attribute selector assumption
* no provider-type `switch` in frontend

### 6.3 Future extension

If the platform later gains frontend plugin surfaces, providers may supply richer
custom UX without changing the backend contract defined here.

---

## 7. Acceptance Criteria

* Adapters without `DirectorySyncCapability` return `501` from directory-sync endpoints.
* The request payload is always carried under `provider_request`; handlers/workers do not inspect provider-specific keys.
* `go test ./internal/jobs/... -run TestDirectorySyncWorker` covers conflict classification for `external_id`, `username`, and `email`.
* `go test ./internal/api/handlers/... -run TestDirectorySyncPreviewUsesOpaqueProviderRequest` proves handler code never parses vendor-specific request keys.
* `pkg/authproviderplugin` re-exports the directory-sync capability and canonical DTOs needed by third-party auth-provider plugins.
* `UserDirectoryProfile.attributes` is never read by auth/RBAC/approval/runtime paths.
* provider results may expose normalized cohorts beyond plain groups without changing the core contract.
* CI auth-provider boundary checks keep passing.

## Revisit Conditions

- frontend plugin/runtime extension becomes available
- multi-provider identity linking is introduced
- scheduled or push-based sync becomes first-class
- raw profile data needs lifecycle separate from imported user linkage
