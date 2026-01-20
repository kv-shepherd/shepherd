// Package provider defines the infrastructure provider interfaces.
//
// This is the Anti-Corruption Layer between the platform and KubeVirt.
// All provider methods return domain types, not K8s types.
package provider

import (
	"context"

	"github.com/CloudPasture/kubevirt-shepherd/internal/domain"
)

// InfrastructureProvider is the base interface for all infrastructure providers.
// Supports VM lifecycle, snapshots, clones, and migrations.
type InfrastructureProvider interface {
	// Metadata
	Name() string
	Type() string

	// VM Lifecycle
	GetVM(ctx context.Context, cluster, namespace, name string) (*domain.VM, error)
	ListVMs(ctx context.Context, cluster, namespace string, opts ListOptions) (*domain.VMList, error)
	CreateVM(ctx context.Context, cluster, namespace string, spec *domain.VMSpec) (*domain.VM, error)
	UpdateVM(ctx context.Context, cluster, namespace, name string, spec *domain.VMSpec) (*domain.VM, error)
	DeleteVM(ctx context.Context, cluster, namespace, name string) error

	// VM Power Operations
	StartVM(ctx context.Context, cluster, namespace, name string) error
	StopVM(ctx context.Context, cluster, namespace, name string) error
	RestartVM(ctx context.Context, cluster, namespace, name string) error
	PauseVM(ctx context.Context, cluster, namespace, name string) error
	UnpauseVM(ctx context.Context, cluster, namespace, name string) error

	// Dry Run Validation (ADR-0011)
	ValidateSpec(ctx context.Context, cluster, namespace string, spec *domain.VMSpec) (*domain.ValidationResult, error)
}

// SnapshotProvider provides snapshot capabilities.
type SnapshotProvider interface {
	CreateSnapshot(ctx context.Context, cluster, namespace, vmName, snapshotName string) (*domain.Snapshot, error)
	GetSnapshot(ctx context.Context, cluster, namespace, name string) (*domain.Snapshot, error)
	ListSnapshots(ctx context.Context, cluster, namespace, vmName string) ([]*domain.Snapshot, error)
	DeleteSnapshot(ctx context.Context, cluster, namespace, name string) error
	RestoreFromSnapshot(ctx context.Context, cluster, namespace, snapshotName, targetVMName string) (*domain.VM, error)
}

// CloneProvider provides clone capabilities.
type CloneProvider interface {
	CloneVM(ctx context.Context, cluster, namespace, sourceVM, targetName string) (*domain.VM, error)
	CloneFromSnapshot(ctx context.Context, cluster, namespace, snapshotName, targetName string) (*domain.VM, error)
	GetClone(ctx context.Context, cluster, namespace, name string) (*domain.Clone, error)
	ListClones(ctx context.Context, cluster, namespace string) ([]*domain.Clone, error)
}

// MigrationProvider provides live migration capabilities.
type MigrationProvider interface {
	MigrateVM(ctx context.Context, cluster, namespace, name string) (*domain.Migration, error)
	GetMigration(ctx context.Context, cluster, namespace, name string) (*domain.Migration, error)
	ListMigrations(ctx context.Context, cluster, namespace string) ([]*domain.Migration, error)
	CancelMigration(ctx context.Context, cluster, namespace, name string) error
}

// InstanceTypeProvider provides instance type and preference capabilities.
type InstanceTypeProvider interface {
	ListInstanceTypes(ctx context.Context, cluster, namespace string) ([]*domain.InstanceType, error)
	ListClusterInstanceTypes(ctx context.Context, cluster string) ([]*domain.InstanceType, error)
	ListPreferences(ctx context.Context, cluster, namespace string) ([]*domain.Preference, error)
	ListClusterPreferences(ctx context.Context, cluster string) ([]*domain.Preference, error)
}

// ConsoleProvider provides console access capabilities.
type ConsoleProvider interface {
	GetVNCConnection(ctx context.Context, cluster, namespace, name string) (*domain.ConsoleConnection, error)
	GetSerialConsole(ctx context.Context, cluster, namespace, name string) (*domain.ConsoleConnection, error)
}

// KubeVirtProvider is the combined interface for KubeVirt operations.
// Embeds all capability interfaces.
type KubeVirtProvider interface {
	InfrastructureProvider
	SnapshotProvider
	CloneProvider
	MigrationProvider
	InstanceTypeProvider
	ConsoleProvider
}

// ListOptions contains options for list operations.
type ListOptions struct {
	LabelSelector string
	FieldSelector string
	Limit         int
	Continue      string
}

// CredentialProvider provides cluster credentials.
// Strategy pattern for different credential sources.
type CredentialProvider interface {
	// GetRESTConfig returns K8s REST config for the cluster.
	GetRESTConfig(ctx context.Context, clusterName string) (interface{}, error)

	// Type returns the provider type (for logging/debugging).
	Type() string
}
