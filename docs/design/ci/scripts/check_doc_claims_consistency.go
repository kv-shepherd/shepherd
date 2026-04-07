//go:build ignore

package main

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strings"
)

const (
	phase4Checklist         = "docs/design/checklist/phase-4-checklist.md"
	phase5Checklist         = "docs/design/checklist/phase-5-checklist.md"
	phase5DesignDoc         = "docs/design/phases/05-auth-api-frontend.md"
	phase4Governance        = "docs/design/phases/04-governance.md"
	adr0025                 = "docs/adr/ADR-0025-secret-bootstrap.md"
	adr0033                 = "docs/adr/ADR-0033-realtime-notification-acceleration.md"
	adr0050                 = "docs/adr/ADR-0050-upstream-identity-assertion-runtime-provider.md"
	openAPIFile             = "api/openapi.yaml"
	approvalHandler         = "internal/api/handlers/server_approval.go"
	rbacMiddleware          = "internal/api/middleware/rbac.go"
	vmHandler               = "internal/api/handlers/server_vm.go"
	namespaceHandler        = "internal/api/handlers/server_namespace.go"
	envVisibility           = "internal/api/handlers/environment_visibility.go"
	memberHandler           = "internal/api/handlers/member.go"
	usersPage               = "web/src/app/(protected)/admin/users/page.tsx"
	secretBootstrapResolver = "internal/infrastructure/security_bootstrap.go"
	modulesInfrastructure   = "internal/app/modules/infrastructure.go"
)

var pathRe = regexp.MustCompile(`^\s{2}(/[^:]+):\s*$`)

func main() {
	var violations []string

	violations = append(violations, findForbiddenDocClaims(
		map[string][]string{
			phase4Checklist: {
				"`ExternalApprovalProvider` contract defined (`SubmitForApproval`, `CheckStatus`, `CancelRequest`) — in `internal/provider/auth.go`",
				"**Notification Service (Reserved Interface)** defined (`internal/provider/auth.go`)",
			},
			phase4Governance: {
				"// ExternalApprovalProvider defines the contract for external approval systems",
				"type ExternalApprovalProvider interface {",
			},
			"docs/design/checklist/phase-1-checklist.md": {
				"**AuthProvider Interface** defined (`internal/provider/auth.go`)",
				"**ApprovalProvider Interface** defined (`internal/provider/auth.go`)",
				"**NotificationProvider Interface** defined (`internal/provider/auth.go`)",
			},
			"docs/design/checklist/phase-2-checklist.md": {
				"Interface definitions (`ApprovalProvider` in `internal/provider/auth.go`)",
			},
			adr0033: {
				"Architecture review confirms recipient-scoped fan-out routing model is implemented (no global broadcast for user-scoped events).",
				"Code review confirms DB-trigger-based emission exists for notification acceleration path and is covered by migration/test artifacts.",
				"Integration tests cover:",
				"Load tests validate that listener path does not affect write-path SLO.",
			},
			adr0050: {
				"Runtime auth handlers resolve legacy upstream assertion providers via registry only.",
				"Tests prove Shepherd rejects plain username-only handoff.",
				"Tests prove a verified upstream assertion can normalize into canonical `AuthResult` and create/update users through JIT provisioning.",
				"Authorization tests prove upstream claims do not bypass Shepherd RBAC.",
			},
		},
	)...)

	if !fileExists(memberHandler) {
		violations = append(violations, checkedClaimViolations(
			phase5Checklist,
			"MemberHandler",
			"MemberHandler is checked as done, but internal/api/handlers/member.go does not exist",
		)...)
		if row, line, ok, err := rowClaimedDone(phase5DesignDoc, "Member Handler"); err != nil {
			fmt.Printf("FAIL: inspect %s for Member Handler claim: %v\n", phase5DesignDoc, err)
			os.Exit(1)
		} else if ok {
			violations = append(violations,
				fmt.Sprintf("%s:%d: Member Handler is marked done, but internal/api/handlers/member.go does not exist (%s)", phase5DesignDoc, line, row))
		}
	}

	hasCancelPath, err := openAPIHasPath("/tickets/{ticket_id}/cancel")
	if err != nil {
		fmt.Printf("FAIL: inspect OpenAPI: %v\n", err)
		os.Exit(1)
	}
	hasCancelHandler, err := serverHasMethod(approvalHandler, "CancelTicket")
	if err != nil {
		fmt.Printf("FAIL: inspect approval handler: %v\n", err)
		os.Exit(1)
	}

	if !hasCancelPath || !hasCancelHandler {
		violations = append(violations, checkedClaimViolations(
			phase4Checklist,
			"User Self-Cancellation",
			"User Self-Cancellation is checked as done, but cancel API contract/handler is missing",
		)...)
		violations = append(violations, checkedClaimViolations(
			phase4Checklist,
			"/api/v1/tickets/{id}/cancel",
			"ticket cancel endpoint is checked as done, but OpenAPI/handler implementation is missing",
		)...)
		violations = append(violations, checkedClaimViolations(
			phase4Checklist,
			"Approval API",
			"approval API completeness is checked as done, but cancel endpoint is not implemented",
		)...)
	}

	usersIsPlaceholder, err := usersPageIsPlaceholder(usersPage)
	if err != nil {
		fmt.Printf("FAIL: inspect users page: %v\n", err)
		os.Exit(1)
	}
	if usersIsPlaceholder {
		violations = append(violations,
			fmt.Sprintf("%s: users page still contains placeholder marker", usersPage))
		violations = append(violations, checkedClaimViolations(
			phase5Checklist,
			"Pages feature-complete",
			"frontend pages are checked as feature-complete, but users page is placeholder-only",
		)...)
		if row, line, ok, err := rowClaimedDone(phase5DesignDoc, "Frontend: Users"); err != nil {
			fmt.Printf("FAIL: inspect %s for Frontend: Users claim: %v\n", phase5DesignDoc, err)
			os.Exit(1)
		} else if ok {
			violations = append(violations,
				fmt.Sprintf("%s:%d: Frontend: Users is marked done, but users page is placeholder-only (%s)", phase5DesignDoc, line, row))
		}
	}

	hasEnvFilterEvidence, err := hasAllowedEnvironmentFilteringEvidence()
	if err != nil {
		fmt.Printf("FAIL: inspect environment-filter evidence: %v\n", err)
		os.Exit(1)
	}
	if !hasEnvFilterEvidence {
		violations = append(violations, checkedClaimViolations(
			phase4Checklist,
			"Environment-based query filtering",
			"environment-based query filtering is checked as done, but runtime filtering evidence is missing",
		)...)
		violations = append(violations, checkedClaimViolations(
			phase4Checklist,
			"Visibility Filtering",
			"visibility filtering is checked as done, but runtime allowed_environments filtering is missing",
		)...)
	}

	hasAnyVNCPath, err := openAPIHasPathContaining("/vnc")
	if err != nil {
		fmt.Printf("FAIL: inspect OpenAPI for vnc paths: %v\n", err)
		os.Exit(1)
	}
	if !hasAnyVNCPath {
		if row, line, ok, err := rowClaimedDone(phase4Governance, "§18 VNC Permissions"); err != nil {
			fmt.Printf("FAIL: inspect %s for VNC claim: %v\n", phase4Governance, err)
			os.Exit(1)
		} else if ok {
			violations = append(violations,
				fmt.Sprintf("%s:%d: §18 VNC Permissions is marked done, but OpenAPI has no /vnc path (%s)", phase4Governance, line, row))
		}
	}

	hasAnyBatchPath, err := openAPIHasPathContaining("/batch")
	if err != nil {
		fmt.Printf("FAIL: inspect OpenAPI for batch paths: %v\n", err)
		os.Exit(1)
	}
	if !hasAnyBatchPath {
		if row, line, ok, err := rowClaimedDone(phase4Governance, "§19 Batch Operations"); err != nil {
			fmt.Printf("FAIL: inspect %s for Batch claim: %v\n", phase4Governance, err)
			os.Exit(1)
		} else if ok {
			violations = append(violations,
				fmt.Sprintf("%s:%d: §19 Batch Operations is marked done, but OpenAPI has no /batch path (%s)", phase4Governance, line, row))
		}
	}

	hasBootstrapSecretsEvidence, err := hasBootstrapSecretPersistenceEvidence()
	if err != nil {
		fmt.Printf("FAIL: inspect bootstrap secret persistence evidence: %v\n", err)
		os.Exit(1)
	}
	if !hasBootstrapSecretsEvidence {
		violations = append(violations,
			fmt.Sprintf("%s: accepted ADR still claims persisted bootstrap secrets, but runtime DB-backed bootstrap evidence is missing", adr0025))
	}

	if len(violations) > 0 {
		fmt.Println("FAIL: doc claims consistency check failed")
		for _, v := range violations {
			fmt.Printf(" - %s\n", v)
		}
		fmt.Println("Rule: checklist done-state must have matching implementation evidence (code + contract).")
		os.Exit(1)
	}

	fmt.Println("OK: doc claims consistency check passed")
}

func checkedClaimViolations(filePath, contains, msg string) []string {
	f, err := os.Open(filePath)
	if err != nil {
		return []string{fmt.Sprintf("%s: cannot open file: %v", filePath, err)}
	}
	defer f.Close()

	var out []string
	s := bufio.NewScanner(f)
	lineNo := 0
	for s.Scan() {
		lineNo++
		line := s.Text()
		if !strings.Contains(line, contains) {
			continue
		}
		if strings.Contains(line, "- [x]") {
			out = append(out, fmt.Sprintf("%s:%d: %s", filePath, lineNo, msg))
		}
	}
	if err := s.Err(); err != nil {
		out = append(out, fmt.Sprintf("%s: read error: %v", filePath, err))
	}
	return out
}

func findForbiddenDocClaims(targets map[string][]string) []string {
	var violations []string
	for path, forbidden := range targets {
		b, err := os.ReadFile(path)
		if err != nil {
			violations = append(violations, fmt.Sprintf("%s: cannot read file: %v", path, err))
			continue
		}
		content := string(b)
		for _, marker := range forbidden {
			if strings.Contains(content, marker) {
				violations = append(violations,
					fmt.Sprintf("%s: forbidden stale claim still present: %s", path, marker))
			}
		}
	}
	return violations
}

func openAPIHasPath(target string) (bool, error) {
	f, err := os.Open(openAPIFile)
	if err != nil {
		return false, err
	}
	defer f.Close()

	s := bufio.NewScanner(f)
	for s.Scan() {
		m := pathRe.FindStringSubmatch(s.Text())
		if len(m) != 2 {
			continue
		}
		if strings.TrimSpace(m[1]) == target {
			return true, nil
		}
	}
	if err := s.Err(); err != nil {
		return false, err
	}
	return false, nil
}

func openAPIHasPathContaining(part string) (bool, error) {
	f, err := os.Open(openAPIFile)
	if err != nil {
		return false, err
	}
	defer f.Close()

	s := bufio.NewScanner(f)
	for s.Scan() {
		m := pathRe.FindStringSubmatch(s.Text())
		if len(m) != 2 {
			continue
		}
		if strings.Contains(strings.TrimSpace(m[1]), part) {
			return true, nil
		}
	}
	if err := s.Err(); err != nil {
		return false, err
	}
	return false, nil
}

func rowClaimedDone(path, marker string) (row string, line int, claimedDone bool, err error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, false, err
	}
	defer f.Close()

	s := bufio.NewScanner(f)
	lineNo := 0
	for s.Scan() {
		lineNo++
		text := s.Text()
		if !strings.Contains(text, marker) {
			continue
		}
		return text, lineNo, strings.Contains(text, "✅"), nil
	}
	if err := s.Err(); err != nil {
		return "", 0, false, err
	}
	return "", 0, false, nil
}

func serverHasMethod(path, method string) (bool, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	sig := "func (s *Server) " + method + "("
	return strings.Contains(string(b), sig), nil
}

func usersPageIsPlaceholder(path string) (bool, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	text := strings.ToLower(string(b))
	return strings.Contains(text, "placeholder"), nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func hasAllowedEnvironmentFilteringEvidence() (bool, error) {
	files := []string{rbacMiddleware, vmHandler, namespaceHandler, envVisibility}
	fragments := []string{"AllowedEnvironments", "allowed_environments", "resolveNamespaceVisibility("}

	for _, path := range files {
		b, err := os.ReadFile(path)
		if err != nil {
			return false, err
		}
		text := string(b)
		for _, frag := range fragments {
			if strings.Contains(text, frag) {
				return true, nil
			}
		}
	}
	return false, nil
}

func hasBootstrapSecretPersistenceEvidence() (bool, error) {
	resolverBytes, err := os.ReadFile(secretBootstrapResolver)
	if err != nil {
		return false, err
	}
	modulesBytes, err := os.ReadFile(modulesInfrastructure)
	if err != nil {
		return false, err
	}

	resolverText := string(resolverBytes)
	modulesText := string(modulesBytes)
	return strings.Contains(resolverText, "ResolveBootstrapSecuritySecrets(") &&
		strings.Contains(resolverText, "system_secrets") &&
		strings.Contains(modulesText, "ValidateResolvedSecuritySecrets("), nil
}
