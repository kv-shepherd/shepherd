# KubeVirt Shepherd Makefile
# ADR-0016: Module path kv-shepherd.io/shepherd

.PHONY: all build test lint lint-arch lint-version-check build-shepherd-lint shepherd-lint test-shepherd-linter clean run seed docker help generate api-gen api-generate ent-gen sqlc-gen master-flow-strict master-flow-completion test-backend-docker-pg master-flow-strict-docker-pg pr pr-ci pr-sequential ci-checks ci-prep ci-governance ci-backend ci-frontend ci-api-sync ci-e2e-smoke ci-go-lint ci-go-build ci-go-test ci-master-flow-backend ci-frontend-deadcode ci-frontend-unit ci-api-lint ci-api-breaking ci-api-generated-sync ci-api-contract govulncheck frontend-deadcode-scan frontend-security-audit secrets-scan supplemental-scans kubevirt-schema-check kubevirt-schema-upgrade kubevirt-schema-report authproviderplugin-sdk-smoke

# Go parameters
GO_TOOLCHAIN_VERSION?=go1.25.8
export GOTOOLCHAIN=$(GO_TOOLCHAIN_VERSION)
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
GOVULNCHECK_VERSION=v1.1.4
GITLEAKS_VERSION=v8.28.0
GO_LINT_TARGETS=./cmd/... ./ent/... ./internal/... ./pkg/...
GO_BUILD_TARGETS=./cmd/... ./ent/... ./internal/... ./pkg/... ./plugins/...
GO_TEST_TARGETS=./cmd/... ./ent/... ./internal/... ./pkg/... ./plugins/...

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

## lint: Run the blocking local lint bundle (govulncheck + gitleaks + frontend dead-code scan + frontend high-severity dependency audit + golangci-lint)
## Depends on lint-version-check to fail fast on golang.org/x/tools version drift.
lint: lint-version-check govulncheck secrets-scan frontend-deadcode-scan frontend-security-audit
	@$(MAKE) ci-go-lint

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

## test-shepherd-linter: Run go/analysis unit tests for custom shepherd-linter analyzers
test-shepherd-linter:
	cd $(SHEPHERD_LINTER_DIR) && $(GOTEST) ./...

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

## ci-governance: Run canonical governance/static CI check scripts wired in GitHub Actions
ci-governance:
	@echo "Running CI checks..."
	@$(MAKE) lint-arch
	@$(MAKE) test-shepherd-linter
	@bash docs/design/ci/scripts/check_no_new_run_scripts.sh
	@bash docs/design/ci/scripts/check_no_legacy_batch1_invocations.sh
	@bash docs/design/ci/scripts/check_changed_code_has_tests.sh
	@bash docs/design/ci/scripts/check_no_redis_import.sh
	@bash docs/design/ci/scripts/check_manual_di.sh
	@go run docs/design/ci/scripts/check_no_sqlite_in_tests.go
	@go run docs/design/ci/scripts/check_no_runtime_placeholders.go
	@go run docs/design/ci/scripts/check_provider_wiring.go
	@bash docs/design/ci/scripts/check_sqlc_usage.sh
	@go run docs/design/ci/scripts/check_validate_spec.go
	@go run docs/design/ci/scripts/check_openapi_critical_contract.go
	@go run docs/design/ci/scripts/check_openapi_critical_fingerprint.go
	@go run docs/design/ci/scripts/check_vm_create_status_progression.go
	@go run docs/design/ci/scripts/check_vm_create_spec_completeness.go
	@go run docs/design/ci/scripts/check_critical_test_presence.go
	@go run docs/design/ci/scripts/check_stage5c_behavior_tests.go
	@go run docs/design/ci/scripts/check_stage3_admin_catalog_baseline.go
	@go run docs/design/ci/scripts/check_stage4_system_service_baseline.go
	@go run docs/design/ci/scripts/check_stage5d_delete_baseline.go
	@go run docs/design/ci/scripts/check_duplicate_guard_scope.go
	@go run docs/design/ci/scripts/check_environment_isolation_enforcement.go
	@go run docs/design/ci/scripts/check_stage5e_batch_baseline.go
	@go run docs/design/ci/scripts/check_stage6_vnc_baseline.go
	@bash docs/design/ci/scripts/check_live_e2e_no_mock.sh
	@go run docs/design/ci/scripts/check_no_global_platform_admin_gate.go
	@go run docs/design/ci/scripts/check_handler_explicit_rbac_guards.go
	@go run docs/design/ci/scripts/check_auth_provider_plugin_boundary.go
	@go run docs/design/ci/scripts/check_frontend_openapi_usage.go
	@go run docs/design/ci/scripts/check_frontend_no_non_english_literals.go
	@go run docs/design/ci/scripts/check_frontend_no_placeholder_pages.go
	@go run docs/design/ci/scripts/check_frontend_route_shell_architecture.go
	@go run docs/design/ci/scripts/check_doc_claims_consistency.go
	@go run docs/design/ci/scripts/check_master_flow_api_alignment.go
	@go run docs/design/ci/scripts/check_master_flow_test_matrix.go
	@go run docs/design/ci/scripts/check_master_flow_traceability.go
	@go run docs/design/ci/scripts/check_markdown_links.go
	@bash docs/design/ci/scripts/check_design_doc_governance.sh
	@go run docs/design/ci/scripts/check_module_noop_hooks.go
	@go run docs/design/ci/scripts/check_test_assertions.go
	@go run docs/design/ci/scripts/check_dead_tests.go
	@go run docs/design/ci/scripts/check_repository_tests.go
	@bash docs/design/ci/scripts/check_workflow_make_target_parity.sh
	@$(MAKE) authproviderplugin-sdk-smoke
	@$(MAKE) secrets-scan
	@$(MAKE) kubevirt-schema-check

## authproviderplugin-sdk-smoke: Prove the public auth-provider SDK can be consumed from a separate module.
authproviderplugin-sdk-smoke:
	@cd tools/sdk-smoke/authproviderplugin-external && $(GOCMD) test ./...

## ci-checks: Backward-compatible alias for governance/static gates
ci-checks: ci-governance

## ci-prep: Run shared mutating checks once before parallel lanes
ci-prep:
	@npm ci --prefix web
	@$(MAKE) ci-api-sync

## ci-backend: Run Go/backend required checks once
ci-backend:
	@$(MAKE) govulncheck
	@$(MAKE) ci-go-lint
	@$(MAKE) ci-go-test
	@$(MAKE) ci-go-build
	@$(MAKE) ci-master-flow-backend

## ci-frontend: Run frontend required checks once
ci-frontend:
	@$(MAKE) ci-frontend-deadcode
	@$(MAKE) ci-frontend-unit

## ci-api-sync: Run API contract and generated-code sync checks once
ci-api-sync:
	@$(MAKE) ci-api-lint
	@$(MAKE) ci-api-breaking
	@$(MAKE) ci-api-generated-sync
	@$(MAKE) ci-api-contract

## ci-e2e-smoke: Run frontend mock smoke once
ci-e2e-smoke:
	@set -e; \
	trap 'find web -maxdepth 1 -name "tsconfig.e2e.*.json" -delete' EXIT; \
	find web -maxdepth 1 -name "tsconfig.e2e.*.json" -delete; \
	PW_WEB_PORT="$$(bash ./scripts/pick_free_port.sh)"; \
	if [ -z "$$PW_WEB_PORT" ]; then \
		echo "Failed to allocate Playwright web port"; \
		exit 1; \
	fi; \
	echo "Using Playwright web port $$PW_WEB_PORT"; \
	CI=1 PW_WEB_PORT="$$PW_WEB_PORT" npm run test:e2e:mock --prefix web
	@find web -maxdepth 1 -name 'tsconfig.e2e.*.json' -delete

## ci-go-lint: Run the Go lint target set used by the required CI Lint job
ci-go-lint: lint-version-check
	@if [ -f .custom-gcl.yml ]; then \
		if [ ! -f ./custom-gcl ] || [ .custom-gcl.yml -nt ./custom-gcl ]; then \
			echo "Building custom golangci-lint binary (module plugin)..."; \
			$(GOLANGCI_LINT) custom; \
		fi; \
		echo "Running custom-gcl (includes shepherd-arch)..."; \
		./custom-gcl run $(GO_LINT_TARGETS); \
	else \
		$(GOLANGCI_LINT) run $(GO_LINT_TARGETS); \
	fi

## ci-go-build: Run the Go build target set used by the required CI Build job
ci-go-build:
	@$(GOCMD) build $(GO_BUILD_TARGETS)

## ci-go-test: Run the Go race-test target set used by the required CI Test job
ci-go-test:
	@$(GOCMD) test -race -coverprofile=coverage.out -covermode=atomic $(GO_TEST_TARGETS)

## ci-master-flow-backend: Run the backend behavior suites used by the required Master-Flow Strict job
ci-master-flow-backend:
	@if [ -n "$$DATABASE_URL" ]; then \
		go test -count=1 ./internal/api/handlers; \
		go test -count=1 ./internal/governance/approval; \
		go test -count=1 ./internal/usecase; \
		go test -count=1 ./internal/jobs; \
		go test -count=1 ./internal/service; \
	else \
		./scripts/run_with_docker_pg.sh -- \
			go test -count=1 \
			./internal/api/handlers \
			./internal/governance/approval \
			./internal/usecase \
			./internal/jobs \
			./internal/service; \
	fi

## ci-frontend-deadcode: Run the frontend dependency hygiene target set used by the required Frontend dead-code job
ci-frontend-deadcode:
	@$(MAKE) frontend-security-audit
	@$(MAKE) frontend-deadcode-scan

## ci-frontend-unit: Run the frontend lint/typecheck/unit/build target set used by the required Frontend unit job
ci-frontend-unit:
	@npm run lint --prefix web
	@npm run typecheck --prefix web
	@npm run test:run --prefix web
	@npm run build --prefix web

## ci-api-lint: Run the OpenAPI lint target set used by the required API lint job
ci-api-lint:
	@$(MAKE) api-lint

## ci-api-breaking: Run the OpenAPI breaking-change target set used by the required API breaking-change job
ci-api-breaking:
	@$(MAKE) api-breaking BASE_REF=$${BASE_REF:-main}

## ci-api-generated-sync: Run the API generated-sync target set used by the required API generated-code-sync job
ci-api-generated-sync:
	@echo "🔍 Verifying generated API code sync..."
	@VERSION="$$(go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.5.1 -version | tail -n 1)"; \
	if [ "$$VERSION" != "v2.5.1" ]; then \
		echo "oapi-codegen must be v2.5.1, got $$VERSION"; \
		exit 1; \
	fi
	@REQUIRE_OPENAPI_COMPAT=1 bash ./docs/design/ci/scripts/api-check.sh
	@cd web && npm run api:generate && git diff --exit-code src/types/api.gen.ts && npm run typecheck
	@if [ -n "$$(git status --porcelain internal/api/generated/ web/src/types/api.gen.ts api/openapi.compat.yaml internal/api/specembed/openapi.yaml 2>/dev/null)" ]; then \
		echo "Generated code is out of sync with OpenAPI spec!"; \
		git status --porcelain internal/api/generated/ web/src/types/api.gen.ts api/openapi.compat.yaml internal/api/specembed/openapi.yaml; \
		exit 1; \
	fi

## ci-api-contract: Run the API runtime contract test target set used by the required API contract-test job
ci-api-contract:
	@$(MAKE) api-contract-test

## pr: Short alias for the workflow-equivalent PR validation bundle
pr: pr-ci

## pr-sequential: Run the full local PR gate set serially for easier debugging
pr-sequential:
	@echo "Running sequential PR CI..."
	@$(MAKE) ci-prep
	@$(MAKE) ci-api-sync
	@$(MAKE) ci-backend
	@$(MAKE) ci-governance
	@$(MAKE) ci-frontend
	@$(MAKE) ci-e2e-smoke

## pr-ci: Run the workflow-equivalent local PR validation bundle in parallel
pr-ci:
	@echo "Running workflow-equivalent PR CI..."
	@bash scripts/run_pr_parallel.sh

## govulncheck: Run Go official vulnerability scanner (blocking when invoked directly)
govulncheck:
	@$(GOCMD) run golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION) ./...

## frontend-deadcode-scan: Run Knip dead-code/dependency scan (blocking when invoked directly)
frontend-deadcode-scan:
	@npm run knip --prefix web

## frontend-security-audit: Run npm audit at high severity threshold (blocking when invoked directly)
frontend-security-audit:
	@bash scripts/run_frontend_audit_retry.sh

## secrets-scan: Run gitleaks against the current working tree (blocking when invoked directly)
secrets-scan:
	@$(GOCMD) run github.com/zricethezav/gitleaks/v8@$(GITLEAKS_VERSION) detect --source . --no-git --config .gitleaks.toml --no-banner --redact

## supplemental-scans: Run dedicated scanner gates alongside the core CI bundle
supplemental-scans:
	@echo "Running supplemental scanners..."
	@$(MAKE) govulncheck
	@$(MAKE) secrets-scan
	@$(MAKE) frontend-deadcode-scan
	@$(MAKE) frontend-security-audit

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

## kubevirt-schema-check: Check if embedded KubeVirt schema matches latest GA release (non-blocking by default)
kubevirt-schema-check:
	@go run ./cmd/kubevirt-schema-check

## kubevirt-schema-upgrade: Download and prepare a new KubeVirt schema version
## Usage: make kubevirt-schema-upgrade VERSION=1.8.0
kubevirt-schema-upgrade:
	@test -n "$(VERSION)" || (echo "Usage: make kubevirt-schema-upgrade VERSION=<semver>"; exit 1)
	@go run ./cmd/kubevirt-schema-upgrade $(VERSION)

## kubevirt-schema-report: Print upgrade candidates and missing i18n keys for the current embedded schema baseline
kubevirt-schema-report:
	@go run ./cmd/kubevirt-schema-report

## help: Show this help message
help:
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@sed -n 's/^## //p' $(MAKEFILE_LIST) | column -t -s ':' | sed 's/^/  /'
