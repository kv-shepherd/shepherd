//go:build ignore

package main

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	openAPIPath              = "api/openapi.yaml"
	handlerPath              = "internal/api/handlers/server_vm_batch.go"
	batchIdempotencyTestPath = "internal/api/handlers/server_vm_batch_idempotency_test.go"
	vmHandlerPath            = "internal/api/handlers/server_vm.go"
	batchPowerAtomicPath     = "internal/usecase/batch_power_atomic.go"
	batchApprovalAtomicPath  = "internal/usecase/batch_approval_dispatch_atomic.go"
	batchApprovalJobPath     = "internal/jobs/batch_approval_dispatch.go"
	vmPowerJobPath           = "internal/jobs/vm_power.go"
	vmModulePath             = "internal/app/modules/vm.go"
	batchReplayKeyPath       = "internal/repository/batchreplay/key.go"
	adminHandlerPath         = "internal/api/handlers/server_admin_rate_limit.go"
	gatewayPath              = "internal/governance/ticketing/service.go"
	jobHelperPath            = "internal/jobs/helpers.go"
	jobHelperPredicatesPath  = "internal/jobs/helpers_ent_predicates.go"
	ticketQueriesPath        = "internal/repository/sqlc/queries/ticket.sql"
	databasePath             = "internal/infrastructure/database.go"
	schemaPath               = "ent/schema/batch_ticket.go"
	exemptionSchemaPath      = "ent/schema/rate_limit_exemption.go"
	overrideSchemaPath       = "ent/schema/rate_limit_user_override.go"
	migrationDocPath         = "docs/design/database/migrations.md"
	traceabilityPath         = "docs/design/traceability/master-flow.json"
	governanceDocPath        = "docs/design/phases/04-governance.md"
	masterFlowPath           = "docs/design/interaction-flows/master-flow.md"
	chineseMasterFlowPath    = "docs/i18n/zh-CN/design/interaction-flows/master-flow.md"
	lifecycleDocPath         = "docs/design/database/vm-lifecycle-write-model.md"
	lifecycleRetentionPath   = "docs/design/database/lifecycle-retention.md"
	schemaCatalogPath        = "docs/design/database/schema-catalog.md"
	phase4ChecklistPath      = "docs/design/checklist/phase-4-checklist.md"
	frontendQueueDocPath     = "docs/design/frontend/features/batch-operations-queue.md"
	frontendVMPagePath       = "web/src/app/(protected)/vms/VMsPageContent.tsx"
	frontendVMHookPath       = "web/src/features/vm-management/hooks/useVMManagementController.ts"
	frontendVMHookTests      = "web/src/features/vm-management/hooks/useVMManagementController.test.tsx"
	frontendBatchActions     = "web/src/features/vm-management/batchActions.ts"
	frontendBatchActionTest  = "web/src/features/vm-management/batchActions.test.ts"
	frontendBatchIntentStore = "web/src/features/vm-management/batchRequestIntentStorage.ts"
	frontendBatchIntentTest  = "web/src/features/vm-management/batchRequestIntentStorage.test.ts"
	frontendCooldownHook     = "web/src/features/vm-management/hooks/useBatchActionCooldown.ts"
	frontendCooldownTest     = "web/src/features/vm-management/hooks/useBatchActionCooldown.test.tsx"
	frontendAPIQueryHook     = "web/src/hooks/useApiQuery.ts"
	frontendAPIQueryTest     = "web/src/hooks/useApiQuery.test.tsx"
	frontendBatchList        = "web/src/app/(protected)/vms/batch/VMBatchListPageContent.tsx"
	frontendBatchListTest    = "web/src/app/(protected)/vms/batch/VMBatchListPageContent.test.tsx"
	frontendBatchDetail      = "web/src/app/(protected)/vms/batch/[id]/VMBatchDetailPageContent.tsx"
	frontendBatchDetailTest  = "web/src/app/(protected)/vms/batch/[id]/VMBatchDetailPageContent.test.tsx"
	allowlistPath            = "docs/design/ci/allowlists/master_flow_api_deferred.txt"
)

func main() {
	var violations []string

	checkOpenAPI(&violations)
	checkSchemaFragments(&violations)
	checkHandlerFragments(&violations)
	checkBatchPowerAtomicFragments(&violations)
	checkBatchApprovalAtomicFragments(&violations)
	checkBatchApprovalDispatchFragments(&violations)
	checkBatchReplayLookupFragments(&violations)
	checkBatchRetrySQLFragments(&violations)
	checkRestartFenceFragments(&violations)
	checkAdminRateLimitHandlerFragments(&violations)
	checkGatewayFragments(&violations)
	checkJobHelperFragments(&violations)
	checkBatchRolloutFragments(&violations)
	checkBatchTraceabilityFragments(&violations)
	checkBatchDocumentationFragments(&violations)
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

func checkBatchReplayLookupFragments(violations *[]string) {
	checkFileContains(violations, batchReplayKeyPath, []string{
		"CandidateLimit bounds historical-duplicate graph loads for one exact",
		"hash collisions are excluded before this limit is",
		"counted and cannot authorize replay",
	})
	checkFileContains(violations, batchPowerAtomicPath, []string{
		`AND "shepherd_batch_replay_sha256"(BTRIM(batch.request_id,`,
		`AND BTRIM(batch.request_id,`,
		"batchreplay.CandidateLimit+1",
	})
	checkFileContains(violations, migrationDocPath, []string{
		"Hash collisions cannot authorize replay",
		"exact predicate excludes them before",
		"The 65th exact-key row is only an",
		"more than 64 historical duplicates is an integrity error",
	})

	content, err := os.ReadFile(migrationDocPath)
	if err == nil && strings.Contains(string(content), "digest collision is also an integrity error") {
		*violations = append(*violations, migrationDocPath+" must not classify a hash collision as a replay integrity error after exact SQL filtering")
	}
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
	if _, ok := mapValue(paths, "/admin/vm-power-events/{event_id}/reconcile"); ok {
		*violations = append(*violations, "unsafe ambiguous-restart fence release API must not be exposed")
	}

	components, ok := mapValue(root, "components")
	if !ok {
		*violations = append(*violations, "missing root.components")
		return
	}
	schemas, ok := mapValue(components, "schemas")
	if !ok {
		*violations = append(*violations, "missing root.components.schemas")
		return
	}
	checkStringEnumSchema(violations, schemas, "VMBatchParentStatus", []string{
		"PENDING_APPROVAL",
		"IN_PROGRESS",
		"COMPLETED",
		"PARTIAL_SUCCESS",
		"FAILED",
		"CANCELLED",
	})
	child, ok := mapValue(schemas, "VMBatchChildStatus")
	if !ok {
		*violations = append(*violations, "missing OpenAPI schema VMBatchChildStatus")
		return
	}
	properties, ok := mapValue(child, "properties")
	if !ok {
		*violations = append(*violations, "VMBatchChildStatus missing properties")
		return
	}
	status, ok := mapValue(properties, "status")
	if !ok {
		*violations = append(*violations, "VMBatchChildStatus missing status property")
		return
	}
	checkStringEnumNode(violations, "VMBatchChildStatus.status", status, []string{
		"PENDING",
		"APPROVED",
		"REJECTED",
		"CANCELLED",
		"EXECUTING",
		"SUCCESS",
		"FAILED",
	})
	submitResponse, ok := mapValue(schemas, "VMBatchSubmitResponse")
	if !ok {
		*violations = append(*violations, "missing OpenAPI schema VMBatchSubmitResponse")
		return
	}
	submitProperties, ok := mapValue(submitResponse, "properties")
	if !ok {
		*violations = append(*violations, "VMBatchSubmitResponse missing properties")
		return
	}
	retryAfter, ok := mapValue(submitProperties, "retry_after_seconds")
	if !ok {
		*violations = append(*violations, "VMBatchSubmitResponse missing retry_after_seconds property")
		return
	}
	retryAfterExample, ok := scalarValueByKey(retryAfter, "example")
	if !ok || retryAfterExample != "2" {
		*violations = append(*violations, "VMBatchSubmitResponse.retry_after_seconds example must match runtime value 2")
	}
}

func checkHandlerFragments(violations *[]string) {
	content, err := os.ReadFile(handlerPath)
	if err != nil {
		*violations = append(*violations, fmt.Sprintf("read %s: %v", handlerPath, err))
		return
	}

	needles := []string{
		`batchResourceType               = "batch"`,
		"func (s *Server) SubmitVMBatch(",
		"func (s *Server) SubmitVMBatchPower(",
		"func (s *Server) GetVMBatch(",
		"func (s *Server) RetryVMBatch(",
		"func (s *Server) CancelVMBatch(",
		"submitBatchPower(",
		"prepareBatchPowerChildren(",
		"CreateBatchPowerAndMaybeEnqueue(",
		"RetryBatchPowerAndEnqueue(",
		"batchPowerChildInputs(",
		"findBatchByRequestIDWithClient(",
		"writeEarlyBatchReplayResponse(",
		"lockBatchSubmissionTransaction(",
		"BatchPowerSubmissionTxPolicy{",
		"evaluateBatchSubmissionRateLimits(",
		"entBatchSubmissionLimitReader{",
		"pgxBatchSubmissionLimitReader{",
		"maxPendingBatchParents",
		"maxPendingBatchParentsUser",
		"maxPendingBatchChildrenUser",
		"maxGlobalBatchRequestsPerMinute",
		"batchSubmitCooldown",
		"batchRetryAfterSeconds          = 2",
		"if child.Status == entticket.StatusFAILED {",
		"child.AttemptCount >= domain.BatchChildMaxAttempts",
		"[]entticket.Status{entticket.StatusPENDING}",
		"[]domainevent.Status{domainevent.StatusPENDING}",
		"usecase.BatchMutationLockKey(parentTicketID)",
		"jobs.SyncParentBatchStatusInTx(ctx, tx, parentTicketID)",
		`_ = s.audit.LogAction(ctx, "vm.batch."+action`,
		"tx.BatchTicket.Create(",
		"projection, err := tx.BatchTicket.Get(ctx, parentTicketID)",
	}
	text := string(content)
	for _, n := range needles {
		if !strings.Contains(text, n) {
			*violations = append(*violations, fmt.Sprintf("%s missing fragment %q", handlerPath, n))
		}
	}
	if strings.Contains(text, "child.Status == entticket.StatusFAILED || child.Status == entticket.StatusREJECTED") {
		*violations = append(*violations, fmt.Sprintf("%s must not make approval-REJECTED children execution-retryable", handlerPath))
	}
	ordinarySubmit := textSection(text, "func (s *Server) submitBatch(c *gin.Context) {", "func (s *Server) submitBatchPower(c *gin.Context) {")
	checkOrderedFragments(violations, handlerPath+" ordinary early replay", ordinarySubmit, []string{
		"requestID := normalizeBatchRequestID(req.RequestId)",
		"writeEarlyBatchReplayResponse(c, actor, op, requestID, batchResourceType)",
		"resolveNamespaceVisibility(c)",
		"prepareBatchChildren(",
	})
	powerSubmit := textSection(text, "func (s *Server) submitBatchPower(c *gin.Context) {", "func batchPowerChildInputs(")
	checkOrderedFragments(violations, handlerPath+" power early replay", powerSubmit, []string{
		"requestID := normalizeBatchRequestID(req.RequestId)",
		`writeEarlyBatchReplayResponse(c, actor, opKey, requestID, "power-batch")`,
		"resolveNamespaceVisibility(c)",
		"prepareBatchPowerChildren(",
	})
	checkFileContains(violations, batchIdempotencyTestPath, []string{
		"TestBatchHandler_SubmitVMBatch_ReplayPrecedesMutableTargetPreparation",
		"TestBatchHandler_SubmitVMBatchPower_ReplayPrecedesMutableTargetPreparation",
	})
	mutate := textSection(
		text,
		"func (s *Server) mutateBatchChildren(",
		"type batchChildTicketEventRef struct {",
	)
	targetSelection := textSection(mutate, "for _, child := range children {", "if len(targetIDs) == 0")
	checkSectionContains(violations, handlerPath+" mutateBatchChildren target selection", targetSelection, []string{
		"case batchActionRetry:",
		"if child.Status == entticket.StatusFAILED {",
		"child.AttemptCount >= domain.BatchChildMaxAttempts",
		"case batchActionCancel:",
		"if child.Status == entticket.StatusPENDING {",
	})
	if strings.Contains(targetSelection, "StatusREJECTED") {
		*violations = append(*violations, fmt.Sprintf("%s retry target selection must not contain REJECTED", handlerPath))
	}
	if strings.Contains(text, "markBatchPowerRetryChildFailedOrAbort") ||
		strings.Contains(text, "markBatchPowerRetryChildrenFailed") ||
		strings.Contains(text, "markBatchChildrenFailed") {
		*violations = append(*violations, fmt.Sprintf("%s must not mutate malformed power-retry children before the atomic writer", handlerPath))
	}
	cancelTx := textSection(
		text,
		"func (s *Server) updateBatchChildTicketAndEventStatus(",
		"func ticketStatusAllowed(",
	)
	checkOrderedFragments(violations, handlerPath+" updateBatchChildTicketAndEventStatus", cancelTx, []string{
		"usecase.BatchMutationLockKey(parentTicketID)",
		"ticketUpdate := tx.Ticket.Update()",
		"eventUpdate := tx.DomainEvent.Update()",
		"jobs.SyncParentBatchStatusInTx(ctx, tx, parentTicketID)",
	})
}

func checkBatchPowerAtomicFragments(violations *[]string) {
	checkFileContains(violations, batchPowerAtomicPath, []string{
		"BatchSubmissionAdvisoryLockKey",
		"func (w *ApprovalAtomicWriter) CreateBatchPowerAndMaybeEnqueue(",
		"func (w *ApprovalAtomicWriter) RetryBatchPowerAndEnqueue(",
		"pgx.ReadCommitted",
		"InsertBatchTicket(",
		"InsertTx(ctx, tx, jobs.VMPowerArgs",
		"ResetPowerRetryTicket(",
		"ResetBatchPowerRetryEvent(",
	})
}

func checkBatchApprovalAtomicFragments(violations *[]string) {
	checkFileContains(violations, batchApprovalAtomicPath, []string{
		"func (w *ApprovalAtomicWriter) ClaimBatchApprovalAndEnqueue(",
		"func (w *ApprovalAtomicWriter) RetryBatchApprovalAndEnqueue(",
		"pgx.ReadCommitted",
		"BatchMutationLockKey(input.ParentTicketID)",
		"qtx.ResetBatchApprovalRetryChild(",
		"MaxAttempts:    int32(domain.BatchChildMaxAttempts)",
		"qtx.ReopenBatchApprovalDispatch(",
		"qtx.SetBatchApprovalEventProcessing(",
		"refreshBatchApprovalProjection(",
		"func (w *ApprovalAtomicWriter) insertBatchApprovalDispatcher(",
		"w.riverClient.InsertTx(ctx, tx, jobs.BatchApprovalDispatchArgs{",
		"BatchApprovalDispatchConflictError",
		"BatchRetryParentNotEligibleError",
	})

	content, err := os.ReadFile(batchApprovalAtomicPath)
	if err != nil {
		return
	}
	text := string(content)
	claim := textSection(
		text,
		"func (w *ApprovalAtomicWriter) ClaimBatchApprovalAndEnqueue(",
		"func (w *ApprovalAtomicWriter) RetryBatchApprovalAndEnqueue(",
	)
	checkOrderedFragments(violations, batchApprovalAtomicPath+" ClaimBatchApprovalAndEnqueue", claim, []string{
		"qtx.ClaimBatchApprovalDispatch(",
		"qtx.ClaimBatchApprovalEventProcessing(",
		"refreshBatchApprovalProjection(",
		"w.insertBatchApprovalDispatcher(ctx, tx, input.ParentTicketID)",
		"tx.Commit(ctx)",
	})
	retry := textSection(
		text,
		"func (w *ApprovalAtomicWriter) RetryBatchApprovalAndEnqueue(",
		"func (w *ApprovalAtomicWriter) insertBatchApprovalDispatcher(",
	)
	checkOrderedFragments(violations, batchApprovalAtomicPath+" RetryBatchApprovalAndEnqueue", retry, []string{
		"BatchMutationLockKey(input.ParentTicketID)",
		"w.insertBatchApprovalDispatcher(ctx, tx, input.ParentTicketID)",
		"qtx.ResetBatchApprovalRetryChild(",
		"qtx.ReopenBatchApprovalDispatch(",
		"qtx.SetBatchApprovalEventProcessing(",
		"refreshBatchApprovalProjection(",
		"tx.Commit(ctx)",
	})
}

func checkBatchApprovalDispatchFragments(violations *[]string) {
	checkFileContains(violations, batchApprovalJobPath, []string{
		`const BatchApprovalDispatchJobKind = "batch_approval_dispatch"`,
		"Queue:       BatchApprovalDispatchJobKind",
		"ByArgs:  true",
		"ByQueue: true",
		"rivertype.JobStateAvailable",
		"rivertype.JobStatePending",
		"rivertype.JobStateRetryable",
		"rivertype.JobStateRunning",
		"rivertype.JobStateScheduled",
		"DispatchBatchApproval(context.Context, string) error",
		"FailPendingBatchApprovalDispatch(context.Context, string, error) error",
		"BatchApprovalDispatchConsistencyError",
		"river.JobCancel(",
		"river.JobSnooze(",
	})
	checkFileContains(violations, databasePath, []string{
		"jobs.BatchApprovalDispatchJobKind: {MaxWorkers: maxWorkers}",
	})
}

func checkBatchRetrySQLFragments(violations *[]string) {
	content, err := os.ReadFile(ticketQueriesPath)
	if err != nil {
		*violations = append(*violations, fmt.Sprintf("read %s: %v", ticketQueriesPath, err))
		return
	}
	text := string(content)

	genericRetry := textSection(text, "-- name: ResetBatchApprovalRetryChild", "-- name: ResetBatchApprovalRetryEvent")
	checkSectionContains(violations, ticketQueriesPath+" ResetBatchApprovalRetryChild", genericRetry, []string{
		"AND child.status = 'FAILED'",
		"AND child.operation_type <> 'POWER'",
		"AND child.attempt_count < sqlc.arg(max_attempts)",
		"AND event.aggregate_type = 'vm'",
		"AND event.status IN ('PENDING', 'FAILED', 'CANCELLED')",
	})
	if strings.Contains(genericRetry, "REJECTED") {
		*violations = append(*violations, fmt.Sprintf("%s ResetBatchApprovalRetryChild must not accept REJECTED", ticketQueriesPath))
	}

	genericEvent := textSection(text, "-- name: ResetBatchApprovalRetryEvent", "-- name: RefreshBatchApprovalProjectionForDispatch")
	checkSectionContains(violations, ticketQueriesPath+" ResetBatchApprovalRetryEvent", genericEvent, []string{
		"child.parent_ticket_id = sqlc.arg(parent_ticket_id)",
		"child.status = 'PENDING'",
		"child.operation_type <> 'POWER'",
		"event.aggregate_type = 'vm'",
		"event.status IN ('PENDING', 'FAILED', 'CANCELLED')",
	})

	parentRetry := textSection(text, "-- name: ReopenBatchApprovalDispatch", "-- name: SetBatchApprovalEventProcessing")
	checkSectionContains(violations, ticketQueriesPath+" ReopenBatchApprovalDispatch", parentRetry, []string{
		"operation_type IN ('CREATE', 'MODIFY', 'DELETE')",
		"status IN ('EXECUTING', 'FAILED')",
		"NULLIF(BTRIM(parent.approver), '') IS NOT NULL",
	})

	powerRetry := textSection(text, "-- name: ResetPowerRetryTicket", "-- name: StartInitialBatchChildAttempt")
	checkSectionContains(violations, ticketQueriesPath+" ResetPowerRetryTicket", powerRetry, []string{
		"AND child.status = 'FAILED'",
		"AND child.operation_type = 'POWER'",
		"AND child.attempt_count < sqlc.arg(max_attempts)",
		"AND event.created_by = child.requester",
		"AND batch.created_by = parent.requester",
	})
	if strings.Contains(powerRetry, "REJECTED") {
		*violations = append(*violations, fmt.Sprintf("%s ResetPowerRetryTicket must not accept REJECTED", ticketQueriesPath))
	}

	powerEvent := textSection(text, "-- name: ResetBatchPowerRetryEvent", "-- name: ReopenBatchPowerParentForRetry")
	checkSectionContains(violations, ticketQueriesPath+" ResetBatchPowerRetryEvent", powerEvent, []string{
		"child.parent_ticket_id = sqlc.arg(parent_ticket_id)",
		"child.status = 'EXECUTING'",
		"event.status IN ('FAILED', 'CANCELLED')",
		"event.created_by = child.requester",
		"batch.batch_type = 'BATCH_POWER'",
		"batch.created_by = parent.requester",
	})
}

func checkRestartFenceFragments(violations *[]string) {
	checkFileContains(violations, vmHandlerPath, []string{
		`params["operator_action_required"] = true`,
		`params["reconciliation_path"] = "operator-runbook:ambiguous-vm-restart"`,
	})
	for _, path := range []string{
		"internal/api/handlers/server_vm_restart_reconciliation.go",
		"internal/api/handlers/server_vm_restart_reconciliation_test.go",
	} {
		if _, err := os.Stat(path); err == nil {
			*violations = append(*violations, fmt.Sprintf("unsafe ambiguous-restart fence release implementation must be absent: %s", path))
		} else if !os.IsNotExist(err) {
			*violations = append(*violations, fmt.Sprintf("stat %s: %v", path, err))
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
		"type BatchTicket struct",
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
		"func (g *Service) DispatchBatchApproval(",
		"func (g *Service) FailPendingBatchApprovalDispatch(",
		"jobs.ReconcileFailedParentBatchStatus(",
		"BatchApprovalDispatchConsistencyError",
		"g.preflightCreateCloneSource(ctx, child, opts)",
		"g.approveCreateWithConfig(ctx, child",
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
		"SyncParentBatchStatusByChildEvent(",
		"SyncParentBatchStatus(",
		"SyncParentBatchStatusInTx(",
		"ReconcileFailedParentBatchStatus(",
		"lockTicketRowForUpdate()",
		"updateParentBatchTicketStatusWithExpected(",
		"updateDomainEventStatusWithExpected(",
		"parentStatus := entticket.StatusEXECUTING",
	}
	text := string(content)
	for _, n := range needles {
		if !strings.Contains(text, n) {
			*violations = append(*violations, fmt.Sprintf("%s missing fragment %q", jobHelperPath, n))
		}
	}
	checkFileContains(violations, jobHelperPredicatesPath, []string{
		"selector.ForUpdate()",
	})
}

func checkBatchRolloutFragments(violations *[]string) {
	checkFileContains(violations, vmPowerJobPath, []string{
		"case domain.VMPowerDispatchDirect:",
		"case domain.VMPowerDispatchTicket:",
		"missing or invalid immutable dispatch mode",
	})
	checkFileContains(violations, vmModulePath, []string{
		"river.AddWorker(workers, jobs.NewVMPowerWorker(",
	})
	checkFileContains(violations, migrationDocPath, []string{
		"### Historical Replay Payload Integrity Gate",
		"(created_by, normalized operation, trimmed request_id)",
		"malformed or operation-less payloads",
		"### Legacy VM Power `dispatch_mode` Admission Gate",
		"A new-release instance automatically registers this worker",
		"Freeze all",
		"direct VM power submissions",
		"fully consume and drain every runnable `vm_power` job",
		"A new-release worker must not",
		"consume such a job.",
		"reviewed normal",
		"application cancellation/termination flow",
		"`PENDING` legacy item with missing/invalid mode",
		"A direct `PENDING` Event with no Ticket and no runnable old-worker job",
		"hard rollout blocker, not a quarantine candidate",
		"and reviewed reconciliation capability would be required before re-running",
		"this runbook does not assume that such a utility exists",
		"For a `PROCESSING` START/STOP Event",
		"For a `PROCESSING` RESTART Event",
		"Quarantine is reserved for unsafe `PROCESSING` work.",
		"zero unresolved `PENDING`",
		"Events (with or without Tickets) with a missing or invalid `dispatch_mode`",
		"zero runnable",
		"Only after step 3 succeeds, start the new release",
		"perform a blind",
		"backfill, or reinsert a River job",
		"### Dispatcher Recovery and Reconciliation",
		"cancelled` or `discarded` `batch_approval_dispatch`",
		"do not blindly insert another",
		"PROCESSING` restart events",
		"no API can release the fence, and manual workflow-row",
		"edits or redispatch are prohibited.",
		"provider receipt",
		"never redispatch",
	})
}

func checkBatchTraceabilityFragments(violations *[]string) {
	checkFileContains(violations, traceabilityPath, []string{
		"internal/usecase/batch_approval_dispatch_atomic.go",
		"internal/jobs/batch_approval_dispatch.go",
		"internal/governance/ticketing/service.go",
		"internal/repository/sqlc/queries/ticket.sql",
		"internal/api/handlers/server_vm_batch_safety_regression_test.go",
	})
}

func checkBatchDocumentationFragments(violations *[]string) {
	checkFileContains(violations, governanceDocPath, []string{
		"`batch_tickets` (API projection)",
		"best-effort",
		"not durable actor attribution",
		"no public or administrative API releases",
		"provider receipt",
		"**Known contract debt:** accepted ADR-0015 specifies limits",
	})
	checkFileContains(violations, masterFlowPath, []string{
		"Workflow parent Ticket (raw)",
		"Workflow parent Event (raw)",
		"there is no public `APPROVED` parent status",
		"`REJECTED` never flows into execution",
		"best-effort supplemental `vm.batch.retry`/`vm.batch.cancel`",
		"`operator_action_required=true`",
		"No public or administrative API releases this fence.",
		"no API may clear the fence or redispatch an ambiguous restart",
		"Before any new-release instance starts and automatically",
		"unresolved `PENDING` Events (with or without Tickets) with a missing/invalid",
		"mode and zero runnable `vm_power` jobs",
		"A direct orphan `PENDING` Event",
		"with no Ticket and no runnable old-worker job",
		"blocker, not a quarantine candidate",
		"payload is never backfilled and the operation is never replayed or",
	})
	checkFileContains(violations, chineseMasterFlowPath, []string{
		"规范入口：POST /vms/batch",
		"公开父批次状态绝不包含 `APPROVED`。",
		"同一用户意图在超时、网络错误、`5xx`、`429` 或 `409` 后重试时必须复用同一个不透明 `request_id`；只有请求成功或用户明确开始新的意图时才能轮换。",
		"首次分派计为 1，每个子项最多允许三次逻辑尝试。",
		"审批为 `REJECTED` 的子工单是终态，绝不能进入执行或重试。",
		"任何公开或管理 API 都不能清除或重新派发结果不明确的重启围栏。",
		"启动任何会自动注册 worker 的新版本实例前",
		"未解决 `PENDING` Event（无论有无 Ticket）为零",
		"direct orphan `PENDING` Event 在当前版本没有安全转换路径",
		"是升级硬阻断项，不能作为隔离项放行",
		"绝不回填不可变 payload",
	})
	checkFileContains(violations, lifecycleDocPath, []string{
		"`batch_approval_dispatch` River job on its dedicated queue",
		"Approval-`REJECTED` is terminal and cannot enter execution retry.",
		"best-effort",
		"supplemental audit call",
		"No API can release the fence:",
		"provider receipt",
	})
	checkFileContains(violations, frontendQueueDocPath, []string{
		"The list endpoint exposes these parent-row fields",
		"must not invent a public",
		"`retry_after_seconds` (`2s` in the",
		"`operator_action_required=true`",
		"must never expose a\n  fence-clear action",
	})
	checkFileContains(violations, lifecycleRetentionPath, []string{
		"`tickets`, `batch_tickets`",
		"`tickets.parent_ticket_id` carries child lineage",
	})
	checkFileContains(violations, schemaCatalogPath, []string{
		"`tickets`, `batch_tickets`, `approval_policies`",
		"`rate_limit_exemptions`, `rate_limit_user_overrides`",
	})
	checkFileContains(violations, phase4ChecklistPath, []string{
		"`batch_tickets` parent API-projection table implemented",
		"`tickets.parent_ticket_id` child linkage implemented",
		"**Ambiguous Restart Fence**",
		"No API may clear or redispatch an ambiguous restart fence",
	})
}

func checkAllowlist(violations *[]string) {
	content, err := os.ReadFile(allowlistPath)
	if err != nil {
		*violations = append(*violations, fmt.Sprintf("read %s: %v", allowlistPath, err))
		return
	}
	lines := parseAllowlistLines(string(content))

	blocked := []string{
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
		"extractBatchRetryAfterSeconds(",
		"resolveStoredBatchRequestIntent({",
		"getStableBatchRequestIntent(\"CREATE\"",
		"getStableBatchRequestIntent(\"MODIFY\"",
		"getStableBatchRequestIntent(\"DELETE\"",
		"getStableBatchRequestIntent(`POWER:${operation}`",
		"request_id: intent.requestId",
		"clearStoredBatchRequestIntent(submission.intent)",
		"currentCreateIntentRef",
		"currentModifyIntentRef",
		"submissionSequence: number;",
		"nextBatchSubmissionSequenceRef",
		"trackedBatchSubmissionSequenceRef",
		"submission.submissionSequence <= trackedBatchSubmissionSequenceRef.current",
		"trackedBatchSubmissionSequenceRef.current = submission.submissionSequence",
		"submissionSequence: nextBatchSubmissionSequence()",
		"type VMBatchActionMutationInput = {",
		"submission.actorKey !== currentActorKey",
		"submission?.targetTicketIDs ?? []",
		"captureRestartReconciliationNotice(err)",
		"batchRateLimited",
		"batchRetryAfterSeconds",
		"pickBatchActionTargets(",
		"lastBatchActionFeedback",
	})
	checkFileContains(violations, frontendVMHookTests, []string{
		"uses status_url for active batch tracking when batch submit succeeds",
		"enters cooldown on BATCH_RATE_LIMITED and blocks batch actions while countdown active",
		"uses only eligible FAILED children and records authoritative retry/cancel results",
		"reuses request_id after a lost response and rotates it only after success or a new intent",
		"normalizes batch create text before fingerprinting whitespace-equivalent retries",
		"keeps a failed batch modify form open and resets it only after accepted success",
		"keeps a pending modify request_id across close and reopen",
		"normalizes batch modify text before fingerprinting whitespace-equivalent retries",
		"reuses an unresolved power request_id after unmount and remount",
		"retains independent A and B power intents for their own retries",
		"reuses delete and power intents when only localized automatic reasons change",
		"tracks out-of-order successes without letting an older success overwrite a newer one",
		"keeps an earlier accepted batch tracked when a newer submission fails",
		"submissionSequence: expect.any(Number)",
		"surfaces ambiguous restart recovery metadata from batch submit and retry errors",
	})
	checkFileContains(violations, frontendAPIQueryHook, []string{
		"onSuccess: async (data, variables)",
		"options?.onSuccess?.(data, variables)",
		"onError: (error, variables)",
		"options?.onError?.(error, variables)",
	})
	checkFileContains(violations, frontendAPIQueryTest, []string{
		"invalidates configured keys and forwards per-mutation variables on success",
		"forwards per-mutation variables when a mutation fails",
		"expect(onSuccess).toHaveBeenCalledWith({ id: 'sys-1', name: 'demo' }, 'demo')",
		"'failed-submission'",
	})
	checkFileContains(violations, frontendBatchActions, []string{
		"createOpaqueBatchRequestId",
		"globalThis.crypto.randomUUID()",
		"createCanonicalBatchIntentFingerprint",
		"operator-runbook:ambiguous-vm-restart",
		"error.params?.operator_action_required !== true",
	})
	checkFileContains(violations, frontendBatchActionTest, []string{
		"canonicalizes key and item order while distinguishing changed batch intent",
		"normalizes Retry-After, conflict state, and strict restart recovery metadata",
	})
	checkFileContains(violations, frontendBatchIntentStore, []string{
		"BATCH_REQUEST_INTENTS_VERSION = 1",
		"BATCH_REQUEST_INTENT_TTL_MS",
		"BATCH_REQUEST_INTENT_CAPACITY",
		"window.sessionStorage",
		"volatileIntents",
		"sessionStorageUnavailable",
		"intent.actorKey === normalizedActorKey",
		"intent.operationKey === normalizedOperationKey",
		"intent.fingerprint === fingerprint",
		"intent.requestId === accepted.requestId",
	})
	checkFileContains(violations, frontendBatchIntentTest, []string{
		"persists separate canonical intents and clears only the exact accepted identity",
		"isolates the same operation and fingerprint by authenticated actor",
		"recovers from corrupt and incompatible storage without throwing",
		"rotates expired entries and prunes them from the persisted envelope",
		"bounds unresolved intent storage and evicts the oldest entry first",
		"reuses the exact in-memory intent when sessionStorage writes throw",
		"reuses the exact in-memory intent when sessionStorage reads throw",
	})
	checkFileContains(violations, frontendCooldownHook, []string{
		"error.params?.contact_admin === true",
		"untilMs: Math.max(current.untilMs, requestedUntilMs)",
		"contactAdmin: (current.untilMs > now && current.contactAdmin) || contactAdmin",
		"setCooldown({ untilMs: 0, contactAdmin: false })",
	})
	checkFileContains(violations, frontendCooldownTest, []string{
		"clears contact-admin guidance together with the cooldown",
		"never shortens an active cooldown or drops administrator guidance",
		"params: { contact_admin: true }",
	})
	for _, path := range []string{frontendBatchList, frontendBatchDetail} {
		checkFileContains(violations, path, []string{
			"extractRestartReconciliationNotice(",
			`data-testid="restart-reconciliation-alert"`,
			"restart_reconciliation.readonly_description",
			"cooldown.contactAdmin",
			"batch.rate_limited_contact_admin",
			"batchActionPending",
		})
	}
	checkFileContains(violations, frontendBatchList, []string{
		"['vm-batch-list', page, pageSize]",
		"params: { query: { page, per_page: pageSize } }",
		"current: data?.pagination?.page ?? page",
		"onChange={(nextPagination: TablePaginationConfig)",
		"batch.advanced_search_help",
	})
	checkFileContains(violations, frontendBatchListTest, []string{
		"requests controlled server pages and makes the twenty-first batch reachable",
		"disables retry and cancel while %s is pending",
	})
	checkFileContains(violations, frontendBatchDetailTest, []string{
		"disables retry and cancel while %s is pending",
	})
	for _, path := range []string{frontendBatchListTest, frontendBatchDetailTest} {
		checkFileContains(violations, path, []string{
			"shows ambiguous restart retry metadata as read-only runbook guidance",
			"operator-runbook:ambiguous-vm-restart",
			"restart_reconciliation.dismiss",
			"and contact an administrator",
		})
	}
	for _, path := range []string{
		frontendVMPagePath,
		frontendVMHookPath,
		frontendBatchActions,
		frontendBatchList,
		frontendBatchDetail,
	} {
		checkFileNotContains(violations, path, []string{
			"/admin/vm-power-events/",
			"restart-reconciliation/release",
		})
	}
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

func checkFileNotContains(violations *[]string, path string, needles []string) {
	content, err := os.ReadFile(path)
	if err != nil {
		*violations = append(*violations, fmt.Sprintf("read %s: %v", path, err))
		return
	}
	text := string(content)
	for _, needle := range needles {
		if strings.Contains(text, needle) {
			*violations = append(*violations, fmt.Sprintf("%s contains forbidden fragment %q", path, needle))
		}
	}
}

func checkSectionContains(violations *[]string, label, section string, needles []string) {
	if section == "" {
		*violations = append(*violations, fmt.Sprintf("%s section was not found", label))
		return
	}
	for _, needle := range needles {
		if !strings.Contains(section, needle) {
			*violations = append(*violations, fmt.Sprintf("%s missing fragment %q", label, needle))
		}
	}
}

func checkOrderedFragments(violations *[]string, label, section string, fragments []string) {
	if section == "" {
		*violations = append(*violations, fmt.Sprintf("%s section was not found", label))
		return
	}
	position := 0
	for _, fragment := range fragments {
		relative := strings.Index(section[position:], fragment)
		if relative < 0 {
			*violations = append(*violations, fmt.Sprintf("%s missing ordered fragment %q", label, fragment))
			return
		}
		position += relative + len(fragment)
	}
}

func textSection(text, start, end string) string {
	startIndex := strings.Index(text, start)
	if startIndex < 0 {
		return ""
	}
	if end == "" {
		return text[startIndex:]
	}
	relativeEnd := strings.Index(text[startIndex+len(start):], end)
	if relativeEnd < 0 {
		return ""
	}
	return text[startIndex : startIndex+len(start)+relativeEnd]
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

func checkStringEnumSchema(violations *[]string, schemas *yaml.Node, name string, expected []string) {
	schema, ok := mapValue(schemas, name)
	if !ok {
		*violations = append(*violations, fmt.Sprintf("missing OpenAPI schema %s", name))
		return
	}
	checkStringEnumNode(violations, name, schema, expected)
}

func checkStringEnumNode(violations *[]string, label string, node *yaml.Node, expected []string) {
	enumNode, ok := mapValue(node, "enum")
	if !ok || enumNode.Kind != yaml.SequenceNode {
		*violations = append(*violations, fmt.Sprintf("%s must define a string enum", label))
		return
	}
	actual := make([]string, 0, len(enumNode.Content))
	for _, item := range enumNode.Content {
		if item.Kind != yaml.ScalarNode {
			*violations = append(*violations, fmt.Sprintf("%s enum must contain only scalar strings", label))
			return
		}
		actual = append(actual, strings.TrimSpace(item.Value))
	}
	if len(actual) != len(expected) {
		*violations = append(*violations, fmt.Sprintf("%s enum = %v, want %v", label, actual, expected))
		return
	}
	for i := range expected {
		if actual[i] != expected[i] {
			*violations = append(*violations, fmt.Sprintf("%s enum = %v, want %v", label, actual, expected))
			return
		}
	}
}
