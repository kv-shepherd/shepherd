# KubeVirt Shepherd Makefile
# ADR-0016: Module path kv-shepherd.io/shepherd

.PHONY: all build test lint clean run seed docker help generate api-gen api-generate ent-gen sqlc-gen master-flow-strict master-flow-completion test-backend-docker-pg master-flow-strict-docker-pg

# Go parameters
GOCMD=go
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

## test: Run unit tests
test:
	$(GOTEST) -race -count=1 ./...

## test-cover: Run tests with coverage
test-cover:
	$(GOTEST) -race -coverprofile=coverage.out -covermode=atomic ./...
	$(GOCMD) tool cover -html=coverage.out -o coverage.html

## lint: Run golangci-lint
lint:
	golangci-lint run ./...

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
