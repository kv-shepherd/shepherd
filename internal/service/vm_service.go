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
	"net"
	"strings"

	"go.uber.org/zap"
	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"kv-shepherd.io/shepherd/internal/domain"
	apperrors "kv-shepherd.io/shepherd/internal/pkg/errors"
	"kv-shepherd.io/shepherd/internal/pkg/logger"
	"kv-shepherd.io/shepherd/internal/provider"
	infracontract "kv-shepherd.io/shepherd/internal/provider/infracontract"
)

// VMService handles VM business logic.
// Depends on narrow interfaces (ADR-0024), not monolithic KubeVirtProvider.
type VMService struct {
	infra infracontract.InfrastructureProvider
}

// NewVMService creates a new VMService.
func NewVMService(infra infracontract.InfrastructureProvider) *VMService {
	return &VMService{infra: infra}
}

func (s *VMService) infrastructureProvider() (infracontract.InfrastructureProvider, error) {
	if s == nil || s.infra == nil {
		return nil, fmt.Errorf("vm infrastructure provider is not configured")
	}
	return s.infra, nil
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
	infra, err := s.infrastructureProvider()
	if err != nil {
		return nil, err
	}
	vm, err := infra.GetVM(ctx, cluster, namespace, name)
	if err != nil {
		return nil, fmt.Errorf("get vm: %w", err)
	}
	return vm, nil
}

func (s *VMService) GetVMManifestYAML(ctx context.Context, cluster, namespace, name string) (string, error) {
	infra, err := s.infrastructureProvider()
	if err != nil {
		return "", err
	}
	manifestProvider, ok := infra.(infracontract.VMManifestProvider)
	if !ok {
		return "", fmt.Errorf("vm infrastructure provider does not expose manifest queries")
	}
	manifestYAML, err := manifestProvider.GetVMManifestYAML(ctx, cluster, namespace, name)
	if err != nil {
		return "", fmt.Errorf("get vm manifest yaml: %w", err)
	}
	return manifestYAML, nil
}

// ListVMs lists VMs with filtering.
func (s *VMService) ListVMs(ctx context.Context, cluster, namespace string, opts infracontract.ListOptions) (*domain.VMList, error) {
	infra, err := s.infrastructureProvider()
	if err != nil {
		return nil, err
	}
	list, err := infra.ListVMs(ctx, cluster, namespace, opts)
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
	if renderErr := ensureRenderedYAML(namespace, spec); renderErr != nil {
		return nil, fmt.Errorf("render vm yaml: %w", renderErr)
	}
	vm, err := s.infra.CreateVM(ctx, cluster, namespace, spec)
	if err != nil {
		if apierrors.IsAlreadyExists(err) {
			existing, lookupErr := s.existingVMForAlreadyExistsCreate(ctx, cluster, namespace, spec)
			if lookupErr == nil {
				logger.Info("VM already exists on K8s, treating create as idempotent",
					zap.String("cluster", cluster),
					zap.String("namespace", namespace),
					zap.String("name", existing.Name),
				)
				return existing, nil
			}
			logger.Error("K8s VM already exists but ownership verification failed",
				zap.String("cluster", cluster),
				zap.String("namespace", namespace),
				zap.Error(lookupErr),
			)
			return nil, fmt.Errorf("execute k8s create: %w; verify existing vm: %w", err, lookupErr)
		}
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

func (s *VMService) existingVMForAlreadyExistsCreate(
	ctx context.Context,
	cluster, namespace string,
	spec *domain.VMSpec,
) (*domain.VM, error) {
	if spec == nil {
		return nil, fmt.Errorf("requested vm spec is nil")
	}
	name := strings.TrimSpace(spec.Name)
	if name == "" {
		return nil, fmt.Errorf("requested vm name is empty")
	}

	infra, err := s.infrastructureProvider()
	if err != nil {
		return nil, err
	}
	existing, err := infra.GetVM(ctx, cluster, namespace, name)
	if err != nil {
		return nil, fmt.Errorf("get existing vm %s/%s: %w", namespace, name, err)
	}
	if existing == nil {
		return nil, fmt.Errorf("get existing vm %s/%s returned nil", namespace, name)
	}

	requestedEventID := strings.TrimSpace(spec.Labels[domain.ShepherdEventIDLabel])
	if requestedEventID == "" {
		return nil, fmt.Errorf("requested vm %s/%s has no %s label", namespace, name, domain.ShepherdEventIDLabel)
	}
	existingEventID := strings.TrimSpace(existing.Spec.Labels[domain.ShepherdEventIDLabel])
	if existingEventID != requestedEventID {
		return nil, fmt.Errorf(
			"existing vm %s/%s %s label %q does not match requested %q",
			namespace,
			name,
			domain.ShepherdEventIDLabel,
			existingEventID,
			requestedEventID,
		)
	}

	return existing, nil
}

// ExecuteK8sUpdate applies a narrow SSA patch or full desired spec to an existing VM.
func (s *VMService) ExecuteK8sUpdate(ctx context.Context, cluster, namespace, name string, spec *domain.VMSpec) (*domain.VM, error) {
	if spec == nil {
		return nil, fmt.Errorf("update vm: spec is nil")
	}
	if namespace == "" {
		return nil, fmt.Errorf("update vm: namespace is required")
	}
	if name == "" {
		return nil, fmt.Errorf("update vm: name is required")
	}
	infra, err := s.infrastructureProvider()
	if err != nil {
		return nil, err
	}
	if spec.Name == "" {
		spec.Name = name
	}
	if renderErr := ensureRenderedYAML(namespace, spec); renderErr != nil {
		return nil, fmt.Errorf("render vm yaml: %w", renderErr)
	}
	vm, err := infra.UpdateVM(ctx, cluster, namespace, name, spec)
	if err != nil {
		logger.Error("K8s VM update failed",
			zap.String("cluster", cluster),
			zap.String("namespace", namespace),
			zap.String("name", name),
			zap.Error(err),
		)
		return nil, fmt.Errorf("execute k8s update: %w", err)
	}
	return vm, nil
}

func (s *VMService) DryRunVMMutation(ctx context.Context, cluster, namespace, name string, mutation *domain.VMMutation) error {
	if mutation == nil {
		return fmt.Errorf("vm mutation is nil")
	}
	if namespace == "" {
		return fmt.Errorf("vm mutation: namespace is required")
	}
	if name == "" {
		return fmt.Errorf("vm mutation: name is required")
	}
	infra, err := s.infrastructureProvider()
	if err != nil {
		return err
	}
	mutator, ok := infra.(infracontract.VMMutationProvider)
	if !ok {
		return fmt.Errorf("vm infrastructure provider does not support vm mutation dry-run")
	}
	if err := mutator.DryRunVMMutation(ctx, cluster, namespace, name, mutation); err != nil {
		return fmt.Errorf("dry-run vm mutation: %w", err)
	}
	return nil
}

func (s *VMService) ExecuteVMMutation(ctx context.Context, cluster, namespace, name string, mutation *domain.VMMutation) (*domain.VM, error) {
	if mutation == nil {
		return nil, fmt.Errorf("vm mutation is nil")
	}
	if namespace == "" {
		return nil, fmt.Errorf("vm mutation: namespace is required")
	}
	if name == "" {
		return nil, fmt.Errorf("vm mutation: name is required")
	}
	infra, err := s.infrastructureProvider()
	if err != nil {
		return nil, err
	}
	mutator, ok := infra.(infracontract.VMMutationProvider)
	if !ok {
		return nil, fmt.Errorf("vm infrastructure provider does not support vm mutation execution")
	}
	vm, err := mutator.ExecuteVMMutation(ctx, cluster, namespace, name, mutation)
	if err != nil {
		logger.Error("KubeVirt VM mutation failed",
			zap.String("cluster", cluster),
			zap.String("namespace", namespace),
			zap.String("name", name),
			zap.Error(err),
		)
		return nil, fmt.Errorf("execute vm mutation: %w", err)
	}
	return vm, nil
}

// GetStorageProfile returns the CDI StorageProfile for a target storage class.
// It is used by approval-time root-volume resolution and clone advisories.
func (s *VMService) GetStorageProfile(ctx context.Context, cluster, name string) (*domain.StorageProfile, error) {
	infra, err := s.infrastructureProvider()
	if err != nil {
		return nil, err
	}
	query, ok := infra.(infracontract.ProvisioningQueryProvider)
	if !ok {
		return nil, fmt.Errorf("vm infrastructure provider does not expose storage profile queries")
	}
	return query.GetStorageProfile(ctx, cluster, name)
}

func (s *VMService) ensureNamespaceReady(ctx context.Context, cluster, namespace string) error {
	infra, err := s.infrastructureProvider()
	if err != nil {
		return err
	}
	provisioner, ok := infra.(infracontract.NamespaceProvisioner)
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
	infra, err := s.infrastructureProvider()
	if err != nil {
		return err
	}
	return infra.StartVM(ctx, cluster, namespace, name)
}

// StopVM stops a VM.
func (s *VMService) StopVM(ctx context.Context, cluster, namespace, name string) error {
	infra, err := s.infrastructureProvider()
	if err != nil {
		return err
	}
	return infra.StopVM(ctx, cluster, namespace, name)
}

// RestartVM restarts a VM.
func (s *VMService) RestartVM(ctx context.Context, cluster, namespace, name string) error {
	infra, err := s.infrastructureProvider()
	if err != nil {
		return err
	}
	return infra.RestartVM(ctx, cluster, namespace, name)
}

// DeleteVM deletes a VM.
func (s *VMService) DeleteVM(ctx context.Context, cluster, namespace, name string) error {
	infra, err := s.infrastructureProvider()
	if err != nil {
		return err
	}
	return infra.DeleteVM(ctx, cluster, namespace, name)
}

// OpenVNCStream returns a raw VNC stream for the target VM when the provider supports it.
func (s *VMService) OpenVNCStream(ctx context.Context, cluster, namespace, name string) (net.Conn, error) {
	infra, err := s.infrastructureProvider()
	if err != nil {
		return nil, err
	}
	console, ok := infra.(infracontract.VNCStreamProvider)
	if !ok {
		return nil, fmt.Errorf("vm infrastructure provider does not support vnc streaming")
	}
	return console.OpenVNCStream(ctx, cluster, namespace, name)
}

// OpenSerialConsoleStream returns a raw serial console stream for the target VM when the provider supports it.
func (s *VMService) OpenSerialConsoleStream(ctx context.Context, cluster, namespace, name string) (net.Conn, error) {
	infra, err := s.infrastructureProvider()
	if err != nil {
		return nil, err
	}
	console, ok := infra.(infracontract.SerialConsoleStreamProvider)
	if !ok {
		return nil, fmt.Errorf("vm infrastructure provider does not support serial console streaming")
	}
	return console.OpenSerialConsoleStream(ctx, cluster, namespace, name)
}
