// Package riverjobargs implements a go/analysis Analyzer that enforces
// the River Queue Claim Check pattern (ADR-0009).
//
// This Analyzer is a go/analysis-compatible re-implementation of the
// legacy Batch1 CI script `check_river_job_args.go` (removed post-ADR-0039 migration).
//
// Rule enforced (ADR-0009):
//   - River Job Args structs (names ending in "JobArgs" or "Args") must not
//     contain business payload fields such as operation, VM identity, cluster
//     identity, or ticket snapshots.
//   - Workers must receive only claim-check identifiers and retrieve full data
//     from DomainEvent or the owning job table.
//
// Exemptions:
//   - Fields named EventID, JobID, BatchID, TraceID are allowed.
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
	Doc:      "Enforces ADR-0009 River Claim Check: Job Args structs must contain only claim-check identifiers",
	Requires: []*analysis.Analyzer{inspect.Analyzer},
	Run:      run,
}

// allowedJobArgFields are claim-check identifiers, not business payload.
var allowedJobArgFields = map[string]bool{
	"EventID": true,
	"JobID":   true,
	"BatchID": true,
	"TraceID": true,
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
				if !allowedJobArgFields[ident.Name] {
					pass.Reportf(field.Pos(),
						"River Job Args %s contains non-claim-check field %q: "+
							"job args may only carry EventID, JobID, BatchID, or TraceID; "+
							"retrieve business data from DomainEvent or the owning job table (ADR-0009)",
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
