//go:build ignore

package main

import (
	"fmt"
	"os"
	"strings"
)

type fileGuardRule struct {
	path      string
	required  []string
	forbidden []string
}

func main() {
	rules := []fileGuardRule{
		{
			path: "internal/api/handlers/member.go",
			required: []string{
				`requireActorWithAnyGlobalPermission(c, "user:manage", "rbac:read", "rbac:manage")`,
			},
		},
		{
			path: "internal/api/handlers/server_namespace.go",
			required: []string{
				`requireActorWithAnyGlobalPermission(c, "cluster:read", "cluster:write", "cluster:manage")`,
				`requireActorWithAnyGlobalPermission(c, "cluster:write", "cluster:manage")`,
			},
			forbidden: []string{
				`middleware.GetUserID(`,
			},
		},
		{
			path: "internal/api/handlers/server_admin.go",
			required: []string{
				`requireAnyGlobalPermission(c, "vm:create", "template:read", "template:manage")`,
				`requireAnyGlobalPermission(c, "vm:create", "instance_size:read", "instance_size:write")`,
			},
		},
	}

	var failures []string
	for _, rule := range rules {
		raw, err := os.ReadFile(rule.path)
		if err != nil {
			failures = append(failures, fmt.Sprintf("read %s failed: %v", rule.path, err))
			continue
		}
		content := string(raw)
		for _, snippet := range rule.required {
			if !strings.Contains(content, snippet) {
				failures = append(failures, fmt.Sprintf("%s missing required RBAC guard: %s", rule.path, snippet))
			}
		}
		for _, snippet := range rule.forbidden {
			if strings.Contains(content, snippet) {
				failures = append(failures, fmt.Sprintf("%s contains forbidden legacy pattern: %s", rule.path, snippet))
			}
		}
	}

	if len(failures) > 0 {
		fmt.Println("FAIL: explicit handler RBAC guard check failed")
		for _, item := range failures {
			fmt.Printf(" - %s\n", item)
		}
		fmt.Println("Rule: protected handlers must keep explicit, fail-closed permission guards.")
		os.Exit(1)
	}

	fmt.Println("OK: explicit handler RBAC guards are present")
}
