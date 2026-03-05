// shepherd-lint is the KubeVirt Shepherd architecture enforcement linter.
//
// It bundles all custom go/analysis Analyzers into a single binary,
// replacing `go run` CI scripts with a single cached binary execution.
//
// Usage:
//
//	shepherd-lint ./...
//	shepherd-lint ./internal/...
//
// Each Analyzer enforces a specific ADR or project convention:
//
// Batch 1 (AST-only):
//
//	nakedgoroutine   - ADR-0031: no naked `go` statements in internal code
//	forbiddenimports - Detects fake clients, GORM, outbox imports, hardcoded paths
//	riverbypass      - ADR-0006: UseCase layer must use River Queue for protected entity writes
//	runtimemock      - Detects runtime wiring of MockProvider (test-only construct)
//	semaphoreusage   - ADR-0031: Acquire() must have paired defer Release()
//	txboundary       - Enforces service layer cannot manage transactions (Tx/Commit/Rollback)
//	riverjobargs     - ADR-0009: River Job Args must use Claim Check (EventID only, no direct IDs)
//
// Batch 2 (provider-layer + transaction safety):
//
//	ssacompliance    - ADR-0011: provider write paths must not use typed KubeVirt structs or .Create()/.Update()
//	k8sintransaction - ADR-0006/ADR-0012: K8s provider calls inside DB transaction callbacks (advisory)
//
// golangci-lint v2 Module Plugin:
//
//	The root package (shepherdlinter) exports New() compatible with the
//	golangci-lint v2 module plugin system (register.LinterPlugin interface).
//	Configure via .custom-gcl.yml + .golangci.yml to run as shepherd-arch linter.
package main

import (
	shepherdlinter "kv-shepherd.io/shepherd-linter"

	"golang.org/x/tools/go/analysis/multichecker"
)

func main() {
	multichecker.Main(shepherdlinter.AllAnalyzers...)
}
