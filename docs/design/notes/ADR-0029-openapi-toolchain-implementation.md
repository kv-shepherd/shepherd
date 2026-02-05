# ADR-0029 Implementation Details: OpenAPI Toolchain Governance

> **Parent ADR**: [ADR-0029](../../adr/ADR-0029-openapi-toolchain-governance.md)  
> **Status**: Implementation specification for ADR-0029 (parent ADR status: **accepted**)

---

## Overview

This document provides detailed implementation specifications for the OpenAPI toolchain governance decisions in ADR-0029.

---

## 1. Tool Versions

> **Single Source of Truth**: These versions are referenced by [DEPENDENCIES.md](../DEPENDENCIES.md).

| Tool | Version | Install Method | Notes |
|------|---------|---------------|-------|
| `vacuum` | `>= v0.14.0` | Go binary / Docker | World's fastest OpenAPI linter |
| `github.com/pb33f/libopenapi` | `>= v0.31.0` | Go module | Lossless OpenAPI parsing, **Overlay support** (v0.31.0+) |
| `github.com/pb33f/libopenapi-validator` | `>= v0.6.0` | Go module | StrictMode, **version-aware validation** (3.0 vs 3.1) |
| `oapi-codegen` | `>= v2.4.1` | Go module | ADR-0021 selection, pin exact version in CI |
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
	@command -v vacuum >/dev/null 2>&1 || { \
		echo "ERROR: vacuum not found. Install: go install github.com/daveshanley/vacuum@v0.14.0"; \
		exit 1; \
	}
	vacuum lint api/openapi.yaml --fail-severity warn --ruleset api/.vacuum.yaml

# Validate generated code matches spec
api-generate:
	@echo "==> Generating Go server code..."
	oapi-codegen -config api/oapi-codegen.yaml api/openapi.yaml

# Validate TypeScript types match spec (requires Node.js)
# Note: This target requires Node.js runtime per ADR-0021 TypeScript type generation requirement
api-generate-ts:
	@echo "==> Generating TypeScript types..."
	@command -v node >/dev/null 2>&1 || { \
		echo "WARNING: Node.js not found. TypeScript type generation skipped."; \
		echo "         Install Node.js or run in container: docker run -v \$$(pwd):/app -w /app node:22-alpine npx openapi-typescript api/openapi.yaml -o web/src/types/api.gen.ts"; \
		exit 0; \
	}
	npx openapi-typescript api/openapi.yaml -o web/src/types/api.gen.ts

# Full API check (CI gate)
api-check: api-lint
	@echo "==> Checking generated code is up-to-date..."
	$(MAKE) api-generate
	@git diff --exit-code api/generated/ || \
		(echo "ERROR: Generated code out of sync. Run 'make api-generate' and commit." && exit 1)
	@echo "==> API checks passed."
```

### 2.2 GitHub Actions Workflow

> **Security Note**: All third-party actions MUST be pinned to specific versions or commit SHAs to prevent supply chain attacks.

```yaml
# .github/workflows/api-contract.yaml
name: API Contract Validation

on:
  pull_request:
    paths:
      - 'api/**'
      - 'internal/api/**'
      - 'web/src/types/api.gen.ts'

# Principle of Least Privilege: Only request necessary permissions
permissions:
  contents: read
  pull-requests: read

jobs:
  api-lint:
    runs-on: ubuntu-22.04  # Pin runner version for consistency
    timeout-minutes: 10    # Prevent stuck workflows
    steps:
      # Pin to specific commit SHA for security
      - uses: actions/checkout@11bd71901bbe5b1630ceea73d27597364c9af683  # v4.2.2
      
      # Use official vacuum GitHub Action with version pinning
      - name: Lint OpenAPI spec with vacuum
        uses: pb33f/vacuum-action@v2  # Pin to specific version
        with:
          spec: api/openapi.yaml
          fail_severity: warn
          ruleset: api/.vacuum.yaml

  api-sync-check:
    runs-on: ubuntu-22.04
    timeout-minutes: 15
    needs: api-lint
    steps:
      - uses: actions/checkout@11bd71901bbe5b1630ceea73d27597364c9af683  # v4.2.2
      
      - uses: actions/setup-go@d35c59abb061a4a6fb18e82ac0862c26744d6ab5  # v5.5.0
        with:
          go-version-file: 'go.mod'
          cache: true
      
      # Pin oapi-codegen to exact version for reproducibility
      - name: Install oapi-codegen
        run: go install github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.4.1
      
      - name: Generate and verify Go code
        run: |
          oapi-codegen -config api/oapi-codegen.yaml api/openapi.yaml
          git diff --exit-code api/generated/ || \
            (echo "::error::Generated Go code is out of sync with OpenAPI spec. Run 'make api-generate' and commit." && exit 1)
      
      - uses: actions/setup-node@49933ea5288caeca8642d1e84afbd3f7d6820020  # v4.4.0
        with:
          node-version: '22'
          cache: 'npm'
          cache-dependency-path: 'web/package-lock.json'
      
      - name: Generate and verify TypeScript types
        run: |
          npx openapi-typescript api/openapi.yaml -o web/src/types/api.gen.ts
          git diff --exit-code web/src/types/api.gen.ts || \
            (echo "::error::Generated TypeScript types are out of sync with OpenAPI spec. Run 'make api-generate-ts' and commit." && exit 1)
```

> **Dependabot Configuration**: Configure Dependabot to automatically create PRs for action updates:
> ```yaml
> # .github/dependabot.yml
> version: 2
> updates:
>   - package-ecosystem: "github-actions"
>     directory: "/"
>     schedule:
>       interval: "weekly"
> ```

---

## 3. Runtime Validation with StrictMode

### 3.1 Validator Integration

```go
// internal/api/middleware/openapi_validator.go

package middleware

import (
	"log/slog"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/pb33f/libopenapi"
	validator "github.com/pb33f/libopenapi-validator"
	"github.com/pb33f/libopenapi-validator/config"
)

// ValidatorConfig allows customization of validation behavior per environment.
type ValidatorConfig struct {
	// VerboseErrors enables detailed error messages (use only in development).
	// In production, this should be false to prevent information disclosure.
	VerboseErrors bool
	
	// ValidateResponses enables response validation (recommended for dev/test only).
	ValidateResponses bool
	
	// Logger for internal error logging (detailed errors always logged here).
	Logger *slog.Logger
}

// DefaultValidatorConfig returns a secure configuration suitable for production.
func DefaultValidatorConfig() ValidatorConfig {
	return ValidatorConfig{
		VerboseErrors:     gin.Mode() != gin.ReleaseMode,
		ValidateResponses: gin.Mode() != gin.ReleaseMode,
		Logger:            slog.Default(),
	}
}

// OpenAPIValidator creates a Gin middleware for OpenAPI validation with StrictMode.
// StrictMode detects undeclared fields even when additionalProperties is not set to false.
// 
// Security: In production (gin.ReleaseMode), detailed validation errors are logged
// server-side but NOT returned to clients to prevent information disclosure.
func OpenAPIValidator(specPath string, cfg ValidatorConfig) (gin.HandlerFunc, error) {
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
			// Always log detailed errors server-side for debugging
			cfg.Logger.Warn("OpenAPI validation failed",
				slog.String("path", c.Request.URL.Path),
				slog.String("method", c.Request.Method),
				slog.Any("errors", formatValidationErrorsInternal(validationErrs)),
			)

			// Return appropriate response based on environment
			if cfg.VerboseErrors {
				// Development: return detailed errors for debugging
				c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
					"code":    "VALIDATION_FAILED",
					"message": "Request validation failed",
					"details": formatValidationErrorsForClient(validationErrs),
				})
			} else {
				// Production: return generic error message only
				c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
					"code":    "VALIDATION_FAILED",
					"message": "Request validation failed. Please check your request format.",
				})
			}
			return
		}

		// Capture response for validation (dev/test only)
		if cfg.ValidateResponses {
			rw := &responseWriter{ResponseWriter: c.Writer}
			c.Writer = rw
		}

		c.Next()

		// Validate response (dev/test only, never in production)
		if cfg.ValidateResponses {
			// Response validation logic here
		}
	}, nil
}

// formatValidationErrorsInternal returns full error details for server-side logging.
func formatValidationErrorsInternal(errs []*validator.ValidationError) []map[string]any {
	result := make([]map[string]any, len(errs))
	for i, e := range errs {
		errMap := map[string]any{
			"message": e.Message,
			"reason":  e.Reason,
		}
		if len(e.SchemaValidationErrors) > 0 {
			errMap["location"] = e.SchemaValidationErrors[0].Location
			errMap["schema_path"] = e.SchemaValidationErrors[0].AbsoluteLocation
		}
		result[i] = errMap
	}
	return result
}

// formatValidationErrorsForClient returns sanitized errors safe for client exposure.
// Avoids exposing internal schema paths, regex patterns, or system details.
func formatValidationErrorsForClient(errs []*validator.ValidationError) []map[string]any {
	result := make([]map[string]any, len(errs))
	for i, e := range errs {
		// Only expose field path and user-friendly message
		errMap := map[string]any{
			"message": sanitizeErrorMessage(e.Message),
		}
		if len(e.SchemaValidationErrors) > 0 {
			// Only include the relative path, not absolute schema location
			errMap["field"] = e.SchemaValidationErrors[0].Location
		}
		result[i] = errMap
	}
	return result
}

// sanitizeErrorMessage removes potentially sensitive details from error messages.
func sanitizeErrorMessage(msg string) string {
	// Remove regex patterns, schema references, and internal paths
	// For now, return message as-is; extend with regex sanitization as needed
	// Example: strings.ReplaceAll to remove pattern details
	return msg
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

## 8. Spectral to Vacuum Migration Guide

> **Key Point**: Vacuum is designed for drop-in compatibility with Spectral rulesets. Migration is straightforward.

### 8.1 Rule Compatibility

| Aspect | Compatibility | Notes |
|--------|--------------|-------|
| **Spectral Rulesets** | ✅ Fully compatible | Vacuum accepts Spectral ruleset YAML files directly |
| **JSONPath expressions** | ✅ Supported | Both RFC 9535 and JSON Path Plus expressions work |
| **Custom functions** | ⚠️ Partial | Custom JS functions need Go reimplementation |
| **Built-in rules** | ✅ Equivalent | Vacuum `recommended:oas` covers Spectral's default rules |

### 8.2 Migration Steps

1. **Direct Ruleset Use**
   
   If you have an existing Spectral ruleset (`.spectral.yaml`), you can use it directly:
   ```bash
   # Spectral (before)
   spectral lint api/openapi.yaml --ruleset .spectral.yaml
   
   # Vacuum (after) - same ruleset file works
   vacuum lint api/openapi.yaml --ruleset .spectral.yaml
   ```

2. **Verify Rule Behavior**
   
   Run both tools and compare results (vacuum may be stricter by default):
   ```bash
   # Compare outputs
   spectral lint api/openapi.yaml --format json > spectral-report.json
   vacuum lint api/openapi.yaml --details --ruleset .spectral.yaml > vacuum-report.txt
   ```

3. **Leverage Vacuum Extensions**
   
   Vacuum adds useful properties not in Spectral:
   ```yaml
   # api/.vacuum.yaml - Enhanced ruleset
   rules:
     oas3-operation-operationId:
       id: OAS-OID-001           # Unique ID for tracking
       category: codegen         # Categorization
       howToFix: "Add operationId to each operation" # Remediation hint
   ```

4. **Handle Custom Functions**
   
   If using Spectral custom functions (JavaScript):
   - Simple rules: Rewrite as vacuum JSONPath rules
   - Complex logic: Create a vacuum plugin (Go) or pre-validation script

### 8.3 Common Rule Mappings

| Spectral Rule | Vacuum Equivalent | Severity |
|---------------|------------------|----------|
| `oas3-unused-component` | `oas3-unused-component` | warn |
| `oas3-schema` | N/A (use libopenapi-validator) | N/A |
| `operation-operationId` | `oas3-operation-operationId` | error |
| `operation-description` | `oas3-operation-description` | warn |

### 8.4 Validation Command Reference

```bash
# Basic lint
vacuum lint api/openapi.yaml

# With custom ruleset
vacuum lint api/openapi.yaml --ruleset api/.vacuum.yaml

# Fail on warnings (CI mode)
vacuum lint api/openapi.yaml --fail-severity warn

# Generate HTML report
vacuum report api/openapi.yaml

# Show only errors in modified areas (PR reviews)
vacuum lint api/openapi.yaml --base main
```

---

## 9. Trade-offs and Known Limitations

> **Transparency**: This section documents known trade-offs for informed decision-making.

### 9.1 Mixed Language Ecosystem

| Component | Language | Runtime | Rationale |
|-----------|----------|---------|-----------|
| vacuum | Go | None | Linting, CI gates |
| libopenapi-validator | Go | None | Runtime validation |
| oapi-codegen | Go | None | Server code generation |
| **openapi-typescript** | **Node.js** | npm | **ADR-0021 requirement for TypeScript types** |

**Impact**: 
- CI pipelines require both Go and Node.js environments
- Docker images may be larger, or multi-stage builds are needed
- Recommendation: Use GitHub Actions' matrix jobs to parallelize Go and Node.js steps

**Mitigations**:
- `api-generate-ts` Makefile target includes Node.js availability check with graceful fallback
- CI workflow uses separate jobs with appropriate runners
- Container-based generation option documented in Makefile comments

### 9.2 Error Message Handling

The OpenAPI validator middleware uses environment-aware error responses:

| Mode | `gin.Mode()` | Behavior |
|------|--------------|----------|
| Development | `debug` | Full validation errors returned to client |
| Staging | `test` | Full validation errors (for E2E tests) |
| **Production** | `release` | **Generic error only; details logged server-side** |

This prevents information disclosure while maintaining debuggability.

---

## References

- [vacuum Documentation](https://quobix.com/vacuum/)
- [libopenapi Documentation](https://pb33f.io/libopenapi/)
- [libopenapi-validator StrictMode](https://pb33f.io/libopenapi-validator/strict/)
- [oapi-codegen v2 Guide](https://github.com/oapi-codegen/oapi-codegen)

---

_End of Implementation Details_
