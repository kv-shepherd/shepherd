// Package riverjobargs implements a go/analysis Analyzer that enforces
// the River Queue Claim Check pattern (ADR-0009).
//
// This Analyzer is a go/analysis-compatible re-implementation of the
// legacy Batch1 CI script `check_river_job_args.go` (removed post-ADR-0039 migration).
//
// Rule enforced (ADR-0009):
//   - River Job Args structs (names ending in "JobArgs" or "Args") must not
//     contain direct business-entity ID fields (vm_id, ticket_id, cluster_id, etc.).
//   - Workers must receive only an EventID and retrieve full data via DomainEvent lookup.
//
// Exemptions:
//   - Fields named EventID, BatchID, Metadata, TraceID are allowed.
//
// Applies to: internal/usecase, internal/worker, internal/jobs packages.
package riverjobargs

import (
	"go/ast"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

// Analyzer is the exported go/analysis.Analyzer for River Job Args claim check.
var Analyzer = &analysis.Analyzer{
	Name:     "riverjobargs",
	Doc:      "Enforces ADR-0009 River Claim Check: Job Args structs must not contain direct business-entity ID fields",
	Requires: []*analysis.Analyzer{inspect.Analyzer},
	Run:      run,
}

// forbiddenJobArgFields are field names that violate the Claim Check pattern.
var forbiddenJobArgFields = map[string]bool{
	"VMID":       true,
	"VmID":       true,
	"VMId":       true,
	"vm_id":      true,
	"TicketID":   true,
	"ticket_id":  true,
	"ServiceID":  true,
	"service_id": true,
	"SystemID":   true,
	"system_id":  true,
	"ClusterID":  true,
	"cluster_id": true,
}

func run(pass *analysis.Pass) (interface{}, error) {
	// Only enforce inside job-related packages.
	if !isJobPkg(pass) {
		return nil, nil
	}

	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

	// Visit struct type declarations.
	nodeFilter := []ast.Node{(*ast.TypeSpec)(nil)}
	insp.Preorder(nodeFilter, func(n ast.Node) {
		ts, ok := n.(*ast.TypeSpec)
		if !ok {
			return
		}

		// Only check types whose name ends with "JobArgs" or "Args".
		name := ts.Name.Name
		if !strings.HasSuffix(name, "JobArgs") && !strings.HasSuffix(name, "Args") {
			return
		}

		st, ok := ts.Type.(*ast.StructType)
		if !ok || st.Fields == nil {
			return
		}

		// Skip test files.
		pos := pass.Fset.Position(ts.Pos())
		if strings.HasSuffix(pos.Filename, "_test.go") {
			return
		}

		for _, field := range st.Fields.List {
			for _, ident := range field.Names {
				if forbiddenJobArgFields[ident.Name] {
					pass.Reportf(field.Pos(),
						"River Job Args %s contains forbidden field %q: "+
							"use EventID only and retrieve entity data via DomainEvent lookup (ADR-0009)",
						name, ident.Name)
				}
			}
		}
	})

	return nil, nil
}

// isJobPkg returns true if the package is under internal/usecase, internal/worker, or internal/jobs.
func isJobPkg(pass *analysis.Pass) bool {
	if pass.Pkg == nil {
		return false
	}

	pkgPath := pass.Pkg.Path()
	for _, segment := range []string{"/usecase", "/worker", "/jobs"} {
		if strings.Contains(pkgPath, segment) {
			return true
		}
	}
	return false
}
