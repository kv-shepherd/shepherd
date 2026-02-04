---
# MADR 4.0 compatible metadata (YAML frontmatter)
status: "accepted"
date: 2026-02-02
deciders: []
consulted: []
informed: []
---

# ADR-0028: oapi-codegen Optional Field Strategy with Go 1.25 omitzero

> **Review Period**: Until 2026-02-04 (48-hour minimum)  
> **Discussion**: [Issue #83](https://github.com/kv-shepherd/shepherd/issues/83)  
> **Supplements**: [ADR-0021 §Go Code Generation](./ADR-0021-api-contract-first.md)

---

## Context and Problem Statement

ADR-0021 established oapi-codegen as the tool for generating Go server code from OpenAPI specifications, but did not specify how to handle optional and nullable fields in the generated Go types.

By default, oapi-codegen generates pointer types (`*string`, `*int`) for optional fields, leading to:
- Verbose `if ptr != nil { ... }` checks throughout business logic
- Increased risk of nil pointer panics
- Reduced code readability and maintainability

This problem is commonly referred to as "Pointer Hell" in the Go community.

With Go 1.24 introducing the `omitzero` JSON struct tag and oapi-codegen v2.5.0+ supporting the `prefer-skip-optional-pointer-with-omitzero` output option, we now have an opportunity to significantly reduce pointer usage while maintaining correct JSON serialization behavior.

## Decision Drivers

* **Go 1.25 compatibility**: Project targets Go 1.25.x, enabling full `omitzero` support
* **Code quality**: Reduce nil pointer checks and potential panics
* **Developer experience**: Cleaner, more readable generated code
* **JSON correctness**: Must correctly handle optional fields (omit when zero) vs nullable fields (explicit null)
* **API contract alignment**: Generated Go types must match OpenAPI semantics

## Considered Options

* **Option 1**: Default pointer generation (status quo)
* **Option 2**: Use `github.com/oapi-codegen/nullable` package for three-state representation
* **Option 3**: Configure `prefer-skip-optional-pointer-with-omitzero` globally (proposed)

## Decision Outcome

**Chosen option**: "Option 3: Configure `prefer-skip-optional-pointer-with-omitzero` globally", because it leverages Go 1.25's native `omitzero` support to eliminate unnecessary pointers while maintaining correct JSON serialization semantics.

### Consequences

* ✅ Good, because optional fields become value types with `omitzero` tag, eliminating pointer checks
* ✅ Good, because generated code is more readable and idiomatic Go
* ✅ Good, because nil pointer panic risk is significantly reduced
* 🟡 Neutral, because nullable fields still require pointers (this is semantically correct)
* ❌ Bad, because requires Go 1.24+ (mitigated by project's Go 1.25 baseline)

### Confirmation

* oapi-codegen config file includes `prefer-skip-optional-pointer-with-omitzero: true`
* Generated types use value types with `json:",omitzero"` for optional-only fields
* Generated types use pointers for `nullable: true` fields
* CI enforces Go 1.25+ version check

---

## Pros and Cons of the Options

### Option 1: Default Pointer Generation (Status Quo)

Continue with oapi-codegen's default behavior of generating pointers for optional fields.

* ✅ Good, because no configuration changes needed
* ✅ Good, because works with all Go versions
* ❌ Bad, because requires extensive nil checks in business logic
* ❌ Bad, because increases nil pointer panic risk
* ❌ Bad, because makes code verbose and harder to read

### Option 2: Use `github.com/oapi-codegen/nullable` Package

Use the nullable package to represent three-state values (set, null, undefined).

* ✅ Good, because provides explicit three-state representation
* ✅ Good, because useful for PATCH operations where distinguishing "not provided" from "null" matters
* ❌ Bad, because adds external dependency
* ❌ Bad, because nullable types have different API than standard Go types
* ❌ Bad, because may be overkill for most use cases where two-state (present/absent) is sufficient

### Option 3: Configure `prefer-skip-optional-pointer-with-omitzero` (Proposed)

Enable oapi-codegen's native support for Go 1.24+ `omitzero` tag.

* ✅ Good, because uses Go standard library feature
* ✅ Good, because generates idiomatic Go code
* ✅ Good, because no external dependencies
* ✅ Good, because automatic based on configuration
* 🟡 Neutral, because nullable fields still require pointers (semantically correct)
* ❌ Bad, because requires Go 1.24+ (acceptable given project baseline)

---

## More Information

### oapi-codegen Configuration

Update `oapi-codegen.yaml` (or equivalent configuration file):

```yaml
# oapi-codegen configuration
package: api
generate:
  models: true
  gin-server: true
output: internal/api/api.gen.go
output-options:
  # Go 1.24+ omitzero support
  prefer-skip-optional-pointer-with-omitzero: true
```

### Field Generation Rules

| OpenAPI Specification | Generated Go Type | JSON Tag |
|-----------------------|-------------------|----------|
| `type: string` (required) | `string` | `json:"field"` |
| `type: string` (optional, no nullable) | `string` | `json:"field,omitzero"` |
| `type: string` + `nullable: true` | `*string` | `json:"field,omitempty"` |
| `type: string` (optional) + `nullable: true` | `*string` | `json:"field,omitempty"` |

### Example: Before and After

**Before (default pointer generation):**

```go
type VMCreateRequest struct {
    Name        string  `json:"name"`
    Description *string `json:"description,omitempty"`  // optional
    Memory      *int    `json:"memory,omitempty"`       // optional
    CPUs        *int    `json:"cpus,omitempty"`         // optional
}

// Business logic requires many nil checks
func CreateVM(req VMCreateRequest) {
    desc := ""
    if req.Description != nil {
        desc = *req.Description
    }
    // ... more nil checks
}
```

**After (with omitzero):**

```go
type VMCreateRequest struct {
    Name        string `json:"name"`
    Description string `json:"description,omitzero"`  // optional, zero value = omit
    Memory      int    `json:"memory,omitzero"`       // optional, zero value = omit
    CPUs        int    `json:"cpus,omitzero"`         // optional, zero value = omit
}

// Business logic is cleaner
func CreateVM(req VMCreateRequest) {
    desc := req.Description  // direct access, zero value is valid
    // ... no nil checks needed
}
```

### Nullable Fields (Still Use Pointers)

For fields that are both optional AND nullable (where `null` has semantic meaning distinct from "not provided"):

```yaml
# OpenAPI
properties:
  deletedAt:
    type: string
    format: date-time
    nullable: true
    description: "null means not deleted, absent means unchanged"
```

```go
// Generated Go
type Entity struct {
    DeletedAt *time.Time `json:"deletedAt,omitempty"`
}
```

### Related Decisions

* [ADR-0021](./ADR-0021-api-contract-first.md) - API Contract-First Design with OpenAPI

### References

* [Go 1.24 Release Notes - omitzero](https://go.dev/doc/go1.24)
* [Go Issue #45669 - omitzero proposal](https://github.com/golang/go/issues/45669)
* [oapi-codegen Configuration Options](https://github.com/oapi-codegen/oapi-codegen)
* [oapi-codegen nullable package](https://github.com/oapi-codegen/nullable)

### Implementation Notes

1. Update `go.mod` to require Go 1.25
2. Update oapi-codegen configuration
3. Regenerate all API code
4. Update any manual code that relied on pointer semantics
5. CI should enforce minimum Go version

---

## Changelog

| Date | Author | Change |
|------|--------|--------|
| 2026-02-02 | @jindyzhao | Initial draft |
