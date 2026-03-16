// Package service provides business logic services for KubeVirt Shepherd.
//
// ADR-0012: Service layer must NOT directly manage transactions.
// Service receives *ent.Client parameter (in-transaction or not).
// K8s API calls are FORBIDDEN inside transactions.
//
// Import Path (ADR-0016): kv-shepherd.io/shepherd/internal/service
package service

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"kv-shepherd.io/shepherd/internal/domain"
	apperrors "kv-shepherd.io/shepherd/internal/pkg/errors"
	"kv-shepherd.io/shepherd/internal/pkg/logger"
	"kv-shepherd.io/shepherd/internal/provider"
)

// VMService handles VM business logic.
// Depends on narrow interfaces (ADR-0024), not monolithic KubeVirtProvider.
type VMService struct {
	infra provider.InfrastructureProvider
}

// NewVMService creates a new VMService.
func NewVMService(infra provider.InfrastructureProvider) *VMService {
	return &VMService{infra: infra}
}

// ValidateAndPrepare validates a VM creation request (outside transaction).
// Returns prepared spec or validation error.
func (s *VMService) ValidateAndPrepare(ctx context.Context, cluster, namespace string, spec *domain.VMSpec) (*domain.ValidationResult, error) {
	if spec == nil {
		return nil, apperrors.BadRequest(apperrors.CodeValidationFailed, "spec is required")
	}
	if err := s.ensureNamespaceReady(ctx, cluster, namespace); err != nil {
		return nil, err
	}
	if err := ensureRenderedYAML(namespace, spec); err != nil {
		return nil, apperrors.BadRequest(apperrors.CodeValidationFailed, fmt.Sprintf("render vm yaml: %v", err))
	}

	result, err := s.infra.ValidateSpec(ctx, cluster, namespace, spec)
	if err != nil {
		return nil, fmt.Errorf("validate spec: %w", err)
	}

	return result, nil
}

// GetVM retrieves a VM.
func (s *VMService) GetVM(ctx context.Context, cluster, namespace, name string) (*domain.VM, error) {
	vm, err := s.infra.GetVM(ctx, cluster, namespace, name)
	if err != nil {
		return nil, fmt.Errorf("get vm: %w", err)
	}
	return vm, nil
}

// ListVMs lists VMs with filtering.
func (s *VMService) ListVMs(ctx context.Context, cluster, namespace string, opts provider.ListOptions) (*domain.VMList, error) {
	list, err := s.infra.ListVMs(ctx, cluster, namespace, opts)
	if err != nil {
		return nil, fmt.Errorf("list vms: %w", err)
	}
	return list, nil
}

// ExecuteK8sCreate creates the VM on K8s (outside transaction).
// Idempotent: handles AlreadyExists error gracefully.
func (s *VMService) ExecuteK8sCreate(ctx context.Context, cluster, namespace string, spec *domain.VMSpec) (*domain.VM, error) {
	if err := s.ensureNamespaceReady(ctx, cluster, namespace); err != nil {
		return nil, err
	}
	if err := ensureRenderedYAML(namespace, spec); err != nil {
		return nil, fmt.Errorf("render vm yaml: %w", err)
	}
	vm, err := s.infra.CreateVM(ctx, cluster, namespace, spec)
	if err != nil {
		logger.Error("K8s VM creation failed",
			zap.String("cluster", cluster),
			zap.String("namespace", namespace),
			zap.Error(err),
		)
		return nil, fmt.Errorf("execute k8s create: %w", err)
	}

	logger.Info("VM created on K8s",
		zap.String("cluster", cluster),
		zap.String("namespace", namespace),
		zap.String("name", vm.Name),
	)
	return vm, nil
}

// GetStorageProfile returns the CDI StorageProfile for a target storage class.
// It is used by approval-time root-volume resolution and clone advisories.
func (s *VMService) GetStorageProfile(ctx context.Context, cluster, name string) (*domain.StorageProfile, error) {
	if s == nil || s.infra == nil {
		return nil, fmt.Errorf("vm infrastructure provider is not configured")
	}
	query, ok := s.infra.(provider.ProvisioningQueryProvider)
	if !ok {
		return nil, fmt.Errorf("vm infrastructure provider does not expose storage profile queries")
	}
	return query.GetStorageProfile(ctx, cluster, name)
}

func (s *VMService) ensureNamespaceReady(ctx context.Context, cluster, namespace string) error {
	if s == nil || s.infra == nil {
		return fmt.Errorf("vm infrastructure provider is not configured")
	}
	provisioner, ok := s.infra.(provider.NamespaceProvisioner)
	if !ok {
		return fmt.Errorf("vm infrastructure provider does not support namespace provisioning")
	}
	if err := provisioner.EnsureNamespace(ctx, cluster, namespace); err != nil {
		return fmt.Errorf("ensure namespace %s on cluster %s: %w", namespace, cluster, err)
	}
	return nil
}

// ensureRenderedYAML renders spec.RenderedYAML when absent, keeping the provider
// layer strictly focused on SSA submission.
func ensureRenderedYAML(namespace string, spec *domain.VMSpec) error {
	if spec == nil {
		return fmt.Errorf("spec is nil")
	}
	if spec.RenderedYAML != "" {
		return nil
	}
	rendered, err := provider.RenderVMSpecToYAML(namespace, &provider.VMRenderInput{
		Name:            spec.Name,
		CPUCores:        spec.CPU,
		MemoryGi:        spec.MemoryGi,
		DiskGB:          spec.DiskGB,
		Image:           spec.Image,
		StorageClass:    spec.StorageClass,
		CloudInit:       spec.CloudInit,
		Labels:          spec.Labels,
		CPURequest:      spec.CPURequest,
		MemoryRequestGi: spec.MemoryRequestGi,
		SpecOverrides:   spec.SpecOverrides,

		// DV storage mode (explicit — structural DV format change).
		DVAccessModes: spec.DVAccessModes,
		DVVolumeMode:  spec.DVVolumeMode,
	})
	if err != nil {
		return err
	}
	spec.RenderedYAML = rendered
	return nil
}

// StartVM starts a VM.
func (s *VMService) StartVM(ctx context.Context, cluster, namespace, name string) error {
	return s.infra.StartVM(ctx, cluster, namespace, name)
}

// StopVM stops a VM.
func (s *VMService) StopVM(ctx context.Context, cluster, namespace, name string) error {
	return s.infra.StopVM(ctx, cluster, namespace, name)
}

// RestartVM restarts a VM.
func (s *VMService) RestartVM(ctx context.Context, cluster, namespace, name string) error {
	return s.infra.RestartVM(ctx, cluster, namespace, name)
}

// DeleteVM deletes a VM.
func (s *VMService) DeleteVM(ctx context.Context, cluster, namespace, name string) error {
	return s.infra.DeleteVM(ctx, cluster, namespace, name)
}
