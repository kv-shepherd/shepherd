package usecase

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	"kv-shepherd.io/shepherd/ent/instancesize"
	"kv-shepherd.io/shepherd/ent/namespaceregistry"
	enttemplate "kv-shepherd.io/shepherd/ent/template"
	"kv-shepherd.io/shepherd/internal/domain"
	apperrors "kv-shepherd.io/shepherd/internal/pkg/errors"
	"kv-shepherd.io/shepherd/internal/service"
	"kv-shepherd.io/shepherd/internal/testutil"
)

func TestSameCreateResource(t *testing.T) {
	basePayload := domain.VMCreationPayload{
		ServiceID:      "svc-1",
		TemplateID:     "tpl-1",
		InstanceSizeID: "size-1",
		Namespace:      "team-a",
		TargetCPUCores: 2,
		TargetMemoryGi: 4,
		TargetDiskGB:   50,
	}

	testCases := []struct {
		name   string
		input  CreateVMInput
		expect bool
	}{
		{
			name: "same resource",
			input: CreateVMInput{
				ServiceID:      "svc-1",
				TemplateID:     "tpl-1",
				InstanceSizeID: "size-1",
				Namespace:      "team-a",
			},
			expect: true,
		},
		{
			name: "different target cpu",
			input: CreateVMInput{
				ServiceID:      "svc-1",
				TemplateID:     "tpl-1",
				InstanceSizeID: "size-1",
				Namespace:      "team-a",
				TargetCPUCores: float64Ptr(3),
			},
			expect: false,
		},
		{
			name: "different service",
			input: CreateVMInput{
				ServiceID:      "svc-2",
				TemplateID:     "tpl-1",
				InstanceSizeID: "size-1",
				Namespace:      "team-a",
			},
			expect: false,
		},
		{
			name: "different template",
			input: CreateVMInput{
				ServiceID:      "svc-1",
				TemplateID:     "tpl-2",
				InstanceSizeID: "size-1",
				Namespace:      "team-a",
			},
			expect: false,
		},
		{
			name: "different instance size",
			input: CreateVMInput{
				ServiceID:      "svc-1",
				TemplateID:     "tpl-1",
				InstanceSizeID: "size-2",
				Namespace:      "team-a",
			},
			expect: false,
		},
		{
			name: "different namespace",
			input: CreateVMInput{
				ServiceID:      "svc-1",
				TemplateID:     "tpl-1",
				InstanceSizeID: "size-1",
				Namespace:      "team-b",
			},
			expect: false,
		},
		{
			name: "whitespace normalized",
			input: CreateVMInput{
				ServiceID:      " svc-1 ",
				TemplateID:     "tpl-1",
				InstanceSizeID: "size-1",
				Namespace:      " team-a ",
			},
			expect: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			resolvedTargets := service.ResolveVMRequestTargets(
				2,
				0,
				4,
				0,
				50,
				service.VMRequestTargets{
					TargetCPUCores: tc.input.TargetCPUCores,
					TargetMemoryGi: tc.input.TargetMemoryGi,
					TargetDiskGB:   tc.input.TargetDiskGB,
				},
			)
			got := sameCreateResource(basePayload, tc.input, resolvedTargets)
			if got != tc.expect {
				t.Fatalf("sameCreateResource mismatch: got %v want %v", got, tc.expect)
			}
		})
	}
}

func TestSameCreateResource_LegacyPayloadWithoutTargetsMatchesDefaultRequest(t *testing.T) {
	legacyPayload := domain.VMCreationPayload{
		ServiceID:      "svc-1",
		TemplateID:     "tpl-1",
		InstanceSizeID: "size-1",
		Namespace:      "team-a",
	}
	resolvedTargets := service.ResolveVMRequestTargets(2, 0, 4, 0, 50, service.VMRequestTargets{})
	if !sameCreateResource(legacyPayload, CreateVMInput{
		ServiceID:      "svc-1",
		TemplateID:     "tpl-1",
		InstanceSizeID: "size-1",
		Namespace:      "team-a",
	}, resolvedTargets) {
		t.Fatal("legacy payload without target resources should match a default create request")
	}
}

func TestCreateVMUseCase_PersistsResolvedTargetResources(t *testing.T) {
	t.Parallel()

	client := testutil.OpenEntPostgres(t, "create_vm_target_resources")
	uc := NewCreateVMUseCase(
		client,
		nil,
		service.NewInstanceSizeService(client),
		service.NewTemplateService(client),
	)

	if _, err := client.NamespaceRegistry.Create().
		SetID("ns-test").
		SetName("team-test").
		SetEnvironment(namespaceregistry.EnvironmentTest).
		SetCreatedBy("seed").
		SetEnabled(true).
		Save(t.Context()); err != nil {
		t.Fatalf("seed namespace: %v", err)
	}
	if _, err := client.Template.Create().
		SetID("tpl-all").
		SetName("ubuntu-shared").
		SetCreatedBy("seed").
		SetCatalogScope(enttemplate.CatalogScopeAll).
		SetSourceType(service.TemplateSourceCDIImageImport).
		SetImageURL("docker://quay.io/containerdisks/ubuntu:22.04").
		SetEnabled(true).
		Save(t.Context()); err != nil {
		t.Fatalf("seed template: %v", err)
	}
	if _, err := client.InstanceSize.Create().
		SetID("size-test").
		SetName("test-size").
		SetCPUCores(4).
		SetCPURequest(3).
		SetMemoryGi(8).
		SetMemoryRequestGi(6).
		SetDiskGB(80).
		SetCreatedBy("seed").
		SetCatalogScope(instancesize.CatalogScopeTest).
		SetEnabled(true).
		Save(t.Context()); err != nil {
		t.Fatalf("seed instance size: %v", err)
	}

	out, err := uc.Execute(t.Context(), CreateVMInput{
		ServiceID:      "svc-1",
		TemplateID:     "tpl-all",
		InstanceSizeID: "size-test",
		Namespace:      "team-test",
		RequestedBy:    "user-1",
		TargetCPUCores: float64Ptr(2),
		TargetMemoryGi: float64Ptr(4),
		TargetDiskGB:   intPtr(120),
	})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}

	event, err := client.DomainEvent.Get(t.Context(), out.EventID)
	if err != nil {
		t.Fatalf("load domain event: %v", err)
	}
	var payload domain.VMCreationPayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload.TargetCPUCores != 2 {
		t.Fatalf("target_cpu_cores = %v, want 2", payload.TargetCPUCores)
	}
	if payload.TargetMemoryGi != 4 {
		t.Fatalf("target_memory_gi = %v, want 4", payload.TargetMemoryGi)
	}
	if payload.TargetDiskGB != 120 {
		t.Fatalf("target_disk_gb = %v, want 120", payload.TargetDiskGB)
	}
}

func float64Ptr(value float64) *float64 {
	return &value
}

func intPtr(value int) *int {
	return &value
}

func TestCreateVMUseCase_RejectsCatalogScopeMismatch(t *testing.T) {
	t.Parallel()

	client := testutil.OpenEntPostgres(t, "create_vm_scope_mismatch")
	uc := NewCreateVMUseCase(
		client,
		nil,
		service.NewInstanceSizeService(client),
		service.NewTemplateService(client),
	)

	if _, err := client.NamespaceRegistry.Create().
		SetID("ns-prod").
		SetName("team-prod").
		SetEnvironment(namespaceregistry.EnvironmentProd).
		SetCreatedBy("seed").
		SetEnabled(true).
		Save(t.Context()); err != nil {
		t.Fatalf("seed namespace: %v", err)
	}
	if _, err := client.Template.Create().
		SetID("tpl-test").
		SetName("ubuntu-test").
		SetCreatedBy("seed").
		SetCatalogScope(enttemplate.CatalogScopeTest).
		SetSourceType(service.TemplateSourceCDIImageImport).
		SetImageURL("docker://quay.io/containerdisks/ubuntu:22.04").
		SetEnabled(true).
		Save(t.Context()); err != nil {
		t.Fatalf("seed template: %v", err)
	}
	if _, err := client.InstanceSize.Create().
		SetID("size-prod").
		SetName("prod-size").
		SetCPUCores(2).
		SetCPURequest(2).
		SetMemoryGi(4).
		SetMemoryRequestGi(4).
		SetCreatedBy("seed").
		SetCatalogScope(instancesize.CatalogScopeProd).
		SetEnabled(true).
		Save(t.Context()); err != nil {
		t.Fatalf("seed instance size: %v", err)
	}

	_, err := uc.Execute(t.Context(), CreateVMInput{
		ServiceID:      "svc-1",
		TemplateID:     "tpl-test",
		InstanceSizeID: "size-prod",
		Namespace:      "team-prod",
		RequestedBy:    "user-1",
	})
	if err == nil {
		t.Fatal("expected catalog scope mismatch error")
	}
	appErr, ok := apperrors.IsAppError(err)
	if !ok {
		t.Fatalf("expected AppError, got %T", err)
	}
	if appErr.Code != "CATALOG_SCOPE_MISMATCH" {
		t.Fatalf("error code = %q, want %q", appErr.Code, "CATALOG_SCOPE_MISMATCH")
	}
}

func TestCreateVMUseCase_RejectsSystemLabelMismatch(t *testing.T) {
	t.Parallel()

	client := testutil.OpenEntPostgres(t, "create_vm_label_mismatch")
	uc := NewCreateVMUseCase(
		client,
		nil,
		service.NewInstanceSizeService(client),
		service.NewTemplateService(client),
	)

	if _, err := client.Template.Create().
		SetID("tpl-windows").
		SetName("windows-template").
		SetCreatedBy("seed").
		SetCatalogScope(enttemplate.CatalogScopeAll).
		SetSourceType(service.TemplateSourceCDIImageImport).
		SetImageURL("docker://registry.example.com/windows:2022").
		SetSystemLabels([]string{service.SystemLabelOSWindows}).
		SetEnabled(true).
		Save(t.Context()); err != nil {
		t.Fatalf("seed template: %v", err)
	}
	if _, err := client.InstanceSize.Create().
		SetID("size-linux").
		SetName("linux-size").
		SetCPUCores(4).
		SetCPURequest(4).
		SetMemoryGi(8).
		SetMemoryRequestGi(8).
		SetCreatedBy("seed").
		SetCatalogScope(instancesize.CatalogScopeAll).
		SetSystemLabels([]string{service.SystemLabelOSLinux}).
		SetEnabled(true).
		Save(t.Context()); err != nil {
		t.Fatalf("seed instance size: %v", err)
	}

	_, err := uc.Execute(t.Context(), CreateVMInput{
		ServiceID:      "svc-1",
		TemplateID:     "tpl-windows",
		InstanceSizeID: "size-linux",
		Namespace:      "team-prod",
		RequestedBy:    "user-1",
	})
	if err == nil {
		t.Fatal("expected system label mismatch error")
	}
	appErr, ok := apperrors.IsAppError(err)
	if !ok {
		t.Fatalf("expected AppError, got %T", err)
	}
	if appErr.Code != "TEMPLATE_INSTANCE_SIZE_LABEL_MISMATCH" {
		t.Fatalf("error code = %q, want TEMPLATE_INSTANCE_SIZE_LABEL_MISMATCH", appErr.Code)
	}
}

func TestCreateVMUseCase_AllScopeAcceptedForMatchingNamespace(t *testing.T) {
	t.Parallel()

	client := testutil.OpenEntPostgres(t, "create_vm_scope_success")
	uc := NewCreateVMUseCase(
		client,
		nil,
		service.NewInstanceSizeService(client),
		service.NewTemplateService(client),
	)

	if _, err := client.NamespaceRegistry.Create().
		SetID("ns-test").
		SetName("team-test").
		SetEnvironment(namespaceregistry.EnvironmentTest).
		SetCreatedBy("seed").
		SetEnabled(true).
		Save(t.Context()); err != nil {
		t.Fatalf("seed namespace: %v", err)
	}
	if _, err := client.Template.Create().
		SetID("tpl-all").
		SetName("ubuntu-shared").
		SetCreatedBy("seed").
		SetCatalogScope(enttemplate.CatalogScopeAll).
		SetSourceType(service.TemplateSourceCDIImageImport).
		SetImageURL("docker://quay.io/containerdisks/ubuntu:22.04").
		SetEnabled(true).
		Save(t.Context()); err != nil {
		t.Fatalf("seed template: %v", err)
	}
	if _, err := client.InstanceSize.Create().
		SetID("size-test").
		SetName("test-size").
		SetCPUCores(2).
		SetCPURequest(2).
		SetMemoryGi(4).
		SetMemoryRequestGi(4).
		SetCreatedBy("seed").
		SetCatalogScope(instancesize.CatalogScopeTest).
		SetEnabled(true).
		Save(t.Context()); err != nil {
		t.Fatalf("seed instance size: %v", err)
	}

	out, err := uc.Execute(t.Context(), CreateVMInput{
		ServiceID:      "svc-1",
		TemplateID:     "tpl-all",
		InstanceSizeID: "size-test",
		Namespace:      "team-test",
		RequestedBy:    "user-1",
	})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if out == nil || out.TicketID == "" || out.EventID == "" {
		t.Fatalf("unexpected output: %+v", out)
	}
}

func TestCreateVMUseCase_RejectsNonRequestableTemplateSource(t *testing.T) {
	t.Parallel()

	client := testutil.OpenEntPostgres(t, "create_vm_non_requestable_source")
	uc := NewCreateVMUseCase(
		client,
		nil,
		service.NewInstanceSizeService(client),
		service.NewTemplateService(client),
	)

	if _, err := client.NamespaceRegistry.Create().
		SetID("ns-test").
		SetName("team-test").
		SetEnvironment(namespaceregistry.EnvironmentTest).
		SetCreatedBy("seed").
		SetEnabled(true).
		Save(t.Context()); err != nil {
		t.Fatalf("seed namespace: %v", err)
	}
	if _, err := client.Template.Create().
		SetID("tpl-ephemeral").
		SetName("fedora-ephemeral").
		SetSourceType(service.TemplateSourceContainerDisk).
		SetImageURL("docker://quay.io/containerdisks/fedora:40").
		SetCreatedBy("seed").
		SetCatalogScope(enttemplate.CatalogScopeTest).
		SetEnabled(true).
		Save(t.Context()); err != nil {
		t.Fatalf("seed template: %v", err)
	}
	if _, err := client.InstanceSize.Create().
		SetID("size-test").
		SetName("test-size").
		SetCPUCores(2).
		SetCPURequest(2).
		SetMemoryGi(4).
		SetMemoryRequestGi(4).
		SetCreatedBy("seed").
		SetCatalogScope(instancesize.CatalogScopeTest).
		SetEnabled(true).
		Save(t.Context()); err != nil {
		t.Fatalf("seed instance size: %v", err)
	}

	_, err := uc.Execute(t.Context(), CreateVMInput{
		ServiceID:      "svc-1",
		TemplateID:     "tpl-ephemeral",
		InstanceSizeID: "size-test",
		Namespace:      "team-test",
		RequestedBy:    "user-1",
	})
	if err == nil {
		t.Fatal("expected non-requestable template source error")
	}
	appErr, ok := apperrors.IsAppError(err)
	if !ok {
		t.Fatalf("expected AppError, got %T", err)
	}
	if appErr.Code != "TEMPLATE_SOURCE_NOT_REQUESTABLE" {
		t.Fatalf("error code = %q, want %q", appErr.Code, "TEMPLATE_SOURCE_NOT_REQUESTABLE")
	}
}

func TestGenerateID_ReturnsValidUUID(t *testing.T) {
	t.Parallel()

	got := generateID()
	if got == "" {
		t.Fatal("generateID() returned empty string")
	}
	if _, err := uuid.Parse(got); err != nil {
		t.Fatalf("generateID() = %q, want valid UUID: %v", got, err)
	}
}
