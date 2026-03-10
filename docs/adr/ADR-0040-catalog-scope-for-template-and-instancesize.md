---
status: "accepted"
date: 2026-03-06
deciders: ["@jindyzhao"]
consulted: []
informed: []
---

# ADR-0040: Catalog Scope for Template and InstanceSize

> **Review Period**: Until 2026-03-08 (48-hour minimum)<br>
> **Accepted On**: 2026-03-10<br>
> **Amends**: `ADR-0036-template-instancesize-boundary-enforcement.md`, `ADR-0018-instance-size-abstraction.md`<br>
> **Relates To**: `ADR-0015-governance-model-v2.md#7-environment-aware-approval-policies`, `ADR-0015-governance-model-v2.md#15-namespace-cluster-binding-and-environment-type-clarification`

---

## Context and Problem Statement

The platform already has a clear environment model for scheduling and approval: Namespace and Cluster carry an explicit environment type, and that type is one of `test` or `prod`. Namespace names remain free-form (`dev`, `staging`, `uat`, `prod`, `dr`, etc.), but those names are mapped by administrators to the environment type used by RBAC and approval rules.

Template and InstanceSize need an explicit visibility boundary in the VM request flow. Today, [`GetVMRequestContext`](../../internal/api/handlers/server_vm.go) returns all enabled templates and instance sizes, so a user operating in a `test` namespace can see catalog items intended only for `prod` workloads.

The Python production system solved this with a `permission_group` field and later removed the older `environment` field from templates and instance sizes. That production history is important: the requirement is catalog classification, not a third environment model.

**Core question**: How should Template and InstanceSize be classified for user-facing catalog visibility without overloading the meaning of `environment` or weakening zero-trust defaults?

## Decision Drivers

* **Zero-trust default**: Unclassified catalog items must not become visible to regular users by accident
* **Semantic clarity**: `environment` on Namespace/Cluster already means environment type (`test`/`prod`)
* **Production parity**: Python production uses a dedicated catalog classification field (`permission_group`), not `template.environment`
* **Separation of concerns**: Catalog visibility, RBAC, and cluster capability matching are different problems
* **Multi-cluster reality**: A catalog item being visible in `prod` does not imply every `prod` cluster can run it

## Considered Options

* **Option 1**: Add explicit `catalog_scope` to Template and InstanceSize (chosen)
* **Option 2**: Add free-form `environment` string to Template and InstanceSize
* **Option 3**: Reuse RBAC visibility only

## Decision Outcome

**Chosen option**: "Add explicit `catalog_scope` to Template and InstanceSize", because it keeps the existing `environment` semantics intact, aligns with zero-trust defaults, and matches the real production requirement: catalog visibility classification.

### Consequences

* ✅ Good, because Namespace/Cluster `environment` keeps a single meaning: `test` or `prod`
* ✅ Good, because catalog visibility becomes explicit and auditable
* ✅ Good, because `unclassified` is safe by default and does not leak into user request flows
* ✅ Good, because cluster compatibility remains a separate validation concern instead of being hidden inside a visibility field
* 🟡 Neutral, because administrators must classify catalog items before they appear to end users
* ❌ Bad, because catalog maintainers must still review and classify seeded or experimental entries before exposing them to end users; mitigation: keep `unclassified` hidden from regular request flows until reviewed

### Confirmation

1. `GetVMRequestContext` returns only catalog items whose `catalog_scope` matches the selected namespace environment type or is `all`
2. `unclassified` catalog items are hidden from regular user request flows
3. Admin catalog APIs can still list `unclassified` items for remediation
4. `CreateVMRequest` rejects stale or forged selections where template/size scope does not match the selected namespace environment type

---

## Pros and Cons of the Options

### Option 1: Add explicit `catalog_scope` to Template and InstanceSize

Use a dedicated field that represents catalog visibility only.

```go
field.Enum("catalog_scope").
    Values("unclassified", "test", "prod", "all").
    Default("unclassified").
    Comment("Catalog visibility scope only. Not scheduling environment.")
```

Visibility rules:

```go
// namespaceEnv is namespaceregistry.EnvironmentTest or EnvironmentProd.
// User-facing request context excludes unclassified by default.
where := enttemplate.Or(
    enttemplate.CatalogScopeEQ(template.CatalogScopeAll),
    enttemplate.CatalogScopeEQ(template.CatalogScope(namespaceEnv)),
)
```

Defense-in-depth validation:

```go
if tmpl.CatalogScope != template.CatalogScopeAll &&
   tmpl.CatalogScope != template.CatalogScope(namespaceEnv) {
    return apperrors.BadRequest("CATALOG_SCOPE_MISMATCH", "template scope does not match namespace environment")
}
```

* ✅ Good, because it does not overload `environment`
* ✅ Good, because `unclassified` is safer than `NULL = all`
* ✅ Good, because it aligns with the production concept of `permission_group`
* ✅ Good, because it leaves cluster capability checks to approval-time validators
* ❌ Bad, because it adds an extra classification step for admins

### Option 2: Add free-form `environment` string to Template and InstanceSize

Add a nullable string such as `dev`, `staging`, `uat`, `prod`, `dr`.

* ✅ Good, because it looks flexible at first glance
* ❌ Bad, because it conflicts with the existing platform meaning of `environment`
* ❌ Bad, because it creates two incompatible environment models in the same system
* ❌ Bad, because it mixes namespace naming conventions with approval/scheduling semantics

### Option 3: Reuse RBAC visibility only

Only use `RoleBinding.allowed_environments` and do not classify catalog items.

* ✅ Good, because it avoids adding a new field
* ❌ Bad, because RBAC answers "who can access", not "which catalog items belong in this environment type"
* ❌ Bad, because a `prod`-only template would still appear in `test` request flows for authorized users

---

## More Information

### Related Decisions

* `ADR-0015-governance-model-v2.md` - Environment type on Namespace/Cluster remains `test`/`prod`
* `ADR-0018-instance-size-abstraction.md` - InstanceSize remains the canonical hardware-layer model
* `ADR-0036-template-instancesize-boundary-enforcement.md` - Template and InstanceSize remain separate concerns

### References

* Python production model: `oms-kubevirt-python/app/models/vm_template.py`
* Python production model: `oms-kubevirt-python/app/models/instance_size.py`
* Python migration removing old environment fields: `oms-kubevirt-python/migrations/035_unify_permission_groups.py`

### Implementation Notes

**Schema changes**:

| Entity | Field | Type | Default | Notes |
|--------|-------|------|---------|-------|
| `Template` | `catalog_scope` | enum | `unclassified` | User-facing visibility only |
| `InstanceSize` | `catalog_scope` | enum | `unclassified` | User-facing visibility only |

**Query behavior**:

| API Surface | Behavior |
|-------------|----------|
| User request context | Only `matching env type` + `all` |
| User create request | Reject mismatched scope at write-time |
| Admin catalog list | Show all scopes, including `unclassified` |

**Important non-goal**:

This ADR does **not** solve cluster capability or cluster policy matching. A template or instance size being visible in a `prod` namespace does not imply every `prod` cluster can run it. Capability and policy checks remain part of approval-time validation and future cluster-policy work.

**Revisit criteria**:

Revisit if the project later needs per-cluster catalog allowlists or multi-dimensional catalog segmentation beyond `test/prod/all/unclassified`.

---

## Changelog

| Date | Author | Change |
|------|--------|--------|
| 2026-03-06 | @jindyzhao | Initial draft |
| 2026-03-06 | @jindyzhao | Reworked around `catalog_scope`, zero-trust defaults, and separation from environment type |
| 2026-03-10 | @jindyzhao | Status promoted to accepted after review period and implementation merge |
