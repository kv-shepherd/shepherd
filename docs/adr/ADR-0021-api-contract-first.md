---
# MADR 4.0 compatible metadata (YAML frontmatter)
status: "accepted"  # proposed | accepted | deprecated | superseded by ADR-XXXX
date: 2026-01-27
deciders: []  # GitHub usernames of decision makers
consulted: []  # Subject-matter experts consulted (two-way communication)
informed: []  # Stakeholders kept up-to-date (one-way communication)
---

# ADR-0021: API Contract-First Design with OpenAPI

> **Review Period**: Until 2026-01-30 (48-hour minimum)  
> **Discussion**: [Issue #31](https://github.com/kv-shepherd/shepherd/issues/31)  
> **Related**: [ADR-0020](./ADR-0020-frontend-technology-stack.md) (Frontend Technology Stack)

---

## Context and Problem Statement

KubeVirt Shepherd requires a well-defined API contract between:

1. **Backend (Go)**: Provides RESTful APIs for all platform operations
2. **Frontend (React/TypeScript)**: Consumes APIs for UI rendering
3. **External Integrations**: Third-party systems that may integrate with Shepherd

Currently, API contracts are implicitly defined in Go code (structs, handlers), leading to:

- **Type drift**: Frontend TypeScript types manually maintained, prone to mismatch
- **Documentation lag**: API docs become outdated as code evolves
- **Boilerplate code**: DTO structs, validation logic, and handlers are hand-written
- **Verbal contracts**: API changes require manual coordination between teams

We need a formalized approach where the API specification is the **single source of truth**.

---

## Decision Drivers

* **Type safety across stack**: Frontend TypeScript and Backend Go must share identical type definitions
* **Documentation accuracy**: API documentation should never drift from implementation
* **Developer productivity**: Reduce boilerplate code through generation
* **Validation consistency**: Request validation should be derived from specification
* **Change management**: API changes should be explicitly versioned and reviewed
* **Industry alignment**: Follow widely adopted API design standards

---

## Considered Options

* **Option 1**: Contract-First with OpenAPI + oapi-codegen
* **Option 2**: Code-First with swaggo/swag (generate spec from Go annotations)
* **Option 3**: GraphQL with gqlgen
* **Option 4**: gRPC with Protocol Buffers

---

## Decision Outcome

**Recommended option**: "Option 1: Contract-First with OpenAPI + oapi-codegen", because it provides the strongest contract guarantees, bidirectional code generation, and REST API compatibility with existing infrastructure.

### Core Principles

1. **Specification as Truth**: The OpenAPI YAML file is the authoritative API definition
2. **Bidirectional Generation**: Generate both Go server code AND TypeScript client types
3. **Validation from Spec**: Request validation derived from OpenAPI schema
4. **No Hand-Written DTOs**: All request/response types are generated

### Technology Stack

| Component | Technology | Purpose |
|-----------|------------|---------|
| **Specification Format** | OpenAPI 3.1 | API definition language |
| **Go Server Generation** | oapi-codegen | Generate Gin handlers, models, validation |
| **TypeScript Generation** | openapi-typescript | Generate TypeScript types for frontend |
| **Spec Validation** | spectral | Lint OpenAPI specs for quality |
| **Interactive Docs** | Scalar (or Swagger UI) | Developer-facing API documentation |
| **Mock Server** | Prism | Development-time API mocking |

### OpenAPI Version Policy

This project adopts **OpenAPI 3.1** as the specification format. Version selection rationale:

| Version | Status | Notes |
|---------|--------|-------|
| OpenAPI 3.1 | ✅ Adopted | Full JSON Schema compatibility, webhooks support |
| OpenAPI 3.0 | ⚠️ Compatible | May be used for tooling compatibility if needed |
| OpenAPI 2.0 (Swagger) | ❌ Not supported | Legacy format, lacks key features |

**Compatibility Note**: If an external tool only supports OpenAPI 3.0, use `@redocly/cli` to downgrade the spec for that specific use case. The source of truth remains 3.1.

### Consequences

* ✅ Good, because API types are guaranteed consistent between frontend and backend
* ✅ Good, because documentation is always accurate (generated from spec)
* ✅ Good, because validation logic is derived from spec, not hand-written
* ✅ Good, because API changes require spec changes first (explicit review)
* ✅ Good, because oapi-codegen integrates well with Gin router
* 🟡 Neutral, because requires learning OpenAPI specification syntax
* 🟡 Neutral, because adds a code generation step to build pipeline
* ❌ Bad, because complex validation logic may need custom extensions

---

## Implementation

### Directory Structure

```
api/
├── openapi.yaml              # Main OpenAPI specification (single source of truth)
├── schemas/                  # Reusable schema components
│   ├── common.yaml          # Pagination, ErrorResponse, etc.
│   ├── vm.yaml              # VM-related schemas
│   ├── approval.yaml        # Approval workflow schemas
│   ├── governance.yaml      # System, Service, RBAC schemas
│   └── instance-size.yaml   # Instance size schemas
├── paths/                    # Path definitions (optional, can inline in openapi.yaml)
│   ├── vms.yaml
│   ├── approvals.yaml
│   └── admin.yaml
└── generated/                # Generated code (gitignored except for review)
    ├── server/              # Go server types and interfaces
    └── client/              # TypeScript client types

internal/
├── api/
│   ├── server.go            # Generated server interface implementations
│   ├── handlers/            # Hand-written handler logic (implements generated interfaces)
│   │   ├── vm_handler.go
│   │   ├── approval_handler.go
│   │   └── admin_handler.go
│   └── middleware/
│       └── error_mapper.go  # Map domain errors to HTTP responses
```

### Workflow

```
┌─────────────────────────────────────────────────────────────────────────────┐
│  Contract-First Development Workflow                                        │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  1. Design API (spec first)                                                 │
│     │                                                                        │
│     ▼                                                                        │
│  ┌──────────────────────┐                                                   │
│  │  api/openapi.yaml    │  ◄── Single Source of Truth                       │
│  └──────────────────────┘                                                   │
│     │                                                                        │
│     ├──────────────────────────────┬─────────────────────────────────┐      │
│     ▼                              ▼                                 ▼      │
│  ┌────────────────┐    ┌─────────────────────┐    ┌─────────────────────┐  │
│  │ oapi-codegen   │    │ openapi-typescript  │    │ Scalar/Swagger UI   │  │
│  │ (Go generator) │    │ (TS generator)      │    │ (Documentation)     │  │
│  └────────────────┘    └─────────────────────┘    └─────────────────────┘  │
│     │                              │                                        │
│     ▼                              ▼                                        │
│  ┌────────────────┐    ┌─────────────────────┐                             │
│  │ Go interfaces  │    │ TypeScript types    │                             │
│  │ + models       │    │ + API client        │                             │
│  └────────────────┘    └─────────────────────┘                             │
│     │                              │                                        │
│     ▼                              ▼                                        │
│  ┌────────────────┐    ┌─────────────────────┐                             │
│  │ Handler impls  │    │ React components    │                             │
│  │ (hand-written) │    │ (hand-written)      │                             │
│  └────────────────┘    └─────────────────────┘                             │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

### OpenAPI Specification Example

```yaml
# api/openapi.yaml
openapi: 3.1.0
info:
  title: KubeVirt Shepherd API
  version: 1.0.0
  description: Multi-cluster KubeVirt management platform

servers:
  - url: /api/v1
    description: API v1

paths:
  /vms:
    get:
      operationId: listVMs
      tags: [VMs]
      summary: List virtual machines
      parameters:
        - $ref: '#/components/parameters/ServiceIdFilter'
        - $ref: '#/components/parameters/Pagination'
      responses:
        '200':
          description: List of VMs
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/VMListResponse'
        '400':
          $ref: '#/components/responses/BadRequest'
        '401':
          $ref: '#/components/responses/Unauthorized'

  /vms/{id}:
    get:
      operationId: getVM
      tags: [VMs]
      summary: Get VM by ID
      parameters:
        - $ref: '#/components/parameters/VMId'
      responses:
        '200':
          description: VM details
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/VM'
        '404':
          $ref: '#/components/responses/NotFound'

components:
  schemas:
    VM:
      type: object
      required: [id, name, status, serviceId]
      properties:
        id:
          type: string
          format: uuid
        name:
          type: string
          pattern: '^[a-z][a-z0-9-]*[a-z0-9]$'
          minLength: 3
          maxLength: 63
        status:
          $ref: '#/components/schemas/VMStatus'
        serviceId:
          type: string
          format: uuid

    VMStatus:
      type: string
      enum: [pending, approved, provisioning, running, stopped, failed, deleted]

    ErrorResponse:
      type: object
      required: [code, message]
      properties:
        code:
          type: string
          description: Machine-readable error code
        message:
          type: string
          description: Human-readable error message
        details:
          type: object
          additionalProperties: true

  responses:
    BadRequest:
      description: Invalid request
      content:
        application/json:
          schema:
            $ref: '#/components/schemas/ErrorResponse'
    NotFound:
      description: Resource not found
      content:
        application/json:
          schema:
            $ref: '#/components/schemas/ErrorResponse'
    Unauthorized:
      description: Authentication required
      content:
        application/json:
          schema:
            $ref: '#/components/schemas/ErrorResponse'
```

### Go Code Generation

```yaml
# oapi-codegen.yaml
package: api
generate:
  gin-server: true
  models: true
  embedded-spec: true
output: internal/api/generated/server.gen.go
```

```bash
# Generate Go code
oapi-codegen -config oapi-codegen.yaml api/openapi.yaml
```

Generated interface (example):

```go
// internal/api/generated/server.gen.go (generated, do not edit)

// ServerInterface represents all server handlers.
type ServerInterface interface {
    // ListVMs (GET /vms)
    ListVMs(c *gin.Context, params ListVMsParams)
    // GetVM (GET /vms/{id})
    GetVM(c *gin.Context, id string)
    // CreateVM (POST /vms)
    CreateVM(c *gin.Context)
}

// VM defines model for VM.
type VM struct {
    Id        string   `json:"id"`
    Name      string   `json:"name"`
    Status    VMStatus `json:"status"`
    ServiceId string   `json:"serviceId"`
}
```

Handler implementation:

```go
// internal/api/handlers/vm_handler.go (hand-written)

type VMHandler struct {
    vmUseCase usecases.VMUseCase
}

func (h *VMHandler) GetVM(c *gin.Context, id string) {
    vm, err := h.vmUseCase.GetByID(c.Request.Context(), id)
    if err != nil {
        // Error mapping handled by middleware
        _ = c.Error(err)
        return
    }
    c.JSON(http.StatusOK, toAPIVM(vm))
}
```

### TypeScript Generation

```bash
# Generate TypeScript types (in shepherd-ui)
npx openapi-typescript ../shepherd/api/openapi.yaml -o src/types/api.gen.ts
```

Generated types:

```typescript
// src/types/api.gen.ts (generated)

export interface components {
  schemas: {
    VM: {
      id: string;
      name: string;
      status: "pending" | "approved" | "provisioning" | "running" | "stopped" | "failed" | "deleted";
      serviceId: string;
    };
    ErrorResponse: {
      code: string;
      message: string;
      details?: Record<string, unknown>;
    };
  };
}

export type VM = components["schemas"]["VM"];
export type VMStatus = components["schemas"]["VM"]["status"];
```

---

## Mock Server Integration

To enable parallel frontend-backend development, integrate a mock server that serves realistic responses based on the OpenAPI specification.

### Development Workflow with Mocking

```
┌─────────────────────────────────────────────────────────────────────────────┐
│  Parallel Development with Mock Server                                       │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  1. API Designer defines endpoints in openapi.yaml                           │
│     │                                                                        │
│     ├─────────────────────────────────┬─────────────────────────────────┐   │
│     ▼                                 ▼                                 │   │
│  ┌────────────────┐       ┌─────────────────────┐                       │   │
│  │ Backend Team   │       │ Frontend Team       │                       │   │
│  │ Implements API │       │ Uses Mock Server    │                       │   │
│  └────────────────┘       └─────────────────────┘                       │   │
│     │                                 │                                 │   │
│     │ (1-2 weeks)                     │ (immediate start)               │   │
│     ▼                                 ▼                                 │   │
│  ┌────────────────┐       ┌─────────────────────┐                       │   │
│  │ Real API Ready │ ◄──── │ Switch to Real API  │                       │   │
│  └────────────────┘       └─────────────────────┘                       │   │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Prism Mock Server Setup

```bash
# Install Prism
npm install -g @stoplight/prism-cli

# Run mock server
prism mock api/openapi.yaml --port 4010

# Frontend can now develop against http://localhost:4010
```

### Mock Data with Examples

```yaml
# In openapi.yaml, add examples for realistic mock responses
components:
  schemas:
    VM:
      type: object
      properties:
        id:
          type: string
          format: uuid
          example: "550e8400-e29b-41d4-a716-446655440000"
        name:
          type: string
          example: "web-server-01"
        status:
          type: string
          enum: [pending, running, stopped]
          example: "running"
```

---

## Error Handling Integration

Per the architecture improvement suggestions, implement a centralized error system:

### Domain Errors (Go)

```go
// internal/apperror/errors.go

package apperror

type Code string

const (
    CodeResourceNotFound  Code = "RESOURCE_NOT_FOUND"
    CodeQuotaExceeded     Code = "QUOTA_EXCEEDED"
    CodeApprovalRequired  Code = "APPROVAL_REQUIRED"
    CodeValidationFailed  Code = "VALIDATION_FAILED"
    CodePermissionDenied  Code = "PERMISSION_DENIED"
    CodeConflict          Code = "CONFLICT"
)

type AppError struct {
    Code    Code   `json:"code"`
    Message string `json:"message"`
    Cause   error  `json:"-"`
}

func (e *AppError) Error() string {
    return e.Message
}

func (e *AppError) Unwrap() error {
    return e.Cause
}

// Constructor functions
func NotFound(resource string) *AppError {
    return &AppError{
        Code:    CodeResourceNotFound,
        Message: fmt.Sprintf("%s not found", resource),
    }
}

func QuotaExceeded(reason string) *AppError {
    return &AppError{
        Code:    CodeQuotaExceeded,
        Message: reason,
    }
}
```

### Error Mapping Middleware

```go
// internal/api/middleware/error_mapper.go

func ErrorMapper() gin.HandlerFunc {
    return func(c *gin.Context) {
        c.Next()

        if len(c.Errors) == 0 {
            return
        }

        err := c.Errors.Last().Err
        var appErr *apperror.AppError
        if errors.As(err, &appErr) {
            status := mapCodeToHTTPStatus(appErr.Code)
            c.JSON(status, api.ErrorResponse{
                Code:    string(appErr.Code),
                Message: appErr.Message,
            })
            return
        }

        // Unknown error - log and return 500
        slog.Error("unhandled error", "error", err)
        c.JSON(http.StatusInternalServerError, api.ErrorResponse{
            Code:    "INTERNAL_ERROR",
            Message: "An unexpected error occurred",
        })
    }
}

func mapCodeToHTTPStatus(code apperror.Code) int {
    switch code {
    case apperror.CodeResourceNotFound:
        return http.StatusNotFound
    case apperror.CodeQuotaExceeded, apperror.CodePermissionDenied:
        return http.StatusForbidden
    case apperror.CodeApprovalRequired:
        return http.StatusForbidden
    case apperror.CodeValidationFailed:
        return http.StatusBadRequest
    case apperror.CodeConflict:
        return http.StatusConflict
    default:
        return http.StatusInternalServerError
    }
}
```

---

## Pros and Cons of the Options

### Option 1: Contract-First with OpenAPI + oapi-codegen (Recommended)

* ✅ Good, because spec is single source of truth
* ✅ Good, because bidirectional generation (Go + TypeScript)
* ✅ Good, because validation derived from spec
* ✅ Good, because widely adopted standard (OpenAPI)
* ✅ Good, because REST-compatible with existing infrastructure
* 🟡 Neutral, because requires maintaining YAML spec
* ❌ Bad, because complex custom validation may need extensions

### Option 2: Code-First with swaggo/swag

* ✅ Good, because Go code is primary artifact
* ✅ Good, because annotations inline with code
* ❌ Bad, because spec is derived, not authoritative
* ❌ Bad, because TypeScript types still need separate generation
* ❌ Bad, because docs can drift if annotations not updated

### Option 3: GraphQL with gqlgen

* ✅ Good, because strong typing with schema
* ✅ Good, because flexible querying
* ❌ Bad, because adds complexity for simple CRUD
* ❌ Bad, because less standard tooling for K8s ecosystem
* ❌ Bad, because steeper learning curve

### Option 4: gRPC with Protocol Buffers

* ✅ Good, because strongest type guarantees
* ✅ Good, because excellent performance
* ❌ Bad, because requires gRPC-Web gateway for browser clients
* ❌ Bad, because more complex infrastructure
* ❌ Bad, because less human-readable than REST

---

## Acceptance Checklist (Execution Tasks)

Upon acceptance, perform the following:

1. [ ] Create `api/` directory structure
2. [ ] Create initial `api/openapi.yaml` with core endpoints
3. [ ] Add `oapi-codegen` to Go dependencies (`tools.go` pattern)
4. [ ] Add generation scripts to `Makefile`:
   - `make generate-api`
   - `make validate-api`
   - `make mock-server`
5. [ ] Create `internal/apperror/` package for domain errors
6. [ ] Create error mapping middleware
7. [ ] Update frontend (`shepherd-ui`) to use generated types
8. [ ] Add `spectral` linting to CI pipeline
9. [ ] Document API development workflow in `docs/development/api-guidelines.md`
10. [ ] Set up Prism mock server for frontend development

### SDK Publishing Strategy (Future)

When external integrations require SDK access:

| Language | Package | Generation Tool | Registry |
|----------|---------|-----------------|----------|
| TypeScript | `@kv-shepherd/api-client` | openapi-typescript + openapi-fetch | npm |
| Go | `github.com/kv-shepherd/shepherd-go` | oapi-codegen (client mode) | Go modules |
| Python | `kv-shepherd-client` | openapi-python-client | PyPI |

**Versioning**: SDK versions MUST follow the API version. Breaking API changes require major version bump in all SDKs.

---

## References

* [OpenAPI Specification 3.1](https://spec.openapis.org/oas/v3.1.0)
* [oapi-codegen GitHub](https://github.com/oapi-codegen/oapi-codegen)
* [openapi-typescript GitHub](https://github.com/openapi-ts/openapi-typescript)
* [OpenAPI Initiative - Design-First vs Code-First](https://learn.openapis.org/best-practices.html)
* [ADR-0020: Frontend Technology Stack](./ADR-0020-frontend-technology-stack.md)
* [ADR-0022: Modular Provider Pattern](./ADR-0022-modular-provider-pattern.md)

---

## Changelog

| Date | Author | Change |
|------|--------|--------|
| 2026-01-28 | @jindyzhao | Added Mock Server integration, OpenAPI version policy, and SDK publishing strategy |
| 2026-01-27 | @jindyzhao | Initial draft based on 2026 best practices research |

---

_End of ADR-0021_
