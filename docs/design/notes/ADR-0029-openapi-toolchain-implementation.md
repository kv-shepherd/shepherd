# ADR-0029 Implementation Details: OpenAPI Toolchain Governance

> **Parent ADR**: [ADR-0029](../../adr/ADR-0029-openapi-toolchain-governance.md)  
> **Status**: Implementation specification for accepted ADR

---

## Overview

This document provides detailed implementation specifications for the OpenAPI toolchain governance decisions in ADR-0029.

---

## 1. Tool Versions

> **Single Source of Truth**: These versions are referenced by [DEPENDENCIES.md](../DEPENDENCIES.md).

| Tool | Version | Install Method | Notes |
|------|---------|---------------|-------|
| `vacuum` | `>= v0.14.0` | Go binary / Docker | World's fastest OpenAPI linter |
| `github.com/pb33f/libopenapi` | `>= v0.21.0` | Go module | Lossless OpenAPI parsing |
| `github.com/pb33f/libopenapi-validator` | `>= v0.2.0` | Go module | Strict mode validation |
| `oapi-codegen` | `>= v2.4.0` | Go module | ADR-0021 selection |
| `openapi-typescript` | `>= v7.0.0` | npm | TypeScript type generation |

---

## 2. CI Pipeline Configuration

### 2.1 Makefile Targets

```makefile
# Makefile

# === OpenAPI Toolchain (ADR-0029) ===

.PHONY: api-lint api-validate api-generate api-generate-ts api-check

# Lint OpenAPI spec with vacuum (Go-native, Spectral-compatible)
api-lint:
	@echo "==> Linting OpenAPI spec with vacuum..."
	vacuum lint api/openapi.yaml --fail-severity warn

# Validate generated code matches spec
api-generate:
	@echo "==> Generating Go server code..."
	oapi-codegen -config api/oapi-codegen.yaml api/openapi.yaml

# Validate TypeScript types match spec
api-generate-ts:
	@echo "==> Generating TypeScript types..."
	npx openapi-typescript api/openapi.yaml -o ui/src/types/api.gen.ts

# Full API check (CI gate)
api-check: api-lint
	@echo "==> Checking generated code is up-to-date..."
	$(MAKE) api-generate
	@git diff --exit-code api/generated/ || \
		(echo "ERROR: Generated code out of sync. Run 'make api-generate' and commit." && exit 1)
	@echo "==> API checks passed."
```

### 2.2 GitHub Actions Workflow

```yaml
# .github/workflows/api-contract.yaml
name: API Contract Validation

on:
  pull_request:
    paths:
      - 'api/**'
      - 'internal/api/**'
      - 'ui/src/types/api.gen.ts'

jobs:
  api-lint:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      
      - name: Install vacuum
        run: |
          curl -fsSL https://get.vacuum.sh | sh
          sudo mv vacuum /usr/local/bin/
      
      - name: Lint OpenAPI spec
        run: vacuum lint api/openapi.yaml --fail-severity warn

  api-sync-check:
    runs-on: ubuntu-latest
    needs: api-lint
    steps:
      - uses: actions/checkout@v4
      
      - uses: actions/setup-go@v5
        with:
          go-version: '1.25'
      
      - name: Install oapi-codegen
        run: go install github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@latest
      
      - name: Generate and verify Go code
        run: |
          oapi-codegen -config api/oapi-codegen.yaml api/openapi.yaml
          git diff --exit-code api/generated/ || \
            (echo "::error::Generated Go code is out of sync with OpenAPI spec" && exit 1)
      
      - uses: actions/setup-node@v4
        with:
          node-version: '22'
      
      - name: Generate and verify TypeScript types
        run: |
          npx openapi-typescript api/openapi.yaml -o ui/src/types/api.gen.ts
          git diff --exit-code ui/src/types/api.gen.ts || \
            (echo "::error::Generated TypeScript types are out of sync with OpenAPI spec" && exit 1)
```

---

## 3. Runtime Validation with StrictMode

### 3.1 Validator Integration

```go
// internal/api/middleware/openapi_validator.go

package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/pb33f/libopenapi"
	validator "github.com/pb33f/libopenapi-validator"
	"github.com/pb33f/libopenapi-validator/config"
)

// OpenAPIValidator creates a Gin middleware for OpenAPI validation with StrictMode.
// StrictMode detects undeclared fields even when additionalProperties is not set to false.
func OpenAPIValidator(specPath string) (gin.HandlerFunc, error) {
	// Load OpenAPI spec
	specBytes, err := os.ReadFile(specPath)
	if err != nil {
		return nil, fmt.Errorf("read spec: %w", err)
	}

	doc, err := libopenapi.NewDocument(specBytes)
	if err != nil {
		return nil, fmt.Errorf("parse spec: %w", err)
	}

	// Create validator with StrictMode enabled (ADR-0029 requirement)
	v, errs := validator.NewValidator(doc,
		config.WithStrictMode(), // Detect undeclared properties
	)
	if len(errs) > 0 {
		return nil, fmt.Errorf("create validator: %v", errs)
	}

	return func(c *gin.Context) {
		// Validate request
		valid, validationErrs := v.ValidateHttpRequest(c.Request)
		if !valid {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
				"code":    "VALIDATION_FAILED",
				"message": "Request validation failed",
				"details": formatValidationErrors(validationErrs),
			})
			return
		}

		// Capture response for validation
		rw := &responseWriter{ResponseWriter: c.Writer}
		c.Writer = rw

		c.Next()

		// Validate response (optional, can be enabled in dev/test only)
		if gin.Mode() != gin.ReleaseMode {
			// Response validation logic here
		}
	}, nil
}

func formatValidationErrors(errs []*validator.ValidationError) []map[string]any {
	result := make([]map[string]any, len(errs))
	for i, e := range errs {
		result[i] = map[string]any{
			"path":    e.SchemaValidationErrors[0].Location,
			"message": e.Message,
		}
	}
	return result
}
```

### 3.2 StrictMode Behavior

| Scenario | Standard Validation | StrictMode (ADR-0029) |
|----------|--------------------|-----------------------|
| Request has undeclared field | ✅ Allowed (if additionalProperties not false) | ❌ **Rejected** |
| Response has undeclared field | ✅ Allowed | ❌ **Rejected** |
| Query param not in spec | ⚠️ Ignored | ❌ **Rejected** |
| Header not in spec | ⚠️ Ignored | ❌ **Rejected** |

---

## 4. Vacuum Configuration

### 4.1 Ruleset Configuration

```yaml
# api/.vacuum.yaml
# Vacuum configuration with Spectral-compatible rules

extends:
  - recommended:oas

rules:
  # Require operation IDs for all endpoints (code generation dependency)
  oas3-operation-operationId: error
  
  # Require descriptions for all schemas
  oas3-schema-description: warn
  
  # Enforce consistent naming
  oas3-schema-names-snake: off  # Use camelCase for this project
  
  # Security
  oas3-security-defined: error
  
  # Custom rules for this project
  custom:
    # Require examples for all request/response bodies
    require-examples:
      severity: warn
      message: "All request/response bodies should have examples for mock server"
```

### 4.2 Pre-commit Hook

```bash
#!/bin/bash
# .git/hooks/pre-commit (or use pre-commit framework)

# Lint OpenAPI spec before commit
if git diff --cached --name-only | grep -q "^api/"; then
    echo "OpenAPI spec changed, running vacuum lint..."
    vacuum lint api/openapi.yaml --fail-severity warn
    if [ $? -ne 0 ]; then
        echo "OpenAPI lint failed. Please fix issues before committing."
        exit 1
    fi
fi
```

---

## 5. Overlay Processing with libopenapi

### 5.1 Go Implementation (replaces oas-patch)

```go
// tools/overlay/main.go

package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/pb33f/libopenapi"
	"github.com/pb33f/libopenapi/overlays"
)

func main() {
	specPath := flag.String("spec", "api/openapi.yaml", "OpenAPI spec path")
	overlayPath := flag.String("overlay", "api/overlays/compat-3.0.yaml", "Overlay file path")
	outputPath := flag.String("output", "api/openapi-3.0.yaml", "Output path")
	flag.Parse()

	// Load base spec
	specBytes, err := os.ReadFile(*specPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading spec: %v\n", err)
		os.Exit(1)
	}

	// Load overlay
	overlayBytes, err := os.ReadFile(*overlayPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading overlay: %v\n", err)
		os.Exit(1)
	}

	// Parse and apply overlay
	doc, err := libopenapi.NewDocument(specBytes)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing spec: %v\n", err)
		os.Exit(1)
	}

	result, err := overlays.ApplyOverlay(doc, overlayBytes)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error applying overlay: %v\n", err)
		os.Exit(1)
	}

	// Write output
	if err := os.WriteFile(*outputPath, result, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing output: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Overlay applied successfully: %s\n", *outputPath)
}
```

### 5.2 Makefile Integration

```makefile
# Generate OpenAPI 3.0 compatible spec for legacy tools
api-compat:
	@echo "==> Generating OpenAPI 3.0 compatible spec..."
	go run tools/overlay/main.go \
		-spec api/openapi.yaml \
		-overlay api/overlays/compat-3.0.yaml \
		-output api/openapi-3.0.yaml
```

---

## 6. Dependency Update in DEPENDENCIES.md

The following section should be updated in `docs/design/DEPENDENCIES.md`:

```markdown
## API Contract-First Tooling (ADR-0021, ADR-0029)

> **Note**: Toolchain governance defined by ADR-0029. Pin versions here.

| Package | Version | Release Date | Description |
|---------|---------|--------------|-------------|
| **OpenAPI Specification** | `3.1.1` | 2024-10 | Canonical spec version for `api/openapi.yaml` |
| **OpenAPI Overlay Spec** | `1.1.0` | 2026-01-14 | Overlay version for compat generation |
| `github.com/pb33f/libopenapi` | `>= v0.21.0` | 2025-11 | Lossless OpenAPI parsing, Overlay support (Go-native) |
| `github.com/pb33f/libopenapi-validator` | `>= v0.2.0` | 2025-10 | **StrictMode** request/response validation |
| `vacuum` | `>= v0.14.0` | 2025-12 | Go-native OpenAPI linter (replaces spectral) |
| `github.com/oapi-codegen/oapi-codegen/v2` | `>= v2.4.0` | 2025-07 | Go server/client code generation |

> **Removed Dependencies** (ADR-0029):
> - `spectral` → replaced by `vacuum` (Go-native)
> - `oas-patch` → replaced by `libopenapi` overlay support (Go-native)
> - `kin-openapi` validation → replaced by `libopenapi-validator` (StrictMode)
>
> Note: `kin-openapi` remains as a transitive dependency of `oapi-codegen`.
```

---

## 7. Migration Checklist

- [ ] Install vacuum CLI in CI environment
- [ ] Add `libopenapi` and `libopenapi-validator` to go.mod
- [ ] Create `api/.vacuum.yaml` ruleset configuration
- [ ] Update Makefile with new targets
- [ ] Create GitHub Actions workflow for API contract validation
- [ ] Implement OpenAPI validator middleware with StrictMode
- [ ] Update DEPENDENCIES.md with new tool versions
- [ ] Remove oas-patch from any existing scripts
- [ ] (Optional) Create Go overlay tool if Overlay processing is needed

---

## References

- [vacuum Documentation](https://quobix.com/vacuum/)
- [libopenapi Documentation](https://pb33f.io/libopenapi/)
- [libopenapi-validator StrictMode](https://pb33f.io/libopenapi-validator/strict/)
- [oapi-codegen v2 Guide](https://github.com/oapi-codegen/oapi-codegen)

---

_End of Implementation Details_
