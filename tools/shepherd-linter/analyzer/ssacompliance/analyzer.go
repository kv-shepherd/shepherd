// Package ssacompliance implements a go/analysis Analyzer that enforces
// Server-Side Apply (SSA) compliance in the KubeVirt provider layer (ADR-0011).
//
// This Analyzer is a go/analysis-compatible re-implementation of the
// legacy CI script `check_kubevirt_ssa_compliance.go`.
//
// Rules enforced (in internal/provider/ packages, non-test files):
//
//  1. Forbidden struct literals: Write paths must not construct KubeVirt typed structs.
//     e.g. kubevirtv1.VirtualMachine{...} is forbidden outside exempt files.
//     Use YAML template + Unstructured + SSA instead.
//
//  2. Forbidden method calls: Typed-client .Create() and .Update() are banned.
//     Use DynamicSSAClient.ApplyYAML() for writes.
//
// Exemptions (file-level, by basename):
//   - mapper.go           — read-only type mapping, no write paths
//   - kubecli_adapter.go  — forwarding layer that wraps kubecli
//   - mock.go             — test doubles, not production write paths
//   - client.go           — interface definitions only
//   - interface.go        — interface definitions only
//   - capability.go       — capability detection, no VM writes
//   - health_checker.go   — health probes, no VM writes
//   - auth.go             — authentication helpers
//
// Per-line suppression: annotate the offending line with // ssa-compliance:ignore
//
// Reference: ADR-0011 K8s Resource Submission Strategy — SSA + Unstructured
package ssacompliance

import (
	"go/ast"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

// Analyzer is the exported go/analysis.Analyzer for SSA compliance checking.
var Analyzer = &analysis.Analyzer{
	Name:     "ssacompliance",
	Doc:      "ADR-0011: enforces that KubeVirt provider write paths use SSA+Unstructured, not typed structs or .Create()/.Update()",
	Requires: []*analysis.Analyzer{inspect.Analyzer},
	Run:      run,
}

// exemptFiles are excluded from all checks by filename (basename only).
// These files either perform read-only mapping or act as pure forwarding layers
// and therefore legitimately reference KubeVirt typed structs.
var exemptFiles = map[string]bool{
	"mapper.go":          true, // read-only: maps K8s types → domain types
	"kubecli_adapter.go": true, // forwarding: wraps kubecli calls, no business logic
	"mock.go":            true, // test doubles, not production write paths
	"client.go":          true, // interface definitions only
	"interface.go":       true, // interface definitions only
	"capability.go":      true, // capability detection, no VM writes
	"health_checker.go":  true, // health probes, no VM writes
	"auth.go":            true, // authentication helpers
}

// forbiddenStructTypes are KubeVirt typed struct literals forbidden in write paths.
// Read paths (mapper.go) are exempt. Write paths must use YAML + Unstructured + SSA.
var forbiddenStructTypes = map[string]bool{
	"kubevirtv1.VirtualMachine":         true,
	"kubevirtv1.VirtualMachineSpec":     true,
	"kubevirtv1.VirtualMachineInstance": true,
	"kubevirtv1.DomainSpec":             true,
	"kubevirtv1.ResourceRequirements":   true,
	"kubevirtv1.CPU":                    true,
	"kubevirtv1.Memory":                 true,
	"kubevirtv1.Disk":                   true,
	"kubevirtv1.Volume":                 true,
	"v1.VirtualMachine":                 true,
	"v1.VirtualMachineSpec":             true,
	"v1.VirtualMachineInstance":         true,
	"v1.DomainSpec":                     true,
}

// forbiddenMethodMessages maps typed-client Write method names to diagnostic messages.
// These represent Get-Modify-Put anti-patterns (ADR-0011).
var forbiddenMethodMessages = map[string]string{
	"Create": "typed-client .Create() is forbidden (ADR-0011): use DynamicSSAClient.ApplyYAML() instead",
	"Update": "typed-client .Update() is forbidden (ADR-0011): Get-Modify-Put is banned; use SSA Patch instead",
}

// suppressionMarker is the per-line annotation that disables this check for one line.
const suppressionMarker = "ssa-compliance:ignore"

func run(pass *analysis.Pass) (interface{}, error) {
	// Only enforce inside internal/provider packages.
	if !isProviderPkg(pass) {
		return nil, nil
	}

	// Build a per-file, per-line comment index for suppression lookups.
	// go/analysis best practice: use pass.Files (already parsed by framework),
	// never read files from disk inside Run().
	suppressedLines := buildSuppressedLines(pass)

	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

	// --- Check 1: forbidden struct literals (CompositeLit) ---
	insp.Preorder([]ast.Node{(*ast.CompositeLit)(nil)}, func(n ast.Node) {
		cl := n.(*ast.CompositeLit)
		pos := pass.Fset.Position(cl.Pos())

		// Skip test files and exempt files.
		if strings.HasSuffix(pos.Filename, "_test.go") || exemptFiles[filepath.Base(pos.Filename)] {
			return
		}

		// Skip suppressed lines.
		if suppressedLines[fileLineKey(pos)] {
			return
		}

		typeName := selectorExprToString(cl.Type)
		if forbiddenStructTypes[typeName] {
			pass.Reportf(cl.Pos(),
				"write path constructs forbidden KubeVirt typed struct %s{...} (ADR-0011): use YAML template + Unstructured + SSA instead",
				typeName)
		}
	})

	// --- Check 2: forbidden method calls (.Create / .Update) ---
	insp.Preorder([]ast.Node{(*ast.CallExpr)(nil)}, func(n ast.Node) {
		call := n.(*ast.CallExpr)
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return
		}

		msg, forbidden := forbiddenMethodMessages[sel.Sel.Name]
		if !forbidden {
			return
		}

		pos := pass.Fset.Position(call.Pos())
		if strings.HasSuffix(pos.Filename, "_test.go") || exemptFiles[filepath.Base(pos.Filename)] {
			return
		}
		if suppressedLines[fileLineKey(pos)] {
			return
		}

		pass.Reportf(call.Pos(), "%s", msg)
	})

	return nil, nil
}

// isProviderPkg returns true if this package is under internal/provider.
func isProviderPkg(pass *analysis.Pass) bool {
	if pass.Pkg == nil {
		return false
	}
	pkgPath := pass.Pkg.Path()
	return strings.Contains(pkgPath, "/provider") ||
		strings.HasSuffix(pkgPath, "/provider")
}

// fileLineKey returns a stable key for (filename, line) pairs used in suppression lookup.
func fileLineKey(pos token.Position) string {
	return pos.Filename + ":" + strconv.Itoa(pos.Line)
}

// buildSuppressedLines scans all comments in pass.Files and returns a set of
// "filename:line" keys where the suppression marker is present.
//
// go/analysis best practice: use pass.Files (framework-provided parsed ASTs)
// rather than os.ReadFile. This keeps the analyzer hermetic and fast.
func buildSuppressedLines(pass *analysis.Pass) map[string]bool {
	result := make(map[string]bool)
	for _, f := range pass.Files {
		for _, cg := range f.Comments {
			for _, c := range cg.List {
				if strings.Contains(c.Text, suppressionMarker) {
					cPos := pass.Fset.Position(c.Pos())
					result[fileLineKey(cPos)] = true
				}
			}
		}
	}
	return result
}

// selectorExprToString converts an AST expression to a "pkg.TypeName" string.
// Handles pointer types (*kubevirtv1.VirtualMachine → kubevirtv1.VirtualMachine).
// Based on go/ast best practice: recursive SelectorExpr traversal.
//
// Context7 reference: go/ast SelectorExpr pattern — e.X is package Ident, e.Sel is type Ident.
func selectorExprToString(expr ast.Expr) string {
	if expr == nil {
		return ""
	}
	switch e := expr.(type) {
	case *ast.SelectorExpr:
		return selectorExprToString(e.X) + "." + e.Sel.Name
	case *ast.Ident:
		return e.Name
	case *ast.StarExpr:
		// Dereference pointer types transparently.
		return selectorExprToString(e.X)
	default:
		return ""
	}
}
