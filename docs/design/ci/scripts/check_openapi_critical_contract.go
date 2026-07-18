//go:build ignore

package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"reflect"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	specPath              = "api/openapi.yaml"
	oapiCodegenConfigPath = "api/oapi-codegen.yaml"
	generatedServerPath   = "internal/api/generated/server.gen.go"
)

var allowedGeneratedOptionalPointerFields = map[string]bool{
	`AuditLog.ActorSummary json:"actor_summary"`:                               true,
	`AuditLog.PlacementSummary json:"placement_summary"`:                       true,
	`AuditLog.ResourceSummary json:"resource_summary"`:                         true,
	`AuditLog.TicketSummary json:"ticket_summary"`:                             true,
	`Ticket.PlacementEvaluation json:"placement_evaluation"`:                   true,
	`Ticket.Provisioning json:"provisioning"`:                                  true,
	`Ticket.Summary json:"summary"`:                                            true,
	`VM.ConsoleCapabilities json:"console_capabilities"`:                       true,
	`VM.Provisioning json:"provisioning"`:                                      true,
	`VMBatchChildStatus.Provisioning json:"provisioning"`:                      true,
	`VMConsoleCapabilities.PreferredConsoleType json:"preferred_console_type"`: true,
	`VMConsoleRequestInput.PreferredConsoleType json:"preferred_console_type"`: true,
}

type requiredPathContract struct {
	path             string
	op               string
	operationID      string
	requestSchemaRef string
	responses        []requiredResponseContract
}

type requiredResponseContract struct {
	code        string
	schemaRef   string
	responseRef string
	noContent   bool
}

func main() {
	specBytes, err := os.ReadFile(specPath)
	if err != nil {
		fmt.Printf("FAIL: read %s: %v\n", specPath, err)
		os.Exit(1)
	}

	var doc yaml.Node
	if err := yaml.Unmarshal(specBytes, &doc); err != nil {
		fmt.Printf("FAIL: parse %s: %v\n", specPath, err)
		os.Exit(1)
	}

	root := documentRoot(&doc)
	if root == nil || root.Kind != yaml.MappingNode {
		fmt.Printf("FAIL: %s root must be a mapping node\n", specPath)
		os.Exit(1)
	}

	var violations []string

	checkDuplicateMappingKeyGuard(&violations)
	checkNoDuplicateMappingKeys(root, "$", &violations)
	checkOpenAPIVersion(root, &violations)
	checkGlobalSecurity(root, &violations)
	checkPathContracts(root, &violations)
	checkSchemaContracts(root, &violations)
	checkOapiCodegenOptionalFieldStrategy(&violations)

	if len(violations) > 0 {
		fmt.Println("FAIL: OpenAPI critical contract check failed")
		for _, v := range violations {
			fmt.Println(" -", v)
		}
		fmt.Println("Rule: stage-critical API contracts must not regress (auth/vm/approval/audit/notification + global security).")
		os.Exit(1)
	}

	fmt.Println("OK: OpenAPI critical contract checks passed")
}

func checkDuplicateMappingKeyGuard(violations *[]string) {
	const fixture = "operation:\n  parameters: []\n  parameters: []\n"
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(fixture), &doc); err != nil {
		*violations = append(*violations, fmt.Sprintf("duplicate-key guard fixture must parse into a YAML node: %v", err))
		return
	}
	root := documentRoot(&doc)
	var findings []string
	checkNoDuplicateMappingKeys(root, "$", &findings)
	if len(findings) != 1 || !strings.Contains(findings[0], `duplicate mapping key "parameters"`) {
		*violations = append(*violations, "duplicate-key guard self-test failed: duplicate parameters key was not rejected")
	}
}

func checkNoDuplicateMappingKeys(node *yaml.Node, path string, violations *[]string) {
	if node == nil {
		return
	}
	switch node.Kind {
	case yaml.MappingNode:
		seen := make(map[string]bool, len(node.Content)/2)
		for i := 0; i+1 < len(node.Content); i += 2 {
			key := node.Content[i].Value
			if seen[key] {
				*violations = append(*violations, fmt.Sprintf("%s has duplicate mapping key %q", path, key))
			}
			seen[key] = true
			checkNoDuplicateMappingKeys(node.Content[i+1], path+"."+key, violations)
		}
	case yaml.SequenceNode:
		for i, child := range node.Content {
			checkNoDuplicateMappingKeys(child, fmt.Sprintf("%s[%d]", path, i), violations)
		}
	}
}

func checkOpenAPIVersion(root *yaml.Node, violations *[]string) {
	v, ok := scalarValueByKey(root, "openapi")
	if !ok {
		*violations = append(*violations, "missing root field: openapi")
		return
	}
	if !strings.HasPrefix(v, "3.1.") {
		*violations = append(*violations, fmt.Sprintf("canonical spec must stay on OpenAPI 3.1.x, got %q", v))
	}
}

func checkGlobalSecurity(root *yaml.Node, violations *[]string) {
	components, ok := mapValue(root, "components")
	if !ok {
		*violations = append(*violations, "missing root.components")
		return
	}

	securitySchemes, ok := mapValue(components, "securitySchemes")
	if !ok {
		*violations = append(*violations, "missing components.securitySchemes")
	} else if _, ok := mapValue(securitySchemes, "BearerAuth"); !ok {
		*violations = append(*violations, "missing components.securitySchemes.BearerAuth")
	}

	globalSecurity, ok := mapValue(root, "security")
	if !ok {
		*violations = append(*violations, "missing root.security")
		return
	}
	if !sequenceContainsMapKey(globalSecurity, "BearerAuth") {
		*violations = append(*violations, "root.security must include BearerAuth")
	}
}

func checkPathContracts(root *yaml.Node, violations *[]string) {
	paths, ok := mapValue(root, "paths")
	if !ok {
		*violations = append(*violations, "missing root.paths")
		return
	}

	pathCount := len(paths.Content) / 2
	if pathCount < 25 {
		*violations = append(*violations, fmt.Sprintf("unexpectedly low path count: %d (< 25)", pathCount))
	}

	required := []requiredPathContract{
		{
			path:             "/auth/login",
			op:               "post",
			operationID:      "login",
			requestSchemaRef: "#/components/schemas/LoginRequest",
			responses:        []requiredResponseContract{{code: "200", schemaRef: "#/components/schemas/LoginResponse"}},
		},
		{
			path:        "/auth/me",
			op:          "get",
			operationID: "getCurrentUser",
			responses:   []requiredResponseContract{{code: "200", schemaRef: "#/components/schemas/UserInfo"}},
		},
		{
			path:             "/auth/change-password",
			op:               "post",
			operationID:      "changePassword",
			requestSchemaRef: "#/components/schemas/ChangePasswordRequest",
			responses:        []requiredResponseContract{{code: "204", noContent: true}},
		},
		{
			path:        "/vms",
			op:          "get",
			operationID: "listVMs",
			responses:   []requiredResponseContract{{code: "200", schemaRef: "#/components/schemas/VMList"}},
		},
		{
			path:             "/vms/request",
			op:               "post",
			operationID:      "createVMRequest",
			requestSchemaRef: "#/components/schemas/VMCreateRequest",
			responses:        []requiredResponseContract{{code: "202", schemaRef: "#/components/schemas/TicketResponse"}},
		},
		{
			path:        "/vms/{vm_id}",
			op:          "get",
			operationID: "getVM",
			responses:   []requiredResponseContract{{code: "200", schemaRef: "#/components/schemas/VM"}},
		},
		{
			path:        "/vms/{vm_id}",
			op:          "delete",
			operationID: "deleteVM",
			responses:   []requiredResponseContract{{code: "202", schemaRef: "#/components/schemas/DeleteVMResponse"}},
		},
		{
			path:        "/vms/{vm_id}/console/request",
			op:          "post",
			operationID: "requestVMConsoleAccess",
			responses: []requiredResponseContract{
				{code: "200", schemaRef: "#/components/schemas/VMConsoleRequestResponse"},
				{code: "202", schemaRef: "#/components/schemas/VMConsoleRequestResponse"},
			},
		},
		{
			path:        "/vms/{vm_id}/console/status",
			op:          "get",
			operationID: "getVMConsoleStatus",
			responses:   []requiredResponseContract{{code: "200", schemaRef: "#/components/schemas/VMConsoleStatusResponse"}},
		},
		{
			path:        "/vms/{vm_id}/vnc",
			op:          "get",
			operationID: "openVMVNC",
			responses:   []requiredResponseContract{{code: "200", schemaRef: "#/components/schemas/VMVNCSessionResponse"}},
		},
		{
			path:        "/tickets",
			op:          "get",
			operationID: "listTickets",
			responses:   []requiredResponseContract{{code: "200", schemaRef: "#/components/schemas/TicketList"}},
		},
		{
			path:        "/builtin-approval/tasks",
			op:          "get",
			operationID: "listBuiltinApprovalTasks",
			responses:   []requiredResponseContract{{code: "200", schemaRef: "#/components/schemas/TicketList"}},
		},
		{
			path:             "/builtin-approval/tasks/{ticket_id}/approve",
			op:               "post",
			operationID:      "approveBuiltinApprovalTask",
			requestSchemaRef: "#/components/schemas/ApprovalDecisionRequest",
			responses:        []requiredResponseContract{{code: "204", noContent: true}},
		},
		{
			path:             "/builtin-approval/tasks/{ticket_id}/reject",
			op:               "post",
			operationID:      "rejectBuiltinApprovalTask",
			requestSchemaRef: "#/components/schemas/RejectDecisionRequest",
			responses:        []requiredResponseContract{{code: "204", noContent: true}},
		},
		{
			path:        "/tickets/{ticket_id}/cancel",
			op:          "post",
			operationID: "cancelTicket",
			responses:   []requiredResponseContract{{code: "204", noContent: true}},
		},
		{
			path:        "/audit-logs",
			op:          "get",
			operationID: "listAuditLogs",
			responses:   []requiredResponseContract{{code: "200", schemaRef: "#/components/schemas/AuditLogList"}},
		},
		{
			path:        "/notifications",
			op:          "get",
			operationID: "listNotifications",
			responses:   []requiredResponseContract{{code: "200", schemaRef: "#/components/schemas/NotificationList"}},
		},
		{
			path:        "/notifications/unread-count",
			op:          "get",
			operationID: "getUnreadCount",
			responses:   []requiredResponseContract{{code: "200", schemaRef: "#/components/schemas/UnreadCount"}},
		},
		{
			path:        "/notifications/{notification_id}/read",
			op:          "patch",
			operationID: "markNotificationRead",
			responses:   []requiredResponseContract{{code: "204", noContent: true}},
		},
		{
			path:        "/notifications/mark-all-read",
			op:          "post",
			operationID: "markAllNotificationsRead",
			responses:   []requiredResponseContract{{code: "204", noContent: true}},
		},
	}

	for _, contract := range required {
		checkRequiredPathContract(paths, contract, violations)
	}

	checkSystemMemberConflictContracts(paths, violations)
	checkExternalAuthFailureContracts(paths, violations)
	checkAuthSecurityContracts(paths, violations)
	checkBatchSubmitContracts(root, paths, violations)
}

func checkBatchSubmitContracts(root, paths *yaml.Node, violations *[]string) {
	for _, path := range []string{"/vms/batch", "/vms/batch/power"} {
		pathNode, ok := mapValue(paths, path)
		if !ok {
			*violations = append(*violations, "paths."+path+" is missing")
			continue
		}
		operationNode, ok := mapValue(pathNode, "post")
		if !ok {
			*violations = append(*violations, "paths."+path+".post is missing")
			continue
		}
		notFound, ok := operationResponseNode(operationNode, "404")
		if !ok {
			*violations = append(*violations, "paths."+path+".post.responses.404 is missing")
		} else if ref, ok := scalarValueByKey(notFound, "$ref"); !ok || ref != "#/components/responses/NotFound" {
			*violations = append(*violations, "paths."+path+".post.responses.404 must use #/components/responses/NotFound")
		}

		rateLimited, ok := operationResponseNode(operationNode, "429")
		if !ok {
			*violations = append(*violations, "paths."+path+".post.responses.429 is missing")
			continue
		}
		description, _ := scalarValueByKey(rateLimited, "description")
		if !strings.Contains(description, "BATCH_RATE_LIMITED") {
			*violations = append(*violations, "paths."+path+".post.responses.429 must document BATCH_RATE_LIMITED")
		}
		if ref, ok := responseSchemaRef(rateLimited); !ok || ref != "#/components/schemas/Error" {
			*violations = append(*violations, "paths."+path+".post.responses.429 must use the Error schema")
		}
		if !responseExampleCodes(rateLimited)["BATCH_RATE_LIMITED"] {
			*violations = append(*violations, "paths."+path+".post.responses.429 example must use BATCH_RATE_LIMITED")
		}
		headers, headersOK := mapValue(rateLimited, "headers")
		retryAfter, retryOK := mapValue(headers, "Retry-After")
		retrySchema, schemaOK := mapValue(retryAfter, "schema")
		retryType, typeOK := scalarValueByKey(retrySchema, "type")
		minimum, minimumOK := scalarValueByKey(retrySchema, "minimum")
		if !headersOK || !retryOK || !schemaOK || !typeOK || retryType != "integer" || !minimumOK || minimum != "1" {
			*violations = append(*violations, "paths."+path+".post.responses.429 must define integer Retry-After with minimum 1")
		}
		content, _ := mapValue(rateLimited, "content")
		media, _ := mapValue(content, "application/json")
		example, _ := mapValue(media, "example")
		params, _ := mapValue(example, "params")
		if retrySeconds, ok := scalarValueByKey(params, "retry_after_seconds"); !ok || retrySeconds != "2" {
			*violations = append(*violations, "paths."+path+".post.responses.429 example must include params.retry_after_seconds=2")
		}
	}

	batchPath, _ := mapValue(paths, "/vms/batch")
	batchPost, _ := mapValue(batchPath, "post")
	requestBody, _ := mapValue(batchPost, "requestBody")
	content, _ := mapValue(requestBody, "content")
	media, _ := mapValue(content, "application/json")
	schema, _ := mapValue(media, "schema")
	checkBatchCreateExample("paths./vms/batch.post request schema example", mapValueOrNil(schema, "example"), violations)
	checkBatchCreateExample("paths./vms/batch.post media example", mapValueOrNil(media, "example"), violations)

	components, _ := mapValue(root, "components")
	schemas, _ := mapValue(components, "schemas")
	requestSchema, _ := mapValue(schemas, "VMBatchSubmitRequest")
	checkBatchCreateExample("components.schemas.VMBatchSubmitRequest example", mapValueOrNil(requestSchema, "example"), violations)

	childSchema, _ := mapValue(schemas, "VMBatchChildItem")
	checkBatchCreateItemExample("components.schemas.VMBatchChildItem example", mapValueOrNil(childSchema, "example"), violations)
	childProperties, _ := mapValue(childSchema, "properties")
	for _, contract := range []struct {
		field    string
		expected string
	}{
		{field: "service_id", expected: "11111111-1111-4111-8111-111111111111"},
		{field: "template_id", expected: "22222222-2222-4222-8222-222222222222"},
		{field: "instance_size_id", expected: "33333333-3333-4333-8333-333333333333"},
	} {
		property, _ := mapValue(childProperties, contract.field)
		if example, ok := scalarValueByKey(property, "example"); !ok || example != contract.expected {
			*violations = append(*violations, "components.schemas.VMBatchChildItem.properties."+contract.field+" example must be a valid UUID")
		}
	}
}

func mapValueOrNil(node *yaml.Node, key string) *yaml.Node {
	value, _ := mapValue(node, key)
	return value
}

func checkBatchCreateExample(label string, example *yaml.Node, violations *[]string) {
	if example == nil {
		*violations = append(*violations, label+" is missing")
		return
	}
	operation, _ := scalarValueByKey(example, "operation")
	items, itemsOK := mapValue(example, "items")
	if operation != "CREATE" || !itemsOK || items.Kind != yaml.SequenceNode || len(items.Content) == 0 {
		*violations = append(*violations, label+" must contain a CREATE item")
		return
	}
	checkBatchCreateItemExample(label+" CREATE item", items.Content[0], violations)
}

func checkBatchCreateItemExample(label string, item *yaml.Node, violations *[]string) {
	if item == nil || item.Kind != yaml.MappingNode {
		*violations = append(*violations, label+" must be an object")
		return
	}
	for _, field := range []string{"service_id", "template_id", "instance_size_id", "namespace"} {
		if value, ok := scalarValueByKey(item, field); !ok || strings.TrimSpace(value) == "" {
			*violations = append(*violations, label+" missing required "+field)
		}
	}
	if _, ok := scalarValueByKey(item, "vm_id"); ok {
		*violations = append(*violations, label+" CREATE item must not claim an existing vm_id")
	}
}

func checkExternalAuthFailureContracts(paths *yaml.Node, violations *[]string) {
	const submitPath = "/auth/providers/{provider_id}/login/submit"
	submitPathNode, ok := mapValue(paths, submitPath)
	if !ok {
		*violations = append(*violations, "paths."+submitPath+" is missing")
	} else if submitNode, ok := mapValue(submitPathNode, "post"); !ok {
		*violations = append(*violations, "paths."+submitPath+".post is missing")
	} else {
		checkJSONErrorResponseCode(submitPath, "post", submitNode, "403", "USER_DISABLED", violations)
		checkJSONErrorResponseCode(submitPath, "post", submitNode, "404", "AUTH_PROVIDER_NOT_FOUND", violations)
		checkJSONErrorResponseCode(submitPath, "post", submitNode, "409", "AUTH_PROVIDER_CHANGED", violations)
		checkJSONErrorResponseCode(submitPath, "post", submitNode, "409", "EXTERNAL_IDENTITY_CONFLICT", violations)
	}

	const callbackPath = "/auth/providers/{provider_id}/callback"
	callbackPathNode, ok := mapValue(paths, callbackPath)
	if !ok {
		*violations = append(*violations, "paths."+callbackPath+" is missing")
		return
	}
	for _, op := range []string{"get", "post"} {
		operationNode, ok := mapValue(callbackPathNode, op)
		if !ok {
			*violations = append(*violations, "paths."+callbackPath+"."+op+" is missing")
			continue
		}
		checkHTMLBridgeResponseCodes(callbackPath, op, operationNode, "403", []string{"USER_DISABLED"}, violations)
		checkHTMLBridgeResponseCodes(callbackPath, op, operationNode, "404", []string{"AUTH_PROVIDER_NOT_FOUND"}, violations)
		checkHTMLBridgeResponseCodes(callbackPath, op, operationNode, "409", []string{"AUTH_PROVIDER_CHANGED", "EXTERNAL_IDENTITY_CONFLICT"}, violations)
	}
}

func checkJSONErrorResponseCode(path, op string, operationNode *yaml.Node, status, code string, violations *[]string) {
	responseNode, ok := operationResponseNode(operationNode, status)
	if !ok {
		*violations = append(*violations, fmt.Sprintf("paths.%s.%s.responses.%s is missing", path, op, status))
		return
	}
	description, _ := scalarValueByKey(responseNode, "description")
	if !strings.Contains(description, code) {
		*violations = append(*violations, fmt.Sprintf("paths.%s.%s.responses.%s.description must document %s", path, op, status, code))
	}
	if ref, ok := responseSchemaRef(responseNode); !ok || ref != "#/components/schemas/Error" {
		*violations = append(*violations, fmt.Sprintf("paths.%s.%s.responses.%s must use the application/json Error schema", path, op, status))
	}
	if !responseExampleCodes(responseNode)[code] {
		*violations = append(*violations, fmt.Sprintf("paths.%s.%s.responses.%s example must include code %s", path, op, status, code))
	}
}

func checkHTMLBridgeResponseCodes(path, op string, operationNode *yaml.Node, status string, codes []string, violations *[]string) {
	responseNode, ok := operationResponseNode(operationNode, status)
	if !ok {
		*violations = append(*violations, fmt.Sprintf("paths.%s.%s.responses.%s is missing", path, op, status))
		return
	}
	description, _ := scalarValueByKey(responseNode, "description")
	contentNode, hasContent := mapValue(responseNode, "content")
	mediaNode, hasHTML := mapValue(contentNode, "text/html")
	schemaNode, hasSchema := mapValue(mediaNode, "schema")
	schemaType, isString := scalarValueByKey(schemaNode, "type")
	if !hasContent || !hasHTML || !hasSchema || !isString || schemaType != "string" {
		*violations = append(*violations, fmt.Sprintf("paths.%s.%s.responses.%s must model the HTML bridge as text/html string content", path, op, status))
	}
	exampleCodes := htmlResponseExampleCodes(mediaNode)
	for _, code := range codes {
		if !strings.Contains(description, code) {
			*violations = append(*violations, fmt.Sprintf("paths.%s.%s.responses.%s.description must document %s", path, op, status, code))
		}
		if !exampleCodes[code] {
			*violations = append(*violations, fmt.Sprintf("paths.%s.%s.responses.%s HTML example must include code %s", path, op, status, code))
		}
	}
}

func operationResponseNode(operationNode *yaml.Node, status string) (*yaml.Node, bool) {
	responsesNode, ok := mapValue(operationNode, "responses")
	if !ok {
		return nil, false
	}
	return mapValue(responsesNode, status)
}

func htmlResponseExampleCodes(mediaNode *yaml.Node) map[string]bool {
	codes := make(map[string]bool)
	if exampleNode, ok := mapValue(mediaNode, "example"); ok && exampleNode.Kind == yaml.ScalarNode {
		collectKnownExternalAuthCodes(exampleNode.Value, codes)
	}
	if examplesNode, ok := mapValue(mediaNode, "examples"); ok && examplesNode.Kind == yaml.MappingNode {
		for i := 1; i < len(examplesNode.Content); i += 2 {
			valueNode, ok := mapValue(examplesNode.Content[i], "value")
			if ok && valueNode.Kind == yaml.ScalarNode {
				collectKnownExternalAuthCodes(valueNode.Value, codes)
			}
		}
	}
	return codes
}

func collectKnownExternalAuthCodes(value string, codes map[string]bool) {
	for _, code := range []string{"USER_DISABLED", "AUTH_PROVIDER_NOT_FOUND", "AUTH_PROVIDER_CHANGED", "EXTERNAL_IDENTITY_CONFLICT"} {
		if strings.Contains(value, code) {
			codes[code] = true
		}
	}
}

func checkSystemMemberConflictContracts(paths *yaml.Node, violations *[]string) {
	const memberPath = "/systems/{system_id}/members/{user_id}"
	pathNode, ok := mapValue(paths, memberPath)
	if !ok {
		*violations = append(*violations, "paths."+memberPath+" is missing")
		return
	}

	patchNode, ok := mapValue(pathNode, "patch")
	if !ok {
		*violations = append(*violations, "paths."+memberPath+".patch is missing")
	} else {
		checkSystemMemberPatchConflict(memberPath, patchNode, violations)
	}

	deleteNode, ok := mapValue(pathNode, "delete")
	if !ok {
		*violations = append(*violations, "paths."+memberPath+".delete is missing")
		return
	}
	responsesNode, ok := mapValue(deleteNode, "responses")
	if !ok {
		*violations = append(*violations, "paths."+memberPath+".delete.responses is missing")
		return
	}
	conflictNode, ok := mapValue(responsesNode, "409")
	if !ok {
		*violations = append(*violations, "paths."+memberPath+".delete.responses.409 is missing")
		return
	}
	ref, ok := scalarValueByKey(conflictNode, "$ref")
	if !ok || ref != "#/components/responses/LastSystemOwnerConflict" {
		*violations = append(*violations, "paths."+memberPath+".delete.responses.409.$ref must remain #/components/responses/LastSystemOwnerConflict")
	}
}

func checkSystemMemberPatchConflict(path string, patchNode *yaml.Node, violations *[]string) {
	responsesNode, ok := mapValue(patchNode, "responses")
	if !ok {
		*violations = append(*violations, "paths."+path+".patch.responses is missing")
		return
	}
	conflictNode, ok := mapValue(responsesNode, "409")
	if !ok {
		*violations = append(*violations, "paths."+path+".patch.responses.409 is missing")
		return
	}

	description, _ := scalarValueByKey(conflictNode, "description")
	for _, code := range []string{"USER_DISABLED", "LAST_OWNER_CANNOT_BE_REMOVED"} {
		if !strings.Contains(description, code) {
			*violations = append(*violations, fmt.Sprintf("paths.%s.patch.responses.409.description must document %s", path, code))
		}
	}
	if ref, ok := responseSchemaRef(conflictNode); !ok || ref != "#/components/schemas/Error" {
		*violations = append(*violations, "paths."+path+".patch.responses.409 schema ref must be #/components/schemas/Error")
	}

	codes := responseExampleCodes(conflictNode)
	for _, code := range []string{"USER_DISABLED", "LAST_OWNER_CANNOT_BE_REMOVED"} {
		if !codes[code] {
			*violations = append(*violations, fmt.Sprintf("paths.%s.patch.responses.409 examples must include code %s", path, code))
		}
	}
}

func responseExampleCodes(responseNode *yaml.Node) map[string]bool {
	codes := make(map[string]bool)
	contentNode, ok := mapValue(responseNode, "content")
	if !ok {
		return codes
	}
	mediaNode, ok := mapValue(contentNode, "application/json")
	if !ok {
		return codes
	}
	if exampleNode, ok := mapValue(mediaNode, "example"); ok {
		if code, ok := scalarValueByKey(exampleNode, "code"); ok {
			codes[code] = true
		}
	}
	examplesNode, ok := mapValue(mediaNode, "examples")
	if !ok || examplesNode.Kind != yaml.MappingNode {
		return codes
	}
	for i := 1; i < len(examplesNode.Content); i += 2 {
		valueNode, ok := mapValue(examplesNode.Content[i], "value")
		if !ok {
			continue
		}
		if code, ok := scalarValueByKey(valueNode, "code"); ok {
			codes[code] = true
		}
	}
	return codes
}

func checkRequiredPathContract(paths *yaml.Node, contract requiredPathContract, violations *[]string) {
	pathNode, ok := mapValue(paths, contract.path)
	if !ok {
		*violations = append(*violations, fmt.Sprintf("missing path: paths.%s", contract.path))
		return
	}
	operationNode, ok := mapValue(pathNode, contract.op)
	if !ok {
		*violations = append(*violations, fmt.Sprintf("missing operation: paths.%s.%s", contract.path, contract.op))
		return
	}

	if contract.operationID != "" {
		operationID, ok := scalarValueByKey(operationNode, "operationId")
		if !ok || operationID != contract.operationID {
			*violations = append(*violations, fmt.Sprintf("paths.%s.%s.operationId must be %q", contract.path, contract.op, contract.operationID))
		}
	}

	if contract.requestSchemaRef != "" {
		actualRef, ok := operationRequestSchemaRef(operationNode)
		if !ok || actualRef != contract.requestSchemaRef {
			*violations = append(*violations, fmt.Sprintf("paths.%s.%s request schema ref must be %q", contract.path, contract.op, contract.requestSchemaRef))
		}
	}

	for _, response := range contract.responses {
		checkRequiredResponseContract(contract.path, contract.op, operationNode, response, violations)
	}
}

func checkRequiredResponseContract(path string, op string, operationNode *yaml.Node, contract requiredResponseContract, violations *[]string) {
	responsesNode, ok := mapValue(operationNode, "responses")
	if !ok {
		*violations = append(*violations, fmt.Sprintf("paths.%s.%s.responses is missing", path, op))
		return
	}

	responseNode, ok := mapValue(responsesNode, contract.code)
	if !ok {
		*violations = append(*violations, fmt.Sprintf("paths.%s.%s.responses.%s is missing", path, op, contract.code))
		return
	}

	if contract.responseRef != "" {
		ref, ok := scalarValueByKey(responseNode, "$ref")
		if !ok || ref != contract.responseRef {
			*violations = append(*violations, fmt.Sprintf("paths.%s.%s.responses.%s.$ref must be %q", path, op, contract.code, contract.responseRef))
		}
	}

	if contract.noContent {
		if _, hasContent := mapValue(responseNode, "content"); hasContent {
			*violations = append(*violations, fmt.Sprintf("paths.%s.%s.responses.%s must not define content", path, op, contract.code))
		}
		return
	}

	if contract.schemaRef != "" {
		ref, ok := responseSchemaRef(responseNode)
		if !ok || ref != contract.schemaRef {
			*violations = append(*violations, fmt.Sprintf("paths.%s.%s.responses.%s schema ref must be %q", path, op, contract.code, contract.schemaRef))
		}
	}
}

func checkAuthSecurityContracts(paths *yaml.Node, violations *[]string) {
	loginPath, ok := mapValue(paths, "/auth/login")
	if ok {
		if loginOp, ok := mapValue(loginPath, "post"); ok {
			securityNode, ok := mapValue(loginOp, "security")
			if !ok || securityNode.Kind != yaml.SequenceNode || len(securityNode.Content) != 0 {
				*violations = append(*violations, "paths./auth/login.post.security must be an explicit empty array")
			}
		}
	}

	mePath, ok := mapValue(paths, "/auth/me")
	if ok {
		if meOp, ok := mapValue(mePath, "get"); ok {
			securityNode, ok := mapValue(meOp, "security")
			if !ok || !sequenceContainsMapKey(securityNode, "BearerAuth") {
				*violations = append(*violations, "paths./auth/me.get.security must include BearerAuth")
			}
		}
	}
}

func checkSchemaContracts(root *yaml.Node, violations *[]string) {
	components, ok := mapValue(root, "components")
	if !ok {
		*violations = append(*violations, "missing root.components")
		return
	}

	schemas, ok := mapValue(components, "schemas")
	if !ok {
		*violations = append(*violations, "missing components.schemas")
		return
	}

	for _, name := range []string{
		"Error",
		"Pagination",
		"VM",
		"VMList",
		"VMCreateRequest",
		"TicketResponse",
		"DeleteVMResponse",
		"Ticket",
		"TicketList",
		"ApprovalDecisionRequest",
		"RejectDecisionRequest",
		"LoginRequest",
		"LoginResponse",
		"UserInfo",
		"ChangePasswordRequest",
		"AuditLog",
		"AuditLogList",
		"Notification",
		"NotificationList",
		"UnreadCount",
		"VMBatchJobSummary",
	} {
		if _, ok := mapValue(schemas, name); !ok {
			*violations = append(*violations, fmt.Sprintf("missing schema: components.schemas.%s", name))
		}
	}

	if schema, ok := mapValue(schemas, "Error"); ok {
		checkErrorSchema(schema, violations)
	}
	if schema, ok := mapValue(schemas, "Pagination"); ok {
		checkPaginationSchema(schema, violations)
	}
	if schema, ok := mapValue(schemas, "VMCreateRequest"); ok {
		checkVMCreateRequestSchema(schema, violations)
	}
	if schema, ok := mapValue(schemas, "TicketResponse"); ok {
		checkTicketResponseSchema(schema, violations)
	}
	if schema, ok := mapValue(schemas, "DeleteVMResponse"); ok {
		checkDeleteVMResponseSchema(schema, violations)
	}
	if schema, ok := mapValue(schemas, "Ticket"); ok {
		checkTicketSchema(schema, violations)
	}
	if schema, ok := mapValue(schemas, "RejectDecisionRequest"); ok {
		checkRejectDecisionRequestSchema(schema, violations)
	}
	if schema, ok := mapValue(schemas, "ApprovalDecisionRequest"); ok {
		checkApprovalDecisionRequestSchema(schema, violations)
	}
	if schema, ok := mapValue(schemas, "AuditLogList"); ok {
		checkAuditLogListSchema(schema, violations)
	}
	if schema, ok := mapValue(schemas, "NotificationList"); ok {
		checkNotificationListSchema(schema, violations)
	}
	if schema, ok := mapValue(schemas, "UnreadCount"); ok {
		checkUnreadCountSchema(schema, violations)
	}
	if schema, ok := mapValue(schemas, "VMBatchJobSummary"); ok {
		checkVMBatchJobSummarySchema(schema, violations)
	}
}

func checkVMBatchJobSummarySchema(schema *yaml.Node, violations *[]string) {
	properties, ok := mapValue(schema, "properties")
	if !ok {
		*violations = append(*violations, "components.schemas.VMBatchJobSummary must define properties")
		return
	}

	examples := make(map[string]int, 4)
	for _, field := range []string{"child_count", "success_count", "failed_count", "pending_count"} {
		property, ok := mapValue(properties, field)
		if !ok {
			*violations = append(*violations, fmt.Sprintf("components.schemas.VMBatchJobSummary.properties.%s is missing", field))
			continue
		}
		example, ok := scalarValueByKey(property, "example")
		if !ok {
			*violations = append(*violations, fmt.Sprintf("components.schemas.VMBatchJobSummary.properties.%s.example is missing", field))
			continue
		}
		value, err := strconv.Atoi(example)
		if err != nil || value < 0 {
			*violations = append(*violations, fmt.Sprintf("components.schemas.VMBatchJobSummary.properties.%s.example must be a non-negative integer", field))
			continue
		}
		examples[field] = value
	}

	if len(examples) != 4 {
		return
	}
	terminalAndPending := examples["success_count"] + examples["failed_count"] + examples["pending_count"]
	if terminalAndPending != examples["child_count"] {
		*violations = append(*violations, fmt.Sprintf(
			"components.schemas.VMBatchJobSummary property examples are inconsistent: success_count + failed_count + pending_count = %d, child_count = %d",
			terminalAndPending,
			examples["child_count"],
		))
	}
}

func checkOapiCodegenOptionalFieldStrategy(violations *[]string) {
	cfgBytes, err := os.ReadFile(oapiCodegenConfigPath)
	if err != nil {
		*violations = append(*violations, fmt.Sprintf("read %s: %v", oapiCodegenConfigPath, err))
		return
	}

	var cfgDoc yaml.Node
	if err := yaml.Unmarshal(cfgBytes, &cfgDoc); err != nil {
		*violations = append(*violations, fmt.Sprintf("parse %s: %v", oapiCodegenConfigPath, err))
		return
	}
	cfgRoot := documentRoot(&cfgDoc)
	outputOptions, ok := mapValue(cfgRoot, "output-options")
	if !ok {
		*violations = append(*violations, "api/oapi-codegen.yaml must define output-options")
		return
	}
	if v, ok := scalarValueByKey(outputOptions, "prefer-skip-optional-pointer"); !ok || v != "true" {
		*violations = append(*violations, "api/oapi-codegen.yaml output-options.prefer-skip-optional-pointer must be true")
	}
	if v, ok := scalarValueByKey(outputOptions, "prefer-skip-optional-pointer-with-omitzero"); !ok || v != "true" {
		*violations = append(*violations, "api/oapi-codegen.yaml output-options.prefer-skip-optional-pointer-with-omitzero must be true")
	}
	if v, ok := scalarValueByKey(outputOptions, "nullable-type"); ok && v == "true" {
		*violations = append(*violations, "api/oapi-codegen.yaml output-options.nullable-type must stay disabled under ADR-0028 pointer-based nullable semantics")
	}

	generatedFile, err := parser.ParseFile(token.NewFileSet(), generatedServerPath, nil, parser.ParseComments)
	if err != nil {
		*violations = append(*violations, fmt.Sprintf("parse %s: %v", generatedServerPath, err))
		return
	}

	ast.Inspect(generatedFile, func(node ast.Node) bool {
		typeSpec, ok := node.(*ast.TypeSpec)
		if !ok {
			return true
		}
		structType, ok := typeSpec.Type.(*ast.StructType)
		if !ok {
			return false
		}
		for _, field := range structType.Fields.List {
			if field.Tag == nil {
				continue
			}
			jsonTag := reflect.StructTag(strings.Trim(field.Tag.Value, "`")).Get("json")
			if jsonTag == "" || jsonTag == "-" {
				continue
			}
			jsonName, jsonOptions := parseJSONTag(jsonTag)
			if jsonName == "" || jsonName == "-" {
				continue
			}
			hasOmitEmpty := jsonOptions["omitempty"]
			hasOmitZero := jsonOptions["omitzero"]
			if !hasOmitEmpty && !hasOmitZero {
				continue
			}

			fieldName := "<anonymous>"
			if len(field.Names) > 0 {
				fieldName = field.Names[0].Name
			}
			label := fmt.Sprintf("%s.%s json:%q", typeSpec.Name.Name, fieldName, jsonName)
			isPointer := isPointerExpr(field.Type)

			if hasOmitZero && isPointer {
				*violations = append(*violations, fmt.Sprintf("%s must not combine pointer type with omitzero", label))
			}
			if hasOmitEmpty && !hasOmitZero && !isPointer {
				*violations = append(*violations, fmt.Sprintf("%s is optional value field but lacks omitzero", label))
			}
			if hasOmitEmpty && !hasOmitZero && isPointerToBuiltinScalar(field.Type) {
				*violations = append(*violations, fmt.Sprintf("%s must not use an unnecessary pointer to a builtin scalar", label))
			}
			if hasOmitEmpty && !hasOmitZero && isPointer && !allowedGeneratedOptionalPointerFields[label] {
				*violations = append(*violations, fmt.Sprintf("%s is a new optional pointer field; either use omitzero value semantics or add an explicit ADR-0028 exception", label))
			}
		}
		return false
	})
}

func parseJSONTag(tag string) (string, map[string]bool) {
	parts := strings.Split(tag, ",")
	name := parts[0]
	options := make(map[string]bool, len(parts))
	for _, opt := range parts[1:] {
		if opt != "" {
			options[opt] = true
		}
	}
	return name, options
}

func isPointerExpr(expr ast.Expr) bool {
	_, ok := expr.(*ast.StarExpr)
	return ok
}

func isPointerToBuiltinScalar(expr ast.Expr) bool {
	star, ok := expr.(*ast.StarExpr)
	if !ok {
		return false
	}
	ident, ok := star.X.(*ast.Ident)
	if !ok {
		return false
	}
	switch ident.Name {
	case "string", "bool", "int", "int8", "int16", "int32", "int64",
		"uint", "uint8", "uint16", "uint32", "uint64",
		"float32", "float64":
		return true
	default:
		return false
	}
}

func checkErrorSchema(schema *yaml.Node, violations *[]string) {
	requireSchemaRequiredFields("Error", schema, []string{"code"}, violations)

	properties, ok := mapValue(schema, "properties")
	if !ok {
		*violations = append(*violations, "components.schemas.Error must define properties")
		return
	}

	params, ok := mapValue(properties, "params")
	if !ok {
		*violations = append(*violations, "components.schemas.Error.properties.params is missing")
		return
	}

	if typ, ok := scalarValueByKey(params, "type"); !ok || typ != "object" {
		*violations = append(*violations, "components.schemas.Error.properties.params.type must be object")
	}
	if ap, ok := scalarValueByKey(params, "additionalProperties"); !ok || ap != "true" {
		*violations = append(*violations, "components.schemas.Error.properties.params.additionalProperties must be true")
	}
}

func checkPaginationSchema(schema *yaml.Node, violations *[]string) {
	properties, ok := mapValue(schema, "properties")
	if !ok {
		*violations = append(*violations, "components.schemas.Pagination must define properties")
		return
	}
	for _, field := range []string{"page", "per_page", "total", "total_pages"} {
		prop, ok := mapValue(properties, field)
		if !ok {
			*violations = append(*violations, fmt.Sprintf("components.schemas.Pagination.properties.%s is missing", field))
			continue
		}
		if typ, ok := scalarValueByKey(prop, "type"); !ok || typ != "integer" {
			*violations = append(*violations, fmt.Sprintf("components.schemas.Pagination.properties.%s.type must be integer", field))
		}
	}
}

func checkVMCreateRequestSchema(schema *yaml.Node, violations *[]string) {
	requireSchemaRequiredFields("VMCreateRequest", schema, []string{"service_id", "template_id", "instance_size_id", "namespace", "reason"}, violations)

	properties, ok := mapValue(schema, "properties")
	if !ok {
		*violations = append(*violations, "components.schemas.VMCreateRequest must define properties")
		return
	}

	for _, field := range []string{"service_id", "template_id", "instance_size_id", "namespace", "reason"} {
		if _, ok := mapValue(properties, field); !ok {
			*violations = append(*violations, fmt.Sprintf("components.schemas.VMCreateRequest.properties.%s is missing", field))
		}
	}

	if _, hasClusterID := mapValue(properties, "cluster_id"); hasClusterID {
		*violations = append(*violations, "components.schemas.VMCreateRequest must not define cluster_id (ADR-0017)")
	}
}

func checkTicketResponseSchema(schema *yaml.Node, violations *[]string) {
	requireSchemaRequiredFields("TicketResponse", schema, []string{"ticket_id", "status"}, violations)

	status, ok := schemaProperty(schema, "status")
	if !ok {
		*violations = append(*violations, "components.schemas.TicketResponse.properties.status is missing")
		return
	}
	if !enumContains(status, "PENDING") {
		*violations = append(*violations, "components.schemas.TicketResponse.properties.status.enum must include PENDING")
	}
}

func checkDeleteVMResponseSchema(schema *yaml.Node, violations *[]string) {
	requireSchemaRequiredFields("DeleteVMResponse", schema, []string{"ticket_id", "event_id", "status"}, violations)

	status, ok := schemaProperty(schema, "status")
	if !ok {
		*violations = append(*violations, "components.schemas.DeleteVMResponse.properties.status is missing")
		return
	}
	if !enumContains(status, "PENDING") {
		*violations = append(*violations, "components.schemas.DeleteVMResponse.properties.status.enum must include PENDING")
	}
}

func checkTicketSchema(schema *yaml.Node, violations *[]string) {
	requireSchemaRequiredFields("Ticket", schema, []string{"id", "event_id", "status", "requester"}, violations)

	operationType, ok := schemaProperty(schema, "operation_type")
	if !ok {
		*violations = append(*violations, "components.schemas.Ticket.properties.operation_type is missing")
	} else {
		for _, v := range []string{"CREATE", "DELETE"} {
			if !enumContains(operationType, v) {
				*violations = append(*violations, fmt.Sprintf("components.schemas.Ticket.properties.operation_type.enum must include %s", v))
			}
		}
	}

	status, ok := schemaProperty(schema, "status")
	if !ok {
		*violations = append(*violations, "components.schemas.Ticket.properties.status is missing")
	} else {
		for _, v := range []string{"PENDING", "APPROVED", "REJECTED", "CANCELLED", "EXECUTING", "SUCCESS", "FAILED"} {
			if !enumContains(status, v) {
				*violations = append(*violations, fmt.Sprintf("components.schemas.Ticket.properties.status.enum must include %s", v))
			}
		}
	}
}

func checkRejectDecisionRequestSchema(schema *yaml.Node, violations *[]string) {
	requireSchemaRequiredFields("RejectDecisionRequest", schema, []string{"reason"}, violations)
}

func checkApprovalDecisionRequestSchema(schema *yaml.Node, violations *[]string) {
	if _, ok := schemaProperty(schema, "selected_cluster_id"); !ok {
		*violations = append(*violations, "components.schemas.ApprovalDecisionRequest.properties.selected_cluster_id is missing")
	}
}

func checkAuditLogListSchema(schema *yaml.Node, violations *[]string) {
	properties, ok := mapValue(schema, "properties")
	if !ok {
		*violations = append(*violations, "components.schemas.AuditLogList must define properties")
		return
	}

	items, ok := mapValue(properties, "items")
	if !ok {
		*violations = append(*violations, "components.schemas.AuditLogList.properties.items is missing")
	} else {
		if typ, ok := scalarValueByKey(items, "type"); !ok || typ != "array" {
			*violations = append(*violations, "components.schemas.AuditLogList.properties.items.type must be array")
		}
		nestedItems, ok := mapValue(items, "items")
		if !ok {
			*violations = append(*violations, "components.schemas.AuditLogList.properties.items.items is missing")
		} else {
			ref, ok := scalarValueByKey(nestedItems, "$ref")
			if !ok || ref != "#/components/schemas/AuditLog" {
				*violations = append(*violations, "components.schemas.AuditLogList.properties.items.items.$ref must be '#/components/schemas/AuditLog'")
			}
		}
	}

	pagination, ok := mapValue(properties, "pagination")
	if !ok {
		*violations = append(*violations, "components.schemas.AuditLogList.properties.pagination is missing")
	} else {
		ref, ok := scalarValueByKey(pagination, "$ref")
		if !ok || ref != "#/components/schemas/Pagination" {
			*violations = append(*violations, "components.schemas.AuditLogList.properties.pagination.$ref must be '#/components/schemas/Pagination'")
		}
	}
}

func checkNotificationListSchema(schema *yaml.Node, violations *[]string) {
	properties, ok := mapValue(schema, "properties")
	if !ok {
		*violations = append(*violations, "components.schemas.NotificationList must define properties")
		return
	}

	items, ok := mapValue(properties, "items")
	if !ok {
		*violations = append(*violations, "components.schemas.NotificationList.properties.items is missing")
	} else {
		if typ, ok := scalarValueByKey(items, "type"); !ok || typ != "array" {
			*violations = append(*violations, "components.schemas.NotificationList.properties.items.type must be array")
		}
		nestedItems, ok := mapValue(items, "items")
		if !ok {
			*violations = append(*violations, "components.schemas.NotificationList.properties.items.items is missing")
		} else {
			ref, ok := scalarValueByKey(nestedItems, "$ref")
			if !ok || ref != "#/components/schemas/Notification" {
				*violations = append(*violations, "components.schemas.NotificationList.properties.items.items.$ref must be '#/components/schemas/Notification'")
			}
		}
	}

	pagination, ok := mapValue(properties, "pagination")
	if !ok {
		*violations = append(*violations, "components.schemas.NotificationList.properties.pagination is missing")
	} else {
		ref, ok := scalarValueByKey(pagination, "$ref")
		if !ok || ref != "#/components/schemas/Pagination" {
			*violations = append(*violations, "components.schemas.NotificationList.properties.pagination.$ref must be '#/components/schemas/Pagination'")
		}
	}
}

func checkUnreadCountSchema(schema *yaml.Node, violations *[]string) {
	properties, ok := mapValue(schema, "properties")
	if !ok {
		*violations = append(*violations, "components.schemas.UnreadCount must define properties")
		return
	}

	count, ok := mapValue(properties, "count")
	if !ok {
		*violations = append(*violations, "components.schemas.UnreadCount.properties.count is missing")
		return
	}

	typ, ok := scalarValueByKey(count, "type")
	if !ok || typ != "integer" {
		*violations = append(*violations, "components.schemas.UnreadCount.properties.count.type must be integer")
	}
}

func requireSchemaRequiredFields(schemaName string, schema *yaml.Node, fields []string, violations *[]string) {
	required, ok := mapValue(schema, "required")
	if !ok {
		*violations = append(*violations, fmt.Sprintf("components.schemas.%s.required is missing", schemaName))
		return
	}
	for _, field := range fields {
		if !sequenceContainsScalar(required, field) {
			*violations = append(*violations, fmt.Sprintf("components.schemas.%s.required must include %q", schemaName, field))
		}
	}
}

func schemaProperty(schema *yaml.Node, name string) (*yaml.Node, bool) {
	properties, ok := mapValue(schema, "properties")
	if !ok {
		return nil, false
	}
	return mapValue(properties, name)
}

func enumContains(property *yaml.Node, value string) bool {
	enumNode, ok := mapValue(property, "enum")
	if !ok {
		return false
	}
	return sequenceContainsScalar(enumNode, value)
}

func operationRequestSchemaRef(operationNode *yaml.Node) (string, bool) {
	requestBody, ok := mapValue(operationNode, "requestBody")
	if !ok {
		return "", false
	}
	content, ok := mapValue(requestBody, "content")
	if !ok {
		return "", false
	}
	jsonContent, ok := mapValue(content, "application/json")
	if !ok {
		return "", false
	}
	schema, ok := mapValue(jsonContent, "schema")
	if !ok {
		return "", false
	}
	return scalarValueByKey(schema, "$ref")
}

func responseSchemaRef(responseNode *yaml.Node) (string, bool) {
	content, ok := mapValue(responseNode, "content")
	if !ok {
		return "", false
	}
	jsonContent, ok := mapValue(content, "application/json")
	if !ok {
		return "", false
	}
	schema, ok := mapValue(jsonContent, "schema")
	if !ok {
		return "", false
	}
	return scalarValueByKey(schema, "$ref")
}

func documentRoot(doc *yaml.Node) *yaml.Node {
	if doc == nil {
		return nil
	}
	if doc.Kind == yaml.DocumentNode && len(doc.Content) > 0 {
		return doc.Content[0]
	}
	return doc
}

func mapValue(node *yaml.Node, key string) (*yaml.Node, bool) {
	node = documentRoot(node)
	if node == nil || node.Kind != yaml.MappingNode {
		return nil, false
	}

	for i := 0; i+1 < len(node.Content); i += 2 {
		k := node.Content[i]
		v := node.Content[i+1]
		if k.Kind == yaml.ScalarNode && k.Value == key {
			return v, true
		}
	}
	return nil, false
}

func scalarValueByKey(node *yaml.Node, key string) (string, bool) {
	v, ok := mapValue(node, key)
	if !ok || v == nil || v.Kind != yaml.ScalarNode {
		return "", false
	}
	return v.Value, true
}

func sequenceContainsMapKey(node *yaml.Node, key string) bool {
	node = documentRoot(node)
	if node == nil || node.Kind != yaml.SequenceNode {
		return false
	}
	for _, item := range node.Content {
		if _, ok := mapValue(item, key); ok {
			return true
		}
	}
	return false
}

func sequenceContainsScalar(node *yaml.Node, value string) bool {
	node = documentRoot(node)
	if node == nil || node.Kind != yaml.SequenceNode {
		return false
	}
	for _, item := range node.Content {
		if item.Kind == yaml.ScalarNode && item.Value == value {
			return true
		}
	}
	return false
}
