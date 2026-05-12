// Package shepherdlinter provides architecture enforcement analyzers for the
// KubeVirt Shepherd project as a golangci-lint v2 module plugin.
//
// This module serves two roles:
//   - golangci-lint v2 module plugin: registered via init() → register.Plugin()
//   - Standalone binary: via cmd/shepherd-lint using multichecker.Main()
//
// See ADR-0039 for the full decision record.
package shepherdlinter

import (
	"github.com/golangci/plugin-module-register/register"
	"golang.org/x/tools/go/analysis"

	"kv-shepherd.io/shepherd-linter/analyzer/authproviderlayering"
	"kv-shepherd.io/shepherd-linter/analyzer/entquerysafety"
	"kv-shepherd.io/shepherd-linter/analyzer/forbiddenimports"
	"kv-shepherd.io/shepherd-linter/analyzer/k8sintransaction"
	"kv-shepherd.io/shepherd-linter/analyzer/k8spollingrv"
	"kv-shepherd.io/shepherd-linter/analyzer/manualdi"
	"kv-shepherd.io/shepherd-linter/analyzer/nakedgoroutine"
	"kv-shepherd.io/shepherd-linter/analyzer/openapirbaccontract"
	"kv-shepherd.io/shepherd-linter/analyzer/rbacguards"
	"kv-shepherd.io/shepherd-linter/analyzer/riverbypass"
	"kv-shepherd.io/shepherd-linter/analyzer/riverjobargs"
	"kv-shepherd.io/shepherd-linter/analyzer/runtimemock"
	"kv-shepherd.io/shepherd-linter/analyzer/semaphoreusage"
	"kv-shepherd.io/shepherd-linter/analyzer/ssacompliance"
	"kv-shepherd.io/shepherd-linter/analyzer/txboundary"
)

func init() {
	register.Plugin("shepherd-arch", New)
}

// AllAnalyzers is the canonical list of architecture enforcement analyzers.
// Used by both the golangci-lint plugin (via BuildAnalyzers) and the standalone
// binary (via cmd/shepherd-lint/main.go).
var AllAnalyzers = []*analysis.Analyzer{
	nakedgoroutine.Analyzer,
	forbiddenimports.Analyzer,
	entquerysafety.Analyzer,
	manualdi.Analyzer,
	openapirbaccontract.Analyzer,
	authproviderlayering.Analyzer,
	rbacguards.Analyzer,
	riverbypass.Analyzer,
	runtimemock.Analyzer,
	semaphoreusage.Analyzer,
	txboundary.Analyzer,
	riverjobargs.Analyzer,
	// Batch 2: provider-layer compliance (ADR-0011) and transaction safety (ADR-0006/ADR-0012)
	ssacompliance.Analyzer,
	k8sintransaction.Analyzer,
	// Batch 3: ADR-0038 ResourceVersion enforcement for K8s polling
	k8spollingrv.Analyzer,
}

// New is the golangci-lint v2 module plugin entrypoint.
// Signature: func New(settings any) (register.LinterPlugin, error)
func New(settings any) (register.LinterPlugin, error) {
	return &shepherdArchPlugin{}, nil
}

// shepherdArchPlugin implements register.LinterPlugin.
type shepherdArchPlugin struct{}

// BuildAnalyzers returns all architecture enforcement analyzers.
func (p *shepherdArchPlugin) BuildAnalyzers() ([]*analysis.Analyzer, error) {
	return AllAnalyzers, nil
}

// GetLoadMode returns the load mode for the plugin.
// k8spollingrv uses pass.TypesInfo to recognize real Kubernetes metav1 option
// types, so the plugin must request type information in golangci-lint too.
func (p *shepherdArchPlugin) GetLoadMode() string {
	return register.LoadModeTypesInfo
}
