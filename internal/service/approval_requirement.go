package service

import (
	"context"
	"fmt"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/ent/approvalpolicy"
	"kv-shepherd.io/shepherd/ent/namespaceregistry"
)

// ApprovalRequirementService decides whether an operation requires approval in
// a given namespace environment.
//
// It prefers explicit ApprovalPolicy rows and falls back to the accepted
// ADR-0015 baseline matrix when no policy row exists yet. The fallback keeps
// runtime behavior stable while seed/admin management catches up.
type ApprovalRequirementService struct {
	client *ent.Client
}

// NewApprovalRequirementService creates a new ApprovalRequirementService.
func NewApprovalRequirementService(client *ent.Client) *ApprovalRequirementService {
	return &ApprovalRequirementService{client: client}
}

// RequiresApproval returns whether the given operation requires approval in the
// specified namespace environment.
func (s *ApprovalRequirementService) RequiresApproval(
	ctx context.Context,
	operation approvalpolicy.Operation,
	env namespaceregistry.Environment,
) (bool, error) {
	if env != namespaceregistry.EnvironmentTest && env != namespaceregistry.EnvironmentProd {
		return false, fmt.Errorf("unsupported namespace environment %q", env)
	}
	if s == nil || s.client == nil {
		return defaultApprovalRequirement(operation, env)
	}

	rule, err := s.client.ApprovalPolicy.Query().
		Where(
			approvalpolicy.EnabledEQ(true),
			approvalpolicy.OperationEQ(operation),
			approvalpolicy.EnvironmentTypeIn(
				approvalpolicy.EnvironmentType(env),
				approvalpolicy.EnvironmentTypeAll,
			),
		).
		Order(ent.Asc(approvalpolicy.FieldPriority)).
		First(ctx)
	if err == nil {
		return rule.RequiresApproval, nil
	}
	if ent.IsNotFound(err) {
		return defaultApprovalRequirement(operation, env)
	}
	return false, err
}

func defaultApprovalRequirement(
	operation approvalpolicy.Operation,
	env namespaceregistry.Environment,
) (bool, error) {
	switch operation {
	case approvalpolicy.OperationSTART_VM,
		approvalpolicy.OperationSTOP_VM,
		approvalpolicy.OperationRESTART_VM,
		approvalpolicy.OperationVNC_ACCESS:
		return env == namespaceregistry.EnvironmentProd, nil
	case approvalpolicy.OperationCREATE_VM,
		approvalpolicy.OperationMODIFY_VM,
		approvalpolicy.OperationDELETE_VM:
		return true, nil
	default:
		return false, fmt.Errorf("unsupported approval policy operation %q", operation)
	}
}
