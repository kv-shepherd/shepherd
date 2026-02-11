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
