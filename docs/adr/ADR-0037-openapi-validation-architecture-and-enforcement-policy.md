---
# MADR 4.0 compatible metadata (YAML frontmatter)
status: "accepted"  # proposed | accepted | deprecated | superseded by ADR-XXXX
date: 2026-02-28
deciders:
  - jindyzhao
consulted: []
informed: []
---

# ADR-0037: OpenAPI Validation Architecture and Enforcement Policy

> **Status**: Accepted<br>
> **Discussion**: [Issue #280](https://github.com/kv-shepherd/shepherd/issues/280)<br>
> **Amends**: [ADR-0021](./ADR-0021-api-contract-first.md), [ADR-0029](./ADR-0029-openapi-toolchain-governance.md)

---

## Context and Problem Statement

The project has an accepted Contract-First direction (ADR-0021) and OpenAPI toolchain governance direction (ADR-0029), but implementation and acceptance criteria are still partially inconsistent. We need one explicit policy for runtime validation, business validation, and CI enforcement that can be executed and verified end-to-end.

This decision is made before production rollout, so architecture can be optimized for best-state consistency instead of migration minimization.

## Decision Drivers

* Contract correctness must be enforced before merge, not by convention.
* Validation rules should be spec-driven to avoid duplicated handler logic.
* Runtime behavior must be deterministic across environments.
* Governance controls (required checks/branch protection) must be mandatory.
* Decision quality should be evidence-first and measurable.

## Considered Options

* **Option 1**: Keep current mixed state (`kin-openapi` runtime + scattered handler checks)
* **Option 2**: Best-state target (`libopenapi-validator` strict runtime + spec-driven business validation + enforced CI gates)
* **Option 3**: Hybrid deferment (keep current runtime now, evaluate strict runtime later)

## Decision Outcome

**Chosen option**: "Option 2", because it provides the strongest contract enforcement model and removes known consistency gaps while the project still has no production migration constraints.

### Core Policy

1. **Runtime OpenAPI validation**: use `github.com/pb33f/libopenapi-validator` with strict mode.
   - Runtime validator must load the **canonical** OpenAPI spec from independent embed bytes (`internal/api/specembed/openapi.yaml`), not from `generated.GetSwagger()`.
   - Validator lifecycle policy is fixed as **per-validation instance creation** (no shared instance reuse, no `sync.Pool` reuse for validator instances).
2. **Business validation**: use `github.com/go-playground/validator/v10`.
3. **Validation source-of-truth**: define `validate` tags via `x-oapi-codegen-extra-tags` in OpenAPI schema.
4. **Lint and diff enforcement**:
   - keep Vacuum as linter
   - fail CI on lint errors
   - enforce breaking-change checks on PRs
5. **Governance enforcement**:
   - required status checks on `main`
   - no bypass as default policy
6. **Compatibility guardrail**:
   - provide `make api-diff` alias to `api-breaking` for workflow and issue compatibility.
   - `api/openapi.compat.yaml` is a derived artifact for **Go codegen only**, not for runtime validation, linting, or frontend type generation.
   - Compat removal verification must run on canonical path first: disable/remove compat artifact, then verify canonical codegen succeeds before deleting compat pipeline steps.

### Consequences

* ✅ Good, because spec-to-runtime consistency becomes enforceable and testable.
* ✅ Good, because validation logic moves from duplicated handlers to generated contract + unified validator layer.
* ✅ Good, because strict runtime checks surface schema drift early in CI and tests.
* ✅ Good, because branch governance becomes measurable and auditable.
* 🟡 Neutral, because initial implementation scope is broader and requires staged PR rollout.
* ❌ Bad, because strict validation may initially reject requests previously tolerated by permissive checks.

### Confirmation

This ADR is considered correctly implemented when all items below are true:

1. `main` branch has required checks for API contract workflow and core CI workflow.
2. `make api-lint` reports `0 errors`.
3. Runtime middleware uses `libopenapi-validator` strict mode and has coverage for:
   - undeclared body fields
   - undeclared query parameters
   - production-safe error responses
   - canonical spec source verification (runtime validator does not consume compat-downgraded spec)
   - `-race` coverage for middleware path
4. `internal/api/validator` is used for business validation and produces standardized field error output.
5. Request schema validation tags are declared in OpenAPI via `x-oapi-codegen-extra-tags`.
6. PR pipeline performs breaking-change detection and blocks on violations.
7. Generated-code CI path is deterministic and ordered: `api-compat-generate -> api-generate -> api-compat -> sync diff`.

---

## Pros and Cons of the Options

### Option 1: Keep Current Mixed State

* ✅ Good, because immediate implementation churn is low.
* ❌ Bad, because validation remains fragmented and inconsistent.
* ❌ Bad, because governance and acceptance remain ambiguous.

### Option 2: Best-State Target (Selected)

* ✅ Good, because contract enforcement becomes complete across lint/runtime/business/CI.
* ✅ Good, because rules are spec-driven and maintainable.
* ✅ Good, because it matches pre-launch optimization conditions.
* ❌ Bad, because rollout requires disciplined serial PR execution.

### Option 3: Hybrid Deferment

* ✅ Good, because initial risk appears lower.
* ❌ Bad, because known consistency debt is intentionally retained.
* ❌ Bad, because issue closure criteria remain partially non-verifiable.

---

## More Information

### Related Decisions

* [ADR-0021](./ADR-0021-api-contract-first.md) - Contract-First API direction
* [ADR-0029](./ADR-0029-openapi-toolchain-governance.md) - OpenAPI toolchain governance baseline

### References

* [Issue #280](https://github.com/kv-shepherd/shepherd/issues/280)
* [oapi-codegen: x-oapi-codegen-extra-tags](https://github.com/oapi-codegen/oapi-codegen#x-oapi-codegen-extra-tags---generate-arbitrary-struct-tags-to-fields)
* [oapi-codegen OpenAPI 3.1 support status](https://github.com/oapi-codegen/oapi-codegen#does-oapi-codegen-support-openapi-31)
* [libopenapi-validator](https://github.com/pb33f/libopenapi-validator)
* [libopenapi-validator/strict](https://pkg.go.dev/github.com/pb33f/libopenapi-validator/strict)
* [go-playground/validator](https://github.com/go-playground/validator)
* [Go embed package](https://pkg.go.dev/embed)

### Implementation Notes

1. Detailed rollout and file-level execution are tracked in:
   - `docs/design/notes/ADR-0037-openapi-validation-architecture-and-enforcement-policy.md`
2. Normative design specs follow this accepted ADR.
3. After acceptance, append amendment blocks to prior ADRs as needed.

---

## Changelog

| Date | Author | Change |
|------|--------|--------|
| 2026-02-26 | @jindyzhao | Initial draft |
| 2026-02-26 | @jindyzhao | Sync remediation/design-note corrections: canonical runtime spec source, per-validation validator lifecycle, compat exit verification order |

---

_End of ADR-0037_
