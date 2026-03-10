package service

import (
	"testing"

	"github.com/google/uuid"

	"kv-shepherd.io/shepherd/ent/approvalpolicy"
	"kv-shepherd.io/shepherd/ent/namespaceregistry"
	"kv-shepherd.io/shepherd/internal/testutil"
)

func TestApprovalRequirementService_UsesDefaultMatrixWhenNoPolicyExists(t *testing.T) {
	t.Parallel()

	client := testutil.OpenEntPostgres(t, "approval_requirement_default_matrix")
	svc := NewApprovalRequirementService(client)

	got, err := svc.RequiresApproval(t.Context(), approvalpolicy.OperationSTART_VM, namespaceregistry.EnvironmentTest)
	if err != nil {
		t.Fatalf("RequiresApproval(test/start) error = %v", err)
	}
	if got {
		t.Fatal("RequiresApproval(test/start) = true, want false")
	}

	got, err = svc.RequiresApproval(t.Context(), approvalpolicy.OperationSTART_VM, namespaceregistry.EnvironmentProd)
	if err != nil {
		t.Fatalf("RequiresApproval(prod/start) error = %v", err)
	}
	if !got {
		t.Fatal("RequiresApproval(prod/start) = false, want true")
	}

	got, err = svc.RequiresApproval(t.Context(), approvalpolicy.OperationCREATE_VM, namespaceregistry.EnvironmentTest)
	if err != nil {
		t.Fatalf("RequiresApproval(test/create) error = %v", err)
	}
	if !got {
		t.Fatal("RequiresApproval(test/create) = false, want true")
	}
}

func TestApprovalRequirementService_PrefersExplicitPolicyByPriority(t *testing.T) {
	t.Parallel()

	client := testutil.OpenEntPostgres(t, "approval_requirement_priority")
	svc := NewApprovalRequirementService(client)

	_, err := client.ApprovalPolicy.Create().
		SetID("policy-" + uuid.NewString()).
		SetName("prod-start-default").
		SetEnvironmentType(approvalpolicy.EnvironmentTypeAll).
		SetOperation(approvalpolicy.OperationSTART_VM).
		SetRequiresApproval(false).
		SetPriority(200).
		SetCreatedBy("seed").
		Save(t.Context())
	if err != nil {
		t.Fatalf("create fallback policy: %v", err)
	}

	_, err = client.ApprovalPolicy.Create().
		SetID("policy-" + uuid.NewString()).
		SetName("prod-start-override").
		SetEnvironmentType(approvalpolicy.EnvironmentTypeProd).
		SetOperation(approvalpolicy.OperationSTART_VM).
		SetRequiresApproval(true).
		SetPriority(10).
		SetCreatedBy("seed").
		Save(t.Context())
	if err != nil {
		t.Fatalf("create override policy: %v", err)
	}

	got, err := svc.RequiresApproval(t.Context(), approvalpolicy.OperationSTART_VM, namespaceregistry.EnvironmentProd)
	if err != nil {
		t.Fatalf("RequiresApproval(prod/start) error = %v", err)
	}
	if !got {
		t.Fatal("RequiresApproval(prod/start) = false, want true from higher-priority explicit rule")
	}

	got, err = svc.RequiresApproval(t.Context(), approvalpolicy.OperationSTART_VM, namespaceregistry.EnvironmentTest)
	if err != nil {
		t.Fatalf("RequiresApproval(test/start) error = %v", err)
	}
	if got {
		t.Fatal("RequiresApproval(test/start) = true, want false from all-env rule")
	}
}

func TestApprovalRequirementService_RejectsUnsupportedEnvironment(t *testing.T) {
	t.Parallel()

	client := testutil.OpenEntPostgres(t, "approval_requirement_invalid_env")
	svc := NewApprovalRequirementService(client)

	if _, err := svc.RequiresApproval(t.Context(), approvalpolicy.OperationSTOP_VM, namespaceregistry.Environment("staging")); err == nil {
		t.Fatal("RequiresApproval(staging/stop) expected error, got nil")
	}
}
