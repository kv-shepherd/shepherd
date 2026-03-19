# Design Note: ADR-0048 — Directory Sync Capability for Auth Providers

> **Status**: Proposed (ADR-0048 under review until 2026-03-21)
> **Related ADR**: [ADR-0048](../../adr/ADR-0048-directory-sync-capability.md)
> **Owner**: @jindyzhao
> **Created**: 2026-03-19
> **Last Updated**: 2026-03-19

## Summary

ADR-0048 no longer standardizes a universal vendor workflow such as "department
tree + attribute picker + preview wizard".

Instead:

* the provider owns the sync request shape and acquisition workflow
* the core owns canonical preview/import result shape, conflict classification,
  async job semantics, and persistence invariants

This note captures the implementation shape that follows that boundary.

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

File: `internal/provider/directory_sync.go`

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

// DirectoryUserRecord is the canonical user-import contract consumed by core.
type DirectoryUserRecord struct {
    ExternalID  string                 `json:"external_id"`
    Username    string                 `json:"username"`
    DisplayName string                 `json:"display_name"`
    Email       string                 `json:"email,omitempty"`
    Groups      []string               `json:"groups,omitempty"`
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
type DirectoryPreviewItem struct {
    Record    DirectoryUserRecord  `json:"record"`
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

### 1.3 Why this shape

This keeps the interface small and stable while avoiding the earlier mistake of
turning vendor workflow vocabulary into core API shape.

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
        field.Int("created_count").Default(0),
        field.Int("updated_count").Default(0),
        field.Int("skipped_count").Default(0),
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
        "display_name": "张三",
        "email": "zhangsan@example.com",
        "groups": ["tech"],
        "attributes": { "department_name": "技术部" }
      },
      "conflicts": []
    }
  ]
}
```

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

### 4.4 `GET /admin/auth-providers/{id}/directory/sync-jobs`

Standard paginated list response.

### 4.5 `GET /admin/auth-providers/{id}/directory/sync-jobs/{jobId}`

Returns job summary and counts only. It does not expose provider workflow
internals as first-class fields.

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
7. Update `DirectorySyncJob` counters

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
* `UserDirectoryProfile.attributes` is never read by auth/RBAC/approval/runtime paths.
* CI auth-provider boundary checks keep passing.

## Revisit Conditions

- frontend plugin/runtime extension becomes available
- multi-provider identity linking is introduced
- scheduled or push-based sync becomes first-class
- raw profile data needs lifecycle separate from imported user linkage
