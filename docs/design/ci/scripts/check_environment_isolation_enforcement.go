//go:build ignore

package main

import (
	"fmt"
	"os"
	"regexp"
)

type fileRequirement struct {
	path          string
	patternGroups [][]string
}

func main() {
	requirements := []fileRequirement{
		{
			path: "internal/governance/ticketing/service.go",
			patternGroups: [][]string{
				{
					`(?s)ValidateApproval\(ctx,.*payload\.Namespace`,
					`(?s)EvaluateClusterPlacement\(ctx,.*Namespace:\s*payload\.Namespace`,
				},
			},
		},
		{
			path: "internal/service/approval_validator.go",
			patternGroups: [][]string{
				{
					regexp.QuoteMeta("Where(namespaceregistry.NameEQ(strings.TrimSpace(namespace)))."),
					regexp.QuoteMeta("Where(namespaceregistry.NameEQ(strings.TrimSpace(input.Namespace)))."),
				},
				{
					regexp.QuoteMeta("validateNamespaceClusterEnvironment(string(ns.Environment), string(cl.Environment))"),
				},
				{
					regexp.QuoteMeta("NAMESPACE_CLUSTER_ENV_MISMATCH"),
				},
			},
		},
		{
			path: "internal/jobs/vm_create.go",
			patternGroups: [][]string{
				{
					`ensureNamespaceClusterEnvironment\(ctx,\s*clusterID,\s*namespace\)`,
				},
				{
					regexp.QuoteMeta("func (w *VMCreateWorker) ensureNamespaceClusterEnvironment("),
				},
				{
					regexp.QuoteMeta("Where(namespaceregistry.NameEQ(nsName))."),
				},
				{
					regexp.QuoteMeta("validateNamespaceClusterEnvironment(string(ns.Environment), string(cl.Environment))"),
				},
			},
		},
		{
			path: "internal/api/handlers/environment_visibility.go",
			patternGroups: [][]string{
				{
					regexp.QuoteMeta("func (s *Server) resolveNamespaceVisibility("),
				},
				{
					regexp.QuoteMeta("rolebinding.HasUserWith(entuser.IDEQ(actor))"),
				},
				{
					regexp.QuoteMeta("rb.AllowedEnvironments"),
				},
			},
		},
		{
			path: "internal/api/handlers/server_namespace.go",
			patternGroups: [][]string{
				{
					`resolveNamespaceVisibility\(c\)`,
				},
				{
					`EnvironmentIn\(visibility\.envs\.\.\.\)`,
				},
			},
		},
		{
			path: "internal/api/handlers/server_vm.go",
			patternGroups: [][]string{
				{
					`resolveNamespaceVisibility\(c\)`,
				},
				{
					`listVisibleNamespaceNames\(ctx,\s*visibility\)`,
				},
				{
					`isNamespaceVisible\(ctx,\s*req\.Namespace,\s*visibility\)`,
				},
			},
		},
	}

	var violations []string
	for _, req := range requirements {
		src, err := os.ReadFile(req.path)
		if err != nil {
			violations = append(violations, fmt.Sprintf("read %s: %v", req.path, err))
			continue
		}
		text := string(src)
		for _, patternGroup := range req.patternGroups {
			matched := false
			for _, pattern := range patternGroup {
				groupMatch, matchErr := regexp.MatchString(pattern, text)
				if matchErr != nil {
					violations = append(violations, fmt.Sprintf("%s: invalid pattern %q: %v", req.path, pattern, matchErr))
					matched = true
					break
				}
				if groupMatch {
					matched = true
					break
				}
			}
			if !matched {
				violations = append(violations, fmt.Sprintf("%s: missing any acceptable pattern in %q", req.path, patternGroup))
			}
		}
	}

	if len(violations) > 0 {
		fmt.Println("FAIL: environment isolation enforcement check failed")
		for _, v := range violations {
			fmt.Println(" -", v)
		}
		fmt.Println("Rule: namespace.environment isolation must be enforced in approval path, worker path, and user visibility filtering path.")
		os.Exit(1)
	}

	fmt.Println("OK: environment isolation enforcement check passed")
}
