# KubeVirt Shepherd Makefile
# ADR-0016: Module path kv-shepherd.io/shepherd

.PHONY: all build test lint clean run seed docker help generate api-gen ent-gen sqlc-gen

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
generate: ent-gen api-gen sqlc-gen

## api-gen: Generate Go server types from OpenAPI spec (ADR-0021)
api-gen:
	$(GOCMD) run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@latest \
		-generate gin,models,spec -package generated \
		-o internal/api/generated/server.gen.go api/openapi.yaml

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

## help: Show this help message
help:
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@sed -n 's/^## //p' $(MAKEFILE_LIST) | column -t -s ':' | sed 's/^/  /'
