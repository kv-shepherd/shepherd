---
# MADR 4.0 compatible metadata (YAML frontmatter)
status: "accepted"  # proposed | accepted | deprecated | superseded by ADR-XXXX
date: 2026-02-03
deciders: []  # GitHub usernames of decision makers
consulted: []  # Subject-matter experts consulted (two-way communication)
informed: []  # Stakeholders kept up-to-date (one-way communication)
---

# ADR-0029: OpenAPI Toolchain Governance

> **Accepted**: 2026-02-05 (48-hour review period completed)  
> **Discussion**: [Issue #96](https://github.com/kv-shepherd/shepherd/issues/96)  
> **Amends**: [ADR-0021 §Technology Stack](./ADR-0021-api-contract-first.md)

---

## Context and Problem Statement

ADR-0021 established the Contract-First API design principle with OpenAPI. However, the specific toolchain for **linting, validation, and overlay processing** was not fully specified. The current proposed tooling in DEPENDENCIES.md introduces inconsistencies:

1. **Mixed language dependencies**: `spectral` (Node.js), `oas-patch` (Python), while project is Go-based
2. **Validation gaps**: `kin-openapi` lacks strict mode for detecting undeclared fields
3. **CI efficiency**: Node.js-based linters are slower and require additional runtime

We need a unified, Go-native toolchain that provides **strict contract enforcement** in CI pipelines.

## Decision Drivers

* **Go-native backend tooling**: Align backend validation/linting with project's primary language; TypeScript generation remains Node.js-based per ADR-0021
* **Strict validation**: Catch undeclared fields, schema violations at CI stage
* **Performance**: Fast feedback in CI pipelines
* **Governance-first**: Tools should enforce constraints, not just report them

---

## Considered Options

* **Option 1**: Go-native toolchain (vacuum + libopenapi-validator)
* **Option 2**: Maintain current mixed toolchain (spectral + kin-openapi + oas-patch)
* **Option 3**: Full libopenapi ecosystem (replace oapi-codegen with custom generation)

---

## Decision Outcome

**Chosen option**: "Option 1: Go-native toolchain", because it provides strict validation, unified language stack, and optimal CI performance while preserving ADR-0021's core decisions (oapi-codegen for code generation).

### Toolchain Selection

| Layer | Tool | Replaces | Rationale |
|-------|------|----------|-----------|
| **Linting** | `vacuum` | spectral | Go-native, 10x faster, Spectral-rule compatible |
| **Runtime Validation** | `libopenapi-validator` | kin-openapi (validation) | Strict mode, undeclared field detection |
| **Overlay Processing** | `libopenapi` | oas-patch | Go-native, same ecosystem |
| **Code Generation** | `oapi-codegen` | (unchanged) | ADR-0021 decision preserved |
| **TypeScript Types** | `openapi-typescript` | (unchanged) | ADR-0021 decision preserved |

### CI Enforcement Gates

All gates are **blocking** (fail CI if violated):

| Gate | Tool | Check |
|------|------|-------|
| **Spec Lint** | vacuum | `--fail-severity warn` |
| **Code Sync** | oapi-codegen | Generated code matches spec |
| **Type Sync** | openapi-typescript | Generated types match spec |
| **Contract Test** | libopenapi-validator | Strict mode validation suite |

### Consequences

* ✅ Good, because **backend** tooling is Go-native (no Node.js/Python for validation/linting)
* ✅ Good, because strict mode catches undeclared fields automatically
* ✅ Good, because vacuum is 10x faster than spectral in CI
* ✅ Good, because oapi-codegen integration preserved (ADR-0021 compliance)
* ✅ Good, because vacuum is fully compatible with Spectral rulesets (see migration guide)
* 🟡 Neutral, because adds libopenapi-validator as new dependency
* 🟡 Neutral, because TypeScript type generation (`openapi-typescript`) still requires Node.js per ADR-0021 (mitigated via Makefile graceful fallback and containerized option)
* ❌ Bad, because CI pipelines require both Go and Node.js environments (mitigated via separate CI jobs)
* ❌ Bad, because existing spectral rulesets may need minor adaptation for custom JS functions

### Confirmation

* [ ] CI pipeline includes all four gates as blocking checks
* [ ] CI actions are pinned to specific versions or commit SHAs (supply chain security)
* [ ] libopenapi-validator StrictMode is enabled in validation middleware
* [ ] Production environment returns generic validation errors only (no information disclosure)
* [ ] No Python or Node.js dependencies in production Docker image

---

## Pros and Cons of the Options

### Option 1: Go-native toolchain (Recommended)

* ✅ Good, because unified Go ecosystem
* ✅ Good, because StrictMode catches undeclared fields
* ✅ Good, because static binaries, no runtime dependencies
* ✅ Good, because vacuum compatible with Spectral rules
* 🟡 Neutral, because requires adding libopenapi-validator
* ❌ Bad, because migration from spectral needed

### Option 2: Maintain mixed toolchain

* ✅ Good, because no migration effort
* ❌ Bad, because Python + Node.js dependencies in CI
* ❌ Bad, because kin-openapi lacks strict validation mode
* ❌ Bad, because slower CI feedback cycles

### Option 3: Full libopenapi ecosystem

* ✅ Good, because complete ecosystem consistency
* ❌ Bad, because replacing oapi-codegen violates ADR-0021
* ❌ Bad, because libopenapi code generation less mature

---

## Documents Requiring Updates

Upon acceptance, the following documents require updates:

| Document | Action | Description |
|----------|--------|-------------|
| `docs/design/DEPENDENCIES.md` | UPDATE | Add libopenapi, libopenapi-validator; mark spectral/oas-patch as replaced |
| `Makefile` | ADD | Add `api-lint`, `api-validate`, `api-check` targets |
| `.github/workflows/api-contract.yaml` | UPDATE | Add vacuum lint gate, libopenapi-validator gate |
| `internal/api/middleware/openapi_validator.go` | CREATE | Implement StrictMode validation middleware |
| `api/.vacuum.yaml` | CREATE | Vacuum ruleset configuration |
| `go.mod` | UPDATE | Add `github.com/pb33f/libopenapi`, `github.com/pb33f/libopenapi-validator` |

---

## More Information

### Related Decisions

* [ADR-0021](./ADR-0021-api-contract-first.md) - API Contract-First Design (this ADR amends toolchain details)

### References

* [vacuum - World's fastest OpenAPI linter](https://github.com/daveshanley/vacuum)
* [libopenapi-validator - Strict validation](https://github.com/pb33f/libopenapi-validator)
* [pb33f OpenAPI ecosystem](https://pb33f.io/)

### Implementation Notes

For detailed implementation specifications including CI pipeline configuration and Makefile targets, see:

**→ [ADR-0029 Implementation Details](../design/notes/ADR-0029-openapi-toolchain-implementation.md)**

---

## Changelog

| Date | Author | Change |
|------|--------|--------|
| 2026-02-03 | @jindyzhao | Initial draft |

---

_End of ADR-0029_
