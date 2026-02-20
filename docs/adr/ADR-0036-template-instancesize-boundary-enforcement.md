---
status: "proposed"
date: 2026-02-20
deciders: []
consulted: []
informed: []
---

# ADR-0036: Template / InstanceSize Boundary Enforcement

> **Review Period**: Until 2026-02-22 (48-hour minimum)<br>
> **Amends**: `ADR-0018-instance-size-abstraction.md#core-design-principles`

---

## Context and Problem Statement

ADR-0018 established a critical separation of concerns between Template (software baseline: OS image, cloud-init) and InstanceSize (hardware capabilities: CPU, memory, disk, GPU, SR-IOV, hugepages). However, the current implementation does not enforce this boundary:

1. **Template `spec` field** (`type: object, additionalProperties: true`) accepts arbitrary JSON, including hardware paths such as `cpu`, `memory`, `resources`, and `domain.cpu`.
2. **Backend handlers** pass Template spec directly to the database without content boundary validation.
3. **InstanceSize `spec_overrides`** JSONB field has no enforcement of the required `spec.*` path prefix.
4. No mechanism detects conflicts between `spec_overrides` values and the indexed scheduling columns (`cpu_cores`, `memory_mb`, etc.).

This creates a "Template explosion" anti-pattern where administrators embed hardware configurations inside templates, leading to tight coupling, N×M maintenance complexity, and silent override conflicts during VM creation when Template spec and InstanceSize parameters are merged.

## Decision Drivers

* **ADR-0018 Compliance**: The existing ADR explicitly separates Template (software) from InstanceSize (hardware)
* **Operational Safety**: Prevent hardware configuration from leaking into templates where it cannot be queried or validated
* **Fail-Fast Validation**: Catch architectural violations at write-time rather than during VM creation
* **Zero False Positives**: The allowlist/blocklist must not reject legitimate template configurations (image, cloud-init)

## Considered Options

* **Option 1**: Backend Prohibited-Path Validation (chosen)
* **Option 2**: OpenAPI Schema Restriction via `oneOf`/`allOf`
* **Option 3**: Frontend-Only Validation

## Decision Outcome

**Chosen option**: "Backend Prohibited-Path Validation", because it enforces the boundary at the API layer where it cannot be bypassed, provides immediate feedback with actionable error messages, and requires no schema changes to the existing OpenAPI contract.

### Consequences

* ✅ Good, because hardware configuration in Templates is blocked at write-time
* ✅ Good, because existing API schema (`type: object, additionalProperties: true`) remains unchanged
* ✅ Good, because validation errors include the specific prohibited path and guidance to use InstanceSize
* 🟡 Neutral, because administrators with existing templates containing hardware paths must migrate those values to InstanceSize before updating the template — a one-time data cleanup
* ❌ Bad, because the prohibited prefix list requires manual maintenance when KubeVirt adds new hardware-related spec paths — mitigated by periodic reviews during KubeVirt version upgrades

### Confirmation

1. Unit tests in `internal/service/template_validator_test.go` cover:
   - All allowed paths (image, cloud_init, pvc_name, volumes, source)
   - All prohibited paths (cpu, memory, resources, domain.*, requires_*, dedicated_cpu, hugepages_size)
   - Recursive nested map detection
   - Case-insensitive path matching
2. Unit tests in `internal/service/instancesize_validator_test.go` cover:
   - `spec.*` prefix enforcement for spec_overrides
   - Conflict detection between spec_overrides and indexed columns
   - Dedicated CPU + overcommit consistency validation
3. Handler integration: both `CreateAdminTemplate` and `UpdateAdminTemplate` return HTTP 400 with error code `TEMPLATE_SPEC_VIOLATION` for any prohibited path
4. Handler integration: both `CreateAdminInstanceSize` and `UpdateAdminInstanceSize` return HTTP 400 with error code `INVALID_SPEC_OVERRIDES` for invalid paths

---

## Pros and Cons of the Options

### Option 1: Backend Prohibited-Path Validation

Server-side validation using a prohibited-prefix list that recursively checks all keys in the Template spec JSON against known hardware configuration paths.

* ✅ Good, because it enforces at the API layer (cannot be bypassed by alternative clients)
* ✅ Good, because it preserves backward compatibility with the existing OpenAPI schema
* ✅ Good, because it provides clear, actionable error messages mentioning both the path and the correct target (InstanceSize)
* 🟡 Neutral, because the prefix list requires periodic updates when KubeVirt adds new spec paths
* ❌ Bad, because it's a blocklist approach (new unknown hardware paths are allowed until the list is updated)

### Option 2: OpenAPI Schema Restriction via `oneOf`/`allOf`

Restructure the OpenAPI `TemplateCreateRequest.spec` to only allow specific properties (image, cloud_init, etc.) by removing `additionalProperties: true`.

* ✅ Good, because validation happens at schema level, automatically enforced by generated code
* ❌ Bad, because it requires regenerating all API types and updating existing clients
* ❌ Bad, because it prevents administrators from storing any future-proof metadata in template spec
* ❌ Bad, because `oapi-codegen` struct generation doesn't support complex `oneOf` validation well

### Option 3: Frontend-Only Validation

Replace the Template `spec_text` TextArea with semantic form fields, preventing hardware input.

* ✅ Good, because it provides a better UX with dedicated fields
* ❌ Bad, because it can be bypassed via direct API calls (curl, scripts, automation)
* ❌ Bad, because it doesn't protect against programmatic template creation

---

## More Information

### Related Decisions

* `ADR-0018-instance-size-abstraction.md` - Parent decision defining the Template/InstanceSize separation
* `ADR-0007-template-storage.md` - Template storage design (PostgreSQL)
* `ADR-0021-api-contract-first.md` - API contract-first approach

### Implementation Notes

**Files created/modified:**

| File | Change |
|------|--------|
| `internal/service/template_validator.go` | New: `ValidateTemplateSpec()` with prohibited prefix list |
| `internal/service/template_validator_test.go` | New: 50+ test cases covering allowed/prohibited paths |
| `internal/service/instancesize_validator.go` | New: `ValidateSpecOverrides()` + `DetectSpecOverridesConflicts()` |
| `internal/service/instancesize_validator_test.go` | New: spec_overrides path and conflict detection tests |
| `internal/api/handlers/server_admin_catalog.go` | Modified: integrated validators into Create/Update handlers |

**Error codes:**

| Code | HTTP Status | When |
|------|-------------|------|
| `TEMPLATE_SPEC_VIOLATION` | 400 | Template spec contains hardware configuration paths |
| `INVALID_SPEC_OVERRIDES` | 400 | InstanceSize spec_overrides uses non-`spec.*` paths |

**Future phases** (not in this ADR scope):
- Frontend semantic form for Templates (image + cloud-init only)
- Dual-engine UI for InstanceSize spec_overrides (Form + YAML)
- Full KubeVirt JSON Schema + Mask infrastructure

**Revisit when**: KubeVirt major version introduces new hardware-related spec paths that should be added to the prohibited prefix list.

---

## Changelog

| Date | Author | Change |
|------|--------|--------|
| 2026-02-20 | jindyzhao | Initial draft |
