# KubeVirt Shepherd Makefile
# ADR-0016: Module path kv-shepherd.io/shepherd

.PHONY: all build test lint lint-arch lint-version-check build-shepherd-lint shepherd-lint clean run seed docker help generate api-gen api-generate ent-gen sqlc-gen master-flow-strict master-flow-completion test-backend-docker-pg master-flow-strict-docker-pg

# Go parameters
GOCMD=go

# golangci-lint binary — prefer PATH, fall back to ~/go/bin (ADR-0039)
GOLANGCI_LINT=$(shell which golangci-lint 2>/dev/null || echo $(HOME)/go/bin/golangci-lint)

# shepherd-linter paths (ADR-0039)
SHEPHERD_LINTER_DIR=tools/shepherd-linter
GOBUILD=$(GOCMD) build
GOTEST=$(GOCMD) test
GOMOD=$(GOCMD) mod
BINARY_NAME=shepherd
SEED_BINARY=seed

# Build directories
BUILD_DIR=bin

# Include API contract-first targets (ADR-0021, ADR-0029)
-include build/api.mk

all: generate lint test build

## generate: Run all code generation (Ent + OpenAPI + sqlc)
generate: ent-gen api-generate sqlc-gen

## api-gen: Generate Go server types from OpenAPI spec (ADR-0021)
api-gen:
	@$(MAKE) api-generate-go

## ent-gen: Generate Ent ORM code from schemas (ADR-0003)
ent-gen:
	$(GOCMD) generate ./ent

## sqlc-gen: Generate sqlc query code for ADR-0012 atomic transactions
sqlc-gen:
	$(GOCMD) run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.30.0 generate

## build: Build the server binary
build:
	$(GOBUILD) -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/server/...

## build-seed: Build the seed binary
build-seed:
	$(GOBUILD) -o $(BUILD_DIR)/$(SEED_BINARY) ./cmd/seed/...

## run: Run the server locally
run:
	$(GOCMD) run ./cmd/server/...

## seed: Run data seeding
seed:
	$(GOCMD) run ./cmd/seed/...

## test: Run all tests (PostgreSQL packages auto-start a temporary Docker postgres:18 container)
##       Set TEST_DATABASE_URL to reuse an existing PostgreSQL instance instead.
test:
	$(GOTEST) -race -count=1 -timeout=300s ./...

## test-cover: Run tests with coverage
test-cover:
	$(GOTEST) -race -coverprofile=coverage.out -covermode=atomic -timeout=300s ./...
	$(GOCMD) tool cover -html=coverage.out -o coverage.html

## lint: Run golangci-lint (auto-detects module plugin via .custom-gcl.yml; ADR-0039)
## Depends on lint-version-check to fail fast on golang.org/x/tools version drift.
lint: lint-version-check
	@if [ -f .custom-gcl.yml ]; then \
		if [ ! -f ./custom-gcl ] || [ .custom-gcl.yml -nt ./custom-gcl ]; then \
			echo "Building custom golangci-lint binary (module plugin)..."; \
			$(GOLANGCI_LINT) custom; \
		fi; \
		echo "Running custom-gcl (includes shepherd-arch)..."; \
		./custom-gcl run ./...; \
	else \
		$(GOLANGCI_LINT) run ./...; \
	fi

## lint-arch: Run architecture enforcement linters via shepherd-lint binary (ADR-0039)
## Replaces individual 'go run docs/design/ci/scripts/check_*.go' invocations for Batch 1 scripts.
lint-arch:
	@if [ ! -f $(BUILD_DIR)/shepherd-lint ]; then $(MAKE) build-shepherd-lint; fi
	./$(BUILD_DIR)/shepherd-lint ./internal/... ./cmd/... ./pkg/...

## lint-version-check: Verify golang.org/x/tools version alignment between golangci-lint and shepherd-linter/go.mod
## Prevents plugin ABI mismatches (ADR-0039 §critical constraint).
lint-version-check:
	@set -e; \
	GCLLINT=$$(command -v golangci-lint 2>/dev/null || echo "$$HOME/go/bin/golangci-lint"); \
	LINT_VER=$$(go version -m "$$GCLLINT" 2>/dev/null | awk '/golang\.org\/x\/tools/{print $$3}'); \
	MOD_VER=$$(awk '/^require golang\.org\/x\/tools /{print $$3} /^\tgolang\.org\/x\/tools /{print $$2}' $(SHEPHERD_LINTER_DIR)/go.mod | head -1); \
	if [ -z "$$LINT_VER" ]; then \
		echo "WARNING (ADR-0039): cannot detect golang.org/x/tools version from golangci-lint binary"; \
		exit 0; \
	fi; \
	if [ "$$LINT_VER" != "$$MOD_VER" ]; then \
		echo "ERROR (ADR-0039): golang.org/x/tools version mismatch!"; \
		echo "  golangci-lint built with: $$LINT_VER"; \
		echo "  shepherd-linter go.mod:  $$MOD_VER"; \
		echo "  Fix: run 'cd tools/shepherd-linter && go get golang.org/x/tools@$$LINT_VER && go mod tidy'"; \
		exit 1; \
	fi; \
	echo "OK (ADR-0039): golang.org/x/tools aligned at $$LINT_VER"

## build-shepherd-lint: Build the shepherd-lint architecture enforcement binary (ADR-0039)
build-shepherd-lint:
	@echo "Building shepherd-lint..."
	@mkdir -p $(BUILD_DIR)
	cd $(SHEPHERD_LINTER_DIR) && $(GOCMD) build -o ../../$(BUILD_DIR)/shepherd-lint ./cmd/shepherd-lint/
	@echo "shepherd-lint built: $(BUILD_DIR)/shepherd-lint"

## shepherd-lint: Build and run shepherd-lint on the main project (ADR-0039)
shepherd-lint: build-shepherd-lint
	./$(BUILD_DIR)/shepherd-lint ./internal/... ./cmd/... ./pkg/...

## fmt: Format code
fmt:
	goimports -w .
	$(GOCMD) fmt ./...

## tidy: Tidy go modules
tidy:
	$(GOMOD) tidy

## clean: Clean build artifacts
clean:
	rm -rf $(BUILD_DIR)
	rm -f coverage.out coverage.html

## docker: Build Docker image
docker:
	docker build -t kubevirt-shepherd:latest .

## ci-checks: Run CI check scripts
ci-checks:
	@echo "Running CI checks..."
	@for script in docs/design/ci/scripts/*.sh; do \
		echo "Running $$script..."; \
		bash "$$script" || exit 1; \
	done

## master-flow-strict: Run strict master-flow test-first gate chain (requires DATABASE_URL)
master-flow-strict:
	@test -n "$$DATABASE_URL" || (echo "DATABASE_URL is required (PostgreSQL-only tests)"; exit 1)
	go run docs/design/ci/scripts/check_master_flow_api_alignment.go
	go run docs/design/ci/scripts/check_master_flow_test_matrix.go
	go run docs/design/ci/scripts/check_master_flow_traceability.go
	bash docs/design/ci/scripts/check_changed_code_has_tests.sh
	go run docs/design/ci/scripts/check_no_sqlite_in_tests.go
	go run docs/design/ci/scripts/check_stage3_admin_catalog_baseline.go
	go run docs/design/ci/scripts/check_stage4_system_service_baseline.go
	go run docs/design/ci/scripts/check_stage5d_delete_baseline.go
	go run docs/design/ci/scripts/check_stage6_vnc_baseline.go
	bash docs/design/ci/scripts/check_live_e2e_no_mock.sh
	go run docs/design/ci/scripts/check_no_global_platform_admin_gate.go
	go run docs/design/ci/scripts/check_handler_explicit_rbac_guards.go
	go run docs/design/ci/scripts/check_auth_provider_plugin_boundary.go
	go run docs/design/ci/scripts/check_frontend_openapi_usage.go
	go run docs/design/ci/scripts/check_frontend_no_placeholder_pages.go
	go run docs/design/ci/scripts/check_doc_claims_consistency.go
	go test -count=1 ./internal/api/handlers ./internal/governance/approval ./internal/usecase ./internal/jobs ./internal/repository/sqlc ./internal/service
	npm run typecheck --prefix web
	npm run test:run --prefix web
	bash scripts/run_e2e_live.sh --no-db-wrapper

## master-flow-completion: Check if full master-flow completion can be claimed (no deferred/exemption debt)
master-flow-completion:
	go run docs/design/ci/scripts/check_master_flow_completion_readiness.go

## test-backend-docker-pg: Run backend PostgreSQL test suites against an isolated Docker PostgreSQL container
test-backend-docker-pg:
	./scripts/run_with_docker_pg.sh

## master-flow-strict-docker-pg: Run master-flow strict chain against an isolated Docker PostgreSQL container
master-flow-strict-docker-pg:
	./scripts/run_with_docker_pg.sh -- make master-flow-strict

## help: Show this help message
help:
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@sed -n 's/^## //p' $(MAKEFILE_LIST) | column -t -s ':' | sed 's/^/  /'
