# API Contract-First Make Targets (ADR-0021, ADR-0029)
# Include this file from the main Makefile:
#   include build/api.mk
#
# Prerequisites:
#   - Go 1.25+ (for oapi-codegen, vacuum)
#   - Node.js 20+ (for frontend OpenAPI type generation only)
#   - Optional local binaries: vacuum, oasdiff

# ─────────────────────────────────────────────────────────────────────────────
# Configuration
# ─────────────────────────────────────────────────────────────────────────────

OPENAPI_SPEC := api/openapi.yaml
COMPAT_SPEC := api/openapi.compat.yaml
VACUUM_CONFIG := api/.vacuum.yaml
GO_GENERATED_DIR := internal/api/generated
TS_GENERATED_FILE := web/src/types/api.gen.ts
OAPI_CODEGEN_CONFIG := api/oapi-codegen.yaml
OAPI_CODEGEN_INPUT := $(OPENAPI_SPEC)

# Tool versions (pin in docs/design/DEPENDENCIES.md; override via env if needed)
OAPI_CODEGEN_VERSION ?= v2.5.1
VACUUM_VERSION ?= v0.23.8
OASDIFF_VERSION ?= v1.11.10

VACUUM_CMD := go run github.com/daveshanley/vacuum@$(VACUUM_VERSION)
OASDIFF_CMD := go run github.com/oasdiff/oasdiff@$(OASDIFF_VERSION)
OAPI_CODEGEN_CMD := go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@$(OAPI_CODEGEN_VERSION)
OPENAPI_COMPAT_GEN_CMD := go run ./cmd/openapi-compat-gen/main.go

# ─────────────────────────────────────────────────────────────────────────────
# Main Targets
# ─────────────────────────────────────────────────────────────────────────────

.PHONY: api-lint
api-lint: ## Validate OpenAPI spec with Vacuum (ADR-0029)
	@echo "🔍 Linting OpenAPI specification..."
	@$(VACUUM_CMD) lint $(OPENAPI_SPEC) --ruleset $(VACUUM_CONFIG)
	@echo "✅ OpenAPI spec is valid"

.PHONY: api-generate
api-generate: api-generate-go api-generate-ts ## Generate all API code from OpenAPI spec
	@echo "✅ All API code generated successfully"

.PHONY: api-generate-go
api-generate-go: ## Generate Go server code
	@echo "🔄 Generating Go server code..."
	@mkdir -p $(GO_GENERATED_DIR)
	@if [ -f $(COMPAT_SPEC) ]; then \
		OAPI_CODEGEN_INPUT=$(COMPAT_SPEC); \
	fi; \
	$(OAPI_CODEGEN_CMD) -config $(OAPI_CODEGEN_CONFIG) $${OAPI_CODEGEN_INPUT:-$(OPENAPI_SPEC)}
	@echo "✅ Go server code generated: $(GO_GENERATED_DIR)/"

.PHONY: api-generate-ts
api-generate-ts: ## Generate TypeScript types
	@echo "🔄 Generating TypeScript types..."
	@mkdir -p $(dir $(TS_GENERATED_FILE))
	@cd web && npm run api:generate
	@echo "✅ TypeScript types generated: $(TS_GENERATED_FILE)"

.PHONY: api-check
api-check: ## Verify generated code is in sync with spec (CI target)
	@echo "🔍 Checking generated code sync..."
	@bash ./docs/design/ci/scripts/api-check.sh

.PHONY: api-compat
api-compat: ## Verify compat spec exists and is fresh (set REQUIRE_OPENAPI_COMPAT=1 to enforce)
	@bash ./docs/design/ci/scripts/openapi-compat.sh

.PHONY: api-compat-generate
api-compat-generate: ## Generate OpenAPI 3.0-compatible spec (Go-native)
	@$(OPENAPI_COMPAT_GEN_CMD) $(OPENAPI_SPEC) $(COMPAT_SPEC)

.PHONY: api-breaking
api-breaking: ## Detect breaking changes vs main branch
	@echo "🔍 Checking for breaking changes..."
	@git fetch origin main --quiet 2>/dev/null || true
	@if git show origin/main:$(OPENAPI_SPEC) > /tmp/openapi-base.yaml 2>/dev/null; then \
		$(OASDIFF_CMD) breaking /tmp/openapi-base.yaml $(OPENAPI_SPEC) --fail-on ERR; \
	else \
		echo "⚠️  No base spec found on main branch (new API?)"; \
	fi

.PHONY: api-changelog
api-changelog: ## Generate changelog vs main branch
	@echo "📝 Generating API changelog..."
	@git fetch origin main --quiet 2>/dev/null || true
	@if git show origin/main:$(OPENAPI_SPEC) > /tmp/openapi-base.yaml 2>/dev/null; then \
		$(OASDIFF_CMD) changelog /tmp/openapi-base.yaml $(OPENAPI_SPEC) --format markdown; \
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
	# Go tools
	go install github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@$(OAPI_CODEGEN_VERSION)
	# Optional local binaries for faster/lower-network local runs
	go install github.com/daveshanley/vacuum@$(VACUUM_VERSION)
	go install github.com/oasdiff/oasdiff@$(OASDIFF_VERSION)
	# Node.js tools (TypeScript generation only - ADR-0029)
	cd web && npm ci
	@npm install -g @stoplight/prism-cli @scalar/cli || \
		echo "⚠️  Global npm install skipped; use 'npx @stoplight/prism-cli' and 'npx @scalar/cli' if needed."
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
	@echo "  api-lint       Validate OpenAPI spec with Vacuum"
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
