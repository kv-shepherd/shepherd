//go:build ignore

package main

import (
	"fmt"
	"os"
	"strings"
)

type fileRequirement struct {
	path      string
	fragments []string
}

func main() {
	requirements := []fileRequirement{
		{
			path: "internal/governance/approval/gateway.go",
			fragments: []string{
				"ValidateApproval(ctx, clusterID, effectiveInstanceSizeID, payload.Namespace)",
			},
		},
		{
			path: "internal/service/approval_validator.go",
			fragments: []string{
				"Where(namespaceregistry.NameEQ(strings.TrimSpace(namespace))).",
				"validateNamespaceClusterEnvironment(string(ns.Environment), string(cl.Environment))",
				"NAMESPACE_CLUSTER_ENV_MISMATCH",
			},
		},
		{
			path: "internal/jobs/vm_create.go",
			fragments: []string{
				"if err := w.ensureNamespaceClusterEnvironment(ctx, clusterID, namespace); err != nil {",
				"func (w *VMCreateWorker) ensureNamespaceClusterEnvironment(",
				"Where(namespaceregistry.NameEQ(nsName)).",
				"validateNamespaceClusterEnvironment(string(ns.Environment), string(cl.Environment))",
			},
		},
		{
			path: "internal/api/handlers/environment_visibility.go",
			fragments: []string{
				"func (s *Server) resolveNamespaceVisibility(",
				"rolebinding.HasUserWith(entuser.IDEQ(actor))",
				"rb.AllowedEnvironments",
			},
		},
		{
			path: "internal/api/handlers/server_namespace.go",
			fragments: []string{
				"visibility, err := s.resolveNamespaceVisibility(c)",
				"query = query.Where(namespaceregistry.EnvironmentIn(visibility.envs...))",
			},
		},
		{
			path: "internal/api/handlers/server_vm.go",
			fragments: []string{
				"visibility, err := s.resolveNamespaceVisibility(c)",
				"visibleNamespaces, err := s.listVisibleNamespaceNames(ctx, visibility)",
				"visible, err := s.isNamespaceVisible(ctx, req.Namespace, visibility)",
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
		for _, fragment := range req.fragments {
			if !strings.Contains(text, fragment) {
				violations = append(violations, fmt.Sprintf("%s: missing %q", req.path, fragment))
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
