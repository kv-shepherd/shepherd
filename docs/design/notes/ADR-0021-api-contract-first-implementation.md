# ADR-0021 Implementation: API Contract-First CI/CD Pipeline

> **Related ADR**: [ADR-0021](../../adr/ADR-0021-api-contract-first.md)  
> **Status**: Implementation Design  
> **Created**: 2026-02-02

---

## Overview

This document details the implementation of the CI/CD pipeline for enforcing API Contract-First development as defined in ADR-0021.

**Compatibility note**: The canonical spec remains OpenAPI 3.1 (`api/openapi.yaml`) and should target the latest patch version (3.1.1). If 3.1-only features are required, generate a 3.0-compatible artifact (`api/openapi.compat.yaml`) for Go codegen and validation while keeping the canonical spec as the single source of truth. When downgrading, use an OpenAPI Overlay to produce a 3.0-compatible file without mutating the canonical spec.

## Goals

1. **Prevent drift** between OpenAPI spec and generated code
2. **Detect breaking changes** before merge
3. **Validate contract compliance** at runtime (testing)
4. **Automate** the entire workflow via `make` targets

---

## 1. Makefile Targets

### Directory Structure

```
project/
├── api/
│   ├── openapi.yaml              # OpenAPI 3.1 canonical spec (source of truth)
│   └── openapi.compat.yaml       # OpenAPI 3.0-compatible spec (generated, optional)
├── internal/
│   └── api/
│       └── generated/
│           └── server.gen.go     # Generated Go code
├── web/
│   └── src/
│       └── types/
│           └── api.gen.ts        # Generated TypeScript types
├── api/
│   └── oapi-codegen.yaml         # oapi-codegen configuration
└── Makefile
```

### Make Targets

```makefile
# ============================================================
# API Contract-First Targets
# ============================================================

# Path configuration
OPENAPI_SPEC := api/openapi.yaml
COMPAT_SPEC  := api/openapi.compat.yaml
OAPI_CONFIG  := api/oapi-codegen.yaml
GO_OUTPUT    := internal/api/generated/server.gen.go
TS_OUTPUT    := web/src/types/api.gen.ts

.PHONY: api-lint api-generate api-check api-diff
.PHONY: api-compat api-compat-generate

## api-lint: Validate OpenAPI specification
api-lint:
	@echo "==> Linting OpenAPI spec..."
	npx @stoplight/spectral-cli lint $(OPENAPI_SPEC) --ruleset api/.spectral.yaml

## api-generate: Generate Go and TypeScript code from OpenAPI
api-generate: api-lint
	@echo "==> Generating Go server code..."
	@if [ -f $(COMPAT_SPEC) ]; then \
		oapi-codegen --config $(OAPI_CONFIG) $(COMPAT_SPEC); \
	else \
		oapi-codegen --config $(OAPI_CONFIG) $(OPENAPI_SPEC); \
	fi
	@echo "==> Generating TypeScript types..."
	npx openapi-typescript $(OPENAPI_SPEC) -o $(TS_OUTPUT)
	@echo "==> Generation complete"

## api-check: Verify generated code is in sync (for CI)
api-check: api-generate
	@echo "==> Checking for uncommitted changes..."
	@git diff --exit-code $(GO_OUTPUT) $(TS_OUTPUT) || \
		(echo "ERROR: Generated code is out of sync. Run 'make api-generate' and commit." && exit 1)

## api-compat: Ensure 3.0 compat spec is present and fresh (when required)
api-compat:
	@REQUIRE_OPENAPI_COMPAT=1 ./docs/design/ci/scripts/openapi-compat.sh

## api-compat-generate: Generate 3.0-compatible spec from canonical 3.1 spec
api-compat-generate:
	@./docs/design/ci/scripts/openapi-compat-generate.sh

## api-diff: Show breaking changes vs main branch
api-diff:
	@echo "==> Checking for breaking changes..."
	oasdiff breaking origin/main:$(OPENAPI_SPEC) $(OPENAPI_SPEC) --fail-on ERR
```

### Compat Overlay (3.1 → 3.0)

Use an OpenAPI Overlay to downgrade the canonical spec without changing it:

```yaml
# docs/design/ci/api-templates/openapi-overlay-3.0.yaml
overlay: 1.1.0
info:
  title: "OpenAPI 3.1 -> 3.0 Compatibility Overlay"
  version: "1.1.0"
actions:
  - target: "$.openapi"
    update: "3.0.3"
  - target: "$.jsonSchemaDialect"
    remove: true
  - target: "$.components.schemas.*.$schema"
    remove: true
```

Apply the overlay with `oas-patch` to generate `api/openapi.compat.yaml`:

```bash
oas-patch overlay api/openapi.yaml docs/design/ci/api-templates/openapi-overlay-3.0.yaml -o api/openapi.compat.yaml
```

---

## 2. Breaking Change Detection (oasdiff)

### Installation

```bash
# Install oasdiff
go install github.com/tufin/oasdiff@latest
```

### GitHub Action

```yaml
# .github/workflows/api-check.yml
name: API Contract Check

on:
  pull_request:
    paths:
      - 'api/openapi.yaml'
      - 'api/openapi.compat.yaml'
      - 'internal/api/**'

jobs:
  api-check:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0  # Need full history for comparison

      - name: Setup Go
        uses: actions/setup-go@v5
        with:
          go-version: '1.25'

      - name: Setup Node.js
        uses: actions/setup-node@v4
        with:
          node-version: '20'

      - name: Install tools
        run: |
          go install github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@latest
          go install github.com/tufin/oasdiff@latest
          npm install -g @stoplight/spectral-cli openapi-typescript

      - name: Lint OpenAPI spec
        run: make api-lint

      - name: Check generated code in sync
        run: make api-check

      - name: Check breaking changes
        id: breaking
        run: |
          oasdiff breaking origin/main:api/openapi.yaml api/openapi.yaml \
            --format markdown > breaking-changes.md || true
          if [ -s breaking-changes.md ]; then
            echo "has_breaking=true" >> $GITHUB_OUTPUT
          fi

      - name: Comment on PR with breaking changes
        if: steps.breaking.outputs.has_breaking == 'true'
        uses: actions/github-script@v7
        with:
          script: |
            const fs = require('fs');
            const body = fs.readFileSync('breaking-changes.md', 'utf8');
            github.rest.issues.createComment({
              issue_number: context.issue.number,
              owner: context.repo.owner,
              repo: context.repo.repo,
              body: '## ⚠️ API Breaking Changes Detected\n\n' + body
            });
```

### Breaking Change Severity Levels

| Level | Description | CI Behavior |
|-------|-------------|-------------|
| `ERR` | Breaking change (removal, type change) | ❌ Fail CI |
| `WARN` | Potentially breaking (deprecation) | ⚠️ Warning only |
| `INFO` | Backward compatible addition | ✅ Pass |

---

## 3. Contract Validation Testing (kin-openapi)

### Purpose

Runtime validation ensures that:
- Request bodies match OpenAPI schema
- Response bodies match OpenAPI schema
- Required fields are present
- Enum values are valid

This catches discrepancies between implementation and specification that code generation might miss.

**Security note**: Pin `kin-openapi` to `>= v0.131.0` to avoid known multipart zip decompression issues and track CVE fixes in `docs/design/DEPENDENCIES.md`.

### Integration Options

#### Option A: Test Middleware (Recommended for Testing)

```go
// internal/api/middleware/contract_validator.go
package middleware

import (
	"context"
	"net/http"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers/gorillamux"
	"github.com/gin-gonic/gin"
)

// ContractValidator validates requests/responses against OpenAPI spec
type ContractValidator struct {
	router  *gorillamux.Router
	options *openapi3filter.Options
}

// NewContractValidator creates a new validator from OpenAPI spec
func NewContractValidator(specPath string) (*ContractValidator, error) {
	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromFile(specPath)
	if err != nil {
		return nil, err
	}
	
	if err := doc.Validate(context.Background()); err != nil {
		return nil, err
	}
	
	router, err := gorillamux.NewRouter(doc)
	if err != nil {
		return nil, err
	}
	
	return &ContractValidator{
		router: router,
		options: &openapi3filter.Options{
			MultiError: true,
		},
	}, nil
}

// ValidateRequest validates incoming request against spec
func (cv *ContractValidator) ValidateRequest(c *gin.Context) error {
	route, pathParams, err := cv.router.FindRoute(c.Request)
	if err != nil {
		return err
	}
	
	input := &openapi3filter.RequestValidationInput{
		Request:    c.Request,
		PathParams: pathParams,
		Route:      route,
		Options:    cv.options,
	}
	
	return openapi3filter.ValidateRequest(c.Request.Context(), input)
}
```

#### Option B: Test Helper Functions

```go
// internal/testutil/contract_test.go
package testutil

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/getkin/kin-openapi/openapi3filter"
)

// AssertResponseMatchesContract validates response against OpenAPI spec
func AssertResponseMatchesContract(t *testing.T, resp *httptest.ResponseRecorder, route *openapi3.Route) {
	t.Helper()
	
	responseValidationInput := &openapi3filter.ResponseValidationInput{
		RequestValidationInput: &openapi3filter.RequestValidationInput{
			Route: route,
		},
		Status: resp.Code,
		Header: resp.Header(),
		Body:   io.NopCloser(resp.Body),
	}
	
	if err := openapi3filter.ValidateResponse(context.Background(), responseValidationInput); err != nil {
		t.Errorf("Response does not match OpenAPI contract: %v", err)
	}
}
```

### When to Enable

| Environment | Request Validation | Response Validation |
|-------------|-------------------|---------------------|
| Development | ✅ Enabled | ✅ Enabled |
| Testing/CI | ✅ Enabled | ✅ Enabled |
| Production | ❌ Disabled (performance) | ❌ Disabled |

---

## 5. Cross-Field Validation (go-playground/validator)

### Purpose

OpenAPI can express single-field constraints (min, max, format), but **cannot express**:
- Cross-field validation (`startTime < endTime`)
- Conditional requirements (`if type == "premium", subscriptionId is required`)
- Complex business rules (`memory/cpu ratio must be in range`)

We use `go-playground/validator` to fill this gap with a **consistent, tag-based approach**.

### Why This Matters for AI Development

| Without Standard | With Standard |
|------------------|---------------|
| AI might use ad-hoc `if` checks | AI uses consistent validate tags |
| Different validation styles per API | Uniform pattern across all APIs |
| Error messages vary | Standardized error format |
| Refactoring needed later | Correct from day one |

### Installation

```bash
go get github.com/go-playground/validator/v10
```

### Validation Tag Reference

#### Basic Constraints (OpenAPI can also express these)

```go
type Example struct {
    Name   string `validate:"required,min=1,max=100"`
    Age    int    `validate:"gte=0,lte=150"`
    Email  string `validate:"required,email"`
    Status string `validate:"oneof=active inactive pending"`
}
```

#### Cross-Field Validation (OpenAPI CANNOT express these)

```go
type TimeRange struct {
    StartTime time.Time `json:"startTime" validate:"required"`
    EndTime   time.Time `json:"endTime" validate:"required,gtfield=StartTime"`
}

type ResourceRequest struct {
    MinReplicas int `json:"minReplicas" validate:"gte=1"`
    MaxReplicas int `json:"maxReplicas" validate:"gtefield=MinReplicas"`
}
```

#### Conditional Validation

```go
type VMCreateRequest struct {
    Type           string `json:"type" validate:"required,oneof=basic premium enterprise"`
    SubscriptionID string `json:"subscriptionId" validate:"required_if=Type premium,required_if=Type enterprise"`
    SupportLevel   string `json:"supportLevel" validate:"required_unless=Type basic"`
}
```

### Integration with Gin

#### Validator Initialization

```go
// internal/api/validator/validator.go
package validator

import (
    "github.com/gin-gonic/gin/binding"
    "github.com/go-playground/validator/v10"
)

// InitValidator registers custom validators and sets up Gin binding
func InitValidator() error {
    if v, ok := binding.Validator.Engine().(*validator.Validate); ok {
        // Register custom validators here
        // v.RegisterValidation("customrule", customRuleFunc)
        
        // Register struct-level validations
        // v.RegisterStructValidation(VMCreateValidation, VMCreateRequest{})
    }
    return nil
}
```

#### Handler Pattern

```go
// internal/handler/vm_handler.go
func (h *VMHandler) CreateVM(c *gin.Context) {
    var req VMCreateRequest
    
    // Gin automatically uses validator if struct has validate tags
    if err := c.ShouldBindJSON(&req); err != nil {
        // Convert validation errors to API error response
        apiErr := h.translateValidationError(err)
        c.JSON(http.StatusBadRequest, apiErr)
        return
    }
    
    // Business logic...
}

// translateValidationError converts validator errors to API format
func (h *VMHandler) translateValidationError(err error) *ErrorResponse {
    var validationErrs validator.ValidationErrors
    if errors.As(err, &validationErrs) {
        details := make([]FieldError, 0, len(validationErrs))
        for _, e := range validationErrs {
            details = append(details, FieldError{
                Field:   e.Field(),
                Tag:     e.Tag(),
                Message: h.translateFieldError(e),
            })
        }
        return &ErrorResponse{
            Code:    "VALIDATION_ERROR",
            Message: "Request validation failed",
            Details: details,
        }
    }
    return &ErrorResponse{
        Code:    "INVALID_REQUEST",
        Message: err.Error(),
    }
}
```

### Common Validation Patterns

| Pattern | Tag | Example |
|---------|-----|---------|
| **Required** | `required` | `validate:"required"` |
| **String length** | `min,max` | `validate:"min=1,max=100"` |
| **Numeric range** | `gte,lte` | `validate:"gte=1,lte=64"` |
| **Enum** | `oneof` | `validate:"oneof=a b c"` |
| **Email** | `email` | `validate:"email"` |
| **UUID** | `uuid` | `validate:"uuid"` |
| **Greater than field** | `gtfield` | `validate:"gtfield=MinValue"` |
| **Greater/equal field** | `gtefield` | `validate:"gtefield=MinReplicas"` |
| **Required if** | `required_if` | `validate:"required_if=Type premium"` |
| **Required unless** | `required_unless` | `validate:"required_unless=Type basic"` |
| **Excluded if** | `excluded_if` | `validate:"excluded_if=Type basic"` |

### OpenAPI x-validation Extension (Optional)

To document validation rules in OpenAPI (for frontend awareness):

```yaml
# api/openapi.yaml
components:
  schemas:
    TimeRange:
      type: object
      properties:
        startTime:
          type: string
          format: date-time
        endTime:
          type: string
          format: date-time
          x-validation:
            - rule: gtfield=StartTime
              message: End time must be after start time
```

### Error Response Format

```json
{
  "code": "VALIDATION_ERROR",
  "message": "Request validation failed",
  "details": [
    {
      "field": "EndTime",
      "tag": "gtfield",
      "message": "EndTime must be greater than StartTime"
    },
    {
      "field": "SubscriptionID", 
      "tag": "required_if",
      "message": "SubscriptionID is required when Type is premium"
    }
  ]
}
```

---

## 6. oapi-codegen Configuration

### Configuration File

```yaml
# api/oapi-codegen.yaml
package: generated
output: internal/api/generated/server.gen.go
generate:
  models: true
  gin-server: true
  embedded-spec: true

output-options:
  # Go 1.24+ omitzero support (ADR-0028)
  prefer-skip-optional-pointer-with-omitzero: true
  
  # Skip generating trivial aliases
  skip-prune: false

import-mapping:
  # Custom type mappings if needed
  # ./components/schemas/UUID.yaml: github.com/google/uuid

additional-imports:
  # Additional imports to include
```

---

## 7. Implementation Checklist

### Phase 1: Foundation (Priority)

- [ ] Create `api/oapi-codegen.yaml` configuration
- [ ] Create `api/.spectral.yaml` for linting configuration
- [ ] Add Makefile `api-*` targets
- [ ] Add `api-compat-generate` target and script (3.1 → 3.0 compat)
- [ ] Verify `make api-generate` works locally

### Phase 2: CI Integration

- [ ] Create `.github/workflows/api-check.yml`
- [ ] Add `api-check` to required status checks
- [ ] Test breaking change detection on a PR

### Phase 3: Contract Testing

- [ ] Add kin-openapi dependency
- [ ] Create contract validation test helper
- [ ] Add contract tests for first implemented endpoint
- [ ] Document testing patterns in `CONTRIBUTING.md`

### Phase 4: Cross-Field Validation

- [ ] Add go-playground/validator dependency
- [ ] Create `internal/api/validator/validator.go` initialization
- [ ] Create validation error translation helper
- [ ] Define common validation patterns in skill documentation
- [ ] Add validation examples to first implemented endpoint

---

## References

- [oapi-codegen documentation](https://github.com/oapi-codegen/oapi-codegen)
- [oasdiff documentation](https://github.com/tufin/oasdiff)
- [kin-openapi documentation](https://github.com/getkin/kin-openapi)
- [Spectral CLI](https://github.com/stoplightio/spectral)
- [go-playground/validator](https://github.com/go-playground/validator)
- [Vacuum](https://github.com/daveshanley/vacuum) - Go-native OpenAPI linter

---

## Addendum: ADR-0029 Toolchain Migration (2026-02-03)

> **Note**: This document was written before ADR-0029 (OpenAPI Toolchain Governance).
>
> Per ADR-0029, the following toolchain changes apply:
>
> | This Document | ADR-0029 Replacement |
> |---------------|----------------------|
> | `spectral lint` | `vacuum lint` (Go-native, 10x faster, Spectral-rule compatible) |
> | `kin-openapi` validation | `libopenapi-validator` (StrictMode) |
> | `oas-patch` | `libopenapi` overlay support |
>
> **Spectral rulesets remain compatible** with Vacuum. No rule file changes required.
>
> See [ADR-0029](../../adr/ADR-0029-openapi-toolchain-governance.md) and
> [ADR-0029 Implementation Details](./ADR-0029-openapi-toolchain-implementation.md) for current toolchain.

