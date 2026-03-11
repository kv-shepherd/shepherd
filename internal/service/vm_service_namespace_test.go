package service

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"kv-shepherd.io/shepherd/internal/domain"
	"kv-shepherd.io/shepherd/internal/pkg/logger"
	"kv-shepherd.io/shepherd/internal/provider"
)

func init() {
	_ = logger.Init("error", "json")
}

type namespaceProvisioningProviderStub struct {
	ensureCalls   int
	validateCalls int
	createCalls   int
	lastCluster   string
	lastNamespace string
	ensureErr     error
}

func (s *namespaceProvisioningProviderStub) Name() string { return "stub" }
func (s *namespaceProvisioningProviderStub) Type() string { return "stub" }

func (s *namespaceProvisioningProviderStub) EnsureNamespace(_ context.Context, cluster, namespace string) error {
	s.ensureCalls++
	s.lastCluster = cluster
	s.lastNamespace = namespace
	return s.ensureErr
}

func (s *namespaceProvisioningProviderStub) GetVM(context.Context, string, string, string) (*domain.VM, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *namespaceProvisioningProviderStub) ListVMs(context.Context, string, string, provider.ListOptions) (*domain.VMList, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *namespaceProvisioningProviderStub) CreateVM(_ context.Context, _, namespace string, spec *domain.VMSpec) (*domain.VM, error) {
	s.createCalls++
	return &domain.VM{Name: spec.Name, Namespace: namespace, Spec: *spec}, nil
}

func (s *namespaceProvisioningProviderStub) UpdateVM(context.Context, string, string, string, *domain.VMSpec) (*domain.VM, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *namespaceProvisioningProviderStub) DeleteVM(context.Context, string, string, string) error {
	return fmt.Errorf("not implemented")
}

func (s *namespaceProvisioningProviderStub) StartVM(context.Context, string, string, string) error {
	return fmt.Errorf("not implemented")
}

func (s *namespaceProvisioningProviderStub) StopVM(context.Context, string, string, string) error {
	return fmt.Errorf("not implemented")
}

func (s *namespaceProvisioningProviderStub) RestartVM(context.Context, string, string, string) error {
	return fmt.Errorf("not implemented")
}

func (s *namespaceProvisioningProviderStub) PauseVM(context.Context, string, string, string) error {
	return fmt.Errorf("not implemented")
}

func (s *namespaceProvisioningProviderStub) UnpauseVM(context.Context, string, string, string) error {
	return fmt.Errorf("not implemented")
}

func (s *namespaceProvisioningProviderStub) ValidateSpec(_ context.Context, _, _ string, _ *domain.VMSpec) (*domain.ValidationResult, error) {
	s.validateCalls++
	return &domain.ValidationResult{Valid: true}, nil
}

func TestVMServiceValidateAndPrepare_EnsuresNamespaceFirst(t *testing.T) {
	t.Parallel()

	infra := &namespaceProvisioningProviderStub{}
	svc := NewVMService(infra)

	spec := &domain.VMSpec{
		Name:     "vm-a",
		CPU:      2,
		MemoryGi: 4,
		DiskGB:   50,
		Image:    "docker://quay.io/containerdisks/ubuntu:22.04",
	}

	result, err := svc.ValidateAndPrepare(t.Context(), "cluster-a", "team-a", spec)
	if err != nil {
		t.Fatalf("ValidateAndPrepare() error = %v", err)
	}
	if result == nil || !result.Valid {
		t.Fatalf("ValidateAndPrepare() result = %#v, want valid", result)
	}
	if infra.ensureCalls != 1 {
		t.Fatalf("EnsureNamespace calls = %d, want 1", infra.ensureCalls)
	}
	if infra.validateCalls != 1 {
		t.Fatalf("ValidateSpec calls = %d, want 1", infra.validateCalls)
	}
	if infra.lastCluster != "cluster-a" || infra.lastNamespace != "team-a" {
		t.Fatalf("EnsureNamespace args = (%q,%q), want (%q,%q)", infra.lastCluster, infra.lastNamespace, "cluster-a", "team-a")
	}
}

func TestVMServiceExecuteK8sCreate_EnsuresNamespaceFirst(t *testing.T) {
	t.Parallel()

	infra := &namespaceProvisioningProviderStub{}
	svc := NewVMService(infra)

	spec := &domain.VMSpec{
		Name:     "vm-b",
		CPU:      2,
		MemoryGi: 4,
		DiskGB:   50,
		Image:    "docker://quay.io/containerdisks/ubuntu:22.04",
	}

	vm, err := svc.ExecuteK8sCreate(t.Context(), "cluster-b", "team-b", spec)
	if err != nil {
		t.Fatalf("ExecuteK8sCreate() error = %v", err)
	}
	if vm == nil || vm.Namespace != "team-b" {
		t.Fatalf("ExecuteK8sCreate() vm = %#v, want namespace team-b", vm)
	}
	if infra.ensureCalls != 1 {
		t.Fatalf("EnsureNamespace calls = %d, want 1", infra.ensureCalls)
	}
	if infra.createCalls != 1 {
		t.Fatalf("CreateVM calls = %d, want 1", infra.createCalls)
	}
}

func TestVMServiceValidateAndPrepare_PropagatesNamespaceProvisioningError(t *testing.T) {
	t.Parallel()

	infra := &namespaceProvisioningProviderStub{ensureErr: fmt.Errorf("forbidden")}
	svc := NewVMService(infra)

	_, err := svc.ValidateAndPrepare(t.Context(), "cluster-a", "team-a", &domain.VMSpec{
		Name:     "vm-c",
		CPU:      2,
		MemoryGi: 4,
		DiskGB:   50,
		Image:    "docker://quay.io/containerdisks/ubuntu:22.04",
	})
	if err == nil {
		t.Fatal("ValidateAndPrepare() expected namespace provisioning error, got nil")
	}
	if got := err.Error(); got == "" || !containsAll(got, "ensure namespace team-a", "forbidden") {
		t.Fatalf("ValidateAndPrepare() error = %q, want namespace provisioning context", got)
	}
}

func containsAll(input string, parts ...string) bool {
	for _, part := range parts {
		if !strings.Contains(input, part) {
			return false
		}
	}
	return true
}
