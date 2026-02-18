//go:build ignore

package main

import (
	"fmt"
	"os"
	"strings"
)

func main() {
	type checkFile struct {
		path      string
		mustHave  []string
		mustNot   []string
		ruleLabel string
	}

	checks := []checkFile{
		{
			path: "internal/usecase/create_vm.go",
			mustHave: []string{
				"findPendingCreateDuplicate(",
				`"existing_ticket_id"`,
			},
			mustNot: []string{
				"approvalticket.RequesterEQ(input.RequestedBy)",
				"pending VM request already exists for this user",
			},
			ruleLabel: "create duplicate guard must be resource-scoped and return existing ticket reference",
		},
		{
			path: "internal/usecase/delete_vm.go",
			mustHave: []string{
				"findPendingDeleteDuplicate(",
				`"existing_ticket_id"`,
			},
			mustNot: []string{
				"approvalticket.RequesterEQ(input.RequestedBy)",
				"pending VM delete request already exists for this user",
			},
			ruleLabel: "delete duplicate guard must be resource-scoped and return existing ticket reference",
		},
	}

	var violations []string
	for _, check := range checks {
		contentBytes, err := os.ReadFile(check.path)
		if err != nil {
			violations = append(violations, fmt.Sprintf("%s: read failed: %v", check.path, err))
			continue
		}
		content := string(contentBytes)

		for _, token := range check.mustHave {
			if !strings.Contains(content, token) {
				violations = append(violations, fmt.Sprintf("%s: missing token %q (%s)", check.path, token, check.ruleLabel))
			}
		}
		for _, token := range check.mustNot {
			if strings.Contains(content, token) {
				violations = append(violations, fmt.Sprintf("%s: forbidden token %q (%s)", check.path, token, check.ruleLabel))
			}
		}
	}

	if len(violations) > 0 {
		fmt.Println("FAIL: duplicate request guard scope check failed")
		for _, v := range violations {
			fmt.Println(" -", v)
		}
		fmt.Println("Rule: duplicate guard must use same-resource + same-operation semantics and return existing_ticket_id.")
		os.Exit(1)
	}

	fmt.Println("OK: duplicate request guards are resource-scoped with existing ticket reference")
}
