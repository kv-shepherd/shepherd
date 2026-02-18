//go:build ignore

package main

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	openAPIPath         = "api/openapi.yaml"
	handlerPath         = "internal/api/handlers/server_vm_batch.go"
	adminHandlerPath    = "internal/api/handlers/server_admin_rate_limit.go"
	gatewayPath         = "internal/governance/approval/gateway.go"
	jobHelperPath       = "internal/jobs/helpers.go"
	schemaPath          = "ent/schema/batch_approval_ticket.go"
	exemptionSchemaPath = "ent/schema/rate_limit_exemption.go"
	overrideSchemaPath  = "ent/schema/rate_limit_user_override.go"
	frontendVMPagePath  = "web/src/app/(protected)/vms/page.tsx"
	frontendVMHookPath  = "web/src/features/vm-management/hooks/useVMManagementController.ts"
	frontendVMHookTests = "web/src/features/vm-management/hooks/useVMManagementController.test.tsx"
	allowlistPath       = "docs/design/ci/allowlists/master_flow_api_deferred.txt"
)

func main() {
	var violations []string

	checkOpenAPI(&violations)
	checkSchemaFragments(&violations)
	checkHandlerFragments(&violations)
	checkAdminRateLimitHandlerFragments(&violations)
	checkGatewayFragments(&violations)
	checkJobHelperFragments(&violations)
	checkFrontendFragments(&violations)
	checkAllowlist(&violations)

	if len(violations) > 0 {
		fmt.Println("FAIL: Stage 5.E batch baseline check failed")
		for _, v := range violations {
			fmt.Println(" -", v)
		}
		fmt.Println("Rule: canonical batch endpoints and runtime baseline must stay implemented once introduced.")
		os.Exit(1)
	}

	fmt.Println("OK: Stage 5.E batch baseline check passed")
}

func checkOpenAPI(violations *[]string) {
	specBytes, err := os.ReadFile(openAPIPath)
	if err != nil {
		*violations = append(*violations, fmt.Sprintf("read %s: %v", openAPIPath, err))
		return
	}

	var doc yaml.Node
	if err := yaml.Unmarshal(specBytes, &doc); err != nil {
		*violations = append(*violations, fmt.Sprintf("parse %s: %v", openAPIPath, err))
		return
	}

	root := documentRoot(&doc)
	paths, ok := mapValue(root, "paths")
	if !ok {
		*violations = append(*violations, "missing root.paths")
		return
	}

	required := []struct {
		path string
		op   string
		id   string
	}{
		{path: "/vms/batch", op: "post", id: "submitVMBatch"},
		{path: "/vms/batch/{batch_id}", op: "get", id: "getVMBatch"},
		{path: "/vms/batch/{batch_id}/retry", op: "post", id: "retryVMBatch"},
		{path: "/vms/batch/{batch_id}/cancel", op: "post", id: "cancelVMBatch"},
		{path: "/vms/batch/power", op: "post", id: "submitVMBatchPower"},
		{path: "/approvals/batch", op: "post", id: "submitApprovalBatch"},
		{path: "/admin/rate-limits/exemptions", op: "post", id: "createRateLimitExemption"},
		{path: "/admin/rate-limits/exemptions/{user_id}", op: "delete", id: "deleteRateLimitExemption"},
		{path: "/admin/rate-limits/users/{user_id}", op: "put", id: "updateRateLimitUserOverrides"},
		{path: "/admin/rate-limits/status", op: "get", id: "listRateLimitStatus"},
	}
	for _, r := range required {
		p, ok := mapValue(paths, r.path)
		if !ok {
			*violations = append(*violations, fmt.Sprintf("missing OpenAPI path %s", r.path))
			continue
		}
		opNode, ok := mapValue(p, r.op)
		if !ok {
			*violations = append(*violations, fmt.Sprintf("missing OpenAPI operation %s.%s", r.path, r.op))
			continue
		}
		opID, ok := scalarValueByKey(opNode, "operationId")
		if !ok || opID != r.id {
			*violations = append(*violations, fmt.Sprintf("%s.%s operationId must be %s", r.path, r.op, r.id))
		}
	}
}

func checkHandlerFragments(violations *[]string) {
	content, err := os.ReadFile(handlerPath)
	if err != nil {
		*violations = append(*violations, fmt.Sprintf("read %s: %v", handlerPath, err))
		return
	}

	needles := []string{
		"func (s *Server) SubmitVMBatch(",
		"func (s *Server) SubmitApprovalBatch(",
		"func (s *Server) SubmitVMBatchPower(",
		"func (s *Server) GetVMBatch(",
		"func (s *Server) RetryVMBatch(",
		"func (s *Server) CancelVMBatch(",
		"submitBatchPower(",
		"prepareBatchPowerChildren(",
		"enqueueBatchPowerJob(",
		"findBatchByRequestID(",
		"pendingBatchParentCounters(",
		"evaluateAdditionalBatchSubmissionLimits(",
		"maxPendingBatchParents",
		"maxPendingBatchParentsUser",
		"maxPendingBatchChildrenUser",
		"maxGlobalBatchRequestsPerMinute",
		"batchSubmitCooldown",
		"tx.BatchApprovalTicket.Create(",
		"s.client.BatchApprovalTicket.UpdateOneID(",
	}
	text := string(content)
	for _, n := range needles {
		if !strings.Contains(text, n) {
			*violations = append(*violations, fmt.Sprintf("%s missing fragment %q", handlerPath, n))
		}
	}
}

func checkSchemaFragments(violations *[]string) {
	content, err := os.ReadFile(schemaPath)
	if err != nil {
		*violations = append(*violations, fmt.Sprintf("read %s: %v", schemaPath, err))
		return
	}

	needles := []string{
		"type BatchApprovalTicket struct",
		`field.Enum("batch_type")`,
		`field.Int("child_count")`,
		`field.Int("success_count")`,
		`field.Int("failed_count")`,
		`field.Int("pending_count")`,
		`field.Enum("status")`,
	}
	text := string(content)
	for _, n := range needles {
		if !strings.Contains(text, n) {
			*violations = append(*violations, fmt.Sprintf("%s missing fragment %q", schemaPath, n))
		}
	}

	exemptionContent, err := os.ReadFile(exemptionSchemaPath)
	if err != nil {
		*violations = append(*violations, fmt.Sprintf("read %s: %v", exemptionSchemaPath, err))
		return
	}
	exemptionNeedles := []string{
		"type RateLimitExemption struct",
		`field.String("exempted_by")`,
		`field.Time("expires_at")`,
	}
	exemptionText := string(exemptionContent)
	for _, n := range exemptionNeedles {
		if !strings.Contains(exemptionText, n) {
			*violations = append(*violations, fmt.Sprintf("%s missing fragment %q", exemptionSchemaPath, n))
		}
	}

	overrideContent, err := os.ReadFile(overrideSchemaPath)
	if err != nil {
		*violations = append(*violations, fmt.Sprintf("read %s: %v", overrideSchemaPath, err))
		return
	}
	overrideNeedles := []string{
		"type RateLimitUserOverride struct",
		`field.Int("max_pending_parents")`,
		`field.Int("max_pending_children")`,
		`field.Int("cooldown_seconds")`,
		`field.String("updated_by")`,
	}
	overrideText := string(overrideContent)
	for _, n := range overrideNeedles {
		if !strings.Contains(overrideText, n) {
			*violations = append(*violations, fmt.Sprintf("%s missing fragment %q", overrideSchemaPath, n))
		}
	}
}

func checkGatewayFragments(violations *[]string) {
	content, err := os.ReadFile(gatewayPath)
	if err != nil {
		*violations = append(*violations, fmt.Sprintf("read %s: %v", gatewayPath, err))
		return
	}

	needles := []string{
		"approveBatchParent(",
		"isBatchParentTicket(",
		"markChildApprovalDispatchFailed(",
		"g.approveCreate(ctx, child",
		"g.approveDelete(ctx, child",
	}
	text := string(content)
	for _, n := range needles {
		if !strings.Contains(text, n) {
			*violations = append(*violations, fmt.Sprintf("%s missing fragment %q", gatewayPath, n))
		}
	}
}

func checkAdminRateLimitHandlerFragments(violations *[]string) {
	content, err := os.ReadFile(adminHandlerPath)
	if err != nil {
		*violations = append(*violations, fmt.Sprintf("read %s: %v", adminHandlerPath, err))
		return
	}

	needles := []string{
		"func (s *Server) CreateRateLimitExemption(",
		"func (s *Server) DeleteRateLimitExemption(",
		"func (s *Server) UpdateRateLimitUserOverrides(",
		"func (s *Server) ListRateLimitStatus(",
		`requireActorWithAnyGlobalPermission(c, "rate_limit:manage")`,
		"resolveBatchUserLimitPolicy(",
	}
	text := string(content)
	for _, n := range needles {
		if !strings.Contains(text, n) {
			*violations = append(*violations, fmt.Sprintf("%s missing fragment %q", adminHandlerPath, n))
		}
	}
}

func checkJobHelperFragments(violations *[]string) {
	content, err := os.ReadFile(jobHelperPath)
	if err != nil {
		*violations = append(*violations, fmt.Sprintf("read %s: %v", jobHelperPath, err))
		return
	}

	needles := []string{
		"syncParentBatchStatusByChildEvent(",
		"syncParentBatchStatus(",
		"parentStatus := approvalticket.StatusEXECUTING",
	}
	text := string(content)
	for _, n := range needles {
		if !strings.Contains(text, n) {
			*violations = append(*violations, fmt.Sprintf("%s missing fragment %q", jobHelperPath, n))
		}
	}
}

func checkAllowlist(violations *[]string) {
	content, err := os.ReadFile(allowlistPath)
	if err != nil {
		*violations = append(*violations, fmt.Sprintf("read %s: %v", allowlistPath, err))
		return
	}
	lines := parseAllowlistLines(string(content))

	blocked := []string{
		"/approvals/batch",
		"/vms/batch",
		"/vms/batch/power",
		"/vms/batch/{}",
		"/vms/batch/{}/retry",
		"/vms/batch/{}/cancel",
		"/admin/rate-limits/exemptions",
		"/admin/rate-limits/exemptions/{}",
		"/admin/rate-limits/users/{}",
		"/admin/rate-limits/status",
	}
	for _, b := range blocked {
		if _, ok := lines[b]; ok {
			*violations = append(*violations, fmt.Sprintf("allowlist must not contain implemented path %s", b))
		}
	}
}

func checkFrontendFragments(violations *[]string) {
	checkFileContains(violations, frontendVMPagePath, []string{
		`data-testid="batch-status-live"`,
		`aria-live="polite"`,
		`batch.rate_limited_wait`,
		`lastBatchActionFeedback`,
	})
	checkFileContains(violations, frontendVMHookPath, []string{
		"parseBatchIDFromStatusURL(",
		"extractRetryAfterSeconds(",
		"batchRateLimited",
		"batchRetryAfterSeconds",
		"pickBatchActionTargets(",
		"lastBatchActionFeedback",
	})
	checkFileContains(violations, frontendVMHookTests, []string{
		"uses status_url for active batch tracking when batch submit succeeds",
		"enters cooldown on BATCH_RATE_LIMITED and blocks batch actions while countdown active",
		"records affected child ticket ids for retry/cancel feedback",
	})
}

func checkFileContains(violations *[]string, path string, needles []string) {
	content, err := os.ReadFile(path)
	if err != nil {
		*violations = append(*violations, fmt.Sprintf("read %s: %v", path, err))
		return
	}
	text := string(content)
	for _, n := range needles {
		if !strings.Contains(text, n) {
			*violations = append(*violations, fmt.Sprintf("%s missing fragment %q", path, n))
		}
	}
}

func parseAllowlistLines(content string) map[string]struct{} {
	out := make(map[string]struct{})
	for _, raw := range strings.Split(content, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if i := strings.Index(line, "#"); i >= 0 {
			line = strings.TrimSpace(line[:i])
		}
		if line == "" {
			continue
		}
		out[line] = struct{}{}
	}
	return out
}

func documentRoot(n *yaml.Node) *yaml.Node {
	if n == nil {
		return nil
	}
	if n.Kind == yaml.DocumentNode && len(n.Content) > 0 {
		return n.Content[0]
	}
	return n
}

func mapValue(node *yaml.Node, key string) (*yaml.Node, bool) {
	node = documentRoot(node)
	if node == nil || node.Kind != yaml.MappingNode {
		return nil, false
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1], true
		}
	}
	return nil, false
}

func scalarValueByKey(node *yaml.Node, key string) (string, bool) {
	v, ok := mapValue(node, key)
	if !ok || v.Kind != yaml.ScalarNode {
		return "", false
	}
	return strings.TrimSpace(v.Value), true
}
