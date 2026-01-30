# API Contract-First Make Targets (ADR-0021)
# Include this file from the main Makefile:
#   include build/api.mk
#
# Prerequisites:
#   - Go 1.24+ (or tools.go pattern for oapi-codegen)
#   - Node.js 20+ (for openapi-typescript)
#   - npm packages: @stoplight/spectral-cli, openapi-typescript, @stoplight/prism-cli

# ─────────────────────────────────────────────────────────────────────────────
# Configuration
# ─────────────────────────────────────────────────────────────────────────────

OPENAPI_SPEC := api/openapi.yaml
SPECTRAL_CONFIG := api/.spectral.yaml
GO_GENERATED_DIR := internal/api/generated
TS_GENERATED_FILE := web/src/types/api.gen.ts
OAPI_CODEGEN_CONFIG := api/oapi-codegen.yaml

# Tool versions (update as needed)
OAPI_CODEGEN_VERSION := v2.4.1
OPENAPI_TS_VERSION := 7.4.4

# ─────────────────────────────────────────────────────────────────────────────
# Main Targets
# ─────────────────────────────────────────────────────────────────────────────

.PHONY: api-lint
api-lint: ## Validate OpenAPI spec with Spectral
	@echo "🔍 Linting OpenAPI specification..."
	@npx @stoplight/spectral-cli lint $(OPENAPI_SPEC) --ruleset $(SPECTRAL_CONFIG)
	@echo "✅ OpenAPI spec is valid"

.PHONY: api-generate
api-generate: api-generate-go api-generate-ts ## Generate all API code from OpenAPI spec
	@echo "✅ All API code generated successfully"

.PHONY: api-generate-go
api-generate-go: ## Generate Go server code
	@echo "🔄 Generating Go server code..."
	@mkdir -p $(GO_GENERATED_DIR)
	@go tool oapi-codegen -config $(OAPI_CODEGEN_CONFIG) $(OPENAPI_SPEC)
	@echo "✅ Go server code generated: $(GO_GENERATED_DIR)/"

.PHONY: api-generate-ts
api-generate-ts: ## Generate TypeScript types
	@echo "🔄 Generating TypeScript types..."
	@mkdir -p $(dir $(TS_GENERATED_FILE))
	@cd web && npx openapi-typescript@$(OPENAPI_TS_VERSION) ../$(OPENAPI_SPEC) -o src/types/api.gen.ts
	@echo "✅ TypeScript types generated: $(TS_GENERATED_FILE)"

.PHONY: api-check
api-check: ## Verify generated code is in sync with spec (CI target)
	@echo "🔍 Checking generated code sync..."
	@./docs/design/ci/scripts/api-check.sh

.PHONY: api-breaking
api-breaking: ## Detect breaking changes vs main branch
	@echo "🔍 Checking for breaking changes..."
	@git fetch origin main --quiet 2>/dev/null || true
	@if git show origin/main:$(OPENAPI_SPEC) > /tmp/openapi-base.yaml 2>/dev/null; then \
		npx oasdiff breaking /tmp/openapi-base.yaml $(OPENAPI_SPEC) --fail-on ERR; \
	else \
		echo "⚠️  No base spec found on main branch (new API?)"; \
	fi

.PHONY: api-changelog
api-changelog: ## Generate changelog vs main branch
	@echo "📝 Generating API changelog..."
	@git fetch origin main --quiet 2>/dev/null || true
	@if git show origin/main:$(OPENAPI_SPEC) > /tmp/openapi-base.yaml 2>/dev/null; then \
		npx oasdiff changelog /tmp/openapi-base.yaml $(OPENAPI_SPEC) --format markdown; \
	else \
		echo "⚠️  No base spec found on main branch"; \
	fi

.PHONY: api-mock
api-mock: ## Start Prism mock server for frontend development
	@echo "🚀 Starting mock server on http://localhost:4010..."
	@echo "   Press Ctrl+C to stop"
	@npx @stoplight/prism-cli mock $(OPENAPI_SPEC) --port 4010

.PHONY: api-docs
api-docs: ## Serve interactive API documentation
	@echo "📚 Starting API documentation server on http://localhost:8081..."
	@echo "   Press Ctrl+C to stop"
	@npx @scalar/cli serve $(OPENAPI_SPEC) --port 8081

# ─────────────────────────────────────────────────────────────────────────────
# Setup Targets
# ─────────────────────────────────────────────────────────────────────────────

.PHONY: api-tools
api-tools: ## Install required API tooling
	@echo "📦 Installing API development tools..."
	# Go tools (requires Go 1.24+ for go tool directive)
	go get -tool github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@$(OAPI_CODEGEN_VERSION)
	# Node.js tools (install globally or use npx)
	npm install -g @stoplight/spectral-cli @stoplight/prism-cli @scalar/cli
	# For oasdiff (breaking change detection)
	go install github.com/tufin/oasdiff@latest
	@echo "✅ API tools installed"

.PHONY: api-init
api-init: ## Initialize new API project structure
	@echo "📁 Creating API directory structure..."
	@mkdir -p api/schemas api/paths $(GO_GENERATED_DIR) $(dir $(TS_GENERATED_FILE))
	@if [ ! -f $(OPENAPI_SPEC) ]; then \
		echo "Creating initial OpenAPI spec..."; \
		echo 'openapi: 3.1.0' > $(OPENAPI_SPEC); \
		echo 'info:' >> $(OPENAPI_SPEC); \
		echo '  title: KubeVirt Shepherd API' >> $(OPENAPI_SPEC); \
		echo '  version: 1.0.0' >> $(OPENAPI_SPEC); \
		echo 'paths: {}' >> $(OPENAPI_SPEC); \
	fi
	@echo "✅ API structure initialized"

# ─────────────────────────────────────────────────────────────────────────────
# Help
# ─────────────────────────────────────────────────────────────────────────────

.PHONY: api-help
api-help: ## Show API-related targets
	@echo "API Contract-First Development Targets (ADR-0021)"
	@echo ""
	@echo "Development:"
	@echo "  api-lint       Validate OpenAPI spec with Spectral"
	@echo "  api-generate   Generate Go + TypeScript code"
	@echo "  api-mock       Start mock server for frontend development"
	@echo "  api-docs       Serve interactive API documentation"
	@echo ""
	@echo "CI/Review:"
	@echo "  api-check      Verify generated code is in sync"
	@echo "  api-breaking   Detect breaking changes vs main"
	@echo "  api-changelog  Generate changelog vs main"
	@echo ""
	@echo "Setup:"
	@echo "  api-tools      Install required tooling"
	@echo "  api-init       Initialize API structure"
