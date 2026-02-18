//go:build ignore

package main

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

const specPath = "api/openapi.yaml"

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

	checkOpenAPIVersion(root, &violations)
	checkGlobalSecurity(root, &violations)
	checkPathContracts(root, &violations)
	checkSchemaContracts(root, &violations)

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
			responses:        []requiredResponseContract{{code: "202", schemaRef: "#/components/schemas/ApprovalTicketResponse"}},
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
			path:        "/approvals",
			op:          "get",
			operationID: "listApprovals",
			responses:   []requiredResponseContract{{code: "200", schemaRef: "#/components/schemas/ApprovalTicketList"}},
		},
		{
			path:             "/approvals/{ticket_id}/approve",
			op:               "post",
			operationID:      "approveTicket",
			requestSchemaRef: "#/components/schemas/ApprovalDecisionRequest",
			responses:        []requiredResponseContract{{code: "204", noContent: true}},
		},
		{
			path:             "/approvals/{ticket_id}/reject",
			op:               "post",
			operationID:      "rejectTicket",
			requestSchemaRef: "#/components/schemas/RejectDecisionRequest",
			responses:        []requiredResponseContract{{code: "204", noContent: true}},
		},
		{
			path:        "/approvals/{ticket_id}/cancel",
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

	checkAuthSecurityContracts(paths, violations)
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
		"ApprovalTicketResponse",
		"DeleteVMResponse",
		"ApprovalTicket",
		"ApprovalTicketList",
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
	if schema, ok := mapValue(schemas, "ApprovalTicketResponse"); ok {
		checkApprovalTicketResponseSchema(schema, violations)
	}
	if schema, ok := mapValue(schemas, "DeleteVMResponse"); ok {
		checkDeleteVMResponseSchema(schema, violations)
	}
	if schema, ok := mapValue(schemas, "ApprovalTicket"); ok {
		checkApprovalTicketSchema(schema, violations)
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

func checkApprovalTicketResponseSchema(schema *yaml.Node, violations *[]string) {
	requireSchemaRequiredFields("ApprovalTicketResponse", schema, []string{"ticket_id", "status"}, violations)

	status, ok := schemaProperty(schema, "status")
	if !ok {
		*violations = append(*violations, "components.schemas.ApprovalTicketResponse.properties.status is missing")
		return
	}
	if !enumContains(status, "PENDING") {
		*violations = append(*violations, "components.schemas.ApprovalTicketResponse.properties.status.enum must include PENDING")
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

func checkApprovalTicketSchema(schema *yaml.Node, violations *[]string) {
	requireSchemaRequiredFields("ApprovalTicket", schema, []string{"id", "event_id", "status", "requester"}, violations)

	operationType, ok := schemaProperty(schema, "operation_type")
	if !ok {
		*violations = append(*violations, "components.schemas.ApprovalTicket.properties.operation_type is missing")
	} else {
		for _, v := range []string{"CREATE", "DELETE"} {
			if !enumContains(operationType, v) {
				*violations = append(*violations, fmt.Sprintf("components.schemas.ApprovalTicket.properties.operation_type.enum must include %s", v))
			}
		}
	}

	status, ok := schemaProperty(schema, "status")
	if !ok {
		*violations = append(*violations, "components.schemas.ApprovalTicket.properties.status is missing")
	} else {
		for _, v := range []string{"PENDING", "APPROVED", "REJECTED", "CANCELLED", "EXECUTING", "SUCCESS", "FAILED"} {
			if !enumContains(status, v) {
				*violations = append(*violations, fmt.Sprintf("components.schemas.ApprovalTicket.properties.status.enum must include %s", v))
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
