//go:build ignore

// check_kubevirt_ssa_compliance.go
//
// ADR-0011 enforcement: KubeVirt write paths must use Server-Side Apply (SSA).
//
// FORBIDDEN PATTERNS (in internal/provider/*.go, excluding exempt files):
//
//  1. Constructing KubeVirt typed struct literals in write paths:
//     e.g. kubevirtv1.VirtualMachine{...}, kubevirtv1.DomainSpec{...}, etc.
//     Read paths (mapper.go) are exempt; write paths must use YAML + Unstructured + SSA.
//
//  2. Calling typed client Write methods:
//     - .Create(...)  → must be replaced by DynamicSSAClient.ApplyYAML()
//     - .Update(...)  → Get-Modify-Put is banned; use SSA Patch instead
//
// DETECTION STRATEGY (from context7 go/ast best practices):
//   - AST-based: ast.Inspect with *ast.CompositeLit for struct literals (low false-positive)
//   - String-scan: detect .Create( / .Update( call sites (simple, sufficient for this scope)
//
// EXEMPTIONS:
//   - *_test.go files
//   - Files listed in exemptFiles (read-only or forwarding layers)
//   - Lines annotated with: // ssa-compliance:ignore
//
// REFERENCES:
//   - ADR-0011: K8s Resource Submission Strategy — Server-Side Apply + Unstructured
//   - ADR-0001: KubeVirt Client Selection (official kubevirt.io/client-go)
//
// Run locally: go run docs/design/ci/scripts/check_kubevirt_ssa_compliance.go
// Or:          make check-kubevirt-ssa

package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

// providerDir is the only directory subject to this check.
// The KubeVirt provider is the sole component that talks to the KubeVirt API.
const providerDir = "internal/provider"

// exemptFiles are fully excluded from all checks.
// These files either perform read-only mapping or act as pure forwarding layers
// and therefore legitimately reference KubeVirt typed structs.
var exemptFiles = map[string]bool{
	"mapper.go":                       true, // read-only: maps K8s types → domain types
	"kubecli_adapter.go":              true, // forwarding: wraps kubecli calls, no business logic
	"mock.go":                         true, // test doubles, not production write paths
	"client.go":                       true, // interface definitions only
	"interface.go":                    true, // interface definitions only
	"capability.go":                   true, // capability detection, no VM writes
	"health_checker.go":               true, // health probes, no VM writes
	"auth.go":                         true,
	"auth_provider_admin_registry.go": true,
	"ssa_applier.go":                  true, // SSA implementation itself (uses Unstructured, not typed structs)
}

// forbiddenStructTypes lists KubeVirt typed struct names that must NOT be constructed
// in write paths. After the ADR-0011 fix, VM creation/update goes through
// YAML templates → Unstructured → SSA Patch, not typed struct assembly.
//
// Format: "pkgAlias.TypeName" matching *ast.SelectorExpr in a CompositeLit.
var forbiddenStructTypes = []string{
	"kubevirtv1.VirtualMachine",
	"kubevirtv1.VirtualMachineSpec",
	"kubevirtv1.VirtualMachineInstanceTemplateSpec",
	"kubevirtv1.VirtualMachineInstanceSpec",
	"kubevirtv1.DomainSpec",
	"kubevirtv1.CPU",
	"kubevirtv1.ResourceRequirements",
	"kubevirtv1.Disk",
	"kubevirtv1.DiskDevice",
	"kubevirtv1.DiskTarget",
	"kubevirtv1.Volume",
	"kubevirtv1.VolumeSource",
	"kubevirtv1.ContainerDiskSource",
	"kubevirtv1.PersistentVolumeClaimVolumeSource",
	"kubevirtv1.EmptyDiskSource",
}

// violation records a single detected policy breach.
type violation struct {
	file    string
	line    int
	pattern string
	message string
}

func main() {
	var violations []violation

	err := filepath.Walk(providerDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil // skip missing directories gracefully
			}
			return err
		}
		if info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		// Skip test files: test code legitimately builds mock typed structs.
		if strings.HasSuffix(path, "_test.go") {
			return nil
		}
		// Skip fully-exempt files (read paths and forwarding layers).
		if exemptFiles[filepath.Base(path)] {
			return nil
		}

		checkFile(path, &violations)
		return nil
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: walking %s: %v\n", providerDir, err)
		os.Exit(2) // exit 2 = script error (distinct from exit 1 = violations found)
	}

	if len(violations) == 0 {
		fmt.Println("OK: KubeVirt SSA compliance check passed (ADR-0011)")
		return
	}

	fmt.Println("FAIL: KubeVirt SSA compliance violations detected (ADR-0011)")
	fmt.Println()
	for _, v := range violations {
		fmt.Printf("  %s:%d\n", v.file, v.line)
		fmt.Printf("  |- pattern : %s\n", v.pattern)
		fmt.Printf("  `- reason  : %s\n\n", v.message)
	}

	fmt.Println("Rule (ADR-0011): VM write paths (CreateVM/UpdateVM) must use:")
	fmt.Println("  dynamic.Client.Patch(types.ApplyPatchType, ...) with FieldManager=kubevirt-shepherd")
	fmt.Println("  Constructing kubevirtv1.VirtualMachine{} typed structs is forbidden in write paths.")
	fmt.Println("  Calling typed-client .Create() or .Update() is forbidden.")
	fmt.Println("")
	fmt.Println("Exemptions:")
	fmt.Println("  - mapper.go, kubecli_adapter.go (read paths / forwarding layers)")
	fmt.Println("  - *_test.go files")
	fmt.Println("  - Lines annotated with: // ssa-compliance:ignore")
	os.Exit(1)
}

// checkFile runs both AST-based struct literal detection and string-based call detection.
func checkFile(path string, violations *[]violation) {
	src, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "WARNING: cannot read %s: %v\n", path, err)
		return
	}

	lines := strings.Split(string(src), "\n")

	fset := token.NewFileSet()
	// Use parser.AllErrors to tolerate partial parse failures.
	node, parseErr := parser.ParseFile(fset, path, src, parser.AllErrors)
	if parseErr != nil {
		// Fallback to string scanning when AST parse fails.
		fmt.Fprintf(os.Stderr, "WARNING: AST parse failed for %s, falling back to string scan: %v\n", path, parseErr)
		stringFallbackCheck(path, lines, violations)
		return
	}

	// --- AST-based check: detect forbidden struct literal construction ---
	// Based on context7 go/ast pattern: ast.Inspect with *ast.CompositeLit.
	// A CompositeLit whose Type is a SelectorExpr matching "pkg.TypeName"
	// indicates construction of a typed struct literal.
	ast.Inspect(node, func(n ast.Node) bool {
		if n == nil {
			return false
		}
		cl, ok := n.(*ast.CompositeLit)
		if !ok {
			return true // continue traversal
		}

		typeName := selectorExprToString(cl.Type)
		for _, forbidden := range forbiddenStructTypes {
			if typeName != forbidden {
				continue
			}
			pos := fset.Position(cl.Pos())
			if hasIgnoreAnnotation(lines, pos.Line-1) {
				continue
			}
			*violations = append(*violations, violation{
				file:    path,
				line:    pos.Line,
				pattern: forbidden + "{...}",
				message: "Write path must not construct KubeVirt typed structs. Use YAML template + Unstructured + SSA instead.",
			})
		}
		return true
	})

	// --- String-based check: detect forbidden typed-client call sites ---
	detectForbiddenCalls(path, lines, violations)
}

// detectForbiddenCalls scans line-by-line for typed-client Write method calls.
// These are sufficiently distinctive that string matching has low false-positive risk
// within the narrow scope of internal/provider/*.go.
func detectForbiddenCalls(path string, lines []string, violations *[]violation) {
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		// Skip blank lines and full-line comments.
		if trimmed == "" || strings.HasPrefix(trimmed, "//") {
			continue
		}
		// Skip lines with the ignore annotation.
		if hasIgnoreAnnotation(lines, i) {
			continue
		}

		// Detect .Create( — but exclude:
		//   - Function/method declarations: "func ... Create(...)"
		//   - Struct field references: "CreateOptions{"
		//   - Import paths
		if strings.Contains(line, ".Create(") &&
			!strings.Contains(line, "func ") &&
			!strings.Contains(line, "CreateOptions{") &&
			!strings.Contains(line, "\"") {
			*violations = append(*violations, violation{
				file:    path,
				line:    i + 1,
				pattern: ".Create(...)",
				message: "Typed-client .Create() is forbidden. Use DynamicSSAClient.ApplyYAML() instead (ADR-0011).",
			})
		}

		// Detect .Update( — same exclusion logic.
		if strings.Contains(line, ".Update(") &&
			!strings.Contains(line, "func ") &&
			!strings.Contains(line, "UpdateOptions{") &&
			!strings.Contains(line, "\"") {
			*violations = append(*violations, violation{
				file:    path,
				line:    i + 1,
				pattern: ".Update(...)",
				message: "Typed-client .Update() is forbidden. Get-Modify-Put is banned. Use SSA Patch instead (ADR-0011).",
			})
		}
	}
}

// stringFallbackCheck is used when AST parsing fails. It checks both struct
// literals (by presence of "kubevirtv1.TypeName{") and call patterns.
func stringFallbackCheck(path string, lines []string, violations *[]violation) {
	for i, line := range lines {
		if strings.TrimSpace(line) == "" || strings.HasPrefix(strings.TrimSpace(line), "//") {
			continue
		}
		if hasIgnoreAnnotation(lines, i) {
			continue
		}
		for _, forbidden := range forbiddenStructTypes {
			if strings.Contains(line, forbidden+"{") {
				*violations = append(*violations, violation{
					file:    path,
					line:    i + 1,
					pattern: forbidden + "{...}",
					message: "Write path must not construct KubeVirt typed structs (string-scan fallback).",
				})
			}
		}
	}
	detectForbiddenCalls(path, lines, violations)
}

// selectorExprToString converts an AST expression to a "pkg.TypeName" string.
// Used to match CompositeLit type expressions against the forbidden list.
// Based on the go/ast SelectorExpr pattern from context7 documentation.
func selectorExprToString(expr ast.Expr) string {
	if expr == nil {
		return ""
	}
	switch e := expr.(type) {
	case *ast.SelectorExpr:
		// e.X is the package identifier, e.Sel is the type name.
		return selectorExprToString(e.X) + "." + e.Sel.Name
	case *ast.Ident:
		return e.Name
	case *ast.StarExpr:
		// Handle pointer types: *kubevirtv1.VirtualMachine
		return selectorExprToString(e.X)
	default:
		return ""
	}
}

// hasIgnoreAnnotation checks whether the line at index lineIdx (0-based)
// contains the per-line suppression annotation "// ssa-compliance:ignore".
// This allows deliberate one-off exemptions with explicit acknowledgment.
func hasIgnoreAnnotation(lines []string, lineIdx int) bool {
	if lineIdx < 0 || lineIdx >= len(lines) {
		return false
	}
	return strings.Contains(lines[lineIdx], "// ssa-compliance:ignore")
}
