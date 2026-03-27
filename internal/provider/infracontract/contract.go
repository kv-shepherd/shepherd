package infracontract

import (
	"context"
	"net"

	"kv-shepherd.io/shepherd/internal/domain"
)

type InfrastructureProvider interface {
	Name() string
	Type() string

	GetVM(ctx context.Context, cluster, namespace, name string) (*domain.VM, error)
	ListVMs(ctx context.Context, cluster, namespace string, opts ListOptions) (*domain.VMList, error)
	CreateVM(ctx context.Context, cluster, namespace string, spec *domain.VMSpec) (*domain.VM, error)
	UpdateVM(ctx context.Context, cluster, namespace, name string, spec *domain.VMSpec) (*domain.VM, error)
	DeleteVM(ctx context.Context, cluster, namespace, name string) error

	StartVM(ctx context.Context, cluster, namespace, name string) error
	StopVM(ctx context.Context, cluster, namespace, name string) error
	RestartVM(ctx context.Context, cluster, namespace, name string) error
	PauseVM(ctx context.Context, cluster, namespace, name string) error
	UnpauseVM(ctx context.Context, cluster, namespace, name string) error

	ValidateSpec(ctx context.Context, cluster, namespace string, spec *domain.VMSpec) (*domain.ValidationResult, error)
}

type NamespaceProvisioner interface {
	EnsureNamespace(ctx context.Context, cluster, namespace string) error
}

type ProvisioningQueryProvider interface {
	GetDataVolume(ctx context.Context, cluster, namespace, name string) (*domain.DataVolume, error)
	GetPersistentVolumeClaim(ctx context.Context, cluster, namespace, name string) (*domain.PersistentVolumeClaim, error)
	GetStorageClass(ctx context.Context, cluster, name string) (*domain.StorageClass, error)
	GetStorageProfile(ctx context.Context, cluster, name string) (*domain.StorageProfile, error)
	ListEventsForObject(ctx context.Context, cluster string, ref domain.ObjectReference) ([]domain.ProvisioningEvent, error)
}

type PVCClonePreflightProvider interface {
	GetPersistentVolumeClaim(ctx context.Context, cluster, namespace, name string) (*domain.PersistentVolumeClaim, error)
	GetStorageClass(ctx context.Context, cluster, name string) (*domain.StorageClass, error)
	GetStorageProfile(ctx context.Context, cluster, name string) (*domain.StorageProfile, error)
	ListPodsUsingPVC(ctx context.Context, cluster, namespace, claimName string) ([]domain.ObjectReference, error)
	CanClonePVCSource(ctx context.Context, cluster, namespace string) (bool, string, error)
}

type SnapshotProvider interface {
	CreateSnapshot(ctx context.Context, cluster, namespace, vmName, snapshotName string) (*domain.Snapshot, error)
	GetSnapshot(ctx context.Context, cluster, namespace, name string) (*domain.Snapshot, error)
	ListSnapshots(ctx context.Context, cluster, namespace, vmName string) ([]*domain.Snapshot, error)
	DeleteSnapshot(ctx context.Context, cluster, namespace, name string) error
	RestoreFromSnapshot(ctx context.Context, cluster, namespace, snapshotName, targetVMName string) (*domain.VM, error)
}

type CloneProvider interface {
	CloneVM(ctx context.Context, cluster, namespace, sourceVM, targetName string) (*domain.VM, error)
	CloneFromSnapshot(ctx context.Context, cluster, namespace, snapshotName, targetName string) (*domain.VM, error)
	GetClone(ctx context.Context, cluster, namespace, name string) (*domain.Clone, error)
	ListClones(ctx context.Context, cluster, namespace string) ([]*domain.Clone, error)
}

type MigrationProvider interface {
	MigrateVM(ctx context.Context, cluster, namespace, name string) (*domain.Migration, error)
	GetMigration(ctx context.Context, cluster, namespace, name string) (*domain.Migration, error)
	ListMigrations(ctx context.Context, cluster, namespace string) ([]*domain.Migration, error)
	CancelMigration(ctx context.Context, cluster, namespace, name string) error
}

type InstanceTypeProvider interface {
	ListInstanceTypes(ctx context.Context, cluster, namespace string) ([]*domain.InstanceType, error)
	ListClusterInstanceTypes(ctx context.Context, cluster string) ([]*domain.InstanceType, error)
	ListPreferences(ctx context.Context, cluster, namespace string) ([]*domain.Preference, error)
	ListClusterPreferences(ctx context.Context, cluster string) ([]*domain.Preference, error)
}

type ConsoleProvider interface {
	GetVNCConnection(ctx context.Context, cluster, namespace, name string) (*domain.ConsoleConnection, error)
	GetSerialConsole(ctx context.Context, cluster, namespace, name string) (*domain.ConsoleConnection, error)
}

type VMMutationProvider interface {
	DryRunVMMutation(ctx context.Context, cluster, namespace, name string, mutation *domain.VMMutation) error
	ExecuteVMMutation(ctx context.Context, cluster, namespace, name string, mutation *domain.VMMutation) (*domain.VM, error)
}

type VNCStreamProvider interface {
	OpenVNCStream(ctx context.Context, cluster, namespace, name string) (net.Conn, error)
}

type SerialConsoleStreamProvider interface {
	OpenSerialConsoleStream(ctx context.Context, cluster, namespace, name string) (net.Conn, error)
}

type KubeVirtProvider interface {
	InfrastructureProvider
	SnapshotProvider
	CloneProvider
	MigrationProvider
	InstanceTypeProvider
	ConsoleProvider
}

type ListOptions struct {
	LabelSelector     string
	FieldSelector     string
	Limit             int
	Continue          string
	ResourceVersion   string
	SkipVMIEnrichment bool
}

type CredentialProvider interface {
	GetRESTConfig(ctx context.Context, clusterName string) (interface{}, error)
	Type() string
}
